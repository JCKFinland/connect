package flex_settlement

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	postgresrepo "github.com/JCKFinland/connect/backend/internal/repository/postgres"
)

type blockingWorkerDiscovery struct {
	entered chan struct{}
	release chan struct{}
	once    sync.Once

	active    atomic.Int32
	maxActive atomic.Int32
	calls     atomic.Int32
}

func (f *blockingWorkerDiscovery) Next(
	ctx context.Context,
) ([]postgresrepo.FlexSettlementCandidate, error) {
	active := f.active.Add(1)
	defer f.active.Add(-1)

	for {
		previous := f.maxActive.Load()
		if active <= previous ||
			f.maxActive.CompareAndSwap(previous, active) {
			break
		}
	}

	call := f.calls.Add(1)

	if call == 1 {
		f.once.Do(func() {
			close(f.entered)
		})

		select {
		case <-f.release:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	return []postgresrepo.FlexSettlementCandidate{
		workerCandidate("settlement"),
	}, nil
}

type countingWorkerDispatch struct {
	calls atomic.Int32
}

func (f *countingWorkerDispatch) Dispatch(
	_ context.Context,
	candidate postgresrepo.FlexSettlementCandidate,
) (*RecoveryResult, error) {
	f.calls.Add(1)

	return &RecoveryResult{
		SettlementID: candidate.SettlementID,
		Status:       RecoveryPendingProvider,
	}, nil
}

func newBlockingWorker(
	t *testing.T,
) (*DiscoveryWorker, *blockingWorkerDiscovery, *countingWorkerDispatch) {
	t.Helper()

	discovery := &blockingWorkerDiscovery{
		entered: make(chan struct{}),
		release: make(chan struct{}),
	}

	dispatch := &countingWorkerDispatch{}

	worker, err := NewDiscoveryWorker(discovery, dispatch)
	if err != nil {
		t.Fatal(err)
	}

	return worker, discovery, dispatch
}

func awaitWorkerResult(
	t *testing.T,
	result <-chan error,
) error {
	t.Helper()

	select {
	case err := <-result:
		return err
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for worker")
		return nil
	}
}

func TestDiscoveryWorkerSingleFlightNoOverlap(t *testing.T) {
	worker, discovery, dispatch := newBlockingWorker(t)

	first := make(chan error, 1)
	second := make(chan error, 1)

	go func() {
		first <- worker.RunOnce(context.Background(), nil)
	}()

	<-discovery.entered

	go func() {
		second <- worker.RunOnce(context.Background(), nil)
	}()

	// The second caller cannot enter discovery while the first
	// owns the worker execution lock.
	if discovery.maxActive.Load() != 1 {
		t.Fatal("unexpected overlapping discovery")
	}

	close(discovery.release)

	if err := awaitWorkerResult(t, first); err != nil {
		t.Fatal(err)
	}

	if err := awaitWorkerResult(t, second); err != nil {
		t.Fatal(err)
	}

	if discovery.calls.Load() != 2 {
		t.Fatalf(
			"expected two serialized discovery calls, got %d",
			discovery.calls.Load(),
		)
	}

	if discovery.maxActive.Load() != 1 {
		t.Fatalf(
			"discovery overlapped: maximum concurrency %d",
			discovery.maxActive.Load(),
		)
	}

	if dispatch.calls.Load() != 2 {
		t.Fatalf(
			"expected two dispatches, got %d",
			dispatch.calls.Load(),
		)
	}
}

func TestDiscoveryWorkerCancelledWaiterDoesNotDispatch(t *testing.T) {
	worker, discovery, dispatch := newBlockingWorker(t)

	first := make(chan error, 1)

	go func() {
		first <- worker.RunOnce(context.Background(), nil)
	}()

	<-discovery.entered

	ctx, cancel := context.WithCancel(context.Background())

	second := make(chan error, 1)

	go func() {
		second <- worker.RunOnce(ctx, nil)
	}()

	cancel()

	err := awaitWorkerResult(t, second)

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected cancelled waiter, got %v", err)
	}

	if discovery.calls.Load() != 1 {
		t.Fatalf(
			"cancelled waiter entered discovery: %d calls",
			discovery.calls.Load(),
		)
	}

	close(discovery.release)

	if err := awaitWorkerResult(t, first); err != nil {
		t.Fatal(err)
	}

	if dispatch.calls.Load() != 1 {
		t.Fatalf(
			"cancelled waiter dispatched: %d calls",
			dispatch.calls.Load(),
		)
	}
}

func TestDiscoveryWorkerWaiterRunsAfterRelease(t *testing.T) {
	worker, discovery, dispatch := newBlockingWorker(t)

	first := make(chan error, 1)
	second := make(chan error, 1)

	go func() {
		first <- worker.RunOnce(context.Background(), nil)
	}()

	<-discovery.entered

	go func() {
		second <- worker.RunOnce(context.Background(), nil)
	}()

	if discovery.calls.Load() != 1 {
		t.Fatal("second discovery started before release")
	}

	close(discovery.release)

	if err := awaitWorkerResult(t, first); err != nil {
		t.Fatal(err)
	}

	if err := awaitWorkerResult(t, second); err != nil {
		t.Fatal(err)
	}

	if discovery.calls.Load() != 2 ||
		dispatch.calls.Load() != 2 {
		t.Fatalf(
			"unexpected execution counts: discovery=%d dispatch=%d",
			discovery.calls.Load(),
			dispatch.calls.Load(),
		)
	}
}
