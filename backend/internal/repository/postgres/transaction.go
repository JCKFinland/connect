package postgres

import (
	"context"

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
