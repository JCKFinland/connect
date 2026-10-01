package postgres

import (
	"context"
	"fmt"
	"time"
)

// ExpireStaleIdle atomically moves stale idle online presence to OFFLINE.
//
// Heartbeat freshness, idle state, and active-trip protection are evaluated by
// the same PostgreSQL UPDATE that performs the transition. A heartbeat that
// becomes fresh before this statement obtains the row prevents expiration.
func (r *DriverPresenceRepository) ExpireStaleIdle(
	ctx context.Context,
	driverID string,
	staleBefore time.Time,
) (bool, error) {
	if driverID == "" {
		return false, fmt.Errorf("driver ID is required")
	}

	if staleBefore.IsZero() {
		return false, fmt.Errorf("stale-before time is required")
	}

	const query = `
		UPDATE driver_presence AS dp
		SET
			is_online = FALSE,
			availability_status = 'OFFLINE',
			updated_at = NOW()
		WHERE dp.driver_id = $1
		  AND dp.is_online = TRUE
		  AND dp.availability_status IN ('AVAILABLE', 'BREAK')
		  AND (
			dp.last_heartbeat_at IS NULL
			OR dp.last_heartbeat_at < $2
		  )
		  AND NOT EXISTS (
			SELECT 1
			FROM trips AS t
			WHERE t.driver_id = dp.driver_id
			  AND t.is_active = TRUE
			  AND t.deleted_at IS NULL
			  AND t.status NOT IN (
			        'COMPLETED',
			        'CANCELLED'
			  )
		  )
	`

	result, err := r.db.Exec(
		ctx,
		query,
		driverID,
		staleBefore.UTC(),
	)
	if err != nil {
		return false, fmt.Errorf("expire stale idle driver presence: %w", err)
	}

	return result.RowsAffected() == 1, nil
}

// ExpireAllStaleIdle atomically moves all stale idle online presence to
// OFFLINE.
//
// Heartbeat freshness, idle state, and active-trip protection are evaluated by
// the same PostgreSQL UPDATE that performs each transition.
func (r *DriverPresenceRepository) ExpireAllStaleIdle(
	ctx context.Context,
	staleBefore time.Time,
) (int64, error) {
	if staleBefore.IsZero() {
		return 0, fmt.Errorf("stale-before time is required")
	}

	const query = `
		UPDATE driver_presence AS dp
		SET
			is_online = FALSE,
			availability_status = 'OFFLINE',
			updated_at = NOW()
		WHERE dp.is_online = TRUE
		  AND dp.availability_status IN ('AVAILABLE', 'BREAK')
		  AND (
			dp.last_heartbeat_at IS NULL
			OR dp.last_heartbeat_at < $1
		  )
		  AND NOT EXISTS (
			SELECT 1
			FROM trips AS t
			WHERE t.driver_id = dp.driver_id
			  AND t.is_active = TRUE
			  AND t.deleted_at IS NULL
			  AND t.status NOT IN (
			        'COMPLETED',
			        'CANCELLED'
			  )
		  )
	`

	result, err := r.db.Exec(
		ctx,
		query,
		staleBefore.UTC(),
	)
	if err != nil {
		return 0, fmt.Errorf(
			"expire all stale idle driver presence: %w",
			err,
		)
	}

	return result.RowsAffected(), nil
}
