package flex_authorization

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/JCKFinland/connect/backend/internal/payments/stripe"
	postgresrepo "github.com/JCKFinland/connect/backend/internal/repository/postgres"
	"github.com/google/uuid"
	stripego "github.com/stripe/stripe-go/v86"
)

func TestReconcileFlexWebhookAuthorizedRecovery(t *testing.T) {
	f := setupAuthorizationServiceTest(t)
	ctx := context.Background()

	// Create the immutable CONNECT authorization.
	authorization, err := postgresrepo.CreateFlexAuthorization(
		ctx,
		f.DB,
		f.RideID,
		f.AuctionID,
		f.passengerID,
	)
	if err != nil {
		t.Fatalf("create FLEX authorization: %v", err)
	}

	if authorization.ProviderAuthorizationID != nil {
		t.Fatal("expected authorization without provider identity")
	}

	// The application must have initiated authorization processing.
	claimedAuthorization, claimed, err :=
		postgresrepo.ClaimFlexAuthorizationProcessing(
			ctx,
			f.DB,
			authorization.ID,
		)

	if err != nil || !claimed {
		t.Fatalf(
			"claim authorization: claimed=%v err=%v",
			claimed,
			err,
		)
	}

	if claimedAuthorization.Status != "PROCESSING" {
		t.Fatalf(
			"expected PROCESSING, got %s",
			claimedAuthorization.Status,
		)
	}

	const providerID = "pi_flex_webhook_recovery"

	// The fake represents an authoritative Stripe retrieval.
	f.adapter.result = &stripe.FlexAuthorizationResult{
		ProviderAuthorizationID: providerID,
		ProviderStatus:          stripego.PaymentIntentStatusRequiresCapture,
		Authorized:              true,
	}

	// Signature verification happens at the webhook boundary.
	// This event represents the output of that verifier.
	event := &stripe.VerifiedFlexAuthorizationEvent{
		EventID:                 "evt_flex_recovery",
		EventType:               "payment_intent.amount_capturable_updated",
		ProviderAuthorizationID: providerID,
		ProviderStatus:          stripego.PaymentIntentStatusRequiresCapture,
		AuthorizationID:         authorization.ID,
		AuctionID:               authorization.AuctionID,
		OfferID:                 authorization.OfferID,
		PassengerID:             authorization.PassengerID,
		AmountCents:             authorization.AmountCents,
		Currency:                authorization.Currency,
		CaptureMethod:           stripego.PaymentIntentCaptureMethodManual,
	}

	persisted, err := f.service.ReconcileWebhook(ctx, event)
	if err != nil {
		t.Fatalf("reconcile FLEX webhook: %v", err)
	}

	if persisted == nil ||
		persisted.Status != "AUTHORIZED" ||
		persisted.ProviderAuthorizationID == nil ||
		*persisted.ProviderAuthorizationID != providerID {
		t.Fatalf(
			"unexpected reconciliation result: %+v",
			persisted,
		)
	}

	if persisted.ID != authorization.ID ||
		persisted.AuctionID != authorization.AuctionID ||
		persisted.OfferID != authorization.OfferID ||
		persisted.PassengerID != authorization.PassengerID ||
		persisted.AmountCents != authorization.AmountCents ||
		persisted.Currency != authorization.Currency {
		t.Fatalf(
			"authorization identity changed: %+v",
			persisted,
		)
	}

	// The existing adapter must receive a known PaymentIntent ID.
	// Its implementation therefore retrieves rather than creates.
	f.adapter.mu.Lock()

	calls := f.adapter.calls
	requests := append(
		[]stripe.FlexAuthorizationRequest(nil),
		f.adapter.requests...,
	)

	f.adapter.mu.Unlock()

	if calls != 1 || len(requests) != 1 {
		t.Fatalf(
			"expected one provider retrieval, got %d calls",
			calls,
		)
	}

	request := requests[0]

	if request.ProviderAuthorizationID != providerID ||
		request.AuthorizationID != authorization.ID ||
		request.AuctionID != authorization.AuctionID ||
		request.OfferID != authorization.OfferID ||
		request.PassengerID != authorization.PassengerID ||
		request.AmountCents != authorization.AmountCents ||
		request.Currency != authorization.Currency ||
		request.IdempotencyKey != authorization.IdempotencyKey {
		t.Fatalf(
			"incorrect Stripe recovery request: %+v",
			request,
		)
	}

	// Verify the authoritative persisted database state.
	var (
		status            string
		storedProviderID  string
		storedAmountCents int64
	)

	err = f.DB.QueryRow(ctx, `
		SELECT
			status,
			provider_authorization_id,
			ROUND(amount * 100)::bigint
		FROM flex_authorizations
		WHERE id = $1
	`, authorization.ID).Scan(
		&status,
		&storedProviderID,
		&storedAmountCents,
	)
	if err != nil {
		t.Fatalf("read persisted authorization: %v", err)
	}

	if status != "AUTHORIZED" ||
		storedProviderID != providerID ||
		storedAmountCents != authorization.AmountCents {
		t.Fatalf(
			"unexpected database state: %s, %s, %d",
			status,
			storedProviderID,
			storedAmountCents,
		)
	}

	var authorizationCount int

	err = f.DB.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM flex_authorizations
		WHERE auction_id = $1
	`, f.AuctionID).Scan(&authorizationCount)
	if err != nil {
		t.Fatalf("count FLEX authorizations: %v", err)
	}

	if authorizationCount != 1 {
		t.Fatalf(
			"expected exactly one authorization, got %d",
			authorizationCount,
		)
	}
}

func TestReconcileFlexWebhookRejectsPendingAuthorization(t *testing.T) {
	f := setupAuthorizationServiceTest(t)
	ctx := context.Background()

	authorization, err := postgresrepo.CreateFlexAuthorization(
		ctx, f.DB, f.RideID, f.AuctionID, f.passengerID,
	)
	if err != nil {
		t.Fatal(err)
	}

	event := validFlexWebhookTestEvent(
		authorization,
		"pi_flex_pending_rejected",
	)

	_, err = f.service.ReconcileWebhook(ctx, event)

	if !errors.Is(err, ErrFlexWebhookNotReady) {
		t.Fatalf("expected lifecycle rejection, got %v", err)
	}

	if f.adapter.callCount() != 0 {
		t.Fatal("Stripe adapter called for PENDING authorization")
	}

	assertFlexWebhookStoredState(
		t, f, authorization.ID, "PENDING", "",
	)
}

func TestReconcileFlexWebhookRejectsIdentityMismatch(t *testing.T) {
	tests := []struct {
		name   string
		modify func(*stripe.VerifiedFlexAuthorizationEvent)
	}{
		{
			name: "auction",
			modify: func(e *stripe.VerifiedFlexAuthorizationEvent) {
				e.AuctionID = "wrong-auction"
			},
		},
		{
			name: "offer",
			modify: func(e *stripe.VerifiedFlexAuthorizationEvent) {
				e.OfferID = "wrong-offer"
			},
		},
		{
			name: "passenger",
			modify: func(e *stripe.VerifiedFlexAuthorizationEvent) {
				e.PassengerID = "wrong-passenger"
			},
		},
		{
			name: "amount",
			modify: func(e *stripe.VerifiedFlexAuthorizationEvent) {
				e.AmountCents++
			},
		},
		{
			name: "currency",
			modify: func(e *stripe.VerifiedFlexAuthorizationEvent) {
				e.Currency = "USD"
			},
		},
		{
			name: "capture method",
			modify: func(e *stripe.VerifiedFlexAuthorizationEvent) {
				e.CaptureMethod =
					stripego.PaymentIntentCaptureMethodAutomatic
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := setupAuthorizationServiceTest(t)
			ctx := context.Background()

			authorization, err :=
				postgresrepo.CreateFlexAuthorization(
					ctx,
					f.DB,
					f.RideID,
					f.AuctionID,
					f.passengerID,
				)
			if err != nil {
				t.Fatal(err)
			}

			_, claimed, err :=
				postgresrepo.ClaimFlexAuthorizationProcessing(
					ctx, f.DB, authorization.ID,
				)
			if err != nil || !claimed {
				t.Fatalf(
					"claim authorization: %v, %v",
					claimed, err,
				)
			}

			event := validFlexWebhookTestEvent(
				authorization,
				"pi_flex_identity_test",
			)
			tt.modify(event)

			_, err = f.service.ReconcileWebhook(ctx, event)

			if !errors.Is(err, ErrInvalidFlexWebhook) {
				t.Fatalf(
					"expected identity rejection, got %v",
					err,
				)
			}

			if f.adapter.callCount() != 0 {
				t.Fatal("Stripe called before identity validation")
			}

			assertFlexWebhookStoredState(
				t, f, authorization.ID, "PROCESSING", "",
			)
		})
	}
}

func TestReconcileFlexWebhookRejectsStoredProviderMismatch(
	t *testing.T,
) {
	f := setupAuthorizationServiceTest(t)
	ctx := context.Background()

	authorization, err := postgresrepo.CreateFlexAuthorization(
		ctx, f.DB, f.RideID, f.AuctionID, f.passengerID,
	)
	if err != nil {
		t.Fatal(err)
	}

	_, claimed, err :=
		postgresrepo.ClaimFlexAuthorizationProcessing(
			ctx, f.DB, authorization.ID,
		)
	if err != nil || !claimed {
		t.Fatalf("claim authorization: %v, %v", claimed, err)
	}

	_, err = postgresrepo.ReconcileFlexAuthorization(
		ctx,
		f.DB,
		authorization.ID,
		"pi_flex_original",
		postgresrepo.FlexResultProcessing,
	)
	if err != nil {
		t.Fatal(err)
	}

	event := validFlexWebhookTestEvent(
		authorization,
		"pi_flex_substituted",
	)

	_, err = f.service.ReconcileWebhook(ctx, event)

	if !errors.Is(err, ErrInvalidFlexWebhook) {
		t.Fatalf("expected provider mismatch rejection: %v", err)
	}

	if f.adapter.callCount() != 0 {
		t.Fatal("Stripe called for mismatched PaymentIntent")
	}

	assertFlexWebhookStoredState(
		t, f, authorization.ID, "PROCESSING", "pi_flex_original",
	)
}

func validFlexWebhookTestEvent(
	a *postgresrepo.FlexAuthorization,
	providerID string,
) *stripe.VerifiedFlexAuthorizationEvent {
	return &stripe.VerifiedFlexAuthorizationEvent{
		EventID:                 "evt_flex_security",
		EventType:               "payment_intent.amount_capturable_updated",
		ProviderAuthorizationID: providerID,
		ProviderStatus:          stripego.PaymentIntentStatusRequiresCapture,
		AuthorizationID:         a.ID,
		AuctionID:               a.AuctionID,
		OfferID:                 a.OfferID,
		PassengerID:             a.PassengerID,
		AmountCents:             a.AmountCents,
		Currency:                a.Currency,
		CaptureMethod:           stripego.PaymentIntentCaptureMethodManual,
	}
}

func assertFlexWebhookStoredState(
	t *testing.T,
	f *authorizationServiceFixture,
	authorizationID string,
	expectedStatus string,
	expectedProviderID string,
) {
	t.Helper()

	var status string
	var providerID *string

	err := f.DB.QueryRow(context.Background(), `
		SELECT status, provider_authorization_id
		FROM flex_authorizations
		WHERE id = $1
	`, authorizationID).Scan(&status, &providerID)
	if err != nil {
		t.Fatalf("read authorization state: %v", err)
	}

	actualProviderID := ""
	if providerID != nil {
		actualProviderID = *providerID
	}

	if status != expectedStatus ||
		actualProviderID != expectedProviderID {
		t.Fatalf(
			"unexpected stored state: status=%s provider=%s",
			status, actualProviderID,
		)
	}
}

func TestReconcileFlexWebhookAuthorizedReplay(t *testing.T) {
	f := setupAuthorizationServiceTest(t)
	ctx := context.Background()

	authorization, err := postgresrepo.CreateFlexAuthorization(
		ctx, f.DB, f.RideID, f.AuctionID, f.passengerID,
	)
	if err != nil {
		t.Fatal(err)
	}

	_, claimed, err :=
		postgresrepo.ClaimFlexAuthorizationProcessing(
			ctx, f.DB, authorization.ID,
		)
	if err != nil || !claimed {
		t.Fatalf(
			"claim authorization: claimed=%v err=%v",
			claimed, err,
		)
	}

	const providerID = "pi_flex_webhook_replay"

	f.adapter.result = &stripe.FlexAuthorizationResult{
		ProviderAuthorizationID: providerID,
		ProviderStatus:          stripego.PaymentIntentStatusRequiresCapture,
		Authorized:              true,
	}

	event := validFlexWebhookTestEvent(
		authorization,
		providerID,
	)

	first, err := f.service.ReconcileWebhook(ctx, event)
	if err != nil {
		t.Fatalf("first webhook delivery: %v", err)
	}

	if first == nil || first.Status != "AUTHORIZED" {
		t.Fatalf("first delivery did not authorize: %+v", first)
	}

	// Stripe may deliver the same signed event more than once.
	second, err := f.service.ReconcileWebhook(ctx, event)
	if err != nil {
		t.Fatalf("duplicate webhook delivery: %v", err)
	}

	if second == nil ||
		second.ID != first.ID ||
		second.Status != "AUTHORIZED" ||
		second.ProviderAuthorizationID == nil ||
		*second.ProviderAuthorizationID != providerID {
		t.Fatalf(
			"duplicate delivery changed authorization: %+v",
			second,
		)
	}

	assertFlexWebhookStoredState(
		t, f, authorization.ID, "AUTHORIZED", providerID,
	)

	var count int

	err = f.DB.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM flex_authorizations
		WHERE auction_id = $1
	`, f.AuctionID).Scan(&count)
	if err != nil {
		t.Fatal(err)
	}

	if count != 1 {
		t.Fatalf(
			"duplicate delivery created authorizations: %d",
			count,
		)
	}
}

func TestReconcileFlexWebhookRejectsConflictingTerminalEvents(
	t *testing.T,
) {
	tests := []struct {
		name           string
		eventType      string
		providerStatus stripego.PaymentIntentStatus
		providerID     string
	}{
		{
			name:           "delayed processing",
			eventType:      "payment_intent.processing",
			providerStatus: stripego.PaymentIntentStatusProcessing,
			providerID:     "pi_flex_terminal_original",
		},
		{
			name:           "payment failure",
			eventType:      "payment_intent.payment_failed",
			providerStatus: stripego.PaymentIntentStatusRequiresPaymentMethod,
			providerID:     "pi_flex_terminal_original",
		},
		{
			name:           "cancellation",
			eventType:      "payment_intent.canceled",
			providerStatus: stripego.PaymentIntentStatusCanceled,
			providerID:     "pi_flex_terminal_original",
		},
		{
			name:           "different PaymentIntent",
			eventType:      "payment_intent.amount_capturable_updated",
			providerStatus: stripego.PaymentIntentStatusRequiresCapture,
			providerID:     "pi_flex_terminal_substituted",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := setupAuthorizationServiceTest(t)
			ctx := context.Background()

			authorization, err :=
				postgresrepo.CreateFlexAuthorization(
					ctx,
					f.DB,
					f.RideID,
					f.AuctionID,
					f.passengerID,
				)
			if err != nil {
				t.Fatal(err)
			}

			_, claimed, err :=
				postgresrepo.ClaimFlexAuthorizationProcessing(
					ctx,
					f.DB,
					authorization.ID,
				)
			if err != nil || !claimed {
				t.Fatalf(
					"claim authorization: claimed=%v err=%v",
					claimed,
					err,
				)
			}

			const originalProviderID = "pi_flex_terminal_original"

			_, err = postgresrepo.ReconcileFlexAuthorization(
				ctx,
				f.DB,
				authorization.ID,
				originalProviderID,
				postgresrepo.FlexResultAuthorized,
			)
			if err != nil {
				t.Fatalf(
					"establish AUTHORIZED state: %v",
					err,
				)
			}

			event := validFlexWebhookTestEvent(
				authorization,
				tt.providerID,
			)

			event.EventType = tt.eventType
			event.ProviderStatus = tt.providerStatus

			_, err = f.service.ReconcileWebhook(ctx, event)

			if !errors.Is(err, ErrInvalidFlexWebhook) {
				t.Fatalf(
					"expected conflicting event rejection, got %v",
					err,
				)
			}

			if f.adapter.callCount() != 0 {
				t.Fatal(
					"Stripe adapter called for terminal authorization",
				)
			}

			assertFlexWebhookStoredState(
				t,
				f,
				authorization.ID,
				"AUTHORIZED",
				originalProviderID,
			)
		})
	}
}

func TestReconcileFlexWebhookConcurrentDelivery(t *testing.T) {
	f := setupAuthorizationServiceTest(t)
	ctx := context.Background()

	authorization, err := postgresrepo.CreateFlexAuthorization(
		ctx, f.DB, f.RideID, f.AuctionID, f.passengerID,
	)
	if err != nil {
		t.Fatal(err)
	}

	_, claimed, err :=
		postgresrepo.ClaimFlexAuthorizationProcessing(
			ctx, f.DB, authorization.ID,
		)
	if err != nil || !claimed {
		t.Fatalf(
			"claim authorization: claimed=%v err=%v",
			claimed, err,
		)
	}

	const providerID = "pi_flex_webhook_concurrent"

	f.adapter.result = &stripe.FlexAuthorizationResult{
		ProviderAuthorizationID: providerID,
		ProviderStatus:          stripego.PaymentIntentStatusRequiresCapture,
		Authorized:              true,
	}

	event := validFlexWebhookTestEvent(
		authorization,
		providerID,
	)

	const workers = 8

	var wg sync.WaitGroup
	results := make(chan error, workers)

	for i := 0; i < workers; i++ {
		wg.Add(1)

		go func() {
			defer wg.Done()

			result, err := f.service.ReconcileWebhook(
				context.Background(),
				event,
			)

			if err != nil {
				if errors.Is(err, ErrFlexWebhookProcessing) ||
					errors.Is(err, ErrFlexWebhookNotReady) {
					// Another delivery may already own the processing lease,
					// or the authorization may have completed while this
					// request was using an earlier snapshot.
					results <- nil
					return
				}

				results <- err
				return
			}

			if result == nil ||
				result.ID != authorization.ID ||
				result.Status != "AUTHORIZED" ||
				result.ProviderAuthorizationID == nil ||
				*result.ProviderAuthorizationID != providerID {
				results <- fmt.Errorf(
					"unexpected concurrent reconciliation: %+v",
					result,
				)
				return
			}

			results <- nil
		}()
	}

	wg.Wait()
	close(results)

	for err := range results {
		if err != nil {
			t.Fatalf("concurrent webhook delivery: %v", err)
		}
	}

	assertFlexWebhookStoredState(
		t, f, authorization.ID, "AUTHORIZED", providerID,
	)

	var count int

	err = f.DB.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM flex_authorizations
		WHERE auction_id = $1
	`, f.AuctionID).Scan(&count)
	if err != nil {
		t.Fatal(err)
	}

	if count != 1 {
		t.Fatalf(
			"expected one authorization, got %d",
			count,
		)
	}
}

func TestReconcileFlexWebhookProviderFailureImmediateRetry(t *testing.T) {
	f := setupAuthorizationServiceTest(t)
	ctx := context.Background()

	authorization, err := postgresrepo.CreateFlexAuthorization(
		ctx,
		f.DB,
		f.RideID,
		f.AuctionID,
		f.passengerID,
	)
	if err != nil {
		t.Fatal(err)
	}

	_, claimed, err := postgresrepo.ClaimFlexAuthorizationProcessing(
		ctx,
		f.DB,
		authorization.ID,
	)
	if err != nil || !claimed {
		t.Fatalf(
			"claim authorization: claimed=%v err=%v",
			claimed, err,
		)
	}

	const providerID = "pi_flex_failure_immediate_retry"
	const eventID = "evt_flex_failure_immediate_retry"

	event := validFlexWebhookTestEvent(
		authorization,
		providerID,
	)
	event.EventID = eventID

	providerErr := errors.New("simulated Stripe retrieval failure")

	f.adapter.mu.Lock()
	f.adapter.err = providerErr
	f.adapter.result = nil
	f.adapter.mu.Unlock()

	// First delivery: Stripe retrieval fails.
	persisted, err := f.service.ReconcileWebhook(ctx, event)

	if persisted != nil {
		t.Fatalf("unexpected authorization: %+v", persisted)
	}
	if !errors.Is(err, providerErr) {
		t.Fatalf("expected original provider error, got %v", err)
	}

	var (
		webhookID       string
		webhookStatus   string
		attemptCount    int
		lastError       *string
		processingStart *time.Time
		processedAt     *time.Time
	)

	err = f.DB.QueryRow(ctx, `
		SELECT
			id,
			status,
			attempt_count,
			last_error,
			processing_started_at,
			processed_at
		FROM flex_webhook_events
		WHERE provider = 'STRIPE'
		  AND provider_event_id = $1
	`, eventID).Scan(
		&webhookID,
		&webhookStatus,
		&attemptCount,
		&lastError,
		&processingStart,
		&processedAt,
	)
	if err != nil {
		t.Fatal(err)
	}

	if webhookStatus != "FAILED" ||
		attemptCount != 1 ||
		lastError == nil ||
		*lastError != "provider_retrieval_failed" ||
		processingStart != nil ||
		processedAt != nil {
		t.Fatalf(
			"unexpected failed webhook: status=%s attempt=%d "+
				"error=%v processing=%v processed=%v",
			webhookStatus,
			attemptCount,
			lastError,
			processingStart,
			processedAt,
		)
	}

	// Failure must not authorize the payment.
	assertFlexWebhookStoredState(
		t,
		f,
		authorization.ID,
		"PROCESSING",
		"",
	)

	// Restore Stripe success without waiting for lease expiry.
	f.adapter.mu.Lock()
	f.adapter.err = nil
	f.adapter.result = &stripe.FlexAuthorizationResult{
		ProviderAuthorizationID: providerID,
		ProviderStatus:          stripego.PaymentIntentStatusRequiresCapture,
		Authorized:              true,
	}
	f.adapter.mu.Unlock()

	// Second delivery: the identical event is retried.
	persisted, err = f.service.ReconcileWebhook(ctx, event)
	if err != nil {
		t.Fatalf("retry webhook reconciliation: %v", err)
	}

	if persisted == nil ||
		persisted.Status != "AUTHORIZED" ||
		persisted.ProviderAuthorizationID == nil ||
		*persisted.ProviderAuthorizationID != providerID {
		t.Fatalf("unexpected retry result: %+v", persisted)
	}

	err = f.DB.QueryRow(ctx, `
		SELECT
			status,
			attempt_count,
			last_error,
			processing_started_at,
			processed_at
		FROM flex_webhook_events
		WHERE id = $1
	`, webhookID).Scan(
		&webhookStatus,
		&attemptCount,
		&lastError,
		&processingStart,
		&processedAt,
	)
	if err != nil {
		t.Fatal(err)
	}

	if webhookStatus != "PROCESSED" ||
		attemptCount != 2 ||
		lastError != nil ||
		processedAt == nil {
		t.Fatalf(
			"unexpected completed webhook: status=%s attempt=%d "+
				"error=%v processed=%v",
			webhookStatus,
			attemptCount,
			lastError,
			processedAt,
		)
	}

	assertFlexWebhookStoredState(
		t,
		f,
		authorization.ID,
		"AUTHORIZED",
		providerID,
	)

	if calls := f.adapter.callCount(); calls != 2 {
		t.Fatalf("expected two Stripe retrievals, got %d", calls)
	}

	var authorizationCount int

	err = f.DB.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM flex_authorizations
		WHERE auction_id = $1
	`, f.AuctionID).Scan(&authorizationCount)
	if err != nil {
		t.Fatal(err)
	}

	if authorizationCount != 1 {
		t.Fatalf(
			"expected one authorization, got %d",
			authorizationCount,
		)
	}
}

func TestReleaseFailedWebhookClaimAfterRequestCancellation(t *testing.T) {
	f := setupAuthorizationServiceTest(t)
	ctx := context.Background()

	authorization, err := postgresrepo.CreateFlexAuthorization(
		ctx,
		f.DB,
		f.RideID,
		f.AuctionID,
		f.passengerID,
	)
	if err != nil {
		t.Fatal(err)
	}

	const providerID = "pi_flex_cancelled_cleanup"
	const eventID = "evt_flex_cancelled_cleanup"

	event, _, err := postgresrepo.RegisterFlexWebhookEvent(
		ctx,
		f.DB,
		postgresrepo.RegisterFlexWebhookEventRequest{
			ProviderEventID:         eventID,
			EventType:               "payment_intent.amount_capturable_updated",
			AuthorizationID:         authorization.ID,
			ProviderAuthorizationID: providerID,
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	claim, claimed, err := postgresrepo.ClaimFlexWebhookEventProcessing(
		ctx,
		f.DB,
		event.ID,
	)
	if err != nil || !claimed {
		t.Fatalf(
			"claim webhook: claimed=%v err=%v",
			claimed,
			err,
		)
	}

	// Simulate cancellation of the original HTTP request.
	requestCtx, cancel := context.WithCancel(ctx)
	cancel()

	if !errors.Is(requestCtx.Err(), context.Canceled) {
		t.Fatal("expected canceled request context")
	}

	originalErr := errors.New("simulated provider failure")

	releaseErr := f.service.releaseFailedWebhookClaim(
		requestCtx,
		event.ID,
		claim.AttemptCount,
		"provider_retrieval_failed",
		originalErr,
	)

	if !errors.Is(releaseErr, originalErr) {
		t.Fatalf(
			"original processing error was not preserved: %v",
			releaseErr,
		)
	}

	var (
		status            string
		attemptCount      int
		lastError         *string
		processingStarted *time.Time
		processedAt       *time.Time
	)

	err = f.DB.QueryRow(ctx, `
		SELECT
			status,
			attempt_count,
			last_error,
			processing_started_at,
			processed_at
		FROM flex_webhook_events
		WHERE id = $1
	`, event.ID).Scan(
		&status,
		&attemptCount,
		&lastError,
		&processingStarted,
		&processedAt,
	)
	if err != nil {
		t.Fatal(err)
	}

	if status != "FAILED" ||
		attemptCount != claim.AttemptCount ||
		lastError == nil ||
		*lastError != "provider_retrieval_failed" ||
		processingStarted != nil ||
		processedAt != nil {
		t.Fatalf(
			"unexpected webhook after canceled-request cleanup: "+
				"status=%s attempt=%d error=%v processing=%v processed=%v",
			status,
			attemptCount,
			lastError,
			processingStarted,
			processedAt,
		)
	}

	// Verify the failed claim can be reacquired immediately.
	retry, claimed, err := postgresrepo.ClaimFlexWebhookEventProcessing(
		ctx,
		f.DB,
		event.ID,
	)
	if err != nil || !claimed {
		t.Fatalf(
			"retry claim: claimed=%v err=%v",
			claimed,
			err,
		)
	}

	if retry.Status != "PROCESSING" ||
		retry.AttemptCount != claim.AttemptCount+1 {
		t.Fatalf("unexpected retry claim: %+v", retry)
	}
}

func TestReconcileFlexWebhookInvalidProviderResultRecovery(t *testing.T) {
	const providerID = "pi_flex_invalid_provider_recovery"

	tests := []struct {
		name       string
		result     *stripe.FlexAuthorizationResult
		diagnostic string
	}{
		{
			name:       "nil_result",
			result:     nil,
			diagnostic: "provider_identity_invalid",
		},
		{
			name: "wrong_provider_identity",
			result: &stripe.FlexAuthorizationResult{
				ProviderAuthorizationID: "pi_wrong_provider",
				ProviderStatus:          stripego.PaymentIntentStatusRequiresCapture,
				Authorized:              true,
			},
			diagnostic: "provider_identity_invalid",
		},
		{
			name: "requires_capture_without_authorization",
			result: &stripe.FlexAuthorizationResult{
				ProviderAuthorizationID: providerID,
				ProviderStatus:          stripego.PaymentIntentStatusRequiresCapture,
				Authorized:              false,
			},
			diagnostic: "provider_result_invalid",
		},
		{
			name: "canceled_but_authorized",
			result: &stripe.FlexAuthorizationResult{
				ProviderAuthorizationID: providerID,
				ProviderStatus:          stripego.PaymentIntentStatusCanceled,
				Authorized:              true,
			},
			diagnostic: "provider_result_invalid",
		},
		{
			name: "processing_but_authorized",
			result: &stripe.FlexAuthorizationResult{
				ProviderAuthorizationID: providerID,
				ProviderStatus:          stripego.PaymentIntentStatusProcessing,
				Authorized:              true,
			},
			diagnostic: "provider_result_invalid",
		},
		{
			name: "unsupported_provider_status",
			result: &stripe.FlexAuthorizationResult{
				ProviderAuthorizationID: providerID,
				ProviderStatus:          stripego.PaymentIntentStatus("unsupported"),
			},
			diagnostic: "provider_result_invalid",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := setupAuthorizationServiceTest(t)
			ctx := context.Background()

			authorization, err := postgresrepo.CreateFlexAuthorization(
				ctx, f.DB, f.RideID, f.AuctionID, f.passengerID,
			)
			if err != nil {
				t.Fatal(err)
			}

			_, claimed, err := postgresrepo.ClaimFlexAuthorizationProcessing(
				ctx, f.DB, authorization.ID,
			)
			if err != nil || !claimed {
				t.Fatalf("claim authorization: claimed=%v err=%v", claimed, err)
			}

			event := validFlexWebhookTestEvent(authorization, providerID)
			event.EventID = "evt_flex_invalid_" + tt.name

			f.adapter.mu.Lock()
			f.adapter.err = nil
			f.adapter.result = tt.result
			f.adapter.mu.Unlock()

			result, err := f.service.ReconcileWebhook(ctx, event)
			if result != nil || !errors.Is(err, ErrInvalidFlexWebhook) {
				t.Fatalf("expected invalid webhook response, result=%+v err=%v", result, err)
			}

			var (
				webhookID       string
				status          string
				attempt         int
				lastError       *string
				processingStart *time.Time
				processedAt     *time.Time
			)

			err = f.DB.QueryRow(ctx, `
				SELECT id, status, attempt_count, last_error,
				       processing_started_at, processed_at
				FROM flex_webhook_events
				WHERE provider = 'STRIPE'
				  AND provider_event_id = $1
			`, event.EventID).Scan(
				&webhookID,
				&status,
				&attempt,
				&lastError,
				&processingStart,
				&processedAt,
			)
			if err != nil {
				t.Fatal(err)
			}

			if status != "FAILED" ||
				attempt != 1 ||
				lastError == nil ||
				*lastError != tt.diagnostic ||
				processingStart != nil ||
				processedAt != nil {
				t.Fatalf(
					"unexpected failed webhook: status=%s attempt=%d error=%v",
					status, attempt, lastError,
				)
			}

			assertFlexWebhookStoredState(
				t, f, authorization.ID, "PROCESSING", "",
			)

			// Correct the provider response and redeliver the same event.
			f.adapter.mu.Lock()
			f.adapter.result = &stripe.FlexAuthorizationResult{
				ProviderAuthorizationID: providerID,
				ProviderStatus:          stripego.PaymentIntentStatusRequiresCapture,
				Authorized:              true,
			}
			f.adapter.mu.Unlock()

			recovered, err := f.service.ReconcileWebhook(ctx, event)
			if err != nil {
				t.Fatalf("recover webhook: %v", err)
			}
			if recovered == nil || recovered.Status != "AUTHORIZED" {
				t.Fatalf("unexpected recovered authorization: %+v", recovered)
			}

			var finalStatus string
			var finalAttempt int
			var finalError *string
			var finalProcessedAt *time.Time

			err = f.DB.QueryRow(ctx, `
				SELECT status, attempt_count, last_error, processed_at
				FROM flex_webhook_events
				WHERE id = $1
			`, webhookID).Scan(
				&finalStatus,
				&finalAttempt,
				&finalError,
				&finalProcessedAt,
			)
			if err != nil {
				t.Fatal(err)
			}

			if finalStatus != "PROCESSED" ||
				finalAttempt != 2 ||
				finalError != nil ||
				finalProcessedAt == nil {
				t.Fatalf(
					"unexpected recovered webhook: status=%s attempt=%d",
					finalStatus, finalAttempt,
				)
			}

			assertFlexWebhookStoredState(
				t, f, authorization.ID, "AUTHORIZED", providerID,
			)

			if calls := f.adapter.callCount(); calls != 2 {
				t.Fatalf("expected two provider retrievals, got %d", calls)
			}
		})
	}
}

func TestReconcileFlexWebhookAtomicCompletionFailureRecovery(t *testing.T) {
	f := setupAuthorizationServiceTest(t)
	ctx := context.Background()

	authorization, err := postgresrepo.CreateFlexAuthorization(
		ctx, f.DB, f.RideID, f.AuctionID, f.passengerID,
	)
	if err != nil {
		t.Fatal(err)
	}

	_, claimed, err := postgresrepo.ClaimFlexAuthorizationProcessing(
		ctx, f.DB, authorization.ID,
	)
	if err != nil || !claimed {
		t.Fatalf("claim authorization: claimed=%v err=%v", claimed, err)
	}

	const providerID = "pi_flex_atomic_failure_recovery"
	const eventID = "evt_flex_atomic_failure_recovery"

	event := validFlexWebhookTestEvent(authorization, providerID)
	event.EventID = eventID

	f.adapter.mu.Lock()
	f.adapter.err = nil
	f.adapter.result = &stripe.FlexAuthorizationResult{
		ProviderAuthorizationID: providerID,
		ProviderStatus:          stripego.PaymentIntentStatusRequiresCapture,
		Authorized:              true,
	}
	f.adapter.mu.Unlock()

	// Register the webhook before creating the failure trigger,
	// so the trigger can target its exact UUID.
	webhook, _, err := postgresrepo.RegisterFlexWebhookEvent(
		ctx,
		f.DB,
		postgresrepo.RegisterFlexWebhookEventRequest{
			ProviderEventID:         eventID,
			EventType:               event.EventType,
			AuthorizationID:         authorization.ID,
			ProviderAuthorizationID: providerID,
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	triggerName := "flex_webhook_service_fail_" +
		strings.ReplaceAll(uuid.NewString(), "-", "")
	functionName := triggerName + "_fn"

	createFunction := fmt.Sprintf(`
		CREATE FUNCTION %s()
		RETURNS trigger
		LANGUAGE plpgsql
		AS $$
		BEGIN
			IF NEW.id = '%s'::uuid
				AND NEW.status = 'PROCESSED' THEN
				RAISE EXCEPTION
					'forced FLEX webhook completion failure';
			END IF;
			RETURN NEW;
		END;
		$$
	`, functionName, webhook.ID)

	_, err = f.DB.Exec(ctx, createFunction)
	if err != nil {
		t.Fatalf("create failure function: %v", err)
	}

	// Cleanup must be registered immediately after creating
	// the function, including when later assertions fail.
	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(
			context.Background(), 5*time.Second,
		)
		defer cancel()

		if _, err := f.DB.Exec(
			cleanupCtx,
			fmt.Sprintf(
				"DROP FUNCTION IF EXISTS %s() CASCADE",
				functionName,
			),
		); err != nil {
			t.Errorf("cleanup failure function: %v", err)
		}
	})

	_, err = f.DB.Exec(ctx, fmt.Sprintf(`
		CREATE TRIGGER %s
		BEFORE UPDATE ON flex_webhook_events
		FOR EACH ROW
		EXECUTE FUNCTION %s()
	`, triggerName, functionName))
	if err != nil {
		t.Fatalf("create failure trigger: %v", err)
	}

	triggerActive := true

	t.Cleanup(func() {
		if !triggerActive {
			return
		}

		cleanupCtx, cancel := context.WithTimeout(
			context.Background(), 5*time.Second,
		)
		defer cancel()

		if _, err := f.DB.Exec(
			cleanupCtx,
			fmt.Sprintf(
				"DROP TRIGGER IF EXISTS %s ON flex_webhook_events",
				triggerName,
			),
		); err != nil {
			t.Errorf("cleanup failure trigger: %v", err)
		}
	})

	// The provider succeeds, but the database rejects the
	// webhook's PROCESSED transition.
	persisted, err := f.service.ReconcileWebhook(ctx, event)

	if persisted != nil {
		t.Fatalf("unexpected authorization: %+v", persisted)
	}
	if err == nil {
		t.Fatal("expected atomic completion failure")
	}
	if !strings.Contains(
		err.Error(),
		"forced FLEX webhook completion failure",
	) {
		t.Fatalf("original database failure not preserved: %v", err)
	}

	// The authorization update must have rolled back.
	assertFlexWebhookStoredState(
		t, f, authorization.ID, "PROCESSING", "",
	)

	var (
		status          string
		attempt         int
		lastError       *string
		processingStart *time.Time
		processedAt     *time.Time
	)

	err = f.DB.QueryRow(ctx, `
		SELECT status, attempt_count, last_error,
		       processing_started_at, processed_at
		FROM flex_webhook_events
		WHERE id = $1
	`, webhook.ID).Scan(
		&status,
		&attempt,
		&lastError,
		&processingStart,
		&processedAt,
	)
	if err != nil {
		t.Fatal(err)
	}

	if status != "FAILED" ||
		attempt != 1 ||
		lastError == nil ||
		*lastError != "reconciliation_failed" ||
		processingStart != nil ||
		processedAt != nil {
		t.Fatalf(
			"unexpected failed webhook: status=%s attempt=%d "+
				"error=%v processing=%v processed=%v",
			status, attempt, lastError, processingStart, processedAt,
		)
	}

	// Restore normal database operation.
	_, err = f.DB.Exec(
		ctx,
		fmt.Sprintf(
			"DROP TRIGGER IF EXISTS %s ON flex_webhook_events",
			triggerName,
		),
	)
	if err != nil {
		t.Fatalf("remove failure trigger: %v", err)
	}
	triggerActive = false

	// Redeliver the same verified webhook.
	recovered, err := f.service.ReconcileWebhook(ctx, event)
	if err != nil {
		t.Fatalf("retry webhook after database recovery: %v", err)
	}
	if recovered == nil ||
		recovered.Status != "AUTHORIZED" ||
		recovered.ProviderAuthorizationID == nil ||
		*recovered.ProviderAuthorizationID != providerID {
		t.Fatalf("unexpected recovered authorization: %+v", recovered)
	}

	var finalStatus string
	var finalAttempt int
	var finalError *string
	var finalProcessedAt *time.Time

	err = f.DB.QueryRow(ctx, `
		SELECT status, attempt_count, last_error, processed_at
		FROM flex_webhook_events
		WHERE id = $1
	`, webhook.ID).Scan(
		&finalStatus,
		&finalAttempt,
		&finalError,
		&finalProcessedAt,
	)
	if err != nil {
		t.Fatal(err)
	}

	if finalStatus != "PROCESSED" ||
		finalAttempt != 2 ||
		finalError != nil ||
		finalProcessedAt == nil {
		t.Fatalf(
			"unexpected recovered webhook: status=%s "+
				"attempt=%d error=%v processed=%v",
			finalStatus, finalAttempt, finalError, finalProcessedAt,
		)
	}

	assertFlexWebhookStoredState(
		t, f, authorization.ID, "AUTHORIZED", providerID,
	)

	if calls := f.adapter.callCount(); calls != 2 {
		t.Fatalf("expected two provider retrievals, got %d", calls)
	}

	var authorizationCount int

	err = f.DB.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM flex_authorizations
		WHERE auction_id = $1
	`, f.AuctionID).Scan(&authorizationCount)
	if err != nil {
		t.Fatal(err)
	}

	if authorizationCount != 1 {
		t.Fatalf(
			"expected one authorization, got %d",
			authorizationCount,
		)
	}
}
