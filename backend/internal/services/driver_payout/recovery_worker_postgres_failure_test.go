package driver_payout

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	postgresrepo "github.com/JCKFinland/connect/backend/internal/repository/postgres"
)

type postgresRecoveryFailureLookup struct {
	mu        sync.Mutex
	candidate postgresrepo.StaleDriverPayoutExecution
	calls     int
}

func (f *postgresRecoveryFailureLookup) DiscoverCandidates(
	ctx context.Context,
	_ time.Time,
	_ int,
) ([]postgresrepo.StaleDriverPayoutExecution, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	return []postgresrepo.StaleDriverPayoutExecution{
		f.candidate,
	}, nil
}

func (f *postgresRecoveryFailureLookup) InspectCandidate(
	ctx context.Context,
	candidate postgresrepo.StaleDriverPayoutExecution,
) (RecoveryObservation, error) {
	f.mu.Lock()
	f.calls++
	f.mu.Unlock()

	return RecoveryObservation{
		Execution:   candidate,
		LookupError: errors.New("synthetic provider lookup failure"),
	}, nil
}

func TestPostgresPayoutRecoveryWorkerProviderFailure(
	t *testing.T,
) {
	fixture := newSubmissionFixture(t)

	// Cleanup coordination row before fixture financial records.
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

	fake := &postgresRecoveryFailureLookup{
		candidate: postgresRecoveryAuthoritativeCandidate(t, fixture),
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

	var reported []error

	reportError := func(err error) {
		reported = append(reported, err)
	}

	if err := worker.RunOnce(
		context.Background(),
		reportError,
	); err != nil {
		t.Fatal(err)
	}

	fake.mu.Lock()
	calls := fake.calls
	fake.mu.Unlock()

	if calls != 1 {
		t.Fatalf(
			"expected one provider lookup, got %d",
			calls,
		)
	}

	if len(reported) != 1 {
		t.Fatalf(
			"expected one reported provider error, got %d",
			len(reported),
		)
	}

	var released bool
	var cooldownFuture bool
	var lookupCount int64

	err = fixture.db.QueryRow(fixture.ctx, `
        SELECT
            lease_expires_at <= clock_timestamp(),
            next_lookup_at > clock_timestamp(),
            lookup_count
        FROM driver_payout_recovery_leases
        WHERE payout_id = $1::uuid
    `, fixture.payoutID).Scan(
		&released,
		&cooldownFuture,
		&lookupCount,
	)

	if err != nil {
		t.Fatal(err)
	}

	if !released || !cooldownFuture || lookupCount != 1 {
		t.Fatalf(
			"failure left unsafe lease state: released=%v cooldown=%v count=%d",
			released, cooldownFuture, lookupCount,
		)
	}

	// A new worker must respect the persisted cooldown.
	replacement, err := NewPostgresPayoutRecoveryWorker(
		fixture.db,
		fake,
		time.Minute,
		10,
	)
	if err != nil {
		t.Fatal(err)
	}

	if err := replacement.RunOnce(
		context.Background(),
		reportError,
	); err != nil {
		t.Fatal(err)
	}

	fake.mu.Lock()
	calls = fake.calls
	fake.mu.Unlock()

	if calls != 1 {
		t.Fatalf(
			"provider failure bypassed cooldown: calls=%d",
			calls,
		)
	}

	var executionStatus string
	var attemptCount int
	var providerReference *string

	err = fixture.db.QueryRow(fixture.ctx, `
        SELECT
            execution_status,
            attempt_count,
            provider_reference
        FROM driver_payout_executions
        WHERE payout_id = $1::uuid
    `, fixture.payoutID).Scan(
		&executionStatus,
		&attemptCount,
		&providerReference,
	)

	if err != nil {
		t.Fatal(err)
	}

	if executionStatus != "PROCESSING" ||
		attemptCount != 1 ||
		providerReference != nil {
		t.Fatalf(
			"financial execution unexpectedly changed: status=%s attempts=%d reference=%v",
			executionStatus, attemptCount, providerReference,
		)
	}

	t.Log(
		"PASS: provider failure reported; lease released; cooldown preserved; financial state unchanged",
	)
}
