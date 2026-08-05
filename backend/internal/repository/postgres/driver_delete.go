package postgres

import (
	"context"
	"time"
)

// Delete performs a soft delete of a driver.
func (r *DriverRepository) Delete(
	ctx context.Context,
	id string,
) error {

	now := time.Now().UTC()

	query := `
		UPDATE drivers
		SET
			deleted_at = $2,
			updated_at = $2
		WHERE id = $1
		  AND deleted_at IS NULL;
	`

	_, err := r.db.Exec(
		ctx,
		query,
		id,
		now,
	)

	return err
}