package driver_payout

import (
	"context"
	"sync"
	"testing"
	"time"

	postgresrepo "github.com/JCKFinland/connect/backend/internal/repository/postgres"
)

func TestDriverPayoutRecoveryLeaseConcurrency(t *testing.T) {
	fixture := newSubmissionFixture(t)

	// Ensure lease cleanup occurs before the payout fixture cleanup.
	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(
			context.Background(), 15*time.Second,
		)
		defer cancel()

		_, err := fixture.db.Exec(cleanupCtx, `
            DELETE FROM driver_payout_recovery_leases
            WHERE payout_id = $1::uuid
        `, fixture.payoutID)

		if err != nil {
			t.Errorf("cleanup recovery lease: %v", err)
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

	const (
		callers       = 8
		leaseDuration = 90 * time.Second
		cooldown      = 5 * time.Minute
	)

	type acquisition struct {
		lease    *postgresrepo.DriverPayoutRecoveryLease
		acquired bool
		err      error
	}

	results := make([]acquisition, callers)

	var wg sync.WaitGroup
	start := make(chan struct{})

	for i := 0; i < callers; i++ {
		wg.Add(1)

		go func(index int) {
			defer wg.Done()
			<-start

			results[index].lease,
				results[index].acquired,
				results[index].err =
				postgresrepo.AcquireDriverPayoutRecoveryLease(
					fixture.ctx,
					fixture.db,
					fixture.payoutID,
					"recovery-worker",
					leaseDuration,
					cooldown,
				)
		}(i)
	}

	close(start)
	wg.Wait()

	winners := 0
	var first *postgresrepo.DriverPayoutRecoveryLease

	for i, result := range results {
		if result.err != nil {
			t.Fatalf(
				"concurrent caller %d: %v",
				i, result.err,
			)
		}

		if result.acquired {
			winners++
			first = result.lease

			if first == nil {
				t.Fatal("acquisition returned nil lease")
			}
		} else if result.lease != nil {
			t.Fatalf(
				"losing caller %d received lease",
				i,
			)
		}
	}

	if winners != 1 {
		t.Fatalf(
			"expected exactly one lease owner, got %d",
			winners,
		)
	}

	var persistedToken string
	var persistedOwner string
	var lookupCount int64

	err = fixture.db.QueryRow(fixture.ctx, `
        SELECT
            lease_token::text,
            lease_owner,
            lookup_count
        FROM driver_payout_recovery_leases
        WHERE payout_id = $1::uuid
    `, fixture.payoutID).Scan(
		&persistedToken,
		&persistedOwner,
		&lookupCount,
	)

	if err != nil {
		t.Fatal(err)
	}

	if persistedToken != first.Token ||
		persistedOwner != first.Owner ||
		lookupCount != 1 {
		t.Fatalf(
			"invalid persisted lease: token=%s owner=%s count=%d",
			persistedToken, persistedOwner, lookupCount,
		)
	}

	// Active lease cannot be acquired again.
	_, acquired, err :=
		postgresrepo.AcquireDriverPayoutRecoveryLease(
			fixture.ctx,
			fixture.db,
			fixture.payoutID,
			"second-worker",
			leaseDuration,
			cooldown,
		)

	if err != nil || acquired {
		t.Fatalf(
			"active lease reacquired: acquired=%v err=%v",
			acquired, err,
		)
	}

	// Release preserves durable cooldown.
	released, err :=
		postgresrepo.ReleaseDriverPayoutRecoveryLease(
			fixture.ctx,
			fixture.db,
			fixture.payoutID,
			first.Token,
		)

	if err != nil || !released {
		t.Fatalf(
			"release: released=%v err=%v",
			released, err,
		)
	}

	_, acquired, err =
		postgresrepo.AcquireDriverPayoutRecoveryLease(
			fixture.ctx,
			fixture.db,
			fixture.payoutID,
			"cooldown-worker",
			leaseDuration,
			cooldown,
		)

	if err != nil || acquired {
		t.Fatalf(
			"durable cooldown bypassed: acquired=%v err=%v",
			acquired, err,
		)
	}

	// Advance only this synthetic fixture's coordination deadlines.
	// No sleep and no modification of financial records.
	_, err = fixture.db.Exec(fixture.ctx, `
        UPDATE driver_payout_recovery_leases
        SET
            lease_expires_at = clock_timestamp() - interval '1 second',
            last_lookup_at = clock_timestamp() - interval '2 seconds',
            next_lookup_at = clock_timestamp() - interval '1 second'
        WHERE payout_id = $1::uuid
    `, fixture.payoutID)

	if err != nil {
		t.Fatal(err)
	}

	second, acquired, err :=
		postgresrepo.AcquireDriverPayoutRecoveryLease(
			fixture.ctx,
			fixture.db,
			fixture.payoutID,
			"replacement-worker",
			leaseDuration,
			cooldown,
		)

	if err != nil || !acquired || second == nil {
		t.Fatalf(
			"expired lease takeover: lease=%+v acquired=%v err=%v",
			second, acquired, err,
		)
	}

	if second.Token == first.Token {
		t.Fatal("lease takeover reused fencing token")
	}

	// Old owner cannot release the new owner's lease.
	released, err =
		postgresrepo.ReleaseDriverPayoutRecoveryLease(
			fixture.ctx,
			fixture.db,
			fixture.payoutID,
			first.Token,
		)

	if err != nil || released {
		t.Fatalf(
			"stale token released lease: released=%v err=%v",
			released, err,
		)
	}

	var currentToken string
	err = fixture.db.QueryRow(fixture.ctx, `
        SELECT lease_token::text
        FROM driver_payout_recovery_leases
        WHERE payout_id = $1::uuid
    `, fixture.payoutID).Scan(&currentToken)

	if err != nil || currentToken != second.Token {
		t.Fatalf(
			"fencing token changed: token=%s err=%v",
			currentToken, err,
		)
	}

	released, err =
		postgresrepo.ReleaseDriverPayoutRecoveryLease(
			fixture.ctx,
			fixture.db,
			fixture.payoutID,
			second.Token,
		)

	if err != nil || !released {
		t.Fatalf(
			"new owner release: released=%v err=%v",
			released, err,
		)
	}

	// Make cooldown due, then make the payout ineligible.
	_, err = fixture.db.Exec(fixture.ctx, `
        UPDATE driver_payout_recovery_leases
        SET
            last_lookup_at =
                clock_timestamp() - interval '2 seconds',
            next_lookup_at =
                clock_timestamp() - interval '1 second'
        WHERE payout_id = $1::uuid
    `, fixture.payoutID)

	if err != nil {
		t.Fatal(err)
	}

	_, err = fixture.db.Exec(fixture.ctx, `
        UPDATE driver_payout_executions
        SET provider_reference = 'test-confirmed-reference'
        WHERE payout_id = $1::uuid
    `, fixture.payoutID)

	if err != nil {
		t.Fatal(err)
	}

	_, acquired, err =
		postgresrepo.AcquireDriverPayoutRecoveryLease(
			fixture.ctx,
			fixture.db,
			fixture.payoutID,
			"ineligible-worker",
			leaseDuration,
			cooldown,
		)

	if err != nil || acquired {
		t.Fatalf(
			"ineligible execution acquired lease: acquired=%v err=%v",
			acquired, err,
		)
	}

	err = fixture.db.QueryRow(fixture.ctx, `
        SELECT lookup_count
        FROM driver_payout_recovery_leases
        WHERE payout_id = $1::uuid
    `, fixture.payoutID).Scan(&lookupCount)

	if err != nil || lookupCount != 2 {
		t.Fatalf(
			"unexpected lookup count: count=%d err=%v",
			lookupCount, err,
		)
	}

	t.Log(
		"PASS: single concurrent owner;",
		"durable cooldown;",
		"expired lease takeover;",
		"fencing-token protection;",
		"ineligible execution rejected",
	)
}
