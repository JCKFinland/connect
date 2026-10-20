package main

import (
	"context"
	"errors"
	"testing"
	"time"
)

// Startup cleanup must not allow database resource teardown
// until a started financial worker has actually completed.
func TestStartupFlexCleanupWaitsForCompletion(t *testing.T) {
	worker := newControlledFlexShutdownWaiter()

	completed := make(chan error, 1)

	go func() {
		completed <- waitForFlexShutdown(
			worker,
			20*time.Millisecond,
		)
	}()

	deadline := time.After(time.Second)

	for worker.callCount() < 2 {
		select {
		case <-deadline:
			t.Fatal("financial cleanup did not enter completion wait")
		default:
			time.Sleep(time.Millisecond)
		}
	}

	select {
	case <-completed:
		t.Fatal("cleanup returned while worker was active")
	default:
	}

	close(worker.done)

	select {
	case err := <-completed:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("financial cleanup did not finish")
	}
}

func TestStartupFlexCleanupPreservesWorkerError(t *testing.T) {
	worker := newControlledFlexShutdownWaiter()
	expected := errors.New("financial worker failed")
	worker.completionErr = expected

	close(worker.done)

	err := waitForFlexShutdown(
		worker,
		time.Second,
	)

	if !errors.Is(err, expected) {
		t.Fatalf("expected worker error, got %v", err)
	}
}

func TestStartupOperationalCleanupWaitsForCompletion(t *testing.T) {
	workerDone := make(chan struct{})
	cancelled := make(chan struct{})
	finished := make(chan struct{})

	go func() {
		defer close(finished)

		cancel := func() {
			close(cancelled)
		}

		defer func() {
			cancel()
			<-workerDone
		}()
	}()

	select {
	case <-cancelled:
	case <-time.After(time.Second):
		t.Fatal("worker cancellation not issued")
	}

	select {
	case <-finished:
		t.Fatal("cleanup returned before worker completion")
	default:
	}

	close(workerDone)

	select {
	case <-finished:
	case <-time.After(time.Second):
		t.Fatal("cleanup did not finish")
	}
}

func TestStartupCleanupUsesIndependentContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	worker := newControlledFlexShutdownWaiter()
	close(worker.done)

	if err := waitForFlexShutdown(
		worker,
		time.Second,
	); err != nil {
		t.Fatal(err)
	}

	if ctx.Err() == nil {
		t.Fatal("test context should be cancelled")
	}
}
