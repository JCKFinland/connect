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
		reconcileStaleIdleBounded: func(
			ctx context.Context,
			now time.Time,
			batchSize int,
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
		reconcileStaleIdleBounded: func(
			ctx context.Context,
			now time.Time,
			batchSize int,
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
		reconcileStaleIdleBounded: func(
			ctx context.Context,
			now time.Time,
			batchSize int,
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

// TestStartStalePresenceWorkerBatchSize verifies that the worker passes
// the configured candidate limit to bounded reconciliation.
func TestStartStalePresenceWorkerBatchSize(t *testing.T) {
	tests := []struct {
		name      string
		batchSize int
		want      int
	}{
		{
			name:      "default batch size",
			batchSize: 0,
			want:      defaultStaleIdleBatchSize,
		},
		{
			name:      "negative batch size uses default",
			batchSize: -1,
			want:      defaultStaleIdleBatchSize,
		},
		{
			name:      "configured batch size",
			batchSize: 25,
			want:      25,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(
				context.Background(),
			)
			defer cancel()

			received := make(chan int, 1)
			done := make(chan struct{})

			service := &Service{
				cfg: &config.Config{
					Presence: config.PresenceConfig{
						HeartbeatTimeout: 2 * time.Minute,
					},
				},
				reconcileStaleIdleBounded: func(
					ctx context.Context,
					now time.Time,
					batchSize int,
				) (int64, error) {
					select {
					case received <- batchSize:
					default:
					}

					return 0, nil
				},
			}

			go func() {
				defer close(done)

				service.StartStalePresenceWorker(
					ctx,
					StalePresenceWorkerOptions{
						Interval:  time.Hour,
						BatchSize: tc.batchSize,
					},
				)
			}()

			select {
			case got := <-received:
				if got != tc.want {
					t.Errorf(
						"batch size: got %d, want %d",
						got,
						tc.want,
					)
				}

			case <-time.After(3 * time.Second):
				t.Error(
					"worker did not invoke bounded reconciliation",
				)
			}

			cancel()

			select {
			case <-done:
			case <-time.After(3 * time.Second):
				t.Fatal(
					"worker did not stop after cancellation",
				)
			}
		})
	}
}
