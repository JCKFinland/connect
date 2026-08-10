package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// AssignDriver assigns a driver and vehicle to a trip.
func (r *TripRepository) AssignDriver(
	ctx context.Context,
	id string,
	driverID string,
	vehicleID string,
) error {
	const query = `
		UPDATE trips
		SET
			driver_id = $1,
			vehicle_id = $2,
			status = 'ASSIGNED',
			assigned_at = NOW(),
			updated_at = NOW()
		WHERE id = $3
		  AND deleted_at IS NULL
		  AND is_active = TRUE
	`

	result, err := r.db.Exec(
		ctx,
		query,
		driverID,
		vehicleID,
		id,
	)
	if err != nil {
		return fmt.Errorf("assign driver and vehicle to trip: %w", err)
	}

	if result.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}

	return nil
}
