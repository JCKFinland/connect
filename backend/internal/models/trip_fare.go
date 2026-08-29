package models

import "time"

// TripFare represents the authoritative monetary result
// and pricing snapshot for a completed trip.
type TripFare struct {
	BaseModel

	TripID string `db:"trip_id" json:"trip_id"`

	BaseFare     float64 `db:"base_fare" json:"base_fare"`
	DistanceFare float64 `db:"distance_fare" json:"distance_fare"`
	TimeFare     float64 `db:"time_fare" json:"time_fare"`
	WaitingFare  float64 `db:"waiting_fare" json:"waiting_fare"`
	BookingFee   float64 `db:"booking_fee" json:"booking_fee"`

	SurgeMultiplier float64 `db:"surge_multiplier" json:"surge_multiplier"`
	SurgeAmount     float64 `db:"surge_amount" json:"surge_amount"`

	DiscountAmount float64 `db:"discount_amount" json:"discount_amount"`
	TaxAmount      float64 `db:"tax_amount" json:"tax_amount"`
	TollAmount     float64 `db:"toll_amount" json:"toll_amount"`
	ParkingAmount  float64 `db:"parking_amount" json:"parking_amount"`

	TotalAmount float64 `db:"total_amount" json:"total_amount"`
	Currency    string  `db:"currency" json:"currency"`

	// Frozen pricing inputs used to calculate this fare.
	DistanceRatePerKM    float64 `db:"distance_rate_per_km" json:"distance_rate_per_km"`
	TimeRatePerMinute    float64 `db:"time_rate_per_minute" json:"time_rate_per_minute"`
	WaitingRatePerMinute float64 `db:"waiting_rate_per_minute" json:"waiting_rate_per_minute"`

	// Meter values actually used for billing.
	ChargedDistanceMeters  int64 `db:"charged_distance_meters" json:"charged_distance_meters"`
	ChargedDurationSeconds int64 `db:"charged_duration_seconds" json:"charged_duration_seconds"`
	WaitingDurationSeconds int64 `db:"waiting_duration_seconds" json:"waiting_duration_seconds"`

	PricingVersion string `db:"pricing_version" json:"pricing_version"`

	CalculatedAt time.Time `db:"calculated_at" json:"calculated_at"`
}
