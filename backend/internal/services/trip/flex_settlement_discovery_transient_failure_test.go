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

func TestFlexSettlementDiscoveryTransientVerificationFailure(t *testing.T) {
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

	settlement, err :=
		postgresrepo.CreateFlexSettlementFromCompletedTrip(
			ctx, db, lifecycle.TripID,
		)
	if err != nil {
		t.Fatal(err)
	}

	_, claimed, err :=
		postgresrepo.ClaimFlexSettlementProcessing(
			ctx, db, settlement.ID,
		)
	if err != nil || !claimed {
		t.Fatalf("claim settlement: claimed=%v err=%v", claimed, err)
	}

	if _, err := postgresrepo.PrepareFlexSettlementCapture(
		ctx, db, settlement.ID,
	); err != nil {
		t.Fatalf("prepare capture: %v", err)
	}

	if _, err := postgresrepo.MarkFlexSettlementCaptureRequested(
		ctx, db, settlement.ID,
	); err != nil {
		t.Fatalf("persist capture request marker: %v", err)
	}

	providerFailure := errors.New("temporary Stripe verification outage")

	verifier := &flexRecoveryFakeVerifier{
		err: providerFailure,
	}
	capture := &recordingFlexCaptureExecutor{}

	orchestrator := flexsettlement.NewOrchestrator(
		db,
		flexsettlement.NewRecoveryService(db, verifier),
	)

	dispatcher, err := flexsettlement.NewDiscoveryDispatcher(
		orchestrator, capture,
	)
	if err != nil {
		t.Fatal(err)
	}

	scheduler, err := flexsettlement.NewDiscoveryScheduler(db, 200)
	if err != nil {
		t.Fatal(err)
	}

	worker, err := flexsettlement.NewDiscoveryWorker(
		scheduler, dispatcher,
	)
	if err != nil {
		t.Fatal(err)
	}

	candidates, err := scheduler.Next(ctx)
	if err != nil {
		t.Fatal(err)
	}

	kind, found := findFlexSettlementCandidate(
		candidates, settlement.ID,
	)
	if !found ||
		kind != postgresrepo.FlexSettlementCandidateRecovery {
		t.Fatalf(
			"expected RECOVERY candidate: found=%v kind=%q",
			found, kind,
		)
	}

	var reported []error

	run := func() {
		t.Helper()
		if err := worker.RunOnce(ctx, func(err error) {
			reported = append(reported, err)
		}); err != nil {
			t.Fatalf("worker run: %v", err)
		}
	}

	assertUnresolved := func() {
		t.Helper()

		var status string
		var markerExists bool
		var paidCount int
		var earningCount int
		var successfulCaptures int

		err := db.QueryRow(ctx, `
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
			&successfulCaptures,
		)
		if err != nil {
			t.Fatal(err)
		}

		if status != "PROCESSING" ||
			!markerExists ||
			paidCount != 0 ||
			earningCount != 0 ||
			successfulCaptures != 0 {
			t.Fatalf(
				"unsafe unresolved state: status=%s marker=%t paid=%d earnings=%d captures=%d",
				status, markerExists, paidCount,
				earningCount, successfulCaptures,
			)
		}
	}

	// First poll: Stripe verification fails.
	run()

	if verifier.calls != 1 {
		t.Fatalf("expected one verification, got %d", verifier.calls)
	}
	if len(reported) != 1 ||
		!errors.Is(reported[0], providerFailure) {
		t.Fatalf("expected reported provider failure: %v", reported)
	}
	if capture.calls != 0 {
		t.Fatalf("unexpected provider capture calls: %d", capture.calls)
	}
	assertUnresolved()

	// Second poll: the worker must suppress an immediate retry.
	run()

	if verifier.calls != 1 {
		t.Fatalf(
			"cooldown failed; verification calls=%d",
			verifier.calls,
		)
	}
	if len(reported) != 1 {
		t.Fatalf("unexpected additional errors: %v", reported)
	}
	assertUnresolved()

	// The provider recovers and confirms the original capture.
	verifier.err = nil
	verifier.result = &stripepayment.FlexCaptureVerificationResult{
		ProviderCaptureID: lifecycle.ProviderID,
		ProviderStatus:    stripego.PaymentIntentStatusSucceeded,
		Captured:          true,
	}

	// A fresh worker models restart after the transient outage.
	// Its cooldown map starts empty, while the database marker persists.
	recoveredWorker, err := flexsettlement.NewDiscoveryWorker(
		scheduler, dispatcher,
	)
	if err != nil {
		t.Fatal(err)
	}

	if err := recoveredWorker.RunOnce(ctx, func(err error) {
		t.Errorf("recovery dispatch failed: %v", err)
	}); err != nil {
		t.Fatal(err)
	}

	if capture.calls != 0 {
		t.Fatalf("recovery submitted provider capture: %d", capture.calls)
	}

	assertCapturedFlexOrchestratorState(
		t, ctx, db, settlement.ID,
		lifecycle.TripID,
		lifecycle.ProviderID,
		lifecycle.AmountCents,
	)

	// The original worker's cooldown must not affect durable correctness.
	run()

	if capture.calls != 0 {
		t.Fatalf("duplicate provider capture: %d", capture.calls)
	}

	assertCapturedFlexOrchestratorState(
		t, ctx, db, settlement.ID,
		lifecycle.TripID,
		lifecycle.ProviderID,
		lifecycle.AmountCents,
	)
}
