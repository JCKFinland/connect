package fleet

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/JCKFinland/connect/backend/internal/config"
	"github.com/JCKFinland/connect/backend/internal/database"
	"github.com/JCKFinland/connect/backend/internal/repository"
	postgresrepo "github.com/JCKFinland/connect/backend/internal/repository/postgres"
	"github.com/google/uuid"
)

type fleetArchiveFixture struct {
	UserID    string
	CompanyID string
	BranchID  string
	FleetID   string
}

func TestFleetArchiveEnforcesTenantAndVehicleLifecycle(
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

	createFixture := func(
		t *testing.T,
	) fleetArchiveFixture {
		t.Helper()

		fixture := fleetArchiveFixture{
			UserID:    uuid.NewString(),
			CompanyID: uuid.NewString(),
			BranchID:  uuid.NewString(),
			FleetID:   uuid.NewString(),
		}

		_, err := db.Exec(
			ctx,
			`
				INSERT INTO users (
					id,
					email,
					password_hash,
					first_name,
					last_name
				)
				VALUES ($1, $2, $3, $4, $5)
			`,
			fixture.UserID,
			fixture.UserID+"@example.test",
			"test-password-hash",
			"Fleet",
			"Archive",
		)
		if err != nil {
			t.Fatalf("create archive test user: %v", err)
		}

		_, err = db.Exec(
			ctx,
			`
				INSERT INTO companies (
					id,
					name,
					legal_name,
					business_id,
					email,
					country_code,
					timezone
				)
				VALUES (
					$1,
					$2,
					$3,
					$4,
					$5,
					'FI',
					'Europe/Helsinki'
				)
			`,
			fixture.CompanyID,
			"Fleet Archive "+fixture.CompanyID[:8],
			"Fleet Archive "+fixture.CompanyID[:8]+" Oy",
			"FA-"+fixture.CompanyID[:8],
			fixture.CompanyID+"@example.test",
		)
		if err != nil {
			t.Fatalf("create archive test company: %v", err)
		}

		_, err = db.Exec(
			ctx,
			`
				INSERT INTO branches (
					id,
					company_id,
					code,
					name
				)
				VALUES ($1, $2, $3, $4)
			`,
			fixture.BranchID,
			fixture.CompanyID,
			"BR-"+fixture.BranchID[:8],
			"Fleet Archive Branch "+fixture.BranchID[:8],
		)
		if err != nil {
			t.Fatalf("create archive test branch: %v", err)
		}

		_, err = db.Exec(
			ctx,
			`
				INSERT INTO fleets (
					id,
					company_id,
					branch_id,
					code,
					name,
					description,
					is_active
				)
				VALUES ($1, $2, $3, $4, $5, $6, TRUE)
			`,
			fixture.FleetID,
			fixture.CompanyID,
			fixture.BranchID,
			"FL-"+fixture.FleetID[:8],
			"Fleet Archive "+fixture.FleetID[:8],
			"fleet archive service integration test",
		)
		if err != nil {
			t.Fatalf("create archive test fleet: %v", err)
		}

		_, err = db.Exec(
			ctx,
			`
				INSERT INTO company_memberships (
					user_id,
					company_id
				)
				VALUES ($1, $2)
			`,
			fixture.UserID,
			fixture.CompanyID,
		)
		if err != nil {
			t.Fatalf(
				"create archive test membership: %v",
				err,
			)
		}

		t.Cleanup(func() {
			cleanupCtx := context.Background()

			_, _ = db.Exec(
				cleanupCtx,
				`DELETE FROM vehicles WHERE fleet_id=$1`,
				fixture.FleetID,
			)
			_, _ = db.Exec(
				cleanupCtx,
				`DELETE FROM company_memberships WHERE user_id=$1`,
				fixture.UserID,
			)
			_, _ = db.Exec(
				cleanupCtx,
				`DELETE FROM fleets WHERE id=$1`,
				fixture.FleetID,
			)
			_, _ = db.Exec(
				cleanupCtx,
				`DELETE FROM branches WHERE id=$1`,
				fixture.BranchID,
			)
			_, _ = db.Exec(
				cleanupCtx,
				`DELETE FROM companies WHERE id=$1`,
				fixture.CompanyID,
			)
			_, _ = db.Exec(
				cleanupCtx,
				`DELETE FROM users WHERE id=$1`,
				fixture.UserID,
			)
		})

		return fixture
	}

	newService := func(
		roles []string,
	) *Service {
		return NewService(
			Dependencies{
				DB:     db,
				Fleets: postgresrepo.NewFleetRepository(db),
				UserRoles: &fleetCreateUserRoleRepositoryStub{
					roles: roles,
				},
			},
		)
	}

	createVehicle := func(
		t *testing.T,
		fixture fleetArchiveFixture,
		active bool,
		archived bool,
	) string {
		t.Helper()

		vehicleID := uuid.NewString()

		_, err := db.Exec(
			ctx,
			`
				INSERT INTO vehicles (
					id,
					company_id,
					branch_id,
					fleet_id,
					registration_number,
					make,
					model,
					vehicle_type,
					seating_capacity,
					is_active,
					deleted_at
				)
				VALUES (
					$1,
					$2,
					$3,
					$4,
					$5,
					'CONNECT',
					'Archive Test',
					'SEDAN',
					4,
					$6,
					CASE
						WHEN $7 THEN NOW()
						ELSE NULL
					END
				)
			`,
			vehicleID,
			fixture.CompanyID,
			fixture.BranchID,
			fixture.FleetID,
			"FAR-"+vehicleID[:8],
			active,
			archived,
		)
		if err != nil {
			t.Fatalf("create archive test vehicle: %v", err)
		}

		return vehicleID
	}

	t.Run(
		"company member archives empty fleet and preserves active state",
		func(t *testing.T) {
			fixture := createFixture(t)

			service := newService(
				[]string{"COMPANY_ADMIN"},
			)

			if err := service.Delete(
				ctx,
				fixture.UserID,
				fixture.FleetID,
			); err != nil {
				t.Fatalf("archive own fleet: %v", err)
			}

			var (
				archived bool
				active   bool
			)

			err := db.QueryRow(
				ctx,
				`
					SELECT
						deleted_at IS NOT NULL,
						is_active
					FROM fleets
					WHERE id=$1
				`,
				fixture.FleetID,
			).Scan(
				&archived,
				&active,
			)
			if err != nil {
				t.Fatalf(
					"inspect archived fleet: %v",
					err,
				)
			}

			if !archived {
				t.Fatal(
					"fleet archive did not set deleted_at",
				)
			}
			if !active {
				t.Fatal(
					"fleet archive changed active state",
				)
			}
		},
	)

	t.Run(
		"active non-archived vehicle blocks archive",
		func(t *testing.T) {
			fixture := createFixture(t)
			createVehicle(t, fixture, true, false)

			service := newService(
				[]string{"COMPANY_ADMIN"},
			)

			err := service.Delete(
				ctx,
				fixture.UserID,
				fixture.FleetID,
			)
			if !errors.Is(
				err,
				ErrFleetHasVehicles,
			) {
				t.Fatalf(
					"expected ErrFleetHasVehicles, got %v",
					err,
				)
			}
		},
	)

	t.Run(
		"inactive non-archived vehicle blocks archive",
		func(t *testing.T) {
			fixture := createFixture(t)
			createVehicle(t, fixture, false, false)

			service := newService(
				[]string{"COMPANY_ADMIN"},
			)

			err := service.Delete(
				ctx,
				fixture.UserID,
				fixture.FleetID,
			)
			if !errors.Is(
				err,
				ErrFleetHasVehicles,
			) {
				t.Fatalf(
					"expected ErrFleetHasVehicles, got %v",
					err,
				)
			}
		},
	)

	t.Run(
		"archived vehicle does not block fleet archive",
		func(t *testing.T) {
			fixture := createFixture(t)
			vehicleID := createVehicle(
				t,
				fixture,
				true,
				true,
			)

			service := newService(
				[]string{"COMPANY_ADMIN"},
			)

			if err := service.Delete(
				ctx,
				fixture.UserID,
				fixture.FleetID,
			); err != nil {
				t.Fatalf(
					"archive fleet with archived child: %v",
					err,
				)
			}

			var (
				fleetArchived   bool
				vehicleArchived bool
				vehicleActive   bool
			)

			if err := db.QueryRow(
				ctx,
				`
					SELECT deleted_at IS NOT NULL
					FROM fleets
					WHERE id=$1
				`,
				fixture.FleetID,
			).Scan(&fleetArchived); err != nil {
				t.Fatalf(
					"inspect fleet archive state: %v",
					err,
				)
			}

			if err := db.QueryRow(
				ctx,
				`
					SELECT
						deleted_at IS NOT NULL,
						is_active
					FROM vehicles
					WHERE id=$1
				`,
				vehicleID,
			).Scan(
				&vehicleArchived,
				&vehicleActive,
			); err != nil {
				t.Fatalf(
					"inspect archived child vehicle: %v",
					err,
				)
			}

			if !fleetArchived {
				t.Fatal("fleet was not archived")
			}
			if !vehicleArchived {
				t.Fatal(
					"fleet archive unexpectedly restored child vehicle",
				)
			}
			if !vehicleActive {
				t.Fatal(
					"fleet archive mutated archived child activation state",
				)
			}
		},
	)

	t.Run(
		"cross-tenant fleet is indistinguishable from nonexistent",
		func(t *testing.T) {
			owner := createFixture(t)
			other := createFixture(t)

			service := newService(
				[]string{"COMPANY_ADMIN"},
			)

			crossTenantErr := service.Delete(
				ctx,
				other.UserID,
				owner.FleetID,
			)
			if !errors.Is(
				crossTenantErr,
				repository.ErrNotFound,
			) {
				t.Fatalf(
					"expected repository.ErrNotFound cross-tenant, got %v",
					crossTenantErr,
				)
			}

			missingErr := service.Delete(
				ctx,
				other.UserID,
				uuid.NewString(),
			)
			if !errors.Is(
				missingErr,
				repository.ErrNotFound,
			) {
				t.Fatalf(
					"expected repository.ErrNotFound nonexistent, got %v",
					missingErr,
				)
			}
		},
	)

	t.Run(
		"system admin archives globally but lifecycle blocker remains",
		func(t *testing.T) {
			fixture := createFixture(t)

			service := newService(
				[]string{"SYSTEM_ADMIN"},
			)

			// SYSTEM_ADMIN does not need company membership.
			if _, err := db.Exec(
				ctx,
				`
					DELETE FROM company_memberships
					WHERE user_id=$1
					  AND company_id=$2
				`,
				fixture.UserID,
				fixture.CompanyID,
			); err != nil {
				t.Fatalf(
					"remove system admin membership: %v",
					err,
				)
			}

			createVehicle(t, fixture, false, false)

			err := service.Delete(
				ctx,
				fixture.UserID,
				fixture.FleetID,
			)
			if !errors.Is(
				err,
				ErrFleetHasVehicles,
			) {
				t.Fatalf(
					"SYSTEM_ADMIN must still obey vehicle blocker: %v",
					err,
				)
			}

			if _, err := db.Exec(
				ctx,
				`
					UPDATE vehicles
					SET deleted_at=NOW()
					WHERE fleet_id=$1
				`,
				fixture.FleetID,
			); err != nil {
				t.Fatalf(
					"archive blocker vehicle: %v",
					err,
				)
			}

			if err := service.Delete(
				ctx,
				fixture.UserID,
				fixture.FleetID,
			); err != nil {
				t.Fatalf(
					"SYSTEM_ADMIN global archive: %v",
					err,
				)
			}
		},
	)
}
