package stripe

import (
	"context"
	"errors"
	"fmt"
	"strings"

	stripego "github.com/stripe/stripe-go/v86"
)

// FlexCaptureVerificationRequest contains the expected authoritative
// identity and financial details of an existing FLEX authorization.
type FlexCaptureVerificationRequest struct {
	ProviderAuthorizationID string

	AuthorizationID string
	AuctionID       string
	OfferID         string
	PassengerID     string

	AmountCents int64
	Currency    string
}

// FlexCaptureVerificationResult contains the verified provider state.
// Captured is true only when Stripe reports a fully captured payment.
type FlexCaptureVerificationResult struct {
	ProviderCaptureID string
	ProviderStatus    stripego.PaymentIntentStatus
	Captured          bool
}

// FlexCaptureVerifier retrieves existing Stripe state.
// It never creates or captures a PaymentIntent.
type FlexCaptureVerifier interface {
	VerifyFlexCapture(
		context.Context,
		FlexCaptureVerificationRequest,
	) (*FlexCaptureVerificationResult, error)
}

func (e *executor) VerifyFlexCapture(
	ctx context.Context,
	req FlexCaptureVerificationRequest,
) (*FlexCaptureVerificationResult, error) {
	if e == nil || e.paymentIntents == nil {
		return nil, errors.New(
			"Stripe PaymentIntent client is not configured",
		)
	}

	if strings.TrimSpace(req.ProviderAuthorizationID) == "" ||
		strings.TrimSpace(req.AuthorizationID) == "" ||
		strings.TrimSpace(req.AuctionID) == "" ||
		strings.TrimSpace(req.OfferID) == "" ||
		strings.TrimSpace(req.PassengerID) == "" ||
		req.AmountCents <= 0 ||
		!strings.EqualFold(req.Currency, "EUR") {
		return nil, fmt.Errorf(
			"%w: invalid FLEX capture verification request",
			ErrInvalidOperation,
		)
	}

	intent, err := e.paymentIntents.Retrieve(
		ctx,
		req.ProviderAuthorizationID,
		&stripego.PaymentIntentRetrieveParams{},
	)
	if err != nil {
		return nil, fmt.Errorf(
			"retrieve FLEX capture PaymentIntent: %w",
			err,
		)
	}

	if intent == nil ||
		intent.ID != req.ProviderAuthorizationID {
		return nil, fmt.Errorf(
			"%w: FLEX capture provider identity mismatch",
			ErrInvalidOperation,
		)
	}

	if intent.Amount != req.AmountCents ||
		!strings.EqualFold(
			string(intent.Currency),
			req.Currency,
		) ||
		intent.CaptureMethod !=
			stripego.PaymentIntentCaptureMethodManual {
		return nil, fmt.Errorf(
			"%w: FLEX capture financial identity mismatch",
			ErrInvalidOperation,
		)
	}

	expectedMetadata := map[string]string{
		"connect_operation":        "FLEX_AUTHORIZE",
		"connect_authorization_id": req.AuthorizationID,
		"connect_auction_id":       req.AuctionID,
		"connect_offer_id":         req.OfferID,
		"connect_passenger_id":     req.PassengerID,
	}

	for key, expected := range expectedMetadata {
		if strings.TrimSpace(intent.Metadata[key]) != expected {
			return nil, fmt.Errorf(
				"%w: FLEX capture metadata mismatch for %s",
				ErrInvalidOperation,
				key,
			)
		}
	}

	result := &FlexCaptureVerificationResult{
		ProviderCaptureID: intent.ID,
		ProviderStatus:    intent.Status,
	}

	switch intent.Status {
	case stripego.PaymentIntentStatusSucceeded:
		if intent.AmountReceived != req.AmountCents ||
			intent.AmountCapturable != 0 {
			return nil, fmt.Errorf(
				"%w: FLEX captured amount mismatch",
				ErrInvalidOperation,
			)
		}

		result.Captured = true

	case stripego.PaymentIntentStatusRequiresCapture:
		if intent.AmountReceived != 0 ||
			intent.AmountCapturable != req.AmountCents {
			return nil, fmt.Errorf(
				"%w: FLEX authorization balance mismatch",
				ErrInvalidOperation,
			)
		}

	case stripego.PaymentIntentStatusProcessing:
		// Provider outcome remains unresolved.

	case stripego.PaymentIntentStatusCanceled:
		// A canceled authorization is not a successful capture.

	default:
		return nil, fmt.Errorf(
			"%w: unsupported FLEX capture provider status %q",
			ErrInvalidOperation,
			intent.Status,
		)
	}

	return result, nil
}

var _ FlexCaptureVerifier = (*executor)(nil)
