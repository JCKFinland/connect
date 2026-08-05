package postgres

import (
	"context"

	"github.com/JCKFinland/connect/backend/internal/models"
)

// Create inserts a new driver into the database.
func (r *DriverRepository) Create(
	ctx context.Context,
	driver *models.Driver,
) error {

	query := `
		INSERT INTO drivers
		(
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
			updated_at
		)
		VALUES
		(
			$1,$2,$3,$4,$5,$6,$7,$8,$9,
			$10,$11,$12,$13,$14,$15,$16,$17,$18
		)
	`

	_, err := r.db.Exec(
		ctx,
		query,
		driver.ID,
		driver.UserID,
		driver.CompanyID,
		driver.BranchID,
		driver.DriverNumber,
		driver.FirstName,
		driver.LastName,
		driver.Phone,
		driver.Email,
		driver.TaxiDriverLicenseNumber,
		driver.DrivingLicenseNumber,
		driver.DrivingLicenseExpiry,
		driver.HireDate,
		driver.Status,
		driver.IsVerified,
		driver.IsActive,
		driver.CreatedAt,
		driver.UpdatedAt,
	)

	return err
}