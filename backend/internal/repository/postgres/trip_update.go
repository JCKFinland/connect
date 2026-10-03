package postgres

import (
	"context"
	"fmt"

	"github.com/JCKFinland/connect/backend/internal/models"
	"github.com/jackc/pgx/v5"
)

// Update persists editable trip fields.
//
// Lifecycle state, assignment state, lifecycle timestamps, cancellation
// metadata, ownership, active state, and actual trip metrics are intentionally
// excluded. Those fields are owned by their dedicated operational mutation
// paths.
func (r *TripRepository) Update(
	ctx context.Context,
	trip *models.Trip,
) error {
	const query = `
		UPDATE trips
		SET
			estimated_distance_km = $1,
			estimated_duration_minutes = $2,

			estimated_distance_meters = $3,
			estimated_duration_seconds = $4,

			scheduled_at = $5,

			pickup_address = $6,
			pickup_latitude = $7,
			pickup_longitude = $8,

			dropoff_address = $9,
			dropoff_latitude = $10,
			dropoff_longitude = $11,

			passenger_note = $12,

			updated_at = NOW()
		WHERE id = $13
		  AND deleted_at IS NULL
		RETURNING updated_at
	`

	err := r.db.QueryRow(
		ctx,
		query,
		trip.EstimatedDistanceKM,
		trip.EstimatedDurationMinutes,

		trip.EstimatedDistanceMeters,
		trip.EstimatedDurationSeconds,

		trip.ScheduledAt,

		trip.PickupAddress,
		trip.PickupLatitude,
		trip.PickupLongitude,

		trip.DropoffAddress,
		trip.DropoffLatitude,
		trip.DropoffLongitude,

		trip.PassengerNote,

		trip.ID,
	).Scan(&trip.UpdatedAt)

	if err != nil {
		if err == pgx.ErrNoRows {
			return fmt.Errorf("trip not found: %w", err)
		}

		return fmt.Errorf("update trip: %w", err)
	}

	return nil
}
