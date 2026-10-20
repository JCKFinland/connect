package flex_settlement

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

type lifecycleExitEvent struct {
	err error
}

func TestDiscoveryLifecycleExitCallbackSuccess(t *testing.T) {
	runner := newLifecycleTestRunner()

	lifecycle, err := NewDiscoveryLifecycle(runner)
	if err != nil {
		t.Fatal(err)
	}

	events := make(chan lifecycleExitEvent, 1)

	if err := lifecycle.SetOnExit(func(err error) {
		events <- lifecycleExitEvent{err: err}
	}); err != nil {
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
		t.Fatal(err)
	}

	select {
	case event := <-events:
		if event.err != nil {
			t.Fatalf("unexpected callback error: %v", event.err)
		}
	case <-ctx.Done():
		t.Fatal("exit callback was not delivered")
	}
}

func TestDiscoveryLifecycleExitCallbackPreservesFailure(
	t *testing.T,
) {
	failure := errors.New("simulated FLEX worker failure")

	runner := &failingLifecycleRunner{
		err: failure,
	}

	lifecycle, err := NewDiscoveryLifecycle(runner)
	if err != nil {
		t.Fatal(err)
	}

	events := make(chan lifecycleExitEvent, 1)

	if err := lifecycle.SetOnExit(func(err error) {
		events <- lifecycleExitEvent{err: err}
	}); err != nil {
		t.Fatal(err)
	}

	if err := lifecycle.Start(
		context.Background(),
		lifecycleTestOptions(),
	); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(
		context.Background(),
		3*time.Second,
	)
	defer cancel()

	if err := lifecycle.Wait(ctx); !errors.Is(err, failure) {
		t.Fatalf("Wait lost worker failure: %v", err)
	}

	select {
	case event := <-events:
		if !errors.Is(event.err, failure) {
			t.Fatalf(
				"callback lost worker failure: %v",
				event.err,
			)
		}
	case <-ctx.Done():
		t.Fatal("failure callback was not delivered")
	}
}

func TestDiscoveryLifecycleExitCallbackRejectsLateRegistration(
	t *testing.T,
) {
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

	if err := lifecycle.SetOnExit(func(error) {}); err == nil {
		t.Fatal("callback registration accepted after Start")
	}

	close(runner.release)

	if err := lifecycle.StopAndWait(
		3 * time.Second,
	); err != nil {
		t.Fatal(err)
	}

	if err := lifecycle.SetOnExit(func(error) {}); err == nil {
		t.Fatal("callback registration accepted after completion")
	}
}

func TestDiscoveryLifecycleExitCallbackDoesNotBlockWait(
	t *testing.T,
) {
	runner := newLifecycleTestRunner()

	lifecycle, err := NewDiscoveryLifecycle(runner)
	if err != nil {
		t.Fatal(err)
	}

	callbackEntered := make(chan struct{})
	callbackRelease := make(chan struct{})
	callbackFinished := make(chan struct{})

	if err := lifecycle.SetOnExit(func(error) {
		close(callbackEntered)
		<-callbackRelease
		close(callbackFinished)
	}); err != nil {
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

	select {
	case <-callbackEntered:
	case <-time.After(3 * time.Second):
		t.Fatal("callback never started")
	}

	ctx, cancel := context.WithTimeout(
		context.Background(),
		3*time.Second,
	)
	defer cancel()

	if err := lifecycle.Wait(ctx); err != nil {
		t.Fatalf("callback blocked lifecycle Wait: %v", err)
	}

	select {
	case <-callbackFinished:
		t.Fatal("callback unexpectedly finished before release")
	default:
	}

	close(callbackRelease)

	select {
	case <-callbackFinished:
	case <-time.After(3 * time.Second):
		t.Fatal("callback did not finish after release")
	}
}

func TestDiscoveryLifecycleConcurrentRegistrationAndStart(
	t *testing.T,
) {
	runner := newLifecycleTestRunner()

	lifecycle, err := NewDiscoveryLifecycle(runner)
	if err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	wg.Add(2)

	startResults := make(chan error, 1)
	registrationResults := make(chan error, 1)

	go func() {
		defer wg.Done()
		startResults <- lifecycle.Start(
			context.Background(),
			lifecycleTestOptions(),
		)
	}()

	go func() {
		defer wg.Done()
		registrationResults <- lifecycle.SetOnExit(
			func(error) {},
		)
	}()

	wg.Wait()

	if err := <-startResults; err != nil {
		t.Fatalf("concurrent Start failed: %v", err)
	}

	// Registration may win or lose the mutex race.
	// Both outcomes are valid, but the race detector
	// must confirm synchronization safety.
	_ = <-registrationResults

	awaitLifecycleStart(t, runner)
	close(runner.release)

	if err := lifecycle.StopAndWait(
		3 * time.Second,
	); err != nil {
		t.Fatal(err)
	}
}

func TestDiscoveryLifecycleNilExitRegistration(t *testing.T) {
	var lifecycle *DiscoveryLifecycle

	if err := lifecycle.SetOnExit(
		func(error) {},
	); err == nil {
		t.Fatal("nil lifecycle accepted callback")
	}

	runner := newLifecycleTestRunner()

	configured, err := NewDiscoveryLifecycle(runner)
	if err != nil {
		t.Fatal(err)
	}

	if err := configured.SetOnExit(nil); err != nil {
		t.Fatalf("nil optional callback rejected: %v", err)
	}

	if err := configured.Start(
		context.Background(),
		lifecycleTestOptions(),
	); err != nil {
		t.Fatal(err)
	}

	awaitLifecycleStart(t, runner)
	close(runner.release)

	if err := configured.StopAndWait(
		3 * time.Second,
	); err != nil {
		t.Fatal(err)
	}
}
