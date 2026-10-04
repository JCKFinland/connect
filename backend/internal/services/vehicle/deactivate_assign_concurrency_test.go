package vehicle

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/JCKFinland/connect/backend/internal/config"
	"github.com/JCKFinland/connect/backend/internal/database"
	"github.com/JCKFinland/connect/backend/internal/models"
	postgresrepo "github.com/JCKFinland/connect/backend/internal/repository/postgres"
	assignmentservice "github.com/JCKFinland/connect/backend/internal/services/assignment"
	"github.com/JCKFinland/connect/backend/internal/testutil"
)

func TestAssignWaitsForVehicleDeactivationAndThenRejectsInactiveVehicle(
	t *testing.T,
) {
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
	t.Cleanup(func() {
		db.Close()
	})

	fixture, cleanup, err :=
		testutil.CreateDriverFixture(
			ctx,
			db,
		)
	if err != nil {
		t.Fatalf("create driver fixture: %v", err)
	}

	t.Cleanup(func() {
		if err := cleanup(context.Background()); err != nil {
			t.Errorf("cleanup driver fixture: %v", err)
		}
	})

	// Return the fixture driver and vehicle to an unassigned state while
	// preserving ownership of the original assignment row for fixture cleanup.
	assignments :=
		postgresrepo.NewDriverAssignmentRepository(db)

	if err := assignments.CloseAssignment(
		ctx,
		fixture.AssignmentID,
	); err != nil {
		t.Fatalf("close fixture assignment: %v", err)
	}

	// Assignment requires a lockable presence row.
	_, err = db.Exec(
		ctx,
		`
			INSERT INTO driver_presence (
				driver_id,
				company_id,
				branch_id,
				is_online,
				availability_status
			)
			VALUES (
				$1,
				$2,
				$3,
				FALSE,
				'OFFLINE'
			)
		`,
		fixture.UserID,
		fixture.CompanyID,
		fixture.BranchID,
	)
	if err != nil {
		t.Fatalf("create fixture driver presence: %v", err)
	}

	deactivateTx, err := db.Begin(ctx)
	if err != nil {
		t.Fatalf("begin deactivation transaction: %v", err)
	}

	deactivateCommitted := false
	defer func() {
		if !deactivateCommitted {
			_ = deactivateTx.Rollback(context.Background())
		}
	}()

	if err := postgresrepo.AcquireTransactionAdvisoryLock(
		ctx,
		deactivateTx,
		"vehicle:"+fixture.VehicleID,
	); err != nil {
		t.Fatalf(
			"acquire deactivation vehicle lock: %v",
			err,
		)
	}

	assignService := assignmentservice.NewService(
		assignmentservice.Dependencies{
			DB: db,
		},
	)

	assignResult := make(chan error, 1)

	go func() {
		_, err := assignService.Assign(
			context.Background(),
			fixture.UserID,
			assignmentservice.AssignDriverRequest{
				VehicleID: fixture.VehicleID,
				Notes:     "deactivation concurrency integration test",
			},
		)

		assignResult <- err
	}()

	// Observe the blocked advisory-lock waiter directly rather than relying on
	// timing as evidence that Assign reached the serialization boundary.
	deadline := time.Now().Add(5 * time.Second)

	for {
		var waiting int

		err := db.QueryRow(
			ctx,
			`
				SELECT COUNT(*)
				FROM pg_locks
				WHERE locktype = 'advisory'
				  AND granted = FALSE
			`,
		).Scan(&waiting)
		if err != nil {
			t.Fatalf(
				"inspect advisory lock waiters: %v",
				err,
			)
		}

		if waiting > 0 {
			break
		}

		if time.Now().After(deadline) {
			t.Fatal(
				"assignment never waited on vehicle advisory lock",
			)
		}

		time.Sleep(10 * time.Millisecond)
	}

	deactivateVehicles :=
		postgresrepo.NewVehicleRepositoryWithDB(
			deactivateTx,
		)

	if err := deactivateVehicles.Deactivate(
		ctx,
		fixture.VehicleID,
	); err != nil {
		t.Fatalf(
			"deactivate vehicle while holding lifecycle lock: %v",
			err,
		)
	}

	if err := deactivateTx.Commit(ctx); err != nil {
		t.Fatalf("commit vehicle deactivation: %v", err)
	}
	deactivateCommitted = true

	// Assign now acquires the same vehicle lock and must perform its
	// authoritative read against the newly inactive vehicle.
	select {
	case err := <-assignResult:
		if !errors.Is(
			err,
			assignmentservice.ErrVehicleInactive,
		) {
			t.Fatalf(
				"expected ErrVehicleInactive after deactivation won serialization, got %v",
				err,
			)
		}

	case <-time.After(5 * time.Second):
		t.Fatal(
			"assignment did not finish after deactivation released vehicle lock",
		)
	}

	var (
		isActive          bool
		activeAssignments int
	)

	err = db.QueryRow(
		ctx,
		`
			SELECT is_active
			FROM vehicles
			WHERE id = $1
		`,
		fixture.VehicleID,
	).Scan(&isActive)
	if err != nil {
		t.Fatalf(
			"inspect vehicle after deactivation race: %v",
			err,
		)
	}

	if isActive {
		t.Fatal(
			"vehicle must remain inactive after winning serialization",
		)
	}

	err = db.QueryRow(
		ctx,
		`
			SELECT COUNT(*)
			FROM driver_assignments
			WHERE vehicle_id = $1
			  AND unassigned_at IS NULL
		`,
		fixture.VehicleID,
	).Scan(&activeAssignments)
	if err != nil {
		t.Fatalf(
			"count assignments after deactivation race: %v",
			err,
		)
	}

	if activeAssignments != 0 {
		t.Fatalf(
			"inactive vehicle has %d active assignments; expected 0",
			activeAssignments,
		)
	}
}

func TestDeactivateWaitsForAssignmentAndThenRejectsActiveAssignment(
	t *testing.T,
) {
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
	t.Cleanup(func() {
		db.Close()
	})

	fixture, cleanup, err :=
		testutil.CreateDriverFixture(
			ctx,
			db,
		)
	if err != nil {
		t.Fatalf("create driver fixture: %v", err)
	}

	t.Cleanup(func() {
		if err := cleanup(context.Background()); err != nil {
			t.Errorf("cleanup driver fixture: %v", err)
		}
	})

	// Close the fixture assignment so the vehicle begins unassigned.
	assignments :=
		postgresrepo.NewDriverAssignmentRepository(db)

	if err := assignments.CloseAssignment(
		ctx,
		fixture.AssignmentID,
	); err != nil {
		t.Fatalf("close fixture assignment: %v", err)
	}

	// Give the fixture user tenant authority for vehicle deactivation.
	memberships :=
		postgresrepo.NewCompanyMembershipRepository(db)

	if err := memberships.Create(
		ctx,
		&models.CompanyMembership{
			UserID:    fixture.UserID,
			CompanyID: fixture.CompanyID,
		},
	); err != nil {
		t.Fatalf("create fixture company membership: %v", err)
	}

	t.Cleanup(func() {
		_, err := db.Exec(
			context.Background(),
			`
				DELETE FROM company_memberships
				WHERE user_id = $1
				  AND company_id = $2
			`,
			fixture.UserID,
			fixture.CompanyID,
		)
		if err != nil {
			t.Errorf(
				"cleanup fixture company membership: %v",
				err,
			)
		}
	})

	assignTx, err := db.Begin(ctx)
	if err != nil {
		t.Fatalf("begin assignment transaction: %v", err)
	}

	assignCommitted := false
	defer func() {
		if !assignCommitted {
			_ = assignTx.Rollback(context.Background())
		}
	}()

	if err := postgresrepo.AcquireTransactionAdvisoryLock(
		ctx,
		assignTx,
		"vehicle:"+fixture.VehicleID,
	); err != nil {
		t.Fatalf(
			"acquire assignment vehicle lock: %v",
			err,
		)
	}

	txAssignments :=
		postgresrepo.NewDriverAssignmentRepositoryWithDB(
			assignTx,
		)

	newAssignment := &models.DriverAssignment{
		DriverID:   fixture.UserID,
		VehicleID:  fixture.VehicleID,
		CompanyID:  fixture.CompanyID,
		BranchID:   fixture.BranchID,
		FleetID:    fixture.FleetID,
		AssignedAt: time.Now().UTC(),
		Notes:      "assignment wins deactivation race",
	}

	if err := txAssignments.Create(
		ctx,
		newAssignment,
	); err != nil {
		t.Fatalf(
			"create assignment while holding lifecycle lock: %v",
			err,
		)
	}

	service := NewService(
		Dependencies{
			DB: db,
			UserRoles: &vehicleUserRoleRepositoryStub{
				roles: []string{"COMPANY_ADMIN"},
			},
			CompanyMemberships: memberships,
		},
	)

	deactivateResult := make(chan error, 1)

	go func() {
		deactivateResult <- service.Deactivate(
			context.Background(),
			fixture.UserID,
			fixture.VehicleID,
		)
	}()

	deadline := time.Now().Add(5 * time.Second)

	for {
		var waiting int

		err := db.QueryRow(
			ctx,
			`
				SELECT COUNT(*)
				FROM pg_locks
				WHERE locktype = 'advisory'
				  AND granted = FALSE
			`,
		).Scan(&waiting)
		if err != nil {
			t.Fatalf(
				"inspect advisory lock waiters: %v",
				err,
			)
		}

		if waiting > 0 {
			break
		}

		if time.Now().After(deadline) {
			t.Fatal(
				"deactivation never waited on vehicle advisory lock",
			)
		}

		time.Sleep(10 * time.Millisecond)
	}

	if err := assignTx.Commit(ctx); err != nil {
		t.Fatalf("commit vehicle assignment: %v", err)
	}
	assignCommitted = true

	select {
	case err := <-deactivateResult:
		if !errors.Is(
			err,
			ErrVehicleHasActiveAssignment,
		) {
			t.Fatalf(
				"expected ErrVehicleHasActiveAssignment after assignment won serialization, got %v",
				err,
			)
		}

	case <-time.After(5 * time.Second):
		t.Fatal(
			"deactivation did not finish after assignment released vehicle lock",
		)
	}

	var (
		isActive          bool
		activeAssignments int
	)

	err = db.QueryRow(
		ctx,
		`
			SELECT is_active
			FROM vehicles
			WHERE id = $1
		`,
		fixture.VehicleID,
	).Scan(&isActive)
	if err != nil {
		t.Fatalf(
			"inspect vehicle after assignment race: %v",
			err,
		)
	}

	if !isActive {
		t.Fatal(
			"vehicle became inactive despite winning active assignment",
		)
	}

	err = db.QueryRow(
		ctx,
		`
			SELECT COUNT(*)
			FROM driver_assignments
			WHERE vehicle_id = $1
			  AND unassigned_at IS NULL
		`,
		fixture.VehicleID,
	).Scan(&activeAssignments)
	if err != nil {
		t.Fatalf(
			"count assignments after assignment race: %v",
			err,
		)
	}

	if activeAssignments != 1 {
		t.Fatalf(
			"expected 1 active assignment after assignment won serialization, got %d",
			activeAssignments,
		)
	}

	// Fixture cleanup owns only its original assignment, so remove the
	// additional assignment created by this test.
	_, err = db.Exec(
		ctx,
		`
			DELETE FROM driver_assignments
			WHERE id = $1
		`,
		newAssignment.ID,
	)
	if err != nil {
		t.Fatalf(
			"delete concurrency assignment: %v",
			err,
		)
	}
}
