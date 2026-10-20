package flex_settlement

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

// inflightFinancialRunner models a provider operation that has
// already started and cannot be considered complete merely
// because its context was cancelled.
//
// It performs no actual financial transaction.
type inflightFinancialRunner struct {
	entered chan struct{}
	release chan struct{}

	mu                 sync.Mutex
	cancellationSeen   bool
	operationCompleted bool
}

func newInflightFinancialRunner() *inflightFinancialRunner {
	return &inflightFinancialRunner{
		entered: make(chan struct{}),
		release: make(chan struct{}),
	}
}

func (r *inflightFinancialRunner) Run(
	ctx context.Context,
	_ DiscoveryWorkerOptions,
) error {
	close(r.entered)

	// Simulate a financial operation already in flight.
	// Cancellation must not falsely mark it complete.
	<-r.release

	r.mu.Lock()
	r.cancellationSeen = ctx.Err() != nil
	r.operationCompleted = true
	r.mu.Unlock()

	return nil
}

func (r *inflightFinancialRunner) state() (bool, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	return r.cancellationSeen, r.operationCompleted
}

func TestDiscoveryLifecycleInflightShutdownTimeout(t *testing.T) {
	runner := newInflightFinancialRunner()

	lifecycle, err := NewDiscoveryLifecycle(runner)
	if err != nil {
		t.Fatal(err)
	}

	if err := lifecycle.Start(
		context.Background(),
		DiscoveryWorkerOptions{
			Interval: time.Second,
		},
	); err != nil {
		t.Fatal(err)
	}

	select {
	case <-runner.entered:
	case <-time.After(3 * time.Second):
		t.Fatal("financial operation never started")
	}

	// StopAndWait must request cancellation but must not
	// claim the in-flight financial operation has completed.
	err = lifecycle.StopAndWait(20 * time.Millisecond)

	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf(
			"expected shutdown deadline exceeded, got %v",
			err,
		)
	}

	select {
	case <-lifecycle.done:
		t.Fatal(
			"lifecycle incorrectly reported financial completion",
		)
	default:
	}

	_, completed := runner.state()
	if completed {
		t.Fatal("financial operation completed before release")
	}

	// Explicitly release the simulated provider operation.
	close(runner.release)

	if err := lifecycle.StopAndWait(
		3 * time.Second,
	); err != nil {
		t.Fatalf(
			"shutdown after provider completion failed: %v",
			err,
		)
	}

	cancelled, completed := runner.state()

	if !cancelled {
		t.Fatal("worker did not observe cancellation")
	}

	if !completed {
		t.Fatal("worker did not finish financial operation")
	}

	// Repeated shutdown must remain safe.
	if err := lifecycle.StopAndWait(
		3 * time.Second,
	); err != nil {
		t.Fatalf("repeated shutdown failed: %v", err)
	}
}

func TestDiscoveryLifecycleInflightWaitDoesNotCancel(t *testing.T) {
	runner := newInflightFinancialRunner()

	lifecycle, err := NewDiscoveryLifecycle(runner)
	if err != nil {
		t.Fatal(err)
	}

	if err := lifecycle.Start(
		context.Background(),
		DiscoveryWorkerOptions{
			Interval: time.Second,
		},
	); err != nil {
		t.Fatal(err)
	}

	select {
	case <-runner.entered:
	case <-time.After(3 * time.Second):
		t.Fatal("financial operation never started")
	}

	waitCtx, cancel := context.WithCancel(
		context.Background(),
	)
	cancel()

	err = lifecycle.Wait(waitCtx)

	if !errors.Is(err, context.Canceled) {
		t.Fatalf(
			"expected cancelled wait, got %v",
			err,
		)
	}

	select {
	case <-lifecycle.done:
		t.Fatal("cancelled wait incorrectly completed worker")
	default:
	}

	// A cancelled waiter must not cancel the actual worker.
	if runnerCtxCancelled, _ := runner.state(); runnerCtxCancelled {
		t.Fatal("wait cancellation affected worker")
	}

	close(runner.release)

	if err := lifecycle.StopAndWait(
		3 * time.Second,
	); err != nil {
		t.Fatal(err)
	}
}
