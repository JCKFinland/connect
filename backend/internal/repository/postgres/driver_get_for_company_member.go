package postgres

import (
	"context"
	"errors"

	"github.com/JCKFinland/connect/backend/internal/models"
	"github.com/JCKFinland/connect/backend/internal/repository"
	"github.com/jackc/pgx/v5"
)

func (r *DriverRepository) GetByIDForCompanyMember(
	ctx context.Context,
	userID string,
	id string,
) (*models.Driver, error) {
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
		WHERE d.id = $2
		  AND d.deleted_at IS NULL
		  AND EXISTS (
			SELECT 1
			FROM company_memberships cm
			WHERE cm.user_id = $1
			  AND cm.company_id = d.company_id
		  )
	`

	driver := &models.Driver{}

	err := r.db.QueryRow(
		ctx,
		query,
		userID,
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
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, repository.ErrNotFound
		}
		return nil, err
	}

	return driver, nil
}
