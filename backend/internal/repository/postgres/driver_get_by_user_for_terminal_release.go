package postgres

import (
	"context"
	"fmt"
)

// LockAllByUserIDForTerminalRelease locks every permanent driver profile
// belonging to a platform user, including archived profiles.
//
// This preserves the driver-before-trip-before-presence lock order
// during terminal trip finalization.
//
// The repository must be transaction-bound.
func (r *DriverRepository) LockAllByUserIDForTerminalRelease(
	ctx context.Context,
	userID string,
) error {
	if userID == "" {
		return fmt.Errorf("driver user ID is required")
	}

	const query = `
		SELECT id
		FROM drivers
		WHERE user_id = $1
		ORDER BY id
		FOR UPDATE
	`

	rows, err := r.db.Query(ctx, query, userID)
	if err != nil {
		return fmt.Errorf(
			"lock driver profiles for terminal release: %w",
			err,
		)
	}
	defer rows.Close()

	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return fmt.Errorf(
				"scan locked driver profile: %w",
				err,
			)
		}
	}

	if err := rows.Err(); err != nil {
		return fmt.Errorf(
			"iterate locked driver profiles: %w",
			err,
		)
	}

	return nil
}
