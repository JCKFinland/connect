package trip

import (
	"context"
	"os"
	"testing"

	stripepayment "github.com/JCKFinland/connect/backend/internal/payments/stripe"
	postgresrepo "github.com/JCKFinland/connect/backend/internal/repository/postgres"
	flexsettlement "github.com/JCKFinland/connect/backend/internal/services/flex_settlement"
	stripego "github.com/stripe/stripe-go/v86"
)

func TestFlexSettlementDiscoveryWorkerCaptureEndToEnd(t *testing.T) {
	if os.Getenv("CONNECT_REQUIRE_SETTLEMENT_TEST_DB") != "1" {
		t.Skip("requires isolated FLEX settlement database")
	}

	if os.Getenv("FLEX_SETTLEMENT_WORKER_ENABLED") != "false" {
		t.Fatal("settlement background worker must be disabled")
	}

	lifecycle := completeAuthorizedFlexLifecycle(t)
	db := lifecycle.Fixture.DB
	ctx := context.Background()

	var databaseName string
	if err := db.QueryRow(
		ctx,
		"SELECT current_database()",
	).Scan(&databaseName); err != nil {
		t.Fatal(err)
	}

	if databaseName != "connect_settlement_test" {
		t.Fatalf(
			"refusing financial integration test on %q",
			databaseName,
		)
	}

	settlement, err := postgresrepo.CreateFlexSettlementFromCompletedTrip(
		ctx,
		db,
		lifecycle.TripID,
	)
	if err != nil {
		t.Fatalf("create FLEX settlement: %v", err)
	}

	verifier := &statefulFlexCaptureVerifier{
		providerID: lifecycle.ProviderID,
	}

	capture := &recordingFlexCaptureExecutor{
		result: &stripepayment.FlexCaptureResult{
			ProviderCaptureID: lifecycle.ProviderID,
			ProviderStatus:    stripego.PaymentIntentStatusSucceeded,
		},
	}

	capture.before = func(ctx context.Context) error {
		var marked bool

		err := db.QueryRow(ctx, `
            SELECT capture_requested_at IS NOT NULL
            FROM flex_settlements
            WHERE id = $1
        `, settlement.ID).Scan(&marked)
		if err != nil {
			return err
		}

		if !marked {
			return postgresrepo.ErrFlexSettlementInvalidState
		}

		verifier.captured = true
		return nil
	}

	orchestrator := flexsettlement.NewOrchestrator(
		db,
		flexsettlement.NewRecoveryService(db, verifier),
	)

	dispatcher, err := flexsettlement.NewDiscoveryDispatcher(
		orchestrator,
		capture,
	)
	if err != nil {
		t.Fatalf("create discovery dispatcher: %v", err)
	}

	scheduler, err := flexsettlement.NewDiscoveryScheduler(db, 200)
	if err != nil {
		t.Fatalf("create discovery scheduler: %v", err)
	}

	worker, err := flexsettlement.NewDiscoveryWorker(
		scheduler,
		dispatcher,
	)
	if err != nil {
		t.Fatalf("create discovery worker: %v", err)
	}

	candidates, err := scheduler.Next(ctx)
	if err != nil {
		t.Fatalf("discover pending settlement: %v", err)
	}

	kind, found := findFlexSettlementCandidate(
		candidates,
		settlement.ID,
	)

	if !found || kind != postgresrepo.FlexSettlementCandidateCapture {
		t.Fatalf(
			"pending settlement not discovered for capture: found=%v kind=%q",
			found,
			kind,
		)
	}

	if err := worker.RunOnce(ctx, func(err error) {
		t.Errorf("settlement worker dispatch failure: %v", err)
	}); err != nil {
		t.Fatalf("run settlement worker: %v", err)
	}

	if capture.calls != 1 {
		t.Fatalf(
			"expected exactly one provider capture, got %d",
			capture.calls,
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

	if err := worker.RunOnce(ctx, func(err error) {
		t.Errorf("settlement worker replay failure: %v", err)
	}); err != nil {
		t.Fatalf("rerun settlement worker: %v", err)
	}

	if capture.calls != 1 {
		t.Fatalf(
			"unsafe duplicate capture after rediscovery: %d",
			capture.calls,
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

func TestFlexSettlementDiscoveryWorkerRecoveryEndToEnd(t *testing.T) {
	if os.Getenv("CONNECT_REQUIRE_SETTLEMENT_TEST_DB") != "1" {
		t.Skip("requires isolated FLEX settlement database")
	}

	if os.Getenv("FLEX_SETTLEMENT_WORKER_ENABLED") != "false" {
		t.Fatal("settlement background worker must be disabled")
	}

	lifecycle := completeAuthorizedFlexLifecycle(t)
	db := lifecycle.Fixture.DB
	ctx := context.Background()

	var databaseName string
	if err := db.QueryRow(
		ctx,
		"SELECT current_database()",
	).Scan(&databaseName); err != nil {
		t.Fatal(err)
	}

	if databaseName != "connect_settlement_test" {
		t.Fatalf(
			"refusing financial integration test on %q",
			databaseName,
		)
	}

	settlement, err := postgresrepo.CreateFlexSettlementFromCompletedTrip(
		ctx,
		db,
		lifecycle.TripID,
	)
	if err != nil {
		t.Fatalf("create FLEX settlement: %v", err)
	}

	_, claimed, err := postgresrepo.ClaimFlexSettlementProcessing(
		ctx,
		db,
		settlement.ID,
	)
	if err != nil || !claimed {
		t.Fatalf(
			"claim settlement: claimed=%v err=%v",
			claimed,
			err,
		)
	}

	verifier := &statefulFlexCaptureVerifier{
		providerID: lifecycle.ProviderID,
		captured:   true,
	}

	capture := &recordingFlexCaptureExecutor{}

	orchestrator := flexsettlement.NewOrchestrator(
		db,
		flexsettlement.NewRecoveryService(db, verifier),
	)

	dispatcher, err := flexsettlement.NewDiscoveryDispatcher(
		orchestrator,
		capture,
	)
	if err != nil {
		t.Fatalf("create dispatcher: %v", err)
	}

	scheduler, err := flexsettlement.NewDiscoveryScheduler(db, 200)
	if err != nil {
		t.Fatalf("create scheduler: %v", err)
	}

	worker, err := flexsettlement.NewDiscoveryWorker(
		scheduler,
		dispatcher,
	)
	if err != nil {
		t.Fatalf("create worker: %v", err)
	}

	candidates, err := scheduler.Next(ctx)
	if err != nil {
		t.Fatalf("discover recovery settlement: %v", err)
	}

	kind, found := findFlexSettlementCandidate(
		candidates,
		settlement.ID,
	)

	if !found || kind != postgresrepo.FlexSettlementCandidateRecovery {
		t.Fatalf(
			"expected recovery candidate: found=%v kind=%q",
			found,
			kind,
		)
	}

	var dispatchError error

	if err := worker.RunOnce(ctx, func(err error) {
		dispatchError = err
	}); err != nil {
		t.Fatalf("run recovery worker: %v", err)
	}

	if dispatchError != nil {
		t.Fatalf("recovery dispatch failed: %v", dispatchError)
	}

	if capture.calls != 0 {
		t.Fatalf(
			"recovery must not submit a provider capture: %d",
			capture.calls,
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

	if err := worker.RunOnce(ctx, func(err error) {
		t.Errorf("recovery replay failure: %v", err)
	}); err != nil {
		t.Fatalf("rerun recovery worker: %v", err)
	}

	if capture.calls != 0 {
		t.Fatalf(
			"recovery replay submitted provider capture: %d",
			capture.calls,
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
