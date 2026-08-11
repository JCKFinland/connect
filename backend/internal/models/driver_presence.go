package models

import "time"

// DriverPresence represents the driver's live dispatch state.
type DriverPresence struct {
	DriverID string `db:"driver_id" json:"driver_id"`

	CompanyID string `db:"company_id" json:"company_id"`

	BranchID *string `db:"branch_id" json:"branch_id,omitempty"`

	VehicleID *string `db:"vehicle_id" json:"vehicle_id,omitempty"`

	AssignmentID *string `db:"assignment_id" json:"assignment_id,omitempty"`

	IsOnline bool `db:"is_online" json:"is_online"`

	AvailabilityStatus string `db:"availability_status" json:"availability_status"`

	Latitude *float64 `db:"latitude" json:"latitude,omitempty"`

	Longitude *float64 `db:"longitude" json:"longitude,omitempty"`

	Heading *float64 `db:"heading" json:"heading,omitempty"`

	Speed *float64 `db:"speed" json:"speed,omitempty"`

	Accuracy *float64 `db:"accuracy" json:"accuracy,omitempty"`

	LastHeartbeatAt *time.Time `db:"last_heartbeat_at" json:"last_heartbeat_at,omitempty"`

	CreatedAt time.Time `db:"created_at" json:"created_at"`

	UpdatedAt time.Time `db:"updated_at" json:"updated_at"`
}
