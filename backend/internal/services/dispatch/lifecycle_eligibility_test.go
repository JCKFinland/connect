package dispatch

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/JCKFinland/connect/backend/internal/config"
	"github.com/JCKFinland/connect/backend/internal/database"
	postgresrepo "github.com/JCKFinland/connect/backend/internal/repository/postgres"
	"github.com/JCKFinland/connect/backend/internal/testutil"
	"github.com/google/uuid"
)

func TestDispatchLifecycleEligibility(t *testing.T) {
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

	ctx := context.Background()
	fixture, cleanup, err := testutil.CreateDriverFixture(ctx, db)
	if err != nil {
		t.Fatalf("create isolated driver fixture: %v", err)
	}
	defer func() {
		if err := cleanup(context.Background()); err != nil {
			t.Errorf("cleanup driver fixture: %v", err)
		}
	}()
	complianceCleanup, err := testutil.MakeDriverRegulatorilyCompliant(ctx, db, fixture)
	if err != nil {
		t.Fatalf("make driver compliant: %v", err)
	}
	defer func() {
		if err := complianceCleanup(context.Background()); err != nil {
			t.Errorf("cleanup compliance fixture: %v", err)
		}
	}()

	expected := &dispatchLifecycleExpectation{
		AssignmentID: fixture.AssignmentID,
		CompanyID:    fixture.CompanyID,
		BranchID:     fixture.BranchID,
		FleetID:      fixture.FleetID,
		VehicleID:    fixture.VehicleID,
	}

	t.Run("eligible matching assignment", func(t *testing.T) {
		tx, err := db.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(context.Background())
		eligible, err := lockAndValidateDispatchLifecycle(ctx, tx, fixture.UserID, expected, time.Now().UTC())
		if err != nil {
			t.Fatalf("validate eligible lifecycle: %v", err)
		}
		if !eligible {
			t.Fatal("expected compliant driver with active hierarchy to be eligible")
		}
	})

	t.Run("reject stale offer assignment", func(t *testing.T) {
		tx, err := db.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(context.Background())
		mismatch := *expected
		mismatch.AssignmentID = uuid.NewString()
		eligible, err := lockAndValidateDispatchLifecycle(ctx, tx, fixture.UserID, &mismatch, time.Now().UTC())
		if err != nil {
			t.Fatalf("validate stale assignment: %v", err)
		}
		if eligible {
			t.Fatal("stale assignment ID must be rejected")
		}
	})

	t.Run("reject stale offer vehicle", func(t *testing.T) {
		tx, err := db.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(context.Background())
		mismatch := *expected
		mismatch.VehicleID = uuid.NewString()
		eligible, err := lockAndValidateDispatchLifecycle(ctx, tx, fixture.UserID, &mismatch, time.Now().UTC())
		if err != nil {
			t.Fatalf("validate stale vehicle: %v", err)
		}
		if eligible {
			t.Fatal("stale vehicle ID must be rejected")
		}
	})

	t.Run("reject inactive hierarchy", func(t *testing.T) {
		tx, err := db.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(context.Background())
		if _, err := tx.Exec(ctx, `UPDATE companies SET is_active = FALSE WHERE id = $1`, fixture.CompanyID); err != nil {
			t.Fatalf("deactivate test company: %v", err)
		}
		eligible, err := lockAndValidateDispatchLifecycle(ctx, tx, fixture.UserID, expected, time.Now().UTC())
		if err != nil {
			t.Fatalf("validate inactive hierarchy: %v", err)
		}
		if eligible {
			t.Fatal("inactive company must reject dispatch")
		}
	})

	t.Run("shared lifecycle locks exclude concurrent mutation", func(t *testing.T) {
		tx1, err := db.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer tx1.Rollback(context.Background())
		eligible, err := lockAndValidateDispatchLifecycle(ctx, tx1, fixture.UserID, expected, time.Now().UTC())
		if err != nil {
			t.Fatalf("lock eligible lifecycle: %v", err)
		}
		if !eligible {
			t.Fatal("expected eligible driver before contention check")
		}

		tx2, err := db.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer tx2.Rollback(context.Background())
		var acquired bool
		err = tx2.QueryRow(ctx, `SELECT pg_try_advisory_xact_lock(hashtextextended($1, 0))`, "company:"+fixture.CompanyID).Scan(&acquired)
		if err != nil {
			t.Fatalf("try exclusive company lifecycle lock: %v", err)
		}
		if acquired {
			t.Fatal("concurrent lifecycle mutation acquired exclusive company lock")
		}
		if err := tx1.Rollback(ctx); err != nil {
			t.Fatalf("release lifecycle locks: %v", err)
		}
		err = tx2.QueryRow(ctx, `SELECT pg_try_advisory_xact_lock(hashtextextended($1, 0))`, "company:"+fixture.CompanyID).Scan(&acquired)
		if err != nil {
			t.Fatalf("retry exclusive company lifecycle lock: %v", err)
		}
		if !acquired {
			t.Fatal("expected exclusive lifecycle lock after release")
		}
	})

	t.Run("candidate savepoint rollback releases locks", func(t *testing.T) {
		tx1, err := db.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer tx1.Rollback(context.Background())
		nested, err := tx1.Begin(ctx)
		if err != nil {
			t.Fatalf("begin candidate savepoint: %v", err)
		}
		eligible, err := lockAndValidateDispatchLifecycle(ctx, nested, fixture.UserID, expected, time.Now().UTC())
		if err != nil {
			_ = nested.Rollback(ctx)
			t.Fatalf("validate candidate: %v", err)
		}
		if !eligible {
			_ = nested.Rollback(ctx)
			t.Fatal("expected eligible candidate")
		}
		if err := nested.Rollback(ctx); err != nil {
			t.Fatalf("rollback candidate savepoint: %v", err)
		}

		tx2, err := db.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer tx2.Rollback(context.Background())
		var acquired bool
		if err := tx2.QueryRow(ctx, `SELECT pg_try_advisory_xact_lock(hashtextextended($1, 0))`, "company:"+fixture.CompanyID).Scan(&acquired); err != nil {
			t.Fatal(err)
		}
		if !acquired {
			t.Fatal("candidate rollback must release advisory locks")
		}
		drivers := postgresrepo.NewDriverRepositoryWithDB(tx2)
		locked, err := drivers.TryLockByID(ctx, fixture.DriverID)
		if err != nil {
			t.Fatal(err)
		}
		if !locked {
			t.Fatal("candidate rollback must release driver row lock")
		}
	})
}
