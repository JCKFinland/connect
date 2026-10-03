package assignment

type AssignDriverRequest struct {
	VehicleID string `json:"vehicle_id" binding:"required"`

	Notes string `json:"notes"`
}
