package ride_request

import "time"

// CreateRideRequestRequest contains the data required to create a ride request.
type CreateRideRequestRequest struct {
	CustomerID string `json:"customer_id" binding:"required"`

	PickupAddress   string  `json:"pickup_address" binding:"required"`
	PickupLatitude  float64 `json:"pickup_latitude" binding:"required"`
	PickupLongitude float64 `json:"pickup_longitude" binding:"required"`

	DestinationAddress   string  `json:"destination_address" binding:"required"`
	DestinationLatitude  float64 `json:"destination_latitude" binding:"required"`
	DestinationLongitude float64 `json:"destination_longitude" binding:"required"`

	RequestedVehicleType string `json:"requested_vehicle_type"`
	PassengerCount       int    `json:"passenger_count"`

	Notes string `json:"notes"`

	ExpiresAt *time.Time `json:"expires_at,omitempty"`
}

// UpdateRideRequestRequest contains editable ride request fields.
type UpdateRideRequestRequest struct {
	PickupAddress   string  `json:"pickup_address" binding:"required"`
	PickupLatitude  float64 `json:"pickup_latitude" binding:"required"`
	PickupLongitude float64 `json:"pickup_longitude" binding:"required"`

	DestinationAddress   string  `json:"destination_address" binding:"required"`
	DestinationLatitude  float64 `json:"destination_latitude" binding:"required"`
	DestinationLongitude float64 `json:"destination_longitude" binding:"required"`

	RequestedVehicleType string `json:"requested_vehicle_type"`
	PassengerCount       int    `json:"passenger_count"`

	Notes string `json:"notes"`

	ExpiresAt *time.Time `json:"expires_at,omitempty"`
}

// UpdateRideRequestStatusRequest changes the lifecycle state.
type UpdateRideRequestStatusRequest struct {
	Status string `json:"status" binding:"required"`
}
