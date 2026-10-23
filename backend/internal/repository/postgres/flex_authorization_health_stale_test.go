package postgres_test

import (
	"context"
	"testing"
	"time"

	postgresrepo "github.com/JCKFinland/connect/backend/internal/repository/postgres"
)

func TestFlexAuthorizationHealthStalePendingClassification(t *testing.T) {
	f := setupFlexAuthorizationTest(t)
	ctx := context.Background()

	authorization, err := createTestFlexAuthorization(ctx, f)
	if err != nil {
		t.Fatal(err)
	}

	fresh, err := postgresrepo.ReadFlexAuthorizationHealthSnapshot(
		ctx, f.DB, 15*time.Minute,
	)
	if err != nil {
		t.Fatal(err)
	}

	stale, err := postgresrepo.ReadFlexAuthorizationHealthSnapshot(
		ctx, f.DB, time.Microsecond,
	)
	if err != nil {
		t.Fatal(err)
	}

	if stale.StalePendingCount <= fresh.StalePendingCount {
		t.Fatalf(
			"expected stale PENDING count to increase: fresh=%d stale=%d",
			fresh.StalePendingCount,
			stale.StalePendingCount,
		)
	}

	if stale.OldestPendingAt == nil {
		t.Fatal("missing oldest PENDING timestamp")
	}

	var status string
	err = f.DB.QueryRow(ctx, `
        SELECT status
        FROM flex_authorizations
        WHERE id = $1
    `, authorization.ID).Scan(&status)
	if err != nil {
		t.Fatal(err)
	}

	if status != "PENDING" {
		t.Fatalf("health snapshot changed authorization state: %s", status)
	}
}

func TestFlexAuthorizationHealthStaleProcessingClassification(t *testing.T) {
	f := setupFlexAuthorizationTest(t)
	ctx := context.Background()

	authorization, err := createTestFlexAuthorization(ctx, f)
	if err != nil {
		t.Fatal(err)
	}

	_, claimed, err := postgresrepo.ClaimFlexAuthorizationProcessing(
		ctx, f.DB, authorization.ID,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !claimed {
		t.Fatal("authorization was not claimed")
	}

	fresh, err := postgresrepo.ReadFlexAuthorizationHealthSnapshot(
		ctx, f.DB, 15*time.Minute,
	)
	if err != nil {
		t.Fatal(err)
	}

	stale, err := postgresrepo.ReadFlexAuthorizationHealthSnapshot(
		ctx, f.DB, time.Microsecond,
	)
	if err != nil {
		t.Fatal(err)
	}

	if stale.StaleProcessingWithoutProviderCount <=
		fresh.StaleProcessingWithoutProviderCount {
		t.Fatal("PROCESSING without provider not classified as stale")
	}

	if stale.OldestProcessingWithoutProviderAt == nil {
		t.Fatal("missing oldest unresolved processing timestamp")
	}

	var status string
	err = f.DB.QueryRow(ctx, `
        SELECT status
        FROM flex_authorizations
        WHERE id = $1
    `, authorization.ID).Scan(&status)
	if err != nil {
		t.Fatal(err)
	}

	if status != "PROCESSING" {
		t.Fatalf("health snapshot changed processing state: %s", status)
	}
}
