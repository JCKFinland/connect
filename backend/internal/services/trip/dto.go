package trip

import "time"

// CreateTripRequest represents the data required to create a trip.
type CreateTripRequest struct {
	RideRequestID string `json:"ride_request_id" binding:"required"`

	CustomerID string `json:"customer_id" binding:"required"`

	DriverID string `json:"driver_id" binding:"required"`

	VehicleID string `json:"vehicle_id" binding:"required"`

	CompanyID string `json:"company_id" binding:"required"`

	BranchID string `json:"branch_id" binding:"required"`

	FleetID string `json:"fleet_id" binding:"required"`

	ScheduledAt *time.Time `json:"scheduled_at,omitempty"`

	PickupAddress   *string  `json:"pickup_address,omitempty"`
	PickupLatitude  *float64 `json:"pickup_latitude,omitempty"`
	PickupLongitude *float64 `json:"pickup_longitude,omitempty"`

	DropoffAddress   *string  `json:"dropoff_address,omitempty"`
	DropoffLatitude  *float64 `json:"dropoff_latitude,omitempty"`
	DropoffLongitude *float64 `json:"dropoff_longitude,omitempty"`

	PassengerNote *string `json:"passenger_note,omitempty"`

	EstimatedDistanceKM      *float64 `json:"estimated_distance_km,omitempty"`
	EstimatedDurationMinutes *int     `json:"estimated_duration_minutes,omitempty"`

	EstimatedDistanceMeters  *int64 `json:"estimated_distance_meters,omitempty"`
	EstimatedDurationSeconds *int64 `json:"estimated_duration_seconds,omitempty"`
}

// UpdateTripRequest represents editable trip information.
type UpdateTripRequest struct {
	ScheduledAt *time.Time `json:"scheduled_at,omitempty"`

	PickupAddress   *string  `json:"pickup_address,omitempty"`
	PickupLatitude  *float64 `json:"pickup_latitude,omitempty"`
	PickupLongitude *float64 `json:"pickup_longitude,omitempty"`

	DropoffAddress   *string  `json:"dropoff_address,omitempty"`
	DropoffLatitude  *float64 `json:"dropoff_latitude,omitempty"`
	DropoffLongitude *float64 `json:"dropoff_longitude,omitempty"`

	PassengerNote *string `json:"passenger_note,omitempty"`

	EstimatedDistanceKM      *float64 `json:"estimated_distance_km,omitempty"`
	EstimatedDurationMinutes *int     `json:"estimated_duration_minutes,omitempty"`

	ActualDistanceKM      *float64 `json:"actual_distance_km,omitempty"`
	ActualDurationMinutes *int     `json:"actual_duration_minutes,omitempty"`

	EstimatedDistanceMeters  *int64 `json:"estimated_distance_meters,omitempty"`
	EstimatedDurationSeconds *int64 `json:"estimated_duration_seconds,omitempty"`

	ActualDistanceMeters  *int64 `json:"actual_distance_meters,omitempty"`
	ActualDurationSeconds *int64 `json:"actual_duration_seconds,omitempty"`
}

// UpdateTripStatusRequest represents a trip lifecycle transition.
type UpdateTripStatusRequest struct {
	Status string `json:"status" binding:"required"`
}

// AssignDriverRequest represents a driver and vehicle assignment.
type AssignDriverRequest struct {
	DriverID  string `json:"driver_id" binding:"required"`
	VehicleID string `json:"vehicle_id" binding:"required"`
}

// ListTripsRequest represents trip filtering and pagination.
type ListTripsRequest struct {
	CompanyID  string `form:"company_id"`
	BranchID   string `form:"branch_id"`
	Status     string `form:"status"`
	DriverID   string `form:"driver_id"`
	CustomerID string `form:"customer_id"`

	Limit  int `form:"limit"`
	Offset int `form:"offset"`
}
