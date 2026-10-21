package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// FlexSettlementHealthSnapshot describes outstanding FLEX settlement
// work without modifying financial state.
type FlexSettlementHealthSnapshot struct {
	ObservedAt time.Time

	PendingCaptureCount int64
	RecoveryCount       int64

	StalePendingCaptureCount int64
	StaleRecoveryCount       int64

	OldestPendingCaptureAt *time.Time
	OldestRecoveryAt       *time.Time
}

// ReadFlexSettlementHealthSnapshot returns one consistent,
// read-only database snapshot.
//
// A pending capture has not recorded a provider capture request.
// Recovery includes all unresolved processing settlements and
// settlements with durable capture-request evidence.
//
// The threshold controls only reporting. It does not trigger
// payment execution or modify settlement records.
func ReadFlexSettlementHealthSnapshot(
	ctx context.Context,
	db *pgxpool.Pool,
	staleAfter time.Duration,
) (*FlexSettlementHealthSnapshot, error) {
	if ctx == nil {
		return nil, errors.New(
			"FLEX settlement health context is required",
		)
	}

	if db == nil {
		return nil, errors.New(
			"FLEX settlement health database is required",
		)
	}

	if staleAfter <= 0 {
		return nil, errors.New(
			"FLEX settlement health stale threshold must be positive",
		)
	}

	snapshot := &FlexSettlementHealthSnapshot{}

	const query = `
        WITH observation AS (
            SELECT clock_timestamp() AS observed_at
        )
        SELECT
            observation.observed_at,

            COUNT(*) FILTER (
                WHERE fs.status = 'PENDING'
                  AND fs.capture_requested_at IS NULL
            )::bigint,

            COUNT(*) FILTER (
                WHERE fs.status IN ('PENDING', 'PROCESSING')
                  AND (
                      fs.status = 'PROCESSING'
                      OR fs.capture_requested_at IS NOT NULL
                  )
            )::bigint,

            COUNT(*) FILTER (
                WHERE fs.status = 'PENDING'
                  AND fs.capture_requested_at IS NULL
                  AND fs.created_at <=
                      observation.observed_at - $1::interval
            )::bigint,

            COUNT(*) FILTER (
                WHERE fs.status IN ('PENDING', 'PROCESSING')
                  AND (
                      fs.status = 'PROCESSING'
                      OR fs.capture_requested_at IS NOT NULL
                  )
                  AND fs.created_at <=
                      observation.observed_at - $1::interval
            )::bigint,

            MIN(fs.created_at) FILTER (
                WHERE fs.status = 'PENDING'
                  AND fs.capture_requested_at IS NULL
            ),

            MIN(fs.created_at) FILTER (
                WHERE fs.status IN ('PENDING', 'PROCESSING')
                  AND (
                      fs.status = 'PROCESSING'
                      OR fs.capture_requested_at IS NOT NULL
                  )
            )
        FROM observation
        LEFT JOIN flex_settlements fs ON TRUE
        GROUP BY observation.observed_at
    `

	err := db.QueryRow(
		ctx,
		query,
		staleAfter.String(),
	).Scan(
		&snapshot.ObservedAt,
		&snapshot.PendingCaptureCount,
		&snapshot.RecoveryCount,
		&snapshot.StalePendingCaptureCount,
		&snapshot.StaleRecoveryCount,
		&snapshot.OldestPendingCaptureAt,
		&snapshot.OldestRecoveryAt,
	)

	if err != nil {
		return nil, fmt.Errorf(
			"read FLEX settlement health snapshot: %w",
			err,
		)
	}

	return snapshot, nil
}
