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

type branchAuthorityUserRoleRepositoryStub struct {
	roles []string
	err   error
}

func (r *branchAuthorityUserRoleRepositoryStub) AssignRole(
	context.Context,
	string,
	string,
) error {
	return nil
}

func (r *branchAuthorityUserRoleRepositoryStub) RemoveRole(
	context.Context,
	string,
	string,
) error {
	return nil
}

func (r *branchAuthorityUserRoleRepositoryStub) UserHasRole(
	context.Context,
	string,
	string,
) (bool, error) {
	return false, nil
}

func (r *branchAuthorityUserRoleRepositoryStub) GetUserRoles(
	context.Context,
	string,
) ([]string, error) {
	return r.roles, r.err
}

var _ repository.UserRoleRepository = (*branchAuthorityUserRoleRepositoryStub)(nil)

type branchAuthorityFixture struct {
	UserID    string
	CompanyID string
	BranchID  string
}

func TestBranchAuthorityAndNullableReads(t *testing.T) {
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

	createFixture := func(t *testing.T) branchAuthorityFixture {
		t.Helper()

		f := branchAuthorityFixture{
			UserID:    uuid.NewString(),
			CompanyID: uuid.NewString(),
			BranchID:  uuid.NewString(),
		}

		if _, err := db.Exec(ctx, `
			INSERT INTO users (
				id, email, password_hash, first_name, last_name
			)
			VALUES ($1, $2, 'test-password-hash', 'Branch', 'Authority')
		`, f.UserID, f.UserID+"@example.test"); err != nil {
			t.Fatalf("create branch authority user: %v", err)
		}

		if _, err := db.Exec(ctx, `
			INSERT INTO companies (
				id, name, legal_name, business_id, email,
				country_code, timezone
			)
			VALUES ($1, $2, $3, $4, $5, 'FI', 'Europe/Helsinki')
		`,
			f.CompanyID,
			"Branch Authority "+f.CompanyID[:8],
			"Branch Authority "+f.CompanyID[:8]+" Oy",
			"BRA-"+f.CompanyID[:8],
			f.CompanyID+"@example.test",
		); err != nil {
			t.Fatalf("create branch authority company: %v", err)
		}

		// Intentionally populate only required branch columns.
		// Optional columns remain SQL NULL so repository reads must
		// normalize them to the Branch model's zero values.
		if _, err := db.Exec(ctx, `
			INSERT INTO branches (
				id, company_id, code, name
			)
			VALUES ($1, $2, $3, $4)
		`,
			f.BranchID,
			f.CompanyID,
			"BR-"+f.BranchID[:8],
			"Branch Authority "+f.BranchID[:8],
		); err != nil {
			t.Fatalf("create branch authority branch: %v", err)
		}

		if _, err := db.Exec(ctx, `
			INSERT INTO company_memberships (user_id, company_id)
			VALUES ($1, $2)
		`, f.UserID, f.CompanyID); err != nil {
			t.Fatalf("create branch authority membership: %v", err)
		}

		t.Cleanup(func() {
			cleanupCtx := context.Background()
			_, _ = db.Exec(cleanupCtx,
				`DELETE FROM fleets WHERE branch_id=$1`, f.BranchID)
			_, _ = db.Exec(cleanupCtx,
				`DELETE FROM company_memberships WHERE user_id=$1`, f.UserID)
			_, _ = db.Exec(cleanupCtx,
				`DELETE FROM branches WHERE company_id=$1`, f.CompanyID)
			_, _ = db.Exec(cleanupCtx,
				`DELETE FROM companies WHERE id=$1`, f.CompanyID)
			_, _ = db.Exec(cleanupCtx,
				`DELETE FROM users WHERE id=$1`, f.UserID)
		})

		return f
	}

	newService := func(roles []string) *Service {
		return NewService(Dependencies{
			DB:                 db,
			Branches:           postgresrepo.NewBranchRepository(db),
			UserRoles:          &branchAuthorityUserRoleRepositoryStub{roles: roles},
			CompanyMemberships: postgresrepo.NewCompanyMembershipRepository(db),
		})
	}

	t.Run("member creates active branch in own company", func(t *testing.T) {
		f := createFixture(t)
		service := newService([]string{"COMPANY_ADMIN"})

		created, err := service.Create(ctx, f.UserID, CreateBranchRequest{
			CompanyID: f.CompanyID,
			Code:      "NEW-" + uuid.NewString()[:8],
			Name:      "Created Branch",
		})
		if err != nil {
			t.Fatalf("create own branch: %v", err)
		}
		if created.CompanyID != f.CompanyID {
			t.Fatalf(
				"expected company %q, got %q",
				f.CompanyID,
				created.CompanyID,
			)
		}
		if !created.IsActive {
			t.Fatal("expected service-owned initial active state")
		}

		var active bool
		if err := db.QueryRow(ctx, `
			SELECT is_active
			FROM branches
			WHERE id=$1
		`, created.ID).Scan(&active); err != nil {
			t.Fatalf("inspect created branch: %v", err)
		}
		if !active {
			t.Fatal("created branch persisted inactive")
		}
	})

	t.Run("nonmember cannot create branch in another company", func(t *testing.T) {
		owner := createFixture(t)
		other := createFixture(t)
		service := newService([]string{"COMPANY_ADMIN"})

		code := "DENIED-" + uuid.NewString()[:8]
		_, err := service.Create(ctx, other.UserID, CreateBranchRequest{
			CompanyID: owner.CompanyID,
			Code:      code,
			Name:      "Denied Branch",
		})
		if !errors.Is(err, ErrBranchCreationAccessDenied) {
			t.Fatalf(
				"expected ErrBranchCreationAccessDenied, got %v",
				err,
			)
		}

		var count int
		if err := db.QueryRow(ctx, `
			SELECT COUNT(*)
			FROM branches
			WHERE company_id=$1 AND code=$2
		`, owner.CompanyID, code).Scan(&count); err != nil {
			t.Fatalf("count denied branch creation: %v", err)
		}
		if count != 0 {
			t.Fatalf("unauthorized creation persisted %d branch(es)", count)
		}
	})

	t.Run("member reads own branch and nullable fields normalize", func(t *testing.T) {
		f := createFixture(t)
		service := newService([]string{"COMPANY_ADMIN"})

		got, err := service.GetByID(ctx, f.UserID, f.BranchID)
		if err != nil {
			t.Fatalf("read own branch: %v", err)
		}

		if got.ID != f.BranchID || got.CompanyID != f.CompanyID {
			t.Fatalf("unexpected branch: %#v", got)
		}

		if got.Email != "" ||
			got.Phone != "" ||
			got.AddressLine1 != "" ||
			got.AddressLine2 != "" ||
			got.City != "" ||
			got.State != "" ||
			got.PostalCode != "" ||
			got.Latitude != 0 ||
			got.Longitude != 0 {
			t.Fatalf("nullable fields were not normalized: %#v", got)
		}
	})

	t.Run("cross-tenant read is indistinguishable from nonexistent", func(t *testing.T) {
		owner := createFixture(t)
		other := createFixture(t)
		service := newService([]string{"COMPANY_ADMIN"})

		_, crossTenantErr := service.GetByID(
			ctx,
			other.UserID,
			owner.BranchID,
		)
		if !errors.Is(crossTenantErr, ErrBranchNotFound) {
			t.Fatalf(
				"expected ErrBranchNotFound cross-tenant, got %v",
				crossTenantErr,
			)
		}

		_, missingErr := service.GetByID(
			ctx,
			other.UserID,
			uuid.NewString(),
		)
		if !errors.Is(missingErr, ErrBranchNotFound) {
			t.Fatalf(
				"expected ErrBranchNotFound nonexistent, got %v",
				missingErr,
			)
		}
	})

	t.Run("member list is restricted to explicit memberships", func(t *testing.T) {
		own := createFixture(t)
		other := createFixture(t)
		service := newService([]string{"COMPANY_ADMIN"})

		branches, err := service.List(ctx, own.UserID)
		if err != nil {
			t.Fatalf("list member branches: %v", err)
		}

		foundOwn := false
		for _, branch := range branches {
			if branch.CompanyID == other.CompanyID {
				t.Fatalf(
					"cross-tenant branch leaked through list: %#v",
					branch,
				)
			}
			if branch.ID == own.BranchID {
				foundOwn = true
				if branch.Email != "" ||
					branch.Latitude != 0 ||
					branch.Longitude != 0 {
					t.Fatalf(
						"nullable list fields were not normalized: %#v",
						branch,
					)
				}
			}
		}

		if !foundOwn {
			t.Fatalf("own branch %q missing from scoped list", own.BranchID)
		}
	})

	t.Run("system admin has global read without membership", func(t *testing.T) {
		f := createFixture(t)

		if _, err := db.Exec(ctx, `
			DELETE FROM company_memberships
			WHERE user_id=$1 AND company_id=$2
		`, f.UserID, f.CompanyID); err != nil {
			t.Fatalf("remove system admin membership: %v", err)
		}

		service := newService([]string{"SYSTEM_ADMIN"})

		got, err := service.GetByID(ctx, f.UserID, f.BranchID)
		if err != nil {
			t.Fatalf("SYSTEM_ADMIN global GetByID: %v", err)
		}
		if got.ID != f.BranchID {
			t.Fatalf("unexpected SYSTEM_ADMIN branch: %#v", got)
		}

		branches, err := service.List(ctx, f.UserID)
		if err != nil {
			t.Fatalf("SYSTEM_ADMIN global List: %v", err)
		}

		found := false
		for _, branch := range branches {
			if branch.ID == f.BranchID {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf(
				"SYSTEM_ADMIN global list omitted branch %q",
				f.BranchID,
			)
		}
	})

	t.Run("system admin creates without company membership", func(t *testing.T) {
		f := createFixture(t)

		if _, err := db.Exec(ctx, `
			DELETE FROM company_memberships
			WHERE user_id=$1 AND company_id=$2
		`, f.UserID, f.CompanyID); err != nil {
			t.Fatalf("remove system admin membership: %v", err)
		}

		service := newService([]string{"SYSTEM_ADMIN"})
		created, err := service.Create(ctx, f.UserID, CreateBranchRequest{
			CompanyID: f.CompanyID,
			Code:      "ADMIN-" + uuid.NewString()[:8],
			Name:      "System Admin Branch",
		})
		if err != nil {
			t.Fatalf("SYSTEM_ADMIN global branch creation: %v", err)
		}
		if created.CompanyID != f.CompanyID || !created.IsActive {
			t.Fatalf("unexpected SYSTEM_ADMIN creation: %#v", created)
		}
	})
}
