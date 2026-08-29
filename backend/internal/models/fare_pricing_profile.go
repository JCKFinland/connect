package models

import "time"

// FarePricingProfile represents an authoritative, effective-dated
// pricing definition controlled by CONNECT.
//
// Pricing profiles are versioned rather than overwritten so that
// historical trip fares remain auditable and reproducible.
type FarePricingProfile struct {
	ID string `db:"id" json:"id"`

	CompanyID         string  `db:"company_id" json:"company_id"`
	BranchID          *string `db:"branch_id" json:"branch_id,omitempty"`
	ServiceCategoryID string  `db:"service_category_id" json:"service_category_id"`

	Version  string `db:"version" json:"version"`
	Currency string `db:"currency" json:"currency"`

	BaseFare             float64 `db:"base_fare" json:"base_fare"`
	DistanceRatePerKM    float64 `db:"distance_rate_per_km" json:"distance_rate_per_km"`
	TimeRatePerMinute    float64 `db:"time_rate_per_minute" json:"time_rate_per_minute"`
	WaitingRatePerMinute float64 `db:"waiting_rate_per_minute" json:"waiting_rate_per_minute"`
	BookingFee           float64 `db:"booking_fee" json:"booking_fee"`
	SurgeMultiplier      float64 `db:"surge_multiplier" json:"surge_multiplier"`

	EffectiveFrom time.Time  `db:"effective_from" json:"effective_from"`
	EffectiveTo   *time.Time `db:"effective_to" json:"effective_to,omitempty"`

	IsActive bool `db:"is_active" json:"is_active"`

	CreatedByUserID *string `db:"created_by_user_id" json:"created_by_user_id,omitempty"`

	CreatedAt time.Time `db:"created_at" json:"created_at"`
}
