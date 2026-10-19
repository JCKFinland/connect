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

type unresolvedPostCaptureVerifier struct {
	calls      int
	providerID string
}

func (v *unresolvedPostCaptureVerifier) VerifyFlexCapture(
	_ context.Context,
	req stripepayment.FlexCaptureVerificationRequest,
) (*stripepayment.FlexCaptureVerificationResult, error) {
	v.calls++

	if req.ProviderAuthorizationID != v.providerID {
		return nil, errors.New("provider identity mismatch")
	}

	if v.calls == 1 {
		return &stripepayment.FlexCaptureVerificationResult{
			ProviderCaptureID: v.providerID,
			ProviderStatus:    stripego.PaymentIntentStatusRequiresCapture,
			Captured:          false,
		}, nil
	}

	return &stripepayment.FlexCaptureVerificationResult{
		ProviderCaptureID: v.providerID,
		ProviderStatus:    stripego.PaymentIntentStatusProcessing,
		Captured:          false,
	}, nil
}

func TestFlexCaptureExecutionUnresolvedPostCaptureVerification(t *testing.T) {
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

	verifier := &unresolvedPostCaptureVerifier{
		providerID: lifecycle.ProviderID,
	}

	executor := &recordingFlexCaptureExecutor{
		result: &stripepayment.FlexCaptureResult{
			ProviderCaptureID: lifecycle.ProviderID,
			ProviderStatus:    stripego.PaymentIntentStatusSucceeded,
		},
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

	if result != nil {
		t.Fatalf("unexpected finalized result: %+v", result)
	}

	if !errors.Is(
		err,
		flexsettlement.ErrCaptureOutcomeUnresolved,
	) {
		t.Fatalf("expected unresolved capture outcome, got %v", err)
	}

	if executor.calls != 1 || verifier.calls != 2 {
		t.Fatalf(
			"unexpected provider calls: captures=%d verifications=%d",
			executor.calls,
			verifier.calls,
		)
	}

	var status string
	var markerExists bool
	var paidCount int
	var earningCount int
	var successfulCaptureCount int

	err = db.QueryRow(ctx, `
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
		&markerExists,
		&paidCount,
		&earningCount,
		&successfulCaptureCount,
	)
	if err != nil {
		t.Fatal(err)
	}

	if status != "PROCESSING" ||
		!markerExists ||
		paidCount != 0 ||
		earningCount != 0 ||
		successfulCaptureCount != 0 {
		t.Fatalf(
			"unsafe unresolved state: status=%s marker=%t paid=%d earnings=%d successful_captures=%d",
			status,
			markerExists,
			paidCount,
			earningCount,
			successfulCaptureCount,
		)
	}

	replay, replayErr := orchestrator.ExecuteCapture(
		ctx,
		settlement.ID,
		executor,
	)

	if replay != nil {
		t.Fatalf("unexpected replay result: %+v", replay)
	}

	if !errors.Is(
		replayErr,
		flexsettlement.ErrCaptureOutcomeUnresolved,
	) {
		t.Fatalf("expected unresolved replay, got %v", replayErr)
	}

	if executor.calls != 1 {
		t.Fatalf(
			"duplicate capture request after unresolved verification: %d",
			executor.calls,
		)
	}

	recovered, recoveryErr := orchestrator.Reconcile(
		ctx,
		settlement.ID,
	)
	if recoveryErr != nil {
		t.Fatalf("reconcile unresolved settlement: %v", recoveryErr)
	}

	if recovered == nil ||
		recovered.Status != flexsettlement.RecoveryPendingProvider {
		t.Fatalf("unexpected recovery result: %+v", recovered)
	}

	if executor.calls != 1 {
		t.Fatalf(
			"reconciliation submitted duplicate capture: %d",
			executor.calls,
		)
	}
}
