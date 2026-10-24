package flex_authorization

import (
	"context"
	"errors"
	"fmt"
	"time"

	postgresrepo "github.com/JCKFinland/connect/backend/internal/repository/postgres"
	"github.com/jackc/pgx/v5/pgxpool"
)

// AuthorizationRecoveryDiscovery discovers stale authorizations
// whose existing Stripe provider identity is already known.
type AuthorizationRecoveryDiscovery interface {
	Discover(
		ctx context.Context,
		staleAfter time.Duration,
		batchSize int,
	) ([]postgresrepo.FlexAuthorizationRecoveryCandidate, error)
}

// AuthorizationRecoveryExecutor verifies and reconciles one
// previously created authorization.
type AuthorizationRecoveryExecutor interface {
	RecoverAndReconcile(
		ctx context.Context,
		authorizationID string,
	) (*RecoveryReconcileResult, error)
}

// PostgresAuthorizationRecoveryDiscovery uses the bounded,
// read-only repository discovery function.
type PostgresAuthorizationRecoveryDiscovery struct {
	DB *pgxpool.Pool
}

func (d PostgresAuthorizationRecoveryDiscovery) Discover(
	ctx context.Context,
	staleAfter time.Duration,
	batchSize int,
) ([]postgresrepo.FlexAuthorizationRecoveryCandidate, error) {
	return postgresrepo.DiscoverFlexAuthorizationRecoveryCandidates(
		ctx,
		d.DB,
		staleAfter,
		batchSize,
	)
}

// AuthorizationRecoveryWorker performs one bounded recovery cycle.
// It does not schedule itself or start background goroutines.
type AuthorizationRecoveryWorker struct {
	discovery  AuthorizationRecoveryDiscovery
	executor   AuthorizationRecoveryExecutor
	staleAfter time.Duration
	batchSize  int

	// The buffered execution gate serializes RunOnce calls and
	// permits waiting callers to stop on context cancellation.
	runGate chan struct{}
}

func NewAuthorizationRecoveryWorker(
	discovery AuthorizationRecoveryDiscovery,
	executor AuthorizationRecoveryExecutor,
	staleAfter time.Duration,
	batchSize int,
) (*AuthorizationRecoveryWorker, error) {
	if discovery == nil {
		return nil, errors.New(
			"FLEX authorization recovery discovery is required",
		)
	}

	if executor == nil {
		return nil, errors.New(
			"FLEX authorization recovery executor is required",
		)
	}

	if staleAfter <= 0 {
		return nil, errors.New(
			"FLEX authorization recovery stale threshold must be positive",
		)
	}

	if batchSize <= 0 {
		batchSize = postgresrepo.DefaultFlexAuthorizationRecoveryBatchSize
	}

	if batchSize > postgresrepo.MaxFlexAuthorizationRecoveryBatchSize {
		batchSize = postgresrepo.MaxFlexAuthorizationRecoveryBatchSize
	}

	return &AuthorizationRecoveryWorker{
		discovery:  discovery,
		executor:   executor,
		staleAfter: staleAfter,
		batchSize:  batchSize,
		runGate:    make(chan struct{}, 1),
	}, nil
}

// RunOnce discovers and processes at most batchSize candidates.
//
// Candidate errors are reported individually without interrupting
// the remaining batch. Discovery errors stop the cycle.
//
// No Stripe calls are performed by the worker itself.
// Only the injected recovery executor can verify a provider.
func (w *AuthorizationRecoveryWorker) RunOnce(
	ctx context.Context,
	onError func(error),
) error {
	if w == nil || w.discovery == nil || w.executor == nil {
		return errors.New(
			"FLEX authorization recovery worker is unavailable",
		)
	}

	if ctx == nil {
		return errors.New(
			"FLEX authorization recovery context is required",
		)
	}

	if err := ctx.Err(); err != nil {
		return err
	}

	if w.runGate == nil {
		return errors.New(
			"FLEX authorization recovery execution gate is unavailable",
		)
	}

	select {
	case w.runGate <- struct{}{}:
		defer func() {
			<-w.runGate
		}()
	case <-ctx.Done():
		return ctx.Err()
	}

	if err := ctx.Err(); err != nil {
		return err
	}

	candidates, err := w.discovery.Discover(
		ctx,
		w.staleAfter,
		w.batchSize,
	)
	if err != nil {
		return fmt.Errorf(
			"discover FLEX authorization recovery work: %w",
			err,
		)
	}

	if len(candidates) > w.batchSize {
		return errors.New(
			"FLEX authorization recovery discovery exceeded batch limit",
		)
	}

	for _, candidate := range candidates {
		if err := ctx.Err(); err != nil {
			return err
		}

		if candidate.AuthorizationID == "" {
			if onError != nil {
				onError(errors.New(
					"FLEX authorization recovery candidate has no ID",
				))
			}
			continue
		}

		_, err := w.executor.RecoverAndReconcile(
			ctx,
			candidate.AuthorizationID,
		)
		if err != nil && onError != nil {
			onError(fmt.Errorf(
				"recover FLEX authorization %s: %w",
				candidate.AuthorizationID,
				err,
			))
		}
	}

	return nil
}
