package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/JCKFinland/connect/backend/internal/models"
	"github.com/JCKFinland/connect/backend/internal/repository"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type DriverPresenceRepository struct {
	db DBTX
}

func NewDriverPresenceRepository(
	db *pgxpool.Pool,
) repository.DriverPresenceRepository {
	return &DriverPresenceRepository{
		db: db,
	}
}

func (r *DriverPresenceRepository) Create(
	ctx context.Context,
	p *models.DriverPresence,
) error {

	query := `
	INSERT INTO driver_presence
	(
		driver_id,
		company_id,
		branch_id,
		vehicle_id,
		assignment_id,
		is_online,
		availability_status,
		latitude,
		longitude,
		heading,
		speed,
		accuracy,
		last_heartbeat_at
	)
	VALUES
	(
		$1,$2,$3,$4,$5,
		$6,$7,$8,$9,$10,
		$11,$12,$13
	)
	`

	_, err := r.db.Exec(
		ctx,
		query,
		p.DriverID,
		p.CompanyID,
		p.BranchID,
		p.VehicleID,
		p.AssignmentID,
		p.IsOnline,
		p.AvailabilityStatus,
		p.Latitude,
		p.Longitude,
		p.Heading,
		p.Speed,
		p.Accuracy,
		p.LastHeartbeatAt,
	)

	return err
}

func (r *DriverPresenceRepository) GetByDriverID(
	ctx context.Context,
	driverID string,
) (*models.DriverPresence, error) {

	query := `
	SELECT
		driver_id,
		company_id,
		branch_id,
		vehicle_id,
		assignment_id,
		is_online,
		availability_status,
		latitude,
		longitude,
		heading,
		speed,
		accuracy,
		last_heartbeat_at,
		created_at,
		updated_at
	FROM driver_presence
	WHERE driver_id=$1
	`

	var p models.DriverPresence

	err := r.db.QueryRow(
		ctx,
		query,
		driverID,
	).Scan(
		&p.DriverID,
		&p.CompanyID,
		&p.BranchID,
		&p.VehicleID,
		&p.AssignmentID,
		&p.IsOnline,
		&p.AvailabilityStatus,
		&p.Latitude,
		&p.Longitude,
		&p.Heading,
		&p.Speed,
		&p.Accuracy,
		&p.LastHeartbeatAt,
		&p.CreatedAt,
		&p.UpdatedAt,
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return nil, repository.ErrNotFound
	}

	if err != nil {
		return nil, err
	}

	return &p, nil
}

func (r *DriverPresenceRepository) Update(
	ctx context.Context,
	p *models.DriverPresence,
) error {

	query := `
	UPDATE driver_presence
	SET
		company_id=$2,
		branch_id=$3,
		vehicle_id=$4,
		assignment_id=$5,
		is_online=$6,
		availability_status=$7,
		latitude=$8,
		longitude=$9,
		heading=$10,
		speed=$11,
		accuracy=$12,
		last_heartbeat_at=$13,
		updated_at=NOW()
	WHERE driver_id=$1
	`

	_, err := r.db.Exec(
		ctx,
		query,
		p.DriverID,
		p.CompanyID,
		p.BranchID,
		p.VehicleID,
		p.AssignmentID,
		p.IsOnline,
		p.AvailabilityStatus,
		p.Latitude,
		p.Longitude,
		p.Heading,
		p.Speed,
		p.Accuracy,
		p.LastHeartbeatAt,
	)

	return err
}

func (r *DriverPresenceRepository) UpdateHeartbeat(
	ctx context.Context,
	driverID string,
	latitude float64,
	longitude float64,
	heading float64,
	speed float64,
	accuracy float64,
) error {

	query := `
	UPDATE driver_presence
	SET
		latitude=$2,
		longitude=$3,
		heading=$4,
		speed=$5,
		accuracy=$6,
		last_heartbeat_at=$7,
		updated_at=NOW()
	WHERE driver_id=$1
	`

	_, err := r.db.Exec(
		ctx,
		query,
		driverID,
		latitude,
		longitude,
		heading,
		speed,
		accuracy,
		time.Now().UTC(),
	)

	return err
}

func (r *DriverPresenceRepository) UpdateAvailability(
	ctx context.Context,
	driverID string,
	status string,
	isOnline bool,
) error {

	query := `
	UPDATE driver_presence
	SET
		is_online=$2,
		availability_status=$3,
		updated_at=NOW()
	WHERE driver_id=$1
	`

	_, err := r.db.Exec(
		ctx,
		query,
		driverID,
		isOnline,
		status,
	)

	return err
}

func (r *DriverPresenceRepository) SetOffline(
	ctx context.Context,
	driverID string,
) error {

	return r.UpdateAvailability(
		ctx,
		driverID,
		"OFFLINE",
		false,
	)
}

func (r *DriverPresenceRepository) Delete(
	ctx context.Context,
	driverID string,
) error {

	_, err := r.db.Exec(
		ctx,
		`DELETE FROM driver_presence WHERE driver_id=$1`,
		driverID,
	)

	return err
}

func (r *DriverPresenceRepository) ListAvailable(
	ctx context.Context,
	companyID string,
) ([]*models.DriverPresence, error) {

	query := `
	SELECT
		driver_id,
		company_id,
		branch_id,
		vehicle_id,
		assignment_id,
		is_online,
		availability_status,
		latitude,
		longitude,
		heading,
		speed,
		accuracy,
		last_heartbeat_at,
		created_at,
		updated_at
	FROM driver_presence
	WHERE company_id=$1
	AND is_online=TRUE
	AND availability_status='AVAILABLE'
	`

	rows, err := r.db.Query(
		ctx,
		query,
		companyID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	drivers := make([]*models.DriverPresence, 0)

	for rows.Next() {

		var p models.DriverPresence

		if err := rows.Scan(
			&p.DriverID,
			&p.CompanyID,
			&p.BranchID,
			&p.VehicleID,
			&p.AssignmentID,
			&p.IsOnline,
			&p.AvailabilityStatus,
			&p.Latitude,
			&p.Longitude,
			&p.Heading,
			&p.Speed,
			&p.Accuracy,
			&p.LastHeartbeatAt,
			&p.CreatedAt,
			&p.UpdatedAt,
		); err != nil {
			return nil, err
		}

		drivers = append(drivers, &p)
	}

	return drivers, rows.Err()
}

func (r *DriverPresenceRepository) ListAllAvailable(
	ctx context.Context,
) ([]*models.DriverPresence, error) {

	query := `
	SELECT
		driver_id,
		company_id,
		branch_id,
		vehicle_id,
		assignment_id,
		is_online,
		availability_status,
		latitude,
		longitude,
		heading,
		speed,
		accuracy,
		last_heartbeat_at,
		created_at,
		updated_at
	FROM driver_presence
	WHERE is_online = TRUE
	  AND availability_status = 'AVAILABLE'
	ORDER BY updated_at DESC
	`

	rows, err := r.db.Query(
		ctx,
		query,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	drivers := make([]*models.DriverPresence, 0)

	for rows.Next() {
		var p models.DriverPresence

		if err := rows.Scan(
			&p.DriverID,
			&p.CompanyID,
			&p.BranchID,
			&p.VehicleID,
			&p.AssignmentID,
			&p.IsOnline,
			&p.AvailabilityStatus,
			&p.Latitude,
			&p.Longitude,
			&p.Heading,
			&p.Speed,
			&p.Accuracy,
			&p.LastHeartbeatAt,
			&p.CreatedAt,
			&p.UpdatedAt,
		); err != nil {
			return nil, err
		}

		drivers = append(
			drivers,
			&p,
		)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return drivers, nil
}
