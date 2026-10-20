package flex_settlement

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	postgresrepo "github.com/JCKFinland/connect/backend/internal/repository/postgres"
)

type workerTestDiscovery struct {
	candidates []postgresrepo.FlexSettlementCandidate
	err        error
	calls      int
	onNext     func()
}

func (f *workerTestDiscovery) Next(
	_ context.Context,
) ([]postgresrepo.FlexSettlementCandidate, error) {
	f.calls++

	if f.onNext != nil {
		f.onNext()
	}

	return f.candidates, f.err
}

type workerTestDispatch struct {
	ids     []string
	errByID map[string]error
	onCall  func(string)
}

func (f *workerTestDispatch) Dispatch(
	_ context.Context,
	candidate postgresrepo.FlexSettlementCandidate,
) (*RecoveryResult, error) {
	f.ids = append(f.ids, candidate.SettlementID)

	if f.onCall != nil {
		f.onCall(candidate.SettlementID)
	}

	if err := f.errByID[candidate.SettlementID]; err != nil {
		return nil, err
	}

	return &RecoveryResult{
		SettlementID: candidate.SettlementID,
		Status:       RecoveryPendingProvider,
	}, nil
}

func workerCandidate(
	id string,
) postgresrepo.FlexSettlementCandidate {
	return postgresrepo.FlexSettlementCandidate{
		SettlementID: id,
		Kind:         postgresrepo.FlexSettlementCandidateRecovery,
	}
}

func TestDiscoveryWorkerProcessesBatchInOrder(t *testing.T) {
	discovery := &workerTestDiscovery{
		candidates: []postgresrepo.FlexSettlementCandidate{
			workerCandidate("first"),
			workerCandidate("second"),
			workerCandidate("third"),
		},
	}

	dispatch := &workerTestDispatch{}

	worker, err := NewDiscoveryWorker(discovery, dispatch)
	if err != nil {
		t.Fatal(err)
	}

	if err := worker.RunOnce(context.Background(), nil); err != nil {
		t.Fatal(err)
	}

	expected := []string{"first", "second", "third"}

	if discovery.calls != 1 || len(dispatch.ids) != len(expected) {
		t.Fatalf(
			"unexpected execution: discovery=%d dispatch=%v",
			discovery.calls,
			dispatch.ids,
		)
	}

	for i, id := range expected {
		if dispatch.ids[i] != id {
			t.Fatalf(
				"dispatch order mismatch: got %v, want %v",
				dispatch.ids,
				expected,
			)
		}
	}
}

func TestDiscoveryWorkerContinuesAfterCandidateFailure(t *testing.T) {
	expectedError := errors.New("provider verification failed")

	discovery := &workerTestDiscovery{
		candidates: []postgresrepo.FlexSettlementCandidate{
			workerCandidate("first"),
			workerCandidate("second"),
			workerCandidate("third"),
		},
	}

	dispatch := &workerTestDispatch{
		errByID: map[string]error{
			"second": expectedError,
		},
	}

	worker, err := NewDiscoveryWorker(discovery, dispatch)
	if err != nil {
		t.Fatal(err)
	}

	var reported []error

	err = worker.RunOnce(
		context.Background(),
		func(err error) {
			reported = append(reported, err)
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	if len(dispatch.ids) != 3 {
		t.Fatalf(
			"candidate failure interrupted batch: %v",
			dispatch.ids,
		)
	}

	if len(reported) != 1 ||
		!errors.Is(reported[0], expectedError) ||
		!strings.Contains(reported[0].Error(), "second") {
		t.Fatalf("unexpected error reporting: %v", reported)
	}
}

func TestDiscoveryWorkerPropagatesDiscoveryFailure(t *testing.T) {
	expectedError := errors.New("database unavailable")

	discovery := &workerTestDiscovery{
		err: expectedError,
	}

	dispatch := &workerTestDispatch{}

	worker, err := NewDiscoveryWorker(discovery, dispatch)
	if err != nil {
		t.Fatal(err)
	}

	err = worker.RunOnce(context.Background(), nil)

	if !errors.Is(err, expectedError) {
		t.Fatalf("expected discovery failure, got %v", err)
	}

	if len(dispatch.ids) != 0 {
		t.Fatalf("dispatched after discovery failure: %v", dispatch.ids)
	}
}

func TestDiscoveryWorkerStopsBetweenCandidatesOnCancellation(
	t *testing.T,
) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	discovery := &workerTestDiscovery{
		candidates: []postgresrepo.FlexSettlementCandidate{
			workerCandidate("first"),
			workerCandidate("second"),
		},
	}

	dispatch := &workerTestDispatch{
		onCall: func(id string) {
			if id == "first" {
				cancel()
			}
		},
	}

	worker, err := NewDiscoveryWorker(discovery, dispatch)
	if err != nil {
		t.Fatal(err)
	}

	err = worker.RunOnce(ctx, nil)

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected cancellation, got %v", err)
	}

	if len(dispatch.ids) != 1 || dispatch.ids[0] != "first" {
		t.Fatalf("unexpected dispatch after cancellation: %v", dispatch.ids)
	}
}

func TestDiscoveryWorkerRejectsInvalidDependencies(t *testing.T) {
	if _, err := NewDiscoveryWorker(
		nil,
		&workerTestDispatch{},
	); err == nil {
		t.Fatal("accepted nil discovery")
	}

	if _, err := NewDiscoveryWorker(
		&workerTestDiscovery{},
		nil,
	); err == nil {
		t.Fatal("accepted nil dispatcher")
	}

	var worker *DiscoveryWorker

	if err := worker.RunOnce(context.Background(), nil); err == nil {
		t.Fatal("accepted nil worker")
	}

	valid, err := NewDiscoveryWorker(
		&workerTestDiscovery{},
		&workerTestDispatch{},
	)
	if err != nil {
		t.Fatal(err)
	}

	if err := valid.RunOnce(nil, nil); err == nil {
		t.Fatal("accepted nil context")
	}

	if err := valid.Run(
		context.Background(),
		DiscoveryWorkerOptions{},
	); err == nil {
		t.Fatal("accepted nonpositive polling interval")
	}
}

func TestDiscoveryWorkerRunStopsAfterCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var once sync.Once

	discovery := &workerTestDiscovery{
		onNext: func() {
			once.Do(cancel)
		},
	}

	dispatch := &workerTestDispatch{}

	worker, err := NewDiscoveryWorker(discovery, dispatch)
	if err != nil {
		t.Fatal(err)
	}

	done := make(chan error, 1)

	go func() {
		done <- worker.Run(
			ctx,
			DiscoveryWorkerOptions{
				Interval: time.Millisecond,
			},
		)
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("worker shutdown returned error: %v", err)
		}

	case <-time.After(3 * time.Second):
		t.Fatal("worker did not stop after cancellation")
	}

	if discovery.calls < 1 {
		t.Fatal("worker never performed discovery")
	}
}

func TestDiscoveryWorkerRunReportsDiscoveryError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	expectedError := errors.New("discovery failed")
	reported := make(chan error, 1)

	discovery := &workerTestDiscovery{
		err: expectedError,
	}

	worker, err := NewDiscoveryWorker(
		discovery,
		&workerTestDispatch{},
	)
	if err != nil {
		t.Fatal(err)
	}

	done := make(chan error, 1)

	go func() {
		done <- worker.Run(
			ctx,
			DiscoveryWorkerOptions{
				Interval: time.Millisecond,
				OnError: func(err error) {
					reported <- err
					cancel()
				},
			},
		)
	}()

	select {
	case err := <-reported:
		if !errors.Is(err, expectedError) {
			t.Fatalf("unexpected reported error: %v", err)
		}

	case <-time.After(3 * time.Second):
		cancel()
		t.Fatal("discovery error was not reported")
	}

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("worker returned unexpected error: %v", err)
		}

	case <-time.After(3 * time.Second):
		t.Fatal("worker did not shut down")
	}
}
