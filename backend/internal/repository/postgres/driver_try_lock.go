package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// TryLockByID acquires the permanent driver aggregate row lock without
// waiting. It returns false when the driver is absent, deleted, or already
// locked by another transaction.
//
// Callers must use a transaction-bound repository and acquire this lock
// before locking driver_presence.
func (r *DriverRepository) TryLockByID(
	ctx context.Context,
	id string,
) (bool, error) {
	const query = `
		SELECT id
		FROM drivers
		WHERE id = $1
		  AND deleted_at IS NULL
		FOR UPDATE SKIP LOCKED
	`

	var lockedID string
	err := r.db.QueryRow(ctx, query, id).Scan(&lockedID)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("try lock driver aggregate: %w", err)
	}

	return true, nil
}
