package flex_authorization

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/JCKFinland/connect/backend/internal/payments/stripe"
	postgresrepo "github.com/JCKFinland/connect/backend/internal/repository/postgres"
	"github.com/JCKFinland/connect/backend/internal/services/flex_offer"
	"github.com/JCKFinland/connect/backend/internal/testutil"
	stripego "github.com/stripe/stripe-go/v86"
)

type fakeFlexAdapter struct {
	mu       sync.Mutex
	calls    int
	requests []stripe.FlexAuthorizationRequest
	result   *stripe.FlexAuthorizationResult
	err      error
}

func (f *fakeFlexAdapter) ExecuteFlexAuthorization(
	ctx context.Context,
	req stripe.FlexAuthorizationRequest,
) (*stripe.FlexAuthorizationResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.calls++
	f.requests = append(f.requests, req)

	return f.result, f.err
}

func (f *fakeFlexAdapter) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

type authorizationServiceFixture struct {
	*testutil.FlexAcceptFixture
	passengerID string
	service     *Service
	adapter     *fakeFlexAdapter
}

func setupAuthorizationServiceTest(
	t *testing.T,
) *authorizationServiceFixture {
	t.Helper()

	f := testutil.SetupFlexAcceptFixture(t)
	ctx := context.Background()

	offerService := flex_offer.NewService(
		flex_offer.Dependencies{
			DB:     f.DB,
			Config: f.Config,
		},
	)

	offer, err := offerService.SubmitAcceptFare(
		ctx,
		f.AuctionID,
		f.DriverID,
	)
	if err != nil {
		t.Fatalf("submit FLEX offer: %v", err)
	}

	var passengerID string

	err = f.DB.QueryRow(ctx, `
		SELECT customer_id
		FROM ride_requests
		WHERE id = $1
	`, f.RideID).Scan(&passengerID)
	if err != nil {
		t.Fatalf("resolve passenger: %v", err)
	}

	_, err = offerService.SelectPassengerOffer(
		ctx,
		f.RideID,
		f.AuctionID,
		offer.ID,
		passengerID,
	)
	if err != nil {
		t.Fatalf("select FLEX offer: %v", err)
	}

	t.Cleanup(func() {
		cleanupCtx := context.Background()

		_, err := f.DB.Exec(cleanupCtx, `
    DELETE FROM flex_webhook_events
    WHERE authorization_id IN (
        SELECT id
        FROM flex_authorizations
        WHERE auction_id = $1
    )
`, f.AuctionID)
		if err != nil {
			t.Errorf("cleanup FLEX webhook events: %v", err)
		}

		_, err = f.DB.Exec(cleanupCtx, `
    DELETE FROM flex_authorizations
    WHERE auction_id = $1
       `, f.AuctionID)
		if err != nil {
			t.Errorf("cleanup authorization: %v", err)
		}

		_, err = f.DB.Exec(cleanupCtx, `
			UPDATE flex_auctions
			SET status = 'OPEN',
				selected_offer_id = NULL,
				resolved_at = NULL,
				updated_at = clock_timestamp()
			WHERE id = $1
			  AND status = 'PENDING_AUTHORIZATION'
		`, f.AuctionID)
		if err != nil {
			t.Errorf("release selected offer: %v", err)
		}
	})

	adapter := &fakeFlexAdapter{}

	service := NewService(Dependencies{
		DB:      f.DB,
		Adapter: adapter,
	})

	return &authorizationServiceFixture{
		FlexAcceptFixture: f,
		passengerID:       passengerID,
		service:           service,
		adapter:           adapter,
	}
}

func TestExecuteFlexAuthorizationAuthorized(t *testing.T) {
	f := setupAuthorizationServiceTest(t)

	f.adapter.result = &stripe.FlexAuthorizationResult{
		ProviderAuthorizationID: "pi_flex_service_success",
		ProviderStatus:          stripego.PaymentIntentStatusRequiresCapture,
		Authorized:              true,
	}

	result, err := f.service.Execute(
		context.Background(),
		f.RideID,
		f.AuctionID,
		f.passengerID,
	)
	if err != nil {
		t.Fatalf("execute FLEX authorization: %v", err)
	}

	if result == nil ||
		result.Status != "AUTHORIZED" ||
		!result.Authorized ||
		result.ProviderAuthorizationID != "pi_flex_service_success" {
		t.Fatalf("unexpected authorization result: %+v", result)
	}

	if f.adapter.callCount() != 1 {
		t.Fatalf("expected one provider call")
	}

	var status string
	var providerID string

	err = f.DB.QueryRow(context.Background(), `
		SELECT status, provider_authorization_id
		FROM flex_authorizations
		WHERE auction_id = $1
	`, f.AuctionID).Scan(&status, &providerID)
	if err != nil {
		t.Fatalf("read persisted authorization: %v", err)
	}

	if status != "AUTHORIZED" ||
		providerID != "pi_flex_service_success" {
		t.Fatalf(
			"unexpected persisted authorization: %s, %s",
			status,
			providerID,
		)
	}
}

func TestExecuteFlexAuthorizationAuthorizedReplay(t *testing.T) {
	f := setupAuthorizationServiceTest(t)

	f.adapter.result = &stripe.FlexAuthorizationResult{
		ProviderAuthorizationID: "pi_flex_service_replay",
		ProviderStatus:          stripego.PaymentIntentStatusRequiresCapture,
		Authorized:              true,
	}

	ctx := context.Background()

	first, err := f.service.Execute(
		ctx, f.RideID, f.AuctionID, f.passengerID,
	)
	if err != nil {
		t.Fatalf("first execution: %v", err)
	}

	second, err := f.service.Execute(
		ctx, f.RideID, f.AuctionID, f.passengerID,
	)
	if err != nil {
		t.Fatalf("replayed execution: %v", err)
	}

	if first.AuthorizationID != second.AuthorizationID ||
		!second.Authorized ||
		f.adapter.callCount() != 1 {
		t.Fatalf(
			"authorization replay was not idempotent: %+v, %+v",
			first,
			second,
		)
	}
}

func TestExecuteFlexAuthorizationProviderUncertain(
	t *testing.T,
) {
	f := setupAuthorizationServiceTest(t)

	f.adapter.err = errors.New(
		"provider connection interrupted after request",
	)

	ctx := context.Background()

	_, err := f.service.Execute(
		ctx, f.RideID, f.AuctionID, f.passengerID,
	)
	if err == nil {
		t.Fatal("expected provider execution error")
	}

	if f.adapter.callCount() != 1 {
		t.Fatal("expected exactly one provider attempt")
	}

	var authorizationID string
	var status string

	err = f.DB.QueryRow(ctx, `
		SELECT id, status
		FROM flex_authorizations
		WHERE auction_id = $1
	`, f.AuctionID).Scan(&authorizationID, &status)
	if err != nil {
		t.Fatalf("read authorization: %v", err)
	}

	if status != "PROCESSING" {
		t.Fatalf("expected PROCESSING, got %s", status)
	}

	_, err = f.service.Execute(
		ctx, f.RideID, f.AuctionID, f.passengerID,
	)

	if !errors.Is(err, ErrAuthorizationPending) {
		t.Fatalf("expected pending recovery, got %v", err)
	}

	if f.adapter.callCount() != 1 {
		t.Fatal("retry created a duplicate provider attempt")
	}

	authorization, claimed, err :=
		postgresrepo.ClaimFlexAuthorizationProcessing(
			ctx, f.DB, authorizationID,
		)
	if err != nil {
		t.Fatalf("inspect processing claim: %v", err)
	}
	if claimed || authorization.Status != "PROCESSING" {
		t.Fatalf("unexpected processing claim result: %+v", authorization)
	}
}

func TestExecuteFlexAuthorizationCustomerAction(t *testing.T) {
	f := setupAuthorizationServiceTest(t)

	f.adapter.result = &stripe.FlexAuthorizationResult{
		ProviderAuthorizationID: "pi_flex_customer_action",
		ProviderStatus:          stripego.PaymentIntentStatusRequiresAction,
		ClientSecret:            "pi_flex_customer_action_secret_test",
		RequiresCustomerAction:  true,
	}

	ctx := context.Background()

	result, err := f.service.Execute(
		ctx, f.RideID, f.AuctionID, f.passengerID,
	)
	if err != nil {
		t.Fatalf("execute customer-action authorization: %v", err)
	}

	if result == nil ||
		result.Status != "PROCESSING" ||
		result.Authorized ||
		!result.RequiresCustomerAction ||
		result.ClientSecret != "pi_flex_customer_action_secret_test" {
		t.Fatalf("unexpected customer-action result: %+v", result)
	}

	var status string
	var providerID string

	err = f.DB.QueryRow(ctx, `
		SELECT status, provider_authorization_id
		FROM flex_authorizations
		WHERE auction_id = $1
	`, f.AuctionID).Scan(&status, &providerID)
	if err != nil {
		t.Fatal(err)
	}

	if status != "PROCESSING" ||
		providerID != "pi_flex_customer_action" {
		t.Fatalf("unexpected persisted state: %s, %s", status, providerID)
	}
}

func TestExecuteFlexAuthorizationCancelled(t *testing.T) {
	f := setupAuthorizationServiceTest(t)

	f.adapter.result = &stripe.FlexAuthorizationResult{
		ProviderAuthorizationID: "pi_flex_cancelled",
		ProviderStatus:          stripego.PaymentIntentStatusCanceled,
	}

	ctx := context.Background()

	result, err := f.service.Execute(
		ctx, f.RideID, f.AuctionID, f.passengerID,
	)
	if err != nil {
		t.Fatalf("execute cancelled authorization: %v", err)
	}

	if result == nil ||
		result.Status != "CANCELLED" ||
		result.Authorized {
		t.Fatalf("unexpected cancelled result: %+v", result)
	}

	_, err = f.service.Execute(
		ctx, f.RideID, f.AuctionID, f.passengerID,
	)

	if !errors.Is(err, ErrAuthorizationNotEligible) {
		t.Fatalf("expected terminal rejection, got %v", err)
	}

	if f.adapter.callCount() != 1 {
		t.Fatalf(
			"terminal retry contacted provider %d times",
			f.adapter.callCount(),
		)
	}
}

func TestExecuteFlexAuthorizationKnownProviderRecovery(t *testing.T) {
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

	_, err = postgresrepo.ReconcileFlexAuthorization(
		ctx,
		f.DB,
		authorization.ID,
		"pi_flex_known_recovery",
		postgresrepo.FlexResultProcessing,
	)
	if err != nil {
		t.Fatal(err)
	}

	f.adapter.result = &stripe.FlexAuthorizationResult{
		ProviderAuthorizationID: "pi_flex_known_recovery",
		ProviderStatus:          stripego.PaymentIntentStatusRequiresCapture,
		Authorized:              true,
	}

	result, err := f.service.Execute(
		ctx, f.RideID, f.AuctionID, f.passengerID,
	)
	if err != nil {
		t.Fatalf("recover known authorization: %v", err)
	}

	if result == nil ||
		result.Status != "AUTHORIZED" ||
		!result.Authorized {
		t.Fatalf("unexpected recovery result: %+v", result)
	}

	f.adapter.mu.Lock()
	defer f.adapter.mu.Unlock()

	if f.adapter.calls != 1 ||
		len(f.adapter.requests) != 1 ||
		f.adapter.requests[0].ProviderAuthorizationID !=
			"pi_flex_known_recovery" {
		t.Fatalf("recovery did not use known provider identity")
	}
}

func TestExecuteFlexAuthorizationConcurrent(t *testing.T) {
	f := setupAuthorizationServiceTest(t)

	f.adapter.result = &stripe.FlexAuthorizationResult{
		ProviderAuthorizationID: "pi_flex_concurrent",
		ProviderStatus:          stripego.PaymentIntentStatusRequiresCapture,
		Authorized:              true,
	}

	const workers = 8

	var wg sync.WaitGroup
	results := make(chan error, workers)

	for i := 0; i < workers; i++ {
		wg.Add(1)

		go func() {
			defer wg.Done()

			result, err := f.service.Execute(
				context.Background(),
				f.RideID,
				f.AuctionID,
				f.passengerID,
			)

			if err != nil &&
				!errors.Is(err, ErrAuthorizationPending) {
				results <- err
				return
			}

			if err == nil &&
				(result == nil ||
					result.Status != "AUTHORIZED" ||
					!result.Authorized) {
				results <- errors.New(
					"unexpected concurrent authorization result",
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
			t.Fatalf("concurrent authorization: %v", err)
		}
	}

	if f.adapter.callCount() != 1 {
		t.Fatalf(
			"expected one provider call, got %d",
			f.adapter.callCount(),
		)
	}

	var count int

	err := f.DB.QueryRow(context.Background(), `
		SELECT COUNT(*)
		FROM flex_authorizations
		WHERE auction_id = $1
	`, f.AuctionID).Scan(&count)
	if err != nil {
		t.Fatal(err)
	}

	if count != 1 {
		t.Fatalf("expected one authorization row, got %d", count)
	}
}

// TestExecuteFlexAuthorizationRejectsInvalidProviderResponse verifies
// that inconsistent Stripe responses cannot authorize or terminate
// an otherwise unresolved FLEX authorization.
func TestExecuteFlexAuthorizationRejectsInvalidProviderResponse(
	t *testing.T,
) {
	tests := []struct {
		name   string
		result stripe.FlexAuthorizationResult
	}{
		{
			name: "requires capture without authorized flag",
			result: stripe.FlexAuthorizationResult{
				ProviderAuthorizationID: "pi_invalid_capture",
				ProviderStatus:          stripego.PaymentIntentStatusRequiresCapture,
			},
		},
		{
			name: "requires capture with customer action",
			result: stripe.FlexAuthorizationResult{
				ProviderAuthorizationID: "pi_invalid_capture_action",
				ProviderStatus:          stripego.PaymentIntentStatusRequiresCapture,
				Authorized:              true,
				RequiresCustomerAction:  true,
			},
		},
		{
			name: "requires action without action flag",
			result: stripe.FlexAuthorizationResult{
				ProviderAuthorizationID: "pi_invalid_action_flag",
				ProviderStatus:          stripego.PaymentIntentStatusRequiresAction,
				ClientSecret:            "secret_test",
			},
		},
		{
			name: "requires action without client secret",
			result: stripe.FlexAuthorizationResult{
				ProviderAuthorizationID: "pi_invalid_action_secret",
				ProviderStatus:          stripego.PaymentIntentStatusRequiresAction,
				RequiresCustomerAction:  true,
			},
		},
		{
			name: "requires action incorrectly authorized",
			result: stripe.FlexAuthorizationResult{
				ProviderAuthorizationID: "pi_invalid_action_authorized",
				ProviderStatus:          stripego.PaymentIntentStatusRequiresAction,
				Authorized:              true,
				RequiresCustomerAction:  true,
				ClientSecret:            "secret_test",
			},
		},
		{
			name: "processing incorrectly authorized",
			result: stripe.FlexAuthorizationResult{
				ProviderAuthorizationID: "pi_invalid_processing",
				ProviderStatus:          stripego.PaymentIntentStatusProcessing,
				Authorized:              true,
			},
		},
		{
			name: "canceled incorrectly authorized",
			result: stripe.FlexAuthorizationResult{
				ProviderAuthorizationID: "pi_invalid_cancelled",
				ProviderStatus:          stripego.PaymentIntentStatusCanceled,
				Authorized:              true,
			},
		},
		{
			name: "unsupported provider status",
			result: stripe.FlexAuthorizationResult{
				ProviderAuthorizationID: "pi_invalid_status",
				ProviderStatus: stripego.PaymentIntentStatus(
					"unexpected_status",
				),
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := setupAuthorizationServiceTest(t)

			f.adapter.result = &tc.result

			result, err := f.service.Execute(
				context.Background(),
				f.RideID,
				f.AuctionID,
				f.passengerID,
			)

			if err == nil {
				t.Fatal("expected invalid provider response rejection")
			}

			if result != nil && result.Authorized {
				t.Fatalf(
					"invalid response exposed authorization: %+v",
					result,
				)
			}

			var status string
			var providerID *string

			err = f.DB.QueryRow(context.Background(), `
				SELECT status, provider_authorization_id
				FROM flex_authorizations
				WHERE auction_id = $1
			`, f.AuctionID).Scan(&status, &providerID)
			if err != nil {
				t.Fatal(err)
			}

			if status != "PROCESSING" || providerID != nil {
				t.Fatalf(
					"invalid provider response changed persisted state: status=%s provider=%v",
					status,
					providerID,
				)
			}

			if f.adapter.callCount() != 1 {
				t.Fatalf("unexpected provider call count")
			}
		})
	}
}

// TestExecuteFlexAuthorizationRejectsRecoveryIdentityMismatch
// verifies that a known Stripe PaymentIntent cannot be replaced by
// a different provider identity during recovery.
func TestExecuteFlexAuthorizationRejectsRecoveryIdentityMismatch(
	t *testing.T,
) {
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
		t.Fatalf("claim authorization: claimed=%v err=%v", claimed, err)
	}

	_, err = postgresrepo.ReconcileFlexAuthorization(
		ctx,
		f.DB,
		authorization.ID,
		"pi_original_identity",
		postgresrepo.FlexResultProcessing,
	)
	if err != nil {
		t.Fatal(err)
	}

	f.adapter.result = &stripe.FlexAuthorizationResult{
		ProviderAuthorizationID: "pi_conflicting_identity",
		ProviderStatus:          stripego.PaymentIntentStatusRequiresCapture,
		Authorized:              true,
	}

	result, err := f.service.Execute(
		ctx,
		f.RideID,
		f.AuctionID,
		f.passengerID,
	)
	if err == nil {
		t.Fatal("expected provider identity mismatch")
	}

	if result != nil && result.Authorized {
		t.Fatalf("conflicting identity exposed authorization: %+v", result)
	}

	var status string
	var providerID string

	err = f.DB.QueryRow(ctx, `
		SELECT status, provider_authorization_id
		FROM flex_authorizations
		WHERE id = $1
	`, authorization.ID).Scan(&status, &providerID)
	if err != nil {
		t.Fatal(err)
	}

	if status != "PROCESSING" ||
		providerID != "pi_original_identity" {
		t.Fatalf(
			"provider identity was corrupted: %s, %s",
			status,
			providerID,
		)
	}

	if f.adapter.callCount() != 1 {
		t.Fatal("expected exactly one recovery call")
	}
}
