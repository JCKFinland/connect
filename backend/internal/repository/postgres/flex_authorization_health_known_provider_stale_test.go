package postgres_test

import (
	"context"
	"testing"
	"time"

	postgresrepo "github.com/JCKFinland/connect/backend/internal/repository/postgres"
)

func TestFlexAuthorizationHealthStaleKnownProvider(t *testing.T) {
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

	_, claimed, err := postgresrepo.ClaimFlexAuthorizationProcessing(
		ctx, f.DB, authorization.ID,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !claimed {
		t.Fatal("authorization was not claimed")
	}

	providerID := "pi_health_known_" + authorization.ID

	tag, err := f.DB.Exec(ctx, `
        UPDATE flex_authorizations
        SET provider_authorization_id = $2,
            updated_at = clock_timestamp() - INTERVAL '16 minutes'
        WHERE id = $1
          AND status = 'PROCESSING'
          AND provider_authorization_id IS NULL
    `, authorization.ID, providerID)
	if err != nil {
		t.Fatal(err)
	}
	if tag.RowsAffected() != 1 {
		t.Fatal("failed to establish known-provider stale fixture")
	}

	snapshot, err := postgresrepo.ReadFlexAuthorizationHealthSnapshot(
		ctx, f.DB, 15*time.Minute,
	)
	if err != nil {
		t.Fatal(err)
	}

	if snapshot.ProcessingWithProviderCount !=
		before.ProcessingWithProviderCount+1 {
		t.Fatal("known-provider PROCESSING count incorrect")
	}

	if snapshot.StaleProcessingWithProviderCount !=
		before.StaleProcessingWithProviderCount+1 {
		t.Fatal("stale known-provider authorization not detected")
	}

	if snapshot.ProcessingWithoutProviderCount !=
		before.ProcessingWithoutProviderCount {
		t.Fatal("known-provider authorization misclassified")
	}

	if snapshot.OldestProcessingWithProviderAt == nil {
		t.Fatal("missing oldest known-provider processing timestamp")
	}

	var status string
	var persistedProviderID string
	var updatedAt time.Time

	err = f.DB.QueryRow(ctx, `
        SELECT status, provider_authorization_id, updated_at
        FROM flex_authorizations
        WHERE id = $1
    `, authorization.ID).Scan(
		&status,
		&persistedProviderID,
		&updatedAt,
	)
	if err != nil {
		t.Fatal(err)
	}

	if status != "PROCESSING" || persistedProviderID != providerID {
		t.Fatal("health monitoring changed authorization identity or state")
	}

	if updatedAt.After(snapshot.ObservedAt.Add(-15 * time.Minute)) {
		t.Fatal("stale authorization timestamp unexpectedly refreshed")
	}

	freshThreshold, err := postgresrepo.ReadFlexAuthorizationHealthSnapshot(
		ctx, f.DB, 20*time.Minute,
	)
	if err != nil {
		t.Fatal(err)
	}

	if freshThreshold.StaleProcessingWithProviderCount !=
		before.StaleProcessingWithProviderCount {
		t.Fatal("known-provider authorization stale before 20 minutes")
	}

	var finalStatus string
	var finalProviderID string
	var finalUpdatedAt time.Time

	err = f.DB.QueryRow(ctx, `
        SELECT status, provider_authorization_id, updated_at
        FROM flex_authorizations
        WHERE id = $1
    `, authorization.ID).Scan(
		&finalStatus,
		&finalProviderID,
		&finalUpdatedAt,
	)
	if err != nil {
		t.Fatal(err)
	}

	if finalStatus != status ||
		finalProviderID != persistedProviderID ||
		!finalUpdatedAt.Equal(updatedAt) {
		t.Fatal("health snapshots modified authorization state")
	}
}
