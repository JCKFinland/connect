package models

import "time"

// RideRequest represents a customer's request for a ride.
type RideRequest struct {
	BaseModel

	// DispatchRetryCount records consecutive automatic dispatch attempts that
	// failed because no eligible driver was available.
	//
	// It is operational telemetry and backoff state only. It does not terminate
	// the ride request; expires_at is the authoritative matching deadline.
	DispatchRetryCount int `db:"dispatch_retry_count" json:"dispatch_retry_count"`

	CustomerID string `db:"customer_id" json:"customer_id"`

	PickupAddress   string  `db:"pickup_address" json:"pickup_address"`
	PickupLatitude  float64 `db:"pickup_latitude" json:"pickup_latitude"`
	PickupLongitude float64 `db:"pickup_longitude" json:"pickup_longitude"`

	DestinationAddress   string  `db:"destination_address" json:"destination_address"`
	DestinationLatitude  float64 `db:"destination_latitude" json:"destination_latitude"`
	DestinationLongitude float64 `db:"destination_longitude" json:"destination_longitude"`

	RequestedVehicleType string `db:"requested_vehicle_type" json:"requested_vehicle_type"`

	ServiceCategoryID *string `db:"service_category_id" json:"service_category_id,omitempty"`

	PassengerCount int    `db:"passenger_count" json:"passenger_count"`
	Status         string `db:"status" json:"status"`

	Notes string `db:"notes" json:"notes"`

	RequestedAt time.Time  `db:"requested_at" json:"requested_at"`
	ExpiresAt   *time.Time `db:"expires_at" json:"expires_at,omitempty"`

	NextDispatchAttemptAt *time.Time `db:"next_dispatch_attempt_at" json:"next_dispatch_attempt_at,omitempty"`

	LastDispatchAttemptAt *time.Time `db:"last_dispatch_attempt_at" json:"last_dispatch_attempt_at,omitempty"`
}
