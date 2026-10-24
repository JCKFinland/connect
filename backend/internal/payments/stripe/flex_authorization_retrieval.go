package stripe

import (
	"context"
	"errors"
	"fmt"
	"strings"

	stripego "github.com/stripe/stripe-go/v86"
)

// FlexAuthorizationRetriever grants only verification of an
// existing Stripe PaymentIntent. It cannot create or capture funds.
type FlexAuthorizationRetriever interface {
	RetrieveFlexAuthorization(
		context.Context,
		FlexAuthorizationRequest,
	) (*FlexAuthorizationResult, error)
}

// flexPaymentIntentRetriever exposes only Stripe retrieval.
type flexPaymentIntentRetriever interface {
	Retrieve(
		context.Context,
		string,
		*stripego.PaymentIntentRetrieveParams,
	) (*stripego.PaymentIntent, error)
}

type flexAuthorizationRetriever struct {
	paymentIntents flexPaymentIntentRetriever
}

func NewFlexAuthorizationRetriever(
	secretKey string,
) (FlexAuthorizationRetriever, error) {
	secretKey = strings.TrimSpace(secretKey)
	if secretKey == "" {
		return nil, fmt.Errorf(
			"%w: Stripe secret key is required",
			ErrInvalidOperation,
		)
	}

	client := stripego.NewClient(secretKey)

	return &flexAuthorizationRetriever{
		paymentIntents: client.V1PaymentIntents,
	}, nil
}

// RetrieveFlexAuthorization never creates a provider resource.
// An existing provider identity is mandatory.
func (a *flexAuthorizationRetriever) RetrieveFlexAuthorization(
	ctx context.Context,
	req FlexAuthorizationRequest,
) (*FlexAuthorizationResult, error) {
	if a == nil || a.paymentIntents == nil {
		return nil, errors.New(
			"Stripe retrieval client is not configured",
		)
	}

	if ctx == nil {
		return nil, errors.New(
			"Stripe retrieval context is required",
		)
	}

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	providerID := strings.TrimSpace(
		req.ProviderAuthorizationID,
	)

	if providerID == "" {
		return nil, fmt.Errorf(
			"%w: existing Stripe PaymentIntent ID is required",
			ErrInvalidOperation,
		)
	}

	if strings.TrimSpace(req.AuthorizationID) == "" ||
		strings.TrimSpace(req.AuctionID) == "" ||
		strings.TrimSpace(req.OfferID) == "" ||
		strings.TrimSpace(req.PassengerID) == "" {
		return nil, fmt.Errorf(
			"%w: incomplete FLEX authorization identity",
			ErrInvalidOperation,
		)
	}

	if req.AmountCents <= 0 ||
		strings.TrimSpace(req.Currency) != "EUR" {
		return nil, fmt.Errorf(
			"%w: invalid FLEX authorization amount or currency",
			ErrInvalidOperation,
		)
	}

	intent, err := a.paymentIntents.Retrieve(
		ctx,
		providerID,
		&stripego.PaymentIntentRetrieveParams{},
	)
	if err != nil {
		return nil, fmt.Errorf(
			"retrieve existing FLEX PaymentIntent: %w",
			err,
		)
	}

	if intent == nil || intent.ID != providerID {
		return nil, fmt.Errorf(
			"%w: Stripe provider identity mismatch",
			ErrInvalidOperation,
		)
	}

	if intent.Amount != req.AmountCents ||
		!strings.EqualFold(
			string(intent.Currency),
			req.Currency,
		) {
		return nil, fmt.Errorf(
			"%w: Stripe financial identity mismatch",
			ErrInvalidOperation,
		)
	}

	if intent.CaptureMethod !=
		stripego.PaymentIntentCaptureMethodManual {
		return nil, fmt.Errorf(
			"%w: Stripe PaymentIntent is not manual capture",
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
				"%w: Stripe metadata mismatch for %s",
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

	case stripego.PaymentIntentStatusProcessing,
		stripego.PaymentIntentStatusCanceled:

	default:
		return nil, fmt.Errorf(
			"%w: unsupported Stripe PaymentIntent status %q",
			ErrInvalidOperation,
			intent.Status,
		)
	}

	return result, nil
}
