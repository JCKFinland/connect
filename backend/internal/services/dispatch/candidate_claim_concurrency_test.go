package dispatch

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/JCKFinland/connect/backend/internal/config"
	"github.com/JCKFinland/connect/backend/internal/database"
	"github.com/JCKFinland/connect/backend/internal/models"
	postgresrepo "github.com/JCKFinland/connect/backend/internal/repository/postgres"
	"github.com/JCKFinland/connect/backend/internal/testutil"
)

// These tests exercise the production candidate-claim savepoint, rather than
// directly testing only the lifecycle validator's advisory locks.
func TestClaimDispatchCandidateSavepointConcurrency(t *testing.T) {
	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir("../../.."); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(originalDir) }()
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
	fixture, cleanup, err := testutil.CreateDriverFixture(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := cleanup(context.Background()); err != nil {
			t.Errorf("cleanup fixture: %v", err)
		}
	}()
	compliantCleanup, err := testutil.MakeDriverRegulatorilyCompliant(ctx, db, fixture)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := compliantCleanup(context.Background()); err != nil {
			t.Errorf("cleanup compliance: %v", err)
		}
	}()
	_, err = db.Exec(ctx, `INSERT INTO driver_presence
 (driver_id,company_id,branch_id,vehicle_id,assignment_id,is_online,availability_status,latitude,longitude,last_heartbeat_at)
 VALUES ($1,$2,$3,$4,$5,TRUE,'AVAILABLE',60.2055,24.6559,NOW())`, fixture.UserID, fixture.CompanyID, fixture.BranchID, fixture.VehicleID, fixture.AssignmentID)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if _, err := db.Exec(context.Background(), `DELETE FROM driver_presence WHERE driver_id=$1`, fixture.UserID); err != nil {
			t.Errorf("cleanup presence: %v", err)
		}
	}()

	tryExclusive := func(t *testing.T, key string) bool {
		t.Helper()
		tx, err := db.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(context.Background())
		var acquired bool
		if err := tx.QueryRow(ctx, `SELECT pg_try_advisory_xact_lock(hashtextextended($1,0))`, key).Scan(&acquired); err != nil {
			t.Fatal(err)
		}
		return acquired
	}
	tryDriver := func(t *testing.T) bool {
		t.Helper()
		tx, err := db.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(context.Background())
		locked, err := postgresrepo.NewDriverRepositoryWithDB(tx).TryLockByID(ctx, fixture.DriverID)
		if err != nil {
			t.Fatal(err)
		}
		return locked
	}

	t.Run("rejected vehicle releases hierarchy and driver locks", func(t *testing.T) {
		tx, err := db.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(context.Background())
		selected, assignment, err := claimDispatchCandidate(ctx, tx, fixture.UserID, func(*models.Vehicle) bool { return false }, time.Now().UTC(), time.Hour, nil)
		if err != nil {
			t.Fatal(err)
		}
		if selected != nil || assignment != nil {
			t.Fatal("rejected vehicle must not be selected")
		}
		if !tryExclusive(t, "company:"+fixture.CompanyID) {
			t.Fatal("rejected candidate retained company lifecycle lock")
		}
		if !tryExclusive(t, "vehicle:"+fixture.VehicleID) {
			t.Fatal("rejected candidate retained vehicle lifecycle lock")
		}
		if !tryDriver(t) {
			t.Fatal("rejected candidate retained driver profile lock")
		}
	})
	t.Run("previous offer rejection releases locks", func(t *testing.T) {
		tx, err := db.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(context.Background())
		offered := map[string]struct{}{fixture.DriverID: {}}
		selected, assignment, err := claimDispatchCandidate(ctx, tx, fixture.UserID, func(*models.Vehicle) bool { return true }, time.Now().UTC(), time.Hour, offered)
		if err != nil {
			t.Fatal(err)
		}
		if selected != nil || assignment != nil {
			t.Fatal("previously offered driver must not be selected")
		}
		if !tryExclusive(t, "company:"+fixture.CompanyID) {
			t.Fatal("previous offer rejection retained lifecycle lock")
		}
		if !tryDriver(t) {
			t.Fatal("previous offer rejection retained driver lock")
		}
	})
	t.Run("selected candidate retains locks until parent rollback", func(t *testing.T) {
		tx, err := db.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(context.Background())
		selected, assignment, err := claimDispatchCandidate(ctx, tx, fixture.UserID, func(*models.Vehicle) bool { return true }, time.Now().UTC(), time.Hour, nil)
		if err != nil {
			t.Fatal(err)
		}
		if selected == nil || assignment == nil {
			t.Fatal("eligible candidate must be selected")
		}
		if assignment.ID != fixture.AssignmentID {
			t.Fatal("selected assignment differs from active fixture")
		}
		if tryExclusive(t, "company:"+fixture.CompanyID) {
			t.Fatal("selected candidate did not retain company lifecycle lock")
		}
		if tryExclusive(t, "vehicle:"+fixture.VehicleID) {
			t.Fatal("selected candidate did not retain vehicle lifecycle lock")
		}
		if tryDriver(t) {
			t.Fatal("selected candidate did not retain driver profile lock")
		}
		if err := tx.Rollback(ctx); err != nil {
			t.Fatal(err)
		}
		if !tryExclusive(t, "company:"+fixture.CompanyID) {
			t.Fatal("company lock not released after parent rollback")
		}
		if !tryDriver(t) {
			t.Fatal("driver lock not released after parent rollback")
		}
	})
}
