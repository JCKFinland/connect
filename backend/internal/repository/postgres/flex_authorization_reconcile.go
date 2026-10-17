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

var (
	ErrFlexAuthorizationProviderConflict = errors.New(
		"FLEX authorization provider identity conflict",
	)

	ErrFlexAuthorizationResultConflict = errors.New(
		"FLEX authorization result conflicts with current state",
	)
)

// FlexAuthorizationResultStatus represents a validated provider outcome.
// PROCESSING means the PaymentIntent exists but authorization is not complete.
type FlexAuthorizationResultStatus string

const (
	FlexResultProcessing FlexAuthorizationResultStatus = "PROCESSING"
	FlexResultAuthorized FlexAuthorizationResultStatus = "AUTHORIZED"
	FlexResultFailed     FlexAuthorizationResultStatus = "FAILED"
	FlexResultCancelled  FlexAuthorizationResultStatus = "CANCELLED"
)

// ReconcileFlexAuthorization persists an already-validated provider result.
//
// The caller must verify provider authenticity, financial identity,
// manual-capture configuration, and FLEX metadata before invoking this.
//
// This operation never contacts Stripe and never confirms a ride.
func ReconcileFlexAuthorization(
	ctx context.Context,
	db *pgxpool.Pool,
	authorizationID string,
	providerAuthorizationID string,
	resultStatus FlexAuthorizationResultStatus,
) (*FlexAuthorization, error) {
	if db == nil {
		return nil, errors.New(
			"FLEX authorization database is required",
		)
	}

	var result *FlexAuthorization

	err := RunInTransaction(ctx, db, func(tx pgx.Tx) error {
		var err error

		result, err = ReconcileFlexAuthorizationTx(
			ctx,
			tx,
			authorizationID,
			providerAuthorizationID,
			resultStatus,
		)

		return err
	})
	if err != nil {
		return nil, err
	}

	return result, nil
}

// ReconcileFlexAuthorizationTx reconciles an authorization inside
// a transaction owned by the caller.
//
// It does not commit or roll back the supplied transaction.
func ReconcileFlexAuthorizationTx(
	ctx context.Context,
	tx pgx.Tx,
	authorizationID string,
	providerAuthorizationID string,
	resultStatus FlexAuthorizationResultStatus,
) (*FlexAuthorization, error) {
	if tx == nil {
		return nil, errors.New(
			"FLEX authorization transaction is required",
		)
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

	switch resultStatus {
	case FlexResultProcessing,
		FlexResultAuthorized,
		FlexResultFailed,
		FlexResultCancelled:
	default:
		return nil, ErrFlexAuthorizationInvalidState
	}

	current, err := scanFlexAuthorization(
		tx.QueryRow(ctx, `
			SELECT `+flexAuthorizationColumns+`
			FROM flex_authorizations
			WHERE id = $1
			FOR UPDATE
		`, authorizationID),
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrFlexAuthorizationNotFound
	}
	if err != nil {
		return nil, fmt.Errorf(
			"lock FLEX authorization for reconciliation: %w",
			err,
		)
	}

	if current.ProviderAuthorizationID != nil &&
		*current.ProviderAuthorizationID != providerAuthorizationID {
		return nil, ErrFlexAuthorizationProviderConflict
	}

	targetStatus := string(resultStatus)

	// Terminal records cannot be reopened or changed.
	switch current.Status {
	case "AUTHORIZED", "FAILED", "CANCELLED", "EXPIRED":
		if current.Status != targetStatus {
			return nil, ErrFlexAuthorizationResultConflict
		}

		if current.ProviderAuthorizationID == nil {
			return nil, ErrFlexAuthorizationResultConflict
		}

		return current, nil

	case "PROCESSING":
		// Valid reconciliation origin.

	default:
		return nil, ErrFlexAuthorizationResultConflict
	}

	if current.Status == targetStatus &&
		current.ProviderAuthorizationID != nil {
		return current, nil
	}

	result, err := scanFlexAuthorization(
		tx.QueryRow(ctx, `
			UPDATE flex_authorizations
			SET
				provider_authorization_id =
					COALESCE(provider_authorization_id, $2),
				status = $3::text,
				authorized_at = CASE
					WHEN $3::text = 'AUTHORIZED'
					THEN COALESCE(
						authorized_at,
						clock_timestamp()
					)
					ELSE authorized_at
				END,
				updated_at = clock_timestamp()
			WHERE id = $1
			RETURNING `+flexAuthorizationColumns,
			authorizationID,
			providerAuthorizationID,
			targetStatus,
		),
	)
	if err != nil {
		return nil, fmt.Errorf(
			"persist FLEX authorization reconciliation: %w",
			err,
		)
	}

	return result, nil
}
