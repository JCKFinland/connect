package postgres

import (
	"context"
	"fmt"
	"time"
)

// StaleIdleDriverCandidate identifies the permanent driver aggregate and the
// authenticated user ID used as driver_presence.driver_id.
type StaleIdleDriverCandidate struct {
	DriverID string
	UserID   string
}

// ListStaleIdleCandidates discovers a bounded page of potentially stale idle
// drivers without acquiring row locks. Callers must lock each permanent driver
// aggregate before attempting expiration, which rechecks eligibility atomically.
// afterDriverID is an exclusive keyset cursor; pass "" for the first page.
func (r *DriverPresenceRepository) ListStaleIdleCandidates(
	ctx context.Context,
	staleBefore time.Time,
	afterDriverID string,
	limit int,
) ([]StaleIdleDriverCandidate, error) {
	if staleBefore.IsZero() {
		return nil, fmt.Errorf("stale-before time is required")
	}
	if limit <= 0 {
		return nil, fmt.Errorf("candidate limit must be greater than zero")
	}

	const query = `
		SELECT d.id, d.user_id
		FROM driver_presence AS dp
		JOIN drivers AS d ON d.user_id = dp.driver_id
		WHERE d.deleted_at IS NULL
		  AND dp.is_online = TRUE
		  AND dp.availability_status IN ('AVAILABLE', 'BREAK')
		  AND (dp.last_heartbeat_at IS NULL OR dp.last_heartbeat_at < $1)
		  AND ($2::uuid IS NULL OR d.id > $2::uuid)
		ORDER BY d.id ASC
		LIMIT $3
	`

	var cursor any
	if afterDriverID != "" {
		cursor = afterDriverID
	}

	rows, err := r.db.Query(ctx, query, staleBefore.UTC(), cursor, limit)
	if err != nil {
		return nil, fmt.Errorf("list stale idle driver candidates: %w", err)
	}
	defer rows.Close()

	candidates := make([]StaleIdleDriverCandidate, 0, limit)
	for rows.Next() {
		var candidate StaleIdleDriverCandidate
		if err := rows.Scan(&candidate.DriverID, &candidate.UserID); err != nil {
			return nil, fmt.Errorf("scan stale idle driver candidate: %w", err)
		}
		candidates = append(candidates, candidate)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate stale idle driver candidates: %w", err)
	}
	return candidates, nil
}
