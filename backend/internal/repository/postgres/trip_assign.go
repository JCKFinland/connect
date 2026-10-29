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
		  AND status = 'ASSIGNED'
		  AND EXISTS (
		      SELECT 1
		      FROM drivers AS d
		      JOIN vehicles AS v
		        ON v.id = $2
		       AND v.company_id = d.company_id
		       AND v.branch_id = d.branch_id
		       AND v.is_active = TRUE
		       AND v.deleted_at IS NULL
		      JOIN driver_assignments AS da
		        ON da.driver_id = d.user_id
		       AND da.vehicle_id = v.id
		       AND da.company_id = d.company_id
		       AND da.branch_id = d.branch_id
		       AND da.fleet_id = v.fleet_id
		       AND da.unassigned_at IS NULL
		      WHERE d.user_id = $1
		        AND d.company_id = trips.company_id
		        AND d.branch_id = trips.branch_id
		        AND v.fleet_id = trips.fleet_id
		        AND d.is_active = TRUE
		        AND d.is_verified = TRUE
		        AND d.status = 'ACTIVE'
		        AND d.deleted_at IS NULL
		  )
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
