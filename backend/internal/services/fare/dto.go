package fare

// PricingSnapshot contains the immutable pricing inputs
// used to calculate a trip fare.
type PricingSnapshot struct {
	BaseFare             float64
	DistanceRatePerKM    float64
	TimeRatePerMinute    float64
	WaitingRatePerMinute float64
	BookingFee           float64
	SurgeMultiplier      float64
	DiscountAmount       float64
	TaxAmount            float64
	TollAmount           float64
	ParkingAmount        float64
	Currency             string
	PricingVersion       string
}

// CalculationInput contains the measured trip values
// used for fare calculation.
type CalculationInput struct {
	TripID string

	DistanceMeters  int64
	DurationSeconds int64
	WaitingSeconds  int64

	Pricing PricingSnapshot
}
