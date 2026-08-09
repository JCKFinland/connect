package postgres

import (
	"context"
	"fmt"
	"strings"

	"github.com/JCKFinland/connect/backend/internal/models"
)

// List retrieves ride requests using optional filters and pagination.
func (r *RideRequestRepository) List(
	ctx context.Context,
	customerID string,
	status string,
	limit int,
	offset int,
) ([]*models.RideRequest, error) {
	const baseQuery = `
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
	`

	var (
		conditions []string
		args       []interface{}
	)

	argIndex := 1

	if customerID != "" {
		conditions = append(
			conditions,
			fmt.Sprintf("customer_id = $%d", argIndex),
		)
		args = append(args, customerID)
		argIndex++
	}

	if status != "" {
		conditions = append(
			conditions,
			fmt.Sprintf("status = $%d", argIndex),
		)
		args = append(args, status)
		argIndex++
	}

	query := baseQuery

	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}

	query += " ORDER BY requested_at DESC"

	if limit <= 0 {
		limit = 50
	}

	if limit > 100 {
		limit = 100
	}

	if offset < 0 {
		offset = 0
	}

	query += fmt.Sprintf(
		" LIMIT $%d OFFSET $%d",
		argIndex,
		argIndex+1,
	)

	args = append(args, limit, offset)

	rows, err := r.db.Query(
		ctx,
		query,
		args...,
	)
	if err != nil {
		return nil, fmt.Errorf("list ride requests: %w", err)
	}
	defer rows.Close()

	requests := make([]*models.RideRequest, 0)

	for rows.Next() {
		request := &models.RideRequest{}

		err := rows.Scan(
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
			return nil, fmt.Errorf("scan ride request: %w", err)
		}

		requests = append(requests, request)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate ride requests: %w", err)
	}

	return requests, nil
}
