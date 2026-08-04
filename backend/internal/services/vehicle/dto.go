package vehicle

// CreateVehicleRequest contains the payload required to register a vehicle.
type CreateVehicleRequest struct {
	CompanyID string `json:"company_id" binding:"required"`

	BranchID string `json:"branch_id" binding:"required"`

	FleetID string `json:"fleet_id" binding:"required"`

	RegistrationNumber string `json:"registration_number" binding:"required"`

	VIN string `json:"vin"`

	Make string `json:"make" binding:"required"`

	Model string `json:"model" binding:"required"`

	ModelYear int `json:"model_year"`

	Color string `json:"color"`

	VehicleType string `json:"vehicle_type" binding:"required"`

	SeatingCapacity int `json:"seating_capacity"`

	IsActive bool `json:"is_active"`
}

// UpdateVehicleRequest contains editable vehicle fields.
type UpdateVehicleRequest struct {
	CompanyID string `json:"company_id"`

	BranchID string `json:"branch_id"`

	FleetID string `json:"fleet_id"`

	RegistrationNumber string `json:"registration_number"`

	VIN string `json:"vin"`

	Make string `json:"make"`

	Model string `json:"model"`

	ModelYear int `json:"model_year"`

	Color string `json:"color"`

	VehicleType string `json:"vehicle_type"`

	SeatingCapacity int `json:"seating_capacity"`

	IsActive bool `json:"is_active"`
}

// VehicleResponse represents a vehicle returned to API clients.
type VehicleResponse struct {
	ID string `json:"id"`

	CompanyID string `json:"company_id"`

	BranchID string `json:"branch_id"`

	FleetID string `json:"fleet_id"`

	RegistrationNumber string `json:"registration_number"`

	VIN string `json:"vin"`

	Make string `json:"make"`

	Model string `json:"model"`

	ModelYear int `json:"model_year"`

	Color string `json:"color"`

	VehicleType string `json:"vehicle_type"`

	SeatingCapacity int `json:"seating_capacity"`

	IsActive bool `json:"is_active"`
}