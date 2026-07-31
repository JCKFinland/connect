package postgres

import (
	"context"
	"database/sql"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/JCKFinland/connect/backend/internal/models"
	"github.com/JCKFinland/connect/backend/internal/repository"
)

type DriverAssignmentRepository struct {
	db *pgxpool.Pool
}

func NewDriverAssignmentRepository(
	db *pgxpool.Pool,
) repository.DriverAssignmentRepository {

	return &DriverAssignmentRepository{
		db: db,
	}
}

func (r *DriverAssignmentRepository) Create(
	ctx context.Context,
	assignment *models.DriverAssignment,
) error {

	query := `
	INSERT INTO driver_assignments
	(
		company_id,
		branch_id,
		fleet_id,
		driver_id,
		vehicle_id,
		assigned_at,
		unassigned_at,
		notes
	)
	VALUES
	(
		$1,$2,$3,$4,$5,$6,$7,$8
	)
	RETURNING id, created_at, updated_at
	`

	return r.db.QueryRow(
		ctx,
		query,
		assignment.CompanyID,
		assignment.BranchID,
		assignment.FleetID,
		assignment.DriverID,
		assignment.VehicleID,
		assignment.AssignedAt,
		assignment.UnassignedAt,
		assignment.Notes,
	).Scan(
		&assignment.ID,
		&assignment.CreatedAt,
		&assignment.UpdatedAt,
	)
}

func (r *DriverAssignmentRepository) Update(
	ctx context.Context,
	assignment *models.DriverAssignment,
) error {

	query := `
	UPDATE driver_assignments
	SET
		company_id=$2,
		branch_id=$3,
		fleet_id=$4,
		driver_id=$5,
		vehicle_id=$6,
		assigned_at=$7,
		unassigned_at=$8,
		notes=$9,
		updated_at=NOW()
	WHERE id=$1
	`

	cmd, err := r.db.Exec(
		ctx,
		query,
		assignment.ID,
		assignment.CompanyID,
		assignment.BranchID,
		assignment.FleetID,
		assignment.DriverID,
		assignment.VehicleID,
		assignment.AssignedAt,
		assignment.UnassignedAt,
		assignment.Notes,
	)
	if err != nil {
		return err
	}

	if cmd.RowsAffected() == 0 {
		return repository.ErrNotFound
	}

	return nil
}

func (r *DriverAssignmentRepository) GetByID(
	ctx context.Context,
	id string,
) (*models.DriverAssignment, error) {

	query := `
	SELECT
		id,
		company_id,
		branch_id,
		fleet_id,
		driver_id,
		vehicle_id,
		assigned_at,
		unassigned_at,
		notes,
		created_at,
		updated_at
	FROM driver_assignments
	WHERE id=$1
	`

	var a models.DriverAssignment

	err := r.db.QueryRow(ctx, query, id).Scan(
		&a.ID,
		&a.CompanyID,
		&a.BranchID,
		&a.FleetID,
		&a.DriverID,
		&a.VehicleID,
		&a.AssignedAt,
		&a.UnassignedAt,
		&a.Notes,
		&a.CreatedAt,
		&a.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, repository.ErrNotFound
	}

	if err != nil {
		return nil, err
	}

	return &a, nil
}

func (r *DriverAssignmentRepository) GetActiveByDriver(
	ctx context.Context,
	driverID string,
) (*models.DriverAssignment, error) {

	query := `
	SELECT
		id,
		company_id,
		branch_id,
		fleet_id,
		driver_id,
		vehicle_id,
		assigned_at,
		unassigned_at,
		notes,
		created_at,
		updated_at
	FROM driver_assignments
	WHERE driver_id=$1
	AND unassigned_at IS NULL
	LIMIT 1
	`

	var a models.DriverAssignment

	err := r.db.QueryRow(ctx, query, driverID).Scan(
		&a.ID,
		&a.CompanyID,
		&a.BranchID,
		&a.FleetID,
		&a.DriverID,
		&a.VehicleID,
		&a.AssignedAt,
		&a.UnassignedAt,
		&a.Notes,
		&a.CreatedAt,
		&a.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, repository.ErrNotFound
	}

	if err != nil {
		return nil, err
	}

	return &a, nil
}

func (r *DriverAssignmentRepository) GetActiveByVehicle(
	ctx context.Context,
	vehicleID string,
) (*models.DriverAssignment, error) {

	query := `
	SELECT
		id,
		company_id,
		branch_id,
		fleet_id,
		driver_id,
		vehicle_id,
		assigned_at,
		unassigned_at,
		notes,
		created_at,
		updated_at
	FROM driver_assignments
	WHERE vehicle_id=$1
	AND unassigned_at IS NULL
	LIMIT 1
	`

	var a models.DriverAssignment

	err := r.db.QueryRow(ctx, query, vehicleID).Scan(
		&a.ID,
		&a.CompanyID,
		&a.BranchID,
		&a.FleetID,
		&a.DriverID,
		&a.VehicleID,
		&a.AssignedAt,
		&a.UnassignedAt,
		&a.Notes,
		&a.CreatedAt,
		&a.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, repository.ErrNotFound
	}

	if err != nil {
		return nil, err
	}

	return &a, nil
}

func (r *DriverAssignmentRepository) ListByDriver(
	ctx context.Context,
	driverID string,
) ([]*models.DriverAssignment, error) {

	return r.list(
		ctx,
		`SELECT id, company_id, branch_id, fleet_id, driver_id, vehicle_id,
		        assigned_at, unassigned_at, notes, created_at, updated_at
		 FROM driver_assignments
		 WHERE driver_id=$1
		 ORDER BY assigned_at DESC`,
		driverID,
	)
}

func (r *DriverAssignmentRepository) ListByVehicle(
	ctx context.Context,
	vehicleID string,
) ([]*models.DriverAssignment, error) {

	return r.list(
		ctx,
		`SELECT id, company_id, branch_id, fleet_id, driver_id, vehicle_id,
		        assigned_at, unassigned_at, notes, created_at, updated_at
		 FROM driver_assignments
		 WHERE vehicle_id=$1
		 ORDER BY assigned_at DESC`,
		vehicleID,
	)
}

func (r *DriverAssignmentRepository) CloseAssignment(
	ctx context.Context,
	driverID string,
) error {

	now := time.Now().UTC()

	cmd, err := r.db.Exec(
		ctx,
		`
		UPDATE driver_assignments
		SET
			unassigned_at=$2,
			updated_at=NOW()
		WHERE driver_id=$1
		AND unassigned_at IS NULL
		`,
		driverID,
		now,
	)
	if err != nil {
		return err
	}

	if cmd.RowsAffected() == 0 {
		return repository.ErrNotFound
	}

	return nil
}

func (r *DriverAssignmentRepository) Delete(
	ctx context.Context,
	id string,
) error {

	cmd, err := r.db.Exec(
		ctx,
		`DELETE FROM driver_assignments WHERE id=$1`,
		id,
	)
	if err != nil {
		return err
	}

	if cmd.RowsAffected() == 0 {
		return repository.ErrNotFound
	}

	return nil
}

func (r *DriverAssignmentRepository) list(
	ctx context.Context,
	query string,
	arg string,
) ([]*models.DriverAssignment, error) {

	rows, err := r.db.Query(ctx, query, arg)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var assignments []*models.DriverAssignment

	for rows.Next() {

		var a models.DriverAssignment

		if err := rows.Scan(
			&a.ID,
			&a.CompanyID,
			&a.BranchID,
			&a.FleetID,
			&a.DriverID,
			&a.VehicleID,
			&a.AssignedAt,
			&a.UnassignedAt,
			&a.Notes,
			&a.CreatedAt,
			&a.UpdatedAt,
		); err != nil {
			return nil, err
		}

		assignments = append(assignments, &a)
	}

	return assignments, rows.Err()
}