package postgres

import (
	"context"

	"github.com/JCKFinland/connect/backend/internal/models"
)

// GetByID returns a single driver by its ID.
func (r *DriverRepository) GetByID(
	ctx context.Context,
	id string,
) (*models.Driver, error) {

	query := `
		SELECT
			id,
			user_id,
			company_id,
			branch_id,
			driver_number,
			first_name,
			last_name,
			phone,
			email,
			taxi_driver_license_number,
			driving_license_number,
			driving_license_expiry,
			hire_date,
			status,
			is_verified,
			is_active,
			created_at,
			updated_at,
			deleted_at
		FROM drivers
		WHERE id = $1
		  AND deleted_at IS NULL
	`

	driver := &models.Driver{}

	err := r.db.QueryRow(
		ctx,
		query,
		id,
	).Scan(
		&driver.ID,
		&driver.UserID,
		&driver.CompanyID,
		&driver.BranchID,
		&driver.DriverNumber,
		&driver.FirstName,
		&driver.LastName,
		&driver.Phone,
		&driver.Email,
		&driver.TaxiDriverLicenseNumber,
		&driver.DrivingLicenseNumber,
		&driver.DrivingLicenseExpiry,
		&driver.HireDate,
		&driver.Status,
		&driver.IsVerified,
		&driver.IsActive,
		&driver.CreatedAt,
		&driver.UpdatedAt,
		&driver.DeletedAt,
	)

	if err != nil {
		return nil, err
	}

	return driver, nil
}