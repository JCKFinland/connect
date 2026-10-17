package stripe

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	stripego "github.com/stripe/stripe-go/v86"
	"github.com/stripe/stripe-go/v86/webhook"
)

// VerifiedFlexAuthorizationEvent is produced only after successful
// Stripe signature verification and basic event validation.
//
// Database identity and financial consistency must be checked
// against the authoritative CONNECT authorization before reconciliation.
type VerifiedFlexAuthorizationEvent struct {
	EventID   string
	EventType string

	ProviderAuthorizationID string
	ProviderStatus          stripego.PaymentIntentStatus

	AuthorizationID string
	AuctionID       string
	OfferID         string
	PassengerID     string

	AmountCents   int64
	Currency      string
	CaptureMethod stripego.PaymentIntentCaptureMethod
}

// FlexWebhookVerifier verifies Stripe events belonging specifically
// to CONNECT FLEX authorization operations.
type FlexWebhookVerifier struct {
	signingSecret string
}

func NewFlexWebhookVerifier(
	signingSecret string,
) (*FlexWebhookVerifier, error) {
	signingSecret = strings.TrimSpace(signingSecret)
	if signingSecret == "" {
		return nil, errors.New(
			"FLEX Stripe webhook signing secret is required",
		)
	}

	return &FlexWebhookVerifier{
		signingSecret: signingSecret,
	}, nil
}

func (v *FlexWebhookVerifier) Verify(
	_ context.Context,
	headers http.Header,
	rawBody []byte,
) (*VerifiedFlexAuthorizationEvent, error) {
	if v == nil || v.signingSecret == "" {
		return nil, errors.New(
			"FLEX Stripe webhook verifier is not configured",
		)
	}

	if len(rawBody) == 0 {
		return nil, errors.New(
			"FLEX Stripe webhook body is empty",
		)
	}

	signature := strings.TrimSpace(
		headers.Get("Stripe-Signature"),
	)
	if signature == "" {
		return nil, errors.New(
			"FLEX Stripe webhook signature is missing",
		)
	}

	event, err := webhook.ConstructEvent(
		rawBody,
		signature,
		v.signingSecret,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"verify FLEX Stripe webhook signature: %w",
			err,
		)
	}

	switch event.Type {
	case "payment_intent.processing",
		"payment_intent.amount_capturable_updated",
		"payment_intent.payment_failed",
		"payment_intent.canceled":
		// Supported FLEX authorization events.

	default:
		return nil, fmt.Errorf(
			"unsupported FLEX Stripe webhook event: %s",
			event.Type,
		)
	}

	if strings.TrimSpace(event.ID) == "" {
		return nil, errors.New(
			"FLEX Stripe webhook event ID is required",
		)
	}

	var intent stripego.PaymentIntent
	if err := json.Unmarshal(
		event.Data.Raw,
		&intent,
	); err != nil {
		return nil, fmt.Errorf(
			"decode FLEX Stripe PaymentIntent: %w",
			err,
		)
	}

	if strings.TrimSpace(intent.ID) == "" {
		return nil, errors.New(
			"FLEX Stripe PaymentIntent ID is required",
		)
	}

	metadata := intent.Metadata

	if metadata["connect_operation"] != "FLEX_AUTHORIZE" {
		return nil, errors.New(
			"Stripe event is not a CONNECT FLEX authorization",
		)
	}

	authorizationID := strings.TrimSpace(
		metadata["connect_authorization_id"],
	)
	auctionID := strings.TrimSpace(
		metadata["connect_auction_id"],
	)
	offerID := strings.TrimSpace(
		metadata["connect_offer_id"],
	)
	passengerID := strings.TrimSpace(
		metadata["connect_passenger_id"],
	)

	if authorizationID == "" ||
		auctionID == "" ||
		offerID == "" ||
		passengerID == "" {
		return nil, errors.New(
			"FLEX Stripe webhook identity metadata is incomplete",
		)
	}

	if intent.Amount <= 0 ||
		!strings.EqualFold(
			string(intent.Currency),
			"EUR",
		) {
		return nil, errors.New(
			"FLEX Stripe webhook financial details are invalid",
		)
	}

	if intent.CaptureMethod !=
		stripego.PaymentIntentCaptureMethodManual {
		return nil, errors.New(
			"FLEX Stripe webhook requires manual capture",
		)
	}

	// Event types and PaymentIntent states must agree.
	switch event.Type {
	case "payment_intent.amount_capturable_updated":
		if intent.Status !=
			stripego.PaymentIntentStatusRequiresCapture {
			return nil, errors.New(
				"FLEX capturable event has inconsistent status",
			)
		}

	case "payment_intent.processing":
		if intent.Status !=
			stripego.PaymentIntentStatusProcessing {
			return nil, errors.New(
				"FLEX processing event has inconsistent status",
			)
		}

	case "payment_intent.canceled":
		if intent.Status !=
			stripego.PaymentIntentStatusCanceled {
			return nil, errors.New(
				"FLEX cancellation event has inconsistent status",
			)
		}

	case "payment_intent.payment_failed":
		if intent.Status !=
			stripego.PaymentIntentStatusRequiresPaymentMethod {
			return nil, errors.New(
				"FLEX failure event has inconsistent status",
			)
		}
	}

	return &VerifiedFlexAuthorizationEvent{
		EventID:   event.ID,
		EventType: string(event.Type),

		ProviderAuthorizationID: intent.ID,
		ProviderStatus:          intent.Status,

		AuthorizationID: authorizationID,
		AuctionID:       auctionID,
		OfferID:         offerID,
		PassengerID:     passengerID,

		AmountCents:   intent.Amount,
		Currency:      strings.ToUpper(string(intent.Currency)),
		CaptureMethod: intent.CaptureMethod,
	}, nil
}
