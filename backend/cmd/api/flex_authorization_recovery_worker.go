package main

import (
	"errors"
	"fmt"
	"strings"
	"time"

	stripepayment "github.com/JCKFinland/connect/backend/internal/payments/stripe"
	flexauthorization "github.com/JCKFinland/connect/backend/internal/services/flex_authorization"
	"github.com/jackc/pgx/v5/pgxpool"
)

// flexAuthorizationRecoveryDependencies contains only the
// Stripe authorization capability needed for recovery.
//
// Construction does not contact Stripe or authorize capture.
type flexAuthorizationRecoveryDependencies struct {
	Authorization stripepayment.FlexAuthorizationAdapter
}

// newFlexAuthorizationRecoveryDependencies constructs an
// authorization adapter without initiating any payment operation.
func newFlexAuthorizationRecoveryDependencies(
	secretKey string,
) (*flexAuthorizationRecoveryDependencies, error) {
	if strings.TrimSpace(secretKey) == "" {
		return nil, errors.New(
			"FLEX authorization recovery requires Stripe secret key",
		)
	}

	adapter, err := stripepayment.NewFlexAuthorizationAdapter(
		secretKey,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"initialize FLEX authorization recovery adapter: %w",
			err,
		)
	}

	return &flexAuthorizationRecoveryDependencies{
		Authorization: adapter,
	}, nil
}

// buildFlexAuthorizationRecoveryLifecycle assembles the
// independent retrieval-and-reconciliation pipeline.
//
// Construction performs no database queries, Stripe requests,
// or background scheduling.
//
// The RecoveryService enforces the existing-provider-ID
// requirement before the Stripe adapter is invoked.
func buildFlexAuthorizationRecoveryLifecycle(
	db *pgxpool.Pool,
	dependencies *flexAuthorizationRecoveryDependencies,
	staleAfter time.Duration,
	batchSize int,
) (*flexauthorization.AuthorizationRecoveryLifecycle, error) {
	if db == nil {
		return nil, errors.New(
			"FLEX authorization recovery requires database",
		)
	}

	if dependencies == nil ||
		dependencies.Authorization == nil {
		return nil, errors.New(
			"FLEX authorization recovery requires authorization adapter",
		)
	}

	if staleAfter <= 0 {
		return nil, errors.New(
			"FLEX authorization recovery stale threshold must be positive",
		)
	}

	recovery := flexauthorization.NewRecoveryService(
		db,
		dependencies.Authorization,
	)

	discovery := flexauthorization.PostgresAuthorizationRecoveryDiscovery{
		DB: db,
	}

	worker, err := flexauthorization.NewAuthorizationRecoveryWorker(
		discovery,
		recovery,
		staleAfter,
		batchSize,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"initialize FLEX authorization recovery worker: %w",
			err,
		)
	}

	scheduler, err := flexauthorization.NewAuthorizationRecoveryScheduler(
		worker,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"initialize FLEX authorization recovery scheduler: %w",
			err,
		)
	}

	lifecycle, err := flexauthorization.NewAuthorizationRecoveryLifecycle(
		scheduler,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"initialize FLEX authorization recovery lifecycle: %w",
			err,
		)
	}

	return lifecycle, nil
}
