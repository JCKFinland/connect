package postgres

import (
	"context"
	"fmt"

	"github.com/JCKFinland/connect/backend/internal/models"
)

// Create persists a new trip.
func (r *TripRepository) Create(
	ctx context.Context,
	trip *models.Trip,
) error {
	const query = `
		INSERT INTO trips (
			id,
			ride_request_id,
			customer_id,
			driver_id,
			vehicle_id,
			company_id,
            branch_id,
            service_category_id,
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
			driver_arrived_at,
			passenger_on_board_at,
			pickup_at,
			started_at,
			completed_at,
			cancelled_at,
			cancelled_by,
			cancellation_reason,
			is_active,
			estimated_distance_meters,
			estimated_duration_seconds,
			actual_distance_meters,
			actual_duration_seconds
		)
		VALUES (
			$1,
			$2,
			$3,
			$4,
			$5,
			$6,
			$7,
			$8,
			$9,
			$10,
			$11,
			$12,
			$13,
			$14,
			$15,
			$16,
			$17,
			$18,
			$19,
			$20,
			$21,
			$22,
			$23,
			$24,
			$25,
			$26,
			$27,
			$28,
			$29,
			$30,
			$31,
			$32,
			$33,
			$34,
			$35,
			$36
		)
		RETURNING
			created_at,
			updated_at
	`

	err := r.db.QueryRow(
		ctx,
		query,
		trip.ID,
		trip.RideRequestID,
		trip.CustomerID,
		trip.DriverID,
		trip.VehicleID,
		trip.CompanyID,
		trip.BranchID,
		trip.ServiceCategoryID,
		trip.FleetID,
		trip.Status,
		trip.EstimatedDistanceKM,
		trip.EstimatedDurationMinutes,
		trip.ActualDistanceKM,
		trip.ActualDurationMinutes,
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
		trip.EstimatedDistanceMeters,
		trip.EstimatedDurationSeconds,
		trip.ActualDistanceMeters,
		trip.ActualDurationSeconds,
	).Scan(
		&trip.CreatedAt,
		&trip.UpdatedAt,
	)

	if err != nil {
		return fmt.Errorf("create trip: %w", err)
	}

	return nil
}
