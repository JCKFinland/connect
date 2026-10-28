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
	ErrDriverPayoutExecutionInvalidInput = errors.New(
		"invalid driver payout execution input",
	)
	ErrDriverPayoutExecutionConflict = errors.New(
		"driver payout execution conflict",
	)
)

type DriverPayoutExecution struct {
	ID             string
	PayoutID       string
	Provider       string
	IdempotencyKey string
	Amount         string
	Currency       string
	Status         string
	AttemptCount   int
}

type CreateDriverPayoutExecutionParams struct {
	PayoutID string
	Provider string
}

func CreateDriverPayoutExecution(
	ctx context.Context,
	db *pgxpool.Pool,
	params CreateDriverPayoutExecutionParams,
) (*DriverPayoutExecution, error) {
	if db == nil {
		return nil, ErrDriverPayoutExecutionInvalidInput
	}

	payoutID, err := uuid.Parse(params.PayoutID)
	if err != nil {
		return nil, ErrDriverPayoutExecutionInvalidInput
	}

	provider := strings.TrimSpace(params.Provider)

	if provider == "" ||
		len(provider) > 50 ||
		provider != strings.ToUpper(provider) {
		return nil, ErrDriverPayoutExecutionInvalidInput
	}

	key := "driver-payout-execution:" + payoutID.String()

	if len(key) > 255 {
		return nil, ErrDriverPayoutExecutionInvalidInput
	}

	var result *DriverPayoutExecution

	// Legacy execution creation is permitted only before destination
	// binding becomes part of the database schema.
	//
	// Once migration 00074 is installed, callers must use the
	// destination-aware creation path.
	var destinationColumnExists bool

	schemaErr := db.QueryRow(ctx, `
        SELECT EXISTS (
            SELECT 1
            FROM information_schema.columns
            WHERE table_schema = current_schema()
              AND table_name = 'driver_payout_executions'
              AND column_name = 'destination_id'
        )
    `).Scan(&destinationColumnExists)

	if schemaErr != nil {
		return nil, fmt.Errorf(
			"check payout destination schema: %w",
			schemaErr,
		)
	}

	if destinationColumnExists {
		return nil, ErrDriverPayoutExecutionConflict
	}

	err = RunInTransaction(ctx, db, func(tx pgx.Tx) error {
		var amount string
		var currency string
		var status string
		var method string

		err := tx.QueryRow(ctx, `
            SELECT
                net_amount::text,
                currency,
                payout_status,
                payout_method
            FROM driver_payouts
            WHERE id = $1
            FOR UPDATE
        `, payoutID).Scan(
			&amount,
			&currency,
			&status,
			&method,
		)

		if errors.Is(err, pgx.ErrNoRows) {
			return ErrDriverPayoutExecutionConflict
		}
		if err != nil {
			return fmt.Errorf(
				"lock driver payout: %w",
				err,
			)
		}

		existing, err := readDriverPayoutExecution(
			ctx,
			tx,
			payoutID.String(),
		)

		if err == nil {
			if existing.Provider != provider ||
				existing.IdempotencyKey != key ||
				existing.Amount != amount ||
				existing.Currency != currency {
				return ErrDriverPayoutExecutionConflict
			}

			result = existing
			return nil
		}

		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}

		if status != "PENDING" ||
			method != "BANK_TRANSFER" ||
			currency != "EUR" {
			return ErrDriverPayoutExecutionConflict
		}

		result = &DriverPayoutExecution{}

		err = tx.QueryRow(ctx, `
            INSERT INTO driver_payout_executions (
                payout_id,
                provider,
                idempotency_key,
                amount,
                currency,
                execution_status
            )
            VALUES (
                $1, $2, $3, $4::numeric, $5, 'PENDING'
            )
            RETURNING
                id,
                payout_id,
                provider,
                idempotency_key,
                amount::text,
                currency,
                execution_status,
                attempt_count
        `,
			payoutID,
			provider,
			key,
			amount,
			currency,
		).Scan(
			&result.ID,
			&result.PayoutID,
			&result.Provider,
			&result.IdempotencyKey,
			&result.Amount,
			&result.Currency,
			&result.Status,
			&result.AttemptCount,
		)

		if err != nil {
			return fmt.Errorf(
				"create driver payout execution: %w",
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

func readDriverPayoutExecution(
	ctx context.Context,
	tx pgx.Tx,
	payoutID string,
) (*DriverPayoutExecution, error) {
	result := &DriverPayoutExecution{}

	err := tx.QueryRow(ctx, `
        SELECT
            id,
            payout_id,
            provider,
            idempotency_key,
            amount::text,
            currency,
            execution_status,
            attempt_count
        FROM driver_payout_executions
        WHERE payout_id = $1
        FOR UPDATE
    `, payoutID).Scan(
		&result.ID,
		&result.PayoutID,
		&result.Provider,
		&result.IdempotencyKey,
		&result.Amount,
		&result.Currency,
		&result.Status,
		&result.AttemptCount,
	)

	if err != nil {
		return nil, err
	}

	return result, nil
}
