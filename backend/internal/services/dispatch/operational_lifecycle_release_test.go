package dispatch

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/JCKFinland/connect/backend/internal/config"
	"github.com/JCKFinland/connect/backend/internal/database"
	postgresrepo "github.com/JCKFinland/connect/backend/internal/repository/postgres"
	"github.com/JCKFinland/connect/backend/internal/services/assignment"
	"github.com/JCKFinland/connect/backend/internal/services/presence"
	"github.com/JCKFinland/connect/backend/internal/testutil"
)

type lifecycleEligibleCompliance struct{}

func (lifecycleEligibleCompliance) IsEligible(context.Context, string, time.Time) (bool, error) {
	return true, nil
}

// TestOperationalLifecycleOfflineUnassignHeartbeat is the first real-service
// cross-package lifecycle slice. Dispatch/offer/trip continuity is a separate test.
func TestOperationalLifecycleOfflineUnassignHeartbeat(t *testing.T) {
	ctx := context.Background()
	original, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir("../../.."); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(original) }()
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	db, err := database.Connect(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	f, cleanup, err := testutil.CreateDriverFixture(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := cleanup(context.Background()); err != nil {
			t.Logf("fixture cleanup: %v", err)
		}
	}()
	complianceCleanup, err := testutil.MakeDriverRegulatorilyCompliant(ctx, db, f)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := complianceCleanup(context.Background()); err != nil {
			t.Logf("compliance cleanup: %v", err)
		}
	}()
	_, err = db.Exec(ctx, `INSERT INTO driver_presence (driver_id,company_id,branch_id,is_online,availability_status) VALUES ($1,$2,$3,FALSE,'OFFLINE')`, f.UserID, f.CompanyID, f.BranchID)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_, _ = db.Exec(context.Background(), `DELETE FROM driver_presence WHERE driver_id=$1`, f.UserID)
	}()
	p := presence.NewService(presence.Dependencies{DB: db, Config: cfg, Drivers: postgresrepo.NewDriverRepository(db), Presence: postgresrepo.NewDriverPresenceRepository(db), Assignments: postgresrepo.NewDriverAssignmentRepository(db), Compliance: lifecycleEligibleCompliance{}})
	if err := p.GoOnline(ctx, presence.GoOnlineRequest{UserID: f.UserID}); err != nil {
		t.Fatalf("go online: %v", err)
	}
	check := func(online bool, status string, assigned bool) {
		t.Helper()
		var gotOnline bool
		var gotStatus string
		var assignmentID *string
		err := db.QueryRow(ctx, `SELECT is_online,availability_status,assignment_id FROM driver_presence WHERE driver_id=$1`, f.UserID).Scan(&gotOnline, &gotStatus, &assignmentID)
		if err != nil {
			t.Fatal(err)
		}
		if gotOnline != online || gotStatus != status || (assignmentID != nil) != assigned {
			t.Fatalf("presence got online=%v status=%s assignment=%v; want online=%v status=%s assigned=%v", gotOnline, gotStatus, assignmentID, online, status, assigned)
		}
	}
	check(true, "AVAILABLE", true)
	if err := p.Heartbeat(ctx, presence.HeartbeatRequest{UserID: f.UserID, Latitude: 60.1699, Longitude: 24.9384}); err != nil {
		t.Fatalf("online heartbeat: %v", err)
	}
	check(true, "AVAILABLE", true)
	if err := p.GoOffline(ctx, presence.GoOfflineRequest{UserID: f.UserID}); err != nil {
		t.Fatalf("go offline: %v", err)
	}
	check(false, "OFFLINE", true)
	a := assignment.NewService(assignment.Dependencies{DB: db})
	if err := a.Unassign(ctx, f.UserID); err != nil {
		t.Fatalf("unassign: %v", err)
	}
	check(false, "OFFLINE", false)
	err = p.Heartbeat(ctx, presence.HeartbeatRequest{UserID: f.UserID, Latitude: 60.1699, Longitude: 24.9384})
	if !errors.Is(err, presence.ErrDriverHeartbeatUnavailable) {
		t.Fatalf("heartbeat after unassign: want ErrDriverHeartbeatUnavailable, got %v", err)
	}
	check(false, "OFFLINE", false)
}
