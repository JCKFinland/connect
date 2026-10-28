package driver_payout

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	postgresrepo "github.com/JCKFinland/connect/backend/internal/repository/postgres"
)

// TestPostgresPayoutRecoveryWorkerDistributedCoordination
// verifies coordination across independent worker instances.
//
// All database mutations are restricted to the isolated test
// fixture and its recovery coordination row.
func TestPostgresPayoutRecoveryWorkerDistributedCoordination(
	t *testing.T,
) {
	fixture := newSubmissionFixture(t)

	// Registered after fixture cleanup so this executes first.
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

	fake := &payoutRecoveryFake{
		candidates: []postgresrepo.StaleDriverPayoutExecution{
			postgresRecoveryAuthoritativeCandidate(t, fixture),
		},
		entered: make(chan struct{}, 1),
		release: make(chan struct{}),
	}

	first, err := NewPostgresPayoutRecoveryWorker(
		fixture.db,
		fake,
		time.Minute,
		10,
	)
	if err != nil {
		t.Fatal(err)
	}

	second, err := NewPostgresPayoutRecoveryWorker(
		fixture.db,
		fake,
		time.Minute,
		10,
	)
	if err != nil {
		t.Fatal(err)
	}

	if first.owner == second.owner {
		t.Fatal("independent workers share ownership identity")
	}

	firstDone := make(chan error, 1)

	go func() {
		firstDone <- first.RunOnce(
			context.Background(),
			nil,
		)
	}()

	select {
	case <-fake.entered:
	case <-time.After(10 * time.Second):
		t.Fatal("first provider lookup did not begin")
	}

	// While the first lookup holds its lease, the second worker
	// must not contact the provider.
	if err := second.RunOnce(
		context.Background(),
		nil,
	); err != nil {
		t.Fatal(err)
	}

	fake.mu.Lock()
	active := fake.active
	lookups := fake.lookups
	fake.mu.Unlock()

	if active != 1 || lookups != 1 {
		t.Fatalf(
			"distributed exclusivity failed: active=%d lookups=%d",
			active, lookups,
		)
	}

	close(fake.release)

	select {
	case err := <-firstDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("first worker failed to finish")
	}

	// Releasing ownership must not reset durable cooldown.
	third, err := NewPostgresPayoutRecoveryWorker(
		fixture.db,
		fake,
		time.Minute,
		10,
	)
	if err != nil {
		t.Fatal(err)
	}

	if err := third.RunOnce(
		context.Background(),
		nil,
	); err != nil {
		t.Fatal(err)
	}

	fake.mu.Lock()
	lookups = fake.lookups
	maxActive := fake.maxActive
	fake.mu.Unlock()

	if lookups != 1 || maxActive != 1 {
		t.Fatalf(
			"cooldown or exclusivity failed: lookups=%d max=%d",
			lookups, maxActive,
		)
	}

	var lookupCount int64
	var cooldownFuture bool
	var leaseReleased bool

	err = fixture.db.QueryRow(fixture.ctx, `
        SELECT
            lookup_count,
            next_lookup_at > clock_timestamp(),
            lease_expires_at <= clock_timestamp()
        FROM driver_payout_recovery_leases
        WHERE payout_id = $1::uuid
    `, fixture.payoutID).Scan(
		&lookupCount,
		&cooldownFuture,
		&leaseReleased,
	)

	if err != nil {
		t.Fatal(err)
	}

	if lookupCount != 1 ||
		!cooldownFuture ||
		!leaseReleased {
		t.Fatalf(
			"invalid durable lease state: count=%d cooldown=%v released=%v",
			lookupCount, cooldownFuture, leaseReleased,
		)
	}

	t.Log(
		"PASS: independent workers; exclusive lookup; durable cooldown; released lease",
	)
}

// TestPostgresPayoutRecoveryWorkerCancellation verifies
// that cancellation stops the lookup and preserves cooldown.
func TestPostgresPayoutRecoveryWorkerCancellation(
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

	fake := &payoutRecoveryFake{
		candidates: []postgresrepo.StaleDriverPayoutExecution{
			postgresRecoveryAuthoritativeCandidate(t, fixture),
		},
		entered: make(chan struct{}, 1),
		release: make(chan struct{}),
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

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)

	go func() {
		done <- worker.RunOnce(ctx, nil)
	}()

	select {
	case <-fake.entered:
	case <-time.After(10 * time.Second):
		t.Fatal("provider lookup did not begin")
	}

	cancel()

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf(
				"expected cancellation, got %v",
				err,
			)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("worker did not stop after cancellation")
	}

	var released bool
	var cooldownFuture bool

	err = fixture.db.QueryRow(fixture.ctx, `
        SELECT
            lease_expires_at <= clock_timestamp(),
            next_lookup_at > clock_timestamp()
        FROM driver_payout_recovery_leases
        WHERE payout_id = $1::uuid
    `, fixture.payoutID).Scan(
		&released,
		&cooldownFuture,
	)

	if err != nil {
		t.Fatal(err)
	}

	if !released || !cooldownFuture {
		t.Fatalf(
			"cancellation lease state invalid: released=%v cooldown=%v",
			released, cooldownFuture,
		)
	}

	// Verify the same instance remains usable.
	close(fake.release)

	if err := worker.RunOnce(
		context.Background(),
		nil,
	); err != nil {
		t.Fatal(err)
	}

	fake.mu.Lock()
	lookups := fake.lookups
	fake.mu.Unlock()

	if lookups != 1 {
		t.Fatalf(
			"cancellation bypassed cooldown: lookups=%d",
			lookups,
		)
	}

	t.Log(
		"PASS: cancellation; lease release; cooldown preservation",
	)
}

// Ensure the shared fake remains race-safe when used by
// independently scheduled recovery workers.
var _ sync.Locker = (*sync.Mutex)(nil)
