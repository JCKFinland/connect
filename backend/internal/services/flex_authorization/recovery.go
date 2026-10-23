package flex_authorization

import (
	"context"
	"errors"
	"fmt"
	"strings"

	stripepayment "github.com/JCKFinland/connect/backend/internal/payments/stripe"
	postgresrepo "github.com/JCKFinland/connect/backend/internal/repository/postgres"
	"github.com/jackc/pgx/v5/pgxpool"
	stripego "github.com/stripe/stripe-go/v86"
)

// RecoveryResult describes the verified state of an existing
// Stripe PaymentIntent.
//
// It does not authorize ride confirmation or fund capture.
type RecoveryResult struct {
	AuthorizationID         string
	ProviderAuthorizationID string
	ProviderStatus          stripego.PaymentIntentStatus
	Authorized              bool
	RequiresCustomerAction  bool
}

// RecoveryService verifies existing FLEX authorizations.
//
// It cannot create a local authorization, claim processing,
// capture funds, or reconcile financial state.
type RecoveryService struct {
	db      *pgxpool.Pool
	adapter stripepayment.FlexAuthorizationAdapter
}

func NewRecoveryService(
	db *pgxpool.Pool,
	adapter stripepayment.FlexAuthorizationAdapter,
) *RecoveryService {
	return &RecoveryService{
		db:      db,
		adapter: adapter,
	}
}

// Recover performs retrieval-only verification of a known
// Stripe PaymentIntent.
//
// A missing provider identity is rejected before any adapter call.
//
// The existing Stripe adapter validates immutable financial
// metadata, amount, currency, manual capture, and provider identity.
//
// This method does not modify PostgreSQL authorization state.
func (s *RecoveryService) Recover(
	ctx context.Context,
	authorizationID string,
) (*RecoveryResult, error) {
	if s == nil || s.db == nil || s.adapter == nil {
		return nil, ErrServiceUnavailable
	}

	if ctx == nil {
		return nil, errors.New(
			"FLEX authorization recovery context is required",
		)
	}

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	authorization, err :=
		postgresrepo.GetFlexAuthorizationForRecovery(
			ctx,
			s.db,
			authorizationID,
		)
	if err != nil {
		return nil, fmt.Errorf(
			"load FLEX authorization for recovery: %w",
			err,
		)
	}

	if authorization == nil ||
		authorization.ProviderAuthorizationID == nil {
		return nil, postgresrepo.ErrFlexAuthorizationNotFound
	}

	providerID := strings.TrimSpace(
		*authorization.ProviderAuthorizationID,
	)
	if providerID == "" {
		return nil, postgresrepo.ErrFlexAuthorizationNotFound
	}

	request := stripepayment.FlexAuthorizationRequest{
		AuthorizationID:         authorization.ID,
		AuctionID:               authorization.AuctionID,
		OfferID:                 authorization.OfferID,
		PassengerID:             authorization.PassengerID,
		AmountCents:             authorization.AmountCents,
		Currency:                authorization.Currency,
		IdempotencyKey:          authorization.IdempotencyKey,
		ProviderAuthorizationID: providerID,
	}

	// A nonempty provider ID forces the production Stripe
	// adapter down its Retrieve path rather than Create.
	verification, err :=
		s.adapter.ExecuteFlexAuthorization(ctx, request)
	if err != nil {
		return nil, fmt.Errorf(
			"verify existing FLEX Stripe authorization: %w",
			err,
		)
	}

	if verification == nil ||
		verification.ProviderAuthorizationID != providerID {
		return nil, errors.New(
			"FLEX recovery provider identity mismatch",
		)
	}

	result := &RecoveryResult{
		AuthorizationID:         authorization.ID,
		ProviderAuthorizationID: providerID,
		ProviderStatus:          verification.ProviderStatus,
	}

	switch verification.ProviderStatus {
	case stripego.PaymentIntentStatusRequiresCapture:
		if !verification.Authorized ||
			verification.RequiresCustomerAction {
			return nil, errors.New(
				"FLEX recovery authorization flags are inconsistent",
			)
		}

		result.Authorized = true

	case stripego.PaymentIntentStatusRequiresAction,
		stripego.PaymentIntentStatusRequiresConfirmation,
		stripego.PaymentIntentStatusRequiresPaymentMethod:
		if verification.Authorized ||
			!verification.RequiresCustomerAction {
			return nil, errors.New(
				"FLEX recovery customer-action flags are inconsistent",
			)
		}

		if verification.ProviderStatus ==
			stripego.PaymentIntentStatusRequiresAction &&
			strings.TrimSpace(verification.ClientSecret) == "" {
			return nil, errors.New(
				"FLEX recovery customer-action secret is missing",
			)
		}

		result.RequiresCustomerAction = true

	case stripego.PaymentIntentStatusProcessing,
		stripego.PaymentIntentStatusCanceled:
		if verification.Authorized ||
			verification.RequiresCustomerAction {
			return nil, errors.New(
				"FLEX recovery provider flags are inconsistent",
			)
		}

	default:
		return nil, fmt.Errorf(
			"unsupported FLEX recovery provider status: %s",
			verification.ProviderStatus,
		)
	}

	return result, nil
}
