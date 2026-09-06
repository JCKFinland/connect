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

	Capture(
		ctx context.Context,
		id string,
		params *stripego.PaymentIntentCaptureParams,
	) (*stripego.PaymentIntent, error)

	Cancel(
		ctx context.Context,
		id string,
		params *stripego.PaymentIntentCancelParams,
	) (*stripego.PaymentIntent, error)
}
