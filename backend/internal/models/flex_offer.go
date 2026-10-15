package models

import "time"

// FlexOffer represents a driver's negotiated-fare bid.
//
// DriverID is the operational users.id identity, not drivers.id.
// AssignmentID pins the authoritative driver/vehicle/organization identity.
type FlexOffer struct {
	ID        string `db:"id" json:"id"`
	AuctionID string `db:"auction_id" json:"auction_id"`

	AssignmentID string `db:"assignment_id" json:"assignment_id"`
	DriverID     string `db:"driver_id" json:"driver_id"`
	VehicleID    string `db:"vehicle_id" json:"vehicle_id"`
	CompanyID    string `db:"company_id" json:"company_id"`
	BranchID     string `db:"branch_id" json:"branch_id"`
	FleetID      string `db:"fleet_id" json:"fleet_id"`

	OfferedAmountCents int64 `json:"offered_amount_cents"`

	// OfferType distinguishes acceptance from a driver counter-offer.
	// It is assigned at creation and must not change afterward.
	OfferType string `db:"offer_type" json:"offer_type"`

	Status string `db:"status" json:"status"`

	OfferedAt   time.Time  `db:"offered_at" json:"offered_at"`
	ExpiresAt   time.Time  `db:"expires_at" json:"expires_at"`
	RespondedAt *time.Time `db:"responded_at" json:"responded_at,omitempty"`

	CreatedAt time.Time `db:"created_at" json:"created_at"`
	UpdatedAt time.Time `db:"updated_at" json:"updated_at"`
}

const (
	FlexOfferTypeAcceptFare   = "ACCEPT_FARE"
	FlexOfferTypeCounterOffer = "COUNTER_OFFER"
)

const (
	FlexOfferStatusPending   = "PENDING"
	FlexOfferStatusSelected  = "SELECTED"
	FlexOfferStatusAccepted  = "ACCEPTED"
	FlexOfferStatusRejected  = "REJECTED"
	FlexOfferStatusExpired   = "EXPIRED"
	FlexOfferStatusCancelled = "CANCELLED"
)
