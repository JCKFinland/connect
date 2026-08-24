package postgres

import (
	"context"
	"fmt"
)

// DetachAssignmentIfIdle atomically clears assignment-related presence
// state only when the driver is not BUSY and has no active non-terminal trip.
//
// The caller should hold the driver's presence row lock when this operation
// participates in a larger lifecycle transaction.
func (r *DriverPresenceRepository) DetachAssignmentIfIdle(
	ctx context.Context,
	driverID string,
) (bool, error) {

	if driverID == "" {
		return false, fmt.Errorf(
			"driver ID is required",
		)
	}

	const query = `
		UPDATE driver_presence AS dp
		SET
			assignment_id = NULL,
			vehicle_id = NULL,
			is_online = FALSE,
			availability_status = 'OFFLINE',
			updated_at = NOW()
		WHERE dp.driver_id = $1
		  AND dp.availability_status <> 'BUSY'
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
	)
	if err != nil {
		return false, fmt.Errorf(
			"detach idle driver assignment: %w",
			err,
		)
	}

	return result.RowsAffected() == 1, nil
}
