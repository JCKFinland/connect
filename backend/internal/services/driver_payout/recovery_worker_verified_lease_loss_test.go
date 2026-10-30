package driver_payout

import (
	"context"
	"errors"
	"testing"
	"time"

	postgresrepo "github.com/JCKFinland/connect/backend/internal/repository/postgres"
	"github.com/google/uuid"
)

func TestPostgresPayoutRecoveryWorkerVerifiedLeaseLoss(t *testing.T) {
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

	provider.beforeReturn = func() {
		tag, err := fixture.db.Exec(fixture.ctx, `
            UPDATE driver_payout_recovery_leases
            SET lease_token=$2::uuid,
                lease_owner='replacement-owner',
                updated_at=clock_timestamp()
            WHERE payout_id=$1::uuid
              AND lease_expires_at > clock_timestamp()
        `, fixture.payoutID, uuid.NewString())

		if err != nil {
			t.Errorf("replace lease token: %v", err)
			return
		}

		if tag.RowsAffected() != 1 {
			t.Errorf(
				"expected one replaced lease, got %d",
				tag.RowsAffected(),
			)
		}
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
		t.Fatalf("worker: %v", err)
	}

	if provider.calls != 1 {
		t.Fatalf(
			"expected one provider lookup, got %d",
			provider.calls,
		)
	}

	inspector.mu.Lock()
	legacyLookups := inspector.lookups
	inspector.mu.Unlock()

	if legacyLookups != 0 {
		t.Fatalf(
			"legacy lookup unexpectedly executed: %d",
			legacyLookups,
		)
	}

	foundLeaseDenial := false

	for _, reportedErr := range reported {
		if errors.Is(
			reportedErr,
			ErrLeaseFencedVerificationDenied,
		) {
			foundLeaseDenial = true
		}
	}

	if !foundLeaseDenial {
		t.Fatalf(
			"expected lease-fenced denial, reported=%v",
			reported,
		)
	}

	var payoutStatus, executionStatus string
	var payoutReference, executionReference *string

	err = fixture.db.QueryRow(fixture.ctx, `
        SELECT p.payout_status,
               e.execution_status,
               p.provider_reference,
               e.provider_reference
        FROM driver_payouts p
        JOIN driver_payout_executions e ON e.payout_id=p.id
        WHERE p.id=$1::uuid
    `, fixture.payoutID).Scan(
		&payoutStatus,
		&executionStatus,
		&payoutReference,
		&executionReference,
	)

	if err != nil {
		t.Fatal(err)
	}

	if payoutStatus != "PENDING" ||
		executionStatus != "PROCESSING" ||
		payoutReference != nil ||
		executionReference != nil {
		t.Fatalf(
			"financial state changed: payout=%s execution=%s payoutRef=%v executionRef=%v",
			payoutStatus,
			executionStatus,
			payoutReference,
			executionReference,
		)
	}

	t.Log(
		"PASS: lease replacement rejected; verified evidence discarded; financial state unchanged",
	)
}
