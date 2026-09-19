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

// CalculationResult contains deterministic fare calculation output
// independent of trip persistence.
type CalculationResult struct {
	BaseFare     float64
	DistanceFare float64
	TimeFare     float64
	WaitingFare  float64
	BookingFee   float64

	SurgeMultiplier float64
	SurgeAmount     float64

	DiscountAmount float64
	TaxAmount      float64
	TollAmount     float64
	ParkingAmount  float64

	TotalAmount float64
	Currency    string

	DistanceRatePerKM    float64
	TimeRatePerMinute    float64
	WaitingRatePerMinute float64

	DistanceMeters  int64
	DurationSeconds int64
	WaitingSeconds  int64

	PricingVersion string
}

// EstimateInput contains route metrics and pricing inputs that do not
// require an existing trip.
type EstimateInput struct {
	DistanceMeters  int64
	DurationSeconds int64
	WaitingSeconds  int64

	Pricing PricingSnapshot
}
