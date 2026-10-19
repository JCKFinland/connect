package trip

import (
	"context"
	"errors"
	"testing"

	stripepayment "github.com/JCKFinland/connect/backend/internal/payments/stripe"
	flexsettlement "github.com/JCKFinland/connect/backend/internal/services/flex_settlement"
)

func TestFlexCaptureExecutionTimeoutPreventsReplay(t *testing.T) {
	db, settlementID := prepareFlexCaptureMarkerFixture(t)
	ctx := context.Background()

	var providerID string

	if err := db.QueryRow(ctx, `
		SELECT provider_authorization_id
		FROM flex_settlements
		WHERE id = $1
	`, settlementID).Scan(&providerID); err != nil {
		t.Fatal(err)
	}

	verifier := &statefulFlexCaptureVerifier{
		providerID: providerID,
	}

	providerTimeout := errors.New("simulated Stripe timeout")

	executor := &recordingFlexCaptureExecutor{
		err: providerTimeout,
	}

	executor.before = func(ctx context.Context) error {
		var marked bool

		err := db.QueryRow(ctx, `
			SELECT capture_requested_at IS NOT NULL
			FROM flex_settlements
			WHERE id = $1
		`, settlementID).Scan(&marked)
		if err != nil {
			return err
		}

		if !marked {
			return errors.New(
				"capture marker was not committed before provider call",
			)
		}

		return nil
	}

	orchestrator := flexsettlement.NewOrchestrator(
		db,
		flexsettlement.NewRecoveryService(db, verifier),
	)

	result, err := orchestrator.ExecuteCapture(
		ctx,
		settlementID,
		executor,
	)

	if result != nil {
		t.Fatalf("unexpected timeout result: %+v", result)
	}

	if !errors.Is(
		err,
		flexsettlement.ErrCaptureOutcomeUnresolved,
	) {
		t.Fatalf("expected unresolved capture outcome, got %v", err)
	}

	if executor.calls != 1 {
		t.Fatalf("capture calls = %d, want 1", executor.calls)
	}

	var status string
	var marked bool
	var paidCount int
	var earningCount int

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
			)
		FROM flex_settlements fs
		WHERE fs.id = $1
	`, settlementID).Scan(
		&status,
		&marked,
		&paidCount,
		&earningCount,
	)
	if err != nil {
		t.Fatal(err)
	}

	if status != "PROCESSING" ||
		!marked ||
		paidCount != 0 ||
		earningCount != 0 {
		t.Fatalf(
			"unsafe timeout state: status=%s marked=%t paid=%d earnings=%d",
			status,
			marked,
			paidCount,
			earningCount,
		)
	}

	replay, replayErr := orchestrator.ExecuteCapture(
		ctx,
		settlementID,
		executor,
	)

	if replay != nil {
		t.Fatalf("unexpected replay result: %+v", replay)
	}

	if !errors.Is(
		replayErr,
		flexsettlement.ErrCaptureAlreadyRequested,
	) {
		t.Fatalf(
			"expected replay protection, got %v",
			replayErr,
		)
	}

	if executor.calls != 1 {
		t.Fatalf(
			"unsafe duplicate provider capture: %d calls",
			executor.calls,
		)
	}

	recovery, recoveryErr := orchestrator.Reconcile(
		ctx,
		settlementID,
	)
	if recoveryErr != nil {
		t.Fatalf("reconcile timeout state: %v", recoveryErr)
	}

	if recovery == nil ||
		recovery.Status != flexsettlement.RecoveryReadyForCapture {
		t.Fatalf(
			"unexpected unresolved recovery state: %+v",
			recovery,
		)
	}

	if executor.calls != 1 {
		t.Fatalf(
			"reconciliation submitted another capture: %d",
			executor.calls,
		)
	}

	if verifier.calls != 3 {
		t.Fatalf(
			"expected initial, replay, and reconciliation verification; got %d",
			verifier.calls,
		)
	}

	_ = stripepayment.FlexCaptureRequest{}
}
