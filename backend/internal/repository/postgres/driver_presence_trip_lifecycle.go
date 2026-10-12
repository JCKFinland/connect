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
