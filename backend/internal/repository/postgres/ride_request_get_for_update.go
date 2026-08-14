package postgres

import (
	"context"
	"fmt"

	"github.com/JCKFinland/connect/backend/internal/models"
	"github.com/jackc/pgx/v5"
)

// GetByIDForUpdate retrieves and locks a ride request for the
// duration of the current PostgreSQL transaction.
//
// This prevents two concurrent dispatch transactions from
// processing the same ride request simultaneously.
func (r *RideRequestRepository) GetByIDForUpdate(
	ctx context.Context,
	id string,
) (*models.RideRequest, error) {

	const query = `
		SELECT
			id,
			customer_id,
			pickup_address,
			pickup_latitude,
			pickup_longitude,
			destination_address,
			destination_latitude,
			destination_longitude,
			requested_vehicle_type,
			passenger_count,
			status,
			notes,
			requested_at,
			expires_at,
			created_at,
			updated_at
		FROM ride_requests
		WHERE id = $1
		FOR UPDATE
	`

	request := &models.RideRequest{}

	err := r.db.QueryRow(
		ctx,
		query,
		id,
	).Scan(
		&request.ID,
		&request.CustomerID,
		&request.PickupAddress,
		&request.PickupLatitude,
		&request.PickupLongitude,
		&request.DestinationAddress,
		&request.DestinationLatitude,
		&request.DestinationLongitude,
		&request.RequestedVehicleType,
		&request.PassengerCount,
		&request.Status,
		&request.Notes,
		&request.RequestedAt,
		&request.ExpiresAt,
		&request.CreatedAt,
		&request.UpdatedAt,
	)

	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf(
				"ride request not found: %w",
				err,
			)
		}

		return nil, fmt.Errorf(
			"get ride request for update: %w",
			err,
		)
	}

	return request, nil
}
