package dispatch

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/JCKFinland/connect/backend/internal/config"
	"github.com/JCKFinland/connect/backend/internal/database"
	"github.com/JCKFinland/connect/backend/internal/models"
	"github.com/JCKFinland/connect/backend/internal/testutil"
)

// A live presence row must identify the same exact assignment as the active
// assignment record before dispatch may claim the driver.
func TestClaimDispatchCandidateRejectsMissingPresenceAssignment(t *testing.T) {
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
 VALUES ($1,$2,$3,$4,NULL,TRUE,'AVAILABLE',60.2055,24.6559,NOW())`,
		fixture.UserID, fixture.CompanyID, fixture.BranchID, fixture.VehicleID)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if _, err := db.Exec(context.Background(), `DELETE FROM driver_presence WHERE driver_id=$1`, fixture.UserID); err != nil {
			t.Errorf("cleanup presence: %v", err)
		}
	}()
	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	presence, assignment, err := claimDispatchCandidate(ctx, tx, fixture.UserID,
		func(*models.Vehicle) bool { return true }, time.Now().UTC(), time.Hour, nil)
	if err != nil {
		t.Fatal(err)
	}
	if presence != nil || assignment != nil {
		t.Fatalf("candidate with missing presence assignment was selected: presence=%+v assignment=%+v", presence, assignment)
	}
}
