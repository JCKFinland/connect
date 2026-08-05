package postgres

import (
	"context"

	"github.com/JCKFinland/connect/backend/internal/models"
)

// List returns all active (non-deleted) drivers.
func (r *DriverRepository) List(
	ctx context.Context,
) ([]models.Driver, error) {

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
		WHERE deleted_at IS NULL
		ORDER BY first_name, last_name;
	`

	rows, err := r.db.Query(
		ctx,
		query,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	drivers := make([]models.Driver, 0)

	for rows.Next() {

		var driver models.Driver

		err := rows.Scan(
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

		drivers = append(
			drivers,
			driver,
		)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return drivers, nil
}