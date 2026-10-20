package flex_settlement

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestDiscoveryLifecycleConcurrentShutdownWaitsForSameWorker(
	t *testing.T,
) {
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

	// First shutdown requests cancellation but cannot claim
	// completion while the financial operation remains blocked.
	err = lifecycle.StopAndWait(20 * time.Millisecond)

	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf(
			"expected first shutdown timeout, got %v",
			err,
		)
	}

	secondDone := make(chan error, 1)

	go func() {
		secondDone <- lifecycle.StopAndWait(
			3 * time.Second,
		)
	}()

	// The second shutdown must not report completion before
	// the original worker has actually exited.
	select {
	case err := <-secondDone:
		t.Fatalf(
			"second shutdown returned before completion: %v",
			err,
		)
	default:
	}

	select {
	case <-lifecycle.done:
		t.Fatal("worker reported completion while blocked")
	default:
	}

	// Release the one original in-flight operation.
	close(runner.release)

	select {
	case err := <-secondDone:
		if err != nil {
			t.Fatalf(
				"second shutdown failed after release: %v",
				err,
			)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("second shutdown never completed")
	}

	select {
	case <-lifecycle.done:
	default:
		t.Fatal("worker completion channel remains open")
	}

	cancelled, completed := runner.state()

	if !cancelled || !completed {
		t.Fatalf(
			"unexpected worker state: cancelled=%v completed=%v",
			cancelled,
			completed,
		)
	}

	// Once the worker has exited, further shutdown calls
	// must observe that same completed execution.
	if err := lifecycle.StopAndWait(
		3 * time.Second,
	); err != nil {
		t.Fatalf(
			"repeated completed shutdown failed: %v",
			err,
		)
	}

	// The lifecycle must never restart its completed worker.
	if err := lifecycle.Start(
		context.Background(),
		DiscoveryWorkerOptions{
			Interval: time.Second,
		},
	); err == nil {
		t.Fatal("completed lifecycle unexpectedly restarted")
	}
}

func TestDiscoveryLifecycleConcurrentWaitersObserveCompletion(
	t *testing.T,
) {
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

	const waiterCount = 8

	results := make(chan error, waiterCount)

	for i := 0; i < waiterCount; i++ {
		go func() {
			ctx, cancel := context.WithTimeout(
				context.Background(),
				3*time.Second,
			)
			defer cancel()

			results <- lifecycle.Wait(ctx)
		}()
	}

	// Cancellation alone must not complete the worker.
	lifecycle.Stop()

	select {
	case <-lifecycle.done:
		t.Fatal("worker exited before operation release")
	default:
	}

	close(runner.release)

	for i := 0; i < waiterCount; i++ {
		select {
		case err := <-results:
			if err != nil {
				t.Fatalf(
					"waiter %d failed: %v",
					i,
					err,
				)
			}
		case <-time.After(3 * time.Second):
			t.Fatalf("waiter %d timed out", i)
		}
	}

	if err := lifecycle.StopAndWait(
		3 * time.Second,
	); err != nil {
		t.Fatal(err)
	}
}
