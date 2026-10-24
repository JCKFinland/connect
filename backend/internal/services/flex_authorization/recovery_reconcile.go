package flex_authorization

import (
	"context"
	"errors"
	"fmt"

	postgresrepo "github.com/JCKFinland/connect/backend/internal/repository/postgres"
	stripego "github.com/stripe/stripe-go/v86"
)

// RecoveryReconcileResult describes the persisted outcome of a
// previously verified Stripe PaymentIntent.
//
// It does not authorize ride confirmation or fund capture.
type RecoveryReconcileResult struct {
	AuthorizationID         string
	ProviderAuthorizationID string
	ProviderStatus          stripego.PaymentIntentStatus
	Status                  string
	Authorized              bool
	RequiresCustomerAction  bool
}

// RecoverAndReconcile retrieves an existing Stripe PaymentIntent,
// validates its financial identity through the configured adapter,
// and conditionally persists its verified authorization state.
//
// Recover remains read-only. This method is the explicit write path.
//
// The repository requires the locked authorization to remain
// PROCESSING with the same previously persisted provider identity.
//
// This method never creates a PaymentIntent, captures funds,
// confirms a ride, or starts a background worker.
func (s *RecoveryService) RecoverAndReconcile(
	ctx context.Context,
	authorizationID string,
) (*RecoveryReconcileResult, error) {
	if s == nil || s.db == nil || s.adapter == nil {
		return nil, ErrServiceUnavailable
	}

	if ctx == nil {
		return nil, errors.New(
			"FLEX recovery reconciliation context is required",
		)
	}

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	// Recover performs retrieval-only verification.
	// A missing provider identity is rejected before any
	// payment-provider adapter call.
	verified, err := s.Recover(ctx, authorizationID)
	if err != nil {
		return nil, fmt.Errorf(
			"verify FLEX authorization before recovery reconciliation: %w",
			err,
		)
	}

	if verified == nil {
		return nil, errors.New(
			"FLEX recovery verification returned no result",
		)
	}

	var target postgresrepo.FlexAuthorizationResultStatus

	switch verified.ProviderStatus {
	case stripego.PaymentIntentStatusRequiresCapture:
		if !verified.Authorized ||
			verified.RequiresCustomerAction {
			return nil, errors.New(
				"FLEX recovery authorization verification is inconsistent",
			)
		}

		target = postgresrepo.FlexResultAuthorized

	case stripego.PaymentIntentStatusCanceled:
		if verified.Authorized ||
			verified.RequiresCustomerAction {
			return nil, errors.New(
				"FLEX recovery cancellation verification is inconsistent",
			)
		}

		target = postgresrepo.FlexResultCancelled

	case stripego.PaymentIntentStatusProcessing,
		stripego.PaymentIntentStatusRequiresAction,
		stripego.PaymentIntentStatusRequiresConfirmation,
		stripego.PaymentIntentStatusRequiresPaymentMethod:
		if verified.Authorized {
			return nil, errors.New(
				"FLEX recovery pending verification is inconsistent",
			)
		}

		target = postgresrepo.FlexResultProcessing

	default:
		return nil, fmt.Errorf(
			"unsupported FLEX recovery reconciliation provider status: %s",
			verified.ProviderStatus,
		)
	}

	// This function locks the authorization row and rechecks
	// both its PROCESSING state and exact provider identity.
	//
	// A concurrent webhook may finalize the authorization
	// between verification and this call. In that case,
	// reconciliation returns a state conflict without
	// modifying the terminal authorization.
	persisted, err :=
		postgresrepo.ReconcileFlexAuthorizationRecovery(
			ctx,
			s.db,
			verified.AuthorizationID,
			verified.ProviderAuthorizationID,
			target,
		)

	if err != nil {
		return nil, fmt.Errorf(
			"persist verified FLEX recovery authorization: %w",
			err,
		)
	}

	if persisted == nil {
		return nil, errors.New(
			"FLEX recovery reconciliation returned no authorization",
		)
	}

	return &RecoveryReconcileResult{
		AuthorizationID:         persisted.ID,
		ProviderAuthorizationID: verified.ProviderAuthorizationID,
		ProviderStatus:          verified.ProviderStatus,
		Status:                  persisted.Status,
		Authorized:              persisted.Status == "AUTHORIZED",
		RequiresCustomerAction:  verified.RequiresCustomerAction,
	}, nil
}
