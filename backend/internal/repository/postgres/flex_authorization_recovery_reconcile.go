package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ReconcileFlexAuthorizationRecovery persists a previously verified
// Stripe result for an authorization with an already-known provider ID.
//
// The caller must have independently verified the Stripe PaymentIntent,
// including its financial identity, metadata, manual-capture method,
// and provider status.
//
// This operation never contacts Stripe, creates a PaymentIntent,
// attaches a new provider identity, or confirms a ride.
//
// Recovery is allowed only while the locked database row remains
// PROCESSING and retains the exact expected provider identity.
//
// Terminal states are rejected, including otherwise-idempotent replays.
// This prevents stale recovery work from reporting a fresh success.
func ReconcileFlexAuthorizationRecovery(
	ctx context.Context,
	db *pgxpool.Pool,
	authorizationID string,
	providerAuthorizationID string,
	resultStatus FlexAuthorizationResultStatus,
) (*FlexAuthorization, error) {
	if ctx == nil {
		return nil, errors.New(
			"FLEX authorization recovery context is required",
		)
	}

	if db == nil {
		return nil, errors.New(
			"FLEX authorization recovery database is required",
		)
	}

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	if _, err := uuid.Parse(authorizationID); err != nil {
		return nil, ErrFlexAuthorizationNotFound
	}

	providerAuthorizationID = strings.TrimSpace(
		providerAuthorizationID,
	)

	if providerAuthorizationID == "" {
		return nil, ErrFlexAuthorizationProviderConflict
	}

	// Recovery may persist only outcomes already supported by the
	// authorization lifecycle.
	//
	// FAILED is deliberately excluded: ambiguous Stripe outcomes
	// must not be converted into local financial failures.
	switch resultStatus {
	case FlexResultProcessing,
		FlexResultAuthorized,
		FlexResultCancelled:
	default:
		return nil, ErrFlexAuthorizationInvalidState
	}

	var result *FlexAuthorization

	err := RunInTransaction(ctx, db, func(tx pgx.Tx) error {
		current, err := scanFlexAuthorization(
			tx.QueryRow(ctx, `
				SELECT `+flexAuthorizationColumns+`
				FROM flex_authorizations
				WHERE id = $1
				FOR UPDATE
			`, authorizationID),
		)

		if errors.Is(err, pgx.ErrNoRows) {
			return ErrFlexAuthorizationNotFound
		}

		if err != nil {
			return fmt.Errorf(
				"lock FLEX authorization for recovery reconciliation: %w",
				err,
			)
		}

		if current.ProviderAuthorizationID == nil {
			return ErrFlexAuthorizationProviderConflict
		}

		if *current.ProviderAuthorizationID !=
			providerAuthorizationID {
			return ErrFlexAuthorizationProviderConflict
		}

		if current.Status != "PROCESSING" ||
			current.Provider != "STRIPE" {
			return ErrFlexAuthorizationResultConflict
		}

		// The existing reconciliation function uses the same
		// transaction and locks the same row.
		//
		// PostgreSQL row locks are retained until commit.
		// No concurrent webhook can change the authorization
		// between this eligibility check and reconciliation.
		result, err = ReconcileFlexAuthorizationTx(
			ctx,
			tx,
			authorizationID,
			providerAuthorizationID,
			resultStatus,
		)

		if err != nil {
			return fmt.Errorf(
				"reconcile verified FLEX recovery result: %w",
				err,
			)
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	return result, nil
}
