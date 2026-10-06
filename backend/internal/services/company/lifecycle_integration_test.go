package company

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/JCKFinland/connect/backend/internal/config"
	"github.com/JCKFinland/connect/backend/internal/database"
	postgresrepo "github.com/JCKFinland/connect/backend/internal/repository/postgres"
	"github.com/google/uuid"
)

type companyLifecycleFixture struct {
	UserID    string
	CompanyID string
}

func TestCompanyLifecycleEnforcesTenantAndBranchAuthority(t *testing.T) {
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

	createFixture := func(t *testing.T) companyLifecycleFixture {
		t.Helper()

		f := companyLifecycleFixture{
			UserID:    uuid.NewString(),
			CompanyID: uuid.NewString(),
		}

		if _, err := db.Exec(ctx, `
			INSERT INTO users (
				id, email, password_hash, first_name, last_name
			)
			VALUES ($1, $2, 'test-password-hash', 'Company', 'Lifecycle')
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
			"Company Lifecycle "+f.CompanyID[:8],
			"Company Lifecycle "+f.CompanyID[:8]+" Oy",
			"COL-"+f.CompanyID[:8],
			f.CompanyID+"@example.test",
		); err != nil {
			t.Fatalf("create lifecycle company: %v", err)
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
				`DELETE FROM fleets WHERE company_id=$1`,
				f.CompanyID,
			)
			_, _ = db.Exec(
				cleanupCtx,
				`DELETE FROM branches WHERE company_id=$1`,
				f.CompanyID,
			)
			_, _ = db.Exec(
				cleanupCtx,
				`DELETE FROM company_memberships WHERE company_id=$1`,
				f.CompanyID,
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

	createBranch := func(
		t *testing.T,
		f companyLifecycleFixture,
		active bool,
	) string {
		t.Helper()

		branchID := uuid.NewString()
		if _, err := db.Exec(ctx, `
			INSERT INTO branches (
				id, company_id, code, name, is_active
			)
			VALUES ($1, $2, $3, $4, $5)
		`,
			branchID,
			f.CompanyID,
			"BR-"+branchID[:8],
			"Company Lifecycle Branch "+branchID[:8],
			active,
		); err != nil {
			t.Fatalf("create lifecycle branch: %v", err)
		}

		return branchID
	}

	newService := func(roles []string) *Service {
		return NewService(Dependencies{
			Config:             cfg,
			DB:                 db,
			Companies:          postgresrepo.NewCompanyRepository(db),
			UserRoles:          &companyAuthorityUserRoleRepositoryStub{roles: roles},
			CompanyMemberships: postgresrepo.NewCompanyMembershipRepository(db),
		})
	}

	t.Run("member deactivates company with no branches", func(t *testing.T) {
		f := createFixture(t)
		service := newService([]string{"COMPANY_ADMIN"})

		if err := service.Deactivate(ctx, f.UserID, f.CompanyID); err != nil {
			t.Fatalf("deactivate own empty company: %v", err)
		}

		var active bool
		if err := db.QueryRow(ctx, `
			SELECT is_active
			FROM companies
			WHERE id=$1
		`, f.CompanyID).Scan(&active); err != nil {
			t.Fatalf("inspect company state: %v", err)
		}
		if active {
			t.Fatal("company remained active after deactivation")
		}
	})

	t.Run("inactive branch does not block company deactivation", func(t *testing.T) {
		f := createFixture(t)
		branchID := createBranch(t, f, false)

		service := newService([]string{"COMPANY_ADMIN"})
		if err := service.Deactivate(ctx, f.UserID, f.CompanyID); err != nil {
			t.Fatalf("deactivate company with inactive branch: %v", err)
		}

		var companyActive bool
		if err := db.QueryRow(ctx, `
			SELECT is_active
			FROM companies
			WHERE id=$1
		`, f.CompanyID).Scan(&companyActive); err != nil {
			t.Fatalf("inspect company state: %v", err)
		}
		if companyActive {
			t.Fatal("company with inactive branch remained active")
		}

		var branchActive bool
		if err := db.QueryRow(ctx, `
			SELECT is_active
			FROM branches
			WHERE id=$1
		`, branchID).Scan(&branchActive); err != nil {
			t.Fatalf("inspect inactive child branch: %v", err)
		}
		if branchActive {
			t.Fatal("inactive child branch was unexpectedly activated")
		}
	})

	t.Run("active branch blocks company deactivation", func(t *testing.T) {
		f := createFixture(t)
		branchID := createBranch(t, f, true)

		service := newService([]string{"COMPANY_ADMIN"})
		err := service.Deactivate(ctx, f.UserID, f.CompanyID)
		if !errors.Is(err, ErrCompanyHasActiveBranches) {
			t.Fatalf(
				"expected ErrCompanyHasActiveBranches, got %v",
				err,
			)
		}

		var companyActive bool
		var branchActive bool
		if err := db.QueryRow(ctx, `
			SELECT is_active
			FROM companies
			WHERE id=$1
		`, f.CompanyID).Scan(&companyActive); err != nil {
			t.Fatalf("inspect blocked company: %v", err)
		}
		if err := db.QueryRow(ctx, `
			SELECT is_active
			FROM branches
			WHERE id=$1
		`, branchID).Scan(&branchActive); err != nil {
			t.Fatalf("inspect child branch: %v", err)
		}

		if !companyActive {
			t.Fatal("company was deactivated despite active branch")
		}
		if !branchActive {
			t.Fatal("company deactivation silently changed child branch")
		}
	})

	t.Run("inactive non-archived branch blocks company archive", func(t *testing.T) {
		f := createFixture(t)
		branchID := createBranch(t, f, false)

		service := newService([]string{"COMPANY_ADMIN"})
		err := service.Delete(ctx, f.UserID, f.CompanyID)
		if !errors.Is(err, ErrCompanyHasBranches) {
			t.Fatalf("expected ErrCompanyHasBranches, got %v", err)
		}

		var companyDeleted bool
		if err := db.QueryRow(ctx, `
			SELECT deleted_at IS NOT NULL
			FROM companies
			WHERE id=$1
		`, f.CompanyID).Scan(&companyDeleted); err != nil {
			t.Fatalf("inspect blocked company archive: %v", err)
		}
		if companyDeleted {
			t.Fatal("company archived despite non-archived branch")
		}

		var branchDeleted bool
		if err := db.QueryRow(ctx, `
			SELECT deleted_at IS NOT NULL
			FROM branches
			WHERE id=$1
		`, branchID).Scan(&branchDeleted); err != nil {
			t.Fatalf("inspect child branch: %v", err)
		}
		if branchDeleted {
			t.Fatal("company archive silently cascaded to child branch")
		}
	})

	t.Run("active non-archived branch blocks company archive", func(t *testing.T) {
		f := createFixture(t)
		createBranch(t, f, true)

		service := newService([]string{"COMPANY_ADMIN"})
		err := service.Delete(ctx, f.UserID, f.CompanyID)
		if !errors.Is(err, ErrCompanyHasBranches) {
			t.Fatalf("expected ErrCompanyHasBranches, got %v", err)
		}
	})

	t.Run("archived branch does not block company archive", func(t *testing.T) {
		f := createFixture(t)
		branchID := createBranch(t, f, false)

		if _, err := db.Exec(ctx, `
			UPDATE branches
			SET deleted_at=NOW()
			WHERE id=$1
		`, branchID); err != nil {
			t.Fatalf("archive fixture branch: %v", err)
		}

		service := newService([]string{"COMPANY_ADMIN"})
		if err := service.Delete(ctx, f.UserID, f.CompanyID); err != nil {
			t.Fatalf("archive company with only archived branch: %v", err)
		}

		var deleted bool
		if err := db.QueryRow(ctx, `
			SELECT deleted_at IS NOT NULL
			FROM companies
			WHERE id=$1
		`, f.CompanyID).Scan(&deleted); err != nil {
			t.Fatalf("inspect archived company: %v", err)
		}
		if !deleted {
			t.Fatal("company was not archived")
		}
	})

	t.Run("empty company archives without mutating active state", func(t *testing.T) {
		f := createFixture(t)
		service := newService([]string{"COMPANY_ADMIN"})

		if err := service.Delete(ctx, f.UserID, f.CompanyID); err != nil {
			t.Fatalf("archive empty company: %v", err)
		}

		var active bool
		var deleted bool
		if err := db.QueryRow(ctx, `
			SELECT is_active, deleted_at IS NOT NULL
			FROM companies
			WHERE id=$1
		`, f.CompanyID).Scan(&active, &deleted); err != nil {
			t.Fatalf("inspect archived company: %v", err)
		}

		if !deleted {
			t.Fatal("company was not archived")
		}
		if !active {
			t.Fatal("archive silently changed operational active state")
		}
	})

	t.Run("member reactivates inactive company", func(t *testing.T) {
		f := createFixture(t)

		if _, err := db.Exec(ctx, `
			UPDATE companies
			SET is_active=FALSE
			WHERE id=$1
		`, f.CompanyID); err != nil {
			t.Fatalf("deactivate fixture company: %v", err)
		}

		service := newService([]string{"COMPANY_ADMIN"})
		if err := service.Reactivate(ctx, f.UserID, f.CompanyID); err != nil {
			t.Fatalf("reactivate own company: %v", err)
		}

		var active bool
		if err := db.QueryRow(ctx, `
			SELECT is_active
			FROM companies
			WHERE id=$1
		`, f.CompanyID).Scan(&active); err != nil {
			t.Fatalf("inspect reactivated company: %v", err)
		}
		if !active {
			t.Fatal("company remained inactive after reactivation")
		}
	})

	t.Run("cross-tenant lifecycle is indistinguishable from nonexistent", func(t *testing.T) {
		owner := createFixture(t)
		other := createFixture(t)
		service := newService([]string{"COMPANY_ADMIN"})

		crossTenantErr := service.Deactivate(
			ctx,
			other.UserID,
			owner.CompanyID,
		)
		if !errors.Is(crossTenantErr, ErrCompanyNotFound) {
			t.Fatalf(
				"expected ErrCompanyNotFound cross-tenant, got %v",
				crossTenantErr,
			)
		}

		missingErr := service.Deactivate(
			ctx,
			other.UserID,
			uuid.NewString(),
		)
		if !errors.Is(missingErr, ErrCompanyNotFound) {
			t.Fatalf(
				"expected ErrCompanyNotFound nonexistent, got %v",
				missingErr,
			)
		}
	})

	t.Run("system admin is global but cannot bypass active branch blocker", func(t *testing.T) {
		f := createFixture(t)
		createBranch(t, f, true)

		if _, err := db.Exec(ctx, `
			DELETE FROM company_memberships
			WHERE user_id=$1 AND company_id=$2
		`, f.UserID, f.CompanyID); err != nil {
			t.Fatalf("remove system admin membership: %v", err)
		}

		service := newService([]string{"SYSTEM_ADMIN"})
		err := service.Deactivate(ctx, f.UserID, f.CompanyID)
		if !errors.Is(err, ErrCompanyHasActiveBranches) {
			t.Fatalf(
				"SYSTEM_ADMIN must obey active branch blocker: %v",
				err,
			)
		}
	})

	t.Run("system admin is global but cannot bypass archive blocker", func(t *testing.T) {
		f := createFixture(t)
		createBranch(t, f, false)

		if _, err := db.Exec(ctx, `
			DELETE FROM company_memberships
			WHERE user_id=$1 AND company_id=$2
		`, f.UserID, f.CompanyID); err != nil {
			t.Fatalf("remove system admin membership: %v", err)
		}

		service := newService([]string{"SYSTEM_ADMIN"})
		err := service.Delete(ctx, f.UserID, f.CompanyID)
		if !errors.Is(err, ErrCompanyHasBranches) {
			t.Fatalf(
				"SYSTEM_ADMIN must obey non-archived branch blocker: %v",
				err,
			)
		}
	})
}
