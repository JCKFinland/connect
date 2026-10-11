package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// GetIDByRideRequestID resolves the non-deleted trip associated with a ride request.
//
// If multiple trips unexpectedly exist for the same ride request, this method
// fails rather than selecting an arbitrary trip.
func (r *TripRepository) GetIDByRideRequestID(
	ctx context.Context,
	rideRequestID string,
) (string, error) {
	const query = `
		SELECT id
		FROM trips
		WHERE ride_request_id = $1
		  AND deleted_at IS NULL
		LIMIT 2
	`

	rows, err := r.db.Query(ctx, query, rideRequestID)
	if err != nil {
		return "", fmt.Errorf("query trip by ride request: %w", err)
	}
	defer rows.Close()

	var tripID string
	count := 0

	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return "", fmt.Errorf("scan trip by ride request: %w", err)
		}
		count++
		tripID = id
	}

	if err := rows.Err(); err != nil {
		return "", fmt.Errorf("read trip by ride request: %w", err)
	}

	if count == 0 {
		return "", fmt.Errorf("trip not found for ride request: %w", pgx.ErrNoRows)
	}

	if count > 1 {
		return "", fmt.Errorf(
			"multiple trips found for ride request %s",
			rideRequestID,
		)
	}

	return tripID, nil
}
