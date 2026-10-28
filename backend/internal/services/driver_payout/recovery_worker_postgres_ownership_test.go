package driver_payout

import (
	"context"
	"testing"
	"time"

	postgresrepo "github.com/JCKFinland/connect/backend/internal/repository/postgres"
)

func TestPostgresPayoutRecoveryWorkerRespectsExistingLease(
	t *testing.T,
) {
	fixture := newSubmissionFixture(t)

	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(
			context.Background(),
			15*time.Second,
		)
		defer cancel()

		_, err := fixture.db.Exec(ctx, `
            DELETE FROM driver_payout_recovery_leases
            WHERE payout_id = $1::uuid
        `, fixture.payoutID)
		if err != nil {
			t.Errorf("recovery lease cleanup: %v", err)
		}
	})

	execution, claimed, err :=
		postgresrepo.ClaimDriverPayoutExecution(
			fixture.ctx,
			fixture.db,
			fixture.payoutID,
		)
	if err != nil || !claimed || execution == nil {
		t.Fatalf(
			"claim execution: claimed=%v execution=%+v err=%v",
			claimed, execution, err,
		)
	}

	candidate := postgresRecoveryAuthoritativeCandidate(
		t, fixture,
	)

	lease, acquired, err :=
		postgresrepo.AcquireDriverPayoutRecoveryLease(
			fixture.ctx,
			fixture.db,
			fixture.payoutID,
			"independent-existing-owner",
			45*time.Second,
			5*time.Minute,
		)
	if err != nil || !acquired || lease == nil {
		t.Fatalf(
			"establish independent lease: acquired=%v lease=%+v err=%v",
			acquired, lease, err,
		)
	}

	fake := &payoutRecoveryFake{
		candidates: []postgresrepo.StaleDriverPayoutExecution{
			candidate,
		},
	}

	worker, err := NewPostgresPayoutRecoveryWorker(
		fixture.db,
		fake,
		time.Minute,
		10,
	)
	if err != nil {
		t.Fatal(err)
	}

	if err := worker.RunOnce(context.Background(), nil); err != nil {
		t.Fatalf("worker run: %v", err)
	}

	fake.mu.Lock()
	lookups := fake.lookups
	fake.mu.Unlock()

	if lookups != 0 {
		t.Fatalf(
			"worker contacted provider without ownership: %d",
			lookups,
		)
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

	if persistedToken != lease.Token ||
		persistedOwner != lease.Owner ||
		!active ||
		!cooldownFuture ||
		lookupCount != 1 {
		t.Fatalf(
			"existing lease changed: token=%s owner=%s active=%v cooldown=%v count=%d",
			persistedToken,
			persistedOwner,
			active,
			cooldownFuture,
			lookupCount,
		)
	}

	var status string
	var attempts int
	var providerReference *string

	err = fixture.db.QueryRow(fixture.ctx, `
        SELECT
            execution_status,
            attempt_count,
            provider_reference
        FROM driver_payout_executions
        WHERE payout_id = $1::uuid
    `, fixture.payoutID).Scan(
		&status,
		&attempts,
		&providerReference,
	)
	if err != nil {
		t.Fatal(err)
	}

	if status != "PROCESSING" ||
		attempts != 1 ||
		providerReference != nil {
		t.Fatalf(
			"financial state changed: status=%s attempts=%d reference=%v",
			status,
			attempts,
			providerReference,
		)
	}

	t.Log(
		"PASS: existing lease retained; zero provider lookups; financial state unchanged",
	)
}
