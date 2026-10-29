package postgres

import (
	"context"
	"fmt"

	"github.com/JCKFinland/connect/backend/internal/models"
)

// ListForCompanyMember returns trips only from companies where userID
// has an explicit membership. Membership is enforced in SQL before
// ordering and pagination.
func (r *TripRepository) ListForCompanyMember(
	ctx context.Context,
	userID string,
	companyID string,
	branchID string,
	status string,
	driverID string,
	customerID string,
	limit int,
	offset int,
) ([]*models.Trip, error) {
	if userID == "" {
		return nil, fmt.Errorf("user ID is required")
	}

	query := `
        SELECT
            t.id,
            t.ride_request_id,
            t.customer_id,
            t.driver_id,
            t.vehicle_id,
            t.company_id,
            t.branch_id,
            t.service_category_id,
            t.pricing_profile_id,
            t.fleet_id,
            t.status,
            t.estimated_distance_km,
            t.estimated_duration_minutes,
            t.actual_distance_km,
            t.actual_duration_minutes,
            t.estimated_distance_meters,
            t.estimated_duration_seconds,
            t.actual_distance_meters,
            t.actual_duration_seconds,
            t.assigned_at,
            t.scheduled_at,
            t.pickup_address,
            t.pickup_latitude,
            t.pickup_longitude,
            t.dropoff_address,
            t.dropoff_latitude,
            t.dropoff_longitude,
            t.passenger_note,
            t.driver_arrived_at,
            t.started_at,
            t.completed_at,
            t.cancelled_at,
            t.cancelled_by,
            t.cancellation_reason,
            t.is_active,
            t.deleted_at,
            t.created_at,
            t.updated_at
        FROM trips t
        WHERE t.deleted_at IS NULL
          AND EXISTS (
              SELECT 1
              FROM company_memberships cm
              WHERE cm.user_id = $1
                AND cm.company_id = t.company_id
          )

    `

	if limit <= 0 {
		limit = 50
	}
	if limit > 100 {
		limit = 100
	}
	if offset < 0 {
		return nil, fmt.Errorf("offset cannot be negative")
	}

	args := []interface{}{userID}
	argIndex := 2

	filters := []struct {
		column string
		value  string
	}{
		{"t.company_id", companyID},
		{"t.branch_id", branchID},
		{"t.status", status},
		{"t.driver_id", driverID},
		{"t.customer_id", customerID},
	}

	for _, filter := range filters {
		if filter.value == "" {
			continue
		}

		query += fmt.Sprintf(
			" AND %s = $%d",
			filter.column,
			argIndex,
		)

		args = append(args, filter.value)
		argIndex++
	}

	query += fmt.Sprintf(
		" ORDER BY t.created_at DESC, t.id DESC LIMIT $%d OFFSET $%d",
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
		return nil, fmt.Errorf("list company-member trips: %w", err)
	}
	defer rows.Close()

	trips := make([]*models.Trip, 0)

	for rows.Next() {
		trip := &models.Trip{}

		if err := rows.Scan(
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
			&trip.PickupAddress,
			&trip.PickupLatitude,
			&trip.PickupLongitude,
			&trip.DropoffAddress,
			&trip.DropoffLatitude,
			&trip.DropoffLongitude,
			&trip.PassengerNote,
			&trip.DriverArrivedAt,
			&trip.StartedAt,
			&trip.CompletedAt,
			&trip.CancelledAt,
			&trip.CancelledBy,
			&trip.CancellationReason,
			&trip.IsActive,
			&trip.DeletedAt,
			&trip.CreatedAt,
			&trip.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan company-member trip: %w", err)
		}

		trips = append(trips, trip)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate company-member trips: %w", err)
	}

	return trips, nil
}
