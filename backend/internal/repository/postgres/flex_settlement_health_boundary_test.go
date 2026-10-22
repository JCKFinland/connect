package postgres

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/JCKFinland/connect/backend/internal/config"
	"github.com/JCKFinland/connect/backend/internal/database"
)

func TestFlexSettlementHealthExactStalenessBoundary(t *testing.T) {
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

	if cfg.Database.Name != "connect_settlement_test" {
		t.Fatalf("unsafe configured database: %q", cfg.Database.Name)
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
		t.Fatalf("unsafe connected database: %q", databaseName)
	}

	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := tx.Rollback(context.Background()); err != nil {
			t.Errorf("rollback boundary test transaction: %v", err)
		}
	}()

	// Transaction-local shadow table. The production financial table
	// remains untouched and its constraints are not disabled.
	_, err = tx.Exec(ctx, `
        CREATE TEMP TABLE flex_settlements (
            status TEXT NOT NULL,
            capture_requested_at TIMESTAMPTZ,
            created_at TIMESTAMPTZ NOT NULL
        ) ON COMMIT DROP
    `)
	if err != nil {
		t.Fatalf("create temporary settlement table: %v", err)
	}

	// PostgreSQL resolves the unqualified relation to pg_temp first.
	// Verify this before executing the production aggregation.
	var relation string
	if err := tx.QueryRow(ctx, `
        SELECT to_regclass('flex_settlements')::text
    `).Scan(&relation); err != nil {
		t.Fatal(err)
	}

	if relation != "flex_settlements" {
		t.Fatalf("unexpected settlement relation: %q", relation)
	}

	observedAt := time.Date(
		2026, time.October, 22,
		12, 0, 0, 0,
		time.UTC,
	)
	const staleAfter = 15 * time.Minute

	cutoff := observedAt.Add(-staleAfter)

	rows := []struct {
		status             string
		captureRequestedAt *time.Time
		createdAt          time.Time
	}{
		{"PENDING", nil, cutoff.Add(-time.Microsecond)},
		{"PENDING", nil, cutoff},
		{"PENDING", nil, cutoff.Add(time.Microsecond)},
		{"PROCESSING", nil, cutoff.Add(-time.Microsecond)},
		{"PROCESSING", nil, cutoff},
		{"PROCESSING", nil, cutoff.Add(time.Microsecond)},
		{"PENDING", &observedAt, cutoff.Add(-time.Microsecond)},
		{"PENDING", &observedAt, cutoff},
		{"PENDING", &observedAt, cutoff.Add(time.Microsecond)},
		{"CAPTURED", nil, cutoff.Add(-time.Hour)},
		{"CANCELLED", nil, cutoff.Add(-time.Hour)},
	}

	for _, row := range rows {
		_, err := tx.Exec(ctx, `
            INSERT INTO pg_temp.flex_settlements (
                status,
                capture_requested_at,
                created_at
            )
            VALUES ($1, $2, $3)
        `,
			row.status,
			row.captureRequestedAt,
			row.createdAt,
		)
		if err != nil {
			t.Fatalf("insert temporary settlement: %v", err)
		}
	}

	snapshot, err := readFlexSettlementHealthSnapshotAt(
		ctx,
		tx,
		staleAfter,
		&observedAt,
	)
	if err != nil {
		t.Fatalf("read fixed-time settlement snapshot: %v", err)
	}

	if !snapshot.ObservedAt.Equal(observedAt) {
		t.Fatalf(
			"observation time: got %s, want %s",
			snapshot.ObservedAt,
			observedAt,
		)
	}

	if snapshot.PendingCaptureCount != 3 {
		t.Fatalf(
			"pending count: got %d, want 3",
			snapshot.PendingCaptureCount,
		)
	}

	if snapshot.RecoveryCount != 6 {
		t.Fatalf(
			"recovery count: got %d, want 6",
			snapshot.RecoveryCount,
		)
	}

	if snapshot.StalePendingCaptureCount != 2 {
		t.Fatalf(
			"stale pending count: got %d, want 2",
			snapshot.StalePendingCaptureCount,
		)
	}

	if snapshot.StaleRecoveryCount != 4 {
		t.Fatalf(
			"stale recovery count: got %d, want 4",
			snapshot.StaleRecoveryCount,
		)
	}

	oldest := cutoff.Add(-time.Microsecond)

	if snapshot.OldestPendingCaptureAt == nil ||
		!snapshot.OldestPendingCaptureAt.Equal(oldest) {
		t.Fatalf(
			"oldest pending: got %v, want %s",
			snapshot.OldestPendingCaptureAt,
			oldest,
		)
	}

	if snapshot.OldestRecoveryAt == nil ||
		!snapshot.OldestRecoveryAt.Equal(oldest) {
		t.Fatalf(
			"oldest recovery: got %v, want %s",
			snapshot.OldestRecoveryAt,
			oldest,
		)
	}
}
