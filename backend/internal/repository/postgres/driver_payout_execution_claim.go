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

		if err := verifyClaimDestination(
			ctx, tx, parsedID.String(),
		); err != nil {
			return err
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

// verifyClaimDestination checks the authoritative destination binding
// when migration 00074 is installed.
//
// The destination row is locked for the duration of the claim transaction,
// preventing a concurrent revocation from invalidating verification
// between the check and the PROCESSING transition.
//
// Before migration 00074, existing test and legacy behavior is preserved.
func verifyClaimDestination(
	ctx context.Context,
	tx pgx.Tx,
	payoutID string,
) error {
	var destinationColumnExists bool

	err := tx.QueryRow(ctx, `
        SELECT EXISTS (
            SELECT 1
            FROM information_schema.columns
            WHERE table_schema = current_schema()
              AND table_name = 'driver_payout_executions'
              AND column_name = 'destination_id'
        )
    `).Scan(&destinationColumnExists)

	if err != nil {
		return fmt.Errorf("check claim destination schema: %w", err)
	}

	if !destinationColumnExists {
		return nil
	}

	var valid bool

	err = tx.QueryRow(ctx, `
        SELECT
            d.verification_status = 'VERIFIED'
            AND d.provider = e.provider
            AND d.driver_id = p.driver_id
            AND d.company_id = p.company_id
        FROM driver_payout_executions e
        JOIN driver_payouts p
          ON p.id = e.payout_id
        JOIN driver_payout_destinations d
          ON d.id = e.destination_id
        WHERE e.payout_id = $1
        FOR SHARE OF d
    `, payoutID).Scan(&valid)

	if errors.Is(err, pgx.ErrNoRows) {
		return ErrDriverPayoutExecutionConflict
	}
	if err != nil {
		return fmt.Errorf("verify claim destination: %w", err)
	}
	if !valid {
		return ErrDriverPayoutExecutionConflict
	}

	return nil
}
