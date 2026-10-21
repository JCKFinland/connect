package trip

import (
	"context"
	"errors"
	"os"
	"testing"

	stripepayment "github.com/JCKFinland/connect/backend/internal/payments/stripe"
	postgresrepo "github.com/JCKFinland/connect/backend/internal/repository/postgres"
	flexsettlement "github.com/JCKFinland/connect/backend/internal/services/flex_settlement"
	stripego "github.com/stripe/stripe-go/v86"
)

func TestFlexCaptureCancellationPreservesRecovery(t *testing.T) {
	if os.Getenv("CONNECT_REQUIRE_SETTLEMENT_TEST_DB") != "1" {
		t.Skip("requires isolated FLEX settlement database")
	}
	if os.Getenv("FLEX_SETTLEMENT_WORKER_ENABLED") != "false" {
		t.Fatal("background settlement worker must remain disabled")
	}

	ctx := context.Background()
	lifecycle := completeAuthorizedFlexLifecycle(t)
	db := lifecycle.Fixture.DB

	var databaseName string
	if err := db.QueryRow(
		ctx, "SELECT current_database()",
	).Scan(&databaseName); err != nil {
		t.Fatal(err)
	}
	if databaseName != "connect_settlement_test" {
		t.Fatalf("unsafe test database: %q", databaseName)
	}

	settlement, err := postgresrepo.CreateFlexSettlementFromCompletedTrip(
		ctx, db, lifecycle.TripID,
	)
	if err != nil {
		t.Fatal(err)
	}

	verifier := &statefulFlexCaptureVerifier{
		providerID: lifecycle.ProviderID,
	}

	captureCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	markerObserved := false

	capture := &recordingFlexCaptureExecutor{}
	capture.before = func(callCtx context.Context) error {
		if callCtx.Err() != nil {
			return errors.New("capture context cancelled before provider call")
		}

		var marked bool
		if err := db.QueryRow(callCtx, `
            SELECT capture_requested_at IS NOT NULL
            FROM flex_settlements
            WHERE id = $1
        `, settlement.ID).Scan(&marked); err != nil {
			return err
		}
		if !marked {
			return errors.New("capture marker not persisted before provider call")
		}

		markerObserved = true

		// The provider request is now in flight. Cancellation
		// makes its outcome ambiguous to the original caller.
		cancel()

		if !errors.Is(callCtx.Err(), context.Canceled) {
			return errors.New("provider call did not observe cancellation")
		}

		return callCtx.Err()
	}

	orchestrator := flexsettlement.NewOrchestrator(
		db,
		flexsettlement.NewRecoveryService(db, verifier),
	)

	result, err := orchestrator.ExecuteCapture(
		captureCtx, settlement.ID, capture,
	)
	if result != nil {
		t.Fatalf("cancelled capture unexpectedly finalized: %+v", result)
	}
	if !errors.Is(err, flexsettlement.ErrCaptureOutcomeUnresolved) {
		t.Fatalf("expected unresolved capture outcome, got %v", err)
	}
	if !markerObserved {
		t.Fatal("provider call was not reached after durable marker")
	}
	if capture.calls != 1 {
		t.Fatalf("expected one provider capture call, got %d", capture.calls)
	}

	var status string
	var marked bool
	var paidCount int
	var earningCount int
	var successfulCaptures int

	if err := db.QueryRow(ctx, `
        SELECT
            fs.status,
            fs.capture_requested_at IS NOT NULL,
            (
                SELECT COUNT(*)
                FROM payments p
                WHERE p.trip_id = fs.trip_id
                  AND p.status = 'PAID'
            ),
            (
                SELECT COUNT(*)
                FROM driver_earnings de
                WHERE de.trip_id = fs.trip_id
            ),
            (
                SELECT COUNT(*)
                FROM payment_transactions pt
                JOIN payments p ON p.id = pt.payment_id
                WHERE p.trip_id = fs.trip_id
                  AND pt.transaction_type = 'CAPTURE'
                  AND pt.status = 'SUCCESS'
            )
        FROM flex_settlements fs
        WHERE fs.id = $1
    `, settlement.ID).Scan(
		&status,
		&marked,
		&paidCount,
		&earningCount,
		&successfulCaptures,
	); err != nil {
		t.Fatal(err)
	}

	if status != "PROCESSING" ||
		!marked ||
		paidCount != 0 ||
		earningCount != 0 ||
		successfulCaptures != 0 {
		t.Fatalf(
			"unsafe cancelled state: status=%s marker=%t paid=%d earnings=%d captures=%d",
			status, marked, paidCount, earningCount, successfulCaptures,
		)
	}

	// Stripe confirms that the original in-flight request
	// succeeded despite cancellation of the caller.
	verifier.captured = true

	restarted := flexsettlement.NewOrchestrator(
		db,
		flexsettlement.NewRecoveryService(db, verifier),
	)

	recovered, err := restarted.Reconcile(ctx, settlement.ID)
	if err != nil {
		t.Fatalf("reconcile cancelled capture: %v", err)
	}
	if recovered == nil ||
		recovered.Status != flexsettlement.RecoveryCaptured ||
		recovered.ProviderStatus != stripego.PaymentIntentStatusSucceeded {
		t.Fatalf("unexpected recovery result: %+v", recovered)
	}

	if capture.calls != 1 {
		t.Fatalf("recovery duplicated provider capture: %d", capture.calls)
	}

	assertCapturedFlexOrchestratorState(
		t, ctx, db, settlement.ID,
		lifecycle.TripID,
		lifecycle.ProviderID,
		lifecycle.AmountCents,
	)

	replay, err := restarted.ExecuteCapture(
		ctx, settlement.ID, capture,
	)
	if err != nil {
		t.Fatalf("replay finalized capture: %v", err)
	}
	if replay == nil ||
		replay.Status != flexsettlement.RecoveryAlreadyFinalized {
		t.Fatalf("unexpected replay state: %+v", replay)
	}
	if capture.calls != 1 {
		t.Fatalf("replay duplicated provider capture: %d", capture.calls)
	}

	_ = stripepayment.FlexCaptureRequest{}
}
