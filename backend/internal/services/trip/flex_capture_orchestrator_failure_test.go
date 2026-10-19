package trip

import (
	"context"
	"errors"
	"os"
	"testing"

	stripepayment "github.com/JCKFinland/connect/backend/internal/payments/stripe"
	"github.com/JCKFinland/connect/backend/internal/repository/postgres"
	flexsettlement "github.com/JCKFinland/connect/backend/internal/services/flex_settlement"
	stripego "github.com/stripe/stripe-go/v86"
)

func TestFlexCaptureOrchestratorProviderFailureRecovery(t *testing.T) {
	if os.Getenv("CONNECT_REQUIRE_SETTLEMENT_TEST_DB") != "1" {
		t.Skip("requires isolated FLEX settlement database")
	}

	lifecycle := completeAuthorizedFlexLifecycle(t)
	ctx := context.Background()
	db := lifecycle.Fixture.DB

	var databaseName string
	if err := db.QueryRow(
		ctx,
		"SELECT current_database()",
	).Scan(&databaseName); err != nil {
		t.Fatal(err)
	}

	if databaseName != "connect_settlement_test" {
		t.Fatalf("refusing settlement test on %q", databaseName)
	}

	settlement, err := postgres.CreateFlexSettlementFromCompletedTrip(
		ctx,
		db,
		lifecycle.TripID,
	)
	if err != nil {
		t.Fatalf("create settlement: %v", err)
	}

	providerFailure := errors.New("simulated Stripe verification failure")

	failingVerifier := &flexRecoveryFakeVerifier{
		err: providerFailure,
	}

	failingOrchestrator := flexsettlement.NewOrchestrator(
		db,
		flexsettlement.NewRecoveryService(db, failingVerifier),
	)

	result, err := failingOrchestrator.Reconcile(
		ctx,
		settlement.ID,
	)

	if result != nil {
		t.Fatalf("expected no result on provider failure: %+v", result)
	}
	if !errors.Is(err, providerFailure) {
		t.Fatalf("expected provider failure, got: %v", err)
	}
	if failingVerifier.calls != 1 {
		t.Fatalf(
			"expected one failed verification, got %d",
			failingVerifier.calls,
		)
	}

	var status string
	var paymentCount int
	var captureCount int
	var earningCount int

	err = db.QueryRow(ctx, `
		SELECT
			fs.status,
			(
				SELECT COUNT(*)
				FROM payments
				WHERE trip_id = fs.trip_id
			),
			(
				SELECT COUNT(*)
				FROM payment_transactions pt
				JOIN payments p ON p.id = pt.payment_id
				WHERE p.trip_id = fs.trip_id
				  AND pt.transaction_type = 'CAPTURE'
			),
			(
				SELECT COUNT(*)
				FROM driver_earnings
				WHERE trip_id = fs.trip_id
			)
		FROM flex_settlements fs
		WHERE fs.id = $1
	`, settlement.ID).Scan(
		&status,
		&paymentCount,
		&captureCount,
		&earningCount,
	)
	if err != nil {
		t.Fatalf("inspect failed reconciliation: %v", err)
	}

	if status != "PENDING" ||
		paymentCount != 0 ||
		captureCount != 0 ||
		earningCount != 0 {
		t.Fatalf(
			"financial mutation after provider failure: "+
				"status=%s payments=%d captures=%d earnings=%d",
			status,
			paymentCount,
			captureCount,
			earningCount,
		)
	}

	successVerifier := &flexRecoveryFakeVerifier{
		result: &stripepayment.FlexCaptureVerificationResult{
			ProviderCaptureID: lifecycle.ProviderID,
			ProviderStatus:    stripego.PaymentIntentStatusSucceeded,
			Captured:          true,
		},
	}

	retryOrchestrator := flexsettlement.NewOrchestrator(
		db,
		flexsettlement.NewRecoveryService(db, successVerifier),
	)

	retry, err := retryOrchestrator.Reconcile(
		ctx,
		settlement.ID,
	)
	if err != nil {
		if errors.Is(err, flexsettlement.ErrSettlementExecutionBusy) {
			t.Fatalf("settlement lock was not released: %v", err)
		}
		t.Fatalf("reconciliation retry failed: %v", err)
	}

	if retry == nil || retry.Status != flexsettlement.RecoveryCaptured {
		t.Fatalf("unexpected retry result: %+v", retry)
	}

	if successVerifier.calls != 1 {
		t.Fatalf(
			"expected one successful verification, got %d",
			successVerifier.calls,
		)
	}

	assertCapturedFlexOrchestratorState(
		t,
		ctx,
		db,
		settlement.ID,
		lifecycle.TripID,
		lifecycle.ProviderID,
		lifecycle.AmountCents,
	)

	finalReplay, err := retryOrchestrator.Reconcile(
		ctx,
		settlement.ID,
	)
	if err != nil {
		t.Fatalf("finalized replay failed: %v", err)
	}
	if finalReplay == nil ||
		finalReplay.Status != flexsettlement.RecoveryAlreadyFinalized {
		t.Fatalf("unexpected finalized replay: %+v", finalReplay)
	}
	if successVerifier.calls != 1 {
		t.Fatal("finalized replay unexpectedly called Stripe")
	}

}
