package postgres

import (
	"context"

	"github.com/JCKFinland/connect/backend/internal/models"
)

// Assign creates a new operational driver-vehicle assignment.
func (r *DriverVehicleAssignmentRepository) Assign(
	ctx context.Context,
	assignment *models.DriverVehicleAssignment,
) error {

	const query = `
	INSERT INTO driver_vehicle_assignments
	(
		id,
		company_id,
		branch_id,
		fleet_id,
		driver_id,
		vehicle_id,
		status,
		assigned_at,
		released_at,
		assigned_by,
		notes,
		is_active,
		created_at,
		updated_at,
		deleted_at
	)
	VALUES
	(
		$1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15
	)`

	_, err := r.db.Exec(
		ctx,
		query,
		assignment.ID,
		assignment.CompanyID,
		assignment.BranchID,
		assignment.FleetID,
		assignment.DriverID,
		assignment.VehicleID,
		assignment.Status,
		assignment.AssignedAt,
		assignment.ReleasedAt,
		assignment.AssignedBy,
		assignment.Notes,
		assignment.IsActive,
		assignment.CreatedAt,
		assignment.UpdatedAt,
		assignment.DeletedAt,
	)

	return err
}