package postgres

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/JCKFinland/connect/backend/internal/models"
	"github.com/JCKFinland/connect/backend/internal/repository"
)

// VehicleRepository implements repository.VehicleRepository.
type VehicleRepository struct {
	db *pgxpool.Pool
}

// Compile-time interface check.
var _ repository.VehicleRepository = (*VehicleRepository)(nil)

// NewVehicleRepository creates a new PostgreSQL vehicle repository.
func NewVehicleRepository(
	db *pgxpool.Pool,
) *VehicleRepository {

	return &VehicleRepository{
		db: db,
	}
}

// Create inserts a new vehicle.
func (r *VehicleRepository) Create(
	ctx context.Context,
	vehicle *models.Vehicle,
) error {

	query := `
		INSERT INTO vehicles
		(
			company_id,
			branch_id,
			fleet_id,
			registration_number,
			vin,
			make,
			model,
			model_year,
			color,
			vehicle_type,
			seating_capacity,
			is_active
		)
		VALUES
		(
			$1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12
		)
		RETURNING
			id,
			created_at,
			updated_at
	`

	return r.db.QueryRow(
		ctx,
		query,
		vehicle.CompanyID,
		vehicle.BranchID,
		vehicle.FleetID,
		vehicle.RegistrationNumber,
		vehicle.VIN,
		vehicle.Make,
		vehicle.Model,
		vehicle.ModelYear,
		vehicle.Color,
		vehicle.VehicleType,
		vehicle.SeatingCapacity,
		vehicle.IsActive,
	).Scan(
		&vehicle.ID,
		&vehicle.CreatedAt,
		&vehicle.UpdatedAt,
	)
}

// GetByID retrieves a vehicle by ID.
func (r *VehicleRepository) GetByID(
	ctx context.Context,
	id string,
) (*models.Vehicle, error) {

	query := `
		SELECT
			id,
			company_id,
			branch_id,
			fleet_id,
			registration_number,
			vin,
			make,
			model,
			model_year,
			color,
			vehicle_type,
			seating_capacity,
			is_active,
			created_at,
			updated_at,
			deleted_at
		FROM vehicles
		WHERE id=$1
		  AND deleted_at IS NULL
	`

	var vehicle models.Vehicle

	err := r.db.QueryRow(
		ctx,
		query,
		id,
	).Scan(
		&vehicle.ID,
		&vehicle.CompanyID,
		&vehicle.BranchID,
		&vehicle.FleetID,
		&vehicle.RegistrationNumber,
		&vehicle.VIN,
		&vehicle.Make,
		&vehicle.Model,
		&vehicle.ModelYear,
		&vehicle.Color,
		&vehicle.VehicleType,
		&vehicle.SeatingCapacity,
		&vehicle.IsActive,
		&vehicle.CreatedAt,
		&vehicle.UpdatedAt,
		&vehicle.DeletedAt,
	)

	if err != nil {
		return nil, err
	}

	return &vehicle, nil
}

// List returns all active vehicles.
func (r *VehicleRepository) List(
	ctx context.Context,
) ([]models.Vehicle, error) {

	query := `
		SELECT
			id,
			company_id,
			branch_id,
			fleet_id,
			registration_number,
			vin,
			make,
			model,
			model_year,
			color,
			vehicle_type,
			seating_capacity,
			is_active,
			created_at,
			updated_at,
			deleted_at
		FROM vehicles
		WHERE deleted_at IS NULL
		ORDER BY created_at DESC
	`

	rows, err := r.db.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var vehicles []models.Vehicle

	for rows.Next() {

		var vehicle models.Vehicle

		err := rows.Scan(
			&vehicle.ID,
			&vehicle.CompanyID,
			&vehicle.BranchID,
			&vehicle.FleetID,
			&vehicle.RegistrationNumber,
			&vehicle.VIN,
			&vehicle.Make,
			&vehicle.Model,
			&vehicle.ModelYear,
			&vehicle.Color,
			&vehicle.VehicleType,
			&vehicle.SeatingCapacity,
			&vehicle.IsActive,
			&vehicle.CreatedAt,
			&vehicle.UpdatedAt,
			&vehicle.DeletedAt,
		)
		if err != nil {
			return nil, err
		}

		vehicles = append(vehicles, vehicle)
	}

	return vehicles, rows.Err()
}

// Update modifies an existing vehicle.
func (r *VehicleRepository) Update(
	ctx context.Context,
	vehicle *models.Vehicle,
) error {

	query := `
		UPDATE vehicles
		SET
			company_id=$2,
			branch_id=$3,
			fleet_id=$4,
			registration_number=$5,
			vin=$6,
			make=$7,
			model=$8,
			model_year=$9,
			color=$10,
			vehicle_type=$11,
			seating_capacity=$12,
			is_active=$13,
			updated_at=NOW()
		WHERE id=$1
		  AND deleted_at IS NULL
	`

	_, err := r.db.Exec(
		ctx,
		query,
		vehicle.ID,
		vehicle.CompanyID,
		vehicle.BranchID,
		vehicle.FleetID,
		vehicle.RegistrationNumber,
		vehicle.VIN,
		vehicle.Make,
		vehicle.Model,
		vehicle.ModelYear,
		vehicle.Color,
		vehicle.VehicleType,
		vehicle.SeatingCapacity,
		vehicle.IsActive,
	)

	return err
}

// Delete performs a soft delete.
func (r *VehicleRepository) Delete(
	ctx context.Context,
	id string,
) error {

	query := `
		UPDATE vehicles
		SET
			deleted_at=NOW(),
			updated_at=NOW()
		WHERE id=$1
		  AND deleted_at IS NULL
	`

	_, err := r.db.Exec(
		ctx,
		query,
		id,
	)

	return err
}