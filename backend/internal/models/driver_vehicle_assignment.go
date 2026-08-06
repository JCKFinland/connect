package models

import "time"

// DriverVehicleAssignment represents the operational assignment
// of a driver to a vehicle.
//
// This is independent of trip dispatching.
// It records which vehicle a driver is operating during a work shift.
type DriverVehicleAssignment struct {
	BaseModel
	SoftDelete

	// Tenant ownership.
	CompanyID string `db:"company_id" json:"company_id"`

	BranchID string `db:"branch_id" json:"branch_id"`

	FleetID string `db:"fleet_id" json:"fleet_id"`

	// Assignment.
	DriverID string `db:"driver_id" json:"driver_id"`

	VehicleID string `db:"vehicle_id" json:"vehicle_id"`

	// Assignment lifecycle.
	Status string `db:"status" json:"status"`

	AssignedAt time.Time `db:"assigned_at" json:"assigned_at"`

	ReleasedAt *time.Time `db:"released_at" json:"released_at,omitempty"`

	// Administrator who performed the assignment.
	AssignedBy string `db:"assigned_by" json:"assigned_by"`

	// Optional operational notes.
	Notes string `db:"notes" json:"notes"`

	IsActive bool `db:"is_active" json:"is_active"`
}