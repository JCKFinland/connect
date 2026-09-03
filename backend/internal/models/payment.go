package models

import "time"

// Payment represents the authoritative payment obligation
// created from the immutable fare of a completed trip.
//
// Amount is represented as a decimal string so PostgreSQL NUMERIC
// values are not converted through binary floating-point.
type Payment struct {
	BaseModel

	TripID     string `db:"trip_id" json:"trip_id"`
	FareID     string `db:"fare_id" json:"fare_id"`
	CustomerID string `db:"customer_id" json:"customer_id"`

	Status        string `db:"status" json:"status"`
	PaymentMethod string `db:"payment_method" json:"payment_method"`

	Amount   string `db:"amount" json:"amount"`
	Currency string `db:"currency" json:"currency"`

	PaidAt *time.Time `db:"paid_at" json:"paid_at,omitempty"`
}
