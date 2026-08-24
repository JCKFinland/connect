package postgres

import (
	"context"
	"fmt"
	"time"
)

// UpdateHeartbeatIfOnline updates a driver's live position and heartbeat
// only while the driver has an active online presence.
//
// AVAILABLE, BUSY, and BREAK are valid online operational states.
// OFFLINE, OFF_DUTY, and SUSPENDED must not refresh live heartbeat data.
func (r *DriverPresenceRepository) UpdateHeartbeatIfOnline(
	ctx context.Context,
	driverID string,
	latitude float64,
	longitude float64,
	heading float64,
	speed float64,
	accuracy float64,
) (bool, error) {

	if driverID == "" {
		return false, fmt.Errorf(
			"driver ID is required",
		)
	}

	const query = `
		UPDATE driver_presence
		SET
			latitude = $2,
			longitude = $3,
			heading = $4,
			speed = $5,
			accuracy = $6,
			last_heartbeat_at = $7,
			updated_at = NOW()
		WHERE driver_id = $1
		  AND is_online = TRUE
		  AND availability_status IN (
			'AVAILABLE',
			'BUSY',
			'BREAK'
		  )
	`

	result, err := r.db.Exec(
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

	if err != nil {
		return false, fmt.Errorf(
			"update online driver heartbeat: %w",
			err,
		)
	}

	return result.RowsAffected() == 1, nil
}
