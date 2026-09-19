package fare_estimate

type EstimateRequest struct {
	PickupLatitude  float64
	PickupLongitude float64

	DestinationLatitude  float64
	DestinationLongitude float64

	ServiceCategoryID string
}

type EstimateResult struct {
	DistanceMeters  int64 `json:"distance_meters"`
	DurationSeconds int64 `json:"duration_seconds"`

	BaseFare     float64 `json:"base_fare"`
	DistanceFare float64 `json:"distance_fare"`
	TimeFare     float64 `json:"time_fare"`
	BookingFee   float64 `json:"booking_fee"`
	SurgeAmount  float64 `json:"surge_amount"`

	TotalAmount float64 `json:"total_amount"`
	Currency    string  `json:"currency"`

	PricingProfileID string `json:"pricing_profile_id"`
	PricingVersion   string `json:"pricing_version"`
}
