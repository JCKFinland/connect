package flex_authorization

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

type authorizationLifecycleRunnerFake struct {
	run func(context.Context, AuthorizationRecoverySchedulerOptions) error
}

func (f authorizationLifecycleRunnerFake) Run(
	ctx context.Context,
	options AuthorizationRecoverySchedulerOptions,
) error {
	return f.run(ctx, options)
}

func authorizationLifecycleOptions() AuthorizationRecoverySchedulerOptions {
	return AuthorizationRecoverySchedulerOptions{
		Interval: time.Millisecond,
	}
}

func TestAuthorizationRecoveryLifecycleRejectsInvalidConfiguration(
	t *testing.T,
) {
	if _, err := NewAuthorizationRecoveryLifecycle(nil); err == nil {
		t.Fatal("expected missing scheduler rejection")
	}

	runner := authorizationLifecycleRunnerFake{
		run: func(context.Context, AuthorizationRecoverySchedulerOptions) error {
			return nil
		},
	}

	lifecycle, err := NewAuthorizationRecoveryLifecycle(runner)
	if err != nil {
		t.Fatal(err)
	}

	if err := lifecycle.Start(nil, authorizationLifecycleOptions()); err == nil {
		t.Fatal("expected nil context rejection")
	}

	if err := lifecycle.Start(
		context.Background(),
		AuthorizationRecoverySchedulerOptions{},
	); err == nil {
		t.Fatal("expected invalid interval rejection")
	}

	if err := lifecycle.Wait(nil); err == nil {
		t.Fatal("expected nil wait context rejection")
	}

	if err := lifecycle.Wait(context.Background()); err == nil {
		t.Fatal("expected wait-before-start rejection")
	}

	if err := lifecycle.StopAndWait(0); err == nil {
		t.Fatal("expected invalid shutdown timeout rejection")
	}
}

func TestAuthorizationRecoveryLifecycleRejectsDuplicateStart(
	t *testing.T,
) {
	started := make(chan struct{})

	runner := authorizationLifecycleRunnerFake{
		run: func(
			ctx context.Context,
			options AuthorizationRecoverySchedulerOptions,
		) error {
			close(started)
			<-ctx.Done()
			return nil
		},
	}

	lifecycle, err := NewAuthorizationRecoveryLifecycle(runner)
	if err != nil {
		t.Fatal(err)
	}

	if err := lifecycle.Start(
		context.Background(),
		authorizationLifecycleOptions(),
	); err != nil {
		t.Fatal(err)
	}

	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("scheduler did not start")
	}

	if err := lifecycle.Start(
		context.Background(),
		authorizationLifecycleOptions(),
	); err == nil {
		t.Fatal("duplicate start was accepted")
	}

	if err := lifecycle.SetOnExit(nil); err == nil {
		t.Fatal("late callback registration was accepted")
	}

	lifecycle.Stop()
	lifecycle.Stop()

	ctx, cancel := context.WithTimeout(
		context.Background(),
		3*time.Second,
	)
	defer cancel()

	if err := lifecycle.Wait(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestAuthorizationRecoveryLifecycleTimeoutDoesNotClaimCompletion(
	t *testing.T,
) {
	started := make(chan struct{})
	release := make(chan struct{})

	runner := authorizationLifecycleRunnerFake{
		run: func(
			ctx context.Context,
			options AuthorizationRecoverySchedulerOptions,
		) error {
			close(started)
			<-release
			return nil
		},
	}

	lifecycle, err := NewAuthorizationRecoveryLifecycle(runner)
	if err != nil {
		t.Fatal(err)
	}

	if err := lifecycle.Start(
		context.Background(),
		authorizationLifecycleOptions(),
	); err != nil {
		t.Fatal(err)
	}

	select {
	case <-started:
	case <-time.After(3 * time.Second):
		close(release)
		t.Fatal("scheduler did not start")
	}

	err = lifecycle.StopAndWait(20 * time.Millisecond)

	if !errors.Is(err, context.DeadlineExceeded) {
		close(release)
		t.Fatalf("expected shutdown timeout, got %v", err)
	}

	close(release)

	ctx, cancel := context.WithTimeout(
		context.Background(),
		3*time.Second,
	)
	defer cancel()

	if err := lifecycle.Wait(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestAuthorizationRecoveryLifecycleConcurrentWaitersObserveExit(
	t *testing.T,
) {
	started := make(chan struct{})
	release := make(chan struct{})

	runner := authorizationLifecycleRunnerFake{
		run: func(
			ctx context.Context,
			options AuthorizationRecoverySchedulerOptions,
		) error {
			close(started)
			<-ctx.Done()
			<-release
			return nil
		},
	}

	lifecycle, err := NewAuthorizationRecoveryLifecycle(runner)
	if err != nil {
		t.Fatal(err)
	}

	if err := lifecycle.Start(
		context.Background(),
		authorizationLifecycleOptions(),
	); err != nil {
		t.Fatal(err)
	}

	select {
	case <-started:
	case <-time.After(3 * time.Second):
		close(release)
		t.Fatal("scheduler did not start")
	}

	lifecycle.Stop()

	const waiters = 5
	results := make(chan error, waiters)

	var group sync.WaitGroup

	for i := 0; i < waiters; i++ {
		group.Add(1)

		go func() {
			defer group.Done()

			ctx, cancel := context.WithTimeout(
				context.Background(),
				3*time.Second,
			)
			defer cancel()

			results <- lifecycle.Wait(ctx)
		}()
	}

	close(release)
	group.Wait()
	close(results)

	for err := range results {
		if err != nil {
			t.Fatalf("concurrent waiter failed: %v", err)
		}
	}
}

func TestAuthorizationRecoveryLifecyclePropagatesUnexpectedExit(
	t *testing.T,
) {
	expected := errors.New("scheduler unexpectedly exited")

	runner := authorizationLifecycleRunnerFake{
		run: func(
			context.Context,
			AuthorizationRecoverySchedulerOptions,
		) error {
			return expected
		},
	}

	lifecycle, err := NewAuthorizationRecoveryLifecycle(runner)
	if err != nil {
		t.Fatal(err)
	}

	callbackResult := make(chan error, 1)

	if err := lifecycle.SetOnExit(func(err error) {
		callbackResult <- err
	}); err != nil {
		t.Fatal(err)
	}

	if err := lifecycle.Start(
		context.Background(),
		authorizationLifecycleOptions(),
	); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(
		context.Background(),
		3*time.Second,
	)
	defer cancel()

	err = lifecycle.Wait(ctx)

	if !errors.Is(err, expected) {
		t.Fatalf("unexpected worker exit result: %v", err)
	}

	select {
	case callbackErr := <-callbackResult:
		if !errors.Is(callbackErr, expected) {
			t.Fatalf("unexpected callback error: %v", callbackErr)
		}
	case <-ctx.Done():
		t.Fatal("exit callback was not invoked")
	}

	if !strings.Contains(err.Error(), "scheduler exited") {
		t.Fatalf("missing exit context: %v", err)
	}
}

func TestAuthorizationRecoveryLifecycleParentCancellation(
	t *testing.T,
) {
	parent, cancel := context.WithCancel(context.Background())
	defer cancel()

	started := make(chan struct{})

	runner := authorizationLifecycleRunnerFake{
		run: func(
			ctx context.Context,
			options AuthorizationRecoverySchedulerOptions,
		) error {
			close(started)
			<-ctx.Done()
			return nil
		},
	}

	lifecycle, err := NewAuthorizationRecoveryLifecycle(runner)
	if err != nil {
		t.Fatal(err)
	}

	if err := lifecycle.Start(
		parent,
		authorizationLifecycleOptions(),
	); err != nil {
		t.Fatal(err)
	}

	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("scheduler did not start")
	}

	cancel()

	ctx, waitCancel := context.WithTimeout(
		context.Background(),
		3*time.Second,
	)
	defer waitCancel()

	if err := lifecycle.Wait(ctx); err != nil {
		t.Fatal(err)
	}
}
