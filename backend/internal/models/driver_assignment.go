package models

import "time"

// DriverAssignment represents a driver's assignment to a vehicle.
type DriverAssignment struct {
	BaseModel

	CompanyID string `db:"company_id" json:"company_id"`

	BranchID string `db:"branch_id" json:"branch_id"`

	FleetID string `db:"fleet_id" json:"fleet_id"`

	DriverID string `db:"driver_id" json:"driver_id"`

	VehicleID string `db:"vehicle_id" json:"vehicle_id"`

	AssignedAt time.Time `db:"assigned_at" json:"assigned_at"`

	UnassignedAt *time.Time `db:"unassigned_at" json:"unassigned_at,omitempty"`

	Notes string `db:"notes" json:"notes"`
}