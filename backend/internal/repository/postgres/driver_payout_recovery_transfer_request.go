package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/JCKFinland/connect/backend/internal/payouts"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ResolveDriverPayoutRecoveryTransferRequest returns the immutable
// transfer request for an already-claimed, unresolved payout.
//
// It accepts PROCESSING and REQUIRES_RECONCILIATION but never
// authorizes another provider initiation.
//
// The function is read-only and does not acquire a recovery lease.
// Callers performing recovery must separately enforce lease ownership.
func ResolveDriverPayoutRecoveryTransferRequest(
	ctx context.Context,
	db *pgxpool.Pool,
	payoutID string,
) (*payouts.TransferRequest, error) {
	if ctx == nil || db == nil {
		return nil, ErrDriverPayoutExecutionInvalidInput
	}

	parsedID, err := uuid.Parse(payoutID)
	if err != nil {
		return nil, ErrDriverPayoutExecutionInvalidInput
	}

	var request payouts.TransferRequest
	var eligible bool

	err = db.QueryRow(ctx, `
        SELECT
            e.payout_id::text,
            e.idempotency_key,
            d.provider_destination_id,
            (e.amount * 100)::bigint,
            e.currency::text,
            (
                e.execution_status IN (
                    'PROCESSING',
                    'REQUIRES_RECONCILIATION'
                )
                AND e.request_started_at IS NOT NULL
                AND e.attempt_count = 1
                AND e.provider_reference IS NULL
                AND e.provider_confirmed_at IS NULL
                AND p.payout_status = 'PENDING'
                AND p.payout_method = 'BANK_TRANSFER'
                AND e.provider = d.provider
                AND e.currency = p.currency
                AND e.amount = p.net_amount
                AND e.amount > 0
                AND d.driver_id = p.driver_id
                AND d.company_id = p.company_id
            )
        FROM driver_payout_executions e
        JOIN driver_payouts p
            ON p.id = e.payout_id
        JOIN driver_payout_destinations d
            ON d.id = e.destination_id
        WHERE e.payout_id = $1
    `, parsedID).Scan(
		&request.PayoutID,
		&request.IdempotencyKey,
		&request.DestinationID,
		&request.AmountCents,
		&request.Currency,
		&eligible,
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrDriverPayoutExecutionConflict
	}

	if err != nil {
		return nil, fmt.Errorf(
			"resolve payout recovery transfer request: %w",
			err,
		)
	}

	if !eligible {
		return nil, ErrDriverPayoutExecutionConflict
	}

	if err := payouts.ValidateTransferRequest(request); err != nil {
		return nil, ErrDriverPayoutExecutionConflict
	}

	return &request, nil
}
