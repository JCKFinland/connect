package postgres

import (
	"context"
	"fmt"

	"github.com/JCKFinland/connect/backend/internal/models"
)

// Create persists a new ride request.
func (r *RideRequestRepository) Create(
	ctx context.Context,
	request *models.RideRequest,
) error {
	if request == nil {
		return fmt.Errorf("ride request is required")
	}

	// Persistence is the final authority for a newly created ride request.
	// Dispatch lifecycle state can only be established after creation.
	request.Status = "PENDING"
	request.DispatchRetryCount = 0
	request.NextDispatchAttemptAt = nil
	request.LastDispatchAttemptAt = nil

	const query = `
	INSERT INTO ride_requests (
		id,
		customer_id,
		pickup_address,
		pickup_latitude,
		pickup_longitude,
		destination_address,
		destination_latitude,
		destination_longitude,
		requested_vehicle_type,
		service_category_id,
		passenger_count,
		status,
		notes,
		requested_at,
		expires_at,
		dispatch_retry_count,
		next_dispatch_attempt_at,
		last_dispatch_attempt_at,
		created_at,
		updated_at
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
			'PENDING',
			$12,
			$13,
			$14,
			0,
			NULL,
			NULL,
			$15,
			$16
		)
		RETURNING
			created_at,
			updated_at
	`

	err := r.db.QueryRow(
		ctx,
		query,
		request.ID,
		request.CustomerID,
		request.PickupAddress,
		request.PickupLatitude,
		request.PickupLongitude,
		request.DestinationAddress,
		request.DestinationLatitude,
		request.DestinationLongitude,
		request.RequestedVehicleType,
		request.ServiceCategoryID,
		request.PassengerCount,
		request.Notes,
		request.RequestedAt,
		request.ExpiresAt,
		request.CreatedAt,
		request.UpdatedAt,
	).Scan(
		&request.CreatedAt,
		&request.UpdatedAt,
	)

	if err != nil {
		return fmt.Errorf("create ride request: %w", err)
	}

	return nil
}
