package stripe

import (
	"context"
	"errors"
	"fmt"
	"strings"

	stripego "github.com/stripe/stripe-go/v86"
)

// FlexAuthorizationRequest contains the immutable financial identity
// of a previously persisted CONNECT FLEX authorization.
type FlexAuthorizationRequest struct {
	AuthorizationID string
	AuctionID       string
	OfferID         string
	PassengerID     string

	AmountCents    int64
	Currency       string
	IdempotencyKey string

	// When present, recovery retrieves this PaymentIntent instead
	// of creating another provider resource.
	ProviderAuthorizationID string
}

// FlexAuthorizationResult is the immediate Stripe response.
// It is not permission to confirm a ride or capture funds.
type FlexAuthorizationResult struct {
	ProviderAuthorizationID string
	ProviderStatus          stripego.PaymentIntentStatus
	ClientSecret            string
	RequiresCustomerAction  bool
	Authorized              bool
}

type FlexAuthorizationAdapter interface {
	ExecuteFlexAuthorization(
		ctx context.Context,
		req FlexAuthorizationRequest,
	) (*FlexAuthorizationResult, error)
}

type flexAuthorizationAdapter struct {
	paymentIntents paymentIntentClient
}

func NewFlexAuthorizationAdapter(
	secretKey string,
) (FlexAuthorizationAdapter, error) {
	secretKey = strings.TrimSpace(secretKey)
	if secretKey == "" {
		return nil, fmt.Errorf(
			"%w: Stripe secret key is required",
			ErrInvalidOperation,
		)
	}

	client := stripego.NewClient(secretKey)

	return &flexAuthorizationAdapter{
		paymentIntents: client.V1PaymentIntents,
	}, nil
}

// This constructor is used by package-local tests to inject a fake
// Stripe client without network requests.
func newFlexAuthorizationAdapterWithClient(
	client paymentIntentClient,
) FlexAuthorizationAdapter {
	return &flexAuthorizationAdapter{
		paymentIntents: client,
	}
}

func (a *flexAuthorizationAdapter) ExecuteFlexAuthorization(
	ctx context.Context,
	req FlexAuthorizationRequest,
) (*FlexAuthorizationResult, error) {
	if a.paymentIntents == nil {
		return nil, errors.New(
			"Stripe PaymentIntent client is not configured",
		)
	}

	if strings.TrimSpace(req.AuthorizationID) == "" ||
		strings.TrimSpace(req.AuctionID) == "" ||
		strings.TrimSpace(req.OfferID) == "" ||
		strings.TrimSpace(req.PassengerID) == "" ||
		strings.TrimSpace(req.IdempotencyKey) == "" {
		return nil, fmt.Errorf(
			"%w: FLEX authorization identity is incomplete",
			ErrInvalidOperation,
		)
	}

	if req.AmountCents <= 0 ||
		strings.TrimSpace(req.Currency) != "EUR" {
		return nil, fmt.Errorf(
			"%w: FLEX authorization amount or currency is invalid",
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

	var (
		intent *stripego.PaymentIntent
		err    error
	)

	providerID := strings.TrimSpace(
		req.ProviderAuthorizationID,
	)

	if providerID != "" {
		intent, err = a.paymentIntents.Retrieve(
			ctx,
			providerID,
			&stripego.PaymentIntentRetrieveParams{},
		)
		if err != nil {
			return nil, fmt.Errorf(
				"retrieve FLEX Stripe PaymentIntent: %w",
				err,
			)
		}
	} else {
		currency := "eur"
		captureMethod := string(
			stripego.PaymentIntentCaptureMethodManual,
		)
		automaticPaymentMethodsEnabled := true
		description := "CONNECT FLEX authorization " +
			req.AuthorizationID

		params := &stripego.PaymentIntentCreateParams{
			Amount:        &req.AmountCents,
			Currency:      &currency,
			CaptureMethod: &captureMethod,
			AutomaticPaymentMethods: &stripego.PaymentIntentCreateAutomaticPaymentMethodsParams{
				Enabled: &automaticPaymentMethodsEnabled,
			},
			Description: &description,
			Metadata:    expectedMetadata,
		}

		params.SetIdempotencyKey(req.IdempotencyKey)

		intent, err = a.paymentIntents.Create(ctx, params)
		if err != nil {
			return nil, fmt.Errorf(
				"create FLEX Stripe PaymentIntent: %w",
				err,
			)
		}
	}

	if intent == nil ||
		strings.TrimSpace(intent.ID) == "" {
		return nil, errors.New(
			"FLEX Stripe PaymentIntent response has no identity",
		)
	}

	if providerID != "" && intent.ID != providerID {
		return nil, fmt.Errorf(
			"%w: FLEX Stripe provider identity mismatch",
			ErrInvalidOperation,
		)
	}

	if intent.Amount != req.AmountCents ||
		!strings.EqualFold(
			string(intent.Currency),
			req.Currency,
		) {
		return nil, fmt.Errorf(
			"%w: FLEX Stripe financial identity mismatch",
			ErrInvalidOperation,
		)
	}

	if intent.CaptureMethod !=
		stripego.PaymentIntentCaptureMethodManual {
		return nil, fmt.Errorf(
			"%w: FLEX Stripe PaymentIntent is not manual capture",
			ErrInvalidOperation,
		)
	}

	for key, expected := range expectedMetadata {
		if strings.TrimSpace(intent.Metadata[key]) != expected {
			return nil, fmt.Errorf(
				"%w: FLEX Stripe metadata mismatch for %s",
				ErrInvalidOperation,
				key,
			)
		}
	}

	result := &FlexAuthorizationResult{
		ProviderAuthorizationID: intent.ID,
		ProviderStatus:          intent.Status,
		ClientSecret:            intent.ClientSecret,
	}

	switch intent.Status {
	case stripego.PaymentIntentStatusRequiresCapture:
		result.Authorized = true

	case stripego.PaymentIntentStatusRequiresAction,
		stripego.PaymentIntentStatusRequiresConfirmation,
		stripego.PaymentIntentStatusRequiresPaymentMethod:
		result.RequiresCustomerAction = true

	case stripego.PaymentIntentStatusProcessing:
		// An in-flight provider operation is not authorization success.

	case stripego.PaymentIntentStatusCanceled:
		// Cancellation is reconciled by the separate persistence boundary.

	default:
		return nil, fmt.Errorf(
			"%w: unsupported FLEX Stripe status %q",
			ErrInvalidOperation,
			intent.Status,
		)
	}

	return result, nil
}
