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

type fleetActivationFixture struct {
	UserID    string
	CompanyID string
	BranchID  string
	FleetID   string
}

func TestFleetActivationEnforcesTenantAndLifecycle(
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
	defer func() { _ = os.Chdir(originalDir) }()

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("load CONNECT configuration: %v", err)
	}

	db, err := database.Connect(cfg)
	if err != nil {
		t.Fatalf("connect database: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	createFixture := func(t *testing.T) fleetActivationFixture {
		t.Helper()

		f := fleetActivationFixture{
			UserID:    uuid.NewString(),
			CompanyID: uuid.NewString(),
			BranchID:  uuid.NewString(),
			FleetID:   uuid.NewString(),
		}

		if _, err := db.Exec(ctx, `
			INSERT INTO users (
				id, email, password_hash, first_name, last_name
			)
			VALUES ($1, $2, 'test-password-hash', 'Fleet', 'Activation')
		`, f.UserID, f.UserID+"@example.test"); err != nil {
			t.Fatalf("create activation user: %v", err)
		}

		if _, err := db.Exec(ctx, `
			INSERT INTO companies (
				id, name, legal_name, business_id, email,
				country_code, timezone
			)
			VALUES ($1, $2, $3, $4, $5, 'FI', 'Europe/Helsinki')
		`,
			f.CompanyID,
			"Fleet Activation "+f.CompanyID[:8],
			"Fleet Activation "+f.CompanyID[:8]+" Oy",
			"FLA-"+f.CompanyID[:8],
			f.CompanyID+"@example.test",
		); err != nil {
			t.Fatalf("create activation company: %v", err)
		}

		if _, err := db.Exec(ctx, `
			INSERT INTO branches (
				id, company_id, code, name
			)
			VALUES ($1, $2, $3, $4)
		`,
			f.BranchID,
			f.CompanyID,
			"BR-"+f.BranchID[:8],
			"Fleet Activation Branch "+f.BranchID[:8],
		); err != nil {
			t.Fatalf("create activation branch: %v", err)
		}

		if _, err := db.Exec(ctx, `
			INSERT INTO fleets (
				id, company_id, branch_id, code, name,
				description, is_active
			)
			VALUES ($1, $2, $3, $4, $5, $6, TRUE)
		`,
			f.FleetID,
			f.CompanyID,
			f.BranchID,
			"FL-"+f.FleetID[:8],
			"Fleet Activation "+f.FleetID[:8],
			"fleet activation service integration test",
		); err != nil {
			t.Fatalf("create activation fleet: %v", err)
		}

		if _, err := db.Exec(ctx, `
			INSERT INTO company_memberships (user_id, company_id)
			VALUES ($1, $2)
		`, f.UserID, f.CompanyID); err != nil {
			t.Fatalf("create activation membership: %v", err)
		}

		t.Cleanup(func() {
			cleanupCtx := context.Background()
			_, _ = db.Exec(cleanupCtx,
				`DELETE FROM vehicles WHERE fleet_id=$1`, f.FleetID)
			_, _ = db.Exec(cleanupCtx,
				`DELETE FROM company_memberships WHERE user_id=$1`, f.UserID)
			_, _ = db.Exec(cleanupCtx,
				`DELETE FROM fleets WHERE id=$1`, f.FleetID)
			_, _ = db.Exec(cleanupCtx,
				`DELETE FROM branches WHERE id=$1`, f.BranchID)
			_, _ = db.Exec(cleanupCtx,
				`DELETE FROM companies WHERE id=$1`, f.CompanyID)
			_, _ = db.Exec(cleanupCtx,
				`DELETE FROM users WHERE id=$1`, f.UserID)
		})

		return f
	}

	newService := func(roles []string) *Service {
		return NewService(Dependencies{
			DB:     db,
			Fleets: postgresrepo.NewFleetRepository(db),
			UserRoles: &fleetCreateUserRoleRepositoryStub{
				roles: roles,
			},
		})
	}

	createVehicle := func(
		t *testing.T,
		f fleetActivationFixture,
		active bool,
	) {
		t.Helper()

		vehicleID := uuid.NewString()

		if _, err := db.Exec(ctx, `
			INSERT INTO vehicles (
				id,
				company_id,
				branch_id,
				fleet_id,
				registration_number,
				vin,
				make,
				model,
				model_year,
				color,
				vehicle_type,
				fuel_type,
				seating_capacity,
				is_active
			)
			VALUES (
				$1, $2, $3, $4, $5, $6,
				'CONNECT', 'Activation', 2026, 'Black',
				'SEDAN', 'EV', 4, $7
			)
		`,
			vehicleID,
			f.CompanyID,
			f.BranchID,
			f.FleetID,
			"FLA-"+vehicleID[:8],
			"VIN"+vehicleID[:14],
			active,
		); err != nil {
			t.Fatalf("create activation vehicle: %v", err)
		}
	}

	t.Run("member deactivates empty fleet", func(t *testing.T) {
		f := createFixture(t)
		service := newService([]string{"COMPANY_ADMIN"})

		if err := service.Deactivate(ctx, f.UserID, f.FleetID); err != nil {
			t.Fatalf("deactivate own fleet: %v", err)
		}

		var active bool
		if err := db.QueryRow(
			ctx,
			`SELECT is_active FROM fleets WHERE id=$1`,
			f.FleetID,
		).Scan(&active); err != nil {
			t.Fatalf("inspect fleet state: %v", err)
		}
		if active {
			t.Fatal("fleet remained active after deactivation")
		}
	})

	t.Run("inactive vehicle does not block deactivation", func(t *testing.T) {
		f := createFixture(t)
		createVehicle(t, f, false)

		service := newService([]string{"COMPANY_ADMIN"})
		if err := service.Deactivate(ctx, f.UserID, f.FleetID); err != nil {
			t.Fatalf("deactivate fleet with inactive child: %v", err)
		}
	})

	t.Run("active vehicle blocks deactivation", func(t *testing.T) {
		f := createFixture(t)
		createVehicle(t, f, true)

		service := newService([]string{"COMPANY_ADMIN"})
		err := service.Deactivate(ctx, f.UserID, f.FleetID)
		if !errors.Is(err, ErrFleetHasActiveVehicles) {
			t.Fatalf(
				"expected ErrFleetHasActiveVehicles, got %v",
				err,
			)
		}
	})

	t.Run("inactive branch blocks reactivation", func(t *testing.T) {
		f := createFixture(t)
		service := newService([]string{"COMPANY_ADMIN"})

		if _, err := db.Exec(ctx,
			`UPDATE fleets SET is_active=FALSE WHERE id=$1`,
			f.FleetID,
		); err != nil {
			t.Fatalf("deactivate fixture fleet: %v", err)
		}
		if _, err := db.Exec(ctx,
			`UPDATE branches SET is_active=FALSE WHERE id=$1`,
			f.BranchID,
		); err != nil {
			t.Fatalf("deactivate fixture branch: %v", err)
		}

		err := service.Reactivate(ctx, f.UserID, f.FleetID)
		if !errors.Is(err, ErrFleetBranchInactive) {
			t.Fatalf(
				"expected ErrFleetBranchInactive, got %v",
				err,
			)
		}
	})

	t.Run("archived branch blocks reactivation", func(t *testing.T) {
		f := createFixture(t)
		service := newService([]string{"COMPANY_ADMIN"})

		if _, err := db.Exec(ctx,
			`UPDATE fleets SET is_active=FALSE WHERE id=$1`,
			f.FleetID,
		); err != nil {
			t.Fatalf("deactivate fixture fleet: %v", err)
		}
		if _, err := db.Exec(ctx,
			`UPDATE branches SET deleted_at=NOW() WHERE id=$1`,
			f.BranchID,
		); err != nil {
			t.Fatalf("archive fixture branch: %v", err)
		}

		err := service.Reactivate(ctx, f.UserID, f.FleetID)
		if !errors.Is(err, ErrFleetBranchInactive) {
			t.Fatalf(
				"expected ErrFleetBranchInactive, got %v",
				err,
			)
		}
	})

	t.Run("active branch permits reactivation", func(t *testing.T) {
		f := createFixture(t)
		service := newService([]string{"COMPANY_ADMIN"})

		if _, err := db.Exec(ctx,
			`UPDATE fleets SET is_active=FALSE WHERE id=$1`,
			f.FleetID,
		); err != nil {
			t.Fatalf("deactivate fixture fleet: %v", err)
		}

		if err := service.Reactivate(ctx, f.UserID, f.FleetID); err != nil {
			t.Fatalf("reactivate own fleet: %v", err)
		}

		var active bool
		if err := db.QueryRow(
			ctx,
			`SELECT is_active FROM fleets WHERE id=$1`,
			f.FleetID,
		).Scan(&active); err != nil {
			t.Fatalf("inspect reactivated fleet: %v", err)
		}
		if !active {
			t.Fatal("fleet remained inactive after reactivation")
		}
	})

	t.Run("cross-tenant is indistinguishable from nonexistent", func(t *testing.T) {
		owner := createFixture(t)
		other := createFixture(t)
		service := newService([]string{"COMPANY_ADMIN"})

		crossTenantErr := service.Deactivate(
			ctx,
			other.UserID,
			owner.FleetID,
		)
		if !errors.Is(crossTenantErr, repository.ErrNotFound) {
			t.Fatalf(
				"expected repository.ErrNotFound cross-tenant, got %v",
				crossTenantErr,
			)
		}

		missingErr := service.Deactivate(
			ctx,
			other.UserID,
			uuid.NewString(),
		)
		if !errors.Is(missingErr, repository.ErrNotFound) {
			t.Fatalf(
				"expected repository.ErrNotFound nonexistent, got %v",
				missingErr,
			)
		}
	})

	t.Run("system admin is global but lifecycle blockers remain", func(t *testing.T) {
		f := createFixture(t)
		createVehicle(t, f, true)

		if _, err := db.Exec(ctx, `
			DELETE FROM company_memberships
			WHERE user_id=$1 AND company_id=$2
		`, f.UserID, f.CompanyID); err != nil {
			t.Fatalf("remove system admin membership: %v", err)
		}

		service := newService([]string{"SYSTEM_ADMIN"})

		err := service.Deactivate(ctx, f.UserID, f.FleetID)
		if !errors.Is(err, ErrFleetHasActiveVehicles) {
			t.Fatalf(
				"SYSTEM_ADMIN must obey active vehicle blocker: %v",
				err,
			)
		}

		if _, err := db.Exec(ctx,
			`UPDATE vehicles SET is_active=FALSE WHERE fleet_id=$1`,
			f.FleetID,
		); err != nil {
			t.Fatalf("deactivate blocker vehicle: %v", err)
		}

		if err := service.Deactivate(ctx, f.UserID, f.FleetID); err != nil {
			t.Fatalf("SYSTEM_ADMIN global deactivation: %v", err)
		}

		if err := service.Reactivate(ctx, f.UserID, f.FleetID); err != nil {
			t.Fatalf("SYSTEM_ADMIN global reactivation: %v", err)
		}
	})
}
