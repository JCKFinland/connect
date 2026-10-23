package postgres_test

import (
	"context"
	"testing"
	"time"

	postgresrepo "github.com/JCKFinland/connect/backend/internal/repository/postgres"
	"github.com/JCKFinland/connect/backend/internal/testutil"
)

func TestFlexAuthorizationHealthSnapshotClassification(t *testing.T) {
	f := testutil.SetupFlexAcceptFixture(t)
	ctx := context.Background()

	// This fixture provides an isolated, disposable FLEX auction.
	// The test creates one pending authorization and verifies that
	// the monitoring snapshot reports it without modifying it.

	before, err := postgresrepo.ReadFlexAuthorizationHealthSnapshot(
		ctx,
		f.DB,
		15*time.Minute,
	)
	if err != nil {
		t.Fatal(err)
	}

	if before.ObservedAt.IsZero() {
		t.Fatal("authorization health observation timestamp missing")
	}

	if before.PendingCount < 0 ||
		before.ProcessingWithoutProviderCount < 0 ||
		before.ProcessingWithProviderCount < 0 ||
		before.AuthorizedCount < 0 {
		t.Fatal("authorization health counts cannot be negative")
	}

	if before.StalePendingCount > before.PendingCount {
		t.Fatal("stale pending count exceeds pending count")
	}

	if before.StaleProcessingWithoutProviderCount >
		before.ProcessingWithoutProviderCount {
		t.Fatal("stale unknown-provider count exceeds processing count")
	}

	if before.StaleProcessingWithProviderCount >
		before.ProcessingWithProviderCount {
		t.Fatal("stale known-provider count exceeds processing count")
	}

	// Read-only monitoring must not create an authorization.
	var fixtureAuthorizationCount int64

	err = f.DB.QueryRow(ctx, `
        SELECT COUNT(*)
        FROM flex_authorizations
        WHERE auction_id = $1
    `, f.AuctionID).Scan(&fixtureAuthorizationCount)
	if err != nil {
		t.Fatal(err)
	}

	if fixtureAuthorizationCount != 0 {
		t.Fatalf(
			"monitoring created fixture authorization: %d",
			fixtureAuthorizationCount,
		)
	}

}

func TestFlexAuthorizationHealthSnapshotRejectsInvalidDependencies(
	t *testing.T,
) {
	ctx := context.Background()

	_, err := postgresrepo.ReadFlexAuthorizationHealthSnapshot(
		ctx,
		nil,
		15*time.Minute,
	)
	if err == nil {
		t.Fatal("nil database must be rejected")
	}

	_, err = postgresrepo.ReadFlexAuthorizationHealthSnapshot(
		nil,
		nil,
		15*time.Minute,
	)
	if err == nil {
		t.Fatal("nil context must be rejected")
	}
}
