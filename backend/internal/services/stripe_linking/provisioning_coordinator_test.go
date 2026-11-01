package stripelinking

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/JCKFinland/connect/backend/internal/models"
	"github.com/jackc/pgx/v5/pgxpool"
)

type coordinatorPermissionStub struct {
	allowed bool
	err     error
	calls   int
}

func (s *coordinatorPermissionStub) HasPermission(
	_ context.Context, actor, permission string,
) (bool, error) {
	s.calls++
	if actor != testActor || permission != "drivers.manage" {
		return false, errors.New("unexpected permission arguments")
	}
	return s.allowed, s.err
}

type coordinatorMembershipStub struct {
	allowed bool
	err     error
	calls   int
}

func (s *coordinatorMembershipStub) Exists(
	_ context.Context, actor, company string,
) (bool, error) {
	s.calls++
	if actor != testActor || company != testCompany {
		return false, errors.New("unexpected membership arguments")
	}
	return s.allowed, s.err
}

type coordinatorDriverStub struct {
	driver *models.Driver
	err    error
	calls  int
}

func (s *coordinatorDriverStub) GetByIDForCompanyMember(
	_ context.Context, actor, driverID string,
) (*models.Driver, error) {
	s.calls++
	if actor != testActor || driverID != testDriverID {
		return nil, errors.New("unexpected driver arguments")
	}
	return s.driver, s.err
}

type coordinatorCreatorStub struct {
	calls int
}

func (s *coordinatorCreatorStub) CreateProvisioningAccount(
	context.Context, string, string, string, string,
) (string, error) {
	s.calls++
	return testAccount, nil
}

type coordinatorFixture struct {
	permissions *coordinatorPermissionStub
	memberships *coordinatorMembershipStub
	drivers     *coordinatorDriverStub
	creator     *coordinatorCreatorStub
}

func newCoordinatorFixture() *coordinatorFixture {
	return &coordinatorFixture{
		permissions: &coordinatorPermissionStub{allowed: true},
		memberships: &coordinatorMembershipStub{allowed: true},
		drivers: &coordinatorDriverStub{
			driver: &models.Driver{
				UserID:    testDriverUser,
				CompanyID: testCompany,
				IsActive:  true,
			},
		},
		creator: &coordinatorCreatorStub{},
	}
}

func (f *coordinatorFixture) deps() ProvisioningCoordinatorDeps {
	return ProvisioningCoordinatorDeps{
		Permissions: f.permissions,
		Memberships: f.memberships,
		Drivers:     f.drivers,
		Accounts:    f.creator,
		// Deliberately nil: these tests verify fail-closed
		// behavior before any database or provider operation.
		DB: nil,
	}
}

func TestProvisioningCoordinatorRejectsInvalidInputs(t *testing.T) {
	canceled, cancel := context.WithCancel(context.Background())
	cancel()

	cases := []struct {
		name   string
		ctx    context.Context
		actor  string
		driver string
	}{
		{"nil_context", nil, testActor, testDriverID},
		{"canceled_context", canceled, testActor, testDriverID},
		{"missing_actor", context.Background(), "", testDriverID},
		{"invalid_actor", context.Background(), "invalid", testDriverID},
		{"missing_driver", context.Background(), testActor, ""},
		{"invalid_driver", context.Background(), testActor, "invalid"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newCoordinatorFixture()
			coordinator := NewProvisioningCoordinator(f.deps())

			account, err := coordinator.ProvisionAccount(
				tc.ctx, tc.actor, tc.driver,
			)
			if account != "" ||
				!errors.Is(err, ErrStripeProvisioningDenied) {
				t.Fatalf("expected denial, got %q, %v", account, err)
			}
			if f.permissions.calls != 0 ||
				f.memberships.calls != 0 ||
				f.drivers.calls != 0 ||
				f.creator.calls != 0 {
				t.Fatal("invalid input reached dependency")
			}
		})
	}
}

func TestProvisioningCoordinatorRejectsMissingDependencies(t *testing.T) {
	cases := []struct {
		name   string
		change func(*ProvisioningCoordinatorDeps)
	}{
		{"permissions", func(d *ProvisioningCoordinatorDeps) {
			d.Permissions = nil
		}},
		{"memberships", func(d *ProvisioningCoordinatorDeps) {
			d.Memberships = nil
		}},
		{"drivers", func(d *ProvisioningCoordinatorDeps) {
			d.Drivers = nil
		}},
		{"database", func(d *ProvisioningCoordinatorDeps) {
			d.DB = nil
		}},
		{"creator", func(d *ProvisioningCoordinatorDeps) {
			d.Accounts = nil
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newCoordinatorFixture()
			deps := f.deps()

			// Non-nil pointer is sufficient here because
			// validation must stop before database use.
			deps.DB = new(pgxpool.Pool)
			tc.change(&deps)

			account, err := NewProvisioningCoordinator(deps).
				ProvisionAccount(
					context.Background(), testActor, testDriverID,
				)

			if account != "" ||
				!errors.Is(err, ErrStripeProvisioningDenied) {
				t.Fatalf("expected denial, got %q, %v", account, err)
			}
			if f.creator.calls != 0 {
				t.Fatal("missing dependency reached creator")
			}
		})
	}

	var nilCoordinator *ProvisioningCoordinator
	account, err := nilCoordinator.ProvisionAccount(
		context.Background(), testActor, testDriverID,
	)
	if account != "" ||
		!errors.Is(err, ErrStripeProvisioningDenied) {
		t.Fatalf("nil coordinator accepted: %q, %v", account, err)
	}
}

func TestProvisioningCoordinatorRejectsInvalidAccountID(t *testing.T) {
	valid := []string{
		"acct_12345678",
		"acct_1234567890123456",
	}
	invalid := []string{
		"",
		"acct_123",
		"acct_1234567",
		"acct_12345678-",
		"acct_12345678 ",
		"acct_12345678/",
		"not_an_account",
	}

	for _, value := range valid {
		if !validProvisioningAccountID(value) {
			t.Fatalf("valid account rejected: %q", value)
		}
	}
	for _, value := range invalid {
		if validProvisioningAccountID(value) {
			t.Fatalf("invalid account accepted: %q", value)
		}
	}
}

func TestProvisioningCoordinatorAuthorizationContracts(t *testing.T) {
	// The real authorization behavior with a database is
	// exercised by the following integration work order.
	// This test verifies the interfaces remain compatible.
	var _ PermissionChecker = (*coordinatorPermissionStub)(nil)
	var _ MembershipChecker = (*coordinatorMembershipStub)(nil)
	var _ DriverResolver = (*coordinatorDriverStub)(nil)
	var _ ProvisioningAccountCreator = (*coordinatorCreatorStub)(nil)

	_ = time.Second
}
