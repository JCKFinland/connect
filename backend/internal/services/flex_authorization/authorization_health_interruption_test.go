package flex_authorization

import (
	"context"
	"errors"
	"testing"
	"time"

	postgresrepo "github.com/JCKFinland/connect/backend/internal/repository/postgres"
)

func TestFlexInterruptedAuthorizationVisibleInHealth(t *testing.T) {
	f := setupAuthorizationServiceTest(t)
	ctx := context.Background()

	before, err := postgresrepo.ReadFlexAuthorizationHealthSnapshot(
		ctx, f.DB, 15*time.Minute,
	)
	if err != nil {
		t.Fatal(err)
	}

	f.adapter.err = errors.New(
		"Stripe connection interrupted after authorization request",
	)

	_, err = f.service.Execute(
		ctx, f.RideID, f.AuctionID, f.passengerID,
	)
	if err == nil {
		t.Fatal("expected interrupted provider execution")
	}

	if f.adapter.callCount() != 1 {
		t.Fatal("expected exactly one provider attempt")
	}

	var authorizationID string
	var status string
	var providerID *string

	err = f.DB.QueryRow(ctx, `
        SELECT id, status, provider_authorization_id
        FROM flex_authorizations
        WHERE auction_id = $1
    `, f.AuctionID).Scan(
		&authorizationID,
		&status,
		&providerID,
	)
	if err != nil {
		t.Fatal(err)
	}

	if status != "PROCESSING" || providerID != nil {
		t.Fatalf(
			"unexpected interrupted authorization: status=%s provider=%v",
			status, providerID,
		)
	}

	snapshot, err := postgresrepo.ReadFlexAuthorizationHealthSnapshot(
		ctx, f.DB, 15*time.Minute,
	)
	if err != nil {
		t.Fatal(err)
	}

	if snapshot.ProcessingWithoutProviderCount !=
		before.ProcessingWithoutProviderCount+1 {
		t.Fatalf(
			"unresolved authorization missing from health: before=%d after=%d",
			before.ProcessingWithoutProviderCount,
			snapshot.ProcessingWithoutProviderCount,
		)
	}

	if snapshot.OldestProcessingWithoutProviderAt == nil {
		t.Fatal("missing unresolved authorization timestamp")
	}

	_, err = f.service.Execute(
		ctx, f.RideID, f.AuctionID, f.passengerID,
	)
	if !errors.Is(err, ErrAuthorizationPending) {
		t.Fatalf("expected pending recovery, got %v", err)
	}

	if f.adapter.callCount() != 1 {
		t.Fatal("repeated execution initiated another provider attempt")
	}

	afterRetry, err := postgresrepo.ReadFlexAuthorizationHealthSnapshot(
		ctx, f.DB, 15*time.Minute,
	)
	if err != nil {
		t.Fatal(err)
	}

	if afterRetry.ProcessingWithoutProviderCount !=
		snapshot.ProcessingWithoutProviderCount {
		t.Fatal("repeated execution changed unresolved health count")
	}

	err = f.DB.QueryRow(ctx, `
        SELECT status, provider_authorization_id
        FROM flex_authorizations
        WHERE id = $1
    `, authorizationID).Scan(
		&status,
		&providerID,
	)
	if err != nil {
		t.Fatal(err)
	}

	if status != "PROCESSING" || providerID != nil {
		t.Fatal("monitoring or retry changed uncertain authorization")
	}
}
