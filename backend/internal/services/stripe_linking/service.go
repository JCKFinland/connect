package stripelinking

import (
	"context"

	"github.com/JCKFinland/connect/backend/internal/models"
	"github.com/JCKFinland/connect/backend/internal/payouts"
)

// PermissionChecker reads current, authoritative CONNECT permissions.
type PermissionChecker interface {
	HasPermission(context.Context, string, string) (bool, error)
}

// MembershipChecker reads current company membership.
type MembershipChecker interface {
	Exists(context.Context, string, string) (bool, error)
}

// DriverResolver restricts driver lookup to the actor's tenant.
type DriverResolver interface {
	GetByIDForCompanyMember(
		context.Context,
		string,
		string,
	) (*models.Driver, error)
}

// TrustedAccountProvider resolves a Stripe account through an
// independently authenticated server-side onboarding workflow.
//
// It must never return an account ID merely because the client
// supplied that ID. No production provider is wired here.
type TrustedAccountProvider interface {
	ResolveAccount(
		context.Context,
		string,
		string,
		string,
	) (string, error)
}

// SessionCreator persists a short-lived account-linking intent.
// Session creation is not proof of Stripe account ownership.
type SessionCreator interface {
	Create(
		context.Context,
		payouts.StripeConnectOwnershipRequest,
	) (string, error)
}

type Dependencies struct {
	Permissions PermissionChecker
	Memberships MembershipChecker
	Drivers     DriverResolver
	Accounts    TrustedAccountProvider
	Sessions    SessionCreator
}

type Service struct {
	deps Dependencies
}

func NewService(deps Dependencies) *Service {
	return &Service{deps: deps}
}

// Begin creates a linking intent after independently resolving
// authorization and a trusted server-side Stripe account.
//
// actorUserID must come from CONNECT authentication middleware.
// driverID identifies the driver profile, not drivers.user_id.
//
// The result is a session ID only. It never represents verified
// ownership, an approved payout destination, or payout authority.
func (s *Service) Begin(
	ctx context.Context,
	actorUserID string,
	driverID string,
) (string, error) {
	denied := payouts.ErrStripeConnectOwnershipNotVerified

	if ctx == nil || ctx.Err() != nil ||
		s == nil ||
		s.deps.Permissions == nil ||
		s.deps.Memberships == nil ||
		s.deps.Drivers == nil ||
		s.deps.Accounts == nil ||
		s.deps.Sessions == nil ||
		actorUserID == "" ||
		driverID == "" {
		return "", denied
	}

	allowed, err := s.deps.Permissions.HasPermission(
		ctx, actorUserID, "drivers.manage",
	)
	if err != nil || !allowed || ctx.Err() != nil {
		return "", denied
	}

	driver, err := s.deps.Drivers.GetByIDForCompanyMember(
		ctx, actorUserID, driverID,
	)
	if err != nil || driver == nil || ctx.Err() != nil {
		return "", denied
	}

	if driver.UserID == "" || driver.CompanyID == "" ||
		!driver.IsActive || driver.DeletedAt != nil {
		return "", denied
	}

	member, err := s.deps.Memberships.Exists(
		ctx, actorUserID, driver.CompanyID,
	)
	if err != nil || !member || ctx.Err() != nil {
		return "", denied
	}

	accountID, err := s.deps.Accounts.ResolveAccount(
		ctx,
		actorUserID,
		driver.UserID,
		driver.CompanyID,
	)
	if err != nil || ctx.Err() != nil {
		return "", denied
	}

	request := payouts.StripeConnectOwnershipRequest{
		ActorUserID:  actorUserID,
		DriverUserID: driver.UserID,
		CompanyID:    driver.CompanyID,
		AccountID:    accountID,
	}

	if err := payouts.ValidateStripeConnectAccountIdentity(
		payouts.StripeConnectAccountIdentity{
			AccountID: accountID,
			Country:   "FI",
			Currency:  "EUR",
		},
	); err != nil {
		return "", denied
	}

	sessionID, err := s.deps.Sessions.Create(ctx, request)
	if err != nil || ctx.Err() != nil {
		return "", denied
	}

	if sessionID == "" {
		return "", denied
	}

	return sessionID, nil
}
