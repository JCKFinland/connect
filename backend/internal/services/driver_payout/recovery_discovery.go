package driver_payout

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/JCKFinland/connect/backend/internal/payouts"
	postgresrepo "github.com/JCKFinland/connect/backend/internal/repository/postgres"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrInvalidRecovery = errors.New(
	"invalid driver payout recovery",
)

// RecoveryObservation contains the authoritative execution identity
// and the outcome of a read-only provider lookup.
//
// A reported provider success is not independent verification.
type RecoveryObservation struct {
	Execution         postgresrepo.StaleDriverPayoutExecution
	ProviderReference string
	LookupSucceeded   bool
	LookupError       error
}

// RecoveryInspector discovers stale executions and performs lookup only.
// It never initiates transfers or changes payout database state.
type RecoveryInspector struct {
	db       *pgxpool.Pool
	provider payouts.Provider
}

func NewRecoveryInspector(
	db *pgxpool.Pool,
	provider payouts.Provider,
) (*RecoveryInspector, error) {
	if db == nil || provider == nil {
		return nil, ErrInvalidRecovery
	}

	return &RecoveryInspector{
		db:       db,
		provider: provider,
	}, nil
}

// DiscoverCandidates reads stale payout executions without contacting
// the provider or changing financial state.
func (r *RecoveryInspector) DiscoverCandidates(
	ctx context.Context,
	olderThan time.Time,
	limit int,
) ([]postgresrepo.StaleDriverPayoutExecution, error) {
	if r == nil || r.db == nil || r.provider == nil || ctx == nil {
		return nil, ErrInvalidRecovery
	}

	return postgresrepo.DiscoverStaleDriverPayoutExecutions(
		ctx, r.db, olderThan, limit,
	)
}

// InspectCandidate performs one provider lookup using the original
// idempotency key. It does not initiate or finalize a transfer.
func (r *RecoveryInspector) InspectCandidate(
	ctx context.Context,
	execution postgresrepo.StaleDriverPayoutExecution,
) (RecoveryObservation, error) {
	observation := RecoveryObservation{
		Execution: execution,
	}

	if r == nil || r.provider == nil || ctx == nil ||
		execution.PayoutID == "" ||
		execution.IdempotencyKey == "" {
		return observation, ErrInvalidRecovery
	}

	// Fail closed at the provider boundary. The worker also validates
	// after acquiring its lease, but this inspector must independently
	// reject stale or mismatched financial execution identities.
	//
	// This read-only check does not hold database locks during the
	// provider request and does not authorize financial finalization.
	if err := postgresrepo.ValidateDriverPayoutRecoveryIdentity(
		ctx, r.db, execution,
	); err != nil {
		return observation, fmt.Errorf(
			"validate payout recovery identity before provider lookup: %w",
			err,
		)
	}

	reconciler, err := NewReconciler(r.provider)
	if err != nil {
		return observation, err
	}

	verified, lookupErr := reconciler.VerifyExistingTransfer(
		ctx,
		ReconciliationRequest{
			PayoutID:       execution.PayoutID,
			IdempotencyKey: execution.IdempotencyKey,
		},
	)

	if lookupErr != nil {
		observation.LookupError = lookupErr
	} else if verified != nil {
		observation.LookupSucceeded = true
		observation.ProviderReference = verified.ProviderReference
	}

	return observation, nil
}

// InspectStaleExecutions performs bounded read-only discovery,
// followed by lookup using the original immutable idempotency key.
//
// The caller must independently verify any successful transfer
// before authorizing financial finalization.
func (r *RecoveryInspector) InspectStaleExecutions(
	ctx context.Context,
	olderThan time.Time,
	limit int,
) ([]RecoveryObservation, error) {
	if r == nil || r.db == nil || r.provider == nil {
		return nil, ErrInvalidRecovery
	}

	executions, err :=
		postgresrepo.DiscoverStaleDriverPayoutExecutions(
			ctx, r.db, olderThan, limit,
		)
	if err != nil {
		return nil, fmt.Errorf(
			"discover payout recovery candidates: %w",
			err,
		)
	}

	observations := make(
		[]RecoveryObservation, 0, len(executions),
	)

	for _, execution := range executions {
		// All provider lookups must pass the same authoritative
		// identity validation, including direct batch inspection.
		observation, inspectErr := r.InspectCandidate(ctx, execution)
		if inspectErr != nil {
			observation.LookupError = inspectErr
		}

		observations = append(
			observations, observation,
		)
	}

	return observations, nil
}
