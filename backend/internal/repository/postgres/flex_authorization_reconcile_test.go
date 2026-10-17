package postgres_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	postgresrepo "github.com/JCKFinland/connect/backend/internal/repository/postgres"
)

func TestReconcileFlexAuthorizationProcessingIdentity(t *testing.T) {
	f := setupFlexAuthorizationTest(t)
	ctx := context.Background()
	auth, err := createTestFlexAuthorization(ctx, f)
	if err != nil {
		t.Fatalf("create FLEX authorization: %v", err)
	}

	claimed, didClaim, err := postgresrepo.ClaimFlexAuthorizationProcessing(
		ctx, f.DB, auth.ID,
	)
	if err != nil || !didClaim || claimed.Status != "PROCESSING" {
		t.Fatalf("claim authorization: claimed=%v didClaim=%v err=%v",
			claimed, didClaim, err)
	}

	result, err := postgresrepo.ReconcileFlexAuthorization(
		ctx, f.DB, auth.ID, "pi_flex_processing_123",
		postgresrepo.FlexResultProcessing,
	)
	if err != nil {
		t.Fatal(err)
	}

	if result.Status != "PROCESSING" ||
		result.ProviderAuthorizationID == nil ||
		*result.ProviderAuthorizationID != "pi_flex_processing_123" {
		t.Fatalf("unexpected processing reconciliation: %+v", result)
	}

	replay, err := postgresrepo.ReconcileFlexAuthorization(
		ctx, f.DB, auth.ID, "pi_flex_processing_123",
		postgresrepo.FlexResultProcessing,
	)
	if err != nil {
		t.Fatal(err)
	}
	if replay.Status != result.Status {
		t.Fatalf("idempotent replay changed status: %+v", replay)
	}
}

func TestReconcileFlexAuthorizationAuthorized(t *testing.T) {
	f := setupFlexAuthorizationTest(t)
	ctx := context.Background()
	auth, err := createTestFlexAuthorization(ctx, f)
	if err != nil {
		t.Fatalf("create FLEX authorization: %v", err)
	}

	_, claimed, err := postgresrepo.ClaimFlexAuthorizationProcessing(
		ctx, f.DB, auth.ID,
	)
	if err != nil || !claimed {
		t.Fatalf("claim: %v, claimed=%v", err, claimed)
	}

	result, err := postgresrepo.ReconcileFlexAuthorization(
		ctx, f.DB, auth.ID, "pi_flex_authorized_123",
		postgresrepo.FlexResultAuthorized,
	)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "AUTHORIZED" {
		t.Fatalf("unexpected status: %s", result.Status)
	}

	var authorizedAtPresent bool
	err = f.DB.QueryRow(ctx, `
		SELECT authorized_at IS NOT NULL
		FROM flex_authorizations
		WHERE id = $1
	`, auth.ID).Scan(&authorizedAtPresent)
	if err != nil || !authorizedAtPresent {
		t.Fatalf("authorized_at missing: %v", err)
	}

	replay, err := postgresrepo.ReconcileFlexAuthorization(
		ctx, f.DB, auth.ID, "pi_flex_authorized_123",
		postgresrepo.FlexResultAuthorized,
	)
	if err != nil || replay.Status != "AUTHORIZED" {
		t.Fatalf("authorized replay: %+v, %v", replay, err)
	}
}

func TestReconcileFlexAuthorizationRejectsProviderConflict(t *testing.T) {
	f := setupFlexAuthorizationTest(t)
	ctx := context.Background()
	auth, err := createTestFlexAuthorization(ctx, f)
	if err != nil {
		t.Fatalf("create FLEX authorization: %v", err)
	}

	_, claimed, err := postgresrepo.ClaimFlexAuthorizationProcessing(
		ctx, f.DB, auth.ID,
	)
	if err != nil || !claimed {
		t.Fatalf("claim: %v", err)
	}

	_, err = postgresrepo.ReconcileFlexAuthorization(
		ctx, f.DB, auth.ID, "pi_original",
		postgresrepo.FlexResultProcessing,
	)
	if err != nil {
		t.Fatal(err)
	}

	_, err = postgresrepo.ReconcileFlexAuthorization(
		ctx, f.DB, auth.ID, "pi_different",
		postgresrepo.FlexResultAuthorized,
	)
	if !errors.Is(err, postgresrepo.ErrFlexAuthorizationProviderConflict) {
		t.Fatalf("expected provider conflict, got %v", err)
	}
}

func TestReconcileFlexAuthorizationRejectsTerminalRegression(t *testing.T) {
	f := setupFlexAuthorizationTest(t)
	ctx := context.Background()
	auth, err := createTestFlexAuthorization(ctx, f)
	if err != nil {
		t.Fatalf("create FLEX authorization: %v", err)
	}

	_, claimed, err := postgresrepo.ClaimFlexAuthorizationProcessing(
		ctx, f.DB, auth.ID,
	)
	if err != nil || !claimed {
		t.Fatalf("claim: %v", err)
	}

	_, err = postgresrepo.ReconcileFlexAuthorization(
		ctx, f.DB, auth.ID, "pi_terminal",
		postgresrepo.FlexResultAuthorized,
	)
	if err != nil {
		t.Fatal(err)
	}

	_, err = postgresrepo.ReconcileFlexAuthorization(
		ctx, f.DB, auth.ID, "pi_terminal",
		postgresrepo.FlexResultCancelled,
	)
	if !errors.Is(err, postgresrepo.ErrFlexAuthorizationResultConflict) {
		t.Fatalf("expected terminal conflict, got %v", err)
	}
}

func TestReconcileFlexAuthorizationRejectsUnclaimed(t *testing.T) {
	f := setupFlexAuthorizationTest(t)
	ctx := context.Background()
	auth, err := createTestFlexAuthorization(ctx, f)
	if err != nil {
		t.Fatalf("create FLEX authorization: %v", err)
	}

	_, err = postgresrepo.ReconcileFlexAuthorization(
		ctx, f.DB, auth.ID, "pi_unclaimed",
		postgresrepo.FlexResultAuthorized,
	)
	if !errors.Is(err, postgresrepo.ErrFlexAuthorizationResultConflict) {
		t.Fatalf("expected unclaimed rejection, got %v", err)
	}
}

func TestReconcileFlexAuthorizationConcurrent(t *testing.T) {
	f := setupFlexAuthorizationTest(t)
	ctx := context.Background()
	auth, err := createTestFlexAuthorization(ctx, f)
	if err != nil {
		t.Fatalf("create FLEX authorization: %v", err)
	}

	_, claimed, err := postgresrepo.ClaimFlexAuthorizationProcessing(
		ctx, f.DB, auth.ID,
	)
	if err != nil || !claimed {
		t.Fatalf("claim: %v", err)
	}

	const workers = 8
	var wg sync.WaitGroup
	results := make(chan error, workers)

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := postgresrepo.ReconcileFlexAuthorization(
				ctx, f.DB, auth.ID, "pi_concurrent",
				postgresrepo.FlexResultAuthorized,
			)
			results <- err
		}()
	}

	wg.Wait()
	close(results)

	for err := range results {
		if err != nil {
			t.Fatalf("concurrent reconciliation: %v", err)
		}
	}

	var count int
	err = f.DB.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM flex_authorizations
		WHERE id = $1
		  AND status = 'AUTHORIZED'
		  AND provider_authorization_id = 'pi_concurrent'
	`, auth.ID).Scan(&count)
	if err != nil || count != 1 {
		t.Fatalf("unexpected authorized row count: %d, %v", count, err)
	}
}

func TestReconcileFlexAuthorizationRejectsTerminalIdentityEnrichment(
	t *testing.T,
) {
	f := setupFlexAuthorizationTest(t)
	ctx := context.Background()
	auth, err := createTestFlexAuthorization(ctx, f)
	if err != nil {
		t.Fatalf("create FLEX authorization: %v", err)
	}

	_, err = f.DB.Exec(ctx, `
		UPDATE flex_authorizations
		SET status = 'FAILED'
		WHERE id = $1
	`, auth.ID)
	if err != nil {
		t.Fatal(err)
	}

	_, err = postgresrepo.ReconcileFlexAuthorization(
		ctx, f.DB, auth.ID, "pi_late",
		postgresrepo.FlexResultFailed,
	)
	if !errors.Is(err, postgresrepo.ErrFlexAuthorizationResultConflict) {
		t.Fatalf(
			"expected terminal identity enrichment rejection, got %v",
			err,
		)
	}
}
