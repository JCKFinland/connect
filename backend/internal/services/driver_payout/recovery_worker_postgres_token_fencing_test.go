package driver_payout

import (
	"context"
	"testing"
	"time"

	postgresrepo "github.com/JCKFinland/connect/backend/internal/repository/postgres"
	"github.com/google/uuid"
)

func TestPostgresPayoutRecoveryLeaseRejectsReplacedToken(
	t *testing.T,
) {
	fixture := newSubmissionFixture(t)

	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(
			context.Background(), 15*time.Second,
		)
		defer cancel()

		_, err := fixture.db.Exec(ctx, `
            DELETE FROM driver_payout_recovery_leases
            WHERE payout_id = $1::uuid
        `, fixture.payoutID)
		if err != nil {
			t.Errorf("lease cleanup: %v", err)
		}
	})

	execution, claimed, err :=
		postgresrepo.ClaimDriverPayoutExecution(
			fixture.ctx, fixture.db, fixture.payoutID,
		)
	if err != nil || !claimed || execution == nil {
		t.Fatalf(
			"claim: claimed=%v execution=%+v err=%v",
			claimed, execution, err,
		)
	}

	lease, acquired, err :=
		postgresrepo.AcquireDriverPayoutRecoveryLease(
			fixture.ctx,
			fixture.db,
			fixture.payoutID,
			"original-owner",
			45*time.Second,
			5*time.Minute,
		)
	if err != nil || !acquired || lease == nil {
		t.Fatalf(
			"acquire: acquired=%v lease=%+v err=%v",
			acquired, lease, err,
		)
	}

	owned, err :=
		postgresrepo.ValidateDriverPayoutRecoveryLease(
			fixture.ctx,
			fixture.db,
			fixture.payoutID,
			lease.Token,
			35*time.Second,
		)
	if err != nil || !owned {
		t.Fatalf(
			"original token should own lease: owned=%v err=%v",
			owned, err,
		)
	}

	replacementToken := uuid.NewString()

	tag, err := fixture.db.Exec(fixture.ctx, `
        UPDATE driver_payout_recovery_leases
        SET lease_token = $2::uuid,
            lease_owner = 'replacement-owner',
            updated_at = clock_timestamp()
        WHERE payout_id = $1::uuid
          AND lease_token = $3::uuid
    `, fixture.payoutID, replacementToken, lease.Token)
	if err != nil {
		t.Fatal(err)
	}
	if tag.RowsAffected() != 1 {
		t.Fatalf(
			"expected one replaced lease, got %d",
			tag.RowsAffected(),
		)
	}

	owned, err =
		postgresrepo.ValidateDriverPayoutRecoveryLease(
			fixture.ctx,
			fixture.db,
			fixture.payoutID,
			lease.Token,
			35*time.Second,
		)
	if err != nil {
		t.Fatal(err)
	}
	if owned {
		t.Fatal("stale token retained ownership")
	}

	replacementOwned, err :=
		postgresrepo.ValidateDriverPayoutRecoveryLease(
			fixture.ctx,
			fixture.db,
			fixture.payoutID,
			replacementToken,
			35*time.Second,
		)
	if err != nil || !replacementOwned {
		t.Fatalf(
			"replacement token should own lease: owned=%v err=%v",
			replacementOwned, err,
		)
	}

	released, err :=
		postgresrepo.ReleaseDriverPayoutRecoveryLease(
			fixture.ctx,
			fixture.db,
			fixture.payoutID,
			lease.Token,
		)
	if err != nil {
		t.Fatal(err)
	}
	if released {
		t.Fatal("stale token released replacement lease")
	}

	var persistedToken string
	var persistedOwner string
	var active bool
	var cooldownFuture bool
	var lookupCount int64

	err = fixture.db.QueryRow(fixture.ctx, `
        SELECT
            lease_token::text,
            lease_owner,
            lease_expires_at > clock_timestamp(),
            next_lookup_at > clock_timestamp(),
            lookup_count
        FROM driver_payout_recovery_leases
        WHERE payout_id = $1::uuid
    `, fixture.payoutID).Scan(
		&persistedToken,
		&persistedOwner,
		&active,
		&cooldownFuture,
		&lookupCount,
	)
	if err != nil {
		t.Fatal(err)
	}

	if persistedToken != replacementToken ||
		persistedOwner != "replacement-owner" ||
		!active ||
		!cooldownFuture ||
		lookupCount != 1 {
		t.Fatalf(
			"replacement lease changed: token=%s owner=%s active=%v cooldown=%v count=%d",
			persistedToken,
			persistedOwner,
			active,
			cooldownFuture,
			lookupCount,
		)
	}

	var status string
	var attempts int
	var reference *string

	err = fixture.db.QueryRow(fixture.ctx, `
        SELECT execution_status, attempt_count, provider_reference
        FROM driver_payout_executions
        WHERE payout_id = $1::uuid
    `, fixture.payoutID).Scan(
		&status, &attempts, &reference,
	)
	if err != nil {
		t.Fatal(err)
	}

	if status != "PROCESSING" ||
		attempts != 1 ||
		reference != nil {
		t.Fatalf(
			"financial state changed: status=%s attempts=%d reference=%v",
			status, attempts, reference,
		)
	}

	t.Log(
		"PASS: replaced token rejected; replacement lease preserved; financial state unchanged",
	)
}
