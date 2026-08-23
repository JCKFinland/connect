package postgres

import (
	"context"
	"fmt"
	"time"
)

// ExpireDispatchableRideRequests moves expired PENDING/MATCHING ride
// requests to the terminal EXPIRED state and clears redispatch retry state.
//
// ACCEPTED and CANCELLED rides are deliberately untouched.
func (r *RideRequestRepository) ExpireDispatchableRideRequests(
	ctx context.Context,
	now time.Time,
) ([]string, error) {

	const query = `
		UPDATE ride_requests
		SET
			status = 'EXPIRED',
			dispatch_retry_count = 0,
			next_dispatch_attempt_at = NULL,
			last_dispatch_attempt_at = NULL,
			updated_at = NOW()
		WHERE status IN ('PENDING', 'MATCHING')
		  AND expires_at IS NOT NULL
		  AND expires_at <= $1
		RETURNING id
	`

	rows, err := r.db.Query(
		ctx,
		query,
		now,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"expire ride requests: %w",
			err,
		)
	}
	defer rows.Close()

	ids := make([]string, 0)

	for rows.Next() {
		var id string

		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf(
				"scan expired ride request: %w",
				err,
			)
		}

		ids = append(ids, id)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf(
			"iterate expired ride requests: %w",
			err,
		)
	}

	return ids, nil
}
