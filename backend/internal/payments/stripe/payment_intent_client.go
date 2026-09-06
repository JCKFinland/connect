package stripe

import (
	"context"

	stripego "github.com/stripe/stripe-go/v86"
)

type paymentIntentClient interface {
	Create(
		ctx context.Context,
		params *stripego.PaymentIntentCreateParams,
	) (*stripego.PaymentIntent, error)
}
