package postgres

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/JCKFinland/connect/backend/internal/config"
	"github.com/JCKFinland/connect/backend/internal/database"
	"github.com/JCKFinland/connect/backend/internal/testutil"
	"github.com/google/uuid"
)

func TestDriverTerminalReleaseLocksArchivedProfiles(t *testing.T) {
	ctx := context.Background()

	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}
	if err := os.Chdir("../../.."); err != nil {
		t.Fatalf("change to backend root: %v", err)
	}
	defer func() { _ = os.Chdir(originalDir) }()

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	db, err := database.Connect(cfg)
	if err != nil {
		t.Fatalf("connect database: %v", err)
	}
	defer db.Close()

	fixture, cleanup, err := testutil.CreateDriverFixture(ctx, db)
	if err != nil {
		t.Fatalf("create fixture: %v", err)
	}
	defer func() {
		if err := cleanup(context.Background()); err != nil {
			t.Errorf("cleanup fixture: %v", err)
		}
	}()

	// Archive the permanent driver profile.
	_, err = db.Exec(
		ctx,
		`UPDATE drivers
		 SET deleted_at = NOW(), is_active = FALSE
		 WHERE id = $1`,
		fixture.DriverID,
	)
	if err != nil {
		t.Fatalf("archive driver: %v", err)
	}

	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatalf("begin lock transaction: %v", err)
	}
	defer tx.Rollback(context.Background())

	repo := NewDriverRepositoryWithDB(tx)

	if err := repo.LockAllByUserIDForTerminalRelease(
		ctx,
		fixture.UserID,
	); err != nil {
		t.Fatalf("lock archived driver: %v", err)
	}

	// NOWAIT must fail because the first transaction owns the row lock.
	probeTx, err := db.Begin(ctx)
	if err != nil {
		t.Fatalf("begin probe transaction: %v", err)
	}
	defer probeTx.Rollback(context.Background())

	_, err = probeTx.Exec(
		ctx,
		`SELECT id FROM drivers WHERE id = $1 FOR UPDATE NOWAIT`,
		fixture.DriverID,
	)
	if err == nil {
		t.Fatal("expected archived driver row to remain locked")
	}

	if err := probeTx.Rollback(ctx); err != nil {
		t.Fatalf("rollback probe transaction: %v", err)
	}

	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit driver lock transaction: %v", err)
	}

	// After commit, the archived row must be lockable again.
	verifyTx, err := db.Begin(ctx)
	if err != nil {
		t.Fatalf("begin verification transaction: %v", err)
	}
	defer verifyTx.Rollback(context.Background())

	var lockedID string
	err = verifyTx.QueryRow(
		ctx,
		`SELECT id FROM drivers WHERE id = $1 FOR UPDATE NOWAIT`,
		fixture.DriverID,
	).Scan(&lockedID)
	if err != nil {
		t.Fatalf("lock archived driver after commit: %v", err)
	}
	if lockedID != fixture.DriverID {
		t.Fatalf("unexpected driver ID: %s", lockedID)
	}

	// A nonexistent user has no permanent driver rows to lock.
	missingRepo := NewDriverRepositoryWithDB(verifyTx)
	if err := missingRepo.LockAllByUserIDForTerminalRelease(
		ctx,
		uuid.NewString(),
	); err != nil {
		t.Fatalf("lock nonexistent driver's profiles: %v", err)
	}

	if err := verifyTx.Rollback(ctx); err != nil {
		t.Fatalf("rollback verification transaction: %v", err)
	}

	_ = time.Second
}
