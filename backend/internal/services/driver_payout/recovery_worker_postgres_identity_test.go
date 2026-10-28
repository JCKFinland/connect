package driver_payout

import (
	"testing"
	"time"

	postgresrepo "github.com/JCKFinland/connect/backend/internal/repository/postgres"
)

// postgresRecoveryAuthoritativeCandidate returns the actual bound
// execution identity from the isolated integration database.
func postgresRecoveryAuthoritativeCandidate(
	t *testing.T,
	fixture submissionFixture,
) postgresrepo.StaleDriverPayoutExecution {
	t.Helper()

	candidates, err :=
		postgresrepo.DiscoverStaleDriverPayoutExecutions(
			fixture.ctx,
			fixture.db,
			time.Now().UTC().Add(time.Minute),
			500,
		)
	if err != nil {
		t.Fatal(err)
	}

	for _, candidate := range candidates {
		if candidate.PayoutID == fixture.payoutID {
			return candidate
		}
	}

	t.Fatalf(
		"claimed payout %s not found in authoritative discovery",
		fixture.payoutID,
	)

	return postgresrepo.StaleDriverPayoutExecution{}
}
