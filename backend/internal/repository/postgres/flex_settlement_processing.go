package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrFlexSettlementInvalidState = errors.New(
	"FLEX settlement has an invalid state",
)

// ClaimFlexSettlementProcessing atomically claims a PENDING
// settlement for processing.
//
// Exactly one concurrent caller can transition PENDING to PROCESSING.
//
// This operation performs no external payment-provider action.
// A PROCESSING settlement must not be blindly reclaimed or
// recaptured without reconciling the existing provider intent.
func ClaimFlexSettlementProcessing(
	ctx context.Context,
	db *pgxpool.Pool,
	settlementID string,
) (*FlexSettlement, bool, error) {
	if db == nil {
		return nil, false, errors.New(
			"FLEX settlement database is required",
		)
	}

	if _, err := uuid.Parse(settlementID); err != nil {
		return nil, false, ErrFlexSettlementNotFound
	}

	var result *FlexSettlement
	var claimed bool

	err := RunInTransaction(ctx, db, func(tx pgx.Tx) error {
		current, err := scanFlexSettlement(
			tx.QueryRow(ctx, `
				SELECT `+flexSettlementColumns+`
				FROM flex_settlements
				WHERE id = $1
				FOR UPDATE
			`, settlementID),
		)

		if errors.Is(err, pgx.ErrNoRows) {
			return ErrFlexSettlementNotFound
		}
		if err != nil {
			return fmt.Errorf(
				"lock FLEX settlement for processing: %w",
				err,
			)
		}

		switch current.Status {
		case "PENDING":
			result, err = scanFlexSettlement(
				tx.QueryRow(ctx, `
					UPDATE flex_settlements
					SET status = 'PROCESSING',
						attempt_count = attempt_count + 1,
						updated_at = clock_timestamp()
					WHERE id = $1
					  AND status = 'PENDING'
					RETURNING `+flexSettlementColumns,
					settlementID,
				),
			)

			if errors.Is(err, pgx.ErrNoRows) {
				return ErrFlexSettlementInvalidState
			}
			if err != nil {
				return fmt.Errorf(
					"claim FLEX settlement processing: %w",
					err,
				)
			}

			claimed = true
			return nil

		case "PROCESSING", "CAPTURED", "CANCELLED":
			result = current
			claimed = false
			return nil

		default:
			return ErrFlexSettlementInvalidState
		}
	})

	if err != nil {
		return nil, false, err
	}

	return result, claimed, nil
}
