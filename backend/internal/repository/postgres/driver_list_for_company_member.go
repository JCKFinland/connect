package postgres

import (
	"context"

	"github.com/JCKFinland/connect/backend/internal/models"
)

func (r *DriverRepository) ListForCompanyMember(
	ctx context.Context,
	userID string,
) ([]models.Driver, error) {
	query := `
		SELECT
			d.id,
			d.user_id,
			d.company_id,
			d.branch_id,
			d.driver_number,
			d.first_name,
			d.last_name,
			d.phone,
			d.email,
			d.taxi_driver_license_number,
			d.driving_license_number,
			d.driving_license_expiry,
			d.hire_date,
			d.status,
			d.is_verified,
			d.verified_at,
			d.verified_by_user_id,
			d.is_active,
			d.created_at,
			d.updated_at,
			d.deleted_at
		FROM drivers d
		WHERE d.deleted_at IS NULL
		  AND EXISTS (
			SELECT 1
			FROM company_memberships cm
			WHERE cm.user_id = $1
			  AND cm.company_id = d.company_id
		  )
		ORDER BY d.first_name, d.last_name
	`

	rows, err := r.db.Query(ctx, query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	drivers := make([]models.Driver, 0)

	for rows.Next() {
		var driver models.Driver

		if err := rows.Scan(
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
		); err != nil {
			return nil, err
		}

		drivers = append(drivers, driver)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return drivers, nil
}
