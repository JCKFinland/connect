package trip

import (
	"context"
	"os"
	"testing"
	"time"

	stripepayment "github.com/JCKFinland/connect/backend/internal/payments/stripe"
	postgresrepo "github.com/JCKFinland/connect/backend/internal/repository/postgres"
	flexsettlement "github.com/JCKFinland/connect/backend/internal/services/flex_settlement"
	stripego "github.com/stripe/stripe-go/v86"
)

func TestFlexSettlementHealthLifecycle(t *testing.T) {
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

	const staleAfter = 30 * time.Second

	read := func() *postgresrepo.FlexSettlementHealthSnapshot {
		t.Helper()

		snapshot, err := postgresrepo.ReadFlexSettlementHealthSnapshot(
			ctx, db, staleAfter,
		)
		if err != nil {
			t.Fatal(err)
		}
		return snapshot
	}

	// Trip completion already creates the PENDING settlement.
	// The baseline therefore includes this settlement.
	settlement, err := postgresrepo.CreateFlexSettlementFromCompletedTrip(
		ctx, db, lifecycle.TripID,
	)
	if err != nil {
		t.Fatal(err)
	}

	if settlement.Status != "PENDING" {
		t.Fatalf("expected PENDING settlement, got %s", settlement.Status)
	}

	baseline := read()

	if baseline.PendingCaptureCount < 1 {
		t.Fatal("new settlement missing from pending monitoring")
	}

	if baseline.StalePendingCaptureCount > baseline.PendingCaptureCount {
		t.Fatal("invalid baseline stale pending count")
	}

	// Preserve this fixture's timestamp even if a later assertion fails.
	var originalCreatedAt time.Time
	if err := db.QueryRow(ctx, `
		SELECT created_at
		FROM flex_settlements
		WHERE id = $1
	`, settlement.ID).Scan(&originalCreatedAt); err != nil {
		t.Fatalf("read original settlement timestamp: %v", err)
	}

	t.Cleanup(func() {
		if _, err := db.Exec(context.Background(), `
			UPDATE flex_settlements
			SET created_at = $2
			WHERE id = $1
		`, settlement.ID, originalCreatedAt); err != nil {
			t.Errorf("restore settlement timestamp: %v", err)
		}
	})

	// Check this fixture directly rather than assuming the database
	// contains no unrelated stale pending settlements.
	if !originalCreatedAt.After(
		baseline.ObservedAt.Add(-staleAfter),
	) {
		t.Fatalf(
			"fixture settlement unexpectedly stale: created=%s observed=%s",
			originalCreatedAt,
			baseline.ObservedAt,
		)
	}

	commandTag, err := db.Exec(ctx, `
		UPDATE flex_settlements
		SET created_at = clock_timestamp() - interval '2 minutes'
		WHERE id = $1
		  AND status = 'PENDING'
		  AND capture_requested_at IS NULL
	`, settlement.ID)
	if err != nil {
		t.Fatalf("age pending settlement: %v", err)
	}
	if commandTag.RowsAffected() != 1 {
		t.Fatalf(
			"expected one aged pending settlement, got %d",
			commandTag.RowsAffected(),
		)
	}

	stalePending := read()

	if stalePending.StalePendingCaptureCount !=
		baseline.StalePendingCaptureCount+1 {
		t.Fatalf(
			"stale pending capture not counted: before=%d after=%d",
			baseline.StalePendingCaptureCount,
			stalePending.StalePendingCaptureCount,
		)
	}

	if stalePending.OldestPendingCaptureAt == nil {
		t.Fatal("oldest pending capture timestamp missing")
	}

	if _, err := db.Exec(ctx, `
		UPDATE flex_settlements
		SET created_at = $2
		WHERE id = $1
	`, settlement.ID, originalCreatedAt); err != nil {
		t.Fatalf("restore pending settlement timestamp: %v", err)
	}

	_, claimed, err := postgresrepo.ClaimFlexSettlementProcessing(
		ctx, db, settlement.ID,
	)
	if err != nil || !claimed {
		t.Fatalf("claim settlement: claimed=%v err=%v", claimed, err)
	}

	processing := read()

	if processing.PendingCaptureCount != baseline.PendingCaptureCount-1 ||
		processing.RecoveryCount != baseline.RecoveryCount+1 {
		t.Fatalf(
			"incorrect PROCESSING classification: pending=%d recovery=%d",
			processing.PendingCaptureCount,
			processing.RecoveryCount,
		)
	}

	if _, err := postgresrepo.PrepareFlexSettlementCapture(
		ctx, db, settlement.ID,
	); err != nil {
		t.Fatalf("prepare capture: %v", err)
	}

	if _, err := postgresrepo.MarkFlexSettlementCaptureRequested(
		ctx, db, settlement.ID,
	); err != nil {
		t.Fatalf("record capture marker: %v", err)
	}

	marked := read()

	if marked.PendingCaptureCount != baseline.PendingCaptureCount-1 ||
		marked.RecoveryCount != baseline.RecoveryCount+1 {
		t.Fatal("durable capture marker lost recovery classification")
	}

	// Age only this test settlement.
	commandTag, err = db.Exec(ctx, `
		UPDATE flex_settlements
		SET created_at = clock_timestamp() - interval '2 minutes'
		WHERE id = $1
		  AND status = 'PROCESSING'
		  AND capture_requested_at IS NOT NULL
	`, settlement.ID)
	if err != nil {
		t.Fatalf("age test settlement: %v", err)
	}
	if commandTag.RowsAffected() != 1 {
		t.Fatalf(
			"expected one aged settlement, got %d",
			commandTag.RowsAffected(),
		)
	}

	aged := read()

	if aged.StaleRecoveryCount != baseline.StaleRecoveryCount+1 {
		t.Fatalf(
			"stale recovery not counted: before=%d after=%d",
			baseline.StaleRecoveryCount,
			aged.StaleRecoveryCount,
		)
	}

	if aged.OldestRecoveryAt == nil {
		t.Fatal("oldest recovery timestamp missing")
	}

	verifier := &flexRecoveryFakeVerifier{
		result: &stripepayment.FlexCaptureVerificationResult{
			ProviderCaptureID: lifecycle.ProviderID,
			ProviderStatus:    stripego.PaymentIntentStatusSucceeded,
			Captured:          true,
		},
	}

	orchestrator := flexsettlement.NewOrchestrator(
		db,
		flexsettlement.NewRecoveryService(db, verifier),
	)

	result, err := orchestrator.Reconcile(ctx, settlement.ID)
	if err != nil {
		t.Fatalf("reconcile captured settlement: %v", err)
	}
	if result == nil {
		t.Fatal("missing reconciliation result")
	}

	captured := read()

	if captured.PendingCaptureCount != baseline.PendingCaptureCount-1 ||
		captured.RecoveryCount != baseline.RecoveryCount ||
		captured.StaleRecoveryCount != baseline.StaleRecoveryCount {
		t.Fatalf(
			"captured settlement remains outstanding: pending=%d recovery=%d stale=%d",
			captured.PendingCaptureCount,
			captured.RecoveryCount,
			captured.StaleRecoveryCount,
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
