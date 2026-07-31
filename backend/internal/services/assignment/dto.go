package assignment

type AssignDriverRequest struct {
	CompanyID string

	BranchID string

	FleetID string

	DriverID string

	VehicleID string

	Notes string
}

type UnassignDriverRequest struct {
	DriverID string
}
