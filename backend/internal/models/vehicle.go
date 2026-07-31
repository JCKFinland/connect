package models

// Vehicle represents a vehicle available for trips.
type Vehicle struct {
	BaseModel
	SoftDelete

	CompanyID string `db:"company_id" json:"company_id"`

	BranchID string `db:"branch_id" json:"branch_id"`

	FleetID string `db:"fleet_id" json:"fleet_id"`

	RegistrationNumber string `db:"registration_number" json:"registration_number"`

	Make string `db:"make" json:"make"`

	Model string `db:"model" json:"model"`

	Year int `db:"year" json:"year"`

	Color string `db:"color" json:"color"`

	VehicleType string `db:"vehicle_type" json:"vehicle_type"`

	SeatCount int `db:"seat_count" json:"seat_count"`

	IsActive bool `db:"is_active" json:"is_active"`
}