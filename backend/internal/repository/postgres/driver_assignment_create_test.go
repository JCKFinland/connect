package postgres

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
