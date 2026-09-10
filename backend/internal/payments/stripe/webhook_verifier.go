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

	case "payment_intent.amount_capturable_updated":
		return verifiedPaymentIntentCallback(
			event,
			rawBody,
			paymenttransaction.StatusSuccess,
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

	case "refund.created",
		"refund.updated",
		"refund.failed":

		return verifiedRefundCallback(
			event,
			rawBody,
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

	providerTransactionID :=
		strings.TrimSpace(
			paymentIntent.ID,
		)

	if providerTransactionID == "" {
		return nil, fmt.Errorf(
			"%w: Stripe PaymentIntent ID is required",
			paymentcallback.ErrInvalidCallback,
		)
	}

	transactionID :=
		strings.TrimSpace(
			paymentIntent.Metadata["connect_transaction_id"],
		)

	if transactionID == "" {
		return nil, fmt.Errorf(
			"%w: Stripe connect_transaction_id metadata is required",
			paymentcallback.ErrInvalidCallback,
		)
	}

	paymentID :=
		strings.TrimSpace(
			paymentIntent.Metadata["connect_payment_id"],
		)

	if paymentID == "" {
		return nil, fmt.Errorf(
			"%w: Stripe connect_payment_id metadata is required",
			paymentcallback.ErrInvalidCallback,
		)
	}

	return &paymentcallback.VerifiedCallback{
		Provider: ProviderName,

		TransactionID: transactionID,

		PaymentID: paymentID,

		ProviderTransactionID: providerTransactionID,

		ProviderStatus: status,

		RawPayload: append(
			[]byte(nil),
			rawBody...,
		),
	}, nil
}

func verifiedRefundCallback(
	event stripego.Event,
	rawBody []byte,
) (*paymentcallback.VerifiedCallback, error) {
	var refund stripego.Refund

	if err := json.Unmarshal(
		event.Data.Raw,
		&refund,
	); err != nil {
		return nil, fmt.Errorf(
			"%w: decode Stripe Refund: %v",
			paymentcallback.ErrInvalidCallback,
			err,
		)
	}

	providerTransactionID :=
		strings.TrimSpace(
			refund.ID,
		)

	if providerTransactionID == "" {
		return nil, fmt.Errorf(
			"%w: Stripe Refund ID is required",
			paymentcallback.ErrInvalidCallback,
		)
	}

	transactionID :=
		strings.TrimSpace(
			refund.Metadata["connect_transaction_id"],
		)

	if transactionID == "" {
		return nil, fmt.Errorf(
			"%w: Stripe connect_transaction_id metadata is required",
			paymentcallback.ErrInvalidCallback,
		)
	}

	paymentID :=
		strings.TrimSpace(
			refund.Metadata["connect_payment_id"],
		)

	if paymentID == "" {
		return nil, fmt.Errorf(
			"%w: Stripe connect_payment_id metadata is required",
			paymentcallback.ErrInvalidCallback,
		)
	}

	transactionType :=
		strings.TrimSpace(
			refund.Metadata["connect_transaction_type"],
		)

	if transactionType != paymenttransaction.TypeRefund {
		return nil, fmt.Errorf(
			"%w: Stripe connect_transaction_type must be %s",
			paymentcallback.ErrInvalidCallback,
			paymenttransaction.TypeRefund,
		)
	}

	status, _, err :=
		mapRefundStatus(
			refund.Status,
		)
	if err != nil {
		return nil, fmt.Errorf(
			"%w: map Stripe Refund status %q: %v",
			paymentcallback.ErrInvalidCallback,
			refund.Status,
			err,
		)
	}

	return &paymentcallback.VerifiedCallback{
		Provider: ProviderName,

		TransactionID: transactionID,

		PaymentID: paymentID,

		ProviderTransactionID: providerTransactionID,

		ProviderStatus: status,

		RawPayload: append(
			[]byte(nil),
			rawBody...,
		),
	}, nil
}

var _ paymentcallback.Verifier = (*WebhookVerifier)(nil)
