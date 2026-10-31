package driver_payout

import (
	"context"
	"testing"
	"time"

	postgresrepo "github.com/JCKFinland/connect/backend/internal/repository/postgres"
	"github.com/google/uuid"
)

func TestRecoveryFinalizationHoldsLeaseAgainstConcurrentReplacement(t *testing.T) {
	fixture := newSubmissionFixture(t)

	ctx, cancel := context.WithTimeout(
		context.Background(), 30*time.Second,
	)
	defer cancel()

	t.Cleanup(func() {
		cleanupCtx, stop := context.WithTimeout(
			context.Background(), 15*time.Second,
		)
		defer stop()
		if _, err := fixture.db.Exec(cleanupCtx, `
            DELETE FROM driver_payout_recovery_leases
            WHERE payout_id=$1::uuid
        `, fixture.payoutID); err != nil {
			t.Errorf("cleanup recovery lease: %v", err)
		}
	})

	var driverID, companyID string
	if err := fixture.db.QueryRow(ctx, `
        SELECT driver_id::text, company_id::text
        FROM driver_payouts WHERE id=$1::uuid
    `, fixture.payoutID).Scan(&driverID, &companyID); err != nil {
		t.Fatal(err)
	}

	newRecoveryOrganizationFixture(t, fixture.db, companyID)
	financial := newRecoveryFinancialFixture(
		t, fixture.db, fixture.payoutID, driverID, companyID,
	)

	_, claimed, err := postgresrepo.ClaimDriverPayoutExecution(
		ctx, fixture.db, fixture.payoutID,
	)
	if err != nil || !claimed {
		t.Fatalf("claim execution: claimed=%v err=%v", claimed, err)
	}

	lease, acquired, err := postgresrepo.AcquireDriverPayoutRecoveryLease(
		ctx, fixture.db, fixture.payoutID,
		"reverse-concurrency-owner",
		90*time.Second, 5*time.Minute,
	)
	if err != nil || !acquired || lease == nil {
		t.Fatalf("acquire lease: acquired=%v err=%v", acquired, err)
	}

	blocker, err := fixture.db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		cleanupCtx, stop := context.WithTimeout(
			context.Background(), 5*time.Second,
		)
		defer stop()
		_ = blocker.Rollback(cleanupCtx)
	}()

	var payoutID string
	if err := blocker.QueryRow(ctx, `
        SELECT id::text FROM driver_payouts
        WHERE id=$1::uuid FOR UPDATE
    `, fixture.payoutID).Scan(&payoutID); err != nil {
		t.Fatal(err)
	}

	var blockerPID int32
	if err := blocker.QueryRow(ctx, `
        SELECT pg_backend_pid()
    `).Scan(&blockerPID); err != nil {
		t.Fatal(err)
	}

	providerReference := "reverse-" + uuid.NewString()
	finalizationResult := make(chan error, 1)

	go func() {
		finalizationResult <- postgresrepo.FinalizeDriverPayoutRecoverySuccess(
			ctx, fixture.db, fixture.payoutID,
			"TEST_PROVIDER", providerReference, lease.Token,
		)
	}()

	// The finalizer must have acquired the lease row lock
	// before waiting for the payout row lock.
	var finalizerPID int32
	deadline := time.Now().Add(10 * time.Second)

	for time.Now().Before(deadline) {
		err := fixture.db.QueryRow(ctx, `
            SELECT COALESCE((
                SELECT a.pid
                FROM pg_stat_activity a
                WHERE a.datname=current_database()
                  AND a.wait_event_type='Lock'
                  AND $1::integer=ANY(pg_blocking_pids(a.pid))
                  AND a.query LIKE '%FROM driver_payouts%'
                LIMIT 1
            ), 0)
        `, blockerPID).Scan(&finalizerPID)

		if err != nil {
			t.Fatal(err)
		}
		if finalizerPID != 0 {
			break
		}

		select {
		case err := <-finalizationResult:
			t.Fatalf("finalizer completed before payout lock wait: %v", err)
		default:
		}

		time.Sleep(20 * time.Millisecond)
	}

	if finalizerPID == 0 {
		t.Fatal("finalizer was not observed waiting on payout lock")
	}

	t.Log("PASS: finalizer holds lease while waiting on payout")

	replacementResult := make(chan error, 1)
	replacementToken := uuid.NewString()

	go func() {
		_, err := fixture.db.Exec(ctx, `
            UPDATE driver_payout_recovery_leases
            SET lease_token=$2::uuid,
                updated_at=clock_timestamp()
            WHERE payout_id=$1::uuid
              AND lease_token=$3::uuid
        `, fixture.payoutID, replacementToken, lease.Token)
		replacementResult <- err
	}()

	// The competing update must wait on the finalizer's lease lock.
	replacementBlocked := false
	deadline = time.Now().Add(10 * time.Second)

	for time.Now().Before(deadline) {
		var count int

		err := fixture.db.QueryRow(ctx, `
            SELECT COUNT(*)
            FROM pg_stat_activity a
            WHERE a.datname=current_database()
              AND a.wait_event_type='Lock'
              AND $1::integer=ANY(pg_blocking_pids(a.pid))
              AND a.query LIKE '%driver_payout_recovery_leases%'
        `, finalizerPID).Scan(&count)

		if err != nil {
			t.Fatal(err)
		}

		if count > 0 {
			replacementBlocked = true
			break
		}

		select {
		case err := <-replacementResult:
			t.Fatalf("replacement completed before finalization: %v", err)
		default:
		}

		time.Sleep(20 * time.Millisecond)
	}

	if !replacementBlocked {
		t.Fatal("replacement was not blocked by finalizer")
	}

	t.Log("PASS: competing lease replacement blocked")

	if err := blocker.Commit(ctx); err != nil {
		t.Fatalf("release payout lock: %v", err)
	}

	select {
	case err := <-finalizationResult:
		if err != nil {
			t.Fatalf("financial finalization failed: %v", err)
		}
	case <-ctx.Done():
		t.Fatalf("finalization timeout: %v", ctx.Err())
	}

	select {
	case err := <-replacementResult:
		if err != nil {
			t.Fatalf("competing lease update failed: %v", err)
		}
	case <-ctx.Done():
		t.Fatalf("replacement timeout: %v", ctx.Err())
	}

	var payoutStatus, executionStatus, earningStatus string
	err = fixture.db.QueryRow(ctx, `
        SELECT p.payout_status,
               x.execution_status,
               e.settlement_status
        FROM driver_payouts p
        JOIN driver_payout_executions x ON x.payout_id=p.id
        JOIN driver_payout_items i ON i.payout_id=p.id
        JOIN driver_earnings e ON e.id=i.earning_id
        WHERE p.id=$1::uuid AND e.id=$2::uuid
    `, fixture.payoutID, financial.earningID).Scan(
		&payoutStatus, &executionStatus, &earningStatus,
	)
	if err != nil {
		t.Fatal(err)
	}

	if payoutStatus != "PAID" ||
		executionStatus != "SUCCEEDED" ||
		earningStatus != "PAID" {
		t.Fatalf(
			"unexpected financial state: payout=%s execution=%s earning=%s",
			payoutStatus, executionStatus, earningStatus,
		)
	}

	t.Log("PASS: payout PAID, execution SUCCEEDED, earning PAID")
	t.Log("PASS: finalization completed before lease replacement")
}
