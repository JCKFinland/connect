package driver_payout

import (
	"errors"
	"testing"
	"time"

	"github.com/JCKFinland/connect/backend/internal/payouts"
)

func TestRecoveryFinancialAuthorizationFailsClosedWithoutVerifier(t *testing.T) {
	fixture := newSubmissionFixture(t)
	inspector := &payoutRecoveryFake{}

	worker, err := NewPostgresPayoutRecoveryWorker(
		fixture.db, inspector, time.Minute, 10,
	)
	if err != nil {
		t.Fatal(err)
	}

	worker.financialFinalizationEnabled = true

	err = worker.RunOnce(fixture.ctx, nil)
	if !errors.Is(err, ErrInvalidRecovery) {
		t.Fatalf("expected fail-closed denial, got %v", err)
	}

	inspector.mu.Lock()
	defer inspector.mu.Unlock()

	if inspector.lookups != 0 {
		t.Fatalf("unexpected legacy lookups: %d", inspector.lookups)
	}
}

func TestRecoveryFinancialAuthorizationFailsClosedWithWrongDatabase(t *testing.T) {
	fixture := newSubmissionFixture(t)
	inspector := &payoutRecoveryFake{}

	worker, err := NewPostgresPayoutRecoveryWorker(
		fixture.db, inspector, time.Minute, 10,
	)
	if err != nil {
		t.Fatal(err)
	}

	provider := &leaseVerificationProvider{
		details: payouts.VerifiedTransferDetails{},
	}

	verifier, err := NewLeaseFencedVerificationService(
		fixture.db, provider,
	)
	if err != nil {
		t.Fatal(err)
	}

	if err := worker.ConfigureVerifiedRecovery(verifier); err != nil {
		t.Fatal(err)
	}

	if err := worker.ConfigureFinancialFinalization(); err != nil {
		t.Fatal(err)
	}

	worker.verifiedRecovery.db = nil

	err = worker.RunOnce(fixture.ctx, nil)
	if !errors.Is(err, ErrInvalidRecovery) {
		t.Fatalf("expected database mismatch denial, got %v", err)
	}

	if provider.calls != 0 {
		t.Fatalf("provider called with invalid configuration: %d", provider.calls)
	}
}

func TestRecoveryFinancialAuthorizationFailsClosedWithMissingVerifierEngine(t *testing.T) {
	fixture := newSubmissionFixture(t)
	inspector := &payoutRecoveryFake{}

	worker, err := NewPostgresPayoutRecoveryWorker(
		fixture.db, inspector, time.Minute, 10,
	)
	if err != nil {
		t.Fatal(err)
	}

	provider := &leaseVerificationProvider{
		details: payouts.VerifiedTransferDetails{},
	}

	verifier, err := NewLeaseFencedVerificationService(
		fixture.db, provider,
	)
	if err != nil {
		t.Fatal(err)
	}

	if err := worker.ConfigureVerifiedRecovery(verifier); err != nil {
		t.Fatal(err)
	}

	if err := worker.ConfigureFinancialFinalization(); err != nil {
		t.Fatal(err)
	}

	worker.verifiedRecovery.verifier = nil

	err = worker.RunOnce(fixture.ctx, nil)
	if !errors.Is(err, ErrInvalidRecovery) {
		t.Fatalf("expected missing verification engine denial, got %v", err)
	}

	if provider.calls != 0 {
		t.Fatalf("provider called without verifier engine: %d", provider.calls)
	}
}
