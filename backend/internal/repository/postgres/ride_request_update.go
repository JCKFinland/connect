package postgres

import (
	"context"
	"fmt"

	"github.com/JCKFinland/connect/backend/internal/models"
)

// Update persists changes to an existing ride request.
func (r *RideRequestRepository) Update(
	ctx context.Context,
	request *models.RideRequest,
) error {
	const query = `
	UPDATE ride_requests
	SET
		pickup_address = $1,
		pickup_latitude = $2,
		pickup_longitude = $3,
		destination_address = $4,
		destination_latitude = $5,
		destination_longitude = $6,
		requested_vehicle_type = $7,
		service_category_id = $8,
		passenger_count = $9,
		notes = $10,
		expires_at = $11,
		updated_at = NOW()
	WHERE id = $12
	`

	result, err := r.db.Exec(
		ctx,
		query,
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
		request.ExpiresAt,
		request.ID,
	)

	if err != nil {
		return fmt.Errorf("update ride request: %w", err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("ride request not found")
	}

	return nil
}
