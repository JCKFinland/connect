package models

import "time"

// DriverEarning represents the authoritative financial entitlement
// created for a driver from a paid completed trip.
//
// Monetary values are represented as decimal strings so PostgreSQL
// NUMERIC values are never converted through binary floating-point.
type DriverEarning struct {
	BaseModel

	TripID    string  `db:"trip_id" json:"trip_id"`
	DriverID  string  `db:"driver_id" json:"driver_id"`
	CompanyID string  `db:"company_id" json:"company_id"`
	FareID    string  `db:"fare_id" json:"fare_id"`
	PaymentID *string `db:"payment_id" json:"payment_id,omitempty"`

	GrossAmount      string `db:"gross_amount" json:"gross_amount"`
	CommissionAmount string `db:"commission_amount" json:"commission_amount"`
	BonusAmount      string `db:"bonus_amount" json:"bonus_amount"`
	TipAmount        string `db:"tip_amount" json:"tip_amount"`
	AdjustmentAmount string `db:"adjustment_amount" json:"adjustment_amount"`
	TaxWithheld      string `db:"tax_withheld" json:"tax_withheld"`
	NetAmount        string `db:"net_amount" json:"net_amount"`

	Currency         string `db:"currency" json:"currency"`
	SettlementStatus string `db:"settlement_status" json:"settlement_status"`

	CalculatedAt time.Time  `db:"calculated_at" json:"calculated_at"`
	SettledAt    *time.Time `db:"settled_at" json:"settled_at,omitempty"`
}

// DriverEarningsSummary contains authoritative aggregate earnings
// for an authenticated driver.
type DriverEarningsSummary struct {
	GrossAmount      string `json:"gross_amount"`
	CommissionAmount string `json:"commission_amount"`
	BonusAmount      string `json:"bonus_amount"`
	TipAmount        string `json:"tip_amount"`
	AdjustmentAmount string `json:"adjustment_amount"`
	TaxWithheld      string `json:"tax_withheld"`
	NetAmount        string `json:"net_amount"`
	Currency         string `json:"currency"`
	TripCount        int64  `json:"trip_count"`
}

// DriverEarningsDashboard contains the driver's aggregate earnings
// and their most recent authoritative earning records.
type DriverEarningsDashboard struct {
	Summary        DriverEarningsSummary `json:"summary"`
	RecentEarnings []DriverEarning       `json:"recent_earnings"`
}
