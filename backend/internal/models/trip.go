package models

import "time"

// Trip represents an operational passenger trip.
//
// A trip is created from a ride request and progresses through
// the trip lifecycle independently of driver presence and
// driver-vehicle assignment.
type Trip struct {
	BaseModel
	SoftDelete

	// Source ride request.
	RideRequestID string `db:"ride_request_id" json:"ride_request_id"`

	// Passenger/customer requesting the trip.
	CustomerID string `db:"customer_id" json:"customer_id"`

	// Assigned operational resources.
	DriverID  string `db:"driver_id" json:"driver_id"`
	VehicleID string `db:"vehicle_id" json:"vehicle_id"`
	FleetID   string `db:"fleet_id" json:"fleet_id"`

	// Tenant ownership.
	CompanyID string `db:"company_id" json:"company_id"`
	BranchID  string `db:"branch_id" json:"branch_id"`

	ServiceCategoryID *string `db:"service_category_id" json:"service_category_id,omitempty"`

	// Trip lifecycle state.
	Status string `db:"status" json:"status"`

	// Estimated trip metrics.
	EstimatedDistanceKM      *float64 `db:"estimated_distance_km" json:"estimated_distance_km,omitempty"`
	EstimatedDurationMinutes *int     `db:"estimated_duration_minutes" json:"estimated_duration_minutes,omitempty"`

	EstimatedDistanceMeters  *int64 `db:"estimated_distance_meters" json:"estimated_distance_meters,omitempty"`
	EstimatedDurationSeconds *int64 `db:"estimated_duration_seconds" json:"estimated_duration_seconds,omitempty"`

	// Actual trip metrics.
	ActualDistanceKM      *float64 `db:"actual_distance_km" json:"actual_distance_km,omitempty"`
	ActualDurationMinutes *int     `db:"actual_duration_minutes" json:"actual_duration_minutes,omitempty"`

	ActualDistanceMeters  *int64 `db:"actual_distance_meters" json:"actual_distance_meters,omitempty"`
	ActualDurationSeconds *int64 `db:"actual_duration_seconds" json:"actual_duration_seconds,omitempty"`

	// Trip scheduling.
	AssignedAt  time.Time  `db:"assigned_at" json:"assigned_at"`
	ScheduledAt *time.Time `db:"scheduled_at" json:"scheduled_at,omitempty"`

	// Pickup information.
	PickupAddress   *string  `db:"pickup_address" json:"pickup_address,omitempty"`
	PickupLatitude  *float64 `db:"pickup_latitude" json:"pickup_latitude,omitempty"`
	PickupLongitude *float64 `db:"pickup_longitude" json:"pickup_longitude,omitempty"`

	// Destination information.
	DropoffAddress   *string  `db:"dropoff_address" json:"dropoff_address,omitempty"`
	DropoffLatitude  *float64 `db:"dropoff_latitude" json:"dropoff_latitude,omitempty"`
	DropoffLongitude *float64 `db:"dropoff_longitude" json:"dropoff_longitude,omitempty"`

	// Passenger instructions.
	PassengerNote *string `db:"passenger_note" json:"passenger_note,omitempty"`

	// Operational timestamps.
	DriverArrivedAt    *time.Time `db:"driver_arrived_at" json:"driver_arrived_at,omitempty"`
	PassengerOnBoardAt *time.Time `db:"passenger_on_board_at" json:"passenger_on_board_at,omitempty"`
	PickupAt           *time.Time `db:"pickup_at" json:"pickup_at,omitempty"`
	StartedAt          *time.Time `db:"started_at" json:"started_at,omitempty"`
	CompletedAt        *time.Time `db:"completed_at" json:"completed_at,omitempty"`
	CancelledAt        *time.Time `db:"cancelled_at" json:"cancelled_at,omitempty"`

	// Cancellation details.
	CancelledBy        *string `db:"cancelled_by" json:"cancelled_by,omitempty"`
	CancellationReason *string `db:"cancellation_reason" json:"cancellation_reason,omitempty"`

	// Active state.
	IsActive bool `db:"is_active" json:"is_active"`
}
