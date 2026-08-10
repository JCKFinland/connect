package postgres

import (
	"context"
	"fmt"

	"github.com/JCKFinland/connect/backend/internal/models"
	"github.com/jackc/pgx/v5"
)

// Update persists editable trip fields.
func (r *TripRepository) Update(
	ctx context.Context,
	trip *models.Trip,
) error {
	const query = `
		UPDATE trips
		SET
			status = $1,

			estimated_distance_km = $2,
			estimated_duration_minutes = $3,
			actual_distance_km = $4,
			actual_duration_minutes = $5,

			estimated_distance_meters = $6,
			estimated_duration_seconds = $7,
			actual_distance_meters = $8,
			actual_duration_seconds = $9,

			assigned_at = $10,
			scheduled_at = $11,

			pickup_address = $12,
			pickup_latitude = $13,
			pickup_longitude = $14,

			dropoff_address = $15,
			dropoff_latitude = $16,
			dropoff_longitude = $17,

			passenger_note = $18,

			driver_arrived_at = $19,
			passenger_on_board_at = $20,
			pickup_at = $21,
			started_at = $22,
			completed_at = $23,
			cancelled_at = $24,

			cancelled_by = $25,
			cancellation_reason = $26,

			is_active = $27,

			updated_at = NOW()
		WHERE id = $28
		  AND deleted_at IS NULL
		RETURNING updated_at
	`

	err := r.db.QueryRow(
		ctx,
		query,
		trip.Status,

		trip.EstimatedDistanceKM,
		trip.EstimatedDurationMinutes,
		trip.ActualDistanceKM,
		trip.ActualDurationMinutes,

		trip.EstimatedDistanceMeters,
		trip.EstimatedDurationSeconds,
		trip.ActualDistanceMeters,
		trip.ActualDurationSeconds,

		trip.AssignedAt,
		trip.ScheduledAt,

		trip.PickupAddress,
		trip.PickupLatitude,
		trip.PickupLongitude,

		trip.DropoffAddress,
		trip.DropoffLatitude,
		trip.DropoffLongitude,

		trip.PassengerNote,

		trip.DriverArrivedAt,
		trip.PassengerOnBoardAt,
		trip.PickupAt,
		trip.StartedAt,
		trip.CompletedAt,
		trip.CancelledAt,

		trip.CancelledBy,
		trip.CancellationReason,

		trip.IsActive,

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
