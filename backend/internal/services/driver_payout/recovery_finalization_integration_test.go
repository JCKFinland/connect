package driver_payout

import (
	"context"
	"errors"
	"testing"
	"time"

	postgresrepo "github.com/JCKFinland/connect/backend/internal/repository/postgres"
	"github.com/google/uuid"
)

func TestRecoveryFinalizationRejectsInvalidLeases(t *testing.T) {
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

	_, claimed, err := postgresrepo.ClaimDriverPayoutExecution(
		fixture.ctx, fixture.db, fixture.payoutID,
	)
	if err != nil || !claimed {
		t.Fatalf("claim execution: claimed=%v err=%v", claimed, err)
	}

	finalize := func(token string) error {
		return postgresrepo.FinalizeDriverPayoutRecoverySuccess(
			fixture.ctx,
			fixture.db,
			fixture.payoutID,
			"TEST_PROVIDER",
			"verified-recovery-"+uuid.NewString(),
			token,
		)
	}

	assertUnchanged := func(t *testing.T) {
		t.Helper()

		var payoutStatus, executionStatus string
		var payoutReference, executionReference *string
		var payoutProcessed, executionConfirmed bool
		var attempts int

		err := fixture.db.QueryRow(fixture.ctx, `
            SELECT
                p.payout_status,
                e.execution_status,
                p.provider_reference,
                e.provider_reference,
                p.processed_at IS NOT NULL,
                e.provider_confirmed_at IS NOT NULL,
                e.attempt_count
            FROM driver_payouts p
            JOIN driver_payout_executions e
              ON e.payout_id=p.id
            WHERE p.id=$1::uuid
        `, fixture.payoutID).Scan(
			&payoutStatus,
			&executionStatus,
			&payoutReference,
			&executionReference,
			&payoutProcessed,
			&executionConfirmed,
			&attempts,
		)
		if err != nil {
			t.Fatal(err)
		}

		if payoutStatus != "PENDING" ||
			executionStatus != "PROCESSING" ||
			payoutReference != nil ||
			executionReference != nil ||
			payoutProcessed ||
			executionConfirmed ||
			attempts != 1 {
			t.Fatalf(
				"financial state changed: payout=%s execution=%s "+
					"payoutRef=%v executionRef=%v processed=%v "+
					"confirmed=%v attempts=%d",
				payoutStatus,
				executionStatus,
				payoutReference,
				executionReference,
				payoutProcessed,
				executionConfirmed,
				attempts,
			)
		}
	}

	assertDenied := func(t *testing.T, token string) {
		t.Helper()

		err := finalize(token)
		if !errors.Is(
			err, postgresrepo.ErrDriverPayoutRecoveryLeaseInvalid,
		) {
			t.Fatalf("expected recovery lease denial, got %v", err)
		}

		assertUnchanged(t)
	}

	t.Run("malformed_token", func(t *testing.T) {
		assertDenied(t, "not-a-uuid")
	})

	t.Run("missing_lease", func(t *testing.T) {
		assertDenied(t, uuid.NewString())
	})

	lease, acquired, err :=
		postgresrepo.AcquireDriverPayoutRecoveryLease(
			fixture.ctx,
			fixture.db,
			fixture.payoutID,
			"recovery-finalization-test",
			90*time.Second,
			5*time.Minute,
		)
	if err != nil || !acquired || lease == nil {
		t.Fatalf("acquire lease: acquired=%v err=%v", acquired, err)
	}

	t.Run("incorrect_token", func(t *testing.T) {
		assertDenied(t, uuid.NewString())
	})

	var replacement string

	t.Run("replaced_token", func(t *testing.T) {
		replacement = uuid.NewString()

		tag, err := fixture.db.Exec(fixture.ctx, `
            UPDATE driver_payout_recovery_leases
            SET lease_token=$2::uuid,
                lease_owner='replacement-test-owner',
                updated_at=clock_timestamp()
            WHERE payout_id=$1::uuid
              AND lease_token=$3::uuid
        `, fixture.payoutID, replacement, lease.Token)

		if err != nil || tag.RowsAffected() != 1 {
			t.Fatalf("replace lease: rows=%d err=%v",
				tag.RowsAffected(), err)
		}

		assertDenied(t, lease.Token)
	})

	t.Run("expired_lease", func(t *testing.T) {
		tag, err := fixture.db.Exec(fixture.ctx, `
            UPDATE driver_payout_recovery_leases
            SET lease_expires_at=clock_timestamp()-interval '1 second',
                updated_at=clock_timestamp()
            WHERE payout_id=$1::uuid
        `, fixture.payoutID)

		if err != nil || tag.RowsAffected() != 1 {
			t.Fatalf("expire lease: rows=%d err=%v",
				tag.RowsAffected(), err)
		}

		assertDenied(t, replacement)
	})

	t.Log("PASS: invalid leases rejected; financial state unchanged")
}
