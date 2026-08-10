package postgres

import (
	"context"
	"fmt"
	"strings"

	"github.com/JCKFinland/connect/backend/internal/models"
)

// List retrieves active trips using optional filters and pagination.
func (r *TripRepository) List(
	ctx context.Context,
	companyID string,
	branchID string,
	status string,
	driverID string,
	customerID string,
	limit int,
	offset int,
) ([]*models.Trip, error) {
	const baseQuery = `
		SELECT
			id,
			ride_request_id,
			customer_id,
			driver_id,
			vehicle_id,
			company_id,
			branch_id,
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
			deleted_at,

			created_at,
			updated_at
		FROM trips
	`

	var (
		conditions []string
		args       []interface{}
	)

	// Only return active/non-deleted trips.
	conditions = append(conditions, "deleted_at IS NULL")

	argIndex := 1

	if companyID != "" {
		conditions = append(
			conditions,
			fmt.Sprintf("company_id = $%d", argIndex),
		)
		args = append(args, companyID)
		argIndex++
	}

	if branchID != "" {
		conditions = append(
			conditions,
			fmt.Sprintf("branch_id = $%d", argIndex),
		)
		args = append(args, branchID)
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

	if driverID != "" {
		conditions = append(
			conditions,
			fmt.Sprintf("driver_id = $%d", argIndex),
		)
		args = append(args, driverID)
		argIndex++
	}

	if customerID != "" {
		conditions = append(
			conditions,
			fmt.Sprintf("customer_id = $%d", argIndex),
		)
		args = append(args, customerID)
		argIndex++
	}

	query := baseQuery

	query += " WHERE " + strings.Join(conditions, " AND ")

	query += " ORDER BY created_at DESC"

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
		return nil, fmt.Errorf("list trips: %w", err)
	}
	defer rows.Close()

	trips := make([]*models.Trip, 0)

	for rows.Next() {
		trip := &models.Trip{}

		err := rows.Scan(
			&trip.ID,
			&trip.RideRequestID,
			&trip.CustomerID,
			&trip.DriverID,
			&trip.VehicleID,
			&trip.CompanyID,
			&trip.BranchID,
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

			&trip.PickupAddress,
			&trip.PickupLatitude,
			&trip.PickupLongitude,

			&trip.DropoffAddress,
			&trip.DropoffLatitude,
			&trip.DropoffLongitude,

			&trip.PassengerNote,

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

			&trip.CreatedAt,
			&trip.UpdatedAt,
		)

		if err != nil {
			return nil, fmt.Errorf("scan trip: %w", err)
		}

		trips = append(trips, trip)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate trips: %w", err)
	}

	return trips, nil
}
