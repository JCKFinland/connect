package stripelinking

import (
	"context"
	"errors"

	stripe "github.com/stripe/stripe-go/v86"
)

// StripeAccountCreateClient is implemented by the modern Stripe v86
// V1Accounts service. Tests inject a fake implementation.
type StripeAccountCreateClient interface {
	Create(context.Context, *stripe.AccountCreateParams) (*stripe.Account, error)
}

var ErrStripeAccountTransportUnavailable = errors.New(
	"Stripe account creation transport unavailable",
)

// NewStripeAccountCreateTransport adapts the validated legacy account
// parameter shape to the modern Stripe v86 account creation API.
//
// This constructor does not create a Stripe client, load credentials,
// register routes, or authorize a provisioning claim.
func NewStripeAccountCreateTransport(
	client StripeAccountCreateClient,
) AccountCreateFunc {
	return func(
		ctx context.Context,
		params *stripe.AccountParams,
	) (*stripe.Account, error) {
		if ctx == nil || params == nil || client == nil {
			return nil, ErrStripeAccountTransportUnavailable
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		if params.IdempotencyKey == nil ||
			*params.IdempotencyKey == "" {
			return nil, ErrStripeAccountTransportUnavailable
		}

		modern := &stripe.AccountCreateParams{
			Type:            params.Type,
			Country:         params.Country,
			DefaultCurrency: params.DefaultCurrency,
			Metadata:        make(map[string]string, len(params.Metadata)),
		}
		for key, value := range params.Metadata {
			modern.Metadata[key] = value
		}
		modern.SetIdempotencyKey(*params.IdempotencyKey)

		// Preserve both the response and the error. A valid account
		// returned alongside a cancellation/error may be important
		// evidence for the coordinator's reconciliation fence.
		return client.Create(ctx, modern)
	}
}
