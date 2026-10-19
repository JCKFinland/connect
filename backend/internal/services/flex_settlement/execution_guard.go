package flex_settlement

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"time"
)

// settlementExecutionContextKey is private to this package.
type settlementExecutionContextKey struct{}

func settlementAlreadyExecuting(
	ctx context.Context,
	settlementID string,
) bool {
	active, ok := ctx.Value(
		settlementExecutionContextKey{},
	).(map[string]struct{})

	if !ok {
		return false
	}

	_, exists := active[settlementID]
	return exists
}

// ExecutionGuard serializes settlement execution among CONNECT
// workers sharing the same PostgreSQL database.
//
// It uses a session-scoped advisory lock on a reserved connection.
// The lock remains held while the callback runs, including during
// external provider operations.
//
// All settlement capture entry points must use this guard.
type ExecutionGuard struct {
	db *pgxpool.Pool
}

func NewExecutionGuard(db *pgxpool.Pool) *ExecutionGuard {
	return &ExecutionGuard{db: db}
}

var ErrSettlementExecutionBusy = errors.New(
	"FLEX settlement execution is already in progress",
)

// WithSettlementLock executes fn only when this caller owns
// the settlement-specific PostgreSQL advisory lock.
//
// A failed lock attempt does not execute fn.
//
// The lock is explicitly released before returning the
// connection to the pool.
func (g *ExecutionGuard) WithSettlementLock(
	ctx context.Context,
	settlementID string,
	fn func(context.Context) error,
) (resultErr error) {
	if g == nil || g.db == nil {
		return errors.New(
			"FLEX settlement execution database is not configured",
		)
	}

	if _, err := uuid.Parse(settlementID); err != nil {
		return fmt.Errorf(
			"invalid FLEX settlement identity: %w",
			err,
		)
	}

	if fn == nil {
		return errors.New(
			"FLEX settlement execution callback is required",
		)
	}

	if settlementAlreadyExecuting(ctx, settlementID) {
		return ErrSettlementExecutionBusy
	}

	conn, err := g.db.Acquire(ctx)
	if err != nil {
		return fmt.Errorf(
			"acquire FLEX settlement execution connection: %w",
			err,
		)
	}
	defer conn.Release()

	// PostgreSQL hashes this namespace and settlement UUID
	// into a consistent 64-bit advisory lock identifier.
	//
	// Hash collisions may serialize unrelated settlements,
	// but cannot allow two callers using the same settlement
	// identity to execute concurrently.
	var acquired bool

	err = conn.QueryRow(ctx, `
        SELECT pg_try_advisory_lock(
            hashtextextended(
                'connect:flex-settlement:' || $1::text,
                0
            )
        )
    `, settlementID).Scan(&acquired)

	if err != nil {
		return fmt.Errorf(
			"acquire FLEX settlement advisory lock: %w",
			err,
		)
	}

	if !acquired {
		return ErrSettlementExecutionBusy
	}

	// Use a fresh cleanup context: the caller's context may
	// already be cancelled when provider execution finishes.
	//
	// If unlocking fails, destroy the underlying connection
	// rather than return a potentially locked session to the pool.
	defer func() {
		unlockCtx, cancel := context.WithTimeout(
			context.WithoutCancel(ctx),
			5*time.Second,
		)
		defer cancel()

		var released bool
		unlockErr := conn.QueryRow(unlockCtx, `
            SELECT pg_advisory_unlock(
                hashtextextended(
                    'connect:flex-settlement:' || $1::text,
                    0
                )
            )
        `, settlementID).Scan(&released)

		if unlockErr != nil || !released {
			cleanupErr := unlockErr
			if cleanupErr == nil {
				cleanupErr = errors.New(
					"PostgreSQL advisory unlock returned false",
				)
			}

			// A session with uncertain advisory-lock ownership
			// must not be returned to the connection pool.
			closeCtx, closeCancel := context.WithTimeout(
				context.Background(),
				5*time.Second,
			)
			closeErr := conn.Conn().Close(closeCtx)
			closeCancel()

			resultErr = errors.Join(
				resultErr,
				fmt.Errorf(
					"release FLEX settlement advisory lock: %w",
					cleanupErr,
				),
				closeErr,
			)
		}
	}()

	active, _ := ctx.Value(
		settlementExecutionContextKey{},
	).(map[string]struct{})

	next := make(map[string]struct{}, len(active)+1)

	for id := range active {
		next[id] = struct{}{}
	}

	next[settlementID] = struct{}{}

	executionCtx := context.WithValue(
		ctx,
		settlementExecutionContextKey{},
		next,
	)

	return fn(executionCtx)
}
