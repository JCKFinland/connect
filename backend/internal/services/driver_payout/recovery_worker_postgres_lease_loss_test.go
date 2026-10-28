package driver_payout

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	postgresrepo "github.com/JCKFinland/connect/backend/internal/repository/postgres"
	"github.com/google/uuid"
)

func TestPostgresPayoutRecoveryWorkerRejectsLeaseLostBeforeValidation(t *testing.T) {
	fixture := newSubmissionFixture(t)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	payoutID := fixture.payoutID

	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(
			context.Background(), 5*time.Second,
		)
		defer cleanupCancel()

		_, _ = fixture.db.Exec(
			cleanupCtx,
			`DELETE FROM driver_payout_recovery_leases
			 WHERE payout_id = $1::uuid`,
			payoutID,
		)
	})

	_, _, err := postgresrepo.ClaimDriverPayoutExecution(
		ctx,
		fixture.db,
		payoutID,
	)
	if err != nil {
		t.Fatalf("claim execution: %v", err)
	}

	candidate := postgresRecoveryAuthoritativeCandidate(t, fixture)

	fake := &payoutRecoveryFake{
		candidates: []postgresrepo.StaleDriverPayoutExecution{
			candidate,
		},
	}

	worker, err := NewPostgresPayoutRecoveryWorker(
		fixture.db,
		fake,
		time.Minute,
		1,
	)
	if err != nil {
		t.Fatalf("construct worker: %v", err)
	}

	entered := make(chan struct{})
	resume := make(chan struct{})

	var once sync.Once

	worker.beforeLeaseValidation = func() {
		once.Do(func() { close(entered) })

		select {
		case <-resume:
		case <-ctx.Done():
		}
	}

	errorsMu := sync.Mutex{}
	var reported []error

	done := make(chan error, 1)

	go func() {
		done <- worker.RunOnce(ctx, func(err error) {
			errorsMu.Lock()
			reported = append(reported, err)
			errorsMu.Unlock()
		})
	}()

	select {
	case <-entered:
	case <-ctx.Done():
		close(resume)
		t.Fatal("worker never reached lease ownership validation")
	}

	var originalToken string

	err = fixture.db.QueryRow(
		ctx,
		`SELECT lease_token::text
		 FROM driver_payout_recovery_leases
		 WHERE payout_id = $1::uuid
		   AND lease_expires_at > clock_timestamp()`,
		payoutID,
	).Scan(&originalToken)
	if err != nil {
		close(resume)
		t.Fatalf("read acquired lease: %v", err)
	}

	replacementToken := uuid.NewString()

	tag, err := fixture.db.Exec(
		ctx,
		`UPDATE driver_payout_recovery_leases
		 SET lease_token = $2::uuid,
		     lease_owner = 'replacement-test-owner',
		     updated_at = clock_timestamp()
		 WHERE payout_id = $1::uuid
		   AND lease_token = $3::uuid`,
		payoutID,
		replacementToken,
		originalToken,
	)
	if err != nil {
		close(resume)
		t.Fatalf("replace lease token: %v", err)
	}

	if tag.RowsAffected() != 1 {
		close(resume)
		t.Fatalf(
			"expected one lease replacement, got %d",
			tag.RowsAffected(),
		)
	}

	close(resume)

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("worker RunOnce: %v", err)
		}
	case <-ctx.Done():
		t.Fatal("worker did not finish after lease replacement")
	}

	fake.mu.Lock()
	lookups := fake.lookups
	fake.mu.Unlock()

	if lookups != 0 {
		t.Fatalf(
			"provider inspector called after lease loss: %d",
			lookups,
		)
	}

	errorsMu.Lock()
	foundOwnershipError := false
	for _, reportedErr := range reported {
		if reportedErr != nil &&
			(errors.Is(reportedErr, postgresrepo.ErrDriverPayoutRecoveryLeaseInvalid) ||
				containsLeaseOwnershipError(reportedErr)) {
			foundOwnershipError = true
		}
	}
	errorsMu.Unlock()

	if !foundOwnershipError {
		t.Fatalf(
			"expected lease ownership failure; reported errors: %v",
			reported,
		)
	}

	var retainedToken, retainedOwner string

	err = fixture.db.QueryRow(
		ctx,
		`SELECT lease_token::text, lease_owner
		 FROM driver_payout_recovery_leases
		 WHERE payout_id = $1::uuid`,
		payoutID,
	).Scan(&retainedToken, &retainedOwner)
	if err != nil {
		t.Fatalf("read retained lease: %v", err)
	}

	if retainedToken != replacementToken ||
		retainedOwner != "replacement-test-owner" {
		t.Fatalf(
			"replacement lease changed: token=%s owner=%s",
			retainedToken,
			retainedOwner,
		)
	}

	t.Log("PASS: lease lost before validation; zero provider lookups; replacement ownership retained")
}

func containsLeaseOwnershipError(err error) bool {
	return err != nil &&
		strings.Contains(
			err.Error(),
			"recovery lease ownership expired or changed",
		)
}
