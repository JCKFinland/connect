package dispatch

import "time"

// PendingOfferResponse is the driver-safe representation of an active
// dispatch offer together with the ride information required for the
// driver to make an accept/reject decision.
type PendingOfferResponse struct {
	ID            string `json:"id"`
	RideRequestID string `json:"ride_request_id"`
	VehicleID     string `json:"vehicle_id"`
	Status        string `json:"status"`

	OfferedAt time.Time `json:"offered_at"`
	ExpiresAt time.Time `json:"expires_at"`

	Ride PendingOfferRide `json:"ride"`
}

type PendingOfferRide struct {
	PickupAddress   string  `json:"pickup_address"`
	PickupLatitude  float64 `json:"pickup_latitude"`
	PickupLongitude float64 `json:"pickup_longitude"`

	DestinationAddress   string  `json:"destination_address"`
	DestinationLatitude  float64 `json:"destination_latitude"`
	DestinationLongitude float64 `json:"destination_longitude"`

	RequestedVehicleType string  `json:"requested_vehicle_type"`
	ServiceCategoryID    *string `json:"service_category_id,omitempty"`

	PassengerCount int    `json:"passenger_count"`
	Notes          string `json:"notes"`

	RequestedAt time.Time `json:"requested_at"`
}
