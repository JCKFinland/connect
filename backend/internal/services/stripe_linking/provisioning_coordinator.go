package stripelinking

import (
	"context"
	"errors"
	"time"

	"github.com/JCKFinland/connect/backend/internal/repository/postgres"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrStripeProvisioningReconciliationRequired = errors.New(
	"Stripe provisioning requires reconciliation",
)

// ProvisioningAccountCreator must use the supplied durable key as the
// provider's idempotency key. It must independently validate the provider
// response and return only a trusted FI/EUR Express account ID.
//
// This interface is deliberately not wired to the Stripe SDK.
// Implementations must not derive the key from client input.
type ProvisioningAccountCreator interface {
	CreateProvisioningAccount(
		context.Context,
		string, // immutable idempotency key
		string, // actor user ID
		string, // driver user ID
		string, // company ID
	) (string, error)
}

// ProvisioningCoordinatorDeps reuses the existing linking authorization
// interfaces. The database is the durable request fence, not evidence
// of account ownership or payout eligibility.
type ProvisioningCoordinatorDeps struct {
	Permissions PermissionChecker
	Memberships MembershipChecker
	Drivers     DriverResolver
	DB          *pgxpool.Pool
	Accounts    ProvisioningAccountCreator
}

type ProvisioningCoordinator struct {
	deps ProvisioningCoordinatorDeps
}

func NewProvisioningCoordinator(
	deps ProvisioningCoordinatorDeps,
) *ProvisioningCoordinator {
	return &ProvisioningCoordinator{deps: deps}
}

// ProvisionAccount authorizes the actor and attempts account creation
// only after the database commits a new PENDING -> REQUESTED claim.
//
// A REQUESTED or REQUIRES_RECONCILIATION replay never calls the provider.
// An ACCOUNT_RECORDED replay returns the previously recorded account ID,
// but does not prove ownership, onboarding or payout eligibility.
func (c *ProvisioningCoordinator) ProvisionAccount(
	ctx context.Context,
	actorUserID string,
	driverProfileID string,
) (string, error) {
	denied := ErrStripeProvisioningDenied

	if ctx == nil || ctx.Err() != nil ||
		c == nil ||
		c.deps.Permissions == nil ||
		c.deps.Memberships == nil ||
		c.deps.Drivers == nil ||
		c.deps.DB == nil ||
		c.deps.Accounts == nil ||
		!canonicalID(actorUserID) ||
		!canonicalID(driverProfileID) {
		return "", denied
	}

	allowed, err := c.deps.Permissions.HasPermission(
		ctx, actorUserID, "drivers.manage",
	)
	if err != nil || !allowed || ctx.Err() != nil {
		return "", denied
	}

	driver, err := c.deps.Drivers.GetByIDForCompanyMember(
		ctx, actorUserID, driverProfileID,
	)
	if err != nil || driver == nil || ctx.Err() != nil {
		return "", denied
	}

	if !canonicalID(driver.UserID) ||
		!canonicalID(driver.CompanyID) ||
		!driver.IsActive ||
		driver.DeletedAt != nil {
		return "", denied
	}

	member, err := c.deps.Memberships.Exists(
		ctx, actorUserID, driver.CompanyID,
	)
	if err != nil || !member || ctx.Err() != nil {
		return "", denied
	}

	intent, _, err := postgres.ReserveStripeProvisioningIntent(
		ctx, c.deps.DB,
		postgres.ReserveStripeProvisioningIntentParams{
			ActorUserID:  actorUserID,
			DriverUserID: driver.UserID,
			CompanyID:    driver.CompanyID,
		},
	)
	if err != nil || intent == nil || ctx.Err() != nil {
		return "", denied
	}

	if intent.Status == "ACCOUNT_RECORDED" {
		if intent.StripeAccountID == "" {
			return "", denied
		}
		return intent.StripeAccountID, nil
	}

	if intent.Status != "PENDING" {
		return "", ErrStripeProvisioningReconciliationRequired
	}

	claimedIntent, claimed, err := postgres.ClaimStripeProvisioningIntent(
		ctx, c.deps.DB,
		postgres.ClaimStripeProvisioningIntentParams{
			IntentID:       intent.ID,
			ActorUserID:    intent.ActorUserID,
			DriverUserID:   intent.DriverUserID,
			CompanyID:      intent.CompanyID,
			IdempotencyKey: intent.IdempotencyKey,
		},
	)
	if err != nil || claimedIntent == nil {
		return "", denied
	}
	if !claimed {
		if claimedIntent.Status == "ACCOUNT_RECORDED" &&
			claimedIntent.StripeAccountID != "" {
			return claimedIntent.StripeAccountID, nil
		}
		return "", ErrStripeProvisioningReconciliationRequired
	}

	// The provider call is permitted only by this committed claim.
	// A cancellation after the claim is ambiguous: never create again.
	if ctx.Err() != nil {
		c.markUncertain(claimedIntent)
		return "", ErrStripeProvisioningReconciliationRequired
	}

	accountID, providerErr := c.deps.Accounts.CreateProvisioningAccount(
		ctx,
		claimedIntent.IdempotencyKey,
		claimedIntent.ActorUserID,
		claimedIntent.DriverUserID,
		claimedIntent.CompanyID,
	)

	if providerErr != nil ||
		ctx.Err() != nil ||
		!validProvisioningAccountID(accountID) {
		c.markUncertain(claimedIntent)
		return "", ErrStripeProvisioningReconciliationRequired
	}

	recorded, _, recordErr := postgres.RecordStripeProvisioningAccount(
		ctx, c.deps.DB,
		postgres.RecordStripeProvisioningAccountParams{
			IntentID:        claimedIntent.ID,
			ActorUserID:     claimedIntent.ActorUserID,
			DriverUserID:    claimedIntent.DriverUserID,
			CompanyID:       claimedIntent.CompanyID,
			IdempotencyKey:  claimedIntent.IdempotencyKey,
			StripeAccountID: accountID,
		},
	)
	if recordErr != nil || recorded == nil ||
		recorded.Status != "ACCOUNT_RECORDED" ||
		recorded.StripeAccountID != accountID {
		c.markUncertain(claimedIntent)
		return "", ErrStripeProvisioningReconciliationRequired
	}

	return recorded.StripeAccountID, nil
}

func validProvisioningAccountID(value string) bool {
	if len(value) < 13 || len(value) > 69 ||
		len(value) < 5 || value[:5] != "acct_" {
		return false
	}
	for _, ch := range value[5:] {
		if !((ch >= 'a' && ch <= 'z') ||
			(ch >= 'A' && ch <= 'Z') ||
			(ch >= '0' && ch <= '9')) {
			return false
		}
	}
	return true
}

// Reconciliation is best-effort: if persistence is unavailable, the
// already-committed REQUESTED state itself still fences retries.
// Use a fresh bounded context when the request context was canceled.
func (c *ProvisioningCoordinator) markUncertain(
	intent *postgres.StripeProvisioningIntent,
) {
	if c == nil || c.deps.DB == nil || intent == nil {
		return
	}

	ctx, cancel := context.WithTimeout(
		context.Background(), 5*time.Second,
	)
	defer cancel()

	_, _, _ = postgres.MarkStripeProvisioningReconciliation(
		ctx, c.deps.DB,
		postgres.MarkStripeProvisioningReconciliationParams{
			IntentID:       intent.ID,
			ActorUserID:    intent.ActorUserID,
			DriverUserID:   intent.DriverUserID,
			CompanyID:      intent.CompanyID,
			IdempotencyKey: intent.IdempotencyKey,
		},
	)
}
