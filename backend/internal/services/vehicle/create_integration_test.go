package vehicle

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/JCKFinland/connect/backend/internal/config"
	"github.com/JCKFinland/connect/backend/internal/database"
	"github.com/JCKFinland/connect/backend/internal/models"
	postgresrepo "github.com/JCKFinland/connect/backend/internal/repository/postgres"
	"github.com/JCKFinland/connect/backend/internal/testutil"
	"github.com/google/uuid"
)

func TestVehicleCreateEnforcesFleetAndTenantAuthority(
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

	fixture, cleanup, err := testutil.CreateDriverFixture(
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

	createdVehicleIDs := make([]string, 0, 2)
	otherCompanyID := uuid.NewString()

	t.Cleanup(func() {
		cleanupCtx := context.Background()

		for _, vehicleID := range createdVehicleIDs {
			if _, err := db.Exec(
				cleanupCtx,
				`DELETE FROM vehicles WHERE id = $1`,
				vehicleID,
			); err != nil {
				t.Errorf(
					"cleanup created vehicle %s: %v",
					vehicleID,
					err,
				)
			}
		}

		if _, err := db.Exec(
			cleanupCtx,
			`
				DELETE FROM company_memberships
				WHERE user_id = $1
			`,
			fixture.UserID,
		); err != nil {
			t.Errorf("cleanup company memberships: %v", err)
		}

		if _, err := db.Exec(
			cleanupCtx,
			`DELETE FROM companies WHERE id = $1`,
			otherCompanyID,
		); err != nil {
			t.Errorf("cleanup cross-tenant company: %v", err)
		}
	})

	memberService := NewService(
		Dependencies{
			DB:       db,
			Vehicles: postgresrepo.NewVehicleRepository(db),
			Fleets:   postgresrepo.NewFleetRepository(db),
			UserRoles: &vehicleUserRoleRepositoryStub{
				roles: []string{"COMPANY_ADMIN"},
			},
			CompanyMemberships: memberships,
		},
	)

	t.Run(
		"derives tenant from fleet and creates active vehicle",
		func(t *testing.T) {
			registration := "CRT-" + uuid.NewString()[:8]

			result, err := memberService.Create(
				ctx,
				fixture.UserID,
				CreateVehicleRequest{
					FleetID:            fixture.FleetID,
					RegistrationNumber: registration,
					Make:               "Toyota",
					Model:              "Corolla",
					ModelYear:          2026,
					Color:              "Black",
					VehicleType:        "SEDAN",
					FuelType:           "HYBRID",
					SeatingCapacity:    4,
				},
			)
			if err != nil {
				t.Fatalf("create vehicle: %v", err)
			}

			createdVehicleIDs = append(
				createdVehicleIDs,
				result.ID,
			)

			if result.CompanyID != fixture.CompanyID {
				t.Fatalf(
					"expected company %q, got %q",
					fixture.CompanyID,
					result.CompanyID,
				)
			}

			if result.BranchID != fixture.BranchID {
				t.Fatalf(
					"expected branch %q, got %q",
					fixture.BranchID,
					result.BranchID,
				)
			}

			if result.FleetID != fixture.FleetID {
				t.Fatalf(
					"expected fleet %q, got %q",
					fixture.FleetID,
					result.FleetID,
				)
			}

			if !result.IsActive {
				t.Fatal(
					"new vehicle must be active",
				)
			}

			var (
				companyID string
				branchID  string
				fleetID   string
				isActive  bool
			)

			err = db.QueryRow(
				ctx,
				`
					SELECT
						company_id,
						branch_id,
						fleet_id,
						is_active
					FROM vehicles
					WHERE id = $1
				`,
				result.ID,
			).Scan(
				&companyID,
				&branchID,
				&fleetID,
				&isActive,
			)
			if err != nil {
				t.Fatalf(
					"inspect created vehicle: %v",
					err,
				)
			}

			if companyID != fixture.CompanyID ||
				branchID != fixture.BranchID ||
				fleetID != fixture.FleetID {

				t.Fatalf(
					"persisted vehicle tenant mismatch: company=%q branch=%q fleet=%q",
					companyID,
					branchID,
					fleetID,
				)
			}

			if !isActive {
				t.Fatal(
					"persisted new vehicle must be active",
				)
			}
		},
	)

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
		otherCompanyID,
		"Vehicle Create Other Company",
		"Vehicle Create Other Company Oy",
		"VC-"+otherCompanyID[:8],
		otherCompanyID+"@example.test",
	)
	if err != nil {
		t.Fatalf(
			"create cross-tenant company: %v",
			err,
		)
	}

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
			"remove owning company membership: %v",
			err,
		)
	}

	if err := memberships.Create(
		ctx,
		&models.CompanyMembership{
			UserID:    fixture.UserID,
			CompanyID: otherCompanyID,
		},
	); err != nil {
		t.Fatalf(
			"create unrelated company membership: %v",
			err,
		)
	}

	t.Run(
		"rejects cross-tenant company membership",
		func(t *testing.T) {
			registration := "XTN-" + uuid.NewString()[:8]

			_, err := memberService.Create(
				ctx,
				fixture.UserID,
				CreateVehicleRequest{
					FleetID:            fixture.FleetID,
					RegistrationNumber: registration,
					Make:               "Toyota",
					Model:              "Corolla",
					VehicleType:        "SEDAN",
					FuelType:           "HYBRID",
					SeatingCapacity:    4,
				},
			)
			if !errors.Is(
				err,
				ErrVehicleCreationAccessDenied,
			) {
				t.Fatalf(
					"expected ErrVehicleCreationAccessDenied, got %v",
					err,
				)
			}

			var count int
			if err := db.QueryRow(
				ctx,
				`
					SELECT COUNT(*)
					FROM vehicles
					WHERE registration_number = $1
				`,
				registration,
			).Scan(&count); err != nil {
				t.Fatalf(
					"count cross-tenant vehicle: %v",
					err,
				)
			}

			if count != 0 {
				t.Fatal(
					"cross-tenant vehicle must not be created",
				)
			}
		},
	)

	systemService := NewService(
		Dependencies{
			DB:       db,
			Vehicles: postgresrepo.NewVehicleRepository(db),
			Fleets:   postgresrepo.NewFleetRepository(db),
			UserRoles: &vehicleUserRoleRepositoryStub{
				roles: []string{"SYSTEM_ADMIN"},
			},
			CompanyMemberships: memberships,
		},
	)

	t.Run(
		"allows system admin without owning company membership",
		func(t *testing.T) {
			result, err := systemService.Create(
				ctx,
				fixture.UserID,
				CreateVehicleRequest{
					FleetID: fixture.FleetID,
					RegistrationNumber: "SYS-" +
						uuid.NewString()[:8],
					Make:            "Toyota",
					Model:           "Corolla",
					VehicleType:     "SEDAN",
					FuelType:        "HYBRID",
					SeatingCapacity: 4,
				},
			)
			if err != nil {
				t.Fatalf(
					"system admin create vehicle: %v",
					err,
				)
			}

			createdVehicleIDs = append(
				createdVehicleIDs,
				result.ID,
			)

			if result.CompanyID != fixture.CompanyID ||
				result.BranchID != fixture.BranchID ||
				result.FleetID != fixture.FleetID {

				t.Fatalf(
					"unexpected system admin vehicle tenant: %#v",
					result,
				)
			}
		},
	)

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

	t.Cleanup(func() {
		_, cleanupErr := db.Exec(
			context.Background(),
			`
				UPDATE fleets
				SET
					is_active = TRUE,
					updated_at = NOW()
				WHERE id = $1
			`,
			fixture.FleetID,
		)
		if cleanupErr != nil {
			t.Errorf(
				"restore fixture fleet activation: %v",
				cleanupErr,
			)
		}
	})

	t.Run(
		"rejects inactive fleet",
		func(t *testing.T) {
			registration := "INA-" + uuid.NewString()[:8]

			_, err := systemService.Create(
				ctx,
				fixture.UserID,
				CreateVehicleRequest{
					FleetID:            fixture.FleetID,
					RegistrationNumber: registration,
					Make:               "Toyota",
					Model:              "Corolla",
					VehicleType:        "SEDAN",
					FuelType:           "HYBRID",
					SeatingCapacity:    4,
				},
			)
			if !errors.Is(err, ErrInvalidFleet) {
				t.Fatalf(
					"expected ErrInvalidFleet, got %v",
					err,
				)
			}

			var count int
			if err := db.QueryRow(
				ctx,
				`
					SELECT COUNT(*)
					FROM vehicles
					WHERE registration_number = $1
				`,
				registration,
			).Scan(&count); err != nil {
				t.Fatalf(
					"count inactive-fleet vehicle: %v",
					err,
				)
			}

			if count != 0 {
				t.Fatal(
					"vehicle must not be created in inactive fleet",
				)
			}
		},
	)
}
