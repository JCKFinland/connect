package models

import "time"

// Driver represents the permanent driver profile within CONNECT.
// Operational state (presence, assignments, trips) is stored separately.
type Driver struct {
	BaseModel
	SoftDelete

	// Linked platform user account.
	UserID string `db:"user_id" json:"user_id"`

	// Current employing company.
	CompanyID string `db:"company_id" json:"company_id"`

	// Current operating branch.
	BranchID string `db:"branch_id" json:"branch_id"`

	// Driver identification.
	DriverNumber string `db:"driver_number" json:"driver_number"`

	// Personal details.
	FirstName string `db:"first_name" json:"first_name"`

	LastName string `db:"last_name" json:"last_name"`

	Phone string `db:"phone" json:"phone"`

	Email string `db:"email" json:"email"`

	// Regulatory documents.
	TaxiDriverLicenseNumber string `db:"taxi_driver_license_number" json:"taxi_driver_license_number"`

	DrivingLicenseNumber string `db:"driving_license_number" json:"driving_license_number"`

	DrivingLicenseExpiry *time.Time `db:"driving_license_expiry" json:"driving_license_expiry,omitempty"`

	// Employment.
	HireDate *time.Time `db:"hire_date" json:"hire_date,omitempty"`

	Status string `db:"status" json:"status"`

	// Compliance.
	IsVerified bool `db:"is_verified" json:"is_verified"`

	IsActive bool `db:"is_active" json:"is_active"`
}