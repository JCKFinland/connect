package postgres

import (
	"context"
	"fmt"

	"github.com/JCKFinland/connect/backend/internal/models"
	"github.com/jackc/pgx/v5"
)

// GetByIDForUpdate retrieves and locks an active trip for the duration
// of the current PostgreSQL transaction.
//
// This prevents concurrent lifecycle transitions from modifying the same
// trip simultaneously.
func (r *TripRepository) GetByIDForUpdate(
	ctx context.Context,
	id string,
) (*models.Trip, error) {

	const query = `
		SELECT
			id,
			ride_request_id,
			customer_id,
			driver_id,
			vehicle_id,
            company_id,
            branch_id,
            service_category_id,
            pricing_profile_id,
            fleet_id,
            status,

			estimated_distance_km,
			estimated_duration_minutes,
			actual_distance_km,
			actual_duration_minutes,

			assigned_at,
			scheduled_at,

			pickup_address,
			pickup_latitude,
			pickup_longitude,

			dropoff_address,
			dropoff_latitude,
			dropoff_longitude,

			passenger_note,

			created_at,
			updated_at,

			estimated_distance_meters,
			estimated_duration_seconds,
			actual_distance_meters,
			actual_duration_seconds,

			driver_arrived_at,
			passenger_on_board_at,
			pickup_at,
			started_at,
			completed_at,
			cancelled_at,

			cancelled_by,
			cancellation_reason,

			is_active,
			deleted_at
		FROM trips
		WHERE id = $1
		  AND deleted_at IS NULL
		FOR UPDATE
	`

	trip := &models.Trip{}

	err := r.db.QueryRow(
		ctx,
		query,
		id,
	).Scan(
		&trip.ID,
		&trip.RideRequestID,
		&trip.CustomerID,
		&trip.DriverID,
		&trip.VehicleID,
		&trip.CompanyID,
		&trip.BranchID,
		&trip.ServiceCategoryID,
		&trip.PricingProfileID,
		&trip.FleetID,
		&trip.Status,

		&trip.EstimatedDistanceKM,
		&trip.EstimatedDurationMinutes,
		&trip.ActualDistanceKM,
		&trip.ActualDurationMinutes,

		&trip.AssignedAt,
		&trip.ScheduledAt,

		&trip.PickupAddress,
		&trip.PickupLatitude,
		&trip.PickupLongitude,

		&trip.DropoffAddress,
		&trip.DropoffLatitude,
		&trip.DropoffLongitude,

		&trip.PassengerNote,

		&trip.CreatedAt,
		&trip.UpdatedAt,

		&trip.EstimatedDistanceMeters,
		&trip.EstimatedDurationSeconds,
		&trip.ActualDistanceMeters,
		&trip.ActualDurationSeconds,

		&trip.DriverArrivedAt,
		&trip.PassengerOnBoardAt,
		&trip.PickupAt,
		&trip.StartedAt,
		&trip.CompletedAt,
		&trip.CancelledAt,

		&trip.CancelledBy,
		&trip.CancellationReason,

		&trip.IsActive,
		&trip.DeletedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, pgx.ErrNoRows
		}

		return nil, fmt.Errorf(
			"get trip for update: %w",
			err,
		)
	}

	return trip, nil
}
