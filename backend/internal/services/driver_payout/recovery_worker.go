package driver_payout

import (
	"context"
	"errors"
	"fmt"
	"time"

	postgresrepo "github.com/JCKFinland/connect/backend/internal/repository/postgres"
)

const (
	defaultPayoutRecoveryCooldown = 5 * time.Minute
	defaultPayoutRecoveryTimeout  = 30 * time.Second
	maxPayoutRecoveryCooldowns    = 1000
	maxPayoutRecoveryBatchSize    = 100
)

// PayoutRecoveryLookup is a read-only provider investigation.
type PayoutRecoveryLookup interface {
	DiscoverCandidates(
		context.Context,
		time.Time,
		int,
	) ([]postgresrepo.StaleDriverPayoutExecution, error)

	InspectCandidate(
		context.Context,
		postgresrepo.StaleDriverPayoutExecution,
	) (RecoveryObservation, error)
}

// PayoutRecoveryWorker performs one bounded recovery cycle.
//
// It never initiates transfers, finalizes payouts or changes
// financial database records.
//
// This worker is not automatically started.
type PayoutRecoveryWorker struct {
	inspector  PayoutRecoveryLookup
	staleAfter time.Duration
	batchSize  int

	runGate   chan struct{}
	cooldowns map[string]time.Time
	now       func() time.Time
}

func NewPayoutRecoveryWorker(
	inspector PayoutRecoveryLookup,
	staleAfter time.Duration,
	batchSize int,
) (*PayoutRecoveryWorker, error) {
	if inspector == nil ||
		staleAfter <= 0 ||
		batchSize < 1 ||
		batchSize > maxPayoutRecoveryBatchSize {
		return nil, ErrInvalidRecovery
	}

	return &PayoutRecoveryWorker{
		inspector:  inspector,
		staleAfter: staleAfter,
		batchSize:  batchSize,
		runGate:    make(chan struct{}, 1),
		cooldowns:  make(map[string]time.Time),
		now:        time.Now,
	}, nil
}

// RunOnce performs a serial, bounded inspection.
//
// Cooldowns throttle repeated provider lookups within this
// worker instance. They do not constitute a distributed lock.
func (w *PayoutRecoveryWorker) RunOnce(
	ctx context.Context,
	onError func(error),
) error {
	if w == nil ||
		w.inspector == nil ||
		w.runGate == nil ||
		w.staleAfter <= 0 ||
		w.batchSize < 1 ||
		w.batchSize > maxPayoutRecoveryBatchSize {
		return ErrInvalidRecovery
	}

	if ctx == nil {
		return ErrInvalidRecovery
	}

	if err := ctx.Err(); err != nil {
		return err
	}

	// Serialize recovery cycles within this worker instance.
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

	cutoff := w.now().UTC().Add(-w.staleAfter)

	// Discovery does not contact the provider.
	candidates, err := w.inspector.DiscoverCandidates(
		ctx,
		cutoff,
		w.batchSize,
	)
	if err != nil {
		return fmt.Errorf(
			"discover stale payout executions: %w",
			err,
		)
	}

	if len(candidates) > w.batchSize {
		return errors.New(
			"payout recovery discovery exceeded batch limit",
		)
	}

	seen := make(map[string]struct{}, len(candidates))

	for _, candidate := range candidates {
		if err := ctx.Err(); err != nil {
			return err
		}

		id := candidate.PayoutID

		if id == "" || candidate.IdempotencyKey == "" {
			if onError != nil {
				onError(errors.New(
					"payout recovery candidate has incomplete identity",
				))
			}
			continue
		}

		if _, duplicate := seen[id]; duplicate {
			continue
		}
		seen[id] = struct{}{}

		now := w.now()

		// Skip provider lookup while cooldown is active.
		if retryAfter, exists := w.cooldowns[id]; exists {
			if now.Before(retryAfter) {
				continue
			}
			delete(w.cooldowns, id)
		}

		// Reserve cooldown before contacting the provider.
		w.recordCooldown(
			id,
			now.Add(defaultPayoutRecoveryCooldown),
		)

		attemptCtx, cancel := context.WithTimeout(
			ctx,
			defaultPayoutRecoveryTimeout,
		)

		observation, lookupErr := w.inspector.InspectCandidate(
			attemptCtx,
			candidate,
		)

		attemptErr := attemptCtx.Err()
		cancel()

		if err := ctx.Err(); err != nil {
			return err
		}

		if lookupErr == nil && attemptErr != nil {
			lookupErr = attemptErr
		}

		if lookupErr == nil {
			lookupErr = observation.LookupError
		}

		if lookupErr != nil && onError != nil {
			onError(fmt.Errorf(
				"investigate driver payout %s: %w",
				id,
				lookupErr,
			))
		}

		// Provider success is an observation only.
		// This worker never finalizes a payout.
	}

	return nil
}

func (w *PayoutRecoveryWorker) recordCooldown(
	payoutID string,
	retryAfter time.Time,
) {
	if w.cooldowns == nil {
		w.cooldowns = make(map[string]time.Time)
	}

	if _, exists := w.cooldowns[payoutID]; !exists &&
		len(w.cooldowns) >= maxPayoutRecoveryCooldowns {

		var oldestID string
		var oldestTime time.Time

		for id, deadline := range w.cooldowns {
			if oldestID == "" ||
				deadline.Before(oldestTime) ||
				(deadline.Equal(oldestTime) && id < oldestID) {
				oldestID = id
				oldestTime = deadline
			}
		}

		if oldestID != "" {
			delete(w.cooldowns, oldestID)
		}
	}

	w.cooldowns[payoutID] = retryAfter
}
