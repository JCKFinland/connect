package postgres

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/JCKFinland/connect/backend/internal/config"
	"github.com/JCKFinland/connect/backend/internal/database"
	"github.com/JCKFinland/connect/backend/internal/models"
	"github.com/JCKFinland/connect/backend/internal/testutil"
)

func TestDriverAssignmentRepositoryCreateCanonicalizesActiveLifecycle(
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

	fixture, cleanupFixture, err :=
		testutil.CreateDriverFixture(ctx, db)
	if err != nil {
		t.Fatalf("create driver fixture: %v", err)
	}
	t.Cleanup(func() {
		if cleanupErr := cleanupFixture(
			context.Background(),
		); cleanupErr != nil {
			t.Logf("cleanup driver fixture: %v", cleanupErr)
		}
	})

	// The shared fixture starts with an active assignment. Close it only
	// as test setup so the repository can create another assignment for
	// the same driver and vehicle.
	_, err = db.Exec(
		ctx,
		`
			UPDATE driver_assignments
			SET
				unassigned_at = NOW(),
				updated_at = NOW()
			WHERE id = $1
		`,
		fixture.AssignmentID,
	)
	if err != nil {
		t.Fatalf("close fixture assignment: %v", err)
	}

	assignedAt := time.Now().UTC().Truncate(time.Microsecond)
	hostileUnassignedAt := assignedAt.Add(time.Hour)

	assignment := &models.DriverAssignment{
		CompanyID:    fixture.CompanyID,
		BranchID:     fixture.BranchID,
		FleetID:      fixture.FleetID,
		DriverID:     fixture.UserID,
		VehicleID:    fixture.VehicleID,
		AssignedAt:   assignedAt,
		UnassignedAt: &hostileUnassignedAt,
		Notes:        "creation lifecycle authority test",
	}

	repo := NewDriverAssignmentRepository(db)

	if err := repo.Create(ctx, assignment); err != nil {
		t.Fatalf("create driver assignment: %v", err)
	}

	t.Cleanup(func() {
		if assignment.ID == "" {
			return
		}

		if _, cleanupErr := db.Exec(
			context.Background(),
			`DELETE FROM driver_assignments WHERE id = $1`,
			assignment.ID,
		); cleanupErr != nil {
			t.Logf("cleanup created assignment: %v", cleanupErr)
		}
	})

	if assignment.UnassignedAt != nil {
		t.Fatalf(
			"expected returned assignment to remain active, got unassigned_at=%v",
			assignment.UnassignedAt,
		)
	}

	var persistedUnassignedAt *time.Time

	err = db.QueryRow(
		ctx,
		`
			SELECT unassigned_at
			FROM driver_assignments
			WHERE id = $1
		`,
		assignment.ID,
	).Scan(&persistedUnassignedAt)
	if err != nil {
		t.Fatalf("read persisted assignment lifecycle: %v", err)
	}

	if persistedUnassignedAt != nil {
		t.Fatalf(
			"expected persisted assignment to remain active, got unassigned_at=%v",
			persistedUnassignedAt,
		)
	}
}

func TestDriverAssignmentChronologyConstraintRejectsImpossibleHistory(
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

	fixture, cleanupFixture, err :=
		testutil.CreateDriverFixture(ctx, db)
	if err != nil {
		t.Fatalf("create driver fixture: %v", err)
	}
	t.Cleanup(func() {
		if cleanupErr := cleanupFixture(
			context.Background(),
		); cleanupErr != nil {
			t.Logf("cleanup driver fixture: %v", cleanupErr)
		}
	})

	// Close the fixture assignment so the partial uniqueness constraints do
	// not mask the chronology constraint under test.
	_, err = db.Exec(
		ctx,
		`
			UPDATE driver_assignments
			SET
				unassigned_at = NOW(),
				updated_at = NOW()
			WHERE id = $1
		`,
		fixture.AssignmentID,
	)
	if err != nil {
		t.Fatalf("close fixture assignment: %v", err)
	}

	assignedAt := time.Now().UTC().Add(-time.Hour)
	unassignedAt := assignedAt.Add(-time.Minute)

	_, err = db.Exec(
		ctx,
		`
			INSERT INTO driver_assignments
			(
				company_id,
				branch_id,
				fleet_id,
				driver_id,
				vehicle_id,
				assigned_at,
				unassigned_at,
				notes
			)
			VALUES
			(
				$1,$2,$3,$4,$5,$6,$7,$8
			)
		`,
		fixture.CompanyID,
		fixture.BranchID,
		fixture.FleetID,
		fixture.UserID,
		fixture.VehicleID,
		assignedAt,
		unassignedAt,
		"impossible assignment chronology test",
	)
	if err == nil {
		t.Fatal("expected impossible assignment chronology to be rejected")
	}

	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		t.Fatalf(
			"expected PostgreSQL constraint error, got: %v",
			err,
		)
	}

	if pgErr.Code != "23514" {
		t.Fatalf(
			"expected check violation 23514, got %s: %v",
			pgErr.Code,
			err,
		)
	}

	if pgErr.ConstraintName != "chk_driver_assignment_chronology" {
		t.Fatalf(
			"expected chronology constraint violation, got %q: %v",
			pgErr.ConstraintName,
			err,
		)
	}
}
