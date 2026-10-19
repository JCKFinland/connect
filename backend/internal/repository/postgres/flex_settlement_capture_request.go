package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrFlexCaptureAlreadyRequested = errors.New(
	"FLEX capture request has already been recorded",
)

// MarkFlexSettlementCaptureRequested durably records permission
// for the first provider capture request.
//
// The caller must persist this marker before contacting Stripe.
//
// An existing marker is never reset or reused to authorize another
// provider request, including after timeouts or process crashes.
//
// This operation performs no external provider action.
func MarkFlexSettlementCaptureRequested(
	ctx context.Context,
	db *pgxpool.Pool,
	settlementID string,
) (*FlexSettlement, error) {
	if db == nil {
		return nil, errors.New(
			"FLEX settlement database is required",
		)
	}

	if _, err := uuid.Parse(settlementID); err != nil {
		return nil, ErrFlexSettlementNotFound
	}

	var result *FlexSettlement

	err := RunInTransaction(ctx, db, func(tx pgx.Tx) error {
		var status string
		var captureRequestedAt *string
		var paymentID, transactionID *string

		err := tx.QueryRow(ctx, `
			SELECT
				status,
				capture_requested_at::text,
				payment_id::text,
				capture_transaction_id::text
			FROM flex_settlements
			WHERE id = $1
			FOR UPDATE
		`, settlementID).Scan(
			&status,
			&captureRequestedAt,
			&paymentID,
			&transactionID,
		)

		if errors.Is(err, pgx.ErrNoRows) {
			return ErrFlexSettlementNotFound
		}
		if err != nil {
			return fmt.Errorf(
				"lock FLEX capture request: %w",
				err,
			)
		}

		if captureRequestedAt != nil {
			return ErrFlexCaptureAlreadyRequested
		}

		if status != "PROCESSING" ||
			paymentID == nil ||
			transactionID == nil {
			return ErrFlexSettlementInvalidState
		}

		result, err = scanFlexSettlement(
			tx.QueryRow(ctx, `
				UPDATE flex_settlements
				SET capture_requested_at = clock_timestamp(),
				    updated_at = clock_timestamp()
				WHERE id = $1
				  AND status = 'PROCESSING'
				  AND capture_requested_at IS NULL
				  AND payment_id IS NOT NULL
				  AND capture_transaction_id IS NOT NULL
				RETURNING `+flexSettlementColumns,
				settlementID,
			),
		)

		if errors.Is(err, pgx.ErrNoRows) {
			return ErrFlexSettlementInvalidState
		}
		if err != nil {
			return fmt.Errorf(
				"record FLEX capture request: %w",
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
