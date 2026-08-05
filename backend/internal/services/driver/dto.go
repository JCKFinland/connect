package driver

import "time"

// CreateDriverRequest contains the payload required to create a driver.
type CreateDriverRequest struct {
	UserID string `json:"user_id" binding:"required"`

	CompanyID string `json:"company_id" binding:"required"`

	BranchID string `json:"branch_id" binding:"required"`

	DriverNumber string `json:"driver_number" binding:"required"`

	FirstName string `json:"first_name" binding:"required"`

	LastName string `json:"last_name" binding:"required"`

	Phone string `json:"phone" binding:"required"`

	Email string `json:"email"`

	TaxiDriverLicenseNumber string `json:"taxi_driver_license_number" binding:"required"`

	DrivingLicenseNumber string `json:"driving_license_number" binding:"required"`

	DrivingLicenseExpiry *time.Time `json:"driving_license_expiry,omitempty"`

	HireDate *time.Time `json:"hire_date,omitempty"`

	Status string `json:"status"`

	IsVerified bool `json:"is_verified"`

	IsActive bool `json:"is_active"`
}

// UpdateDriverRequest contains mutable driver fields.
type UpdateDriverRequest struct {
	CompanyID string `json:"company_id"`

	BranchID string `json:"branch_id"`

	DriverNumber string `json:"driver_number"`

	FirstName string `json:"first_name"`

	LastName string `json:"last_name"`

	Phone string `json:"phone"`

	Email string `json:"email"`

	TaxiDriverLicenseNumber string `json:"taxi_driver_license_number"`

	DrivingLicenseNumber string `json:"driving_license_number"`

	DrivingLicenseExpiry *time.Time `json:"driving_license_expiry,omitempty"`

	HireDate *time.Time `json:"hire_date,omitempty"`

	Status string `json:"status"`

	IsVerified bool `json:"is_verified"`

	IsActive bool `json:"is_active"`
}