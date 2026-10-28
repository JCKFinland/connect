package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type CreateBoundDriverPayoutExecutionParams struct {
	PayoutID      string
	Provider      string
	DestinationID string
}

func CreateBoundDriverPayoutExecution(
	ctx context.Context,
	db *pgxpool.Pool,
	params CreateBoundDriverPayoutExecutionParams,
) (*DriverPayoutExecution, error) {
	if db == nil {
		return nil, ErrDriverPayoutExecutionInvalidInput
	}

	payoutID, err := uuid.Parse(params.PayoutID)
	if err != nil {
		return nil, ErrDriverPayoutExecutionInvalidInput
	}

	destinationID, err := uuid.Parse(params.DestinationID)
	if err != nil {
		return nil, ErrDriverPayoutDestinationInvalidInput
	}

	if err := ValidateDriverPayoutDestinationBinding(
		DriverPayoutDestinationBinding{
			DestinationID: destinationID.String(),
			Provider:      params.Provider,
		},
	); err != nil {
		return nil, err
	}

	key := "driver-payout-execution:" + payoutID.String()

	var result *DriverPayoutExecution

	err = RunInTransaction(ctx, db, func(tx pgx.Tx) error {
		var driverID, companyID string
		var amount, currency, status, method string

		err := tx.QueryRow(ctx, `
            SELECT driver_id, company_id,
                   net_amount::text, currency,
                   payout_status, payout_method
            FROM driver_payouts
            WHERE id = $1
            FOR UPDATE
        `, payoutID).Scan(
			&driverID, &companyID,
			&amount, &currency, &status, &method,
		)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrDriverPayoutExecutionConflict
		}
		if err != nil {
			return fmt.Errorf("lock payout: %w", err)
		}

		var existingID string
		var existingDestination *string
		var existingProvider, existingKey string
		var existingAmount, existingCurrency, existingStatus string
		var existingAttempts int

		err = tx.QueryRow(ctx, `
            SELECT id, destination_id::text,
                   provider, idempotency_key,
                   amount::text, currency,
                   execution_status, attempt_count
            FROM driver_payout_executions
            WHERE payout_id = $1
            FOR UPDATE
        `, payoutID).Scan(
			&existingID, &existingDestination,
			&existingProvider, &existingKey,
			&existingAmount, &existingCurrency,
			&existingStatus, &existingAttempts,
		)

		if err == nil {
			if existingDestination == nil ||
				*existingDestination != destinationID.String() ||
				existingProvider != params.Provider ||
				existingKey != key ||
				existingAmount != amount ||
				existingCurrency != currency {
				return ErrDriverPayoutExecutionConflict
			}

			result = &DriverPayoutExecution{
				ID:             existingID,
				PayoutID:       payoutID.String(),
				Provider:       existingProvider,
				IdempotencyKey: existingKey,
				Amount:         existingAmount,
				Currency:       existingCurrency,
				Status:         existingStatus,
				AttemptCount:   existingAttempts,
			}
			return nil
		}

		if !errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("read existing execution: %w", err)
		}

		if status != "PENDING" ||
			method != "BANK_TRANSFER" ||
			currency != "EUR" {
			return ErrDriverPayoutExecutionConflict
		}

		var destinationDriver, destinationCompany string
		var destinationProvider, verificationStatus string

		err = tx.QueryRow(ctx, `
            SELECT driver_id, company_id,
                   provider, verification_status
            FROM driver_payout_destinations
            WHERE id = $1
            FOR SHARE
        `, destinationID).Scan(
			&destinationDriver, &destinationCompany,
			&destinationProvider, &verificationStatus,
		)

		if errors.Is(err, pgx.ErrNoRows) {
			return ErrDriverPayoutExecutionConflict
		}
		if err != nil {
			return fmt.Errorf("lock destination: %w", err)
		}

		if destinationDriver != driverID ||
			destinationCompany != companyID ||
			destinationProvider != params.Provider ||
			verificationStatus != "VERIFIED" {
			return ErrDriverPayoutExecutionConflict
		}

		result = &DriverPayoutExecution{}

		err = tx.QueryRow(ctx, `
            INSERT INTO driver_payout_executions (
                payout_id, destination_id,
                provider, idempotency_key,
                amount, currency, execution_status
            )
            VALUES ($1, $2, $3, $4, $5::numeric, $6, 'PENDING')
            RETURNING id, payout_id,
                      provider, idempotency_key,
                      amount::text, currency,
                      execution_status, attempt_count
        `,
			payoutID, destinationID,
			params.Provider, key, amount, currency,
		).Scan(
			&result.ID, &result.PayoutID,
			&result.Provider, &result.IdempotencyKey,
			&result.Amount, &result.Currency,
			&result.Status, &result.AttemptCount,
		)

		if err != nil {
			return fmt.Errorf("create bound execution: %w", err)
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	return result, nil
}
