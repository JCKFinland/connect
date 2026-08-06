package postgres

import (
	"context"
)

// Delete performs a soft delete on a driver-vehicle assignment.
func (r *DriverVehicleAssignmentRepository) Delete(
	ctx context.Context,
	id string,
) error {

	const query = `
	UPDATE driver_vehicle_assignments
	SET
		deleted_at = NOW(),
		updated_at = NOW(),
		is_active = FALSE
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