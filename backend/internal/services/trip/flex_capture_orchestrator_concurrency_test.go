package trip

import (
	"context"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	stripepayment "github.com/JCKFinland/connect/backend/internal/payments/stripe"
	"github.com/JCKFinland/connect/backend/internal/repository/postgres"
	flexsettlement "github.com/JCKFinland/connect/backend/internal/services/flex_settlement"
	stripego "github.com/stripe/stripe-go/v86"
)

type blockingFlexCaptureVerifier struct {
	entered chan struct{}
	release chan struct{}
	once    sync.Once
	result  *stripepayment.FlexCaptureVerificationResult
}

func (v *blockingFlexCaptureVerifier) VerifyFlexCapture(
	ctx context.Context,
	_ stripepayment.FlexCaptureVerificationRequest,
) (*stripepayment.FlexCaptureVerificationResult, error) {
	v.once.Do(func() {
		close(v.entered)
	})

	select {
	case <-v.release:
		return v.result, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func TestFlexCaptureOrchestratorConcurrentReconciliation(t *testing.T) {
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

	verifier := &blockingFlexCaptureVerifier{
		entered: make(chan struct{}),
		release: make(chan struct{}),
		result: &stripepayment.FlexCaptureVerificationResult{
			ProviderCaptureID: lifecycle.ProviderID,
			ProviderStatus:    stripego.PaymentIntentStatusSucceeded,
			Captured:          true,
		},
	}

	first := flexsettlement.NewOrchestrator(
		db,
		flexsettlement.NewRecoveryService(db, verifier),
	)

	secondVerifier := &flexRecoveryFakeVerifier{
		result: verifier.result,
	}

	second := flexsettlement.NewOrchestrator(
		db,
		flexsettlement.NewRecoveryService(db, secondVerifier),
	)

	type reconcileOutcome struct {
		result *flexsettlement.RecoveryResult
		err    error
	}

	firstDone := make(chan reconcileOutcome, 1)

	go func() {
		result, err := first.Reconcile(ctx, settlement.ID)
		firstDone <- reconcileOutcome{result: result, err: err}
	}()

	select {
	case <-verifier.entered:
	case <-time.After(10 * time.Second):
		close(verifier.release)
		t.Fatal("first worker did not reach provider verification")
	}

	busyResult, busyErr := second.Reconcile(ctx, settlement.ID)

	if !errors.Is(
		busyErr,
		flexsettlement.ErrSettlementExecutionBusy,
	) {
		close(verifier.release)
		t.Fatalf(
			"expected settlement execution busy, got result=%+v err=%v",
			busyResult,
			busyErr,
		)
	}
	if busyResult != nil {
		close(verifier.release)
		t.Fatalf("busy worker returned a result: %+v", busyResult)
	}
	if secondVerifier.calls != 0 {
		close(verifier.release)
		t.Fatalf(
			"busy worker unexpectedly called Stripe %d times",
			secondVerifier.calls,
		)
	}

	close(verifier.release)

	select {
	case outcome := <-firstDone:
		if outcome.err != nil {
			t.Fatalf("first worker failed: %v", outcome.err)
		}
		if outcome.result == nil ||
			outcome.result.Status != flexsettlement.RecoveryCaptured {
			t.Fatalf("unexpected first worker result: %+v", outcome.result)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("first worker did not complete")
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

	replay, err := second.Reconcile(ctx, settlement.ID)
	if err != nil {
		t.Fatalf("second worker replay failed: %v", err)
	}
	if replay == nil ||
		replay.Status != flexsettlement.RecoveryAlreadyFinalized {
		t.Fatalf("unexpected second worker replay: %+v", replay)
	}
	if secondVerifier.calls != 0 {
		t.Fatalf(
			"finalized replay unexpectedly called Stripe %d times",
			secondVerifier.calls,
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
}
