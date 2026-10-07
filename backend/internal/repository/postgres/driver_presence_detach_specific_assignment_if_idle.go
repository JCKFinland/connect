package postgres

import (
	"context"
	"fmt"
)

// DetachSpecificAssignmentIfIdle atomically clears assignment-related presence
// state only when the expected assignment is still attached, the driver is not
// BUSY, and there is no active non-terminal trip.
func (r *DriverPresenceRepository) DetachSpecificAssignmentIfIdle(
	ctx context.Context,
	driverID string,
	assignmentID string,
) (bool, error) {

	if driverID == "" {
		return false, fmt.Errorf(
			"driver ID is required",
		)
	}

	if assignmentID == "" {
		return false, fmt.Errorf(
			"assignment ID is required",
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
		  AND dp.assignment_id = $2
		  AND dp.availability_status <> 'BUSY'
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
		assignmentID,
	)
	if err != nil {
		return false, fmt.Errorf(
			"detach specific idle driver assignment: %w",
			err,
		)
	}

	return result.RowsAffected() == 1, nil
}
