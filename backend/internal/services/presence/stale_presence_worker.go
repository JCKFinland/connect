package presence

import (
	"context"
	"errors"
	"time"
)

// StalePresenceWorkerOptions configures stale-presence reconciliation.
type StalePresenceWorkerOptions struct {
	Interval time.Duration

	// BatchSize is the maximum number of stale driver candidates
	// inspected during each reconciliation cycle.
	// A non-positive value uses the default batch size.
	BatchSize int

	// OnError receives non-fatal reconciliation errors.
	// The worker continues operating after these errors.
	OnError func(error)
}

// StartStalePresenceWorker reconciles stale idle presence until ctx is
// cancelled.
//
// One reconciliation runs immediately at startup. Subsequent reconciliation
// runs at the configured interval.
//
// Every reconciliation cycle is bounded. Each candidate is processed
// transactionally after acquiring its permanent driver aggregate lock.
func (s *Service) StartStalePresenceWorker(
	ctx context.Context,
	options StalePresenceWorkerOptions,
) {
	if s == nil || s.cfg == nil {
		return
	}

	interval := options.Interval
	if interval <= 0 {
		interval = s.cfg.Presence.HeartbeatTimeout / 2
	}

	if interval <= 0 {
		if options.OnError != nil {
			options.OnError(
				errors.New(
					"stale presence worker interval must be greater than zero",
				),
			)
		}
		return
	}

	batchSize := options.BatchSize
	if batchSize <= 0 {
		batchSize = defaultStaleIdleBatchSize
	}

	runCycle := func() bool {
		now := time.Now().UTC()

		if s.reconcileStaleIdleBounded != nil {
			_, err := s.reconcileStaleIdleBounded(
				ctx,
				now,
				batchSize,
			)

			if errors.Is(err, context.Canceled) ||
				errors.Is(err, context.DeadlineExceeded) {
				return false
			}

			if err != nil && options.OnError != nil {
				options.OnError(err)
			}
		}

		if ctx.Err() != nil {
			return false
		}

		// Regulatory reconciliation runs independently of
		// stale-heartbeat reconciliation failures.
		if s.reconcileRegulatoryBounded != nil {
			_, err := s.reconcileRegulatoryBounded(
				ctx,
				now,
				batchSize,
			)

			if errors.Is(err, context.Canceled) ||
				errors.Is(err, context.DeadlineExceeded) {
				return false
			}

			if err != nil && options.OnError != nil {
				options.OnError(err)
			}
		}

		return ctx.Err() == nil
	}

	// Reconcile immediately so stale persisted state does not have to wait
	// for the first ticker interval after process startup.
	if !runCycle() {
		return
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return

		case <-ticker.C:
			if !runCycle() {
				return
			}
		}
	}
}
