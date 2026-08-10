package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// UpdateStatus changes the lifecycle state of an active trip.
//
// Lifecycle timestamps are maintained atomically with the status change:
//   - DRIVER_ARRIVED -> driver_arrived_at
//   - IN_PROGRESS     -> started_at
//   - COMPLETED       -> completed_at + is_active=false
//   - CANCELLED       -> cancelled_at + is_active=false
func (r *TripRepository) UpdateStatus(
	ctx context.Context,
	id string,
	status string,
) error {
	const query = `
		UPDATE trips
		SET
			status = $1::varchar,

			driver_arrived_at = CASE
				WHEN $1::varchar = 'DRIVER_ARRIVED'
					THEN COALESCE(driver_arrived_at, NOW())
				ELSE driver_arrived_at
			END,

			started_at = CASE
				WHEN $1::varchar = 'IN_PROGRESS'
					THEN COALESCE(started_at, NOW())
				ELSE started_at
			END,

			completed_at = CASE
				WHEN $1::varchar = 'COMPLETED'
					THEN COALESCE(completed_at, NOW())
				ELSE completed_at
			END,

			cancelled_at = CASE
				WHEN $1::varchar = 'CANCELLED'
					THEN COALESCE(cancelled_at, NOW())
				ELSE cancelled_at
			END,

			is_active = CASE
				WHEN $1::varchar IN ('COMPLETED', 'CANCELLED')
					THEN false
				ELSE is_active
			END,

			updated_at = NOW()

		WHERE id = $2
		  AND deleted_at IS NULL
	`

	result, err := r.db.Exec(
		ctx,
		query,
		status,
		id,
	)
	if err != nil {
		return fmt.Errorf("update trip status: %w", err)
	}

	if result.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}

	return nil
}
