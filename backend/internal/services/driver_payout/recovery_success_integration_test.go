package driver_payout

import (
	"context"
	"errors"
	"testing"
	"time"

	postgresrepo "github.com/JCKFinland/connect/backend/internal/repository/postgres"
	"github.com/google/uuid"
)

func TestRecoveryFinalizationSuccessWithValidLease(t *testing.T) {
	fixture := newSubmissionFixture(t)
	ctx := fixture.ctx

	var driverID, companyID string

	err := fixture.db.QueryRow(ctx, `
        SELECT driver_id::text, company_id::text
        FROM driver_payouts
        WHERE id=$1::uuid
    `, fixture.payoutID).Scan(&driverID, &companyID)
	if err != nil {
		t.Fatal(err)
	}

	newRecoveryOrganizationFixture(t, fixture.db, companyID)

	financial := newRecoveryFinancialFixture(
		t, fixture.db, fixture.payoutID, driverID, companyID,
	)

	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(
			context.Background(), 15*time.Second,
		)
		defer cancel()

		if _, err := fixture.db.Exec(cleanupCtx, `
            DELETE FROM driver_payout_recovery_leases
            WHERE payout_id=$1::uuid
        `, fixture.payoutID); err != nil {
			t.Errorf("cleanup recovery lease: %v", err)
		}
	})

	_, claimed, err := postgresrepo.ClaimDriverPayoutExecution(
		ctx, fixture.db, fixture.payoutID,
	)
	if err != nil || !claimed {
		t.Fatalf("claim execution: claimed=%v err=%v", claimed, err)
	}

	lease, acquired, err :=
		postgresrepo.AcquireDriverPayoutRecoveryLease(
			ctx,
			fixture.db,
			fixture.payoutID,
			"recovery-success-integration-test",
			90*time.Second,
			5*time.Minute,
		)

	if err != nil || !acquired || lease == nil {
		t.Fatalf(
			"acquire recovery lease: acquired=%v err=%v",
			acquired, err,
		)
	}

	providerReference := "verified-recovery-" + uuid.NewString()

	err = postgresrepo.FinalizeDriverPayoutRecoverySuccess(
		ctx,
		fixture.db,
		fixture.payoutID,
		"TEST_PROVIDER",
		providerReference,
		lease.Token,
	)
	if err != nil {
		t.Fatalf("finalize recovery success: %v", err)
	}

	var payoutStatus, executionStatus, earningStatus string
	var payoutReference, executionReference string
	var payoutProcessed, executionConfirmed, earningSettled bool

	err = fixture.db.QueryRow(ctx, `
        SELECT
            p.payout_status,
            e.execution_status,
            d.settlement_status,
            p.provider_reference,
            e.provider_reference,
            p.processed_at IS NOT NULL,
            e.provider_confirmed_at IS NOT NULL,
            d.settled_at IS NOT NULL
        FROM driver_payouts p
        JOIN driver_payout_executions e
          ON e.payout_id=p.id
        JOIN driver_payout_items i
          ON i.payout_id=p.id
        JOIN driver_earnings d
          ON d.id=i.earning_id
        WHERE p.id=$1::uuid
          AND d.id=$2::uuid
    `, fixture.payoutID, financial.earningID).Scan(
		&payoutStatus,
		&executionStatus,
		&earningStatus,
		&payoutReference,
		&executionReference,
		&payoutProcessed,
		&executionConfirmed,
		&earningSettled,
	)
	if err != nil {
		t.Fatal(err)
	}

	if payoutStatus != "PAID" ||
		executionStatus != "SUCCEEDED" ||
		earningStatus != "PAID" ||
		payoutReference != providerReference ||
		executionReference != providerReference ||
		!payoutProcessed ||
		!executionConfirmed ||
		!earningSettled {
		t.Fatalf(
			"incorrect recovery finalization: payout=%s execution=%s earning=%s",
			payoutStatus, executionStatus, earningStatus,
		)
	}

	type recoverySnapshot struct {
		payoutStatus       string
		executionStatus    string
		earningStatus      string
		payoutReference    string
		executionReference string
		payoutProcessed    string
		executionConfirmed string
		earningSettled     string
	}

	readSnapshot := func() recoverySnapshot {
		t.Helper()

		var snapshot recoverySnapshot

		err := fixture.db.QueryRow(ctx, `
            SELECT
                p.payout_status,
                e.execution_status,
                d.settlement_status,
                p.provider_reference,
                e.provider_reference,
                p.processed_at::text,
                e.provider_confirmed_at::text,
                d.settled_at::text
            FROM driver_payouts p
            JOIN driver_payout_executions e
              ON e.payout_id=p.id
            JOIN driver_payout_items i
              ON i.payout_id=p.id
            JOIN driver_earnings d
              ON d.id=i.earning_id
            WHERE p.id=$1::uuid
              AND d.id=$2::uuid
        `, fixture.payoutID, financial.earningID).Scan(
			&snapshot.payoutStatus,
			&snapshot.executionStatus,
			&snapshot.earningStatus,
			&snapshot.payoutReference,
			&snapshot.executionReference,
			&snapshot.payoutProcessed,
			&snapshot.executionConfirmed,
			&snapshot.earningSettled,
		)
		if err != nil {
			t.Fatal(err)
		}

		return snapshot
	}

	original := readSnapshot()

	err = postgresrepo.FinalizeDriverPayoutRecoverySuccess(
		ctx,
		fixture.db,
		fixture.payoutID,
		"TEST_PROVIDER",
		providerReference,
		lease.Token,
	)
	if err != nil {
		t.Fatalf("exact recovery replay rejected: %v", err)
	}

	if after := readSnapshot(); after != original {
		t.Fatalf(
			"exact replay modified financial state: before=%+v after=%+v",
			original, after,
		)
	}

	err = postgresrepo.FinalizeDriverPayoutRecoverySuccess(
		ctx,
		fixture.db,
		fixture.payoutID,
		"TEST_PROVIDER",
		"different-"+uuid.NewString(),
		lease.Token,
	)
	if err == nil {
		t.Fatal("different provider reference unexpectedly accepted")
	}

	if after := readSnapshot(); after != original {
		t.Fatalf(
			"reference conflict modified financial state: before=%+v after=%+v",
			original, after,
		)
	}

	err = postgresrepo.FinalizeDriverPayoutRecoverySuccess(
		ctx,
		fixture.db,
		fixture.payoutID,
		"TEST_PROVIDER",
		providerReference,
		uuid.NewString(),
	)
	if !errors.Is(
		err,
		postgresrepo.ErrDriverPayoutRecoveryLeaseInvalid,
	) {
		t.Fatalf("expected invalid lease denial on replay, got %v", err)
	}

	if after := readSnapshot(); after != original {
		t.Fatalf(
			"invalid lease replay modified financial state: before=%+v after=%+v",
			original, after,
		)
	}

	t.Log("PASS: recovery replay idempotent")
	t.Log("PASS: conflicting reference rejected")
	t.Log("PASS: invalid lease replay rejected")

	t.Log("PASS: payout PAID")
	t.Log("PASS: execution SUCCEEDED")
	t.Log("PASS: earning PAID")
	t.Log("PASS: matching provider references")
	t.Log("PASS: finalization timestamps populated")
}
