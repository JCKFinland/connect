package stripe

import (
	"context"

	stripego "github.com/stripe/stripe-go/v86"
)

// refundClient defines the Stripe Refund operations required by CONNECT.
//
// Keeping this behind a small interface allows provider execution to be
// tested without making real Stripe API calls.
type refundClient interface {
	Create(
		ctx context.Context,
		params *stripego.RefundCreateParams,
	) (*stripego.Refund, error)
}
