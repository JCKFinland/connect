package postgres

import (
	"context"
	"fmt"
)

// UpdateAvailabilityIfIdle updates manually controlled availability only
// when the driver is not BUSY and has no active non-terminal trip.
//
// The guard and mutation are performed by one PostgreSQL UPDATE statement.
// This prevents a manual availability request from overwriting BUSY after
// a concurrent offer-acceptance transaction commits.
func (r *DriverPresenceRepository) UpdateAvailabilityIfIdle(
	ctx context.Context,
	driverID string,
	status string,
	isOnline bool,
) (bool, error) {

	if driverID == "" {
		return false, fmt.Errorf(
			"driver ID is required",
		)
	}

	const query = `
		UPDATE driver_presence AS dp
		SET
			is_online = $2,
			availability_status = $3,
			updated_at = NOW()
		WHERE dp.driver_id = $1

		  -- BUSY is controlled by trip/dispatch lifecycle code.
		  -- A manual presence mutation must never overwrite it.
		  AND dp.availability_status <> 'BUSY'

		  -- The Trip is the authoritative operational commitment.
		  AND NOT EXISTS (
			SELECT 1
			FROM trips AS t
			WHERE t.driver_id = dp.driver_id
			  AND t.is_active = TRUE
			  AND t.deleted_at IS NULL
			  AND t.status NOT IN (
				'COMPLETED',
				'CANCELLED',
				'NO_DRIVER_AVAILABLE',
				'EXPIRED'
			  )
		  )
	`

	result, err := r.db.Exec(
		ctx,
		query,
		driverID,
		isOnline,
		status,
	)
	if err != nil {
		return false, fmt.Errorf(
			"update idle driver availability: %w",
			err,
		)
	}

	return result.RowsAffected() == 1, nil
}
