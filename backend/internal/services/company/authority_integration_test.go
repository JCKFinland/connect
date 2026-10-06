package company

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

type companyAuthorityUserRoleRepositoryStub struct {
	roles []string
	err   error
}

func (r *companyAuthorityUserRoleRepositoryStub) AssignRole(
	context.Context,
	string,
	string,
) error {
	return nil
}

func (r *companyAuthorityUserRoleRepositoryStub) RemoveRole(
	context.Context,
	string,
	string,
) error {
	return nil
}

func (r *companyAuthorityUserRoleRepositoryStub) UserHasRole(
	context.Context,
	string,
	string,
) (bool, error) {
	return false, nil
}

func (r *companyAuthorityUserRoleRepositoryStub) GetUserRoles(
	context.Context,
	string,
) ([]string, error) {
	return r.roles, r.err
}

var _ repository.UserRoleRepository = (*companyAuthorityUserRoleRepositoryStub)(nil)

type companyAuthorityFixture struct {
	UserID    string
	CompanyID string
}

func TestCompanyAuthorityAndNullableReads(t *testing.T) {
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

	createFixture := func(t *testing.T) companyAuthorityFixture {
		t.Helper()

		f := companyAuthorityFixture{
			UserID:    uuid.NewString(),
			CompanyID: uuid.NewString(),
		}

		if _, err := db.Exec(ctx, `
			INSERT INTO users (
				id, email, password_hash, first_name, last_name
			)
			VALUES ($1, $2, 'test-password-hash', 'Company', 'Authority')
		`, f.UserID, f.UserID+"@example.test"); err != nil {
			t.Fatalf("create company authority user: %v", err)
		}

		// Intentionally populate only required/non-null company columns.
		// Optional nullable columns remain SQL NULL so repository reads must
		// normalize them to the Company model's zero values.
		if _, err := db.Exec(ctx, `
			INSERT INTO companies (
				id, name, country_code, timezone
			)
			VALUES ($1, $2, 'FI', 'Europe/Helsinki')
		`,
			f.CompanyID,
			"Company Authority "+f.CompanyID[:8],
		); err != nil {
			t.Fatalf("create company authority company: %v", err)
		}

		if _, err := db.Exec(ctx, `
			INSERT INTO company_memberships (user_id, company_id)
			VALUES ($1, $2)
		`, f.UserID, f.CompanyID); err != nil {
			t.Fatalf("create company authority membership: %v", err)
		}

		t.Cleanup(func() {
			cleanupCtx := context.Background()
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

	newService := func(roles []string) *Service {
		return NewService(Dependencies{
			Config:             cfg,
			DB:                 db,
			Companies:          postgresrepo.NewCompanyRepository(db),
			UserRoles:          &companyAuthorityUserRoleRepositoryStub{roles: roles},
			CompanyMemberships: postgresrepo.NewCompanyMembershipRepository(db),
		})
	}

	t.Run("system admin creates active company", func(t *testing.T) {
		f := createFixture(t)
		service := newService([]string{"SYSTEM_ADMIN"})

		created, err := service.Create(ctx, f.UserID, CreateCompanyRequest{
			Name:        "Created Company " + uuid.NewString()[:8],
			LegalName:   "Created Company Oy",
			BusinessID:  "CREATE-" + uuid.NewString()[:8],
			Email:       uuid.NewString() + "@example.test",
			CountryCode: "FI",
			Timezone:    "Europe/Helsinki",
		})
		if err != nil {
			t.Fatalf("SYSTEM_ADMIN company creation: %v", err)
		}

		t.Cleanup(func() {
			_, _ = db.Exec(
				context.Background(),
				`DELETE FROM companies WHERE id=$1`,
				created.ID,
			)
		})

		if !created.IsActive {
			t.Fatal("expected service-owned initial active state")
		}

		var active bool
		if err := db.QueryRow(ctx, `
			SELECT is_active
			FROM companies
			WHERE id=$1
		`, created.ID).Scan(&active); err != nil {
			t.Fatalf("inspect created company: %v", err)
		}
		if !active {
			t.Fatal("created company persisted inactive")
		}
	})

	t.Run("non-system user cannot create company", func(t *testing.T) {
		f := createFixture(t)
		service := newService([]string{"COMPANY_ADMIN"})

		name := "Denied Company " + uuid.NewString()[:8]
		_, err := service.Create(ctx, f.UserID, CreateCompanyRequest{
			Name:        name,
			CountryCode: "FI",
			Timezone:    "Europe/Helsinki",
		})
		if !errors.Is(err, ErrCompanyCreationAccessDenied) {
			t.Fatalf(
				"expected ErrCompanyCreationAccessDenied, got %v",
				err,
			)
		}

		var count int
		if err := db.QueryRow(ctx, `
			SELECT COUNT(*)
			FROM companies
			WHERE name=$1
		`, name).Scan(&count); err != nil {
			t.Fatalf("count denied company creation: %v", err)
		}
		if count != 0 {
			t.Fatalf("unauthorized creation persisted %d company(s)", count)
		}
	})

	t.Run("member reads own company and nullable fields normalize", func(t *testing.T) {
		f := createFixture(t)
		service := newService([]string{"COMPANY_ADMIN"})

		got, err := service.GetByID(ctx, f.UserID, f.CompanyID)
		if err != nil {
			t.Fatalf("read own company: %v", err)
		}

		if got.ID != f.CompanyID {
			t.Fatalf("unexpected company: %#v", got)
		}

		if got.LegalName != "" ||
			got.BusinessID != "" ||
			got.Email != "" ||
			got.Phone != "" ||
			got.Website != "" ||
			got.AddressLine1 != "" ||
			got.AddressLine2 != "" ||
			got.City != "" ||
			got.State != "" ||
			got.PostalCode != "" ||
			got.LogoURL != "" {
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
			owner.CompanyID,
		)
		if !errors.Is(crossTenantErr, ErrCompanyNotFound) {
			t.Fatalf(
				"expected ErrCompanyNotFound cross-tenant, got %v",
				crossTenantErr,
			)
		}

		_, missingErr := service.GetByID(
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

	t.Run("member list is restricted to explicit memberships", func(t *testing.T) {
		own := createFixture(t)
		other := createFixture(t)
		service := newService([]string{"COMPANY_ADMIN"})

		companies, err := service.List(ctx, own.UserID)
		if err != nil {
			t.Fatalf("list member companies: %v", err)
		}

		foundOwn := false
		for _, company := range companies {
			if company.ID == other.CompanyID {
				t.Fatalf(
					"cross-tenant company leaked through list: %#v",
					company,
				)
			}

			if company.ID == own.CompanyID {
				foundOwn = true
				if company.LegalName != "" ||
					company.Email != "" ||
					company.Phone != "" ||
					company.Website != "" ||
					company.LogoURL != "" {
					t.Fatalf(
						"nullable list fields were not normalized: %#v",
						company,
					)
				}
			}
		}

		if !foundOwn {
			t.Fatalf(
				"own company %q missing from scoped list",
				own.CompanyID,
			)
		}
	})

	t.Run("member updates own details without changing lifecycle state", func(t *testing.T) {
		f := createFixture(t)

		if _, err := db.Exec(ctx, `
			UPDATE companies
			SET is_active=FALSE
			WHERE id=$1
		`, f.CompanyID); err != nil {
			t.Fatalf("deactivate fixture company: %v", err)
		}

		service := newService([]string{"COMPANY_ADMIN"})
		err := service.Update(ctx, f.UserID, f.CompanyID, UpdateCompanyRequest{
			Name:         "Updated Company",
			LegalName:    "Updated Company Oy",
			BusinessID:   "UPDATED-" + f.CompanyID[:8],
			Email:        "updated-" + f.CompanyID[:8] + "@example.test",
			Phone:        "+358401234567",
			Website:      "https://example.test",
			CountryCode:  "FI",
			Timezone:     "Europe/Helsinki",
			AddressLine1: "1 Testikatu",
			AddressLine2: "Suite 2",
			City:         "Helsinki",
			State:        "Uusimaa",
			PostalCode:   "00100",
			LogoURL:      "https://example.test/logo.png",
		})
		if err != nil {
			t.Fatalf("update own company: %v", err)
		}

		var name string
		var active bool
		if err := db.QueryRow(ctx, `
			SELECT name, is_active
			FROM companies
			WHERE id=$1
		`, f.CompanyID).Scan(&name, &active); err != nil {
			t.Fatalf("inspect updated company: %v", err)
		}

		if name != "Updated Company" {
			t.Fatalf("expected updated name, got %q", name)
		}
		if active {
			t.Fatal("ordinary company update changed lifecycle state")
		}
	})

	t.Run("cross-tenant update is indistinguishable from nonexistent", func(t *testing.T) {
		owner := createFixture(t)
		other := createFixture(t)
		service := newService([]string{"COMPANY_ADMIN"})

		req := UpdateCompanyRequest{
			Name:        "Denied Update",
			CountryCode: "FI",
			Timezone:    "Europe/Helsinki",
		}

		crossTenantErr := service.Update(
			ctx,
			other.UserID,
			owner.CompanyID,
			req,
		)
		if !errors.Is(crossTenantErr, ErrCompanyNotFound) {
			t.Fatalf(
				"expected ErrCompanyNotFound cross-tenant, got %v",
				crossTenantErr,
			)
		}

		missingErr := service.Update(
			ctx,
			other.UserID,
			uuid.NewString(),
			req,
		)
		if !errors.Is(missingErr, ErrCompanyNotFound) {
			t.Fatalf(
				"expected ErrCompanyNotFound nonexistent, got %v",
				missingErr,
			)
		}
	})

	t.Run("system admin has global read update and list without membership", func(t *testing.T) {
		f := createFixture(t)

		if _, err := db.Exec(ctx, `
			DELETE FROM company_memberships
			WHERE user_id=$1 AND company_id=$2
		`, f.UserID, f.CompanyID); err != nil {
			t.Fatalf("remove system admin membership: %v", err)
		}

		service := newService([]string{"SYSTEM_ADMIN"})

		got, err := service.GetByID(ctx, f.UserID, f.CompanyID)
		if err != nil {
			t.Fatalf("SYSTEM_ADMIN global GetByID: %v", err)
		}
		if got.ID != f.CompanyID {
			t.Fatalf("unexpected SYSTEM_ADMIN company: %#v", got)
		}

		if err := service.Update(
			ctx,
			f.UserID,
			f.CompanyID,
			UpdateCompanyRequest{
				Name:        "System Admin Updated",
				CountryCode: "FI",
				Timezone:    "Europe/Helsinki",
			},
		); err != nil {
			t.Fatalf("SYSTEM_ADMIN global Update: %v", err)
		}

		companies, err := service.List(ctx, f.UserID)
		if err != nil {
			t.Fatalf("SYSTEM_ADMIN global List: %v", err)
		}

		found := false
		for _, company := range companies {
			if company.ID == f.CompanyID {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf(
				"SYSTEM_ADMIN global list omitted company %q",
				f.CompanyID,
			)
		}
	})
}
