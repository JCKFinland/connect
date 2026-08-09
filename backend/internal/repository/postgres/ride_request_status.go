package postgres

import (
	"context"
	"fmt"
)

// UpdateStatus changes the status of a ride request.
func (r *RideRequestRepository) UpdateStatus(
	ctx context.Context,
	id string,
	status string,
) error {
	const query = `
		UPDATE ride_requests
		SET
			status = $1,
			updated_at = NOW()
		WHERE id = $2
	`

	result, err := r.db.Exec(
		ctx,
		query,
		status,
		id,
	)

	if err != nil {
		return fmt.Errorf("update ride request status: %w", err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("ride request not found")
	}

	return nil
}
