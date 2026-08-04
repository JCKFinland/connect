package models

// Vehicle represents a taxi vehicle belonging to a fleet.
type Vehicle struct {
	BaseModel
	SoftDelete

	CompanyID string `db:"company_id" json:"company_id"`

	BranchID string `db:"branch_id" json:"branch_id"`

	FleetID string `db:"fleet_id" json:"fleet_id"`

	RegistrationNumber string `db:"registration_number" json:"registration_number"`

	VIN string `db:"vin" json:"vin"`

	Make string `db:"make" json:"make"`

	Model string `db:"model" json:"model"`

	ModelYear int `db:"model_year" json:"model_year"`

	Color string `db:"color" json:"color"`

	VehicleType string `db:"vehicle_type" json:"vehicle_type"`

	SeatingCapacity int `db:"seating_capacity" json:"seating_capacity"`

	IsActive bool `db:"is_active" json:"is_active"`
}