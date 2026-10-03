package postgres

import (
	"context"
	"fmt"

	"github.com/JCKFinland/connect/backend/internal/repository"
)

// UpdateStatus atomically changes a ride request's lifecycle status only when
// its persisted status still matches the caller's expected source state.
func (r *RideRequestRepository) UpdateStatus(
	ctx context.Context,
	id string,
	expectedStatus string,
	newStatus string,
) error {
	const query = `
		UPDATE ride_requests
		SET
			status = $1,
			updated_at = NOW()
		WHERE id = $2
		  AND status = $3
	`

	result, err := r.db.Exec(
		ctx,
		query,
		newStatus,
		id,
		expectedStatus,
	)
	if err != nil {
		return fmt.Errorf("update ride request status: %w", err)
	}

	if result.RowsAffected() == 0 {
		return repository.ErrNotFound
	}

	return nil
}
