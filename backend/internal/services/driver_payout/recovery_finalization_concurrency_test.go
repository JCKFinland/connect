package driver_payout

import (
	"context"
	"errors"
	"testing"
	"time"

	postgresrepo "github.com/JCKFinland/connect/backend/internal/repository/postgres"
	"github.com/google/uuid"
)

func TestRecoveryFinalizationRejectsConcurrentLeaseReplacement(t *testing.T) {
	fixture := newSubmissionFixture(t)

	ctx, cancel := context.WithTimeout(
		context.Background(), 20*time.Second,
	)
	defer cancel()

	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(
			context.Background(), 15*time.Second,
		)
		defer cleanupCancel()

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

	lease, acquired, err := postgresrepo.AcquireDriverPayoutRecoveryLease(
		ctx,
		fixture.db,
		fixture.payoutID,
		"concurrency-original-owner",
		90*time.Second,
		5*time.Minute,
	)
	if err != nil || !acquired || lease == nil {
		t.Fatalf("acquire lease: acquired=%v err=%v", acquired, err)
	}

	tx, err := fixture.db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		rollbackCtx, rollbackCancel := context.WithTimeout(
			context.Background(), 5*time.Second,
		)
		defer rollbackCancel()
		_ = tx.Rollback(rollbackCtx)
	}()

	var lockedToken string
	err = tx.QueryRow(ctx, `
        SELECT lease_token::text
        FROM driver_payout_recovery_leases
        WHERE payout_id=$1::uuid
        FOR UPDATE
    `, fixture.payoutID).Scan(&lockedToken)
	if err != nil {
		t.Fatal(err)
	}
	if lockedToken != lease.Token {
		t.Fatal("unexpected token before replacement")
	}

	replacement := uuid.NewString()

	_, err = tx.Exec(ctx, `
        UPDATE driver_payout_recovery_leases
        SET lease_token=$2::uuid,
            lease_owner='concurrency-replacement-owner',
            updated_at=clock_timestamp()
        WHERE payout_id=$1::uuid
          AND lease_token=$3::uuid
    `, fixture.payoutID, replacement, lease.Token)
	if err != nil {
		t.Fatal(err)
	}

	// Identify the backend holding the recovery lease row lock.
	var blockerPID int32
	if err := tx.QueryRow(ctx, `
        SELECT pg_backend_pid()
    `).Scan(&blockerPID); err != nil {
		t.Fatalf("read blocker PID: %v", err)
	}

	result := make(chan error, 1)

	go func() {
		result <- postgresrepo.FinalizeDriverPayoutRecoverySuccess(
			ctx,
			fixture.db,
			fixture.payoutID,
			"TEST_PROVIDER",
			"concurrent-"+uuid.NewString(),
			lease.Token,
		)
	}()

	// Observe a real PostgreSQL lock wait before releasing
	// the transaction that replaces the recovery lease.
	observed := false
	deadline := time.Now().Add(10 * time.Second)

	for time.Now().Before(deadline) {
		var waiting int

		err := fixture.db.QueryRow(ctx, `
            SELECT COUNT(*)
            FROM pg_stat_activity a
            WHERE a.datname = current_database()
              AND a.pid <> pg_backend_pid()
              AND a.wait_event_type = 'Lock'
              AND $1::integer = ANY(pg_blocking_pids(a.pid))
              AND a.query LIKE '%driver_payout_recovery_leases%'
        `, blockerPID).Scan(&waiting)

		if err != nil {
			t.Fatalf("inspect PostgreSQL lock wait: %v", err)
		}

		if waiting > 0 {
			observed = true
			break
		}

		select {
		case err := <-result:
			t.Fatalf(
				"finalizer completed before lock contention: %v",
				err,
			)
		default:
		}

		select {
		case <-ctx.Done():
			t.Fatalf("context expired during lock observation: %v", ctx.Err())
		case <-time.After(20 * time.Millisecond):
		}
	}

	if !observed {
		t.Fatal("finalizer was not observed waiting on lease lock")
	}

	t.Log("PASS: PostgreSQL confirmed finalizer lock wait")

	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit replacement: %v", err)
	}

	select {
	case err := <-result:
		if !errors.Is(
			err,
			postgresrepo.ErrDriverPayoutRecoveryLeaseInvalid,
		) {
			t.Fatalf("expected stale lease rejection, got %v", err)
		}
	case <-ctx.Done():
		t.Fatalf("finalization timed out: %v", ctx.Err())
	}

	var payoutStatus, executionStatus string
	err = fixture.db.QueryRow(ctx, `
        SELECT p.payout_status, e.execution_status
        FROM driver_payouts p
        JOIN driver_payout_executions e
          ON e.payout_id=p.id
        WHERE p.id=$1::uuid
    `, fixture.payoutID).Scan(&payoutStatus, &executionStatus)
	if err != nil {
		t.Fatal(err)
	}

	if payoutStatus != "PENDING" || executionStatus != "PROCESSING" {
		t.Fatalf(
			"financial state changed: payout=%s execution=%s",
			payoutStatus, executionStatus,
		)
	}

	t.Log("PASS: concurrent stale lease rejected")
	t.Log("PASS: payout and execution remain unchanged")
}
