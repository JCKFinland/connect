package trip

import (
	"context"
	"os"
	"testing"

	stripepayment "github.com/JCKFinland/connect/backend/internal/payments/stripe"
	"github.com/JCKFinland/connect/backend/internal/repository/postgres"
	flexsettlement "github.com/JCKFinland/connect/backend/internal/services/flex_settlement"
	stripego "github.com/stripe/stripe-go/v86"
)

func TestFlexCaptureExecutionFromPendingSettlement(t *testing.T) {
	if os.Getenv("CONNECT_REQUIRE_SETTLEMENT_TEST_DB") != "1" {
		t.Skip("requires isolated FLEX settlement test database")
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
		t.Fatalf(
			"refusing FLEX capture test on database %q",
			databaseName,
		)
	}

	settlement, err := postgres.CreateFlexSettlementFromCompletedTrip(
		ctx,
		db,
		lifecycle.TripID,
	)
	if err != nil {
		t.Fatalf("create FLEX settlement: %v", err)
	}

	if settlement.Status != "PENDING" {
		t.Fatalf(
			"expected PENDING settlement, got %s",
			settlement.Status,
		)
	}

	verifier := &statefulFlexCaptureVerifier{
		providerID: lifecycle.ProviderID,
	}

	executor := &recordingFlexCaptureExecutor{
		result: &stripepayment.FlexCaptureResult{
			ProviderCaptureID: lifecycle.ProviderID,
			ProviderStatus:    stripego.PaymentIntentStatusSucceeded,
		},
	}

	executor.before = func(ctx context.Context) error {
		var status string
		var marked bool
		var paymentID *string
		var transactionID *string

		err := db.QueryRow(ctx, `
			SELECT
				status,
				capture_requested_at IS NOT NULL,
				payment_id::text,
				capture_transaction_id::text
			FROM flex_settlements
			WHERE id = $1
		`, settlement.ID).Scan(
			&status,
			&marked,
			&paymentID,
			&transactionID,
		)
		if err != nil {
			return err
		}

		if status != "PROCESSING" ||
			!marked ||
			paymentID == nil ||
			transactionID == nil {
			return postgres.ErrFlexSettlementInvalidState
		}

		verifier.captured = true
		return nil
	}

	orchestrator := flexsettlement.NewOrchestrator(
		db,
		flexsettlement.NewRecoveryService(db, verifier),
	)

	result, err := orchestrator.ExecuteCapture(
		ctx,
		settlement.ID,
		executor,
	)
	if err != nil {
		t.Fatalf("execute capture from PENDING: %v", err)
	}

	if result == nil ||
		result.Status != flexsettlement.RecoveryCaptured {
		t.Fatalf("unexpected execution result: %+v", result)
	}

	if executor.calls != 1 {
		t.Fatalf(
			"expected exactly one capture request, got %d",
			executor.calls,
		)
	}

	if verifier.calls != 2 {
		t.Fatalf(
			"expected pre- and post-capture verification, got %d",
			verifier.calls,
		)
	}

	if executor.request.ProviderAuthorizationID != lifecycle.ProviderID ||
		executor.request.AmountCents != lifecycle.AmountCents ||
		executor.request.Currency != lifecycle.Currency ||
		executor.request.CaptureIdempotencyKey !=
			settlement.CaptureIdempotencyKey {
		t.Fatalf(
			"capture request identity mismatch: %+v",
			executor.request,
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

	replay, err := orchestrator.ExecuteCapture(
		ctx,
		settlement.ID,
		executor,
	)
	if err != nil {
		t.Fatalf("replay finalized capture: %v", err)
	}

	if replay == nil ||
		replay.Status != flexsettlement.RecoveryAlreadyFinalized {
		t.Fatalf("unexpected replay result: %+v", replay)
	}

	if executor.calls != 1 {
		t.Fatalf(
			"duplicate capture after finalization: %d calls",
			executor.calls,
		)
	}
}
