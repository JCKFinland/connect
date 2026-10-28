package driver_payout

import (
	"errors"
	"testing"

	postgresrepo "github.com/JCKFinland/connect/backend/internal/repository/postgres"
)

func TestRecoveryInspectorRejectsMismatchedIdentityBeforeLookup(
	t *testing.T,
) {
	fixture := newSubmissionFixture(t)

	_, claimed, err := postgresrepo.ClaimDriverPayoutExecution(
		fixture.ctx,
		fixture.db,
		fixture.payoutID,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !claimed {
		t.Fatal("expected initial execution claim")
	}

	candidate := postgresRecoveryAuthoritativeCandidate(t, fixture)
	candidate.IdempotencyKey += "-incorrect"

	provider := &recoveryLookupProvider{}
	inspector, err := NewRecoveryInspector(fixture.db, provider)
	if err != nil {
		t.Fatal(err)
	}

	_, err = inspector.InspectCandidate(
		fixture.ctx,
		candidate,
	)
	if !errors.Is(
		err,
		postgresrepo.ErrDriverPayoutRecoveryIdentityMismatch,
	) {
		t.Fatalf("expected identity mismatch, got %v", err)
	}

	if provider.lookups != 0 || provider.initiations != 0 {
		t.Fatalf(
			"invalid identity contacted provider: lookups=%d initiations=%d",
			provider.lookups,
			provider.initiations,
		)
	}

	var status string
	var attempts int
	var reference *string

	err = fixture.db.QueryRow(fixture.ctx, `
		SELECT execution_status, attempt_count, provider_reference
		FROM driver_payout_executions
		WHERE payout_id=$1
	`, fixture.payoutID).Scan(&status, &attempts, &reference)
	if err != nil {
		t.Fatal(err)
	}

	if status != "PROCESSING" ||
		attempts != 1 ||
		reference != nil {
		t.Fatalf(
			"financial execution changed: status=%s attempts=%d reference=%v",
			status,
			attempts,
			reference,
		)
	}
}
