package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// Delete performs a soft delete on a trip.
func (r *TripRepository) Delete(
	ctx context.Context,
	id string,
) error {
	const query = `
		UPDATE trips
		SET
			is_active = FALSE,
			deleted_at = $1,
			updated_at = NOW()
		WHERE id = $2
		  AND deleted_at IS NULL
	`

	result, err := r.db.Exec(
		ctx,
		query,
		time.Now().UTC(),
		id,
	)
	if err != nil {
		return fmt.Errorf("delete trip: %w", err)
	}

	if result.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}

	return nil
}
