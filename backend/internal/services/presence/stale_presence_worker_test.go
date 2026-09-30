package presence

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/JCKFinland/connect/backend/internal/config"
)

func TestStartStalePresenceWorkerRunsImmediately(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	called := make(chan struct{}, 1)

	service := &Service{
		cfg: &config.Config{
			Presence: config.PresenceConfig{
				HeartbeatTimeout: 2 * time.Minute,
			},
		},
		expireAllStaleIdle: func(
			ctx context.Context,
			staleBefore time.Time,
		) (int64, error) {
			select {
			case called <- struct{}{}:
			default:
			}

			return 0, nil
		},
	}

	done := make(chan struct{})

	go func() {
		defer close(done)

		service.StartStalePresenceWorker(
			ctx,
			StalePresenceWorkerOptions{
				Interval: time.Hour,
			},
		)
	}()

	select {
	case <-called:
	case <-time.After(time.Second):
		t.Fatal("worker did not run reconciliation immediately")
	}

	cancel()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("worker did not stop after cancellation")
	}
}

func TestStartStalePresenceWorkerContinuesAfterCycleError(
	t *testing.T,
) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	repositoryErr := errors.New("database unavailable")

	var cycleCount atomic.Int32

	secondCycle := make(chan struct{}, 1)
	reportedError := make(chan error, 1)

	service := &Service{
		cfg: &config.Config{
			Presence: config.PresenceConfig{
				HeartbeatTimeout: 2 * time.Minute,
			},
		},
		expireAllStaleIdle: func(
			ctx context.Context,
			staleBefore time.Time,
		) (int64, error) {
			current := cycleCount.Add(1)

			if current == 1 {
				return 0, repositoryErr
			}

			if current == 2 {
				select {
				case secondCycle <- struct{}{}:
				default:
				}
			}

			return 0, nil
		},
	}

	done := make(chan struct{})

	go func() {
		defer close(done)

		service.StartStalePresenceWorker(
			ctx,
			StalePresenceWorkerOptions{
				Interval: 10 * time.Millisecond,
				OnError: func(err error) {
					select {
					case reportedError <- err:
					default:
					}
				},
			},
		)
	}()

	select {
	case err := <-reportedError:
		if !errors.Is(err, repositoryErr) {
			t.Fatalf(
				"expected wrapped repository error, got %v",
				err,
			)
		}

	case <-time.After(time.Second):
		t.Fatal("worker did not report first-cycle error")
	}

	select {
	case <-secondCycle:
	case <-time.After(time.Second):
		t.Fatal("worker did not continue after cycle error")
	}

	cancel()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("worker did not stop after cancellation")
	}
}

func TestStartStalePresenceWorkerStopsOnCancelledCycle(
	t *testing.T,
) {
	ctx, cancel := context.WithCancel(context.Background())

	started := make(chan struct{}, 1)

	var (
		errorMu       sync.Mutex
		reportedError error
	)

	service := &Service{
		cfg: &config.Config{
			Presence: config.PresenceConfig{
				HeartbeatTimeout: 2 * time.Minute,
			},
		},
		expireAllStaleIdle: func(
			ctx context.Context,
			staleBefore time.Time,
		) (int64, error) {
			select {
			case started <- struct{}{}:
			default:
			}

			<-ctx.Done()

			return 0, ctx.Err()
		},
	}

	done := make(chan struct{})

	go func() {
		defer close(done)

		service.StartStalePresenceWorker(
			ctx,
			StalePresenceWorkerOptions{
				Interval: time.Hour,
				OnError: func(err error) {
					errorMu.Lock()
					reportedError = err
					errorMu.Unlock()
				},
			},
		)
	}()

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("worker did not start reconciliation")
	}

	cancel()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("worker did not stop after cycle cancellation")
	}

	errorMu.Lock()
	defer errorMu.Unlock()

	if reportedError != nil {
		t.Fatalf(
			"expected cancellation not to be reported as error, got %v",
			reportedError,
		)
	}
}

func TestStartStalePresenceWorkerRejectsInvalidDerivedInterval(
	t *testing.T,
) {
	reportedError := make(chan error, 1)

	service := &Service{
		cfg: &config.Config{
			Presence: config.PresenceConfig{
				HeartbeatTimeout: 0,
			},
		},
	}

	service.StartStalePresenceWorker(
		context.Background(),
		StalePresenceWorkerOptions{
			OnError: func(err error) {
				reportedError <- err
			},
		},
	)

	select {
	case err := <-reportedError:
		if err == nil {
			t.Fatal("expected invalid interval error")
		}

	default:
		t.Fatal("expected invalid interval to be reported")
	}
}
