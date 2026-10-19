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

func TestFlexCaptureExecutionConcurrentWorkers(t *testing.T) {
	if os.Getenv("CONNECT_REQUIRE_SETTLEMENT_TEST_DB") != "1" {
		t.Skip("requires isolated FLEX settlement test database")
	}

	lifecycle := completeAuthorizedFlexLifecycle(t)
	ctx := context.Background()
	db := lifecycle.Fixture.DB

	var databaseName string
	if err := db.QueryRow(ctx, "SELECT current_database()").
		Scan(&databaseName); err != nil {
		t.Fatal(err)
	}
	if databaseName != "connect_settlement_test" {
		t.Fatalf("refusing concurrency test on %q", databaseName)
	}

	settlement, err := postgres.CreateFlexSettlementFromCompletedTrip(
		ctx, db, lifecycle.TripID,
	)
	if err != nil {
		t.Fatalf("create settlement: %v", err)
	}

	verifier := &statefulFlexCaptureVerifier{
		providerID: lifecycle.ProviderID,
	}

	entered := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once

	firstExecutor := &recordingFlexCaptureExecutor{
		result: &stripepayment.FlexCaptureResult{
			ProviderCaptureID: lifecycle.ProviderID,
			ProviderStatus:    stripego.PaymentIntentStatusSucceeded,
		},
	}

	firstExecutor.before = func(ctx context.Context) error {
		var marked bool
		if err := db.QueryRow(ctx, `
			SELECT capture_requested_at IS NOT NULL
			FROM flex_settlements
			WHERE id = $1
		`, settlement.ID).Scan(&marked); err != nil {
			return err
		}
		if !marked {
			return errors.New("capture marker not committed")
		}

		once.Do(func() { close(entered) })

		select {
		case <-release:
			verifier.captured = true
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	secondExecutor := &recordingFlexCaptureExecutor{
		result: &stripepayment.FlexCaptureResult{
			ProviderCaptureID: lifecycle.ProviderID,
			ProviderStatus:    stripego.PaymentIntentStatusSucceeded,
		},
	}

	firstOrchestrator := flexsettlement.NewOrchestrator(
		db,
		flexsettlement.NewRecoveryService(db, verifier),
	)

	secondOrchestrator := flexsettlement.NewOrchestrator(
		db,
		flexsettlement.NewRecoveryService(db, verifier),
	)

	type executionOutcome struct {
		result *flexsettlement.RecoveryResult
		err    error
	}

	firstDone := make(chan executionOutcome, 1)

	go func() {
		result, err := firstOrchestrator.ExecuteCapture(
			ctx, settlement.ID, firstExecutor,
		)
		firstDone <- executionOutcome{result: result, err: err}
	}()

	select {
	case <-entered:
	case <-time.After(15 * time.Second):
		close(release)
		t.Fatal("first worker never reached provider capture")
	}

	secondResult, secondErr := secondOrchestrator.ExecuteCapture(
		ctx, settlement.ID, secondExecutor,
	)

	if secondResult != nil {
		close(release)
		t.Fatalf("unexpected second worker result: %+v", secondResult)
	}

	if !errors.Is(
		secondErr,
		flexsettlement.ErrSettlementExecutionBusy,
	) {
		close(release)
		t.Fatalf(
			"expected advisory lock contention, got %v",
			secondErr,
		)
	}

	if secondExecutor.calls != 0 {
		close(release)
		t.Fatalf(
			"second worker contacted Stripe: %d calls",
			secondExecutor.calls,
		)
	}

	close(release)

	select {
	case outcome := <-firstDone:
		if outcome.err != nil {
			t.Fatalf("first worker failed: %v", outcome.err)
		}
		if outcome.result == nil ||
			outcome.result.Status != flexsettlement.RecoveryCaptured {
			t.Fatalf("unexpected first result: %+v", outcome.result)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("first worker did not complete")
	}

	if firstExecutor.calls != 1 || secondExecutor.calls != 0 {
		t.Fatalf(
			"unexpected capture counts: first=%d second=%d",
			firstExecutor.calls,
			secondExecutor.calls,
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

	replay, err := secondOrchestrator.ExecuteCapture(
		ctx, settlement.ID, secondExecutor,
	)
	if err != nil {
		t.Fatalf("replay after completion: %v", err)
	}

	if replay == nil ||
		replay.Status != flexsettlement.RecoveryAlreadyFinalized {
		t.Fatalf("unexpected replay result: %+v", replay)
	}

	if secondExecutor.calls != 0 {
		t.Fatalf(
			"replay contacted Stripe: %d calls",
			secondExecutor.calls,
		)
	}
}
