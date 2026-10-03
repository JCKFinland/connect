package postgres

import (
	"context"
	"fmt"
)

// ClaimForTrip transitions a driver's operational presence to BUSY only when
// the specified trip is an active non-terminal trip owned by that driver.
//
// The lifecycle evidence and presence mutation are checked atomically by one
// PostgreSQL statement.
func (r *DriverPresenceRepository) ClaimForTrip(
	ctx context.Context,
	driverID string,
	tripID string,
) (bool, error) {
	if driverID == "" {
		return false, fmt.Errorf("driver ID is required")
	}
	if tripID == "" {
		return false, fmt.Errorf("trip ID is required")
	}

	const query = `
		UPDATE driver_presence AS dp
		SET
			is_online = TRUE,
			availability_status = 'BUSY',
			updated_at = NOW()
		WHERE dp.driver_id = $1
		  AND dp.is_online = TRUE
		  AND dp.availability_status = 'AVAILABLE'
		  AND EXISTS (
			SELECT 1
			FROM trips AS t
			WHERE t.id = $2
			  AND t.driver_id = dp.driver_id
			  AND t.is_active = TRUE
			  AND t.deleted_at IS NULL
			  AND t.status NOT IN ('COMPLETED', 'CANCELLED')
		  )
	`

	result, err := r.db.Exec(
		ctx,
		query,
		driverID,
		tripID,
	)
	if err != nil {
		return false, fmt.Errorf("claim driver presence for trip: %w", err)
	}

	return result.RowsAffected() == 1, nil
}

// ReleaseFromTrip transitions a driver's operational presence to AVAILABLE
// only when the specified trip is terminal and inactive and no other active
// non-terminal trip still commits that driver.
//
// The exact terminal trip authorizes release; the NOT EXISTS guard prevents
// one completed/cancelled trip from releasing a driver still owned by another
// active trip.
func (r *DriverPresenceRepository) ReleaseFromTrip(
	ctx context.Context,
	driverID string,
	tripID string,
) (bool, error) {
	if driverID == "" {
		return false, fmt.Errorf("driver ID is required")
	}
	if tripID == "" {
		return false, fmt.Errorf("trip ID is required")
	}

	const query = `
		UPDATE driver_presence AS dp
		SET
			is_online = TRUE,
			availability_status = 'AVAILABLE',
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
		driverID,
		tripID,
	)
	if err != nil {
		return false, fmt.Errorf("release driver presence from trip: %w", err)
	}

	return result.RowsAffected() == 1, nil
}
