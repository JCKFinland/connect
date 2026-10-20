package flex_settlement

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	postgresrepo "github.com/JCKFinland/connect/backend/internal/repository/postgres"
)

type SettlementDiscovery interface {
	Next(context.Context) ([]postgresrepo.FlexSettlementCandidate, error)
}

type SettlementDispatch interface {
	Dispatch(
		context.Context,
		postgresrepo.FlexSettlementCandidate,
	) (*RecoveryResult, error)
}

type DiscoveryWorkerOptions struct {
	Interval time.Duration
	OnError  func(error)
}

type DiscoveryWorker struct {
	discovery SettlementDiscovery
	dispatch  SettlementDispatch

	// Serialize scheduler cursor updates and cooldown access.
	runMu sync.Mutex

	// Cooldowns reduce repeated processing of unresolved settlements.
	// They do not authorize or prevent financial capture.
	cooldowns map[string]time.Time
	now       func() time.Time
}

const (
	defaultSettlementRetryCooldown = 30 * time.Second
	maxSettlementCooldownEntries   = 10000
)

func NewDiscoveryWorker(
	discovery SettlementDiscovery,
	dispatch SettlementDispatch,
) (*DiscoveryWorker, error) {
	if discovery == nil || dispatch == nil {
		return nil, errors.New(
			"FLEX settlement worker dependencies are required",
		)
	}

	return &DiscoveryWorker{
		discovery: discovery,
		dispatch:  dispatch,
		cooldowns: make(map[string]time.Time),
		now:       time.Now,
	}, nil
}

// Run starts a serial, cancellation-aware settlement polling loop.
// All financial execution remains delegated to the guarded dispatcher.
func (w *DiscoveryWorker) Run(
	ctx context.Context,
	options DiscoveryWorkerOptions,
) error {
	if w == nil || w.discovery == nil || w.dispatch == nil {
		return errors.New("FLEX settlement worker is not configured")
	}

	if ctx == nil {
		return errors.New("FLEX settlement worker context is required")
	}

	if options.Interval <= 0 {
		return errors.New(
			"FLEX settlement worker interval must be positive",
		)
	}

	ticker := time.NewTicker(options.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil

		case <-ticker.C:
			if err := w.RunOnce(ctx, options.OnError); err != nil {
				if ctx.Err() != nil {
					return nil
				}

				if options.OnError != nil {
					options.OnError(err)
				}
			}
		}
	}
}

// RunOnce discovers a bounded batch and processes candidates serially.
// An individual candidate error is reported without abandoning the batch.
func (w *DiscoveryWorker) RunOnce(
	ctx context.Context,
	onError func(error),
) error {
	if w == nil || w.discovery == nil || w.dispatch == nil {
		return errors.New("FLEX settlement worker is not configured")
	}

	if ctx == nil {
		return errors.New("FLEX settlement worker context is required")
	}

	if err := ctx.Err(); err != nil {
		return err
	}

	// A waiting caller cannot begin discovery or financial dispatch
	// after its context has been cancelled.
	if err := w.lockRun(ctx); err != nil {
		return err
	}
	defer w.runMu.Unlock()

	if err := ctx.Err(); err != nil {
		return err
	}

	candidates, err := w.discovery.Next(ctx)
	if err != nil {
		return fmt.Errorf(
			"discover FLEX settlement candidates: %w",
			err,
		)
	}

	for _, candidate := range candidates {
		if err := ctx.Err(); err != nil {
			return err
		}

		now := w.now()

		if retryAfter, exists := w.cooldowns[candidate.SettlementID]; exists {
			if now.Before(retryAfter) {
				continue
			}

			delete(w.cooldowns, candidate.SettlementID)
		}

		_, err := w.dispatch.Dispatch(ctx, candidate)
		if err != nil {
			w.recordCooldown(
				candidate.SettlementID,
				now.Add(defaultSettlementRetryCooldown),
			)

			if onError != nil {
				onError(fmt.Errorf(
					"dispatch FLEX settlement %s: %w",
					candidate.SettlementID,
					err,
				))
			}
		} else {
			delete(w.cooldowns, candidate.SettlementID)
		}
	}

	return nil
}

// recordCooldown bounds in-memory retry state.
// Eviction only removes throttling; it never changes settlement state.
func (w *DiscoveryWorker) recordCooldown(
	settlementID string,
	retryAfter time.Time,
) {
	if w.cooldowns == nil {
		w.cooldowns = make(map[string]time.Time)
	}

	if _, exists := w.cooldowns[settlementID]; !exists &&
		len(w.cooldowns) >= maxSettlementCooldownEntries {
		var oldestID string
		var oldestTime time.Time

		for id, deadline := range w.cooldowns {
			if oldestID == "" || deadline.Before(oldestTime) {
				oldestID = id
				oldestTime = deadline
			}
		}

		if oldestID != "" {
			delete(w.cooldowns, oldestID)
		}
	}

	w.cooldowns[settlementID] = retryAfter
}

// lockRun serializes worker execution while honoring cancellation.
func (w *DiscoveryWorker) lockRun(ctx context.Context) error {
	if w.runMu.TryLock() {
		return nil
	}

	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()

		case <-ticker.C:
			if w.runMu.TryLock() {
				return nil
			}
		}
	}
}
