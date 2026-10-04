package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/JCKFinland/connect/backend/internal/models"
	"github.com/JCKFinland/connect/backend/internal/repository"
)

// VehicleRepository implements repository.VehicleRepository.
type VehicleRepository struct {
	db DBTX
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
			fuel_type,
			seating_capacity,
			is_active
		)
		VALUES
		(
			$1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13
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
		vehicle.FuelType,
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
			fuel_type,
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
		&vehicle.FuelType,
		&vehicle.SeatingCapacity,
		&vehicle.IsActive,
		&vehicle.CreatedAt,
		&vehicle.UpdatedAt,
		&vehicle.DeletedAt,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, repository.ErrNotFound
		}
		return nil, err
	}

	return &vehicle, nil
}

// GetByIDForCompanyMember retrieves a vehicle only when the user has
// explicit membership in the vehicle's company.
func (r *VehicleRepository) GetByIDForCompanyMember(
	ctx context.Context,
	userID string,
	id string,
) (*models.Vehicle, error) {

	query := `
		SELECT
			v.id,
			v.company_id,
			v.branch_id,
			v.fleet_id,
			v.registration_number,
			v.vin,
			v.make,
			v.model,
			v.model_year,
			v.color,
			v.vehicle_type,
			v.fuel_type,
			v.seating_capacity,
			v.is_active,
			v.created_at,
			v.updated_at,
			v.deleted_at
		FROM vehicles v
		INNER JOIN company_memberships cm
			ON cm.company_id = v.company_id
		   AND cm.user_id = $1
		WHERE v.id = $2
		  AND v.deleted_at IS NULL
	`

	var vehicle models.Vehicle

	err := r.db.QueryRow(
		ctx,
		query,
		userID,
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
		&vehicle.FuelType,
		&vehicle.SeatingCapacity,
		&vehicle.IsActive,
		&vehicle.CreatedAt,
		&vehicle.UpdatedAt,
		&vehicle.DeletedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, repository.ErrNotFound
		}
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
			fuel_type,
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
			&vehicle.FuelType,
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

// ListForCompanyMember returns vehicles only from companies in which the
// user has explicit membership.
func (r *VehicleRepository) ListForCompanyMember(
	ctx context.Context,
	userID string,
) ([]models.Vehicle, error) {

	query := `
		SELECT
			v.id,
			v.company_id,
			v.branch_id,
			v.fleet_id,
			v.registration_number,
			v.vin,
			v.make,
			v.model,
			v.model_year,
			v.color,
			v.vehicle_type,
			v.fuel_type,
			v.seating_capacity,
			v.is_active,
			v.created_at,
			v.updated_at,
			v.deleted_at
		FROM vehicles v
		INNER JOIN company_memberships cm
			ON cm.company_id = v.company_id
		   AND cm.user_id = $1
		WHERE v.deleted_at IS NULL
		ORDER BY v.created_at DESC
	`

	rows, err := r.db.Query(ctx, query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	vehicles := make([]models.Vehicle, 0)

	for rows.Next() {
		var vehicle models.Vehicle

		if err := rows.Scan(
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
			&vehicle.FuelType,
			&vehicle.SeatingCapacity,
			&vehicle.IsActive,
			&vehicle.CreatedAt,
			&vehicle.UpdatedAt,
			&vehicle.DeletedAt,
		); err != nil {
			return nil, err
		}

		vehicles = append(vehicles, vehicle)
	}

	return vehicles, rows.Err()
}

// UpdateDetails modifies descriptive vehicle fields only. Tenant, fleet, and
// activation authority cannot be changed through this repository operation.
func (r *VehicleRepository) UpdateDetails(
	ctx context.Context,
	vehicle *models.Vehicle,
) error {
	query := `
		UPDATE vehicles
		SET
			registration_number=$2,
			vin=$3,
			make=$4,
			model=$5,
			model_year=$6,
			color=$7,
			vehicle_type=$8,
			fuel_type=$9,
			seating_capacity=$10,
			updated_at=NOW()
		WHERE id=$1
		  AND deleted_at IS NULL
	`

	result, err := r.db.Exec(
		ctx,
		query,
		vehicle.ID,
		vehicle.RegistrationNumber,
		vehicle.VIN,
		vehicle.Make,
		vehicle.Model,
		vehicle.ModelYear,
		vehicle.Color,
		vehicle.VehicleType,
		vehicle.FuelType,
		vehicle.SeatingCapacity,
	)
	if err != nil {
		return err
	}

	if result.RowsAffected() == 0 {
		return repository.ErrNotFound
	}

	return nil
}

// UpdateDetailsForCompanyMember modifies descriptive vehicle fields only when
// the user has explicit membership in the vehicle's current company. The
// membership check is part of the UPDATE so authorization cannot become stale
// between a preceding read and the mutation.
func (r *VehicleRepository) UpdateDetailsForCompanyMember(
	ctx context.Context,
	userID string,
	vehicle *models.Vehicle,
) error {
	query := `
		UPDATE vehicles v
		SET
			registration_number=$3,
			vin=$4,
			make=$5,
			model=$6,
			model_year=$7,
			color=$8,
			vehicle_type=$9,
			fuel_type=$10,
			seating_capacity=$11,
			updated_at=NOW()
		WHERE v.id=$1
		  AND v.deleted_at IS NULL
		  AND EXISTS (
			SELECT 1
			FROM company_memberships cm
			WHERE cm.user_id=$2
			  AND cm.company_id=v.company_id
		  )
	`

	result, err := r.db.Exec(
		ctx,
		query,
		vehicle.ID,
		userID,
		vehicle.RegistrationNumber,
		vehicle.VIN,
		vehicle.Make,
		vehicle.Model,
		vehicle.ModelYear,
		vehicle.Color,
		vehicle.VehicleType,
		vehicle.FuelType,
		vehicle.SeatingCapacity,
	)
	if err != nil {
		return err
	}

	if result.RowsAffected() == 0 {
		return repository.ErrNotFound
	}

	return nil
}

// Archive hides a non-deleted vehicle from active repository reads.
func (r *VehicleRepository) Archive(
	ctx context.Context,
	id string,
) error {

	const query = `
		UPDATE vehicles
		SET
			deleted_at=NOW(),
			updated_at=NOW()
		WHERE id=$1
		  AND deleted_at IS NULL
	`

	result, err := r.db.Exec(
		ctx,
		query,
		id,
	)
	if err != nil {
		return err
	}

	if result.RowsAffected() == 0 {
		return repository.ErrNotFound
	}

	return nil
}

// ArchiveForCompanyMember hides a vehicle only when the authenticated user has
// explicit membership in the vehicle's company.
func (r *VehicleRepository) ArchiveForCompanyMember(
	ctx context.Context,
	userID string,
	id string,
) error {

	const query = `
		UPDATE vehicles AS v
		SET
			deleted_at=NOW(),
			updated_at=NOW()
		WHERE v.id=$1
		  AND v.deleted_at IS NULL
		  AND EXISTS (
			SELECT 1
			FROM company_memberships AS cm
			WHERE cm.company_id=v.company_id
			  AND cm.user_id=$2
		  )
	`

	result, err := r.db.Exec(
		ctx,
		query,
		id,
		userID,
	)
	if err != nil {
		return err
	}

	if result.RowsAffected() == 0 {
		return repository.ErrNotFound
	}

	return nil
}
