package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// UpdateActualMetrics persists the authoritative operational
// meter measurements used when finalizing a trip.
//
// The meter-based fields are authoritative. Legacy kilometre/minute
// fields are intentionally not derived here.
func (r *TripRepository) UpdateActualMetrics(
	ctx context.Context,
	tripID string,
	actualDistanceMeters int64,
	actualDurationSeconds int64,
) error {
	const query = `
		UPDATE trips
		SET
			actual_distance_meters = $1,
			actual_duration_seconds = $2,
			updated_at = NOW()
		WHERE id = $3
		  AND deleted_at IS NULL
	`

	result, err := r.db.Exec(
		ctx,
		query,
		actualDistanceMeters,
		actualDurationSeconds,
		tripID,
	)
	if err != nil {
		return fmt.Errorf(
			"update trip actual metrics: %w",
			err,
		)
	}

	if result.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}

	return nil
}
