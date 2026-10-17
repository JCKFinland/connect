package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrFlexAuthorizationNotFound = errors.New(
		"FLEX authorization not found",
	)

	ErrFlexAuthorizationInvalidState = errors.New(
		"FLEX authorization has an invalid processing state",
	)
)

// ClaimFlexAuthorizationProcessing atomically reserves one authorization
// for provider execution.
//
// Exactly one concurrent caller can transition PENDING -> PROCESSING.
//
// claimed=false means no new provider execution should be started.
// In particular, PROCESSING must not be treated as permission to
// create another Stripe PaymentIntent.
//
// This operation performs no external payment-provider calls.
func ClaimFlexAuthorizationProcessing(
	ctx context.Context,
	db *pgxpool.Pool,
	authorizationID string,
) (*FlexAuthorization, bool, error) {
	if db == nil {
		return nil, false, errors.New(
			"FLEX authorization database is required",
		)
	}

	if _, err := uuid.Parse(authorizationID); err != nil {
		return nil, false, ErrFlexAuthorizationNotFound
	}

	var result *FlexAuthorization
	var claimed bool

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
				"lock FLEX authorization for processing: %w",
				err,
			)
		}

		switch current.Status {
		case "PENDING":
			result, err = scanFlexAuthorization(
				tx.QueryRow(ctx, `
					UPDATE flex_authorizations
					SET status = 'PROCESSING',
					    updated_at = clock_timestamp()
					WHERE id = $1
					  AND status = 'PENDING'
					  AND expires_at > clock_timestamp()
					RETURNING `+flexAuthorizationColumns,
					authorizationID,
				),
			)

			if errors.Is(err, pgx.ErrNoRows) {
				return ErrFlexAuthorizationNotEligible
			}
			if err != nil {
				return fmt.Errorf(
					"claim FLEX authorization processing: %w",
					err,
				)
			}

			claimed = true
			return nil

		case "PROCESSING":
			result = current
			claimed = false
			return nil

		case "AUTHORIZED", "FAILED", "CANCELLED", "EXPIRED":
			result = current
			claimed = false
			return nil

		default:
			return ErrFlexAuthorizationInvalidState
		}
	})

	if err != nil {
		return nil, false, err
	}

	return result, claimed, nil
}
