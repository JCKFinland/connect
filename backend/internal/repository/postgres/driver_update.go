package postgres

import (
	"context"
	"time"

	"github.com/JCKFinland/connect/backend/internal/models"
)

// Update modifies an existing driver.
func (r *DriverRepository) Update(
	ctx context.Context,
	driver *models.Driver,
) error {

	driver.UpdatedAt = time.Now().UTC()

	query := `
		UPDATE drivers
		SET
			user_id = $2,
			company_id = $3,
			branch_id = $4,
			driver_number = $5,
			first_name = $6,
			last_name = $7,
			phone = $8,
			email = $9,
			taxi_driver_license_number = $10,
			driving_license_number = $11,
			driving_license_expiry = $12,
			hire_date = $13,
			status = $14,
            is_verified = $15,
            verified_at = $16,
            verified_by_user_id = $17,
            is_active = $18,
            updated_at = $19
		WHERE id = $1
		  AND deleted_at IS NULL;
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
		driver.VerifiedAt,
		driver.VerifiedByUserID,
		driver.IsActive,
		driver.UpdatedAt,
	)

	return err
}
