package flex_authorization

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/JCKFinland/connect/backend/internal/payments/stripe"
	postgresrepo "github.com/JCKFinland/connect/backend/internal/repository/postgres"
	stripego "github.com/stripe/stripe-go/v86"
)

// ExecuteResult describes the local authorization state and any
// customer action required by Stripe.
type ExecuteResult struct {
	AuthorizationID         string
	Status                  string
	ProviderAuthorizationID string
	ClientSecret            string
	RequiresCustomerAction  bool
	Authorized              bool
}

// Execute establishes local identity, claims processing, and coordinates
// one provider authorization attempt.
//
// Stripe calls are performed outside PostgreSQL transactions.
func (s *Service) Execute(
	ctx context.Context,
	rideRequestID string,
	auctionID string,
	passengerID string,
) (*ExecuteResult, error) {
	if s == nil || s.db == nil || s.adapter == nil {
		return nil, ErrServiceUnavailable
	}

	authorization, err := postgresrepo.CreateFlexAuthorization(
		ctx,
		s.db,
		rideRequestID,
		auctionID,
		passengerID,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"create FLEX authorization: %w",
			err,
		)
	}

	current, claimed, err :=
		postgresrepo.ClaimFlexAuthorizationProcessing(
			ctx,
			s.db,
			authorization.ID,
		)
	if err != nil {
		return nil, fmt.Errorf(
			"claim FLEX authorization: %w",
			err,
		)
	}

	if current == nil {
		return nil, ErrAuthorizationNotEligible
	}

	result := &ExecuteResult{
		AuthorizationID: current.ID,
		Status:          current.Status,
	}

	if current.ProviderAuthorizationID != nil {
		result.ProviderAuthorizationID =
			*current.ProviderAuthorizationID
	}

	switch current.Status {
	case "AUTHORIZED":
		result.Authorized = true
		return result, nil

	case "FAILED", "CANCELLED", "EXPIRED":
		return result, ErrAuthorizationNotEligible

	case "PROCESSING":
		// A non-claiming request may recover only a known provider ID.
		// It must never initiate another PaymentIntent.
		if !claimed && result.ProviderAuthorizationID == "" {
			return result, ErrAuthorizationPending
		}

	default:
		return result, ErrAuthorizationNotEligible
	}

	request := stripe.FlexAuthorizationRequest{
		AuthorizationID: current.ID,
		AuctionID:       current.AuctionID,
		OfferID:         current.OfferID,
		PassengerID:     current.PassengerID,
		AmountCents:     current.AmountCents,
		Currency:        current.Currency,
		IdempotencyKey:  current.IdempotencyKey,
	}

	if !claimed {
		request.ProviderAuthorizationID =
			result.ProviderAuthorizationID
	}

	providerResult, err :=
		s.adapter.ExecuteFlexAuthorization(ctx, request)
	if err != nil {
		// Provider errors may be ambiguous. Never blindly create a
		// replacement authorization or mark it FAILED.
		return result, fmt.Errorf(
			"execute FLEX provider authorization: %w",
			err,
		)
	}

	if providerResult == nil ||
		strings.TrimSpace(
			providerResult.ProviderAuthorizationID,
		) == "" {
		return result, errors.New(
			"FLEX provider returned no authorization identity",
		)
	}

	// Never accept a different provider identity during recovery.
	if result.ProviderAuthorizationID != "" &&
		providerResult.ProviderAuthorizationID !=
			result.ProviderAuthorizationID {
		return result, errors.New(
			"FLEX provider authorization identity mismatch",
		)
	}

	// Provider flags must agree with the verified PaymentIntent status.
	// Reject inconsistencies before persisting any provider outcome.
	switch providerResult.ProviderStatus {
	case stripego.PaymentIntentStatusRequiresCapture:
		if !providerResult.Authorized ||
			providerResult.RequiresCustomerAction {
			return result, errors.New(
				"FLEX provider authorization flags are inconsistent",
			)
		}

	case stripego.PaymentIntentStatusRequiresAction:
		if providerResult.Authorized ||
			!providerResult.RequiresCustomerAction ||
			strings.TrimSpace(providerResult.ClientSecret) == "" {
			return result, errors.New(
				"FLEX provider customer-action response is invalid",
			)
		}

	case stripego.PaymentIntentStatusRequiresConfirmation,
		stripego.PaymentIntentStatusRequiresPaymentMethod:
		if providerResult.Authorized ||
			!providerResult.RequiresCustomerAction {
			return result, errors.New(
				"FLEX provider customer-action flags are inconsistent",
			)
		}

	case stripego.PaymentIntentStatusProcessing,
		stripego.PaymentIntentStatusCanceled:
		if providerResult.Authorized ||
			providerResult.RequiresCustomerAction {
			return result, errors.New(
				"FLEX provider status flags are inconsistent",
			)
		}
	}

	var target postgresrepo.FlexAuthorizationResultStatus

	switch providerResult.ProviderStatus {
	case stripego.PaymentIntentStatusRequiresCapture:
		if !providerResult.Authorized {
			return result, errors.New(
				"FLEX provider success flag mismatch",
			)
		}
		target = postgresrepo.FlexResultAuthorized

	case stripego.PaymentIntentStatusCanceled:
		target = postgresrepo.FlexResultCancelled

	case stripego.PaymentIntentStatusRequiresAction,
		stripego.PaymentIntentStatusRequiresConfirmation,
		stripego.PaymentIntentStatusRequiresPaymentMethod,
		stripego.PaymentIntentStatusProcessing:
		target = postgresrepo.FlexResultProcessing

	default:
		return result, fmt.Errorf(
			"unsupported FLEX provider status: %s",
			providerResult.ProviderStatus,
		)
	}

	persisted, err := postgresrepo.ReconcileFlexAuthorization(
		ctx,
		s.db,
		current.ID,
		providerResult.ProviderAuthorizationID,
		target,
	)
	if err != nil {
		return result, fmt.Errorf(
			"reconcile FLEX provider authorization: %w",
			err,
		)
	}

	result.Status = persisted.Status
	result.ProviderAuthorizationID =
		providerResult.ProviderAuthorizationID
	result.Authorized = persisted.Status == "AUTHORIZED"

	// Client secrets are returned only for a verified,
	// non-authorized provider response requiring customer action.
	if persisted.Status == "PROCESSING" &&
		providerResult.RequiresCustomerAction {
		result.ClientSecret = providerResult.ClientSecret
		result.RequiresCustomerAction = true
	}

	return result, nil
}
