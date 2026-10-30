package driver_payout

import (
	"context"
	"testing"
	"time"

	postgresrepo "github.com/JCKFinland/connect/backend/internal/repository/postgres"
)

func TestPostgresPayoutRecoveryWorkerVerifiedPath(t *testing.T) {
	fixture := newSubmissionFixture(t)

	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(
			context.Background(), 15*time.Second,
		)
		defer cancel()

		if _, err := fixture.db.Exec(ctx, `
            DELETE FROM driver_payout_recovery_leases
            WHERE payout_id=$1::uuid
        `, fixture.payoutID); err != nil {
			t.Errorf("lease cleanup: %v", err)
		}
	})

	execution, claimed, err :=
		postgresrepo.ClaimDriverPayoutExecution(
			fixture.ctx, fixture.db, fixture.payoutID,
		)

	if err != nil || !claimed || execution == nil {
		t.Fatalf("claim: claimed=%v err=%v", claimed, err)
	}

	expected, err :=
		postgresrepo.ResolveDriverPayoutRecoveryTransferRequest(
			fixture.ctx, fixture.db, fixture.payoutID,
		)
	if err != nil || expected == nil {
		t.Fatalf("resolve: %v", err)
	}

	inspector := &payoutRecoveryFake{
		candidates: []postgresrepo.StaleDriverPayoutExecution{
			postgresRecoveryAuthoritativeCandidate(t, fixture),
		},
	}

	provider := &leaseVerificationProvider{
		details: successfulVerifiedReconciliationDetails(*expected),
	}

	verifier, err := NewLeaseFencedVerificationService(
		fixture.db, provider,
	)
	if err != nil {
		t.Fatal(err)
	}

	worker, err := NewPostgresPayoutRecoveryWorker(
		fixture.db, inspector, time.Minute, 10,
	)
	if err != nil {
		t.Fatal(err)
	}

	if err := worker.ConfigureVerifiedRecovery(verifier); err != nil {
		t.Fatal(err)
	}

	var reported []error
	err = worker.RunOnce(
		fixture.ctx,
		func(err error) {
			reported = append(reported, err)
		},
	)
	if err != nil {
		t.Fatalf("run worker: %v", err)
	}

	if len(reported) != 0 {
		t.Fatalf("unexpected recovery errors: %v", reported)
	}

	if provider.calls != 1 {
		t.Fatalf("expected one verified lookup, got %d", provider.calls)
	}

	inspector.mu.Lock()
	legacyLookups := inspector.lookups
	inspector.mu.Unlock()

	if legacyLookups != 0 {
		t.Fatalf("legacy inspector lookup executed: %d", legacyLookups)
	}

	var lookupCount int64
	var cooldownActive, leaseReleased bool

	err = fixture.db.QueryRow(fixture.ctx, `
        SELECT lookup_count,
               next_lookup_at > clock_timestamp(),
               lease_expires_at <= clock_timestamp()
        FROM driver_payout_recovery_leases
        WHERE payout_id=$1::uuid
    `, fixture.payoutID).Scan(
		&lookupCount, &cooldownActive, &leaseReleased,
	)
	if err != nil {
		t.Fatal(err)
	}

	if lookupCount != 1 || !cooldownActive || !leaseReleased {
		t.Fatalf(
			"lease state: count=%d cooldown=%v released=%v",
			lookupCount, cooldownActive, leaseReleased,
		)
	}

	var payoutStatus, executionStatus string
	var providerReference *string

	err = fixture.db.QueryRow(fixture.ctx, `
        SELECT p.payout_status, e.execution_status, e.provider_reference
        FROM driver_payouts p
        JOIN driver_payout_executions e ON e.payout_id=p.id
        WHERE p.id=$1::uuid
    `, fixture.payoutID).Scan(
		&payoutStatus, &executionStatus, &providerReference,
	)
	if err != nil {
		t.Fatal(err)
	}

	if payoutStatus != "PENDING" ||
		executionStatus != "PROCESSING" ||
		providerReference != nil {
		t.Fatalf(
			"financial state changed: payout=%s execution=%s ref=%v",
			payoutStatus, executionStatus, providerReference,
		)
	}

	t.Log("PASS: verified lookup; legacy bypassed; lease released; financial state unchanged")
}
