package driver_payout

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	postgresrepo "github.com/JCKFinland/connect/backend/internal/repository/postgres"
)

type payoutRecoveryFake struct {
	mu         sync.Mutex
	candidates []postgresrepo.StaleDriverPayoutExecution
	lookups    int
	active     int
	maxActive  int
	entered    chan struct{}
	release    chan struct{}
}

func (f *payoutRecoveryFake) DiscoverCandidates(
	ctx context.Context,
	_ time.Time,
	_ int,
) ([]postgresrepo.StaleDriverPayoutExecution, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	return append(
		[]postgresrepo.StaleDriverPayoutExecution(nil),
		f.candidates...,
	), nil
}

func (f *payoutRecoveryFake) InspectCandidate(
	ctx context.Context,
	candidate postgresrepo.StaleDriverPayoutExecution,
) (RecoveryObservation, error) {
	f.mu.Lock()
	f.lookups++
	f.active++

	if f.active > f.maxActive {
		f.maxActive = f.active
	}

	entered := f.entered
	release := f.release
	f.mu.Unlock()

	defer func() {
		f.mu.Lock()
		f.active--
		f.mu.Unlock()
	}()

	if entered != nil {
		select {
		case entered <- struct{}{}:
		default:
		}
	}

	if release != nil {
		select {
		case <-release:
		case <-ctx.Done():
			return RecoveryObservation{}, ctx.Err()
		}
	}

	return RecoveryObservation{
		Execution:         candidate,
		LookupSucceeded:   true,
		ProviderReference: "synthetic-reference",
	}, nil
}

func recoveryCandidate(id string) postgresrepo.StaleDriverPayoutExecution {
	return postgresrepo.StaleDriverPayoutExecution{
		PayoutID:       id,
		IdempotencyKey: "key-" + id,
		Status:         "PROCESSING",
		AttemptCount:   1,
	}
}

func TestPayoutRecoveryWorkerCooldownBeforeLookup(t *testing.T) {
	fake := &payoutRecoveryFake{
		candidates: []postgresrepo.StaleDriverPayoutExecution{
			recoveryCandidate("payout-1"),
		},
	}

	worker, err := NewPayoutRecoveryWorker(
		fake,
		time.Minute,
		10,
	)
	if err != nil {
		t.Fatal(err)
	}

	now := time.Now().UTC()
	worker.now = func() time.Time { return now }

	if err := worker.RunOnce(context.Background(), nil); err != nil {
		t.Fatal(err)
	}

	if err := worker.RunOnce(context.Background(), nil); err != nil {
		t.Fatal(err)
	}

	fake.mu.Lock()
	count := fake.lookups
	fake.mu.Unlock()

	if count != 1 {
		t.Fatalf(
			"cooldown failed: expected 1 lookup, got %d",
			count,
		)
	}

	now = now.Add(defaultPayoutRecoveryCooldown)

	if err := worker.RunOnce(context.Background(), nil); err != nil {
		t.Fatal(err)
	}

	fake.mu.Lock()
	count = fake.lookups
	fake.mu.Unlock()

	if count != 2 {
		t.Fatalf(
			"lookup did not resume after cooldown: %d",
			count,
		)
	}
}

func TestPayoutRecoveryWorkerSerialExecution(t *testing.T) {
	fake := &payoutRecoveryFake{
		candidates: []postgresrepo.StaleDriverPayoutExecution{
			recoveryCandidate("payout-serial"),
		},
		entered: make(chan struct{}, 1),
		release: make(chan struct{}),
	}

	worker, err := NewPayoutRecoveryWorker(
		fake,
		time.Minute,
		10,
	)
	if err != nil {
		t.Fatal(err)
	}

	firstDone := make(chan error, 1)

	go func() {
		firstDone <- worker.RunOnce(
			context.Background(),
			nil,
		)
	}()

	select {
	case <-fake.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("first lookup did not begin")
	}

	secondCtx, cancel := context.WithTimeout(
		context.Background(),
		100*time.Millisecond,
	)
	defer cancel()

	secondErr := worker.RunOnce(secondCtx, nil)

	if !errors.Is(secondErr, context.DeadlineExceeded) {
		t.Fatalf(
			"expected waiting caller timeout, got %v",
			secondErr,
		)
	}

	close(fake.release)

	select {
	case err := <-firstDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("first worker did not finish")
	}

	fake.mu.Lock()
	maxActive := fake.maxActive
	lookups := fake.lookups
	fake.mu.Unlock()

	if maxActive != 1 || lookups != 1 {
		t.Fatalf(
			"unexpected concurrency: max=%d lookups=%d",
			maxActive,
			lookups,
		)
	}

	if err := worker.RunOnce(context.Background(), nil); err != nil {
		t.Fatalf(
			"worker gate remained locked: %v",
			err,
		)
	}
}

func TestPayoutRecoveryWorkerCancellation(t *testing.T) {
	fake := &payoutRecoveryFake{
		candidates: []postgresrepo.StaleDriverPayoutExecution{
			recoveryCandidate("payout-cancel"),
		},
		entered: make(chan struct{}, 1),
		release: make(chan struct{}),
	}

	worker, err := NewPayoutRecoveryWorker(
		fake,
		time.Minute,
		10,
	)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)

	go func() {
		done <- worker.RunOnce(ctx, nil)
	}()

	select {
	case <-fake.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("lookup did not start")
	}

	cancel()

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf(
				"expected cancellation, got %v",
				err,
			)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("worker did not stop on cancellation")
	}

	close(fake.release)

	if err := worker.RunOnce(context.Background(), nil); err != nil {
		t.Fatalf(
			"worker unusable after cancellation: %v",
			err,
		)
	}
}
