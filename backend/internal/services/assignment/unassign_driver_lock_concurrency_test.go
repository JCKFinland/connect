package assignment

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/JCKFinland/connect/backend/internal/config"
	"github.com/JCKFinland/connect/backend/internal/database"
	postgresrepo "github.com/JCKFinland/connect/backend/internal/repository/postgres"
	"github.com/JCKFinland/connect/backend/internal/testutil"
)

func TestUnassignWaitsForPermanentDriverLock(t *testing.T) {
	ctx := context.Background()

	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}

	if err := os.Chdir("../../.."); err != nil {
		t.Fatalf("change to backend root: %v", err)
	}
	defer func() {
		_ = os.Chdir(originalDir)
	}()

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
			t.Errorf("cleanup driver fixture: %v", err)
		}
	}()

	_, err = db.Exec(
		ctx,
		`
			INSERT INTO driver_presence (
				driver_id,
				company_id,
				branch_id,
				vehicle_id,
				assignment_id,
				is_online,
				availability_status,
				last_heartbeat_at
			)
			VALUES ($1, $2, $3, $4, $5, TRUE, 'AVAILABLE', NOW())
		`,
		fixture.UserID,
		fixture.CompanyID,
		fixture.BranchID,
		fixture.VehicleID,
		fixture.AssignmentID,
	)
	if err != nil {
		t.Fatalf("create driver presence: %v", err)
	}

	defer func() {
		if _, err := db.Exec(
			context.Background(),
			`DELETE FROM driver_presence WHERE driver_id = $1`,
			fixture.UserID,
		); err != nil {
			t.Errorf("cleanup driver presence: %v", err)
		}
	}()

	lockTx, err := db.Begin(ctx)
	if err != nil {
		t.Fatalf("begin driver lock transaction: %v", err)
	}

	lockReleased := false
	defer func() {
		if !lockReleased {
			_ = lockTx.Rollback(context.Background())
		}
	}()

	drivers := postgresrepo.NewDriverRepositoryWithDB(lockTx)

	lockedDriver, err := drivers.GetByUserIDForUpdate(
		ctx,
		fixture.UserID,
	)
	if err != nil {
		t.Fatalf("lock permanent driver: %v", err)
	}

	if lockedDriver.ID != fixture.DriverID {
		t.Fatalf(
			"locked driver %s; expected %s",
			lockedDriver.ID,
			fixture.DriverID,
		)
	}

	service := NewService(
		Dependencies{
			DB: db,
		},
	)

	unassignResult := make(chan error, 1)

	go func() {
		unassignResult <- service.Unassign(
			context.Background(),
			fixture.UserID,
		)
	}()

	// Confirm an actual PostgreSQL lock wait rather than relying
	// solely on a timing-based assertion.
	deadline := time.Now().Add(5 * time.Second)

	for {
		var waiting int

		err := db.QueryRow(
			ctx,
			`
				SELECT COUNT(*)
				FROM pg_stat_activity
				WHERE datname = current_database()
				  AND wait_event_type = 'Lock'
				  AND state = 'active'
				  AND query LIKE '%FROM drivers%'
				  AND query LIKE '%FOR UPDATE%'
			`,
		).Scan(&waiting)
		if err != nil {
			t.Fatalf("inspect driver lock waiters: %v", err)
		}

		if waiting > 0 {
			break
		}

		select {
		case err := <-unassignResult:
			t.Fatalf(
				"unassignment finished before driver lock release: %v",
				err,
			)
		default:
		}

		if time.Now().After(deadline) {
			t.Fatal("unassignment never waited on permanent driver lock")
		}

		time.Sleep(10 * time.Millisecond)
	}

	// The blocked transaction must not have detached presence
	// or closed the assignment.
	var activeAssignments int

	err = db.QueryRow(
		ctx,
		`
			SELECT COUNT(*)
			FROM driver_assignments
			WHERE id = $1
			  AND unassigned_at IS NULL
		`,
		fixture.AssignmentID,
	).Scan(&activeAssignments)
	if err != nil {
		t.Fatalf("inspect active assignment while blocked: %v", err)
	}

	if activeAssignments != 1 {
		t.Fatalf(
			"expected assignment to remain active while blocked; got %d",
			activeAssignments,
		)
	}

	var linkedPresence int

	err = db.QueryRow(
		ctx,
		`
			SELECT COUNT(*)
			FROM driver_presence
			WHERE driver_id = $1
			  AND assignment_id = $2
			  AND vehicle_id = $3
		`,
		fixture.UserID,
		fixture.AssignmentID,
		fixture.VehicleID,
	).Scan(&linkedPresence)
	if err != nil {
		t.Fatalf("inspect presence while blocked: %v", err)
	}

	if linkedPresence != 1 {
		t.Fatal("presence was detached before permanent driver lock release")
	}

	if err := lockTx.Commit(ctx); err != nil {
		t.Fatalf("release permanent driver lock: %v", err)
	}
	lockReleased = true

	select {
	case err := <-unassignResult:
		if err != nil {
			t.Fatalf("unassignment failed after lock release: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("unassignment did not finish after driver lock release")
	}

	err = db.QueryRow(
		ctx,
		`
			SELECT COUNT(*)
			FROM driver_assignments
			WHERE id = $1
			  AND unassigned_at IS NULL
		`,
		fixture.AssignmentID,
	).Scan(&activeAssignments)
	if err != nil {
		t.Fatalf("inspect final assignment state: %v", err)
	}

	if activeAssignments != 0 {
		t.Fatal("assignment remained active after successful unassignment")
	}

	err = db.QueryRow(
		ctx,
		`
			SELECT COUNT(*)
			FROM driver_presence
			WHERE driver_id = $1
			  AND assignment_id IS NULL
			  AND vehicle_id IS NULL
		`,
		fixture.UserID,
	).Scan(&linkedPresence)
	if err != nil {
		t.Fatalf("inspect final presence state: %v", err)
	}

	if linkedPresence != 1 {
		t.Fatal("presence was not detached after successful unassignment")
	}
}
