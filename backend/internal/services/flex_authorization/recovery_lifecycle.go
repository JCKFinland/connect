package flex_authorization

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

// AuthorizationRecoverySchedulerRunner is the background
// scheduling contract. It does not grant financial permissions.
type AuthorizationRecoverySchedulerRunner interface {
	Run(context.Context, AuthorizationRecoverySchedulerOptions) error
}

// AuthorizationRecoveryLifecycle owns one scheduler execution.
//
// Construction does not start background processing.
// Start may succeed only once.
// Stop requests cancellation; Wait confirms actual completion.
type AuthorizationRecoveryLifecycle struct {
	worker AuthorizationRecoverySchedulerRunner

	mu      sync.Mutex
	started bool
	cancel  context.CancelFunc
	done    chan struct{}
	runErr  error
	onExit  func(error)
}

func NewAuthorizationRecoveryLifecycle(
	worker AuthorizationRecoverySchedulerRunner,
) (*AuthorizationRecoveryLifecycle, error) {
	if worker == nil {
		return nil, errors.New(
			"FLEX authorization recovery lifecycle worker is required",
		)
	}

	return &AuthorizationRecoveryLifecycle{
		worker: worker,
	}, nil
}

// SetOnExit registers a completion callback before startup.
//
// The callback runs after the completion channel closes.
// It must be short and nonblocking.
func (l *AuthorizationRecoveryLifecycle) SetOnExit(
	callback func(error),
) error {
	if l == nil {
		return errors.New(
			"FLEX authorization recovery lifecycle is unavailable",
		)
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	if l.started {
		return errors.New(
			"FLEX authorization recovery lifecycle already started",
		)
	}

	l.onExit = callback
	return nil
}

// Start launches one independently cancellable scheduler.
//
// The caller's context remains the parent, so application
// cancellation also propagates to the scheduler.
func (l *AuthorizationRecoveryLifecycle) Start(
	parent context.Context,
	options AuthorizationRecoverySchedulerOptions,
) error {
	if l == nil || l.worker == nil {
		return errors.New(
			"FLEX authorization recovery lifecycle is unavailable",
		)
	}

	if parent == nil {
		return errors.New(
			"FLEX authorization recovery lifecycle context is required",
		)
	}

	if err := parent.Err(); err != nil {
		return err
	}

	if options.Interval <= 0 {
		return errors.New(
			"FLEX authorization recovery lifecycle interval must be positive",
		)
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	if l.started {
		return errors.New(
			"FLEX authorization recovery lifecycle already started",
		)
	}

	if err := parent.Err(); err != nil {
		return err
	}

	ctx, cancel := context.WithCancel(parent)

	l.cancel = cancel
	l.done = make(chan struct{})
	l.started = true

	done := l.done

	go func() {
		err := l.worker.Run(ctx, options)

		l.mu.Lock()
		l.runErr = err
		callback := l.onExit
		l.mu.Unlock()

		close(done)

		if callback != nil {
			callback(err)
		}
	}()

	return nil
}

// Stop requests cancellation without claiming that the
// scheduler or an in-flight recovery attempt has exited.
func (l *AuthorizationRecoveryLifecycle) Stop() {
	if l == nil {
		return
	}

	l.mu.Lock()
	cancel := l.cancel
	l.mu.Unlock()

	if cancel != nil {
		cancel()
	}
}

// Wait observes actual scheduler completion.
//
// A wait timeout never establishes that the scheduler stopped.
func (l *AuthorizationRecoveryLifecycle) Wait(
	ctx context.Context,
) error {
	if l == nil {
		return errors.New(
			"FLEX authorization recovery lifecycle is unavailable",
		)
	}

	if ctx == nil {
		return errors.New(
			"FLEX authorization recovery wait context is required",
		)
	}

	l.mu.Lock()
	started := l.started
	done := l.done
	l.mu.Unlock()

	if !started {
		return errors.New(
			"FLEX authorization recovery lifecycle has not started",
		)
	}

	select {
	case <-done:
		l.mu.Lock()
		err := l.runErr
		l.mu.Unlock()

		if err != nil {
			return fmt.Errorf(
				"FLEX authorization recovery scheduler exited: %w",
				err,
			)
		}

		return nil

	case <-ctx.Done():
		return ctx.Err()
	}
}

// StopAndWait requests cancellation and waits for actual exit.
//
// The timeout bounds the wait, not the underlying goroutine.
// Callers must not treat a deadline error as proof of shutdown.
func (l *AuthorizationRecoveryLifecycle) StopAndWait(
	timeout time.Duration,
) error {
	if timeout <= 0 {
		return errors.New(
			"FLEX authorization recovery shutdown timeout must be positive",
		)
	}

	l.Stop()

	ctx, cancel := context.WithTimeout(
		context.Background(),
		timeout,
	)
	defer cancel()

	return l.Wait(ctx)
}
