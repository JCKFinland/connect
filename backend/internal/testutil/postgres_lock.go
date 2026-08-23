package testutil

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// AcquirePostgresFixtureLock obtains a session-scoped PostgreSQL advisory
// lock using a dedicated pool connection.
//
// Integration tests that share mutable database fixtures can use the same
// key to prevent separate `go test` package processes from modifying that
// fixture concurrently.
//
// The returned release function must always be called.
func AcquirePostgresFixtureLock(
	ctx context.Context,
	db *pgxpool.Pool,
	key string,
) (func(context.Context) error, error) {

	if db == nil {
		return nil, fmt.Errorf(
			"database pool is required",
		)
	}

	if key == "" {
		return nil, fmt.Errorf(
			"fixture lock key is required",
		)
	}

	conn, err := db.Acquire(ctx)
	if err != nil {
		return nil, fmt.Errorf(
			"acquire fixture lock connection: %w",
			err,
		)
	}

	const lockQuery = `
		SELECT pg_advisory_lock(
			hashtextextended($1::text, 0)
		)
	`

	if _, err := conn.Exec(
		ctx,
		lockQuery,
		key,
	); err != nil {

		conn.Release()

		return nil, fmt.Errorf(
			"acquire PostgreSQL fixture lock: %w",
			err,
		)
	}

	release := func(
		releaseCtx context.Context,
	) error {

		defer conn.Release()

		const unlockQuery = `
			SELECT pg_advisory_unlock(
				hashtextextended($1::text, 0)
			)
		`

		var unlocked bool

		if err := conn.QueryRow(
			releaseCtx,
			unlockQuery,
			key,
		).Scan(
			&unlocked,
		); err != nil {
			return fmt.Errorf(
				"release PostgreSQL fixture lock: %w",
				err,
			)
		}

		if !unlocked {
			return fmt.Errorf(
				"PostgreSQL fixture lock was not held",
			)
		}

		return nil
	}

	return release, nil
}
