package postgres_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	postgresrepo "github.com/JCKFinland/connect/backend/internal/repository/postgres"
	"github.com/google/uuid"
)

func setupFlexWebhookEventTest(
	t *testing.T,
) (*flexAuthorizationTestFixture, *postgresrepo.FlexAuthorization) {
	t.Helper()

	f := setupFlexAuthorizationTest(t)
	ctx := context.Background()

	authorization, err := createTestFlexAuthorization(ctx, f)
	if err != nil {
		t.Fatalf("create FLEX authorization: %v", err)
	}

	// Registered after the parent fixture cleanup, so this runs first.
	t.Cleanup(func() {
		_, err := f.DB.Exec(context.Background(), `
			DELETE FROM flex_webhook_events
			WHERE authorization_id = $1
		`, authorization.ID)
		if err != nil {
			t.Errorf("cleanup FLEX webhook events: %v", err)
		}
	})

	return f, authorization
}

func flexWebhookRegistrationRequest(
	authorizationID string,
) postgresrepo.RegisterFlexWebhookEventRequest {
	return postgresrepo.RegisterFlexWebhookEventRequest{
		ProviderEventID:         "evt_" + uuid.NewString(),
		EventType:               "payment_intent.amount_capturable_updated",
		AuthorizationID:         authorizationID,
		ProviderAuthorizationID: "pi_" + uuid.NewString(),
	}
}

func TestRegisterFlexWebhookEventNewAndReplay(t *testing.T) {
	f, authorization := setupFlexWebhookEventTest(t)
	ctx := context.Background()

	request := flexWebhookRegistrationRequest(authorization.ID)

	first, created, err := postgresrepo.RegisterFlexWebhookEvent(
		ctx, f.DB, request,
	)
	if err != nil || !created {
		t.Fatalf("register new event: created=%v err=%v", created, err)
	}

	if first.ID == "" ||
		first.Provider != "STRIPE" ||
		first.ProviderEventID != request.ProviderEventID ||
		first.EventType != request.EventType ||
		first.AuthorizationID != authorization.ID ||
		first.ProviderAuthorizationID != request.ProviderAuthorizationID ||
		first.Status != "RECEIVED" ||
		first.AttemptCount != 0 ||
		first.ProcessingStartedAt != nil ||
		first.ProcessedAt != nil {
		t.Fatalf("unexpected new event: %+v", first)
	}

	second, created, err := postgresrepo.RegisterFlexWebhookEvent(
		ctx, f.DB, request,
	)
	if err != nil || created {
		t.Fatalf("register replay: created=%v err=%v", created, err)
	}

	if second.ID != first.ID ||
		second.Status != first.Status ||
		second.AttemptCount != first.AttemptCount ||
		!second.ReceivedAt.Equal(first.ReceivedAt) ||
		!second.UpdatedAt.Equal(first.UpdatedAt) {
		t.Fatalf("replay changed event state: first=%+v second=%+v",
			first, second)
	}

	var count int
	err = f.DB.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM flex_webhook_events
		WHERE provider = 'STRIPE'
		  AND provider_event_id = $1
	`, request.ProviderEventID).Scan(&count)
	if err != nil || count != 1 {
		t.Fatalf("expected one event: count=%d err=%v", count, err)
	}
}

func TestRegisterFlexWebhookEventIdentityConflict(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*postgresrepo.RegisterFlexWebhookEventRequest)
	}{
		{
			name: "event type",
			mutate: func(r *postgresrepo.RegisterFlexWebhookEventRequest) {
				r.EventType = "payment_intent.canceled"
			},
		},
		{
			name: "authorization",
			mutate: func(r *postgresrepo.RegisterFlexWebhookEventRequest) {
				r.AuthorizationID = uuid.NewString()
			},
		},
		{
			name: "PaymentIntent",
			mutate: func(r *postgresrepo.RegisterFlexWebhookEventRequest) {
				r.ProviderAuthorizationID = "pi_" + uuid.NewString()
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, authorization := setupFlexWebhookEventTest(t)
			ctx := context.Background()

			request := flexWebhookRegistrationRequest(authorization.ID)

			original, created, err :=
				postgresrepo.RegisterFlexWebhookEvent(ctx, f.DB, request)
			if err != nil || !created {
				t.Fatalf("initial registration: %v", err)
			}

			conflicting := request
			tt.mutate(&conflicting)

			_, _, err = postgresrepo.RegisterFlexWebhookEvent(
				ctx, f.DB, conflicting,
			)
			if !errors.Is(
				err,
				postgresrepo.ErrFlexWebhookEventIdentityConflict,
			) {
				t.Fatalf("expected identity conflict, got %v", err)
			}

			replayed, created, err :=
				postgresrepo.RegisterFlexWebhookEvent(ctx, f.DB, request)
			if err != nil || created || replayed.ID != original.ID {
				t.Fatalf("original event changed: created=%v err=%v",
					created, err)
			}
		})
	}
}

func TestRegisterFlexWebhookEventInvalidInput(t *testing.T) {
	f, authorization := setupFlexWebhookEventTest(t)
	ctx := context.Background()

	tests := []struct {
		name   string
		mutate func(*postgresrepo.RegisterFlexWebhookEventRequest)
	}{
		{
			name: "missing event ID",
			mutate: func(r *postgresrepo.RegisterFlexWebhookEventRequest) {
				r.ProviderEventID = ""
			},
		},
		{
			name: "missing event type",
			mutate: func(r *postgresrepo.RegisterFlexWebhookEventRequest) {
				r.EventType = ""
			},
		},
		{
			name: "invalid authorization UUID",
			mutate: func(r *postgresrepo.RegisterFlexWebhookEventRequest) {
				r.AuthorizationID = "invalid"
			},
		},
		{
			name: "missing PaymentIntent",
			mutate: func(r *postgresrepo.RegisterFlexWebhookEventRequest) {
				r.ProviderAuthorizationID = ""
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request := flexWebhookRegistrationRequest(authorization.ID)
			tt.mutate(&request)

			_, _, err := postgresrepo.RegisterFlexWebhookEvent(
				ctx, f.DB, request,
			)
			if !errors.Is(
				err,
				postgresrepo.ErrFlexWebhookEventInvalid,
			) {
				t.Fatalf("expected invalid event, got %v", err)
			}
		})
	}
}

func TestRegisterFlexWebhookEventRejectsMissingAuthorization(
	t *testing.T,
) {
	f, _ := setupFlexWebhookEventTest(t)

	request := flexWebhookRegistrationRequest(uuid.NewString())

	_, _, err := postgresrepo.RegisterFlexWebhookEvent(
		context.Background(), f.DB, request,
	)
	if err == nil {
		t.Fatal("expected foreign-key rejection")
	}
}

func TestRegisterFlexWebhookEventConcurrent(t *testing.T) {
	f, authorization := setupFlexWebhookEventTest(t)
	ctx := context.Background()

	request := flexWebhookRegistrationRequest(authorization.ID)

	const workers = 8

	type result struct {
		event   *postgresrepo.FlexWebhookEvent
		created bool
		err     error
	}

	results := make(chan result, workers)

	var wg sync.WaitGroup

	for i := 0; i < workers; i++ {
		wg.Add(1)

		go func() {
			defer wg.Done()

			event, created, err :=
				postgresrepo.RegisterFlexWebhookEvent(
					context.Background(), f.DB, request,
				)

			results <- result{
				event:   event,
				created: created,
				err:     err,
			}
		}()
	}

	wg.Wait()
	close(results)

	createdCount := 0
	eventID := ""

	for r := range results {
		if r.err != nil {
			t.Fatalf("concurrent registration: %v", r.err)
		}
		if r.event == nil {
			t.Fatal("concurrent registration returned nil event")
		}

		if r.created {
			createdCount++
		}

		if eventID == "" {
			eventID = r.event.ID
		} else if eventID != r.event.ID {
			t.Fatal("concurrent registration returned different records")
		}
	}

	if createdCount != 1 {
		t.Fatalf("expected one creator, got %d", createdCount)
	}

	var count int
	err := f.DB.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM flex_webhook_events
		WHERE provider_event_id = $1
	`, request.ProviderEventID).Scan(&count)
	if err != nil || count != 1 {
		t.Fatalf("expected one persisted event: count=%d err=%v",
			count, err)
	}
}

func TestClaimFlexWebhookEventProcessingLifecycle(t *testing.T) {
	f, authorization := setupFlexWebhookEventTest(t)
	ctx := context.Background()

	request := flexWebhookRegistrationRequest(authorization.ID)

	event, _, err := postgresrepo.RegisterFlexWebhookEvent(
		ctx, f.DB, request,
	)
	if err != nil {
		t.Fatal(err)
	}

	first, claimed, err := postgresrepo.ClaimFlexWebhookEventProcessing(
		ctx, f.DB, event.ID,
	)
	if err != nil || !claimed {
		t.Fatalf("initial claim: claimed=%v err=%v", claimed, err)
	}

	if first.Status != "PROCESSING" ||
		first.AttemptCount != 1 ||
		first.ProcessingStartedAt == nil {
		t.Fatalf("unexpected first claim: %+v", first)
	}

	second, claimed, err := postgresrepo.ClaimFlexWebhookEventProcessing(
		ctx, f.DB, event.ID,
	)
	if err != nil || claimed {
		t.Fatalf("duplicate claim: claimed=%v err=%v", claimed, err)
	}

	if second.AttemptCount != 1 {
		t.Fatalf("duplicate claim changed attempts: %d", second.AttemptCount)
	}

	_, err = f.DB.Exec(ctx, `
		UPDATE flex_webhook_events
		SET processing_started_at =
			clock_timestamp() - INTERVAL '10 minutes'
		WHERE id = $1
	`, event.ID)
	if err != nil {
		t.Fatal(err)
	}

	recovered, claimed, err :=
		postgresrepo.ClaimFlexWebhookEventProcessing(
			ctx, f.DB, event.ID,
		)
	if err != nil || !claimed {
		t.Fatalf("recovery claim: claimed=%v err=%v", claimed, err)
	}

	if recovered.Status != "PROCESSING" ||
		recovered.AttemptCount != 2 {
		t.Fatalf("unexpected recovery: %+v", recovered)
	}

	_, err = f.DB.Exec(ctx, `
		UPDATE flex_webhook_events
		SET status = 'FAILED',
			last_error = 'test retry',
			processing_started_at = NULL
		WHERE id = $1
	`, event.ID)
	if err != nil {
		t.Fatal(err)
	}

	retry, claimed, err := postgresrepo.ClaimFlexWebhookEventProcessing(
		ctx, f.DB, event.ID,
	)
	if err != nil || !claimed {
		t.Fatalf("failed-event retry: claimed=%v err=%v", claimed, err)
	}

	if retry.AttemptCount != 3 ||
		retry.LastError != nil {
		t.Fatalf("unexpected retry state: %+v", retry)
	}

	_, err = f.DB.Exec(ctx, `
		UPDATE flex_webhook_events
		SET status = 'PROCESSED',
			processed_at = clock_timestamp()
		WHERE id = $1
	`, event.ID)
	if err != nil {
		t.Fatal(err)
	}

	processed, claimed, err :=
		postgresrepo.ClaimFlexWebhookEventProcessing(
			ctx, f.DB, event.ID,
		)
	if err != nil || claimed {
		t.Fatalf("processed-event claim: claimed=%v err=%v",
			claimed, err)
	}

	if processed.Status != "PROCESSED" ||
		processed.AttemptCount != 3 {
		t.Fatalf("processed event changed: %+v", processed)
	}
}

func TestClaimFlexWebhookEventProcessingConcurrent(t *testing.T) {
	f, authorization := setupFlexWebhookEventTest(t)
	ctx := context.Background()

	request := flexWebhookRegistrationRequest(authorization.ID)

	event, _, err := postgresrepo.RegisterFlexWebhookEvent(
		ctx, f.DB, request,
	)
	if err != nil {
		t.Fatal(err)
	}

	const workers = 8

	type result struct {
		event   *postgresrepo.FlexWebhookEvent
		claimed bool
		err     error
	}

	results := make(chan result, workers)
	var wg sync.WaitGroup

	for i := 0; i < workers; i++ {
		wg.Add(1)

		go func() {
			defer wg.Done()

			current, claimed, err :=
				postgresrepo.ClaimFlexWebhookEventProcessing(
					context.Background(), f.DB, event.ID,
				)

			results <- result{
				event:   current,
				claimed: claimed,
				err:     err,
			}
		}()
	}

	wg.Wait()
	close(results)

	claimCount := 0

	for r := range results {
		if r.err != nil {
			t.Fatalf("concurrent claim: %v", r.err)
		}

		if r.event == nil {
			t.Fatal("nil event")
		}

		if r.claimed {
			claimCount++
		}

		if r.event.AttemptCount != 1 {
			t.Fatalf("unexpected attempt count: %d",
				r.event.AttemptCount)
		}
	}

	if claimCount != 1 {
		t.Fatalf("expected one successful claim, got %d",
			claimCount)
	}
}

func TestClaimFlexWebhookEventProcessingInvalidID(t *testing.T) {
	f, _ := setupFlexWebhookEventTest(t)

	_, _, err := postgresrepo.ClaimFlexWebhookEventProcessing(
		context.Background(),
		f.DB,
		"invalid",
	)

	if !errors.Is(
		err,
		postgresrepo.ErrFlexWebhookEventInvalid,
	) {
		t.Fatalf("expected invalid event ID, got %v", err)
	}
}

func TestClaimFlexWebhookEventProcessingMissingEvent(t *testing.T) {
	f, _ := setupFlexWebhookEventTest(t)

	_, _, err := postgresrepo.ClaimFlexWebhookEventProcessing(
		context.Background(),
		f.DB,
		uuid.NewString(),
	)

	if !errors.Is(
		err,
		postgresrepo.ErrFlexWebhookEventNotFound,
	) {
		t.Fatalf("expected event not found, got %v", err)
	}
}

func TestCompleteFlexWebhookEventAuthorized(t *testing.T) {
	f, authorization := setupFlexWebhookEventTest(t)
	ctx := context.Background()

	_, claimed, err := postgresrepo.ClaimFlexAuthorizationProcessing(
		ctx, f.DB, authorization.ID,
	)
	if err != nil || !claimed {
		t.Fatalf("claim authorization: claimed=%v err=%v", claimed, err)
	}

	request := flexWebhookRegistrationRequest(authorization.ID)

	event, _, err := postgresrepo.RegisterFlexWebhookEvent(
		ctx, f.DB, request,
	)
	if err != nil {
		t.Fatal(err)
	}

	claim, claimed, err := postgresrepo.ClaimFlexWebhookEventProcessing(
		ctx, f.DB, event.ID,
	)
	if err != nil || !claimed {
		t.Fatalf("claim webhook: claimed=%v err=%v", claimed, err)
	}

	result, err := postgresrepo.CompleteFlexWebhookEvent(
		ctx,
		f.DB,
		event.ID,
		claim.AttemptCount,
		request.ProviderAuthorizationID,
		postgresrepo.FlexResultAuthorized,
	)
	if err != nil {
		t.Fatalf("complete webhook: %v", err)
	}

	if result.Status != "AUTHORIZED" ||
		result.ProviderAuthorizationID == nil ||
		*result.ProviderAuthorizationID != request.ProviderAuthorizationID {
		t.Fatalf("unexpected authorization: %+v", result)
	}

	var eventStatus string
	var processedAtValid bool

	err = f.DB.QueryRow(ctx, `
		SELECT status, processed_at IS NOT NULL
		FROM flex_webhook_events
		WHERE id = $1
	`, event.ID).Scan(&eventStatus, &processedAtValid)
	if err != nil {
		t.Fatal(err)
	}

	if eventStatus != "PROCESSED" || !processedAtValid {
		t.Fatalf(
			"event not completed: status=%s processed_at=%v",
			eventStatus,
			processedAtValid,
		)
	}
}

func TestCompleteFlexWebhookEventRollbackOnReconcileFailure(t *testing.T) {
	f, authorization := setupFlexWebhookEventTest(t)
	ctx := context.Background()

	request := flexWebhookRegistrationRequest(authorization.ID)

	event, _, err := postgresrepo.RegisterFlexWebhookEvent(
		ctx, f.DB, request,
	)
	if err != nil {
		t.Fatal(err)
	}

	claim, claimed, err := postgresrepo.ClaimFlexWebhookEventProcessing(
		ctx, f.DB, event.ID,
	)
	if err != nil || !claimed {
		t.Fatalf("claim webhook: claimed=%v err=%v", claimed, err)
	}

	// Authorization remains PENDING, so reconciliation must fail.
	_, err = postgresrepo.CompleteFlexWebhookEvent(
		ctx,
		f.DB,
		event.ID,
		claim.AttemptCount,
		request.ProviderAuthorizationID,
		postgresrepo.FlexResultAuthorized,
	)
	if !errors.Is(
		err,
		postgresrepo.ErrFlexAuthorizationResultConflict,
	) {
		t.Fatalf("expected reconciliation conflict, got %v", err)
	}

	var eventStatus string
	var authorizationStatus string

	err = f.DB.QueryRow(ctx, `
		SELECT status
		FROM flex_webhook_events
		WHERE id = $1
	`, event.ID).Scan(&eventStatus)
	if err != nil {
		t.Fatal(err)
	}

	err = f.DB.QueryRow(ctx, `
		SELECT status
		FROM flex_authorizations
		WHERE id = $1
	`, authorization.ID).Scan(&authorizationStatus)
	if err != nil {
		t.Fatal(err)
	}

	if eventStatus != "PROCESSING" ||
		authorizationStatus != "PENDING" {
		t.Fatalf(
			"rollback failed: event=%s authorization=%s",
			eventStatus,
			authorizationStatus,
		)
	}
}

func TestCompleteFlexWebhookEventRejectsExpiredLease(t *testing.T) {
	f, authorization := setupFlexWebhookEventTest(t)
	ctx := context.Background()

	_, claimed, err := postgresrepo.ClaimFlexAuthorizationProcessing(
		ctx, f.DB, authorization.ID,
	)
	if err != nil || !claimed {
		t.Fatalf("claim authorization: claimed=%v err=%v", claimed, err)
	}

	request := flexWebhookRegistrationRequest(authorization.ID)

	event, _, err := postgresrepo.RegisterFlexWebhookEvent(
		ctx, f.DB, request,
	)
	if err != nil {
		t.Fatal(err)
	}

	claim, claimed, err := postgresrepo.ClaimFlexWebhookEventProcessing(
		ctx, f.DB, event.ID,
	)
	if err != nil || !claimed {
		t.Fatalf("claim webhook: claimed=%v err=%v", claimed, err)
	}

	_, err = f.DB.Exec(ctx, `
		UPDATE flex_webhook_events
		SET processing_started_at =
			clock_timestamp() - INTERVAL '10 minutes'
		WHERE id = $1
	`, event.ID)
	if err != nil {
		t.Fatal(err)
	}

	_, err = postgresrepo.CompleteFlexWebhookEvent(
		ctx,
		f.DB,
		event.ID,
		claim.AttemptCount,
		request.ProviderAuthorizationID,
		postgresrepo.FlexResultAuthorized,
	)
	if !errors.Is(err, postgresrepo.ErrFlexWebhookClaimConflict) {
		t.Fatalf("expected expired lease rejection, got %v", err)
	}

	var authorizationStatus string
	var eventStatus string

	err = f.DB.QueryRow(ctx, `
		SELECT status
		FROM flex_authorizations
		WHERE id = $1
	`, authorization.ID).Scan(&authorizationStatus)
	if err != nil {
		t.Fatal(err)
	}

	err = f.DB.QueryRow(ctx, `
		SELECT status
		FROM flex_webhook_events
		WHERE id = $1
	`, event.ID).Scan(&eventStatus)
	if err != nil {
		t.Fatal(err)
	}

	if authorizationStatus != "PROCESSING" ||
		eventStatus != "PROCESSING" {
		t.Fatalf(
			"expired worker modified state: authorization=%s event=%s",
			authorizationStatus,
			eventStatus,
		)
	}
}

func TestCompleteFlexWebhookEventRejectsStaleWorker(t *testing.T) {
	f, authorization := setupFlexWebhookEventTest(t)
	ctx := context.Background()

	_, claimed, err := postgresrepo.ClaimFlexAuthorizationProcessing(
		ctx, f.DB, authorization.ID,
	)
	if err != nil || !claimed {
		t.Fatalf("claim authorization: claimed=%v err=%v", claimed, err)
	}

	request := flexWebhookRegistrationRequest(authorization.ID)

	event, _, err := postgresrepo.RegisterFlexWebhookEvent(
		ctx, f.DB, request,
	)
	if err != nil {
		t.Fatal(err)
	}

	first, claimed, err := postgresrepo.ClaimFlexWebhookEventProcessing(
		ctx, f.DB, event.ID,
	)
	if err != nil || !claimed {
		t.Fatalf("first claim: claimed=%v err=%v", claimed, err)
	}

	_, err = f.DB.Exec(ctx, `
		UPDATE flex_webhook_events
		SET processing_started_at =
			clock_timestamp() - INTERVAL '10 minutes'
		WHERE id = $1
	`, event.ID)
	if err != nil {
		t.Fatal(err)
	}

	second, claimed, err := postgresrepo.ClaimFlexWebhookEventProcessing(
		ctx, f.DB, event.ID,
	)
	if err != nil || !claimed {
		t.Fatalf("recovery claim: claimed=%v err=%v", claimed, err)
	}

	if second.AttemptCount != first.AttemptCount+1 {
		t.Fatalf(
			"expected new fencing token: first=%d second=%d",
			first.AttemptCount,
			second.AttemptCount,
		)
	}

	_, err = postgresrepo.CompleteFlexWebhookEvent(
		ctx,
		f.DB,
		event.ID,
		first.AttemptCount,
		request.ProviderAuthorizationID,
		postgresrepo.FlexResultAuthorized,
	)
	if !errors.Is(err, postgresrepo.ErrFlexWebhookClaimConflict) {
		t.Fatalf("expected stale-worker rejection, got %v", err)
	}

	result, err := postgresrepo.CompleteFlexWebhookEvent(
		ctx,
		f.DB,
		event.ID,
		second.AttemptCount,
		request.ProviderAuthorizationID,
		postgresrepo.FlexResultAuthorized,
	)
	if err != nil {
		t.Fatalf("current worker completion: %v", err)
	}

	if result.Status != "AUTHORIZED" {
		t.Fatalf("unexpected authorization status: %s", result.Status)
	}

	var eventStatus string
	var attemptCount int

	err = f.DB.QueryRow(ctx, `
		SELECT status, attempt_count
		FROM flex_webhook_events
		WHERE id = $1
	`, event.ID).Scan(&eventStatus, &attemptCount)
	if err != nil {
		t.Fatal(err)
	}

	if eventStatus != "PROCESSED" ||
		attemptCount != second.AttemptCount {
		t.Fatalf(
			"unexpected event state: status=%s attempt=%d",
			eventStatus,
			attemptCount,
		)
	}
}

func TestReconcileFlexAuthorizationTxRollback(t *testing.T) {
	f, authorization := setupFlexWebhookEventTest(t)
	ctx := context.Background()

	_, claimed, err := postgresrepo.ClaimFlexAuthorizationProcessing(
		ctx, f.DB, authorization.ID,
	)
	if err != nil || !claimed {
		t.Fatalf("claim authorization: claimed=%v err=%v", claimed, err)
	}

	providerID := "pi_" + uuid.NewString()

	tx, err := f.DB.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)

	result, err := postgresrepo.ReconcileFlexAuthorizationTx(
		ctx,
		tx,
		authorization.ID,
		providerID,
		postgresrepo.FlexResultAuthorized,
	)
	if err != nil {
		t.Fatalf("reconcile inside transaction: %v", err)
	}

	if result.Status != "AUTHORIZED" {
		t.Fatalf("unexpected in-transaction status: %s", result.Status)
	}

	// Simulate a failure after reconciliation by rolling back
	// the caller-owned transaction.
	if err := tx.Rollback(ctx); err != nil {
		t.Fatalf("rollback transaction: %v", err)
	}

	var status string
	var storedProviderID *string

	err = f.DB.QueryRow(ctx, `
		SELECT status, provider_authorization_id
		FROM flex_authorizations
		WHERE id = $1
	`, authorization.ID).Scan(&status, &storedProviderID)
	if err != nil {
		t.Fatal(err)
	}

	if status != "PROCESSING" || storedProviderID != nil {
		t.Fatalf(
			"authorization escaped rollback: status=%s provider=%v",
			status,
			storedProviderID,
		)
	}
}

func TestCompleteFlexWebhookEventRollbackOnEventUpdateFailure(t *testing.T) {
	f, authorization := setupFlexWebhookEventTest(t)
	ctx := context.Background()

	_, claimed, err := postgresrepo.ClaimFlexAuthorizationProcessing(
		ctx, f.DB, authorization.ID,
	)
	if err != nil || !claimed {
		t.Fatalf("claim authorization: claimed=%v err=%v", claimed, err)
	}

	request := flexWebhookRegistrationRequest(authorization.ID)

	event, _, err := postgresrepo.RegisterFlexWebhookEvent(
		ctx, f.DB, request,
	)
	if err != nil {
		t.Fatal(err)
	}

	claim, claimed, err := postgresrepo.ClaimFlexWebhookEventProcessing(
		ctx, f.DB, event.ID,
	)
	if err != nil || !claimed {
		t.Fatalf("claim webhook: claimed=%v err=%v", claimed, err)
	}

	// A session-scoped trigger would require controlling the
	// connection used by CompleteFlexWebhookEvent.
	//
	// Instead, use a transaction-independent database constraint:
	// temporarily set the event's processed_at constraint to
	// reject PROCESSED transitions.
	//
	// This test must not modify shared database constraints,
	// so the failure is simulated using a test-specific trigger
	// that targets only this event's UUID.

	triggerName := "flex_webhook_test_fail_" +
		strings.ReplaceAll(uuid.NewString(), "-", "")

	functionName := triggerName + "_fn"

	quotedEventID := "'" + event.ID + "'"

	createFunction := fmt.Sprintf(`
		CREATE FUNCTION %s()
		RETURNS trigger
		LANGUAGE plpgsql
		AS $$
		BEGIN
			IF NEW.id = %s::uuid
				AND NEW.status = 'PROCESSED' THEN
				RAISE EXCEPTION
					'forced FLEX webhook completion failure';
			END IF;
			RETURN NEW;
		END;
		$$
	`, functionName, quotedEventID)

	_, err = f.DB.Exec(ctx, createFunction)
	if err != nil {
		t.Fatalf("create test trigger function: %v", err)
	}

	t.Cleanup(func() {
		_, _ = f.DB.Exec(
			context.Background(),
			fmt.Sprintf("DROP FUNCTION IF EXISTS %s() CASCADE", functionName),
		)
	})

	_, err = f.DB.Exec(ctx, fmt.Sprintf(`
		CREATE TRIGGER %s
		BEFORE UPDATE ON flex_webhook_events
		FOR EACH ROW
		EXECUTE FUNCTION %s()
	`, triggerName, functionName))
	if err != nil {
		t.Fatalf("create test trigger: %v", err)
	}

	t.Cleanup(func() {
		_, _ = f.DB.Exec(
			context.Background(),
			fmt.Sprintf(
				"DROP TRIGGER IF EXISTS %s ON flex_webhook_events",
				triggerName,
			),
		)
	})

	_, err = postgresrepo.CompleteFlexWebhookEvent(
		ctx,
		f.DB,
		event.ID,
		claim.AttemptCount,
		request.ProviderAuthorizationID,
		postgresrepo.FlexResultAuthorized,
	)
	if err == nil {
		t.Fatal("expected forced webhook completion failure")
	}

	var authorizationStatus string
	var providerID *string

	err = f.DB.QueryRow(ctx, `
		SELECT status, provider_authorization_id
		FROM flex_authorizations
		WHERE id = $1
	`, authorization.ID).Scan(&authorizationStatus, &providerID)
	if err != nil {
		t.Fatal(err)
	}

	var eventStatus string
	var processedAt *time.Time

	err = f.DB.QueryRow(ctx, `
		SELECT status, processed_at
		FROM flex_webhook_events
		WHERE id = $1
	`, event.ID).Scan(&eventStatus, &processedAt)
	if err != nil {
		t.Fatal(err)
	}

	if authorizationStatus != "PROCESSING" ||
		providerID != nil ||
		eventStatus != "PROCESSING" ||
		processedAt != nil {
		t.Fatalf(
			"atomic rollback failed: authorization=%s provider=%v event=%s processed_at=%v",
			authorizationStatus,
			providerID,
			eventStatus,
			processedAt,
		)
	}
}

func TestFailFlexWebhookEventImmediateRetry(t *testing.T) {
	f, authorization := setupFlexWebhookEventTest(t)
	ctx := context.Background()

	request := flexWebhookRegistrationRequest(authorization.ID)

	event, _, err := postgresrepo.RegisterFlexWebhookEvent(
		ctx, f.DB, request,
	)
	if err != nil {
		t.Fatal(err)
	}

	first, claimed, err := postgresrepo.ClaimFlexWebhookEventProcessing(
		ctx, f.DB, event.ID,
	)
	if err != nil || !claimed {
		t.Fatalf("first claim: claimed=%v err=%v", claimed, err)
	}

	failed, err := postgresrepo.FailFlexWebhookEvent(
		ctx,
		f.DB,
		event.ID,
		first.AttemptCount,
		"provider_retrieval_failed",
	)
	if err != nil {
		t.Fatal(err)
	}

	if failed.Status != "FAILED" ||
		failed.ProcessingStartedAt != nil ||
		failed.ProcessedAt != nil ||
		failed.LastError == nil ||
		*failed.LastError != "provider_retrieval_failed" {
		t.Fatalf("unexpected failed event: %+v", failed)
	}

	second, claimed, err := postgresrepo.ClaimFlexWebhookEventProcessing(
		ctx, f.DB, event.ID,
	)
	if err != nil || !claimed {
		t.Fatalf("retry claim: claimed=%v err=%v", claimed, err)
	}

	if second.Status != "PROCESSING" ||
		second.AttemptCount != first.AttemptCount+1 ||
		second.LastError != nil {
		t.Fatalf("unexpected retry state: %+v", second)
	}
}

func TestFailFlexWebhookEventRejectsStaleWorker(t *testing.T) {
	f, authorization := setupFlexWebhookEventTest(t)
	ctx := context.Background()

	event, _, err := postgresrepo.RegisterFlexWebhookEvent(
		ctx,
		f.DB,
		flexWebhookRegistrationRequest(authorization.ID),
	)
	if err != nil {
		t.Fatal(err)
	}

	first, claimed, err := postgresrepo.ClaimFlexWebhookEventProcessing(
		ctx, f.DB, event.ID,
	)
	if err != nil || !claimed {
		t.Fatalf("first claim: claimed=%v err=%v", claimed, err)
	}

	_, err = f.DB.Exec(ctx, `
		UPDATE flex_webhook_events
		SET processing_started_at =
			clock_timestamp() - INTERVAL '10 minutes'
		WHERE id = $1
	`, event.ID)
	if err != nil {
		t.Fatal(err)
	}

	second, claimed, err := postgresrepo.ClaimFlexWebhookEventProcessing(
		ctx, f.DB, event.ID,
	)
	if err != nil || !claimed {
		t.Fatalf("replacement claim: claimed=%v err=%v", claimed, err)
	}

	_, err = postgresrepo.FailFlexWebhookEvent(
		ctx, f.DB, event.ID,
		first.AttemptCount,
		"provider_retrieval_failed",
	)
	if !errors.Is(err, postgresrepo.ErrFlexWebhookClaimConflict) {
		t.Fatalf("expected stale-worker conflict, got %v", err)
	}

	var status string
	var attemptCount int

	err = f.DB.QueryRow(ctx, `
		SELECT status, attempt_count
		FROM flex_webhook_events
		WHERE id = $1
	`, event.ID).Scan(&status, &attemptCount)
	if err != nil {
		t.Fatal(err)
	}

	if status != "PROCESSING" ||
		attemptCount != second.AttemptCount {
		t.Fatalf(
			"stale worker changed claim: status=%s attempt=%d",
			status, attemptCount,
		)
	}
}

func TestFailFlexWebhookEventRejectsExpiredLease(t *testing.T) {
	f, authorization := setupFlexWebhookEventTest(t)
	ctx := context.Background()

	event, _, err := postgresrepo.RegisterFlexWebhookEvent(
		ctx,
		f.DB,
		flexWebhookRegistrationRequest(authorization.ID),
	)
	if err != nil {
		t.Fatal(err)
	}

	claim, claimed, err := postgresrepo.ClaimFlexWebhookEventProcessing(
		ctx, f.DB, event.ID,
	)
	if err != nil || !claimed {
		t.Fatalf("claim: claimed=%v err=%v", claimed, err)
	}

	_, err = f.DB.Exec(ctx, `
		UPDATE flex_webhook_events
		SET processing_started_at =
			clock_timestamp() - INTERVAL '10 minutes'
		WHERE id = $1
	`, event.ID)
	if err != nil {
		t.Fatal(err)
	}

	_, err = postgresrepo.FailFlexWebhookEvent(
		ctx, f.DB, event.ID,
		claim.AttemptCount,
		"provider_retrieval_failed",
	)
	if !errors.Is(err, postgresrepo.ErrFlexWebhookClaimConflict) {
		t.Fatalf("expected expired-lease conflict, got %v", err)
	}
}

func TestFailFlexWebhookEventRejectsInvalidInput(t *testing.T) {
	f, authorization := setupFlexWebhookEventTest(t)
	ctx := context.Background()

	event, _, err := postgresrepo.RegisterFlexWebhookEvent(
		ctx,
		f.DB,
		flexWebhookRegistrationRequest(authorization.ID),
	)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name       string
		eventID    string
		attempt    int
		diagnostic string
	}{
		{"invalid_uuid", "invalid", 1, "provider_retrieval_failed"},
		{"invalid_attempt", event.ID, 0, "provider_retrieval_failed"},
		{"empty_diagnostic", event.ID, 1, ""},
		{"unsafe_diagnostic", event.ID, 1, "Stripe API key: secret"},
		{"oversized_diagnostic", event.ID, 1, strings.Repeat("a", 101)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := postgresrepo.FailFlexWebhookEvent(
				ctx, f.DB, tt.eventID,
				tt.attempt, tt.diagnostic,
			)
			if !errors.Is(err, postgresrepo.ErrFlexWebhookEventInvalid) {
				t.Fatalf("expected invalid event error, got %v", err)
			}
		})
	}
}

func TestFailFlexWebhookEventRejectsProcessedEvent(t *testing.T) {
	f, authorization := setupFlexWebhookEventTest(t)
	ctx := context.Background()

	event, _, err := postgresrepo.RegisterFlexWebhookEvent(
		ctx,
		f.DB,
		flexWebhookRegistrationRequest(authorization.ID),
	)
	if err != nil {
		t.Fatal(err)
	}

	claim, claimed, err := postgresrepo.ClaimFlexWebhookEventProcessing(
		ctx, f.DB, event.ID,
	)
	if err != nil || !claimed {
		t.Fatalf("claim: claimed=%v err=%v", claimed, err)
	}

	// Complete the webhook through the real atomic completion path.
	// The authorization must first be in PROCESSING.
	_, authClaimed, err := postgresrepo.ClaimFlexAuthorizationProcessing(
		ctx, f.DB, authorization.ID,
	)
	if err != nil || !authClaimed {
		t.Fatalf(
			"claim authorization: claimed=%v err=%v",
			authClaimed, err,
		)
	}

	completed, err := postgresrepo.CompleteFlexWebhookEvent(
		ctx,
		f.DB,
		event.ID,
		claim.AttemptCount,
		event.ProviderAuthorizationID,
		postgresrepo.FlexResultAuthorized,
	)
	if err != nil {
		t.Fatalf("complete webhook: %v", err)
	}
	if completed == nil || completed.Status != "AUTHORIZED" {
		t.Fatalf("unexpected authorization: %+v", completed)
	}

	_, err = postgresrepo.FailFlexWebhookEvent(
		ctx,
		f.DB,
		event.ID,
		claim.AttemptCount,
		"provider_retrieval_failed",
	)
	if !errors.Is(err, postgresrepo.ErrFlexWebhookClaimConflict) {
		t.Fatalf("expected processed-event conflict, got %v", err)
	}

	var status string
	var lastError *string
	var processedAt *time.Time

	err = f.DB.QueryRow(ctx, `
		SELECT status, last_error, processed_at
		FROM flex_webhook_events
		WHERE id = $1
	`, event.ID).Scan(
		&status,
		&lastError,
		&processedAt,
	)
	if err != nil {
		t.Fatal(err)
	}

	if status != "PROCESSED" ||
		lastError != nil ||
		processedAt == nil {
		t.Fatalf(
			"processed event was modified: status=%s error=%v processed_at=%v",
			status, lastError, processedAt,
		)
	}
}
