package driver

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/JCKFinland/connect/backend/internal/config"
	"github.com/JCKFinland/connect/backend/internal/database"
	"github.com/JCKFinland/connect/backend/internal/models"
	postgresrepo "github.com/JCKFinland/connect/backend/internal/repository/postgres"
	"github.com/JCKFinland/connect/backend/internal/testutil"
	"github.com/google/uuid"
)

type readAuthorityUserRoleRepositoryStub struct {
	roles []string
}

func (r *readAuthorityUserRoleRepositoryStub) AssignRole(
	context.Context,
	string,
	string,
) error {
	return nil
}

func (r *readAuthorityUserRoleRepositoryStub) RemoveRole(
	context.Context,
	string,
	string,
) error {
	return nil
}

func (r *readAuthorityUserRoleRepositoryStub) UserHasRole(
	context.Context,
	string,
	string,
) (bool, error) {
	return false, nil
}

func (r *readAuthorityUserRoleRepositoryStub) GetUserRoles(
	context.Context,
	string,
) ([]string, error) {
	return r.roles, nil
}

func TestDriverReadAuthority(t *testing.T) {
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

	createFixture := func(t *testing.T) *testutil.DriverFixture {
		t.Helper()

		fixture, cleanup, err := testutil.CreateDriverFixture(ctx, db)
		if err != nil {
			t.Fatalf("create driver fixture: %v", err)
		}

		t.Cleanup(func() {
			if cleanupErr := cleanup(context.Background()); cleanupErr != nil {
				t.Logf("cleanup driver fixture: %v", cleanupErr)
			}
		})

		return fixture
	}

	addMembership := func(t *testing.T, userID, companyID string) {
		t.Helper()

		if _, err := db.Exec(
			ctx,
			`
				INSERT INTO company_memberships (
					user_id,
					company_id
				)
				VALUES ($1, $2)
				ON CONFLICT (user_id, company_id) DO NOTHING
			`,
			userID,
			companyID,
		); err != nil {
			t.Fatalf("create company membership: %v", err)
		}

		t.Cleanup(func() {
			_, _ = db.Exec(
				context.Background(),
				`
					DELETE FROM company_memberships
					WHERE user_id=$1 AND company_id=$2
				`,
				userID,
				companyID,
			)
		})
	}

	newService := func(roles []string) *Service {
		return NewService(Dependencies{
			DB:                 db,
			Drivers:            postgresrepo.NewDriverRepository(db),
			UserRoles:          &readAuthorityUserRoleRepositoryStub{roles: roles},
			CompanyMemberships: postgresrepo.NewCompanyMembershipRepository(db),
		})
	}

	containsDriver := func(drivers []models.Driver, driverID string) bool {
		for _, driver := range drivers {
			if driver.ID == driverID {
				return true
			}
		}
		return false
	}

	t.Run("company member reads own company driver", func(t *testing.T) {
		target := createFixture(t)
		caller := createFixture(t)
		addMembership(t, caller.UserID, target.CompanyID)

		got, err := newService([]string{"COMPANY_ADMIN"}).GetByID(
			ctx,
			caller.UserID,
			target.DriverID,
		)
		if err != nil {
			t.Fatalf("read own-company driver: %v", err)
		}
		if got.ID != target.DriverID || got.CompanyID != target.CompanyID {
			t.Fatalf("unexpected driver: %#v", got)
		}
	})

	t.Run("cross tenant read is indistinguishable from nonexistent", func(t *testing.T) {
		target := createFixture(t)
		caller := createFixture(t)
		addMembership(t, caller.UserID, caller.CompanyID)

		service := newService([]string{"COMPANY_ADMIN"})

		_, crossTenantErr := service.GetByID(
			ctx,
			caller.UserID,
			target.DriverID,
		)
		if !errors.Is(crossTenantErr, ErrDriverNotFound) {
			t.Fatalf(
				"expected ErrDriverNotFound cross-tenant, got %v",
				crossTenantErr,
			)
		}

		_, missingErr := service.GetByID(
			ctx,
			caller.UserID,
			uuid.NewString(),
		)
		if !errors.Is(missingErr, ErrDriverNotFound) {
			t.Fatalf(
				"expected ErrDriverNotFound nonexistent, got %v",
				missingErr,
			)
		}
	})

	t.Run("member list is restricted to explicit memberships", func(t *testing.T) {
		own := createFixture(t)
		other := createFixture(t)
		caller := createFixture(t)

		addMembership(t, caller.UserID, own.CompanyID)

		drivers, err := newService([]string{"COMPANY_ADMIN"}).List(
			ctx,
			caller.UserID,
		)
		if err != nil {
			t.Fatalf("list member drivers: %v", err)
		}

		if !containsDriver(drivers, own.DriverID) {
			t.Fatalf(
				"own-company driver %q missing from scoped list",
				own.DriverID,
			)
		}
		if containsDriver(drivers, other.DriverID) {
			t.Fatalf(
				"cross-tenant driver %q leaked through scoped list",
				other.DriverID,
			)
		}
	})

	t.Run("member list includes all explicitly joined companies", func(t *testing.T) {
		first := createFixture(t)
		second := createFixture(t)
		other := createFixture(t)
		caller := createFixture(t)

		addMembership(t, caller.UserID, first.CompanyID)
		addMembership(t, caller.UserID, second.CompanyID)

		drivers, err := newService([]string{"COMPANY_ADMIN"}).List(
			ctx,
			caller.UserID,
		)
		if err != nil {
			t.Fatalf("list multi-company member drivers: %v", err)
		}

		if !containsDriver(drivers, first.DriverID) {
			t.Fatalf("first member-company driver %q missing", first.DriverID)
		}
		if !containsDriver(drivers, second.DriverID) {
			t.Fatalf("second member-company driver %q missing", second.DriverID)
		}
		if containsDriver(drivers, other.DriverID) {
			t.Fatalf(
				"nonmember-company driver %q leaked through scoped list",
				other.DriverID,
			)
		}
	})

	t.Run("system admin has global read without membership", func(t *testing.T) {
		target := createFixture(t)
		caller := createFixture(t)

		service := newService([]string{"SYSTEM_ADMIN"})

		got, err := service.GetByID(
			ctx,
			caller.UserID,
			target.DriverID,
		)
		if err != nil {
			t.Fatalf("SYSTEM_ADMIN global GetByID: %v", err)
		}
		if got.ID != target.DriverID {
			t.Fatalf("unexpected SYSTEM_ADMIN driver: %#v", got)
		}

		drivers, err := service.List(ctx, caller.UserID)
		if err != nil {
			t.Fatalf("SYSTEM_ADMIN global List: %v", err)
		}
		if !containsDriver(drivers, target.DriverID) {
			t.Fatalf(
				"SYSTEM_ADMIN global list omitted driver %q",
				target.DriverID,
			)
		}
	})

	t.Run("system admin nonexistent driver is not found", func(t *testing.T) {
		caller := createFixture(t)

		_, err := newService([]string{"SYSTEM_ADMIN"}).GetByID(
			ctx,
			caller.UserID,
			uuid.NewString(),
		)
		if !errors.Is(err, ErrDriverNotFound) {
			t.Fatalf(
				"expected ErrDriverNotFound for SYSTEM_ADMIN nonexistent driver, got %v",
				err,
			)
		}
	})

	t.Run("archived driver is hidden from get and list", func(t *testing.T) {
		target := createFixture(t)
		caller := createFixture(t)
		addMembership(t, caller.UserID, target.CompanyID)

		if _, err := db.Exec(
			ctx,
			`
				UPDATE drivers
				SET deleted_at=$2
				WHERE id=$1
			`,
			target.DriverID,
			time.Now().UTC(),
		); err != nil {
			t.Fatalf("archive driver fixture: %v", err)
		}

		service := newService([]string{"COMPANY_ADMIN"})

		_, err := service.GetByID(ctx, caller.UserID, target.DriverID)
		if !errors.Is(err, ErrDriverNotFound) {
			t.Fatalf(
				"expected archived driver GetByID to return ErrDriverNotFound, got %v",
				err,
			)
		}

		drivers, err := service.List(ctx, caller.UserID)
		if err != nil {
			t.Fatalf("list after driver archive: %v", err)
		}
		if containsDriver(drivers, target.DriverID) {
			t.Fatalf(
				"archived driver %q leaked through scoped list",
				target.DriverID,
			)
		}
	})
}
