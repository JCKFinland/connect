package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/JCKFinland/connect/backend/internal/models"
	"github.com/JCKFinland/connect/backend/internal/repository"
)

// GetByUserIDForUpdate returns the active driver profile associated with a
// platform user and locks that driver row for the current transaction.
//
// Driver lifecycle operations acquire the driver row before the operational
// presence row. Assignment uses this same order so assignment and archival
// cannot make decisions from mutually stale driver lifecycle state.
func (r *DriverRepository) GetByUserIDForUpdate(
	ctx context.Context,
	userID string,
) (*models.Driver, error) {
	const query = `
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
		WHERE user_id = $1
		  AND deleted_at IS NULL
		LIMIT 1
		FOR UPDATE
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
		&driver.VerifiedAt,
		&driver.VerifiedByUserID,
		&driver.IsActive,
		&driver.CreatedAt,
		&driver.UpdatedAt,
		&driver.DeletedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, repository.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf(
			"get driver by user id for update: %w",
			err,
		)
	}

	return driver, nil
}
