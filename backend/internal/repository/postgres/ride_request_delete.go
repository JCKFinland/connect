package postgres

import (
	"context"
	"fmt"
)

// Delete removes a ride request.
func (r *RideRequestRepository) Delete(
	ctx context.Context,
	id string,
) error {
	const query = `
		DELETE FROM ride_requests
		WHERE id = $1
	`

	result, err := r.db.Exec(
		ctx,
		query,
		id,
	)

	if err != nil {
		return fmt.Errorf("delete ride request: %w", err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("ride request not found")
	}

	return nil
}
