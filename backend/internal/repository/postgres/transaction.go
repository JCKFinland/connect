package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// RunInTransaction executes fn inside a PostgreSQL transaction.
//
// If fn returns an error, the transaction is rolled back.
// If fn succeeds, the transaction is committed.
func RunInTransaction(
	ctx context.Context,
	db *pgxpool.Pool,
	fn func(pgx.Tx) error,
) error {

	tx, err := db.Begin(ctx)
	if err != nil {
		return err
	}

	defer func() {
		_ = tx.Rollback(ctx)
	}()

	if err := fn(tx); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

// AcquireTransactionAdvisorySharedLock acquires a PostgreSQL
// transaction-scoped shared advisory lock for the supplied logical key.
//
// Shared locks allow concurrent lifecycle readers while still conflicting
// with lifecycle writers that acquire the corresponding exclusive lock.
//
// hashtextextended converts the logical string key into a stable BIGINT
// advisory-lock key.
func AcquireTransactionAdvisorySharedLock(
	ctx context.Context,
	tx pgx.Tx,
	key string,
) error {
	if key == "" {
		return fmt.Errorf(
			"advisory lock key is required",
		)
	}

	const query = `
		SELECT pg_advisory_xact_lock_shared(
			hashtextextended($1, 0)
		)
	`

	if _, err := tx.Exec(
		ctx,
		query,
		key,
	); err != nil {
		return fmt.Errorf(
			"acquire transaction shared advisory lock: %w",
			err,
		)
	}

	return nil
}

// AcquireTransactionAdvisoryLock acquires a PostgreSQL transaction-scoped
// exclusive advisory lock for the supplied logical key.
//
// The lock:
//
//   - is shared across all CONNECT backend instances using this database
//   - remains held until the current transaction commits or rolls back
//   - is released automatically by PostgreSQL
//
// hashtextextended converts the logical string key into a stable BIGINT
// advisory-lock key.
func AcquireTransactionAdvisoryLock(
	ctx context.Context,
	tx pgx.Tx,
	key string,
) error {

	if key == "" {
		return fmt.Errorf(
			"advisory lock key is required",
		)
	}

	const query = `
		SELECT pg_advisory_xact_lock(
			hashtextextended($1, 0)
		)
	`

	if _, err := tx.Exec(
		ctx,
		query,
		key,
	); err != nil {
		return fmt.Errorf(
			"acquire transaction advisory lock: %w",
			err,
		)
	}

	return nil
}
