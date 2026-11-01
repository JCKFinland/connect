package stripelinking

import (
	"context"
	"errors"

	"github.com/JCKFinland/connect/backend/internal/payouts"
	"github.com/google/uuid"
	stripe "github.com/stripe/stripe-go/v86"
)

var ErrStripeProvisioningDenied = errors.New(
	"Stripe Connect account provisioning denied",
)

// AccountCreateFunc is a trusted server-side operation.
//
// No live SDK client or production creation operation is wired
// by this package. Callers must separately enforce idempotency
// and durable actor/driver/company-to-account association before
// enabling creation.
type AccountCreateFunc func(
	context.Context,
	*stripe.AccountParams,
) (*stripe.Account, error)

type AccountProvisioner struct {
	create AccountCreateFunc
}

func NewAccountProvisioner(
	create AccountCreateFunc,
) *AccountProvisioner {
	return &AccountProvisioner{create: create}
}

func canonicalID(value string) bool {
	parsed, err := uuid.Parse(value)
	return err == nil &&
		parsed != uuid.Nil &&
		parsed.String() == value
}

// Provision requests a Finnish EUR Express account using only
// server-generated parameters. The input contains CONNECT
// identities, never a client-supplied Stripe account ID.
//
// Successful provisioning does not prove Stripe account
// ownership, onboarding completion, or payout eligibility.
func (p *AccountProvisioner) Provision(
	ctx context.Context,
	actorUserID string,
	driverUserID string,
	companyID string,
) (string, error) {
	denied := ErrStripeProvisioningDenied

	if ctx == nil ||
		ctx.Err() != nil ||
		p == nil ||
		p.create == nil ||
		!canonicalID(actorUserID) ||
		!canonicalID(driverUserID) ||
		!canonicalID(companyID) {
		return "", denied
	}

	params := &stripe.AccountParams{
		Type:            stripe.String("express"),
		Country:         stripe.String("FI"),
		DefaultCurrency: stripe.String("eur"),
		Metadata: map[string]string{
			"connect_actor_user_id":  actorUserID,
			"connect_driver_user_id": driverUserID,
			"connect_company_id":     companyID,
		},
	}

	account, err := p.create(ctx, params)
	// Once the provider returns a valid response without error,
	// cancellation must not discard that evidence. The coordinator
	// persists it using its independent bounded database context.
	if err != nil || account == nil {
		return "", denied
	}

	if account.Deleted ||
		account.Country != "FI" ||
		string(account.DefaultCurrency) != "eur" ||
		string(account.Type) != "express" ||
		payouts.ValidateStripeConnectAccountIdentity(
			payouts.StripeConnectAccountIdentity{
				AccountID: account.ID,
				Country:   account.Country,
				Currency:  "EUR",
			},
		) != nil {
		return "", denied
	}

	return account.ID, nil
}
