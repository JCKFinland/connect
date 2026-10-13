package models

import "time"

// FlexAuction represents a passenger-initiated negotiated-fare auction.
//
// Monetary amounts are expressed in integer minor units (cents for EUR).
// The PostgreSQL persistence layer converts these values to NUMERIC(10,2).
type FlexAuction struct {
	ID            string `db:"id" json:"id"`
	RideRequestID string `db:"ride_request_id" json:"ride_request_id"`

	PassengerOfferCents int64  `json:"passenger_offer_cents"`
	Currency            string `db:"currency" json:"currency"`

	// Immutable pricing provenance captured when the auction opens.
	// Monetary values are represented in integer EUR cents.
	BaselineCents     int64  `json:"baseline_cents"`
	MinimumOfferCents int64  `json:"minimum_offer_cents"`
	MaximumOfferCents int64  `json:"maximum_offer_cents"`
	PricingProfileID  string `db:"pricing_profile_id" json:"pricing_profile_id"`
	PricingVersion    string `db:"pricing_version" json:"pricing_version"`

	Status          string  `db:"status" json:"status"`
	SelectedOfferID *string `db:"selected_offer_id" json:"selected_offer_id,omitempty"`

	OpenedAt   time.Time  `db:"opened_at" json:"opened_at"`
	ExpiresAt  time.Time  `db:"expires_at" json:"expires_at"`
	ResolvedAt *time.Time `db:"resolved_at" json:"resolved_at,omitempty"`

	CreatedAt time.Time `db:"created_at" json:"created_at"`
	UpdatedAt time.Time `db:"updated_at" json:"updated_at"`
}

const (
	FlexAuctionStatusOpen                 = "OPEN"
	FlexAuctionStatusPendingAuthorization = "PENDING_AUTHORIZATION"
	FlexAuctionStatusConfirmed            = "CONFIRMED"
	FlexAuctionStatusExpired              = "EXPIRED"
	FlexAuctionStatusCancelled            = "CANCELLED"
	FlexAuctionStatusFailed               = "FAILED"
)
