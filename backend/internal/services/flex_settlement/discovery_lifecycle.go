package flex_settlement

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

// DiscoveryRunner is the background worker execution contract.
type DiscoveryRunner interface {
	Run(context.Context, DiscoveryWorkerOptions) error
}

// DiscoveryLifecycle manages one background worker instance.
//
// A lifecycle can be started only once. Stop is safe to call
// repeatedly, and Wait observes the worker's actual completion.
type DiscoveryLifecycle struct {
	worker DiscoveryRunner

	mu      sync.Mutex
	started bool
	cancel  context.CancelFunc
	done    chan struct{}
	runErr  error
	onExit  func(error)
}

func NewDiscoveryLifecycle(
	worker DiscoveryRunner,
) (*DiscoveryLifecycle, error) {
	if worker == nil {
		return nil, errors.New(
			"FLEX discovery lifecycle worker is required",
		)
	}

	return &DiscoveryLifecycle{
		worker: worker,
	}, nil
}

// Start launches one worker using an independent cancellable context.
// SetOnExit registers an optional worker completion callback.
//
// Registration is allowed only before Start.
// The callback executes after the worker completion channel closes.
// It must be short and nonblocking.
func (l *DiscoveryLifecycle) SetOnExit(
	callback func(error),
) error {
	if l == nil {
		return errors.New(
			"FLEX discovery lifecycle is not configured",
		)
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	if l.started {
		return errors.New(
			"FLEX discovery lifecycle already started",
		)
	}

	l.onExit = callback
	return nil
}

func (l *DiscoveryLifecycle) Start(
	parent context.Context,
	options DiscoveryWorkerOptions,
) error {
	if l == nil || l.worker == nil {
		return errors.New(
			"FLEX discovery lifecycle is not configured",
		)
	}

	if parent == nil {
		return errors.New(
			"FLEX discovery lifecycle context is required",
		)
	}

	if err := parent.Err(); err != nil {
		return err
	}

	if options.Interval <= 0 {
		return errors.New(
			"FLEX discovery lifecycle interval must be positive",
		)
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	if l.started {
		return errors.New(
			"FLEX discovery lifecycle already started",
		)
	}

	if err := parent.Err(); err != nil {
		return err
	}

	ctx, cancel := context.WithCancel(parent)

	l.cancel = cancel
	l.done = make(chan struct{})
	l.started = true

	go func() {
		err := l.worker.Run(ctx, options)

		l.mu.Lock()
		l.runErr = err
		callback := l.onExit
		l.mu.Unlock()

		// Worker completion must be observable independently
		// of callback execution.
		close(l.done)

		if callback != nil {
			callback(err)
		}
	}()

	return nil
}

// Stop requests cancellation without pretending the worker
// has already finished its in-flight operation.
func (l *DiscoveryLifecycle) Stop() {
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

// Wait blocks until the worker exits or the waiting context expires.
// A timeout does not imply that financial processing has stopped.
func (l *DiscoveryLifecycle) Wait(
	ctx context.Context,
) error {
	if l == nil {
		return errors.New(
			"FLEX discovery lifecycle is not configured",
		)
	}

	if ctx == nil {
		return errors.New(
			"FLEX discovery lifecycle wait context is required",
		)
	}

	l.mu.Lock()
	started := l.started
	done := l.done
	l.mu.Unlock()

	if !started {
		return errors.New(
			"FLEX discovery lifecycle has not started",
		)
	}

	select {
	case <-done:
		l.mu.Lock()
		err := l.runErr
		l.mu.Unlock()

		if err != nil {
			return fmt.Errorf(
				"FLEX discovery worker exited: %w",
				err,
			)
		}

		return nil

	case <-ctx.Done():
		return ctx.Err()
	}
}

// StopAndWait requests cancellation and waits for actual worker exit.
func (l *DiscoveryLifecycle) StopAndWait(
	timeout time.Duration,
) error {
	if timeout <= 0 {
		return errors.New(
			"FLEX discovery shutdown timeout must be positive",
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
