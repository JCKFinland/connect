package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// StaleDriverPayoutExecution identifies a previously claimed payout
// that requires investigation.
//
// Discovery never authorizes another provider initiation.
type StaleDriverPayoutExecution struct {
	PayoutID         string
	Provider         string
	IdempotencyKey   string
	DestinationID    string
	AmountCents      int64
	Currency         string
	Status           string
	AttemptCount     int
	RequestStartedAt time.Time
}

// DiscoverStaleDriverPayoutExecutions returns claimed payouts whose
// submission outcome requires investigation.
//
// This function is read-only. It does not claim, retry, reconcile,
// finalize or otherwise mutate any payout.
func DiscoverStaleDriverPayoutExecutions(
	ctx context.Context,
	db *pgxpool.Pool,
	olderThan time.Time,
	limit int,
) ([]StaleDriverPayoutExecution, error) {
	if db == nil || olderThan.IsZero() ||
		limit < 1 || limit > 500 {
		return nil, ErrDriverPayoutExecutionInvalidInput
	}

	rows, err := db.Query(ctx, `
        SELECT
            e.payout_id::text,
            e.provider,
            e.idempotency_key,
            d.provider_destination_id,
            (e.amount * 100)::bigint,
            e.currency::text,
            e.execution_status,
            e.attempt_count,
            e.request_started_at
        FROM driver_payout_executions e
        JOIN driver_payouts p
          ON p.id = e.payout_id
        JOIN driver_payout_destinations d
          ON d.id = e.destination_id
        WHERE e.execution_status IN (
            'PROCESSING',
            'REQUIRES_RECONCILIATION'
        )
          AND e.request_started_at <= $1
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
        ORDER BY
            e.request_started_at ASC,
            e.payout_id ASC
        LIMIT $2
    `, olderThan, limit)

	if err != nil {
		return nil, fmt.Errorf(
			"discover stale driver payout executions: %w",
			err,
		)
	}
	defer rows.Close()

	results := make([]StaleDriverPayoutExecution, 0)

	for rows.Next() {
		var item StaleDriverPayoutExecution

		if err := rows.Scan(
			&item.PayoutID,
			&item.Provider,
			&item.IdempotencyKey,
			&item.DestinationID,
			&item.AmountCents,
			&item.Currency,
			&item.Status,
			&item.AttemptCount,
			&item.RequestStartedAt,
		); err != nil {
			return nil, fmt.Errorf(
				"scan stale driver payout execution: %w",
				err,
			)
		}

		results = append(results, item)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf(
			"iterate stale driver payout executions: %w",
			err,
		)
	}

	return results, nil
}
