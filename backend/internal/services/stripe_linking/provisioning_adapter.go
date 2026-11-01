package stripelinking

import (
	"context"
	"strings"

	stripe "github.com/stripe/stripe-go/v86"
)

// StripeProvisioningAdapter bridges the durable provisioning coordinator
// to the existing FI/EUR Express account provisioner.
//
// The create function is injected. This adapter does not initialize
// Stripe credentials, construct a live SDK client, or authorize payouts.
type StripeProvisioningAdapter struct {
	create AccountCreateFunc
}

var _ ProvisioningAccountCreator = (*StripeProvisioningAdapter)(nil)

func NewStripeProvisioningAdapter(
	create AccountCreateFunc,
) *StripeProvisioningAdapter {
	return &StripeProvisioningAdapter{create: create}
}

// CreateProvisioningAccount applies the immutable ledger key as Stripe's
// request-level Idempotency-Key header, never as account metadata.
//
// The coordinator must have committed the REQUESTED claim before calling
// this method. This adapter does not independently authorize that claim.
func (a *StripeProvisioningAdapter) CreateProvisioningAccount(
	ctx context.Context,
	idempotencyKey string,
	actorUserID string,
	driverUserID string,
	companyID string,
) (string, error) {
	if ctx == nil || ctx.Err() != nil ||
		a == nil || a.create == nil {
		return "", ErrStripeProvisioningDenied
	}

	const prefix = "connect-stripe-provisioning:"
	suffix, ok := strings.CutPrefix(idempotencyKey, prefix)
	if !ok || !canonicalID(suffix) {
		return "", ErrStripeProvisioningDenied
	}

	create := func(
		requestCtx context.Context,
		params *stripe.AccountParams,
	) (*stripe.Account, error) {
		if requestCtx == nil || requestCtx.Err() != nil ||
			params == nil {
			return nil, ErrStripeProvisioningDenied
		}

		params.SetIdempotencyKey(idempotencyKey)
		return a.create(requestCtx, params)
	}

	return NewAccountProvisioner(create).Provision(
		ctx, actorUserID, driverUserID, companyID,
	)
}
