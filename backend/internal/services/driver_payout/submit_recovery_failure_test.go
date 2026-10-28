package driver_payout

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/JCKFinland/connect/backend/internal/payouts"
	"github.com/jackc/pgx/v5"
)

type recoveryBlockingProvider struct {
	mu        sync.Mutex
	calls     int
	request   payouts.TransferRequest
	initiated chan struct{}
	release   chan struct{}
}

func (p *recoveryBlockingProvider) InitiateTransfer(
	ctx context.Context,
	request payouts.TransferRequest,
) (payouts.TransferResult, error) {
	p.mu.Lock()
	p.calls++
	p.request = request
	p.mu.Unlock()

	close(p.initiated)

	select {
	case <-p.release:
	case <-ctx.Done():
		return payouts.TransferResult{}, ctx.Err()
	}

	return payouts.TransferResult{
		Status: payouts.TransferUnknown,
	}, nil
}

func (p *recoveryBlockingProvider) LookupTransfer(
	context.Context,
	payouts.TransferLookup,
) (payouts.TransferResult, error) {
	return payouts.TransferResult{}, errors.New(
		"lookup forbidden in submission test",
	)
}

func TestSubmitPayoutReconciliationPersistenceFailure(
	t *testing.T,
) {
	fixture := newSubmissionFixture(t)

	provider := &recoveryBlockingProvider{
		initiated: make(chan struct{}),
		release:   make(chan struct{}),
	}

	submitter, err := NewSubmitter(
		fixture.db, provider,
	)
	if err != nil {
		t.Fatal(err)
	}

	resultCh := make(chan error, 1)

	go func() {
		resultCh <- submitter.SubmitPayout(
			fixture.ctx, fixture.payoutID,
		)
	}()

	select {
	case <-provider.initiated:
	case <-time.After(15 * time.Second):
		t.Fatal("provider initiation did not start")
	}

	// The provider call is paused after the claim commits.
	// Acquire a separate transaction lock on the payout row.
	lockCtx, lockCancel := context.WithTimeout(
		context.Background(), 25*time.Second,
	)
	defer lockCancel()

	tx, err := fixture.db.BeginTx(
		lockCtx, pgx.TxOptions{},
	)
	if err != nil {
		t.Fatal(err)
	}

	lockReleased := false

	t.Cleanup(func() {
		if !lockReleased {
			cleanupCtx, cancel := context.WithTimeout(
				context.Background(), 5*time.Second,
			)
			defer cancel()
			if err := tx.Rollback(cleanupCtx); err != nil {
				t.Errorf("release payout lock: %v", err)
			}
		}
	})

	var lockedID string

	err = tx.QueryRow(lockCtx, `
        SELECT id::text
        FROM driver_payouts
        WHERE id=$1
        FOR UPDATE
    `, fixture.payoutID).Scan(&lockedID)
	if err != nil {
		t.Fatal(err)
	}

	if lockedID != fixture.payoutID {
		t.Fatal("incorrect payout row locked")
	}

	// Permit the synthetic provider to return while the
	// payout row remains locked.
	close(provider.release)

	// Reconciliation waits on our lock until its 10-second
	// context deadline expires.
	var submissionErr error

	select {
	case submissionErr = <-resultCh:
	case <-time.After(17 * time.Second):
		t.Fatal("reconciliation recovery exceeded deadline")
	}

	if !errors.Is(
		submissionErr, ErrSubmissionUnresolved,
	) {
		t.Fatalf(
			"expected unresolved submission: %v",
			submissionErr,
		)
	}

	if !strings.Contains(
		submissionErr.Error(),
		"reconciliation state update failed",
	) {
		t.Fatalf(
			"expected persistence failure: %v",
			submissionErr,
		)
	}

	// Release the lock before inspecting the committed state.
	rollbackCtx, rollbackCancel := context.WithTimeout(
		context.Background(), 5*time.Second,
	)
	defer rollbackCancel()

	if err := tx.Rollback(rollbackCtx); err != nil {
		t.Fatal(err)
	}
	lockReleased = true

	var executionStatus string
	var payoutStatus string
	var attempts int

	err = fixture.db.QueryRow(fixture.ctx, `
        SELECT
            e.execution_status,
            p.payout_status,
            e.attempt_count
        FROM driver_payout_executions e
        JOIN driver_payouts p
          ON p.id=e.payout_id
        WHERE e.payout_id=$1
    `, fixture.payoutID).Scan(
		&executionStatus,
		&payoutStatus,
		&attempts,
	)
	if err != nil {
		t.Fatal(err)
	}

	if executionStatus != "PROCESSING" ||
		payoutStatus != "PENDING" ||
		attempts != 1 {
		t.Fatalf(
			"unsafe state after failed reconciliation: execution=%s payout=%s attempts=%d",
			executionStatus,
			payoutStatus,
			attempts,
		)
	}

	// The existing PROCESSING state must never authorize
	// another provider initiation.
	replayErr := submitter.SubmitPayout(
		fixture.ctx, fixture.payoutID,
	)

	if !errors.Is(
		replayErr, ErrSubmissionNotAuthorized,
	) {
		t.Fatalf(
			"replay unexpectedly authorized: %v",
			replayErr,
		)
	}

	provider.mu.Lock()
	calls := provider.calls
	request := provider.request
	provider.mu.Unlock()

	if calls != 1 {
		t.Fatalf(
			"expected one provider initiation, got %d",
			calls,
		)
	}

	if request.PayoutID != fixture.payoutID ||
		request.DestinationID != fixture.externalDestination ||
		request.AmountCents != 9300 ||
		request.Currency != "EUR" {
		t.Fatalf(
			"incorrect provider request: %+v",
			request,
		)
	}

	t.Log(
		"PASS: reconciliation persistence failed;",
		"execution remains PROCESSING;",
		"attempt_count=1;",
		"provider initiation cannot repeat",
	)
}
