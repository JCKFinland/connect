package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ClaimDriverPayoutExecution atomically records that a payout request
// may be submitted to its provider.
//
// Only a successful PENDING -> PROCESSING transition returns claimed=true.
//
// PROCESSING is never permission to submit another provider request.
// An uncertain provider outcome requires reconciliation, not resubmission.
//
// This function performs database operations only. It never contacts
// an external payment or payout provider.
func ClaimDriverPayoutExecution(
	ctx context.Context,
	db *pgxpool.Pool,
	payoutID string,
) (*DriverPayoutExecution, bool, error) {
	if db == nil {
		return nil, false, ErrDriverPayoutExecutionInvalidInput
	}

	parsedID, err := uuid.Parse(payoutID)
	if err != nil {
		return nil, false, ErrDriverPayoutExecutionInvalidInput
	}

	var result *DriverPayoutExecution
	var claimed bool

	err = RunInTransaction(ctx, db, func(tx pgx.Tx) error {
		var payoutStatus string
		var payoutMethod string

		err := tx.QueryRow(ctx, `
            SELECT payout_status, payout_method
            FROM driver_payouts
            WHERE id = $1
            FOR UPDATE
        `, parsedID).Scan(
			&payoutStatus,
			&payoutMethod,
		)

		if errors.Is(err, pgx.ErrNoRows) {
			return ErrDriverPayoutExecutionConflict
		}
		if err != nil {
			return fmt.Errorf("lock driver payout: %w", err)
		}

		current, err := readDriverPayoutExecution(
			ctx,
			tx,
			parsedID.String(),
		)

		if errors.Is(err, pgx.ErrNoRows) {
			return ErrDriverPayoutExecutionConflict
		}
		if err != nil {
			return fmt.Errorf("read payout execution: %w", err)
		}

		result = current

		if current.Status != "PENDING" {
			return nil
		}

		if payoutStatus != "PENDING" ||
			payoutMethod != "BANK_TRANSFER" {
			return ErrDriverPayoutExecutionConflict
		}

		updated := &DriverPayoutExecution{}

		err = tx.QueryRow(ctx, `
            UPDATE driver_payout_executions
            SET
                execution_status = 'PROCESSING',
                request_started_at = NOW(),
                attempt_count = 1,
                updated_at = NOW()
            WHERE payout_id = $1
              AND execution_status = 'PENDING'
              AND request_started_at IS NULL
              AND attempt_count = 0
            RETURNING
                id,
                payout_id,
                provider,
                idempotency_key,
                amount::text,
                currency,
                execution_status,
                attempt_count
        `, parsedID).Scan(
			&updated.ID,
			&updated.PayoutID,
			&updated.Provider,
			&updated.IdempotencyKey,
			&updated.Amount,
			&updated.Currency,
			&updated.Status,
			&updated.AttemptCount,
		)

		if errors.Is(err, pgx.ErrNoRows) {
			return ErrDriverPayoutExecutionConflict
		}
		if err != nil {
			return fmt.Errorf("claim payout execution: %w", err)
		}

		result = updated
		claimed = true

		return nil
	})

	if err != nil {
		return nil, false, err
	}

	return result, claimed, nil
}
