package postgres

import (
	"context"

	"github.com/JCKFinland/connect/backend/internal/models"
)

// GetByIDForUpdate returns a driver and locks the row for the
// duration of the current database transaction.
func (r *DriverRepository) GetByIDForUpdate(
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
			verified_at,
            verified_by_user_id,
			is_active,
			created_at,
			updated_at,
			deleted_at
		FROM drivers
		WHERE id = $1
		  AND deleted_at IS NULL
		FOR UPDATE
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
		&driver.VerifiedAt,
		&driver.VerifiedByUserID,
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
