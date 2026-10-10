package postgres

import (
	"context"
	"fmt"
)

// ReleaseFromTripWithTarget releases a driver from a terminal trip
// to an explicitly authorized operational state.
//
// The caller must determine regulatory eligibility before choosing
// AVAILABLE. OFFLINE is used when the driver is ineligible.
//
// This method must run on a transaction-bound repository.
func (r *DriverPresenceRepository) ReleaseFromTripWithTarget(
	ctx context.Context,
	driverUserID string,
	tripID string,
	targetStatus string,
) (bool, error) {
	if driverUserID == "" {
		return false, fmt.Errorf("driver user ID is required")
	}

	if tripID == "" {
		return false, fmt.Errorf("trip ID is required")
	}

	if targetStatus != "AVAILABLE" && targetStatus != "OFFLINE" {
		return false, fmt.Errorf(
			"invalid terminal release target: %s",
			targetStatus,
		)
	}

	const query = `
		UPDATE driver_presence AS dp
		SET
			is_online = ($3 = 'AVAILABLE'),
			availability_status = $3,
			updated_at = NOW()
		WHERE dp.driver_id = $1
		  AND dp.availability_status = 'BUSY'
		  AND EXISTS (
			SELECT 1
			FROM trips AS terminal_trip
			WHERE terminal_trip.id = $2
			  AND terminal_trip.driver_id = dp.driver_id
			  AND terminal_trip.is_active = FALSE
			  AND terminal_trip.deleted_at IS NULL
			  AND terminal_trip.status IN ('COMPLETED', 'CANCELLED')
		  )
		  AND NOT EXISTS (
			SELECT 1
			FROM trips AS active_trip
			WHERE active_trip.driver_id = dp.driver_id
			  AND active_trip.is_active = TRUE
			  AND active_trip.deleted_at IS NULL
			  AND active_trip.status NOT IN ('COMPLETED', 'CANCELLED')
		  )
	`

	result, err := r.db.Exec(
		ctx,
		query,
		driverUserID,
		tripID,
		targetStatus,
	)
	if err != nil {
		return false, fmt.Errorf(
			"release driver presence with terminal target: %w",
			err,
		)
	}

	return result.RowsAffected() == 1, nil
}
