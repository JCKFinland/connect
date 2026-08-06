package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/JCKFinland/connect/backend/internal/models"
)

// GetByID returns a single driver-vehicle assignment.
func (r *DriverVehicleAssignmentRepository) GetByID(
	ctx context.Context,
	id string,
) (*models.DriverVehicleAssignment, error) {

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
	WHERE id = $1
	  AND deleted_at IS NULL;
	`

	var assignment models.DriverVehicleAssignment

	err := r.db.QueryRow(
		ctx,
		query,
		id,
	).Scan(
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

		if errors.Is(err, pgx.ErrNoRows) {
			return nil, pgx.ErrNoRows
		}

		return nil, err
	}

	return &assignment, nil
}