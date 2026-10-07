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
}

// RegisterDriverRequest contains the driver-supplied fields for
// authenticated self-service driver registration.
//
// Account identity is derived from the authenticated user.
// Verification and operational state are controlled by CONNECT.
type RegisterDriverRequest struct {
	CompanyID string `json:"company_id" binding:"required"`

	BranchID string `json:"branch_id" binding:"required"`

	TaxiDriverLicenseNumber string `json:"taxi_driver_license_number" binding:"required"`

	DrivingLicenseNumber string `json:"driving_license_number" binding:"required"`

	DrivingLicenseExpiry *time.Time `json:"driving_license_expiry" binding:"required"`
}

// UpdateDriverRequest contains profile and employment details that may be
// changed without mutating account identity, tenant/branch authority,
// regulatory credentials, verification state, or operational lifecycle state.
type UpdateDriverRequest struct {
	DriverNumber string `json:"driver_number"`

	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`

	Phone string `json:"phone"`
	Email string `json:"email"`

	HireDate *time.Time `json:"hire_date,omitempty"`
}
