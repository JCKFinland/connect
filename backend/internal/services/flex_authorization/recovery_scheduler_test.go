package flex_authorization

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type authorizationSchedulerRunnerFake struct {
	run func(context.Context, func(error)) error
}

func (f authorizationSchedulerRunnerFake) RunOnce(
	ctx context.Context,
	onError func(error),
) error {
	return f.run(ctx, onError)
}

func TestAuthorizationRecoverySchedulerRejectsInvalidConfiguration(
	t *testing.T,
) {
	if _, err := NewAuthorizationRecoveryScheduler(nil); err == nil {
		t.Fatal("expected missing worker rejection")
	}

	worker := authorizationSchedulerRunnerFake{
		run: func(context.Context, func(error)) error {
			return nil
		},
	}

	scheduler, err := NewAuthorizationRecoveryScheduler(worker)
	if err != nil {
		t.Fatal(err)
	}

	if err := scheduler.Run(nil, AuthorizationRecoverySchedulerOptions{
		Interval: time.Millisecond,
	}); err == nil {
		t.Fatal("expected nil context rejection")
	}

	if err := scheduler.Run(
		context.Background(),
		AuthorizationRecoverySchedulerOptions{},
	); err == nil {
		t.Fatal("expected invalid interval rejection")
	}
}

func TestAuthorizationRecoverySchedulerRunsPeriodically(
	t *testing.T,
) {
	ctx, cancel := context.WithTimeout(
		context.Background(),
		3*time.Second,
	)
	defer cancel()

	var calls atomic.Int32

	worker := authorizationSchedulerRunnerFake{
		run: func(
			ctx context.Context,
			onError func(error),
		) error {
			if calls.Add(1) == 3 {
				cancel()
			}

			return nil
		},
	}

	scheduler, err := NewAuthorizationRecoveryScheduler(worker)
	if err != nil {
		t.Fatal(err)
	}

	err = scheduler.Run(ctx, AuthorizationRecoverySchedulerOptions{
		Interval: time.Millisecond,
	})

	if err != nil {
		t.Fatal(err)
	}

	if got := calls.Load(); got != 3 {
		t.Fatalf("expected three cycles, got %d", got)
	}
}

func TestAuthorizationRecoverySchedulerReportsErrorsAndContinues(
	t *testing.T,
) {
	ctx, cancel := context.WithTimeout(
		context.Background(),
		3*time.Second,
	)
	defer cancel()

	var calls int
	var reported []error

	worker := authorizationSchedulerRunnerFake{
		run: func(
			ctx context.Context,
			onError func(error),
		) error {
			calls++

			if calls == 1 {
				return errors.New("discovery failed")
			}

			cancel()
			return nil
		},
	}

	scheduler, err := NewAuthorizationRecoveryScheduler(worker)
	if err != nil {
		t.Fatal(err)
	}

	err = scheduler.Run(ctx, AuthorizationRecoverySchedulerOptions{
		Interval: time.Millisecond,
		OnError: func(err error) {
			reported = append(reported, err)
		},
	})

	if err != nil {
		t.Fatal(err)
	}

	if calls != 2 {
		t.Fatalf("expected two cycles, got %d", calls)
	}

	if len(reported) != 1 ||
		!strings.Contains(reported[0].Error(), "discovery failed") {
		t.Fatalf("unexpected reported errors: %v", reported)
	}
}

func TestAuthorizationRecoverySchedulerStopsOnCancellation(
	t *testing.T,
) {
	ctx, cancel := context.WithCancel(context.Background())

	started := make(chan struct{})
	exited := make(chan error, 1)

	worker := authorizationSchedulerRunnerFake{
		run: func(
			ctx context.Context,
			onError func(error),
		) error {
			close(started)
			<-ctx.Done()
			return ctx.Err()
		},
	}

	scheduler, err := NewAuthorizationRecoveryScheduler(worker)
	if err != nil {
		t.Fatal(err)
	}

	go func() {
		exited <- scheduler.Run(
			ctx,
			AuthorizationRecoverySchedulerOptions{
				Interval: time.Millisecond,
			},
		)
	}()

	select {
	case <-started:
	case <-time.After(3 * time.Second):
		cancel()
		t.Fatal("scheduler did not begin a recovery cycle")
	}

	cancel()

	select {
	case err := <-exited:
		if err != nil {
			t.Fatalf("unexpected shutdown error: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("scheduler did not exit after cancellation")
	}
}

func TestAuthorizationRecoverySchedulerDoesNotOverlapCycles(
	t *testing.T,
) {
	ctx, cancel := context.WithTimeout(
		context.Background(),
		3*time.Second,
	)
	defer cancel()

	var active atomic.Int32
	var maximum atomic.Int32
	var calls atomic.Int32

	worker := authorizationSchedulerRunnerFake{
		run: func(
			ctx context.Context,
			onError func(error),
		) error {
			current := active.Add(1)
			defer active.Add(-1)

			for {
				previous := maximum.Load()
				if current <= previous ||
					maximum.CompareAndSwap(previous, current) {
					break
				}
			}

			time.Sleep(10 * time.Millisecond)

			if calls.Add(1) == 3 {
				cancel()
			}

			return nil
		},
	}

	scheduler, err := NewAuthorizationRecoveryScheduler(worker)
	if err != nil {
		t.Fatal(err)
	}

	err = scheduler.Run(ctx, AuthorizationRecoverySchedulerOptions{
		Interval: time.Millisecond,
	})

	if err != nil {
		t.Fatal(err)
	}

	if maximum.Load() != 1 {
		t.Fatalf(
			"expected serial cycles, maximum concurrent=%d",
			maximum.Load(),
		)
	}

	if calls.Load() != 3 {
		t.Fatalf("unexpected cycle count: %d", calls.Load())
	}
}

func TestAuthorizationRecoverySchedulerForwardsCandidateErrors(
	t *testing.T,
) {
	ctx, cancel := context.WithTimeout(
		context.Background(),
		3*time.Second,
	)
	defer cancel()

	var mu sync.Mutex
	var reported []error

	worker := authorizationSchedulerRunnerFake{
		run: func(
			ctx context.Context,
			onError func(error),
		) error {
			if onError != nil {
				onError(errors.New("candidate recovery failed"))
			}

			cancel()
			return nil
		},
	}

	scheduler, err := NewAuthorizationRecoveryScheduler(worker)
	if err != nil {
		t.Fatal(err)
	}

	err = scheduler.Run(ctx, AuthorizationRecoverySchedulerOptions{
		Interval: time.Millisecond,
		OnError: func(err error) {
			mu.Lock()
			reported = append(reported, err)
			mu.Unlock()
		},
	})

	if err != nil {
		t.Fatal(err)
	}

	mu.Lock()
	defer mu.Unlock()

	if len(reported) != 1 ||
		!strings.Contains(
			reported[0].Error(),
			"candidate recovery failed",
		) {
		t.Fatalf("unexpected candidate errors: %v", reported)
	}
}
