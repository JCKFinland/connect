package stripe

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	stripego "github.com/stripe/stripe-go/v86"
	"github.com/stripe/stripe-go/v86/webhook"

	"github.com/JCKFinland/connect/backend/internal/services/paymentcallback"
	"github.com/JCKFinland/connect/backend/internal/services/paymenttransaction"
)

const ProviderName = "STRIPE"

type WebhookVerifier struct {
	signingSecret string
}

func NewWebhookVerifier(
	signingSecret string,
) (*WebhookVerifier, error) {
	signingSecret =
		strings.TrimSpace(signingSecret)

	if signingSecret == "" {
		return nil, fmt.Errorf(
			"Stripe webhook signing secret is required",
		)
	}

	return &WebhookVerifier{
		signingSecret: signingSecret,
	}, nil
}

func (v *WebhookVerifier) Verify(
	_ context.Context,
	headers http.Header,
	rawBody []byte,
) (*paymentcallback.VerifiedCallback, error) {
	if len(rawBody) == 0 {
		return nil, fmt.Errorf(
			"%w: empty Stripe webhook body",
			paymentcallback.ErrCallbackVerificationFailed,
		)
	}

	signature :=
		headers.Get("Stripe-Signature")

	if signature == "" {
		return nil, fmt.Errorf(
			"%w: missing Stripe-Signature header",
			paymentcallback.ErrCallbackVerificationFailed,
		)
	}

	event, err :=
		webhook.ConstructEvent(
			rawBody,
			signature,
			v.signingSecret,
		)
	if err != nil {
		return nil, fmt.Errorf(
			"%w: %v",
			paymentcallback.ErrCallbackVerificationFailed,
			err,
		)
	}

	return mapStripeEvent(
		event,
		rawBody,
	)
}

func mapStripeEvent(
	event stripego.Event,
	rawBody []byte,
) (*paymentcallback.VerifiedCallback, error) {
	switch event.Type {

	case "payment_intent.processing":
		return verifiedPaymentIntentCallback(
			event,
			rawBody,
			paymenttransaction.StatusProcessing,
		)

	case "payment_intent.succeeded":
		return verifiedPaymentIntentCallback(
			event,
			rawBody,
			paymenttransaction.StatusSuccess,
		)

	case "payment_intent.payment_failed":
		return verifiedPaymentIntentCallback(
			event,
			rawBody,
			paymenttransaction.StatusFailed,
		)

	case "payment_intent.canceled":
		return verifiedPaymentIntentCallback(
			event,
			rawBody,
			paymenttransaction.StatusCancelled,
		)

	default:
		return nil, fmt.Errorf(
			"%w: unsupported Stripe event type %s",
			paymentcallback.ErrInvalidCallback,
			event.Type,
		)
	}
}

func verifiedPaymentIntentCallback(
	event stripego.Event,
	rawBody []byte,
	status string,
) (*paymentcallback.VerifiedCallback, error) {
	var paymentIntent stripego.PaymentIntent

	if err := json.Unmarshal(
		event.Data.Raw,
		&paymentIntent,
	); err != nil {
		return nil, fmt.Errorf(
			"%w: decode Stripe PaymentIntent: %v",
			paymentcallback.ErrInvalidCallback,
			err,
		)
	}

	if strings.TrimSpace(paymentIntent.ID) == "" {
		return nil, fmt.Errorf(
			"%w: Stripe PaymentIntent ID is required",
			paymentcallback.ErrInvalidCallback,
		)
	}

	return &paymentcallback.VerifiedCallback{
		Provider: ProviderName,

		ProviderTransactionID: paymentIntent.ID,

		ProviderStatus: status,

		RawPayload: append(
			[]byte(nil),
			rawBody...,
		),
	}, nil
}

var _ paymentcallback.Verifier = (*WebhookVerifier)(nil)
