package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/JCKFinland/connect/backend/internal/models"
	"github.com/JCKFinland/connect/backend/internal/repository"
)

// GetByIDForUpdate returns a driver and locks the row for the
// duration of the current database transaction.
//
// PostgreSQL's pgx.ErrNoRows is translated into repository.ErrNotFound so
// callers do not need to depend on PostgreSQL-specific error semantics.
func (r *DriverRepository) GetByIDForUpdate(
	ctx context.Context,
	id string,
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

	if errors.Is(err, pgx.ErrNoRows) {
		return nil, repository.ErrNotFound
	}

	if err != nil {
		return nil, fmt.Errorf(
			"get driver by id for update: %w",
			err,
		)
	}

	return driver, nil
}
