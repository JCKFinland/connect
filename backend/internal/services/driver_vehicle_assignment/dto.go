package driver_vehicle_assignment

// AssignDriverVehicleRequest represents a request to assign
// a driver to a vehicle.
type AssignDriverVehicleRequest struct {
	CompanyID string `json:"company_id" binding:"required"`

	BranchID string `json:"branch_id" binding:"required"`

	FleetID string `json:"fleet_id" binding:"required"`

	DriverID string `json:"driver_id" binding:"required"`

	VehicleID string `json:"vehicle_id" binding:"required"`

	AssignedBy string `json:"assigned_by" binding:"required"`

	Notes string `json:"notes"`
}

// ReleaseDriverVehicleRequest represents a request to release
// an active driver-vehicle assignment.
type ReleaseDriverVehicleRequest struct {
	Notes string `json:"notes"`
}