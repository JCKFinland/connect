package stripelinking

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/JCKFinland/connect/backend/internal/models"
	"github.com/JCKFinland/connect/backend/internal/payouts"
)

const (
	testActor      = "11111111-1111-4111-8111-111111111111"
	testDriverID   = "22222222-2222-4222-8222-222222222222"
	testDriverUser = "33333333-3333-4333-8333-333333333333"
	testCompany    = "44444444-4444-4444-8444-444444444444"
	testAccount    = "acct_1234567890123456"
	testSession    = "55555555-5555-4555-8555-555555555555"
)

type permissionStub struct {
	allowed bool
	err     error
	calls   int
}

func (s *permissionStub) HasPermission(
	_ context.Context, actor, permission string,
) (bool, error) {
	s.calls++
	if actor != testActor || permission != "drivers.manage" {
		return false, errors.New("unexpected permission arguments")
	}
	return s.allowed, s.err
}

type membershipStub struct {
	allowed bool
	err     error
	calls   int
}

func (s *membershipStub) Exists(
	_ context.Context, actor, company string,
) (bool, error) {
	s.calls++
	if actor != testActor || company != testCompany {
		return false, errors.New("unexpected membership arguments")
	}
	return s.allowed, s.err
}

type driverStub struct {
	driver *models.Driver
	err    error
	calls  int
}

func (s *driverStub) GetByIDForCompanyMember(
	_ context.Context, actor, driverID string,
) (*models.Driver, error) {
	s.calls++
	if actor != testActor || driverID != testDriverID {
		return nil, errors.New("unexpected driver lookup arguments")
	}
	return s.driver, s.err
}

type accountStub struct {
	account string
	err     error
	calls   int
}

func (s *accountStub) ResolveAccount(
	_ context.Context, actor, driverUser, company string,
) (string, error) {
	s.calls++
	if actor != testActor ||
		driverUser != testDriverUser ||
		company != testCompany {
		return "", errors.New("unexpected account arguments")
	}
	return s.account, s.err
}

type sessionStub struct {
	session string
	err     error
	calls   int
	request payouts.StripeConnectOwnershipRequest
}

func (s *sessionStub) Create(
	_ context.Context,
	request payouts.StripeConnectOwnershipRequest,
) (string, error) {
	s.calls++
	s.request = request
	return s.session, s.err
}

type fixture struct {
	permissions *permissionStub
	memberships *membershipStub
	drivers     *driverStub
	accounts    *accountStub
	sessions    *sessionStub
}

func newFixture() *fixture {
	return &fixture{
		permissions: &permissionStub{allowed: true},
		memberships: &membershipStub{allowed: true},
		drivers: &driverStub{
			driver: &models.Driver{
				UserID:    testDriverUser,
				CompanyID: testCompany,
				IsActive:  true,
			},
		},
		accounts: &accountStub{account: testAccount},
		sessions: &sessionStub{session: testSession},
	}
}

func (f *fixture) service() *Service {
	return NewService(Dependencies{
		Permissions: f.permissions,
		Memberships: f.memberships,
		Drivers:     f.drivers,
		Accounts:    f.accounts,
		Sessions:    f.sessions,
	})
}

func TestStripeLinkingBeginAuthorizedIdentity(t *testing.T) {
	f := newFixture()

	session, err := f.service().Begin(
		context.Background(), testActor, testDriverID,
	)
	if err != nil || session != testSession {
		t.Fatalf("authorized session: %q, %v", session, err)
	}

	if f.permissions.calls != 1 ||
		f.drivers.calls != 1 ||
		f.memberships.calls != 1 ||
		f.accounts.calls != 1 ||
		f.sessions.calls != 1 {
		t.Fatalf("unexpected dependency calls: %+v", f)
	}

	expected := payouts.StripeConnectOwnershipRequest{
		ActorUserID:  testActor,
		DriverUserID: testDriverUser,
		CompanyID:    testCompany,
		AccountID:    testAccount,
	}

	if f.sessions.request != expected {
		t.Fatalf(
			"incorrect authoritative identity: got %+v, want %+v",
			f.sessions.request, expected,
		)
	}
}

func TestStripeLinkingBeginFailClosed(t *testing.T) {
	cases := []struct {
		name   string
		change func(*fixture)
	}{
		{"permission_denied", func(f *fixture) {
			f.permissions.allowed = false
		}},
		{"permission_error", func(f *fixture) {
			f.permissions.err = errors.New("permission unavailable")
		}},
		{"driver_not_found", func(f *fixture) {
			f.drivers.driver = nil
		}},
		{"driver_lookup_error", func(f *fixture) {
			f.drivers.err = errors.New("driver lookup unavailable")
		}},
		{"inactive_driver", func(f *fixture) {
			f.drivers.driver.IsActive = false
		}},
		{"deleted_driver", func(f *fixture) {
			now := time.Now()
			f.drivers.driver.DeletedAt = &now
		}},
		{"missing_driver_user", func(f *fixture) {
			f.drivers.driver.UserID = ""
		}},
		{"missing_driver_company", func(f *fixture) {
			f.drivers.driver.CompanyID = ""
		}},
		{"membership_denied", func(f *fixture) {
			f.memberships.allowed = false
		}},
		{"membership_error", func(f *fixture) {
			f.memberships.err = errors.New("membership unavailable")
		}},
		{"account_provider_error", func(f *fixture) {
			f.accounts.err = errors.New("provider unavailable")
		}},
		{"invalid_stripe_account", func(f *fixture) {
			f.accounts.account = "not_a_stripe_account"
		}},
		{"empty_stripe_account", func(f *fixture) {
			f.accounts.account = ""
		}},
		{"session_create_error", func(f *fixture) {
			f.sessions.err = errors.New("database unavailable")
		}},
		{"empty_session", func(f *fixture) {
			f.sessions.session = ""
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture()
			tc.change(f)

			session, err := f.service().Begin(
				context.Background(), testActor, testDriverID,
			)

			if session != "" ||
				!errors.Is(err, payouts.ErrStripeConnectOwnershipNotVerified) {
				t.Fatalf("expected fail-closed, got %q, %v", session, err)
			}

			if tc.name != "session_create_error" &&
				tc.name != "empty_session" &&
				f.sessions.calls != 0 {
				t.Fatalf("unauthorized session creation attempted")
			}
		})
	}
}

func TestStripeLinkingBeginRejectsMissingDependencies(t *testing.T) {
	f := newFixture()
	base := Dependencies{
		Permissions: f.permissions,
		Memberships: f.memberships,
		Drivers:     f.drivers,
		Accounts:    f.accounts,
		Sessions:    f.sessions,
	}

	cases := []struct {
		name   string
		change func(*Dependencies)
	}{
		{"permissions", func(d *Dependencies) { d.Permissions = nil }},
		{"memberships", func(d *Dependencies) { d.Memberships = nil }},
		{"drivers", func(d *Dependencies) { d.Drivers = nil }},
		{"accounts", func(d *Dependencies) { d.Accounts = nil }},
		{"sessions", func(d *Dependencies) { d.Sessions = nil }},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			deps := base
			tc.change(&deps)

			session, err := NewService(deps).Begin(
				context.Background(), testActor, testDriverID,
			)
			if session != "" ||
				!errors.Is(err, payouts.ErrStripeConnectOwnershipNotVerified) {
				t.Fatalf("missing dependency accepted: %q, %v", session, err)
			}
		})
	}

	if f.sessions.calls != 0 {
		t.Fatal("missing dependencies caused session creation")
	}
}

func TestStripeLinkingBeginRejectsInvalidContextAndIdentity(t *testing.T) {
	cases := []struct {
		name   string
		ctx    context.Context
		actor  string
		driver string
	}{
		{"nil_context", nil, testActor, testDriverID},
		{"missing_actor", context.Background(), "", testDriverID},
		{"missing_driver", context.Background(), testActor, ""},
	}

	canceled, cancel := context.WithCancel(context.Background())
	cancel()

	cases = append(cases, struct {
		name   string
		ctx    context.Context
		actor  string
		driver string
	}{"canceled_context", canceled, testActor, testDriverID})

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture()
			session, err := f.service().Begin(
				tc.ctx, tc.actor, tc.driver,
			)
			if session != "" ||
				!errors.Is(err, payouts.ErrStripeConnectOwnershipNotVerified) {
				t.Fatalf("invalid input accepted: %q, %v", session, err)
			}
			if f.sessions.calls != 0 {
				t.Fatal("invalid input caused session creation")
			}
		})
	}
}
