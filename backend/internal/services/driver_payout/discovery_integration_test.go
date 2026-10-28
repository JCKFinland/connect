package driver_payout

import (
	"errors"
	"testing"
	"time"

	postgresrepo "github.com/JCKFinland/connect/backend/internal/repository/postgres"
)

func TestStaleDriverPayoutExecutionDiscovery(
	t *testing.T,
) {
	fixture := newSubmissionFixture(t)

	cutoff := time.Now().UTC().Add(-30 * time.Minute)

	discover := func() []postgresrepo.StaleDriverPayoutExecution {
		t.Helper()

		results, err := postgresrepo.DiscoverStaleDriverPayoutExecutions(
			fixture.ctx,
			fixture.db,
			cutoff,
			100,
		)
		if err != nil {
			t.Fatal(err)
		}

		var matching []postgresrepo.StaleDriverPayoutExecution

		for _, item := range results {
			if item.PayoutID == fixture.payoutID {
				matching = append(matching, item)
			}
		}

		return matching
	}

	// An unclaimed execution must not be discovered.
	if got := discover(); len(got) != 0 {
		t.Fatalf(
			"PENDING execution discovered: %+v",
			got,
		)
	}

	execution, claimed, err :=
		postgresrepo.ClaimDriverPayoutExecution(
			fixture.ctx,
			fixture.db,
			fixture.payoutID,
		)

	if err != nil {
		t.Fatal(err)
	}
	if !claimed || execution == nil {
		t.Fatal("expected initial claim")
	}

	if execution.Status != "PROCESSING" ||
		execution.AttemptCount != 1 {
		t.Fatalf(
			"unexpected claim state: %+v",
			execution,
		)
	}

	// A recently claimed execution is not stale.
	if got := discover(); len(got) != 0 {
		t.Fatalf(
			"recent PROCESSING execution discovered: %+v",
			got,
		)
	}

	// A future cutoff classifies the same execution as stale,
	// without modifying its database timestamps.
	cutoff = time.Now().UTC().Add(1 * time.Minute)

	processing := discover()

	if len(processing) != 1 {
		t.Fatalf(
			"expected one stale PROCESSING execution, got %d",
			len(processing),
		)
	}

	item := processing[0]

	if item.Status != "PROCESSING" ||
		item.AttemptCount != 1 ||
		item.PayoutID != fixture.payoutID ||
		item.IdempotencyKey != execution.IdempotencyKey ||
		item.DestinationID != fixture.externalDestination ||
		item.AmountCents != 9300 ||
		item.Currency != "EUR" ||
		item.Provider != "TEST_PROVIDER" {
		t.Fatalf(
			"incorrect discovered execution: %+v",
			item,
		)
	}

	originalStartedAt := item.RequestStartedAt

	// Transition to reconciliation without contacting the provider.
	reconciled, err :=
		postgresrepo.MarkDriverPayoutRequiresReconciliation(
			fixture.ctx,
			fixture.db,
			fixture.payoutID,
			"integration discovery test",
		)

	if err != nil {
		t.Fatal(err)
	}
	if reconciled == nil ||
		reconciled.Status != "REQUIRES_RECONCILIATION" {
		t.Fatalf(
			"unexpected reconciliation state: %+v",
			reconciled,
		)
	}

	requiringReconciliation := discover()

	if len(requiringReconciliation) != 1 {
		t.Fatalf(
			"expected one reconciliation candidate, got %d",
			len(requiringReconciliation),
		)
	}

	item = requiringReconciliation[0]

	if item.Status != "REQUIRES_RECONCILIATION" ||
		item.AttemptCount != 1 ||
		item.IdempotencyKey != execution.IdempotencyKey ||
		!item.RequestStartedAt.Equal(originalStartedAt) {
		t.Fatalf(
			"discovery altered execution identity: %+v",
			item,
		)
	}

	// Discovery must not authorize another submission.
	_, replayClaimed, err :=
		postgresrepo.ClaimDriverPayoutExecution(
			fixture.ctx,
			fixture.db,
			fixture.payoutID,
		)

	if err != nil {
		t.Fatal(err)
	}
	if replayClaimed {
		t.Fatal("discovery enabled a duplicate claim")
	}

	// Validate rejection of unsafe discovery arguments.
	_, err = postgresrepo.DiscoverStaleDriverPayoutExecutions(
		fixture.ctx,
		fixture.db,
		time.Time{},
		100,
	)
	if !errors.Is(
		err,
		postgresrepo.ErrDriverPayoutExecutionInvalidInput,
	) {
		t.Fatalf("zero cutoff accepted: %v", err)
	}

	_, err = postgresrepo.DiscoverStaleDriverPayoutExecutions(
		fixture.ctx,
		fixture.db,
		cutoff,
		0,
	)
	if !errors.Is(
		err,
		postgresrepo.ErrDriverPayoutExecutionInvalidInput,
	) {
		t.Fatalf("zero limit accepted: %v", err)
	}

	_, err = postgresrepo.DiscoverStaleDriverPayoutExecutions(
		fixture.ctx,
		fixture.db,
		cutoff,
		501,
	)
	if !errors.Is(
		err,
		postgresrepo.ErrDriverPayoutExecutionInvalidInput,
	) {
		t.Fatalf("oversized limit accepted: %v", err)
	}

	t.Log(
		"PASS: pending excluded;",
		"recent processing excluded;",
		"stale processing discovered;",
		"reconciliation discovered;",
		"immutable transfer identity preserved",
	)
}
