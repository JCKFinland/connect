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

// MarkDriverPayoutRequiresReconciliation records an uncertain
// provider outcome. It never authorizes another provider request.
//
// This function must only be called after the caller has established
// that the previously initiated request has an unknown outcome.
// It does not contact the provider or move money.
func MarkDriverPayoutRequiresReconciliation(
	ctx context.Context,
	db *pgxpool.Pool,
	payoutID string,
	reason string,
) (*DriverPayoutExecution, error) {
	if db == nil {
		return nil, ErrDriverPayoutExecutionInvalidInput
	}

	parsedID, err := uuid.Parse(payoutID)
	if err != nil {
		return nil, ErrDriverPayoutExecutionInvalidInput
	}

	reason = strings.TrimSpace(reason)
	if reason == "" || len(reason) > 2000 {
		return nil, ErrDriverPayoutExecutionInvalidInput
	}

	var result *DriverPayoutExecution

	err = RunInTransaction(ctx, db, func(tx pgx.Tx) error {
		var payoutStatus, payoutMethod string

		err := tx.QueryRow(ctx, `
            SELECT payout_status, payout_method
            FROM driver_payouts
            WHERE id = $1
            FOR UPDATE
        `, parsedID).Scan(&payoutStatus, &payoutMethod)

		if errors.Is(err, pgx.ErrNoRows) {
			return ErrDriverPayoutExecutionConflict
		}
		if err != nil {
			return fmt.Errorf("lock payout: %w", err)
		}

		current, err := readDriverPayoutExecution(
			ctx, tx, parsedID.String(),
		)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrDriverPayoutExecutionConflict
		}
		if err != nil {
			return fmt.Errorf("read execution: %w", err)
		}

		if payoutStatus != "PENDING" ||
			payoutMethod != "BANK_TRANSFER" {
			return ErrDriverPayoutExecutionConflict
		}

		if current.Status == "REQUIRES_RECONCILIATION" {
			result = current
			return nil
		}

		if current.Status != "PROCESSING" ||
			current.AttemptCount != 1 {
			return ErrDriverPayoutExecutionConflict
		}

		result = &DriverPayoutExecution{}

		err = tx.QueryRow(ctx, `
            UPDATE driver_payout_executions
            SET
                execution_status = 'REQUIRES_RECONCILIATION',
                last_error = $2,
                updated_at = NOW()
            WHERE payout_id = $1
              AND execution_status = 'PROCESSING'
              AND request_started_at IS NOT NULL
              AND attempt_count = 1
              AND provider_confirmed_at IS NULL
              AND provider_reference IS NULL
            RETURNING
                id,
                payout_id,
                provider,
                idempotency_key,
                amount::text,
                currency,
                execution_status,
                attempt_count
        `, parsedID, reason).Scan(
			&result.ID,
			&result.PayoutID,
			&result.Provider,
			&result.IdempotencyKey,
			&result.Amount,
			&result.Currency,
			&result.Status,
			&result.AttemptCount,
		)

		if errors.Is(err, pgx.ErrNoRows) {
			return ErrDriverPayoutExecutionConflict
		}
		if err != nil {
			return fmt.Errorf("record unknown payout outcome: %w", err)
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	return result, nil
}
