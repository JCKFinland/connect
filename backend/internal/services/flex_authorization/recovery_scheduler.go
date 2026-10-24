package flex_authorization

import (
	"context"
	"errors"
	"time"
)

// AuthorizationRecoveryRunner is the narrow execution contract
// for a scheduled recovery cycle.
type AuthorizationRecoveryRunner interface {
	RunOnce(context.Context, func(error)) error
}

// AuthorizationRecoveryScheduler schedules one recovery worker.
//
// Scheduling is serial: the next cycle cannot begin until the
// preceding RunOnce call has returned.
//
// Construction does not launch a goroutine or contact Stripe.
type AuthorizationRecoveryScheduler struct {
	worker AuthorizationRecoveryRunner
}

// AuthorizationRecoverySchedulerOptions configures periodic recovery.
type AuthorizationRecoverySchedulerOptions struct {
	Interval time.Duration
	OnError  func(error)
}

func NewAuthorizationRecoveryScheduler(
	worker AuthorizationRecoveryRunner,
) (*AuthorizationRecoveryScheduler, error) {
	if worker == nil {
		return nil, errors.New(
			"FLEX authorization recovery scheduler worker is required",
		)
	}

	return &AuthorizationRecoveryScheduler{
		worker: worker,
	}, nil
}

// Run executes recovery periodically until its parent context
// is cancelled.
//
// Individual cycle failures are reported without terminating
// scheduling. Cancellation is treated as an expected exit.
//
// No detached per-candidate goroutines are created.
func (s *AuthorizationRecoveryScheduler) Run(
	ctx context.Context,
	options AuthorizationRecoverySchedulerOptions,
) error {
	if s == nil || s.worker == nil {
		return errors.New(
			"FLEX authorization recovery scheduler is unavailable",
		)
	}

	if ctx == nil {
		return errors.New(
			"FLEX authorization recovery scheduler context is required",
		)
	}

	if options.Interval <= 0 {
		return errors.New(
			"FLEX authorization recovery scheduler interval must be positive",
		)
	}

	if err := ctx.Err(); err != nil {
		return nil
	}

	ticker := time.NewTicker(options.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil

		case <-ticker.C:
			if err := ctx.Err(); err != nil {
				return nil
			}

			err := s.worker.RunOnce(ctx, options.OnError)

			if ctx.Err() != nil {
				return nil
			}

			if err != nil && options.OnError != nil {
				options.OnError(err)
			}
		}
	}
}
