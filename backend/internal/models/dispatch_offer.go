package models

import "time"

// DispatchOffer represents an offer of a ride request to a specific driver.
type DispatchOffer struct {
	ID string `db:"id" json:"id"`

	RideRequestID string `db:"ride_request_id" json:"ride_request_id"`

	DriverID  string `db:"driver_id" json:"driver_id"`
	VehicleID string `db:"vehicle_id" json:"vehicle_id"`

	CompanyID string `db:"company_id" json:"company_id"`
	BranchID  string `db:"branch_id" json:"branch_id"`
	FleetID   string `db:"fleet_id" json:"fleet_id"`

	Status string `db:"status" json:"status"`

	OfferedAt time.Time `db:"offered_at" json:"offered_at"`
	ExpiresAt time.Time `db:"expires_at" json:"expires_at"`

	RespondedAt *time.Time `db:"responded_at" json:"responded_at,omitempty"`

	RejectionReason *string `db:"rejection_reason" json:"rejection_reason,omitempty"`

	CreatedBy *string `db:"created_by" json:"created_by,omitempty"`

	CreatedAt time.Time `db:"created_at" json:"created_at"`
	UpdatedAt time.Time `db:"updated_at" json:"updated_at"`
}
