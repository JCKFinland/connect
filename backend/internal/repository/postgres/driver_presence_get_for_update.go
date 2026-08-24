package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/JCKFinland/connect/backend/internal/models"
	"github.com/JCKFinland/connect/backend/internal/repository"
	"github.com/jackc/pgx/v5"
)

// GetByDriverIDForUpdate retrieves and locks a driver's presence row.
//
// This is the general lifecycle lock for operations such as assignment
// detachment. Unlike GetAvailableByDriverIDForUpdate, it does not require
// the driver to currently be AVAILABLE.
//
// The row remains locked until the surrounding PostgreSQL transaction
// commits or rolls back.
func (r *DriverPresenceRepository) GetByDriverIDForUpdate(
	ctx context.Context,
	driverID string,
) (*models.DriverPresence, error) {

	if driverID == "" {
		return nil, fmt.Errorf(
			"driver ID is required",
		)
	}

	const query = `
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
		WHERE driver_id = $1
		FOR UPDATE
	`

	p := &models.DriverPresence{}

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
		return nil, fmt.Errorf(
			"get driver presence for update: %w",
			err,
		)
	}

	return p, nil
}
