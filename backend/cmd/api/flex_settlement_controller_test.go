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

type fakeFlexSettlementLifecycle struct {
	mu sync.Mutex

	starts int
	stops  int
	exits  func(error)

	done chan struct{}
	once sync.Once
}

func (f *fakeFlexSettlementLifecycle) SetOnExit(
	callback func(error),
) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.exits = callback
	return nil
}

func (f *fakeFlexSettlementLifecycle) Start(
	ctx context.Context,
	options flexsettlement.DiscoveryWorkerOptions,
) error {
	if options.Interval <= 0 {
		return errors.New("invalid interval")
	}

	f.mu.Lock()
	f.starts++
	f.done = make(chan struct{})
	f.mu.Unlock()

	go func() {
		<-ctx.Done()
		f.Stop()
	}()

	return nil
}

func (f *fakeFlexSettlementLifecycle) Stop() {
	f.mu.Lock()
	f.stops++
	done := f.done
	callback := f.exits
	f.mu.Unlock()

	f.once.Do(func() {
		if done != nil {
			close(done)
		}
		if callback != nil {
			callback(nil)
		}
	})
}

func (f *fakeFlexSettlementLifecycle) Wait(
	ctx context.Context,
) error {
	f.mu.Lock()
	done := f.done
	f.mu.Unlock()

	if done == nil {
		return errors.New("worker not started")
	}

	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func TestFlexSettlementControllerDisabledDoesNotStart(
	t *testing.T,
) {
	lifecycle := &fakeFlexSettlementLifecycle{}

	controller, err := newFlexSettlementController(
		&config.Config{},
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

	lifecycle.mu.Lock()
	starts := lifecycle.starts
	lifecycle.mu.Unlock()

	if starts != 0 {
		t.Fatal("disabled worker was started")
	}

	if err := controller.StopAndWait(
		context.Background(),
	); err != nil {
		t.Fatal(err)
	}
}

func TestFlexSettlementControllerEnabledRequiresLifecycle(
	t *testing.T,
) {
	cfg := &config.Config{
		FlexSettlementWorker: config.FlexSettlementWorkerConfig{
			EnabledRaw: "true",
		},
		Stripe: config.StripeConfig{
			SecretKey: "sk_test_configuration_only",
		},
	}

	if _, err := newFlexSettlementController(
		cfg,
		nil,
	); err == nil {
		t.Fatal("enabled controller accepted missing lifecycle")
	}
}

func TestFlexSettlementControllerStartAndShutdown(
	t *testing.T,
) {
	cfg := &config.Config{
		FlexSettlementWorker: config.FlexSettlementWorkerConfig{
			EnabledRaw: "true",
		},
		Stripe: config.StripeConfig{
			SecretKey: "sk_test_configuration_only",
		},
	}

	lifecycle := &fakeFlexSettlementLifecycle{}

	controller, err := newFlexSettlementController(
		cfg,
		lifecycle,
	)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := controller.Start(
		ctx,
		flexSettlementWorkerOptions(nil),
		nil,
	); err != nil {
		t.Fatal(err)
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(
		context.Background(),
		time.Second,
	)
	defer shutdownCancel()

	if err := controller.StopAndWait(shutdownCtx); err != nil {
		t.Fatal(err)
	}

	lifecycle.mu.Lock()
	starts := lifecycle.starts
	stops := lifecycle.stops
	lifecycle.mu.Unlock()

	if starts != 1 || stops == 0 {
		t.Fatalf(
			"unexpected lifecycle calls: starts=%d stops=%d",
			starts,
			stops,
		)
	}
}

func TestFlexSettlementControllerRejectsDoubleStart(
	t *testing.T,
) {
	cfg := &config.Config{
		FlexSettlementWorker: config.FlexSettlementWorkerConfig{
			EnabledRaw: "true",
		},
		Stripe: config.StripeConfig{
			SecretKey: "sk_test_configuration_only",
		},
	}

	lifecycle := &fakeFlexSettlementLifecycle{}

	controller, err := newFlexSettlementController(
		cfg,
		lifecycle,
	)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := controller.Start(
		ctx,
		flexSettlementWorkerOptions(nil),
		nil,
	); err != nil {
		t.Fatal(err)
	}

	if err := controller.Start(
		ctx,
		flexSettlementWorkerOptions(nil),
		nil,
	); err == nil {
		t.Fatal("double start accepted")
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(
		context.Background(),
		time.Second,
	)
	defer shutdownCancel()

	if err := controller.StopAndWait(shutdownCtx); err != nil {
		t.Fatal(err)
	}
}
