package postgres

import (
	"context"

	"github.com/JCKFinland/connect/backend/internal/models"
)

// List returns all active (non-deleted) driver-vehicle assignments.
func (r *DriverVehicleAssignmentRepository) List(
	ctx context.Context,
) ([]models.DriverVehicleAssignment, error) {

	const query = `
	SELECT
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
	FROM driver_vehicle_assignments
	WHERE deleted_at IS NULL
	ORDER BY assigned_at DESC;
	`

	rows, err := r.db.Query(
		ctx,
		query,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	assignments := make([]models.DriverVehicleAssignment, 0)

	for rows.Next() {

		var assignment models.DriverVehicleAssignment

		err := rows.Scan(
			&assignment.ID,
			&assignment.CompanyID,
			&assignment.BranchID,
			&assignment.FleetID,
			&assignment.DriverID,
			&assignment.VehicleID,
			&assignment.Status,
			&assignment.AssignedAt,
			&assignment.ReleasedAt,
			&assignment.AssignedBy,
			&assignment.Notes,
			&assignment.IsActive,
			&assignment.CreatedAt,
			&assignment.UpdatedAt,
			&assignment.DeletedAt,
		)
		if err != nil {
			return nil, err
		}

		assignments = append(assignments, assignment)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return assignments, nil
}