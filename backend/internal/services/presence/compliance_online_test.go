package presence

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/JCKFinland/connect/backend/internal/config"
	"github.com/JCKFinland/connect/backend/internal/database"
	postgresrepo "github.com/JCKFinland/connect/backend/internal/repository/postgres"
	"github.com/JCKFinland/connect/backend/internal/testutil"
)

func TestGoOnlineRejectsNonCompliantDriverBeforePresenceMutation(
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
	defer db.Close()

	fixture, cleanup, err := testutil.CreateDriverFixture(
		ctx,
		db,
	)
	if err != nil {
		t.Fatalf("create driver fixture: %v", err)
	}

	defer func() {
		if cleanupErr := cleanup(context.Background()); cleanupErr != nil {
			t.Logf("cleanup driver fixture: %v", cleanupErr)
		}
	}()

	// Create an explicit OFFLINE presence row. This gives the test a
	// concrete state that must remain unchanged when compliance rejects
	// the GoOnline transition.
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
				updated_at
			)
			VALUES (
				$1,
				$2,
				$3,
				NULL,
				NULL,
				FALSE,
				'OFFLINE',
				NOW()
			)
		`,
		fixture.UserID,
		fixture.CompanyID,
		fixture.BranchID,
	)
	if err != nil {
		t.Fatalf("create offline driver presence: %v", err)
	}

	defer func() {
		if _, cleanupErr := db.Exec(
			context.Background(),
			`
				DELETE FROM driver_presence
				WHERE driver_id = $1
			`,
			fixture.UserID,
		); cleanupErr != nil {
			t.Logf(
				"cleanup driver presence: %v",
				cleanupErr,
			)
		}
	}()

	compliance := &complianceEvaluatorStub{
		eligible: false,
	}

	service := NewService(
		Dependencies{
			DB:          db,
			Config:      cfg,
			Drivers:     postgresrepo.NewDriverRepository(db),
			Presence:    postgresrepo.NewDriverPresenceRepository(db),
			Assignments: postgresrepo.NewDriverAssignmentRepository(db),
			Compliance:  compliance,
		},
	)

	err = service.GoOnline(
		ctx,
		GoOnlineRequest{
			UserID: fixture.UserID,
		},
	)

	if !errors.Is(
		err,
		ErrDriverComplianceRequired,
	) {
		t.Fatalf(
			"expected ErrDriverComplianceRequired, got %v",
			err,
		)
	}

	if compliance.calls != 1 {
		t.Fatalf(
			"expected compliance evaluator to be called once, got %d",
			compliance.calls,
		)
	}

	if compliance.lastDriverID != fixture.DriverID {
		t.Fatalf(
			"expected compliance evaluation for driver ID %s, got %s",
			fixture.DriverID,
			compliance.lastDriverID,
		)
	}

	var (
		isOnline           bool
		availabilityStatus string
		assignmentID       *string
		vehicleID          *string
	)

	err = db.QueryRow(
		ctx,
		`
			SELECT
				is_online,
				availability_status,
				assignment_id,
				vehicle_id
			FROM driver_presence
			WHERE driver_id = $1
		`,
		fixture.UserID,
	).Scan(
		&isOnline,
		&availabilityStatus,
		&assignmentID,
		&vehicleID,
	)
	if err != nil {
		t.Fatalf("load driver presence after rejection: %v", err)
	}

	if isOnline {
		t.Fatal(
			"non-compliant driver presence changed to online",
		)
	}

	if availabilityStatus != StatusOffline {
		t.Fatalf(
			"expected OFFLINE availability after rejection, got %s",
			availabilityStatus,
		)
	}

	if assignmentID != nil {
		t.Fatalf(
			"expected assignment to remain detached, got %s",
			*assignmentID,
		)
	}

	if vehicleID != nil {
		t.Fatalf(
			"expected vehicle to remain detached, got %s",
			*vehicleID,
		)
	}
}

func TestGoOnlineFailsClosedWhenComplianceEvaluationFails(
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
	defer db.Close()

	fixture, cleanup, err := testutil.CreateDriverFixture(
		ctx,
		db,
	)
	if err != nil {
		t.Fatalf("create driver fixture: %v", err)
	}

	defer func() {
		if cleanupErr := cleanup(context.Background()); cleanupErr != nil {
			t.Logf("cleanup driver fixture: %v", cleanupErr)
		}
	}()

	_, err = db.Exec(
		ctx,
		`
			INSERT INTO driver_presence (
				driver_id,
				company_id,
				branch_id,
				is_online,
				availability_status,
				updated_at
			)
			VALUES (
				$1,
				$2,
				$3,
				FALSE,
				'OFFLINE',
				NOW()
			)
		`,
		fixture.UserID,
		fixture.CompanyID,
		fixture.BranchID,
	)
	if err != nil {
		t.Fatalf("create offline driver presence: %v", err)
	}

	defer func() {
		_, _ = db.Exec(
			context.Background(),
			`
				DELETE FROM driver_presence
				WHERE driver_id = $1
			`,
			fixture.UserID,
		)
	}()

	compliance := failingComplianceStub()

	service := NewService(
		Dependencies{
			DB:          db,
			Config:      cfg,
			Drivers:     postgresrepo.NewDriverRepository(db),
			Presence:    postgresrepo.NewDriverPresenceRepository(db),
			Assignments: postgresrepo.NewDriverAssignmentRepository(db),
			Compliance:  compliance,
		},
	)

	err = service.GoOnline(
		ctx,
		GoOnlineRequest{
			UserID: fixture.UserID,
		},
	)

	if err == nil {
		t.Fatal(
			"expected compliance evaluation failure",
		)
	}

	if errors.Is(
		err,
		ErrDriverComplianceRequired,
	) {
		t.Fatalf(
			"expected infrastructure error, got compliance rejection: %v",
			err,
		)
	}

	var (
		isOnline           bool
		availabilityStatus string
	)

	err = db.QueryRow(
		ctx,
		`
			SELECT
				is_online,
				availability_status
			FROM driver_presence
			WHERE driver_id = $1
		`,
		fixture.UserID,
	).Scan(
		&isOnline,
		&availabilityStatus,
	)
	if err != nil {
		t.Fatalf(
			"load driver presence after compliance failure: %v",
			err,
		)
	}

	if isOnline {
		t.Fatal(
			"driver became online after compliance evaluation failure",
		)
	}

	if availabilityStatus != StatusOffline {
		t.Fatalf(
			"expected OFFLINE after compliance evaluation failure, got %s",
			availabilityStatus,
		)
	}
}
