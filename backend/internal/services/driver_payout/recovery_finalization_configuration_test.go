package driver_payout

import (
	"errors"
	"testing"
	"time"

	"github.com/JCKFinland/connect/backend/internal/payouts"
)

func TestRecoveryFinancialFinalizationRequiresVerifiedConfiguration(
	t *testing.T,
) {
	fixture := newSubmissionFixture(t)

	inspector := &payoutRecoveryFake{}

	worker, err := NewPostgresPayoutRecoveryWorker(
		fixture.db, inspector, time.Minute, 10,
	)
	if err != nil {
		t.Fatal(err)
	}

	if worker.financialFinalizationEnabled {
		t.Fatal("financial finalization enabled by default")
	}

	if err := worker.ConfigureFinancialFinalization(); !errors.Is(
		err, ErrInvalidRecovery,
	) {
		t.Fatalf("expected unverified configuration denial, got %v", err)
	}

	if worker.financialFinalizationEnabled {
		t.Fatal("unverified worker enabled financial finalization")
	}

	var nilWorker *PostgresPayoutRecoveryWorker
	if err := nilWorker.ConfigureFinancialFinalization(); !errors.Is(
		err, ErrInvalidRecovery,
	) {
		t.Fatalf("expected nil worker denial, got %v", err)
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

	if worker.financialFinalizationEnabled {
		t.Fatal("verification configuration enabled finalization")
	}

	if err := worker.ConfigureFinancialFinalization(); err != nil {
		t.Fatal(err)
	}

	if !worker.financialFinalizationEnabled {
		t.Fatal("explicit authorization not enabled")
	}

	t.Log("PASS: finalization requires explicit verified configuration")
}
