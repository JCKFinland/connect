package main

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/JCKFinland/connect/backend/internal/config"
	flexauthorization "github.com/JCKFinland/connect/backend/internal/services/flex_authorization"
)

type controlledAuthorizationRecoveryLifecycle struct {
	mu sync.Mutex

	started  int
	stopped  int
	callback func(error)

	done chan struct{}
	once sync.Once
}

func newControlledAuthorizationRecoveryLifecycle() *controlledAuthorizationRecoveryLifecycle {
	return &controlledAuthorizationRecoveryLifecycle{
		done: make(chan struct{}),
	}
}

func (f *controlledAuthorizationRecoveryLifecycle) SetOnExit(
	callback func(error),
) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.callback = callback
	return nil
}

func (f *controlledAuthorizationRecoveryLifecycle) Start(
	ctx context.Context,
	options flexauthorization.AuthorizationRecoverySchedulerOptions,
) error {
	if ctx == nil || options.Interval <= 0 {
		return errors.New("invalid recovery startup")
	}

	f.mu.Lock()
	f.started++
	f.mu.Unlock()

	return nil
}

func (f *controlledAuthorizationRecoveryLifecycle) Stop() {
	f.mu.Lock()
	f.stopped++
	f.mu.Unlock()
}

func (f *controlledAuthorizationRecoveryLifecycle) Wait(
	ctx context.Context,
) error {
	select {
	case <-f.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (f *controlledAuthorizationRecoveryLifecycle) finish(err error) {
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

func enabledAuthorizationRecoveryControllerConfig() *config.Config {
	return &config.Config{
		App: config.AppConfig{
			Env: "test",
		},
		Database: config.DatabaseConfig{
			Name: "connect_settlement_test",
		},
		Stripe: config.StripeConfig{
			SecretKey: "sk_test_configuration_only",
		},
		FlexAuthorizationRecoveryWorker: config.FlexAuthorizationRecoveryWorkerConfig{
			EnabledRaw: "true",
		},
	}
}

func recoveryControllerOptions() flexauthorization.AuthorizationRecoverySchedulerOptions {
	return flexauthorization.AuthorizationRecoverySchedulerOptions{
		Interval: time.Minute,
	}
}

func TestAuthorizationRecoveryControllerDisabledDoesNotStart(t *testing.T) {
	lifecycle := newControlledAuthorizationRecoveryLifecycle()

	controller, err := newFlexAuthorizationRecoveryController(
		&config.Config{},
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

	lifecycle.mu.Lock()
	started := lifecycle.started
	lifecycle.mu.Unlock()

	if started != 0 {
		t.Fatal("disabled recovery worker started")
	}

	if err := controller.StopAndWait(
		context.Background(),
	); err != nil {
		t.Fatal(err)
	}
}

func TestAuthorizationRecoveryControllerEnabledRequiresLifecycle(t *testing.T) {
	_, err := newFlexAuthorizationRecoveryController(
		enabledAuthorizationRecoveryControllerConfig(),
		nil,
	)

	if err == nil {
		t.Fatal("enabled recovery accepted missing lifecycle")
	}
}

func TestAuthorizationRecoveryControllerStartAndShutdown(t *testing.T) {
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

	lifecycle.finish(nil)

	ctx, cancel := context.WithTimeout(
		context.Background(),
		time.Second,
	)
	defer cancel()

	if err := controller.StopAndWait(ctx); err != nil {
		t.Fatal(err)
	}

	lifecycle.mu.Lock()
	started := lifecycle.started
	stopped := lifecycle.stopped
	lifecycle.mu.Unlock()

	if started != 1 || stopped == 0 {
		t.Fatalf(
			"unexpected lifecycle calls: started=%d stopped=%d",
			started,
			stopped,
		)
	}
}

func TestAuthorizationRecoveryControllerRejectsDoubleStart(t *testing.T) {
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

	if err := controller.Start(
		context.Background(),
		recoveryControllerOptions(),
		nil,
	); err == nil {
		t.Fatal("double start accepted")
	}

	lifecycle.finish(nil)

	if err := controller.StopAndWait(
		context.Background(),
	); err != nil {
		t.Fatal(err)
	}
}

func TestAuthorizationRecoveryControllerShutdownTimeout(t *testing.T) {
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

	if stopped == 0 {
		t.Fatal("shutdown did not request cancellation")
	}

	lifecycle.finish(nil)

	if err := controller.StopAndWait(
		context.Background(),
	); err != nil {
		t.Fatal(err)
	}
}

func TestAuthorizationRecoveryControllerReportsUnexpectedExit(t *testing.T) {
	lifecycle := newControlledAuthorizationRecoveryLifecycle()

	controller, err := newFlexAuthorizationRecoveryController(
		enabledAuthorizationRecoveryControllerConfig(),
		lifecycle,
	)
	if err != nil {
		t.Fatal(err)
	}

	unexpected := make(chan error, 1)

	if err := controller.Start(
		context.Background(),
		recoveryControllerOptions(),
		func(err error) {
			unexpected <- err
		},
	); err != nil {
		t.Fatal(err)
	}

	failure := errors.New("simulated recovery failure")
	lifecycle.finish(failure)

	select {
	case received := <-unexpected:
		if !errors.Is(received, failure) {
			t.Fatalf("unexpected supervision error: %v", received)
		}
	case <-time.After(time.Second):
		t.Fatal("unexpected exit not reported")
	}

	if err := controller.StopAndWait(
		context.Background(),
	); err != nil {
		t.Fatal(err)
	}
}

func TestAuthorizationRecoveryControllerReportsUnexpectedCleanExit(t *testing.T) {
	lifecycle := newControlledAuthorizationRecoveryLifecycle()

	controller, err := newFlexAuthorizationRecoveryController(
		enabledAuthorizationRecoveryControllerConfig(),
		lifecycle,
	)
	if err != nil {
		t.Fatal(err)
	}

	unexpected := make(chan error, 1)

	if err := controller.Start(
		context.Background(),
		recoveryControllerOptions(),
		func(err error) {
			unexpected <- err
		},
	); err != nil {
		t.Fatal(err)
	}

	lifecycle.finish(nil)

	select {
	case received := <-unexpected:
		if received == nil {
			t.Fatal("unexpected clean exit was not reported")
		}
	case <-time.After(time.Second):
		t.Fatal("unexpected clean exit was not reported")
	}

	if err := controller.StopAndWait(
		context.Background(),
	); err != nil {
		t.Fatal(err)
	}
}

func TestAuthorizationRecoveryControllerSuppressesExpectedExit(t *testing.T) {
	lifecycle := newControlledAuthorizationRecoveryLifecycle()

	controller, err := newFlexAuthorizationRecoveryController(
		enabledAuthorizationRecoveryControllerConfig(),
		lifecycle,
	)
	if err != nil {
		t.Fatal(err)
	}

	unexpected := make(chan error, 1)

	if err := controller.Start(
		context.Background(),
		recoveryControllerOptions(),
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
		t.Fatalf("expected pending shutdown, got %v", err)
	}

	lifecycle.finish(nil)

	select {
	case err := <-unexpected:
		t.Fatalf("expected shutdown reported as failure: %v", err)
	default:
	}

	if err := controller.StopAndWait(
		context.Background(),
	); err != nil {
		t.Fatal(err)
	}
}

func TestAuthorizationRecoveryControllerRejectsStartAfterStop(t *testing.T) {
	lifecycle := newControlledAuthorizationRecoveryLifecycle()

	controller, err := newFlexAuthorizationRecoveryController(
		enabledAuthorizationRecoveryControllerConfig(),
		lifecycle,
	)
	if err != nil {
		t.Fatal(err)
	}

	if err := controller.StopAndWait(
		context.Background(),
	); err != nil {
		t.Fatal(err)
	}

	err = controller.Start(
		context.Background(),
		recoveryControllerOptions(),
		nil,
	)

	if err == nil || !strings.Contains(err.Error(), "stopping") {
		t.Fatalf("expected startup rejection, got %v", err)
	}
}

func TestAuthorizationRecoveryControllerConcurrentShutdown(t *testing.T) {
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

func TestAuthorizationRecoveryControllerConcurrentStartAndStop(t *testing.T) {
	const iterations = 100

	for i := 0; i < iterations; i++ {
		lifecycle := newControlledAuthorizationRecoveryLifecycle()

		controller, err := newFlexAuthorizationRecoveryController(
			enabledAuthorizationRecoveryControllerConfig(),
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
				recoveryControllerOptions(),
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
	}
}
