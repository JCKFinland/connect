package postgres

import (
	"context"
	"time"

	"github.com/JCKFinland/connect/backend/internal/repository"
)

// UpdateDrivingLicenseExpiry synchronizes the driver's regulatory
// expiry from independently verified driving-licence evidence.
//
// The caller must enforce document validation and authorization.
// When called within a transaction, this update participates in
// that transaction.
func (r *DriverRepository) UpdateDrivingLicenseExpiry(
	ctx context.Context,
	driverID string,
	expiry time.Time,
) error {
	const query = `
		UPDATE drivers
		SET driving_license_expiry = $2,
		    updated_at = NOW()
		WHERE id = $1
		  AND deleted_at IS NULL
	`

	result, err := r.db.Exec(
		ctx,
		query,
		driverID,
		expiry,
	)
	if err != nil {
		return err
	}

	if result.RowsAffected() != 1 {
		return repository.ErrNotFound
	}

	return nil
}
