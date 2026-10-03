package postgres

import (
	"context"
	"fmt"

	"github.com/JCKFinland/connect/backend/internal/models"
)

// Create persists a new trip in the canonical initial lifecycle state.
func (r *TripRepository) Create(
	ctx context.Context,
	trip *models.Trip,
) error {
	if trip == nil {
		return fmt.Errorf("trip is required")
	}

	if trip.AssignedAt.IsZero() {
		return fmt.Errorf("trip assigned-at time is required")
	}

	trip.Status = "ASSIGNED"
	trip.DriverArrivedAt = nil
	trip.StartedAt = nil
	trip.CompletedAt = nil
	trip.CancelledAt = nil
	trip.CancelledBy = nil
	trip.CancellationReason = nil
	trip.IsActive = true
	trip.ActualDistanceKM = nil
	trip.ActualDurationMinutes = nil
	trip.ActualDistanceMeters = nil
	trip.ActualDurationSeconds = nil

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
			driver_arrived_at,
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
			'ASSIGNED',
			$11,
			$12,
			NULL,
			NULL,
			$13,
			$14,
			$15,
			$16,
			$17,
			$18,
			$19,
			$20,
			$21,
			NULL,
			NULL,
			NULL,
			NULL,
			NULL,
			NULL,
			TRUE,
			$22,
			$23,
			NULL,
			NULL
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
		trip.PricingProfileID,
		trip.FleetID,
		trip.EstimatedDistanceKM,
		trip.EstimatedDurationMinutes,
		trip.AssignedAt,
		trip.ScheduledAt,
		trip.PickupAddress,
		trip.PickupLatitude,
		trip.PickupLongitude,
		trip.DropoffAddress,
		trip.DropoffLatitude,
		trip.DropoffLongitude,
		trip.PassengerNote,
		trip.EstimatedDistanceMeters,
		trip.EstimatedDurationSeconds,
	).Scan(
		&trip.CreatedAt,
		&trip.UpdatedAt,
	)

	if err != nil {
		return fmt.Errorf("create trip: %w", err)
	}

	return nil
}
