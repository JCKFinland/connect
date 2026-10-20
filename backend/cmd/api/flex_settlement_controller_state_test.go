package main

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestFlexSettlementControllerRejectsStartAfterStop(t *testing.T) {
	lifecycle := newControlledFlexLifecycle()

	controller, err := newFlexSettlementController(
		enabledFlexControllerConfig(),
		lifecycle,
	)
	if err != nil {
		t.Fatal(err)
	}

	if err := controller.StopAndWait(
		context.Background(),
	); err != nil {
		t.Fatalf("stop before start failed: %v", err)
	}

	err = controller.Start(
		context.Background(),
		flexSettlementWorkerOptions(nil),
		nil,
	)

	if err == nil ||
		!strings.Contains(err.Error(), "stopping") {
		t.Fatalf(
			"expected startup rejection after shutdown, got %v",
			err,
		)
	}

	lifecycle.mu.Lock()
	started := lifecycle.started
	lifecycle.mu.Unlock()

	if started {
		t.Fatal("worker started after controller shutdown")
	}
}

func TestFlexSettlementControllerConcurrentStartAndStop(t *testing.T) {
	const iterations = 100

	for i := 0; i < iterations; i++ {
		lifecycle := newControlledFlexLifecycle()

		controller, err := newFlexSettlementController(
			enabledFlexControllerConfig(),
			lifecycle,
		)
		if err != nil {
			t.Fatal(err)
		}

		var wg sync.WaitGroup
		wg.Add(2)

		startResults := make(chan error, 1)
		stopResults := make(chan error, 1)

		go func() {
			defer wg.Done()

			startResults <- controller.Start(
				context.Background(),
				flexSettlementWorkerOptions(nil),
				nil,
			)
		}()

		go func() {
			defer wg.Done()

			ctx, cancel := context.WithTimeout(
				context.Background(),
				time.Second,
			)
			defer cancel()

			stopResults <- controller.StopAndWait(ctx)
		}()

		// Completion is explicitly controlled by the fake
		// worker; it must not depend on cancellation timing.
		lifecycle.finish(nil)

		wg.Wait()

		startErr := <-startResults
		stopErr := <-stopResults

		if stopErr != nil {
			t.Fatalf(
				"iteration %d: shutdown failed: %v",
				i,
				stopErr,
			)
		}

		if startErr != nil &&
			!strings.Contains(startErr.Error(), "stopping") {
			t.Fatalf(
				"iteration %d: unexpected startup error: %v",
				i,
				startErr,
			)
		}

		controller.mu.Lock()
		stopping := controller.stopping
		controller.mu.Unlock()

		if !stopping {
			t.Fatalf(
				"iteration %d: controller did not enter stopping state",
				i,
			)
		}
	}
}
