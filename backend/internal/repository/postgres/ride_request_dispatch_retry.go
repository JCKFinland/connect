package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/JCKFinland/connect/backend/internal/repository"
	"github.com/jackc/pgx/v5"
)

// ScheduleDispatchRetry records a failed automatic dispatch attempt and
// calculates the next eligible retry time.
//
// Backoff policy:
//
//	1st failure -> 2 seconds
//	2nd failure -> 5 seconds
//	3rd failure -> 10 seconds
//	4th failure -> 20 seconds
//	5th+        -> 30 seconds
//
// The increment and next-attempt calculation are performed atomically
// by PostgreSQL.
func (r *RideRequestRepository) ScheduleDispatchRetry(
	ctx context.Context,
	rideRequestID string,
	attemptedAt time.Time,
) (int, time.Time, error) {

	if rideRequestID == "" {
		return 0, time.Time{}, fmt.Errorf(
			"ride request ID is required",
		)
	}

	const query = `
		WITH params AS (
			SELECT
				$1::uuid AS ride_request_id,
				$2::timestamptz AS attempted_at
		)
		UPDATE ride_requests AS rr
		SET
			dispatch_retry_count =
				rr.dispatch_retry_count + 1,

			last_dispatch_attempt_at =
				params.attempted_at,

			next_dispatch_attempt_at =
				params.attempted_at +
				CASE rr.dispatch_retry_count
					WHEN 0 THEN INTERVAL '2 seconds'
					WHEN 1 THEN INTERVAL '5 seconds'
					WHEN 2 THEN INTERVAL '10 seconds'
					WHEN 3 THEN INTERVAL '20 seconds'
					ELSE INTERVAL '30 seconds'
				END,

			updated_at = NOW()

		FROM params

		WHERE rr.id = params.ride_request_id
		  AND rr.status = 'PENDING'

		RETURNING
			rr.dispatch_retry_count,
			rr.next_dispatch_attempt_at
	`

	var (
		retryCount    int
		nextAttemptAt time.Time
	)

	err := r.db.QueryRow(
		ctx,
		query,
		rideRequestID,
		attemptedAt,
	).Scan(
		&retryCount,
		&nextAttemptAt,
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return 0, time.Time{}, repository.ErrNotFound
	}

	if err != nil {
		return 0, time.Time{}, fmt.Errorf(
			"schedule ride dispatch retry: %w",
			err,
		)
	}

	return retryCount, nextAttemptAt, nil
}

// ResetDispatchRetry clears automatic redispatch backoff state.
//
// This should happen after CONNECT successfully creates a new dispatch
// offer for the ride. The reset remains part of the same dispatch
// transaction, so it is rolled back if offer creation fails.
func (r *RideRequestRepository) ResetDispatchRetry(
	ctx context.Context,
	rideRequestID string,
) error {

	if rideRequestID == "" {
		return fmt.Errorf(
			"ride request ID is required",
		)
	}

	const query = `
		UPDATE ride_requests
		SET
			dispatch_retry_count = 0,
			next_dispatch_attempt_at = NULL,
			last_dispatch_attempt_at = NULL,
			updated_at = NOW()
		WHERE id = $1
	`

	result, err := r.db.Exec(
		ctx,
		query,
		rideRequestID,
	)
	if err != nil {
		return fmt.Errorf(
			"reset ride dispatch retry: %w",
			err,
		)
	}

	if result.RowsAffected() == 0 {
		return repository.ErrNotFound
	}

	return nil
}
