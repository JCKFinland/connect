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

func TestDriverTryLockByIDSerializesConcurrentTransactions(t *testing.T) {
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
		t.Fatalf("load CONNECT configuration: %v", err)
	}

	db, err := database.Connect(cfg)
	if err != nil {
		t.Fatalf("connect database: %v", err)
	}
	defer db.Close()

	fixture, cleanup, err := testutil.CreateDriverFixture(ctx, db)
	if err != nil {
		t.Fatalf("create driver fixture: %v", err)
	}
	defer func() {
		if err := cleanup(context.Background()); err != nil {
			t.Logf("cleanup driver fixture: %v", err)
		}
	}()

	tx1, err := db.Begin(ctx)
	if err != nil {
		t.Fatalf("begin first transaction: %v", err)
	}
	defer tx1.Rollback(context.Background())

	repo1 := NewDriverRepositoryWithDB(tx1)

	locked, err := repo1.TryLockByID(ctx, fixture.DriverID)
	if err != nil {
		t.Fatalf("acquire driver lock: %v", err)
	}
	if !locked {
		t.Fatal("expected first transaction to acquire driver lock")
	}

	tx2, err := db.Begin(ctx)
	if err != nil {
		t.Fatalf("begin second transaction: %v", err)
	}
	defer tx2.Rollback(context.Background())

	repo2 := NewDriverRepositoryWithDB(tx2)

	checkCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	locked, err = repo2.TryLockByID(checkCtx, fixture.DriverID)
	if err != nil {
		t.Fatalf("contended driver lock should not error: %v", err)
	}
	if locked {
		t.Fatal("second transaction acquired already-locked driver")
	}

	if err := tx1.Rollback(ctx); err != nil {
		t.Fatalf("release first driver lock: %v", err)
	}

	locked, err = repo2.TryLockByID(ctx, fixture.DriverID)
	if err != nil {
		t.Fatalf("acquire released driver lock: %v", err)
	}
	if !locked {
		t.Fatal("expected second transaction to acquire released lock")
	}

	locked, err = repo2.TryLockByID(ctx, uuid.NewString())
	if err != nil {
		t.Fatalf("lock nonexistent driver: %v", err)
	}
	if locked {
		t.Fatal("nonexistent driver must not be lockable")
	}
}
