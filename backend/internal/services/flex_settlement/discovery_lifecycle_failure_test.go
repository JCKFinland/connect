package flex_settlement

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type failingLifecycleRunner struct {
	calls atomic.Int32
	err   error
}

func (r *failingLifecycleRunner) Run(
	_ context.Context,
	_ DiscoveryWorkerOptions,
) error {
	r.calls.Add(1)
	return r.err
}

func TestDiscoveryLifecycleUnexpectedWorkerFailure(t *testing.T) {
	failure := errors.New("simulated settlement worker failure")

	runner := &failingLifecycleRunner{
		err: failure,
	}

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

	ctx, cancel := context.WithTimeout(
		context.Background(),
		3*time.Second,
	)
	defer cancel()

	err = lifecycle.Wait(ctx)

	if !errors.Is(err, failure) {
		t.Fatalf("worker failure not preserved: %v", err)
	}

	if !strings.Contains(
		err.Error(),
		"FLEX discovery worker exited",
	) {
		t.Fatalf("worker failure lacks lifecycle context: %v", err)
	}

	if runner.calls.Load() != 1 {
		t.Fatalf(
			"expected one worker execution, got %d",
			runner.calls.Load(),
		)
	}

	if err := lifecycle.Start(
		context.Background(),
		DiscoveryWorkerOptions{
			Interval: time.Second,
		},
	); err == nil {
		t.Fatal("failed worker unexpectedly restarted")
	}

	if runner.calls.Load() != 1 {
		t.Fatal("worker executed more than once")
	}
}

func TestDiscoveryLifecycleImmediateCleanExit(t *testing.T) {
	runner := &failingLifecycleRunner{}

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

	ctx, cancel := context.WithTimeout(
		context.Background(),
		3*time.Second,
	)
	defer cancel()

	if err := lifecycle.Wait(ctx); err != nil {
		t.Fatalf("unexpected lifecycle error: %v", err)
	}

	if runner.calls.Load() != 1 {
		t.Fatalf(
			"expected one execution, got %d",
			runner.calls.Load(),
		)
	}

	if err := lifecycle.Start(
		context.Background(),
		DiscoveryWorkerOptions{
			Interval: time.Second,
		},
	); err == nil {
		t.Fatal("completed worker unexpectedly restarted")
	}
}

func TestDiscoveryLifecycleRepeatedFailureObservation(t *testing.T) {
	failure := errors.New("persistent worker failure")

	runner := &failingLifecycleRunner{
		err: failure,
	}

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

	for i := 0; i < 5; i++ {
		ctx, cancel := context.WithTimeout(
			context.Background(),
			3*time.Second,
		)

		err := lifecycle.Wait(ctx)
		cancel()

		if !errors.Is(err, failure) {
			t.Fatalf(
				"wait %d lost worker failure: %v",
				i,
				err,
			)
		}
	}

	if err := lifecycle.StopAndWait(
		3 * time.Second,
	); !errors.Is(err, failure) {
		t.Fatalf(
			"shutdown lost worker failure: %v",
			err,
		)
	}
}
