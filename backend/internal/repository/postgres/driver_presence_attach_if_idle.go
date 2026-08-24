package postgres

import (
	"context"
	"fmt"
)

// AttachAssignmentIfIdle attaches assignment/vehicle presence state only
// when the driver is not BUSY and has no active non-terminal trip.
//
// The driver's online/offline availability state is deliberately preserved.
// Vehicle assignment may legitimately occur before the driver goes online.
func (r *DriverPresenceRepository) AttachAssignmentIfIdle(
	ctx context.Context,
	driverID string,
	companyID string,
	branchID string,
	vehicleID string,
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

	if vehicleID == "" {
		return false, fmt.Errorf(
			"vehicle ID is required",
		)
	}

	const query = `
		UPDATE driver_presence AS dp
		SET
			company_id = $2,
			branch_id = $3,
			vehicle_id = $4,
			assignment_id = $5,
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
		companyID,
		branchID,
		vehicleID,
		assignmentID,
	)
	if err != nil {
		return false, fmt.Errorf(
			"attach idle driver assignment: %w",
			err,
		)
	}

	return result.RowsAffected() == 1, nil
}
