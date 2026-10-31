package driver_payout

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/JCKFinland/connect/backend/internal/payouts"
	postgresrepo "github.com/JCKFinland/connect/backend/internal/repository/postgres"
	"github.com/google/uuid"
)

type recoveryWorkerFinancialState struct {
	payoutStatus       string
	executionStatus    string
	earningStatus      string
	payoutReference    *string
	executionReference *string
	payoutProcessed    bool
	executionConfirmed bool
	earningSettled     bool
}

func readRecoveryWorkerFinancialState(
	t *testing.T,
	fixture submissionFixture,
	earningID string,
) recoveryWorkerFinancialState {
	t.Helper()

	var state recoveryWorkerFinancialState

	err := fixture.db.QueryRow(fixture.ctx, `
        SELECT p.payout_status,
               e.execution_status,
               d.settlement_status,
               p.provider_reference,
               e.provider_reference,
               p.processed_at IS NOT NULL,
               e.provider_confirmed_at IS NOT NULL,
               d.settled_at IS NOT NULL
        FROM driver_payouts p
        JOIN driver_payout_executions e ON e.payout_id=p.id
        JOIN driver_payout_items i ON i.payout_id=p.id
        JOIN driver_earnings d ON d.id=i.earning_id
        WHERE p.id=$1::uuid AND d.id=$2::uuid
    `, fixture.payoutID, earningID).Scan(
		&state.payoutStatus,
		&state.executionStatus,
		&state.earningStatus,
		&state.payoutReference,
		&state.executionReference,
		&state.payoutProcessed,
		&state.executionConfirmed,
		&state.earningSettled,
	)
	if err != nil {
		t.Fatal(err)
	}

	return state
}

func assertRecoveryWorkerFinancialPending(
	t *testing.T,
	state recoveryWorkerFinancialState,
) {
	t.Helper()

	if state.payoutStatus != "PENDING" ||
		state.executionStatus != "PROCESSING" ||
		state.earningStatus != "PROCESSING" ||
		state.payoutReference != nil ||
		state.executionReference != nil ||
		state.payoutProcessed ||
		state.executionConfirmed ||
		state.earningSettled {
		t.Fatalf("financial state unexpectedly changed: %+v", state)
	}
}

func prepareRecoveryWorkerFinancialTest(
	t *testing.T,
) (
	submissionFixture,
	*recoveryFinancialFixture,
	payouts.VerifiedTransferDetails,
	*payoutRecoveryFake,
) {
	t.Helper()

	fixture := newSubmissionFixture(t)

	var driverID, companyID string
	if err := fixture.db.QueryRow(fixture.ctx, `
        SELECT driver_id::text, company_id::text
        FROM driver_payouts
        WHERE id=$1::uuid
    `, fixture.payoutID).Scan(&driverID, &companyID); err != nil {
		t.Fatal(err)
	}

	newRecoveryOrganizationFixture(t, fixture.db, companyID)

	financial := newRecoveryFinancialFixture(
		t, fixture.db, fixture.payoutID, driverID, companyID,
	)

	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(
			context.Background(), 15*time.Second,
		)
		defer cancel()

		if _, err := fixture.db.Exec(ctx, `
            DELETE FROM driver_payout_recovery_leases
            WHERE payout_id=$1::uuid
        `, fixture.payoutID); err != nil {
			t.Errorf("cleanup recovery lease: %v", err)
		}
	})

	execution, claimed, err :=
		postgresrepo.ClaimDriverPayoutExecution(
			fixture.ctx, fixture.db, fixture.payoutID,
		)
	if err != nil || !claimed || execution == nil {
		t.Fatalf("claim execution: claimed=%v err=%v", claimed, err)
	}

	expected, err :=
		postgresrepo.ResolveDriverPayoutRecoveryTransferRequest(
			fixture.ctx, fixture.db, fixture.payoutID,
		)
	if err != nil || expected == nil {
		t.Fatalf("resolve immutable transfer request: %v", err)
	}

	inspector := &payoutRecoveryFake{
		candidates: []postgresrepo.StaleDriverPayoutExecution{
			postgresRecoveryAuthoritativeCandidate(t, fixture),
		},
	}

	return fixture, financial,
		successfulVerifiedReconciliationDetails(*expected), inspector
}

func runFinancialRecoveryWorker(
	t *testing.T,
	fixture submissionFixture,
	inspector *payoutRecoveryFake,
	provider *leaseVerificationProvider,
) []error {
	t.Helper()

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
	if err := worker.ConfigureFinancialFinalization(); err != nil {
		t.Fatal(err)
	}

	var reported []error
	if err := worker.RunOnce(
		fixture.ctx,
		func(err error) {
			reported = append(reported, err)
		},
	); err != nil {
		t.Fatalf("worker RunOnce: %v", err)
	}

	if provider.calls != 1 {
		t.Fatalf("expected one provider lookup, got %d", provider.calls)
	}

	inspector.mu.Lock()
	legacyLookups := inspector.lookups
	inspector.mu.Unlock()

	if legacyLookups != 0 {
		t.Fatalf("legacy inspector called %d times", legacyLookups)
	}

	return reported
}

func TestRecoveryWorkerFinancialFinalizationSuccess(t *testing.T) {
	fixture, financial, details, inspector :=
		prepareRecoveryWorkerFinancialTest(t)

	assertRecoveryWorkerFinancialPending(
		t, readRecoveryWorkerFinancialState(
			t, fixture, financial.earningID,
		),
	)

	details.ProviderReference = "worker-financial-" + uuid.NewString()
	provider := &leaseVerificationProvider{details: details}

	reported := runFinancialRecoveryWorker(
		t, fixture, inspector, provider,
	)
	if len(reported) != 0 {
		t.Fatalf("unexpected recovery errors: %v", reported)
	}

	state := readRecoveryWorkerFinancialState(
		t, fixture, financial.earningID,
	)

	if state.payoutStatus != "PAID" ||
		state.executionStatus != "SUCCEEDED" ||
		state.earningStatus != "PAID" ||
		state.payoutReference == nil ||
		*state.payoutReference != details.ProviderReference ||
		state.executionReference == nil ||
		*state.executionReference != details.ProviderReference ||
		!state.payoutProcessed ||
		!state.executionConfirmed ||
		!state.earningSettled {
		t.Fatalf("verified worker failed financial finalization: %+v", state)
	}

	var lookupCount int64
	var leaseReleased, cooldownActive bool
	err := fixture.db.QueryRow(fixture.ctx, `
        SELECT lookup_count,
               lease_expires_at <= clock_timestamp(),
               next_lookup_at > clock_timestamp()
        FROM driver_payout_recovery_leases
        WHERE payout_id=$1::uuid
    `, fixture.payoutID).Scan(
		&lookupCount, &leaseReleased, &cooldownActive,
	)
	if err != nil {
		t.Fatal(err)
	}
	if lookupCount != 1 || !leaseReleased || !cooldownActive {
		t.Fatalf(
			"incorrect recovery lease lifecycle: count=%d released=%v cooldown=%v",
			lookupCount, leaseReleased, cooldownActive,
		)
	}

	t.Log("PASS: verified worker finalized payout, execution and earning")
	t.Log("PASS: provider references and timestamps persisted")
	t.Log("PASS: recovery lease released; cooldown preserved")
}

func TestRecoveryWorkerFinancialRejectsInvalidProviderEvidence(t *testing.T) {
	fixture, financial, details, inspector :=
		prepareRecoveryWorkerFinancialTest(t)

	// Provider reports success but contradicts the immutable amount.
	details.AmountCents++
	provider := &leaseVerificationProvider{details: details}

	reported := runFinancialRecoveryWorker(
		t, fixture, inspector, provider,
	)
	if len(reported) == 0 {
		t.Fatal("expected invalid provider evidence to be reported")
	}

	assertRecoveryWorkerFinancialPending(
		t, readRecoveryWorkerFinancialState(
			t, fixture, financial.earningID,
		),
	)

	t.Log("PASS: mismatched immutable amount rejected")
	t.Log("PASS: no financial state changed")
}

func TestRecoveryWorkerFinancialRejectsLeaseReplacement(t *testing.T) {
	fixture, financial, details, inspector :=
		prepareRecoveryWorkerFinancialTest(t)

	provider := &leaseVerificationProvider{details: details}

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
			t.Errorf("replace recovery lease: %v", err)
			return
		}
		if tag.RowsAffected() != 1 {
			t.Errorf(
				"expected one replaced recovery lease, got %d",
				tag.RowsAffected(),
			)
		}
	}

	reported := runFinancialRecoveryWorker(
		t, fixture, inspector, provider,
	)

	denied := false
	for _, err := range reported {
		if errors.Is(err, ErrLeaseFencedVerificationDenied) {
			denied = true
		}
	}
	if !denied {
		t.Fatalf("expected lease-fenced denial, reported=%v", reported)
	}

	assertRecoveryWorkerFinancialPending(
		t, readRecoveryWorkerFinancialState(
			t, fixture, financial.earningID,
		),
	)

	t.Log("PASS: replaced recovery lease denied")
	t.Log("PASS: no financial state changed")
}
