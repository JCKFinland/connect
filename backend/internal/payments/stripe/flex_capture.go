package stripe

import (
	"context"
	"errors"
	"fmt"
	"strings"

	stripego "github.com/stripe/stripe-go/v86"
)

// FlexCaptureRequest describes a previously persisted FLEX capture
// operation. The caller must supply the settlement's immutable
// authorization identity, amount, and capture idempotency key.
type FlexCaptureRequest struct {
	ProviderAuthorizationID string
	AmountCents             int64
	Currency                string
	CaptureIdempotencyKey   string
}

// FlexCaptureResult describes the immediate provider response.
// This result is not proof that CONNECT has finalized settlement.
type FlexCaptureResult struct {
	ProviderCaptureID string
	ProviderStatus    stripego.PaymentIntentStatus
}

// FlexCaptureExecutor submits a capture request for an existing
// manually authorized Stripe PaymentIntent.
type FlexCaptureExecutor interface {
	ExecuteFlexCapture(
		context.Context,
		FlexCaptureRequest,
	) (*FlexCaptureResult, error)
}

type flexCaptureAdapter struct {
	paymentIntents paymentIntentClient
}

func NewFlexCaptureAdapter(
	secretKey string,
) (FlexCaptureExecutor, error) {
	secretKey = strings.TrimSpace(secretKey)
	if secretKey == "" {
		return nil, fmt.Errorf(
			"%w: Stripe secret key is required",
			ErrInvalidOperation,
		)
	}

	client := stripego.NewClient(secretKey)

	return &flexCaptureAdapter{
		paymentIntents: client.V1PaymentIntents,
	}, nil
}

// Package-local constructor for network-free Stripe unit tests.
func newFlexCaptureAdapterWithClient(
	client paymentIntentClient,
) FlexCaptureExecutor {
	return &flexCaptureAdapter{
		paymentIntents: client,
	}
}

func (a *flexCaptureAdapter) ExecuteFlexCapture(
	ctx context.Context,
	req FlexCaptureRequest,
) (*FlexCaptureResult, error) {
	if a == nil || a.paymentIntents == nil {
		return nil, errors.New(
			"FLEX Stripe capture client is not configured",
		)
	}

	if strings.TrimSpace(req.ProviderAuthorizationID) == "" ||
		strings.TrimSpace(req.ProviderAuthorizationID) !=
			req.ProviderAuthorizationID ||
		strings.TrimSpace(req.CaptureIdempotencyKey) == "" ||
		strings.TrimSpace(req.CaptureIdempotencyKey) !=
			req.CaptureIdempotencyKey ||
		req.AmountCents <= 0 ||
		req.Currency != "EUR" {
		return nil, fmt.Errorf(
			"%w: invalid FLEX capture identity or amount",
			ErrInvalidOperation,
		)
	}

	params := &stripego.PaymentIntentCaptureParams{
		AmountToCapture: &req.AmountCents,
	}

	params.SetIdempotencyKey(
		req.CaptureIdempotencyKey,
	)

	intent, err := a.paymentIntents.Capture(
		ctx,
		req.ProviderAuthorizationID,
		params,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"capture FLEX Stripe PaymentIntent: %w",
			err,
		)
	}

	if intent == nil {
		return nil, errors.New(
			"FLEX Stripe capture returned no PaymentIntent",
		)
	}

	if intent.ID != req.ProviderAuthorizationID {
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

	switch intent.Status {
	case stripego.PaymentIntentStatusSucceeded:
		if intent.AmountReceived != req.AmountCents ||
			intent.AmountCapturable != 0 {
			return nil, fmt.Errorf(
				"%w: FLEX capture amount received mismatch",
				ErrInvalidOperation,
			)
		}

	case stripego.PaymentIntentStatusProcessing:
		// The provider outcome remains unresolved.
		// The caller must reconcile before finalization.

	default:
		return nil, fmt.Errorf(
			"%w: unsupported FLEX capture response status %q",
			ErrInvalidOperation,
			intent.Status,
		)
	}

	return &FlexCaptureResult{
		ProviderCaptureID: intent.ID,
		ProviderStatus:    intent.Status,
	}, nil
}

var _ FlexCaptureExecutor = (*flexCaptureAdapter)(nil)
