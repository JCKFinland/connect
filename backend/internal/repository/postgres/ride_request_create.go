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
			$12,
			$13,
			$14,
			$15,
			$16,
			$17
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
		request.Status,
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
