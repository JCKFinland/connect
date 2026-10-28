package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrDriverPayoutRecoveryIdentityMismatch = errors.New(
	"driver payout recovery identity mismatch",
)

// ValidateDriverPayoutRecoveryIdentity checks a discovered execution
// against the current authoritative database state.
//
// It is strictly read-only. It does not acquire an execution claim,
// initiate a transfer, reconcile an outcome, or finalize a payout.
//
// The caller must independently hold a valid recovery lease.
func ValidateDriverPayoutRecoveryIdentity(
	ctx context.Context,
	db *pgxpool.Pool,
	candidate StaleDriverPayoutExecution,
) error {
	if ctx == nil || db == nil ||
		uuid.Validate(candidate.PayoutID) != nil {
		return ErrDriverPayoutRecoveryIdentityMismatch
	}

	var current StaleDriverPayoutExecution
	var eligible bool

	err := db.QueryRow(ctx, `
		SELECT
			e.payout_id::text,
			e.provider,
			e.idempotency_key,
			d.provider_destination_id,
			(e.amount * 100)::bigint,
			e.currency::text,
			e.execution_status,
			e.attempt_count,
			e.request_started_at,
			(
				p.payout_status = 'PENDING'
				AND p.payout_method = 'BANK_TRANSFER'
				AND e.execution_status IN (
					'PROCESSING',
					'REQUIRES_RECONCILIATION'
				)
				AND e.attempt_count = 1
				AND e.request_started_at IS NOT NULL
				AND e.provider_reference IS NULL
				AND e.provider_confirmed_at IS NULL
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
		WHERE e.payout_id = $1::uuid
	`, candidate.PayoutID).Scan(
		&current.PayoutID,
		&current.Provider,
		&current.IdempotencyKey,
		&current.DestinationID,
		&current.AmountCents,
		&current.Currency,
		&current.Status,
		&current.AttemptCount,
		&current.RequestStartedAt,
		&eligible,
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return ErrDriverPayoutRecoveryIdentityMismatch
	}

	if err != nil {
		return fmt.Errorf(
			"read authoritative payout recovery identity: %w",
			err,
		)
	}

	if !eligible ||
		candidate.PayoutID != current.PayoutID ||
		candidate.Provider != current.Provider ||
		candidate.IdempotencyKey != current.IdempotencyKey ||
		candidate.DestinationID != current.DestinationID ||
		candidate.AmountCents != current.AmountCents ||
		candidate.Currency != current.Currency ||
		candidate.Status != current.Status ||
		candidate.AttemptCount != current.AttemptCount ||
		!candidate.RequestStartedAt.Equal(
			current.RequestStartedAt,
		) {
		return ErrDriverPayoutRecoveryIdentityMismatch
	}

	return nil
}
