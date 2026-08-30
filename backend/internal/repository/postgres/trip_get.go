package postgres

import (
	"context"
	"fmt"

	"github.com/JCKFinland/connect/backend/internal/models"
	"github.com/jackc/pgx/v5"
)

// GetByID retrieves an active trip by its ID.
func (r *TripRepository) GetByID(
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

			estimated_distance_meters,
			estimated_duration_seconds,
			actual_distance_meters,
			actual_duration_seconds,

			assigned_at,
			scheduled_at,

			driver_arrived_at,
			passenger_on_board_at,
			pickup_at,
			started_at,
			completed_at,
			cancelled_at,

			cancellation_reason,
			cancelled_by,

			pickup_address,
			pickup_latitude,
			pickup_longitude,

			dropoff_address,
			dropoff_latitude,
			dropoff_longitude,

			passenger_note,

			is_active,
			deleted_at,

			created_at,
			updated_at
		FROM trips
		WHERE id = $1
		  AND deleted_at IS NULL
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

		&trip.EstimatedDistanceMeters,
		&trip.EstimatedDurationSeconds,
		&trip.ActualDistanceMeters,
		&trip.ActualDurationSeconds,

		&trip.AssignedAt,
		&trip.ScheduledAt,

		&trip.DriverArrivedAt,
		&trip.PassengerOnBoardAt,
		&trip.PickupAt,
		&trip.StartedAt,
		&trip.CompletedAt,
		&trip.CancelledAt,

		&trip.CancellationReason,
		&trip.CancelledBy,

		&trip.PickupAddress,
		&trip.PickupLatitude,
		&trip.PickupLongitude,

		&trip.DropoffAddress,
		&trip.DropoffLatitude,
		&trip.DropoffLongitude,

		&trip.PassengerNote,

		&trip.IsActive,
		&trip.DeletedAt,

		&trip.CreatedAt,
		&trip.UpdatedAt,
	)

	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("trip not found: %w", err)
		}

		return nil, fmt.Errorf("get trip by id: %w", err)
	}

	return trip, nil
}
