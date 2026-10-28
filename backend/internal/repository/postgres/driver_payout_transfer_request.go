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

// ResolveDriverPayoutTransferRequest constructs an authoritative,
// read-only provider request from an existing bound execution.
//
// This function never claims an execution, contacts a provider,
// or authorizes another submission.
//
// A caller must separately possess a successful, newly acquired
// execution claim before initiating a provider transfer.
func ResolveDriverPayoutTransferRequest(
	ctx context.Context,
	db *pgxpool.Pool,
	payoutID string,
) (*payouts.TransferRequest, error) {
	if db == nil {
		return nil, ErrDriverPayoutExecutionInvalidInput
	}

	parsedID, err := uuid.Parse(payoutID)
	if err != nil {
		return nil, ErrDriverPayoutExecutionInvalidInput
	}

	var req payouts.TransferRequest
	var valid bool

	err = db.QueryRow(ctx, `
        SELECT
            e.payout_id::text,
            e.idempotency_key,
            d.provider_destination_id,
            (e.amount * 100)::bigint,
            e.currency::text,
            (
                e.execution_status = 'PROCESSING'
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
		&req.PayoutID,
		&req.IdempotencyKey,
		&req.DestinationID,
		&req.AmountCents,
		&req.Currency,
		&valid,
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrDriverPayoutExecutionConflict
	}
	if err != nil {
		return nil, fmt.Errorf(
			"resolve driver payout transfer request: %w",
			err,
		)
	}

	if !valid {
		return nil, ErrDriverPayoutExecutionConflict
	}

	if err := payouts.ValidateTransferRequest(req); err != nil {
		return nil, ErrDriverPayoutExecutionConflict
	}

	return &req, nil
}
