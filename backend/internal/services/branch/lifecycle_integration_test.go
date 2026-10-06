package branch

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

type branchLifecycleFixture struct {
	UserID    string
	CompanyID string
	BranchID  string
}

func TestBranchLifecycleEnforcesTenantAndFleetAuthority(t *testing.T) {
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

	createFixture := func(t *testing.T) branchLifecycleFixture {
		t.Helper()

		f := branchLifecycleFixture{
			UserID:    uuid.NewString(),
			CompanyID: uuid.NewString(),
			BranchID:  uuid.NewString(),
		}

		if _, err := db.Exec(ctx, `
			INSERT INTO users (
				id, email, password_hash, first_name, last_name
			)
			VALUES ($1, $2, 'test-password-hash', 'Branch', 'Lifecycle')
		`, f.UserID, f.UserID+"@example.test"); err != nil {
			t.Fatalf("create lifecycle user: %v", err)
		}

		if _, err := db.Exec(ctx, `
			INSERT INTO companies (
				id, name, legal_name, business_id, email,
				country_code, timezone
			)
			VALUES ($1, $2, $3, $4, $5, 'FI', 'Europe/Helsinki')
		`,
			f.CompanyID,
			"Branch Lifecycle "+f.CompanyID[:8],
			"Branch Lifecycle "+f.CompanyID[:8]+" Oy",
			"BRL-"+f.CompanyID[:8],
			f.CompanyID+"@example.test",
		); err != nil {
			t.Fatalf("create lifecycle company: %v", err)
		}

		if _, err := db.Exec(ctx, `
			INSERT INTO branches (
				id, company_id, code, name, is_active
			)
			VALUES ($1, $2, $3, $4, TRUE)
		`,
			f.BranchID,
			f.CompanyID,
			"BR-"+f.BranchID[:8],
			"Branch Lifecycle "+f.BranchID[:8],
		); err != nil {
			t.Fatalf("create lifecycle branch: %v", err)
		}

		if _, err := db.Exec(ctx, `
			INSERT INTO company_memberships (user_id, company_id)
			VALUES ($1, $2)
		`, f.UserID, f.CompanyID); err != nil {
			t.Fatalf("create lifecycle membership: %v", err)
		}

		t.Cleanup(func() {
			cleanupCtx := context.Background()
			_, _ = db.Exec(
				cleanupCtx,
				`DELETE FROM fleets WHERE branch_id=$1`,
				f.BranchID,
			)
			_, _ = db.Exec(
				cleanupCtx,
				`DELETE FROM company_memberships WHERE user_id=$1`,
				f.UserID,
			)
			_, _ = db.Exec(
				cleanupCtx,
				`DELETE FROM branches WHERE id=$1`,
				f.BranchID,
			)
			_, _ = db.Exec(
				cleanupCtx,
				`DELETE FROM companies WHERE id=$1`,
				f.CompanyID,
			)
			_, _ = db.Exec(
				cleanupCtx,
				`DELETE FROM users WHERE id=$1`,
				f.UserID,
			)
		})

		return f
	}

	createFleet := func(
		t *testing.T,
		f branchLifecycleFixture,
		active bool,
	) string {
		t.Helper()

		fleetID := uuid.NewString()
		if _, err := db.Exec(ctx, `
			INSERT INTO fleets (
				id,
				company_id,
				branch_id,
				code,
				name,
				description,
				is_active
			)
			VALUES ($1, $2, $3, $4, $5, $6, $7)
		`,
			fleetID,
			f.CompanyID,
			f.BranchID,
			"FL-"+fleetID[:8],
			"Branch Lifecycle Fleet "+fleetID[:8],
			"branch lifecycle integration test",
			active,
		); err != nil {
			t.Fatalf("create lifecycle fleet: %v", err)
		}

		return fleetID
	}

	newService := func(roles []string) *Service {
		return NewService(Dependencies{
			DB:       db,
			Branches: postgresrepo.NewBranchRepository(db),
			UserRoles: &branchAuthorityUserRoleRepositoryStub{
				roles: roles,
			},
			CompanyMemberships: postgresrepo.NewCompanyMembershipRepository(db),
		})
	}

	t.Run("member deactivates empty branch", func(t *testing.T) {
		f := createFixture(t)
		service := newService([]string{"COMPANY_ADMIN"})

		if err := service.Deactivate(ctx, f.UserID, f.BranchID); err != nil {
			t.Fatalf("deactivate own empty branch: %v", err)
		}

		var active bool
		if err := db.QueryRow(ctx, `
			SELECT is_active
			FROM branches
			WHERE id=$1
		`, f.BranchID).Scan(&active); err != nil {
			t.Fatalf("inspect branch state: %v", err)
		}
		if active {
			t.Fatal("branch remained active after deactivation")
		}
	})

	t.Run("inactive fleet does not block branch deactivation", func(t *testing.T) {
		f := createFixture(t)
		createFleet(t, f, false)

		service := newService([]string{"COMPANY_ADMIN"})
		if err := service.Deactivate(ctx, f.UserID, f.BranchID); err != nil {
			t.Fatalf("deactivate branch with inactive fleet: %v", err)
		}

		var active bool
		if err := db.QueryRow(ctx, `
			SELECT is_active
			FROM branches
			WHERE id=$1
		`, f.BranchID).Scan(&active); err != nil {
			t.Fatalf("inspect branch state: %v", err)
		}
		if active {
			t.Fatal("branch with inactive fleet remained active")
		}
	})

	t.Run("active fleet blocks branch deactivation", func(t *testing.T) {
		f := createFixture(t)
		createFleet(t, f, true)

		service := newService([]string{"COMPANY_ADMIN"})
		err := service.Deactivate(ctx, f.UserID, f.BranchID)
		if !errors.Is(err, ErrBranchHasActiveFleets) {
			t.Fatalf(
				"expected ErrBranchHasActiveFleets, got %v",
				err,
			)
		}

		var active bool
		if err := db.QueryRow(ctx, `
			SELECT is_active
			FROM branches
			WHERE id=$1
		`, f.BranchID).Scan(&active); err != nil {
			t.Fatalf("inspect blocked branch: %v", err)
		}
		if !active {
			t.Fatal("blocked branch was deactivated")
		}
	})

	t.Run("inactive non-archived fleet blocks branch archive", func(t *testing.T) {
		f := createFixture(t)
		fleetID := createFleet(t, f, false)

		service := newService([]string{"COMPANY_ADMIN"})
		err := service.Delete(ctx, f.UserID, f.BranchID)
		if !errors.Is(err, ErrBranchHasFleets) {
			t.Fatalf("expected ErrBranchHasFleets, got %v", err)
		}

		var branchDeleted bool
		if err := db.QueryRow(ctx, `
			SELECT deleted_at IS NOT NULL
			FROM branches
			WHERE id=$1
		`, f.BranchID).Scan(&branchDeleted); err != nil {
			t.Fatalf("inspect blocked branch archive: %v", err)
		}
		if branchDeleted {
			t.Fatal("branch archived despite non-archived fleet")
		}

		var fleetDeleted bool
		if err := db.QueryRow(ctx, `
			SELECT deleted_at IS NOT NULL
			FROM fleets
			WHERE id=$1
		`, fleetID).Scan(&fleetDeleted); err != nil {
			t.Fatalf("inspect child fleet: %v", err)
		}
		if fleetDeleted {
			t.Fatal("branch archive silently cascaded to child fleet")
		}
	})

	t.Run("active non-archived fleet blocks branch archive", func(t *testing.T) {
		f := createFixture(t)
		createFleet(t, f, true)

		service := newService([]string{"COMPANY_ADMIN"})
		err := service.Delete(ctx, f.UserID, f.BranchID)
		if !errors.Is(err, ErrBranchHasFleets) {
			t.Fatalf("expected ErrBranchHasFleets, got %v", err)
		}
	})

	t.Run("archived fleet does not block branch archive", func(t *testing.T) {
		f := createFixture(t)
		fleetID := createFleet(t, f, false)

		if _, err := db.Exec(ctx, `
			UPDATE fleets
			SET deleted_at=NOW()
			WHERE id=$1
		`, fleetID); err != nil {
			t.Fatalf("archive fixture fleet: %v", err)
		}

		service := newService([]string{"COMPANY_ADMIN"})
		if err := service.Delete(ctx, f.UserID, f.BranchID); err != nil {
			t.Fatalf("archive branch with only archived fleet: %v", err)
		}

		var deleted bool
		if err := db.QueryRow(ctx, `
			SELECT deleted_at IS NOT NULL
			FROM branches
			WHERE id=$1
		`, f.BranchID).Scan(&deleted); err != nil {
			t.Fatalf("inspect archived branch: %v", err)
		}
		if !deleted {
			t.Fatal("branch was not archived")
		}
	})

	t.Run("empty branch archives without mutating active state", func(t *testing.T) {
		f := createFixture(t)
		service := newService([]string{"COMPANY_ADMIN"})

		if err := service.Delete(ctx, f.UserID, f.BranchID); err != nil {
			t.Fatalf("archive empty branch: %v", err)
		}

		var active bool
		var deleted bool
		if err := db.QueryRow(ctx, `
			SELECT is_active, deleted_at IS NOT NULL
			FROM branches
			WHERE id=$1
		`, f.BranchID).Scan(&active, &deleted); err != nil {
			t.Fatalf("inspect archived branch: %v", err)
		}
		if !deleted {
			t.Fatal("branch was not archived")
		}
		if !active {
			t.Fatal("archive silently changed operational active state")
		}
	})

	t.Run("member reactivates inactive branch", func(t *testing.T) {
		f := createFixture(t)

		if _, err := db.Exec(ctx, `
			UPDATE branches
			SET is_active=FALSE
			WHERE id=$1
		`, f.BranchID); err != nil {
			t.Fatalf("deactivate fixture branch: %v", err)
		}

		service := newService([]string{"COMPANY_ADMIN"})
		if err := service.Reactivate(ctx, f.UserID, f.BranchID); err != nil {
			t.Fatalf("reactivate own branch: %v", err)
		}

		var active bool
		if err := db.QueryRow(ctx, `
			SELECT is_active
			FROM branches
			WHERE id=$1
		`, f.BranchID).Scan(&active); err != nil {
			t.Fatalf("inspect reactivated branch: %v", err)
		}
		if !active {
			t.Fatal("branch remained inactive after reactivation")
		}
	})

	t.Run("cross-tenant lifecycle is indistinguishable from nonexistent", func(t *testing.T) {
		owner := createFixture(t)
		other := createFixture(t)
		service := newService([]string{"COMPANY_ADMIN"})

		crossTenantErr := service.Deactivate(
			ctx,
			other.UserID,
			owner.BranchID,
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

	t.Run("system admin is global but cannot bypass active fleet blocker", func(t *testing.T) {
		f := createFixture(t)
		createFleet(t, f, true)

		if _, err := db.Exec(ctx, `
			DELETE FROM company_memberships
			WHERE user_id=$1 AND company_id=$2
		`, f.UserID, f.CompanyID); err != nil {
			t.Fatalf("remove system admin membership: %v", err)
		}

		service := newService([]string{"SYSTEM_ADMIN"})
		err := service.Deactivate(ctx, f.UserID, f.BranchID)
		if !errors.Is(err, ErrBranchHasActiveFleets) {
			t.Fatalf(
				"expected ErrBranchHasActiveFleets, got %v",
				err,
			)
		}
	})

	t.Run("system admin is global but cannot bypass archive blocker", func(t *testing.T) {
		f := createFixture(t)
		createFleet(t, f, false)

		if _, err := db.Exec(ctx, `
			DELETE FROM company_memberships
			WHERE user_id=$1 AND company_id=$2
		`, f.UserID, f.CompanyID); err != nil {
			t.Fatalf("remove system admin membership: %v", err)
		}

		service := newService([]string{"SYSTEM_ADMIN"})
		err := service.Delete(ctx, f.UserID, f.BranchID)
		if !errors.Is(err, ErrBranchHasFleets) {
			t.Fatalf(
				"expected ErrBranchHasFleets, got %v",
				err,
			)
		}
	})
}
