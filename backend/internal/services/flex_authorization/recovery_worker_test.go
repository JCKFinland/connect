package flex_authorization

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	postgresrepo "github.com/JCKFinland/connect/backend/internal/repository/postgres"
)

type authorizationWorkerDiscoveryFake struct {
	mu         sync.Mutex
	candidates []postgresrepo.FlexAuthorizationRecoveryCandidate
	err        error
	calls      int
	batchSize  int
	staleAfter time.Duration
	onDiscover func()
}

func (f *authorizationWorkerDiscoveryFake) Discover(
	ctx context.Context,
	staleAfter time.Duration,
	batchSize int,
) ([]postgresrepo.FlexAuthorizationRecoveryCandidate, error) {
	f.mu.Lock()
	f.calls++
	f.batchSize = batchSize
	f.staleAfter = staleAfter
	fn := f.onDiscover
	candidates := f.candidates
	err := f.err
	f.mu.Unlock()

	if fn != nil {
		fn()
	}

	return candidates, err
}

type authorizationWorkerExecutorFake struct {
	mu        sync.Mutex
	ids       []string
	errByID   map[string]error
	onExecute func(string)
}

func (f *authorizationWorkerExecutorFake) RecoverAndReconcile(
	ctx context.Context,
	id string,
) (*RecoveryReconcileResult, error) {
	f.mu.Lock()
	f.ids = append(f.ids, id)
	fn := f.onExecute
	err := f.errByID[id]
	f.mu.Unlock()

	if fn != nil {
		fn(id)
	}

	if err != nil {
		return nil, err
	}

	return &RecoveryReconcileResult{
		AuthorizationID: id,
		Status:          "PROCESSING",
	}, nil
}

func authorizationWorkerCandidates(
	ids ...string,
) []postgresrepo.FlexAuthorizationRecoveryCandidate {
	result := make(
		[]postgresrepo.FlexAuthorizationRecoveryCandidate,
		0,
		len(ids),
	)

	for _, id := range ids {
		result = append(
			result,
			postgresrepo.FlexAuthorizationRecoveryCandidate{
				AuthorizationID: id,
			},
		)
	}

	return result
}

func newAuthorizationWorkerForTest(
	t *testing.T,
	discovery *authorizationWorkerDiscoveryFake,
	executor *authorizationWorkerExecutorFake,
	batchSize int,
) *AuthorizationRecoveryWorker {
	t.Helper()

	worker, err := NewAuthorizationRecoveryWorker(
		discovery,
		executor,
		5*time.Minute,
		batchSize,
	)
	if err != nil {
		t.Fatal(err)
	}

	return worker
}

func TestAuthorizationRecoveryWorkerProcessesBatchInOrder(t *testing.T) {
	discovery := &authorizationWorkerDiscoveryFake{
		candidates: authorizationWorkerCandidates(
			"first",
			"second",
			"third",
		),
	}
	executor := &authorizationWorkerExecutorFake{}

	worker := newAuthorizationWorkerForTest(
		t, discovery, executor, 3,
	)

	if err := worker.RunOnce(context.Background(), nil); err != nil {
		t.Fatal(err)
	}

	expected := []string{"first", "second", "third"}

	if !reflect.DeepEqual(executor.ids, expected) {
		t.Fatalf("execution order: got %v, want %v",
			executor.ids, expected)
	}

	if discovery.calls != 1 ||
		discovery.batchSize != 3 ||
		discovery.staleAfter != 5*time.Minute {
		t.Fatalf("unexpected discovery arguments: %+v", discovery)
	}
}

func TestAuthorizationRecoveryWorkerBatchLimit(t *testing.T) {
	discovery := &authorizationWorkerDiscoveryFake{}
	executor := &authorizationWorkerExecutorFake{}

	worker := newAuthorizationWorkerForTest(
		t, discovery, executor, 1000,
	)

	if err := worker.RunOnce(context.Background(), nil); err != nil {
		t.Fatal(err)
	}

	if discovery.batchSize !=
		postgresrepo.MaxFlexAuthorizationRecoveryBatchSize {
		t.Fatalf("batch limit not enforced: %d",
			discovery.batchSize)
	}

	discovery.candidates = authorizationWorkerCandidates(
		"one", "two", "three",
	)

	worker = newAuthorizationWorkerForTest(
		t, discovery, executor, 2,
	)

	if err := worker.RunOnce(context.Background(), nil); err == nil {
		t.Fatal("oversized discovery result accepted")
	}

	if len(executor.ids) != 0 {
		t.Fatal("executed candidates after batch overflow")
	}
}

func TestAuthorizationRecoveryWorkerDefaultBatch(t *testing.T) {
	discovery := &authorizationWorkerDiscoveryFake{}
	executor := &authorizationWorkerExecutorFake{}

	worker := newAuthorizationWorkerForTest(
		t, discovery, executor, 0,
	)

	if err := worker.RunOnce(context.Background(), nil); err != nil {
		t.Fatal(err)
	}

	if discovery.batchSize !=
		postgresrepo.DefaultFlexAuthorizationRecoveryBatchSize {
		t.Fatalf("unexpected default batch: %d",
			discovery.batchSize)
	}
}

func TestAuthorizationRecoveryWorkerContinuesAfterFailure(t *testing.T) {
	failure := errors.New("provider unavailable")

	discovery := &authorizationWorkerDiscoveryFake{
		candidates: authorizationWorkerCandidates(
			"first", "second", "third",
		),
	}

	executor := &authorizationWorkerExecutorFake{
		errByID: map[string]error{
			"second": failure,
		},
	}

	worker := newAuthorizationWorkerForTest(
		t, discovery, executor, 3,
	)

	var reported []error

	err := worker.RunOnce(
		context.Background(),
		func(err error) {
			reported = append(reported, err)
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(
		executor.ids,
		[]string{"first", "second", "third"},
	) {
		t.Fatalf("candidate failure interrupted batch: %v",
			executor.ids)
	}

	if len(reported) != 1 ||
		!errors.Is(reported[0], failure) ||
		!strings.Contains(reported[0].Error(), "second") {
		t.Fatalf("unexpected error reports: %v", reported)
	}
}

func TestAuthorizationRecoveryWorkerRejectsMissingCandidateID(t *testing.T) {
	discovery := &authorizationWorkerDiscoveryFake{
		candidates: authorizationWorkerCandidates(
			"", "valid",
		),
	}

	executor := &authorizationWorkerExecutorFake{}

	worker := newAuthorizationWorkerForTest(
		t, discovery, executor, 2,
	)

	var reported []error

	if err := worker.RunOnce(
		context.Background(),
		func(err error) {
			reported = append(reported, err)
		},
	); err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(executor.ids, []string{"valid"}) {
		t.Fatalf("invalid candidate dispatched: %v", executor.ids)
	}

	if len(reported) != 1 {
		t.Fatalf("expected one candidate error: %v", reported)
	}
}

func TestAuthorizationRecoveryWorkerDiscoveryFailure(t *testing.T) {
	failure := errors.New("database unavailable")

	discovery := &authorizationWorkerDiscoveryFake{
		err: failure,
	}

	executor := &authorizationWorkerExecutorFake{}

	worker := newAuthorizationWorkerForTest(
		t, discovery, executor, 2,
	)

	err := worker.RunOnce(context.Background(), nil)

	if !errors.Is(err, failure) {
		t.Fatalf("expected discovery failure, got %v", err)
	}

	if len(executor.ids) != 0 {
		t.Fatal("executed after discovery failure")
	}
}

func TestAuthorizationRecoveryWorkerStopsOnCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	discovery := &authorizationWorkerDiscoveryFake{
		candidates: authorizationWorkerCandidates(
			"first", "second",
		),
	}

	executor := &authorizationWorkerExecutorFake{
		onExecute: func(id string) {
			if id == "first" {
				cancel()
			}
		},
	}

	worker := newAuthorizationWorkerForTest(
		t, discovery, executor, 2,
	)

	err := worker.RunOnce(ctx, nil)

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected cancellation, got %v", err)
	}

	if !reflect.DeepEqual(executor.ids, []string{"first"}) {
		t.Fatalf("executed after cancellation: %v", executor.ids)
	}
}

func TestAuthorizationRecoveryWorkerCancelledWaiter(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})

	discovery := &authorizationWorkerDiscoveryFake{
		onDiscover: func() {
			entered <- struct{}{}
			<-release
		},
	}

	executor := &authorizationWorkerExecutorFake{}

	worker := newAuthorizationWorkerForTest(
		t, discovery, executor, 2,
	)

	first := make(chan error, 1)

	go func() {
		first <- worker.RunOnce(context.Background(), nil)
	}()

	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("first worker did not enter discovery")
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	second := make(chan error, 1)

	go func() {
		second <- worker.RunOnce(ctx, nil)
	}()

	select {
	case err := <-second:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("expected cancelled waiter, got %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("cancelled waiter blocked behind active worker")
	}

	close(release)

	select {
	case err := <-first:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("first worker did not finish")
	}

	if discovery.calls != 1 {
		t.Fatalf("cancelled waiter entered discovery: %d",
			discovery.calls)
	}
}

func TestAuthorizationRecoveryWorkerSerializesRuns(t *testing.T) {
	entered := make(chan struct{}, 2)
	release := make(chan struct{})

	var mu sync.Mutex
	active := 0
	maxActive := 0

	discovery := &authorizationWorkerDiscoveryFake{
		onDiscover: func() {
			mu.Lock()
			active++
			if active > maxActive {
				maxActive = active
			}
			mu.Unlock()

			entered <- struct{}{}
			<-release

			mu.Lock()
			active--
			mu.Unlock()
		},
	}

	executor := &authorizationWorkerExecutorFake{}

	worker := newAuthorizationWorkerForTest(
		t, discovery, executor, 2,
	)

	first := make(chan error, 1)
	second := make(chan error, 1)

	go func() {
		first <- worker.RunOnce(context.Background(), nil)
	}()

	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("first worker did not enter")
	}

	go func() {
		second <- worker.RunOnce(context.Background(), nil)
	}()

	select {
	case <-entered:
		t.Fatal("overlapping worker execution")
	case <-time.After(100 * time.Millisecond):
	}

	close(release)

	for _, result := range []<-chan error{first, second} {
		select {
		case err := <-result:
			if err != nil {
				t.Fatal(err)
			}
		case <-time.After(3 * time.Second):
			t.Fatal("worker execution timed out")
		}
	}

	if maxActive != 1 {
		t.Fatalf("overlapping discovery: %d", maxActive)
	}

	if discovery.calls != 2 {
		t.Fatalf("expected two serialized calls, got %d",
			discovery.calls)
	}
}

func TestAuthorizationRecoveryWorkerRejectsInvalidConstruction(
	t *testing.T,
) {
	discovery := &authorizationWorkerDiscoveryFake{}
	executor := &authorizationWorkerExecutorFake{}

	cases := []struct {
		name       string
		discovery  AuthorizationRecoveryDiscovery
		executor   AuthorizationRecoveryExecutor
		staleAfter time.Duration
	}{
		{"missing discovery", nil, executor, time.Minute},
		{"missing executor", discovery, nil, time.Minute},
		{"invalid threshold", discovery, executor, 0},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			worker, err := NewAuthorizationRecoveryWorker(
				tc.discovery,
				tc.executor,
				tc.staleAfter,
				10,
			)

			if err == nil || worker != nil {
				t.Fatal("invalid worker construction accepted")
			}
		})
	}
}
