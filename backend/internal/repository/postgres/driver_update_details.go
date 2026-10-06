package postgres

import (
	"context"
	"time"

	"github.com/JCKFinland/connect/backend/internal/models"
	"github.com/JCKFinland/connect/backend/internal/repository"
)

// UpdateDetails modifies only driver profile and employment details.
//
// Tenant ownership, branch placement, account identity, regulatory credentials,
// verification metadata, and lifecycle state are deliberately excluded.
func (r *DriverRepository) UpdateDetails(
	ctx context.Context,
	driver *models.Driver,
) error {
	driver.UpdatedAt = time.Now().UTC()

	const query = `
		UPDATE drivers
		SET
			driver_number = $2,
			first_name = $3,
			last_name = $4,
			phone = $5,
			email = $6,
			hire_date = $7,
			updated_at = $8
		WHERE id = $1
		  AND deleted_at IS NULL;
	`

	result, err := r.db.Exec(
		ctx,
		query,
		driver.ID,
		driver.DriverNumber,
		driver.FirstName,
		driver.LastName,
		driver.Phone,
		driver.Email,
		driver.HireDate,
		driver.UpdatedAt,
	)
	if err != nil {
		return err
	}

	if result.RowsAffected() == 0 {
		return repository.ErrNotFound
	}

	return nil
}
