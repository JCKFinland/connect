package postgres_test

import (
	"context"
	"testing"
	"time"

	postgresrepo "github.com/JCKFinland/connect/backend/internal/repository/postgres"
)

func TestFlexAuthorizationHealthRealStateClassification(t *testing.T) {
	f := setupFlexAuthorizationTest(t)
	ctx := context.Background()

	before, err := postgresrepo.ReadFlexAuthorizationHealthSnapshot(
		ctx, f.DB, 15*time.Minute,
	)
	if err != nil {
		t.Fatal(err)
	}

	authorization, err := createTestFlexAuthorization(ctx, f)
	if err != nil {
		t.Fatal(err)
	}

	pending, err := postgresrepo.ReadFlexAuthorizationHealthSnapshot(
		ctx, f.DB, 15*time.Minute,
	)
	if err != nil {
		t.Fatal(err)
	}

	if pending.PendingCount != before.PendingCount+1 {
		t.Fatalf(
			"PENDING count: before=%d after=%d",
			before.PendingCount, pending.PendingCount,
		)
	}

	claimed, didClaim, err :=
		postgresrepo.ClaimFlexAuthorizationProcessing(
			ctx, f.DB, authorization.ID,
		)
	if err != nil {
		t.Fatal(err)
	}
	if !didClaim || claimed.Status != "PROCESSING" {
		t.Fatal("authorization was not claimed for processing")
	}

	unknownProvider, err :=
		postgresrepo.ReadFlexAuthorizationHealthSnapshot(
			ctx, f.DB, 15*time.Minute,
		)
	if err != nil {
		t.Fatal(err)
	}

	if unknownProvider.PendingCount != before.PendingCount {
		t.Fatal("PROCESSING authorization remained PENDING")
	}

	if unknownProvider.ProcessingWithoutProviderCount !=
		before.ProcessingWithoutProviderCount+1 {
		t.Fatal("unknown-provider PROCESSING classification incorrect")
	}

	_, err = f.DB.Exec(ctx, `
        UPDATE flex_authorizations
        SET provider_authorization_id = $2
        WHERE id = $1
          AND status = 'PROCESSING'
    `, authorization.ID, "pi_health_"+authorization.ID)
	if err != nil {
		t.Fatal(err)
	}

	knownProvider, err :=
		postgresrepo.ReadFlexAuthorizationHealthSnapshot(
			ctx, f.DB, 15*time.Minute,
		)
	if err != nil {
		t.Fatal(err)
	}

	if knownProvider.ProcessingWithoutProviderCount !=
		before.ProcessingWithoutProviderCount {
		t.Fatal("known-provider record remained in unknown-provider group")
	}

	if knownProvider.ProcessingWithProviderCount !=
		before.ProcessingWithProviderCount+1 {
		t.Fatal("known-provider PROCESSING classification incorrect")
	}

	_, err = f.DB.Exec(ctx, `
        UPDATE flex_authorizations
        SET status = 'AUTHORIZED',
            authorized_at = clock_timestamp()
        WHERE id = $1
          AND status = 'PROCESSING'
    `, authorization.ID)
	if err != nil {
		t.Fatal(err)
	}

	authorized, err :=
		postgresrepo.ReadFlexAuthorizationHealthSnapshot(
			ctx, f.DB, 15*time.Minute,
		)
	if err != nil {
		t.Fatal(err)
	}

	if authorized.AuthorizedCount != before.AuthorizedCount+1 {
		t.Fatal("AUTHORIZED classification incorrect")
	}

	if authorized.ProcessingWithProviderCount !=
		before.ProcessingWithProviderCount {
		t.Fatal("AUTHORIZED record remained unresolved")
	}

	var persistedStatus string
	err = f.DB.QueryRow(ctx, `
        SELECT status
        FROM flex_authorizations
        WHERE id = $1
    `, authorization.ID).Scan(&persistedStatus)
	if err != nil {
		t.Fatal(err)
	}

	if persistedStatus != "AUTHORIZED" {
		t.Fatalf("health monitoring changed authorization: %s", persistedStatus)
	}
}
