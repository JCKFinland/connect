package presence

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/JCKFinland/connect/backend/internal/config"
)

func TestStalePresenceWorkerRunsBothPassesInOrder(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var mu sync.Mutex
	var calls []string

	done := make(chan struct{})

	service := &Service{
		cfg: &config.Config{},
		reconcileStaleIdleBounded: func(
			ctx context.Context,
			now time.Time,
			batchSize int,
		) (int64, error) {
			mu.Lock()
			calls = append(calls, "stale")
			mu.Unlock()
			return 0, nil
		},
		reconcileRegulatoryBounded: func(
			ctx context.Context,
			now time.Time,
			batchSize int,
		) (int64, error) {
			mu.Lock()
			calls = append(calls, "regulatory")
			mu.Unlock()
			cancel()
			return 0, nil
		},
	}

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
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("worker did not complete initial cycle")
	}

	mu.Lock()
	defer mu.Unlock()

	if len(calls) != 2 ||
		calls[0] != "stale" ||
		calls[1] != "regulatory" {
		t.Fatalf("unexpected reconciliation order: %v", calls)
	}
}

func TestStalePresenceWorkerRunsRegulatoryAfterStaleError(
	t *testing.T,
) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	staleErr := errors.New("stale reconciliation failed")

	reported := make(chan error, 1)
	regulatoryCalled := make(chan struct{}, 1)
	done := make(chan struct{})

	service := &Service{
		cfg: &config.Config{},
		reconcileStaleIdleBounded: func(
			ctx context.Context,
			now time.Time,
			batchSize int,
		) (int64, error) {
			return 0, staleErr
		},
		reconcileRegulatoryBounded: func(
			ctx context.Context,
			now time.Time,
			batchSize int,
		) (int64, error) {
			regulatoryCalled <- struct{}{}
			cancel()
			return 0, nil
		},
	}

	go func() {
		defer close(done)
		service.StartStalePresenceWorker(
			ctx,
			StalePresenceWorkerOptions{
				Interval: time.Hour,
				OnError: func(err error) {
					reported <- err
				},
			},
		)
	}()

	select {
	case <-regulatoryCalled:
	case <-time.After(3 * time.Second):
		t.Fatal("stale error suppressed regulatory reconciliation")
	}

	select {
	case err := <-reported:
		if !errors.Is(err, staleErr) {
			t.Fatalf("unexpected reported error: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("stale reconciliation error was not reported")
	}

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("worker did not stop")
	}
}

func TestStalePresenceWorkerContinuesAfterRegulatoryError(
	t *testing.T,
) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	regulatoryErr := errors.New("regulatory reconciliation failed")

	reported := make(chan error, 2)
	secondCycle := make(chan struct{}, 1)
	done := make(chan struct{})

	var mu sync.Mutex
	cycles := 0

	service := &Service{
		cfg: &config.Config{},
		reconcileStaleIdleBounded: func(
			ctx context.Context,
			now time.Time,
			batchSize int,
		) (int64, error) {
			mu.Lock()
			cycles++
			current := cycles
			mu.Unlock()

			if current == 2 {
				secondCycle <- struct{}{}
				cancel()
			}

			return 0, nil
		},
		reconcileRegulatoryBounded: func(
			ctx context.Context,
			now time.Time,
			batchSize int,
		) (int64, error) {
			return 0, regulatoryErr
		},
	}

	go func() {
		defer close(done)
		service.StartStalePresenceWorker(
			ctx,
			StalePresenceWorkerOptions{
				Interval: 10 * time.Millisecond,
				OnError: func(err error) {
					reported <- err
				},
			},
		)
	}()

	select {
	case err := <-reported:
		if !errors.Is(err, regulatoryErr) {
			t.Fatalf("unexpected reported error: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("regulatory error was not reported")
	}

	select {
	case <-secondCycle:
	case <-time.After(3 * time.Second):
		t.Fatal("worker stopped after regulatory error")
	}

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("worker did not stop after cancellation")
	}
}
