package postgres

import (
	"context"
	"errors"

	"github.com/JCKFinland/connect/backend/internal/models"
	"github.com/JCKFinland/connect/backend/internal/repository"
	"github.com/jackc/pgx/v5"
)

// GetByUserID returns the driver associated with a user.
func (r *DriverRepository) GetByUserID(
	ctx context.Context,
	userID string,
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
        WHERE user_id = $1
          AND deleted_at IS NULL
        LIMIT 1
    `

	driver := &models.Driver{}

	err := r.db.QueryRow(
		ctx,
		query,
		userID,
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

	if errors.Is(err, pgx.ErrNoRows) {
		return nil, repository.ErrNotFound
	}

	if err != nil {
		return nil, err
	}

	return driver, nil
}
