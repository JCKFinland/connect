package flex_settlement

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

type lifecycleTestRunner struct {
	entered chan struct{}
	release chan struct{}
	once    sync.Once
	runErr  error
}

func newLifecycleTestRunner() *lifecycleTestRunner {
	return &lifecycleTestRunner{
		entered: make(chan struct{}),
		release: make(chan struct{}),
	}
}

func (r *lifecycleTestRunner) Run(
	ctx context.Context,
	_ DiscoveryWorkerOptions,
) error {
	r.once.Do(func() {
		close(r.entered)
	})

	select {
	case <-r.release:
		return r.runErr
	case <-ctx.Done():
		return nil
	}
}

func lifecycleTestOptions() DiscoveryWorkerOptions {
	return DiscoveryWorkerOptions{
		Interval: time.Second,
	}
}

func awaitLifecycleStart(
	t *testing.T,
	runner *lifecycleTestRunner,
) {
	t.Helper()

	select {
	case <-runner.entered:
	case <-time.After(3 * time.Second):
		t.Fatal("lifecycle worker did not start")
	}
}

func TestDiscoveryLifecycleCompletesSuccessfully(t *testing.T) {
	runner := newLifecycleTestRunner()

	lifecycle, err := NewDiscoveryLifecycle(runner)
	if err != nil {
		t.Fatal(err)
	}

	if err := lifecycle.Start(
		context.Background(),
		lifecycleTestOptions(),
	); err != nil {
		t.Fatal(err)
	}

	awaitLifecycleStart(t, runner)
	close(runner.release)

	ctx, cancel := context.WithTimeout(
		context.Background(),
		3*time.Second,
	)
	defer cancel()

	if err := lifecycle.Wait(ctx); err != nil {
		t.Fatalf("successful worker returned error: %v", err)
	}

	if err := lifecycle.Wait(ctx); err != nil {
		t.Fatalf("repeated wait failed: %v", err)
	}
}

func TestDiscoveryLifecycleRejectsDuplicateStart(t *testing.T) {
	runner := newLifecycleTestRunner()

	lifecycle, err := NewDiscoveryLifecycle(runner)
	if err != nil {
		t.Fatal(err)
	}

	if err := lifecycle.Start(
		context.Background(),
		lifecycleTestOptions(),
	); err != nil {
		t.Fatal(err)
	}

	awaitLifecycleStart(t, runner)

	if err := lifecycle.Start(
		context.Background(),
		lifecycleTestOptions(),
	); err == nil {
		t.Fatal("duplicate lifecycle start was accepted")
	}

	if err := lifecycle.StopAndWait(3 * time.Second); err != nil {
		t.Fatal(err)
	}

	if err := lifecycle.Start(
		context.Background(),
		lifecycleTestOptions(),
	); err == nil {
		t.Fatal("restart after completion was accepted")
	}
}

func TestDiscoveryLifecycleStopIsIdempotent(t *testing.T) {
	runner := newLifecycleTestRunner()

	lifecycle, err := NewDiscoveryLifecycle(runner)
	if err != nil {
		t.Fatal(err)
	}

	if err := lifecycle.Start(
		context.Background(),
		lifecycleTestOptions(),
	); err != nil {
		t.Fatal(err)
	}

	awaitLifecycleStart(t, runner)

	lifecycle.Stop()
	lifecycle.Stop()
	lifecycle.Stop()

	ctx, cancel := context.WithTimeout(
		context.Background(),
		3*time.Second,
	)
	defer cancel()

	if err := lifecycle.Wait(ctx); err != nil {
		t.Fatalf("worker did not stop cleanly: %v", err)
	}

	lifecycle.Stop()
}

func TestDiscoveryLifecycleStopAndWait(t *testing.T) {
	runner := newLifecycleTestRunner()

	lifecycle, err := NewDiscoveryLifecycle(runner)
	if err != nil {
		t.Fatal(err)
	}

	if err := lifecycle.Start(
		context.Background(),
		lifecycleTestOptions(),
	); err != nil {
		t.Fatal(err)
	}

	awaitLifecycleStart(t, runner)

	if err := lifecycle.StopAndWait(
		3 * time.Second,
	); err != nil {
		t.Fatalf("shutdown failed: %v", err)
	}
}

func TestDiscoveryLifecycleWaitTimeoutDoesNotClaimExit(t *testing.T) {
	runner := newLifecycleTestRunner()

	lifecycle, err := NewDiscoveryLifecycle(runner)
	if err != nil {
		t.Fatal(err)
	}

	if err := lifecycle.Start(
		context.Background(),
		lifecycleTestOptions(),
	); err != nil {
		t.Fatal(err)
	}

	awaitLifecycleStart(t, runner)

	ctx, cancel := context.WithCancel(
		context.Background(),
	)
	cancel()

	if err := lifecycle.Wait(ctx); !errors.Is(
		err,
		context.Canceled,
	) {
		t.Fatalf("expected cancelled wait, got %v", err)
	}

	select {
	case <-lifecycle.done:
		t.Fatal("wait cancellation incorrectly stopped worker")
	default:
	}

	close(runner.release)

	if err := lifecycle.StopAndWait(
		3 * time.Second,
	); err != nil {
		t.Fatal(err)
	}
}

func TestDiscoveryLifecyclePropagatesWorkerError(t *testing.T) {
	runner := newLifecycleTestRunner()
	failure := errors.New("worker execution failed")
	runner.runErr = failure

	lifecycle, err := NewDiscoveryLifecycle(runner)
	if err != nil {
		t.Fatal(err)
	}

	if err := lifecycle.Start(
		context.Background(),
		lifecycleTestOptions(),
	); err != nil {
		t.Fatal(err)
	}

	awaitLifecycleStart(t, runner)
	close(runner.release)

	ctx, cancel := context.WithTimeout(
		context.Background(),
		3*time.Second,
	)
	defer cancel()

	err = lifecycle.Wait(ctx)

	if !errors.Is(err, failure) {
		t.Fatalf("worker failure not preserved: %v", err)
	}

	if err == nil || !strings.Contains(
		err.Error(),
		"FLEX discovery worker exited",
	) {
		t.Fatalf("missing lifecycle error context: %v", err)
	}
}

func TestDiscoveryLifecycleRejectsInvalidConfiguration(t *testing.T) {
	if _, err := NewDiscoveryLifecycle(nil); err == nil {
		t.Fatal("nil worker accepted")
	}

	runner := newLifecycleTestRunner()

	lifecycle, err := NewDiscoveryLifecycle(runner)
	if err != nil {
		t.Fatal(err)
	}

	if err := lifecycle.Wait(
		context.Background(),
	); err == nil {
		t.Fatal("wait before start accepted")
	}

	if err := lifecycle.Start(
		nil,
		lifecycleTestOptions(),
	); err == nil {
		t.Fatal("nil startup context accepted")
	}

	cancelled, cancel := context.WithCancel(
		context.Background(),
	)
	cancel()

	if err := lifecycle.Start(
		cancelled,
		lifecycleTestOptions(),
	); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled startup accepted: %v", err)
	}

	if err := lifecycle.Start(
		context.Background(),
		DiscoveryWorkerOptions{},
	); err == nil {
		t.Fatal("zero polling interval accepted")
	}

	if err := lifecycle.StopAndWait(0); err == nil {
		t.Fatal("zero shutdown timeout accepted")
	}

	lifecycle.Stop()

	if err := lifecycle.Start(
		context.Background(),
		lifecycleTestOptions(),
	); err != nil {
		t.Fatalf("valid startup failed: %v", err)
	}

	awaitLifecycleStart(t, runner)

	if err := lifecycle.StopAndWait(
		3 * time.Second,
	); err != nil {
		t.Fatal(err)
	}
}
