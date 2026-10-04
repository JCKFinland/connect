package vehicle

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/JCKFinland/connect/backend/internal/config"
	"github.com/JCKFinland/connect/backend/internal/database"
	"github.com/JCKFinland/connect/backend/internal/models"
	"github.com/JCKFinland/connect/backend/internal/repository"
	postgresrepo "github.com/JCKFinland/connect/backend/internal/repository/postgres"
	"github.com/JCKFinland/connect/backend/internal/testutil"
)

func TestVehicleActivationLifecycleEnforcesAssignmentAndFleetIntegrity(
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

	memberships :=
		postgresrepo.NewCompanyMembershipRepository(
			db,
		)

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

	service := NewService(
		Dependencies{
			DB:       db,
			Vehicles: postgresrepo.NewVehicleRepository(db),
			UserRoles: &vehicleUserRoleRepositoryStub{
				roles: []string{"COMPANY_ADMIN"},
			},
			CompanyMemberships: memberships,
		},
	)

	// The fixture begins with an active driver assignment. Deactivation must
	// reject that state rather than silently detaching the driver.
	err = service.Deactivate(
		ctx,
		fixture.UserID,
		fixture.VehicleID,
	)
	if !errors.Is(
		err,
		ErrVehicleHasActiveAssignment,
	) {
		t.Fatalf(
			"expected ErrVehicleHasActiveAssignment, got %v",
			err,
		)
	}

	var (
		isActive bool
		archived bool
	)

	err = db.QueryRow(
		ctx,
		`
			SELECT
				is_active,
				deleted_at IS NOT NULL
			FROM vehicles
			WHERE id = $1
		`,
		fixture.VehicleID,
	).Scan(
		&isActive,
		&archived,
	)
	if err != nil {
		t.Fatalf(
			"inspect vehicle after rejected deactivation: %v",
			err,
		)
	}

	if !isActive {
		t.Fatal(
			"vehicle became inactive despite active assignment",
		)
	}

	if archived {
		t.Fatal(
			"deactivation must not archive vehicle",
		)
	}

	assignments :=
		postgresrepo.NewDriverAssignmentRepository(db)

	if err := assignments.CloseAssignment(
		ctx,
		fixture.AssignmentID,
	); err != nil {
		t.Fatalf("close fixture assignment: %v", err)
	}

	if err := service.Deactivate(
		ctx,
		fixture.UserID,
		fixture.VehicleID,
	); err != nil {
		t.Fatalf("deactivate unassigned vehicle: %v", err)
	}

	err = db.QueryRow(
		ctx,
		`
			SELECT
				is_active,
				deleted_at IS NOT NULL
			FROM vehicles
			WHERE id = $1
		`,
		fixture.VehicleID,
	).Scan(
		&isActive,
		&archived,
	)
	if err != nil {
		t.Fatalf(
			"inspect deactivated vehicle: %v",
			err,
		)
	}

	if isActive {
		t.Fatal("vehicle must be inactive after deactivation")
	}

	if archived {
		t.Fatal(
			"deactivation must preserve deleted_at",
		)
	}

	// Command semantics are idempotent.
	if err := service.Deactivate(
		ctx,
		fixture.UserID,
		fixture.VehicleID,
	); err != nil {
		t.Fatalf(
			"repeat vehicle deactivation must succeed: %v",
			err,
		)
	}

	_, err = db.Exec(
		ctx,
		`
			UPDATE fleets
			SET
				is_active = FALSE,
				updated_at = NOW()
			WHERE id = $1
		`,
		fixture.FleetID,
	)
	if err != nil {
		t.Fatalf("deactivate fixture fleet: %v", err)
	}

	err = service.Reactivate(
		ctx,
		fixture.UserID,
		fixture.VehicleID,
	)
	if !errors.Is(
		err,
		ErrVehicleFleetInactive,
	) {
		t.Fatalf(
			"expected ErrVehicleFleetInactive, got %v",
			err,
		)
	}

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
			"inspect vehicle after rejected reactivation: %v",
			err,
		)
	}

	if isActive {
		t.Fatal(
			"vehicle reactivated while owning fleet was inactive",
		)
	}

	_, err = db.Exec(
		ctx,
		`
			UPDATE fleets
			SET
				is_active = TRUE,
				updated_at = NOW()
			WHERE id = $1
		`,
		fixture.FleetID,
	)
	if err != nil {
		t.Fatalf("reactivate fixture fleet: %v", err)
	}

	if err := service.Reactivate(
		ctx,
		fixture.UserID,
		fixture.VehicleID,
	); err != nil {
		t.Fatalf("reactivate vehicle: %v", err)
	}

	err = db.QueryRow(
		ctx,
		`
			SELECT
				is_active,
				deleted_at IS NOT NULL
			FROM vehicles
			WHERE id = $1
		`,
		fixture.VehicleID,
	).Scan(
		&isActive,
		&archived,
	)
	if err != nil {
		t.Fatalf(
			"inspect reactivated vehicle: %v",
			err,
		)
	}

	if !isActive {
		t.Fatal("vehicle must be active after reactivation")
	}

	if archived {
		t.Fatal(
			"reactivation must preserve deleted_at",
		)
	}

	// Command semantics are idempotent.
	if err := service.Reactivate(
		ctx,
		fixture.UserID,
		fixture.VehicleID,
	); err != nil {
		t.Fatalf(
			"repeat vehicle reactivation must succeed: %v",
			err,
		)
	}

	// Removing tenant membership makes the same vehicle indistinguishable from
	// a nonexistent vehicle to this non-system administrator.
	_, err = db.Exec(
		ctx,
		`
			DELETE FROM company_memberships
			WHERE user_id = $1
			  AND company_id = $2
		`,
		fixture.UserID,
		fixture.CompanyID,
	)
	if err != nil {
		t.Fatalf(
			"remove fixture company membership: %v",
			err,
		)
	}

	err = service.Deactivate(
		ctx,
		fixture.UserID,
		fixture.VehicleID,
	)
	if !errors.Is(
		err,
		repository.ErrNotFound,
	) {
		t.Fatalf(
			"expected cross-tenant deactivation to be not found, got %v",
			err,
		)
	}

	err = service.Reactivate(
		ctx,
		fixture.UserID,
		fixture.VehicleID,
	)
	if !errors.Is(
		err,
		repository.ErrNotFound,
	) {
		t.Fatalf(
			"expected cross-tenant reactivation to be not found, got %v",
			err,
		)
	}

	// Restore membership so this test leaves fixture authority in its original
	// state. The registered cleanup still owns final membership deletion.
	if err := memberships.Create(
		ctx,
		&models.CompanyMembership{
			UserID:    fixture.UserID,
			CompanyID: fixture.CompanyID,
		},
	); err != nil {
		t.Fatalf(
			"restore fixture company membership: %v",
			err,
		)
	}
}
