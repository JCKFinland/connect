package main

import (
	"context"
	"errors"
	"testing"
	"time"
)

func newStartedRecoveryControllerForShutdownTest(
	t *testing.T,
) (
	*flexAuthorizationRecoveryController,
	*controlledAuthorizationRecoveryLifecycle,
) {
	t.Helper()

	lifecycle := newControlledAuthorizationRecoveryLifecycle()

	controller, err := newFlexAuthorizationRecoveryController(
		enabledAuthorizationRecoveryControllerConfig(),
		lifecycle,
	)
	if err != nil {
		t.Fatal(err)
	}

	if err := controller.Start(
		context.Background(),
		recoveryControllerOptions(),
		nil,
	); err != nil {
		t.Fatal(err)
	}

	return controller, lifecycle
}

func TestAuthorizationRecoveryShutdownBarrierNormalCompletion(
	t *testing.T,
) {
	controller, lifecycle :=
		newStartedRecoveryControllerForShutdownTest(t)

	lifecycle.finish(nil)

	if err := waitForFlexShutdown(
		controller,
		time.Second,
	); err != nil {
		t.Fatalf("shutdown barrier failed: %v", err)
	}

	lifecycle.mu.Lock()
	stopped := lifecycle.stopped
	lifecycle.mu.Unlock()

	if stopped == 0 {
		t.Fatal("shutdown barrier did not stop recovery")
	}
}

func TestAuthorizationRecoveryShutdownBarrierWaitsBeyondDeadline(
	t *testing.T,
) {
	controller, lifecycle :=
		newStartedRecoveryControllerForShutdownTest(t)

	result := make(chan error, 1)

	go func() {
		result <- waitForFlexShutdown(
			controller,
			20*time.Millisecond,
		)
	}()

	// The initial timeout must not release the shutdown barrier.
	select {
	case err := <-result:
		t.Fatalf(
			"shutdown returned before worker completion: %v",
			err,
		)
	case <-time.After(80 * time.Millisecond):
	}

	lifecycle.mu.Lock()
	stopped := lifecycle.stopped
	lifecycle.mu.Unlock()

	if stopped == 0 {
		t.Fatal("shutdown did not request cancellation")
	}

	// Only actual worker completion may release the barrier.
	lifecycle.finish(nil)

	select {
	case err := <-result:
		if err != nil {
			t.Fatalf(
				"shutdown failed after completion: %v",
				err,
			)
		}
	case <-time.After(time.Second):
		t.Fatal("shutdown did not finish after worker completion")
	}
}

func TestAuthorizationRecoveryShutdownBarrierRejectsInvalidTimeout(
	t *testing.T,
) {
	controller, lifecycle :=
		newStartedRecoveryControllerForShutdownTest(t)

	if err := waitForFlexShutdown(
		controller,
		0,
	); err == nil {
		t.Fatal("invalid shutdown timeout accepted")
	}

	lifecycle.finish(nil)

	if err := waitForFlexShutdown(
		controller,
		time.Second,
	); err != nil {
		t.Fatal(err)
	}
}

func TestAuthorizationRecoveryShutdownBarrierPreservesLifecycleError(
	t *testing.T,
) {
	controller, lifecycle :=
		newStartedRecoveryControllerForShutdownTest(t)

	failure := errors.New("simulated recovery lifecycle failure")

	// The controlled fake represents completion independently
	// of cancellation, as required by the shutdown contract.
	lifecycle.finish(failure)

	if err := waitForFlexShutdown(
		controller,
		time.Second,
	); err != nil {
		t.Fatalf(
			"shutdown barrier failed after completion: %v",
			err,
		)
	}
}
