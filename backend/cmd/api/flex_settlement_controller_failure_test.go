package main

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/JCKFinland/connect/backend/internal/config"
	flexsettlement "github.com/JCKFinland/connect/backend/internal/services/flex_settlement"
)

func enabledFlexControllerConfig() *config.Config {
	return &config.Config{
		FlexSettlementWorker: config.FlexSettlementWorkerConfig{
			EnabledRaw: "true",
		},
		Stripe: config.StripeConfig{
			SecretKey: "sk_test_configuration_only",
		},
	}
}

// controlledFlexLifecycle simulates a worker that may remain
// in flight after cancellation. Stop does not imply completion.
type controlledFlexLifecycle struct {
	mu sync.Mutex

	callback func(error)
	started  bool
	stopped  bool

	done chan struct{}
	once sync.Once
}

func newControlledFlexLifecycle() *controlledFlexLifecycle {
	return &controlledFlexLifecycle{
		done: make(chan struct{}),
	}
}

func (f *controlledFlexLifecycle) SetOnExit(
	callback func(error),
) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.callback = callback
	return nil
}

func (f *controlledFlexLifecycle) Start(
	ctx context.Context,
	options flexsettlement.DiscoveryWorkerOptions,
) error {
	if ctx == nil || options.Interval <= 0 {
		return errors.New("invalid worker startup")
	}

	f.mu.Lock()
	f.started = true
	f.mu.Unlock()

	return nil
}

func (f *controlledFlexLifecycle) Stop() {
	f.mu.Lock()
	f.stopped = true
	f.mu.Unlock()
}

// finish represents actual worker completion.
func (f *controlledFlexLifecycle) finish(err error) {
	f.once.Do(func() {
		f.mu.Lock()
		callback := f.callback
		f.mu.Unlock()

		close(f.done)

		if callback != nil {
			callback(err)
		}
	})
}

func (f *controlledFlexLifecycle) Wait(
	ctx context.Context,
) error {
	select {
	case <-f.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func TestFlexSettlementControllerShutdownTimeout(
	t *testing.T,
) {
	lifecycle := newControlledFlexLifecycle()

	controller, err := newFlexSettlementController(
		enabledFlexControllerConfig(),
		lifecycle,
	)
	if err != nil {
		t.Fatal(err)
	}

	if err := controller.Start(
		context.Background(),
		flexSettlementWorkerOptions(nil),
		nil,
	); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(
		context.Background(),
		20*time.Millisecond,
	)
	defer cancel()

	err = controller.StopAndWait(ctx)

	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected shutdown timeout, got %v", err)
	}

	lifecycle.mu.Lock()
	stopped := lifecycle.stopped
	lifecycle.mu.Unlock()

	if !stopped {
		t.Fatal("shutdown did not request worker cancellation")
	}

	// Complete simulated in-flight processing after timeout.
	lifecycle.finish(nil)

	if err := controller.StopAndWait(
		context.Background(),
	); err != nil {
		t.Fatalf("second shutdown wait failed: %v", err)
	}
}

func TestFlexSettlementControllerReportsUnexpectedExit(
	t *testing.T,
) {
	lifecycle := newControlledFlexLifecycle()

	controller, err := newFlexSettlementController(
		enabledFlexControllerConfig(),
		lifecycle,
	)
	if err != nil {
		t.Fatal(err)
	}

	unexpected := make(chan error, 1)

	if err := controller.Start(
		context.Background(),
		flexSettlementWorkerOptions(nil),
		func(err error) {
			unexpected <- err
		},
	); err != nil {
		t.Fatal(err)
	}

	failure := errors.New("simulated worker failure")
	lifecycle.finish(failure)

	select {
	case received := <-unexpected:
		if !errors.Is(received, failure) {
			t.Fatalf("unexpected supervision error: %v", received)
		}
	case <-time.After(time.Second):
		t.Fatal("unexpected worker exit was not reported")
	}
}

func TestFlexSettlementControllerSuppressesExpectedExit(
	t *testing.T,
) {
	lifecycle := newControlledFlexLifecycle()

	controller, err := newFlexSettlementController(
		enabledFlexControllerConfig(),
		lifecycle,
	)
	if err != nil {
		t.Fatal(err)
	}

	unexpected := make(chan error, 1)

	if err := controller.Start(
		context.Background(),
		flexSettlementWorkerOptions(nil),
		func(err error) {
			unexpected <- err
		},
	); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(
		context.Background(),
		20*time.Millisecond,
	)
	defer cancel()

	if err := controller.StopAndWait(ctx); !errors.Is(
		err,
		context.DeadlineExceeded,
	) {
		t.Fatalf("expected pending worker shutdown, got %v", err)
	}

	lifecycle.finish(nil)

	select {
	case err := <-unexpected:
		t.Fatalf("expected shutdown reported as failure: %v", err)
	default:
	}
}

func TestFlexSettlementControllerConcurrentShutdown(
	t *testing.T,
) {
	lifecycle := newControlledFlexLifecycle()

	controller, err := newFlexSettlementController(
		enabledFlexControllerConfig(),
		lifecycle,
	)
	if err != nil {
		t.Fatal(err)
	}

	if err := controller.Start(
		context.Background(),
		flexSettlementWorkerOptions(nil),
		nil,
	); err != nil {
		t.Fatal(err)
	}

	const callers = 12

	var wg sync.WaitGroup
	results := make(chan error, callers)

	for i := 0; i < callers; i++ {
		wg.Add(1)

		go func() {
			defer wg.Done()

			ctx, cancel := context.WithTimeout(
				context.Background(),
				time.Second,
			)
			defer cancel()

			results <- controller.StopAndWait(ctx)
		}()
	}

	lifecycle.finish(nil)

	wg.Wait()
	close(results)

	for err := range results {
		if err != nil {
			t.Fatalf("concurrent shutdown failed: %v", err)
		}
	}
}
