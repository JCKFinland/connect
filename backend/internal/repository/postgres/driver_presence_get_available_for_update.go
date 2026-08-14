package postgres

import (
	"context"
	"errors"

	"github.com/JCKFinland/connect/backend/internal/models"
	"github.com/JCKFinland/connect/backend/internal/repository"
	"github.com/jackc/pgx/v5"
)

// GetAvailableByDriverIDForUpdate returns and locks one available driver.
//
// SKIP LOCKED means that if another dispatch transaction already holds
// the driver's presence row, this query behaves as though the driver
// is unavailable instead of waiting for the other transaction.
func (r *DriverPresenceRepository) GetAvailableByDriverIDForUpdate(
	ctx context.Context,
	driverID string,
) (*models.DriverPresence, error) {

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
		  AND is_online = TRUE
		  AND availability_status = 'AVAILABLE'
		FOR UPDATE SKIP LOCKED
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
		return nil, err
	}

	return p, nil
}
