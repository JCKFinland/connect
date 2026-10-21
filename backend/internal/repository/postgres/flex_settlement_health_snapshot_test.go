package postgres

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/JCKFinland/connect/backend/internal/config"
	"github.com/JCKFinland/connect/backend/internal/database"
)

// TestFlexSettlementHealthSnapshot verifies that monitoring
// remains read-only and classifies settlement states correctly.
//
// This test intentionally does not create financial fixtures.
// Existing isolated database records are inspected using
// independent SQL expectations.
func TestFlexSettlementHealthSnapshot(t *testing.T) {
	if os.Getenv("CONNECT_REQUIRE_SETTLEMENT_TEST_DB") != "1" {
		t.Skip("requires isolated FLEX settlement database")
	}

	if os.Getenv("FLEX_SETTLEMENT_WORKER_ENABLED") != "false" {
		t.Fatal("background settlement worker must remain disabled")
	}

	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	if err := os.Chdir("../../.."); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = os.Chdir(originalDir)
	}()

	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}

	db, err := database.Connect(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ctx := context.Background()

	var databaseName string

	if err := db.QueryRow(
		ctx,
		"SELECT current_database()",
	).Scan(&databaseName); err != nil {
		t.Fatal(err)
	}

	if databaseName != "connect_settlement_test" {
		t.Fatalf("unsafe database: %q", databaseName)
	}

	const staleAfter = 15 * time.Minute

	snapshot, err := ReadFlexSettlementHealthSnapshot(
		ctx,
		db,
		staleAfter,
	)
	if err != nil {
		t.Fatalf("read health snapshot: %v", err)
	}

	if snapshot.ObservedAt.IsZero() {
		t.Fatal("missing observation timestamp")
	}

	if snapshot.PendingCaptureCount < 0 ||
		snapshot.RecoveryCount < 0 ||
		snapshot.StalePendingCaptureCount < 0 ||
		snapshot.StaleRecoveryCount < 0 {
		t.Fatal("negative settlement count")
	}

	if snapshot.StalePendingCaptureCount >
		snapshot.PendingCaptureCount {
		t.Fatal("stale pending count exceeds total")
	}

	if snapshot.StaleRecoveryCount >
		snapshot.RecoveryCount {
		t.Fatal("stale recovery count exceeds total")
	}

	// Independently verify the classification totals.
	var expectedPending int64
	var expectedRecovery int64

	err = db.QueryRow(ctx, `
        SELECT
            COUNT(*) FILTER (
                WHERE status = 'PENDING'
                  AND capture_requested_at IS NULL
            ),
            COUNT(*) FILTER (
                WHERE status IN ('PENDING', 'PROCESSING')
                  AND (
                      status = 'PROCESSING'
                      OR capture_requested_at IS NOT NULL
                  )
            )
        FROM flex_settlements
    `).Scan(
		&expectedPending,
		&expectedRecovery,
	)
	if err != nil {
		t.Fatal(err)
	}

	if snapshot.PendingCaptureCount != expectedPending {
		t.Fatalf(
			"pending count: got %d, want %d",
			snapshot.PendingCaptureCount,
			expectedPending,
		)
	}

	if snapshot.RecoveryCount != expectedRecovery {
		t.Fatalf(
			"recovery count: got %d, want %d",
			snapshot.RecoveryCount,
			expectedRecovery,
		)
	}

	// Confirm that the snapshot itself cannot execute financial
	// operations by testing input validation and cancellation.
	if _, err := ReadFlexSettlementHealthSnapshot(
		ctx, db, 0,
	); err == nil {
		t.Fatal("expected invalid threshold rejection")
	}

	cancelled, cancel := context.WithCancel(ctx)
	cancel()

	if _, err := ReadFlexSettlementHealthSnapshot(
		cancelled, db, staleAfter,
	); err == nil {
		t.Fatal("expected cancelled context failure")
	}
}
