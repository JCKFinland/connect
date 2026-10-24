package main

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	postgresrepo "github.com/JCKFinland/connect/backend/internal/repository/postgres"
	flexauthorization "github.com/JCKFinland/connect/backend/internal/services/flex_authorization"
)

type activationRecoveryDiscovery struct {
	calls atomic.Int32
}

func (d *activationRecoveryDiscovery) Discover(
	ctx context.Context,
	staleAfter time.Duration,
	batchSize int,
) ([]postgresrepo.FlexAuthorizationRecoveryCandidate, error) {
	d.calls.Add(1)

	return []postgresrepo.FlexAuthorizationRecoveryCandidate{
		{
			AuthorizationID: "activation-fixture-authorization",
		},
	}, nil
}

type activationRecoveryExecutor struct {
	calls atomic.Int32
	done  chan struct{}
}

func (e *activationRecoveryExecutor) RecoverAndReconcile(
	ctx context.Context,
	authorizationID string,
) (*flexauthorization.RecoveryReconcileResult, error) {
	if authorizationID != "activation-fixture-authorization" {
		return nil, context.Canceled
	}

	if e.calls.Add(1) == 1 {
		close(e.done)
	}

	return &flexauthorization.RecoveryReconcileResult{}, nil
}

func TestAuthorizationRecoveryControlledActivationIntegration(
	t *testing.T,
) {
	discovery := &activationRecoveryDiscovery{}

	executor := &activationRecoveryExecutor{
		done: make(chan struct{}),
	}

	worker, err := flexauthorization.NewAuthorizationRecoveryWorker(
		discovery,
		executor,
		5*time.Minute,
		25,
	)
	if err != nil {
		t.Fatal(err)
	}

	scheduler, err :=
		flexauthorization.NewAuthorizationRecoveryScheduler(worker)
	if err != nil {
		t.Fatal(err)
	}

	lifecycle, err :=
		flexauthorization.NewAuthorizationRecoveryLifecycle(scheduler)
	if err != nil {
		t.Fatal(err)
	}

	controller, err := newFlexAuthorizationRecoveryController(
		enabledAuthorizationRecoveryControllerConfig(),
		lifecycle,
	)
	if err != nil {
		t.Fatal(err)
	}

	if err := controller.Start(
		context.Background(),
		flexauthorization.AuthorizationRecoverySchedulerOptions{
			Interval: 5 * time.Millisecond,
		},
		nil,
	); err != nil {
		t.Fatal(err)
	}

	select {
	case <-executor.done:
	case <-time.After(3 * time.Second):
		t.Fatal("enabled recovery did not execute a candidate")
	}

	shutdownCtx, cancel := context.WithTimeout(
		context.Background(),
		3*time.Second,
	)
	defer cancel()

	if err := controller.StopAndWait(shutdownCtx); err != nil {
		t.Fatal(err)
	}

	if got := executor.calls.Load(); got != 1 {
		t.Fatalf("expected one recovery execution, got %d", got)
	}

	if discovery.calls.Load() == 0 {
		t.Fatal("recovery discovery never executed")
	}

	if err := controller.Start(
		context.Background(),
		flexauthorization.AuthorizationRecoverySchedulerOptions{
			Interval: time.Millisecond,
		},
		nil,
	); err == nil {
		t.Fatal("controller restarted after shutdown")
	}
}
