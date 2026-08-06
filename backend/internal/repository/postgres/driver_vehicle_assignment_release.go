package postgres

import (
	"context"
)

// Release ends an active driver-vehicle assignment.
func (r *DriverVehicleAssignmentRepository) Release(
	ctx context.Context,
	id string,
) error {

	const query = `
	UPDATE driver_vehicle_assignments
	SET
		status = 'RELEASED',
		released_at = NOW(),
		is_active = FALSE,
		updated_at = NOW()
	WHERE id = $1
	  AND deleted_at IS NULL;
	`

	_, err := r.db.Exec(
		ctx,
		query,
		id,
	)

	return err
}