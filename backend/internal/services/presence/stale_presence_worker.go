package presence

import (
	"context"
	"errors"
	"time"
)

// StalePresenceWorkerOptions configures stale-presence reconciliation.
type StalePresenceWorkerOptions struct {
	Interval time.Duration

	// OnError receives non-fatal reconciliation errors.
	//
	// The worker continues operating after these errors.
	OnError func(error)
}

// StartStalePresenceWorker reconciles stale idle presence until ctx is
// cancelled.
//
// One reconciliation runs immediately at startup. Subsequent reconciliation
// runs at the configured interval.
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

	runCycle := func() bool {
		_, err := s.ReconcileAllStaleIdle(
			ctx,
			time.Now().UTC(),
		)
		if err == nil {
			return true
		}

		if errors.Is(err, context.Canceled) {
			return false
		}

		if options.OnError != nil {
			options.OnError(err)
		}

		return true
	}

	// Reconcile immediately so stale persisted state does not have to wait for
	// the first ticker interval after process startup.
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
