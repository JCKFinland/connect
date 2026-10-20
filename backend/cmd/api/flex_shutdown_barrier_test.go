package main

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

type controlledFlexShutdownWaiter struct {
	mu sync.Mutex

	calls int

	done chan struct{}

	completionErr error
}

func newControlledFlexShutdownWaiter() *controlledFlexShutdownWaiter {
	return &controlledFlexShutdownWaiter{
		done: make(chan struct{}),
	}
}

func (w *controlledFlexShutdownWaiter) StopAndWait(
	ctx context.Context,
) error {
	w.mu.Lock()
	w.calls++
	w.mu.Unlock()

	select {
	case <-w.done:
		return w.completionErr
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (w *controlledFlexShutdownWaiter) callCount() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.calls
}

func TestFlexShutdownBarrierNormalCompletion(t *testing.T) {
	worker := newControlledFlexShutdownWaiter()
	close(worker.done)

	if err := waitForFlexShutdown(
		worker,
		time.Second,
	); err != nil {
		t.Fatal(err)
	}

	if got := worker.callCount(); got != 1 {
		t.Fatalf("expected one wait, got %d", got)
	}
}

func TestFlexShutdownBarrierPreservesWorkerError(t *testing.T) {
	worker := newControlledFlexShutdownWaiter()

	failure := errors.New("worker completed with error")
	worker.completionErr = failure

	close(worker.done)

	err := waitForFlexShutdown(
		worker,
		time.Second,
	)

	if !errors.Is(err, failure) {
		t.Fatalf("expected worker error, got %v", err)
	}

	if got := worker.callCount(); got != 1 {
		t.Fatalf(
			"completed worker must not be waited twice: %d",
			got,
		)
	}
}

func TestFlexShutdownBarrierWaitsBeyondDeadline(t *testing.T) {
	worker := newControlledFlexShutdownWaiter()

	result := make(chan error, 1)

	go func() {
		result <- waitForFlexShutdown(
			worker,
			20*time.Millisecond,
		)
	}()

	deadline := time.After(2 * time.Second)

	for worker.callCount() < 2 {
		select {
		case <-deadline:
			t.Fatal("fallback completion wait did not begin")
		default:
			time.Sleep(time.Millisecond)
		}
	}

	select {
	case err := <-result:
		t.Fatalf(
			"shutdown returned before financial completion: %v",
			err,
		)
	default:
	}

	close(worker.done)

	select {
	case err := <-result:
		if err != nil {
			t.Fatalf("completion wait failed: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("shutdown did not finish after worker completion")
	}
}

func TestFlexShutdownBarrierRejectsInvalidInputs(t *testing.T) {
	if err := waitForFlexShutdown(
		nil,
		time.Second,
	); err == nil {
		t.Fatal("nil worker accepted")
	}

	worker := newControlledFlexShutdownWaiter()

	if err := waitForFlexShutdown(
		worker,
		0,
	); err == nil {
		t.Fatal("invalid timeout accepted")
	}

	if got := worker.callCount(); got != 0 {
		t.Fatalf("unexpected worker invocation: %d", got)
	}
}
