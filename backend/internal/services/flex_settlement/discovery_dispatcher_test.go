package flex_settlement

import (
	"context"
	"errors"
	"testing"

	stripepayment "github.com/JCKFinland/connect/backend/internal/payments/stripe"
	postgresrepo "github.com/JCKFinland/connect/backend/internal/repository/postgres"
)

type fakeSettlementExecutor struct {
	captures   int
	reconciles int
	lastID     string
	result     *RecoveryResult
	err        error
}

func (f *fakeSettlementExecutor) ExecuteCapture(
	_ context.Context,
	id string,
	_ stripepayment.FlexCaptureExecutor,
) (*RecoveryResult, error) {
	f.captures++
	f.lastID = id
	return f.result, f.err
}

func (f *fakeSettlementExecutor) Reconcile(
	_ context.Context,
	id string,
) (*RecoveryResult, error) {
	f.reconciles++
	f.lastID = id
	return f.result, f.err
}

type fakeCaptureExecutor struct{}

func (*fakeCaptureExecutor) ExecuteFlexCapture(
	context.Context,
	stripepayment.FlexCaptureRequest,
) (*stripepayment.FlexCaptureResult, error) {
	return nil, errors.New(
		"network-free test executor must not be invoked",
	)
}

func TestDiscoveryDispatcherCaptureRouting(t *testing.T) {
	executor := &fakeSettlementExecutor{
		result: &RecoveryResult{
			Status: RecoveryCaptured,
		},
	}

	dispatcher, err := NewDiscoveryDispatcher(
		executor,
		&fakeCaptureExecutor{},
	)
	if err != nil {
		t.Fatal(err)
	}

	result, err := dispatcher.Dispatch(
		context.Background(),
		postgresrepo.FlexSettlementCandidate{
			SettlementID: "capture-1",
			Kind:         postgresrepo.FlexSettlementCandidateCapture,
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	if result != executor.result ||
		executor.captures != 1 ||
		executor.reconciles != 0 ||
		executor.lastID != "capture-1" {
		t.Fatalf("incorrect capture routing: %+v", executor)
	}
}

func TestDiscoveryDispatcherRecoveryRouting(t *testing.T) {
	executor := &fakeSettlementExecutor{
		result: &RecoveryResult{
			Status: RecoveryPendingProvider,
		},
	}

	dispatcher, err := NewDiscoveryDispatcher(
		executor,
		&fakeCaptureExecutor{},
	)
	if err != nil {
		t.Fatal(err)
	}

	result, err := dispatcher.Dispatch(
		context.Background(),
		postgresrepo.FlexSettlementCandidate{
			SettlementID: "recovery-1",
			Kind:         postgresrepo.FlexSettlementCandidateRecovery,
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	if result != executor.result ||
		executor.captures != 0 ||
		executor.reconciles != 1 ||
		executor.lastID != "recovery-1" {
		t.Fatalf("incorrect recovery routing: %+v", executor)
	}
}

func TestDiscoveryDispatcherRejectsUnknownCategory(t *testing.T) {
	executor := &fakeSettlementExecutor{}

	dispatcher, err := NewDiscoveryDispatcher(
		executor,
		&fakeCaptureExecutor{},
	)
	if err != nil {
		t.Fatal(err)
	}

	_, err = dispatcher.Dispatch(
		context.Background(),
		postgresrepo.FlexSettlementCandidate{
			SettlementID: "unknown-1",
			Kind:         "UNKNOWN",
		},
	)

	if err == nil ||
		executor.captures != 0 ||
		executor.reconciles != 0 {
		t.Fatal("unknown category was not rejected safely")
	}
}

func TestDiscoveryDispatcherRejectsMissingID(t *testing.T) {
	executor := &fakeSettlementExecutor{}

	dispatcher, err := NewDiscoveryDispatcher(
		executor,
		&fakeCaptureExecutor{},
	)
	if err != nil {
		t.Fatal(err)
	}

	_, err = dispatcher.Dispatch(
		context.Background(),
		postgresrepo.FlexSettlementCandidate{
			Kind: postgresrepo.FlexSettlementCandidateCapture,
		},
	)

	if err == nil ||
		executor.captures != 0 ||
		executor.reconciles != 0 {
		t.Fatal("missing settlement ID was not rejected")
	}
}

func TestDiscoveryDispatcherPropagatesExecutionError(t *testing.T) {
	expected := errors.New("capture failed")
	executor := &fakeSettlementExecutor{err: expected}

	dispatcher, err := NewDiscoveryDispatcher(
		executor,
		&fakeCaptureExecutor{},
	)
	if err != nil {
		t.Fatal(err)
	}

	_, err = dispatcher.Dispatch(
		context.Background(),
		postgresrepo.FlexSettlementCandidate{
			SettlementID: "capture-error",
			Kind:         postgresrepo.FlexSettlementCandidateCapture,
		},
	)

	if !errors.Is(err, expected) {
		t.Fatalf("expected original error, got %v", err)
	}
}

func TestDiscoveryDispatcherRejectsCancelledContext(t *testing.T) {
	executor := &fakeSettlementExecutor{}

	dispatcher, err := NewDiscoveryDispatcher(
		executor,
		&fakeCaptureExecutor{},
	)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err = dispatcher.Dispatch(
		ctx,
		postgresrepo.FlexSettlementCandidate{
			SettlementID: "cancelled-1",
			Kind:         postgresrepo.FlexSettlementCandidateCapture,
		},
	)

	if !errors.Is(err, context.Canceled) ||
		executor.captures != 0 ||
		executor.reconciles != 0 {
		t.Fatal("cancelled context was not respected")
	}
}

func TestDiscoveryDispatcherRejectsNilDependencies(t *testing.T) {
	if _, err := NewDiscoveryDispatcher(
		nil,
		&fakeCaptureExecutor{},
	); err == nil {
		t.Fatal("nil settlement executor accepted")
	}

	if _, err := NewDiscoveryDispatcher(
		&fakeSettlementExecutor{},
		nil,
	); err == nil {
		t.Fatal("nil capture executor accepted")
	}

	var dispatcher *DiscoveryDispatcher

	_, err := dispatcher.Dispatch(
		context.Background(),
		postgresrepo.FlexSettlementCandidate{},
	)

	if err == nil {
		t.Fatal("nil dispatcher accepted")
	}
}
