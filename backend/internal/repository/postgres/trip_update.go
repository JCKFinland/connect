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
// metadata, ownership, and active state are intentionally excluded. Those
// fields are owned by their dedicated operational mutation paths.
func (r *TripRepository) Update(
	ctx context.Context,
	trip *models.Trip,
) error {
	const query = `
		UPDATE trips
		SET
			estimated_distance_km = $1,
			estimated_duration_minutes = $2,
			actual_distance_km = $3,
			actual_duration_minutes = $4,

			estimated_distance_meters = $5,
			estimated_duration_seconds = $6,
			actual_distance_meters = $7,
			actual_duration_seconds = $8,

			scheduled_at = $9,

			pickup_address = $10,
			pickup_latitude = $11,
			pickup_longitude = $12,

			dropoff_address = $13,
			dropoff_latitude = $14,
			dropoff_longitude = $15,

			passenger_note = $16,

			updated_at = NOW()
		WHERE id = $17
		  AND deleted_at IS NULL
		RETURNING updated_at
	`

	err := r.db.QueryRow(
		ctx,
		query,
		trip.EstimatedDistanceKM,
		trip.EstimatedDurationMinutes,
		trip.ActualDistanceKM,
		trip.ActualDurationMinutes,

		trip.EstimatedDistanceMeters,
		trip.EstimatedDurationSeconds,
		trip.ActualDistanceMeters,
		trip.ActualDurationSeconds,

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
