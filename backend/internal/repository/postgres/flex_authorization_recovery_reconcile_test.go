package postgres_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	postgresrepo "github.com/JCKFinland/connect/backend/internal/repository/postgres"
)

func setupRecoveryReconciliationTest(
	t *testing.T,
) (string, string, func()) {
	t.Helper()

	f := setupFlexAuthorizationTest(t)
	ctx := context.Background()

	auth, err := createTestFlexAuthorization(ctx, f)
	if err != nil {
		t.Fatal(err)
	}

	_, claimed, err := postgresrepo.ClaimFlexAuthorizationProcessing(
		ctx,
		f.DB,
		auth.ID,
	)
	if err != nil || !claimed {
		t.Fatalf(
			"claim recovery authorization: claimed=%v err=%v",
			claimed,
			err,
		)
	}

	providerID := "pi_recovery_" + auth.ID

	_, err = postgresrepo.ReconcileFlexAuthorization(
		ctx,
		f.DB,
		auth.ID,
		providerID,
		postgresrepo.FlexResultProcessing,
	)
	if err != nil {
		t.Fatal(err)
	}

	return auth.ID, providerID, func() {}
}

func TestFlexRecoveryReconcileAuthorized(t *testing.T) {
	f := setupFlexAuthorizationTest(t)
	ctx := context.Background()

	auth, err := createTestFlexAuthorization(ctx, f)
	if err != nil {
		t.Fatal(err)
	}

	_, claimed, err := postgresrepo.ClaimFlexAuthorizationProcessing(
		ctx, f.DB, auth.ID,
	)
	if err != nil || !claimed {
		t.Fatalf("claim: claimed=%v err=%v", claimed, err)
	}

	providerID := "pi_recovery_authorized_" + auth.ID

	_, err = postgresrepo.ReconcileFlexAuthorization(
		ctx, f.DB, auth.ID, providerID,
		postgresrepo.FlexResultProcessing,
	)
	if err != nil {
		t.Fatal(err)
	}

	result, err := postgresrepo.ReconcileFlexAuthorizationRecovery(
		ctx, f.DB, auth.ID, providerID,
		postgresrepo.FlexResultAuthorized,
	)
	if err != nil {
		t.Fatal(err)
	}

	if result == nil ||
		result.Status != "AUTHORIZED" ||
		result.ProviderAuthorizationID == nil ||
		*result.ProviderAuthorizationID != providerID {
		t.Fatalf("unexpected recovery result: %+v", result)
	}

	var authorizedAtPresent bool

	err = f.DB.QueryRow(ctx, `
		SELECT authorized_at IS NOT NULL
		FROM flex_authorizations
		WHERE id = $1
	`, auth.ID).Scan(&authorizedAtPresent)
	if err != nil || !authorizedAtPresent {
		t.Fatalf("missing authorized_at: %v", err)
	}
}

func TestFlexRecoveryReconcileRejectsUnknownProvider(t *testing.T) {
	f := setupFlexAuthorizationTest(t)
	ctx := context.Background()

	auth, err := createTestFlexAuthorization(ctx, f)
	if err != nil {
		t.Fatal(err)
	}

	_, claimed, err := postgresrepo.ClaimFlexAuthorizationProcessing(
		ctx, f.DB, auth.ID,
	)
	if err != nil || !claimed {
		t.Fatalf("claim: claimed=%v err=%v", claimed, err)
	}

	_, err = postgresrepo.ReconcileFlexAuthorizationRecovery(
		ctx, f.DB, auth.ID, "pi_unpersisted",
		postgresrepo.FlexResultAuthorized,
	)

	if !errors.Is(
		err,
		postgresrepo.ErrFlexAuthorizationProviderConflict,
	) {
		t.Fatalf("expected missing provider rejection, got %v", err)
	}

	var status string
	var providerID *string

	err = f.DB.QueryRow(ctx, `
		SELECT status, provider_authorization_id
		FROM flex_authorizations
		WHERE id = $1
	`, auth.ID).Scan(&status, &providerID)
	if err != nil {
		t.Fatal(err)
	}

	if status != "PROCESSING" || providerID != nil {
		t.Fatalf(
			"unknown provider modified: status=%s provider=%v",
			status,
			providerID,
		)
	}
}

func TestFlexRecoveryReconcileRejectsProviderMismatch(t *testing.T) {
	f := setupFlexAuthorizationTest(t)
	ctx := context.Background()

	auth, err := createTestFlexAuthorization(ctx, f)
	if err != nil {
		t.Fatal(err)
	}

	_, claimed, err := postgresrepo.ClaimFlexAuthorizationProcessing(
		ctx, f.DB, auth.ID,
	)
	if err != nil || !claimed {
		t.Fatalf("claim: claimed=%v err=%v", claimed, err)
	}

	providerID := "pi_recovery_original_" + auth.ID

	_, err = postgresrepo.ReconcileFlexAuthorization(
		ctx, f.DB, auth.ID, providerID,
		postgresrepo.FlexResultProcessing,
	)
	if err != nil {
		t.Fatal(err)
	}

	_, err = postgresrepo.ReconcileFlexAuthorizationRecovery(
		ctx, f.DB, auth.ID, "pi_different",
		postgresrepo.FlexResultAuthorized,
	)

	if !errors.Is(
		err,
		postgresrepo.ErrFlexAuthorizationProviderConflict,
	) {
		t.Fatalf("expected provider mismatch rejection, got %v", err)
	}
}

func TestFlexRecoveryReconcileRejectsTerminalReplay(t *testing.T) {
	f := setupFlexAuthorizationTest(t)
	ctx := context.Background()

	auth, err := createTestFlexAuthorization(ctx, f)
	if err != nil {
		t.Fatal(err)
	}

	_, claimed, err := postgresrepo.ClaimFlexAuthorizationProcessing(
		ctx, f.DB, auth.ID,
	)
	if err != nil || !claimed {
		t.Fatalf("claim: claimed=%v err=%v", claimed, err)
	}

	providerID := "pi_recovery_terminal_" + auth.ID

	_, err = postgresrepo.ReconcileFlexAuthorization(
		ctx, f.DB, auth.ID, providerID,
		postgresrepo.FlexResultAuthorized,
	)
	if err != nil {
		t.Fatal(err)
	}

	_, err = postgresrepo.ReconcileFlexAuthorizationRecovery(
		ctx, f.DB, auth.ID, providerID,
		postgresrepo.FlexResultAuthorized,
	)

	if !errors.Is(
		err,
		postgresrepo.ErrFlexAuthorizationResultConflict,
	) {
		t.Fatalf("expected terminal replay rejection, got %v", err)
	}
}

func TestFlexRecoveryReconcileRejectsFailureOutcome(t *testing.T) {
	_, err := postgresrepo.ReconcileFlexAuthorizationRecovery(
		context.Background(),
		nil,
		"invalid",
		"pi_example",
		postgresrepo.FlexResultFailed,
	)

	if err == nil {
		t.Fatal("expected invalid dependencies rejection")
	}

	f := setupFlexAuthorizationTest(t)
	ctx := context.Background()

	auth, err := createTestFlexAuthorization(ctx, f)
	if err != nil {
		t.Fatal(err)
	}

	_, claimed, err := postgresrepo.ClaimFlexAuthorizationProcessing(
		ctx, f.DB, auth.ID,
	)
	if err != nil || !claimed {
		t.Fatalf("claim: claimed=%v err=%v", claimed, err)
	}

	providerID := "pi_recovery_failure_" + auth.ID

	_, err = postgresrepo.ReconcileFlexAuthorization(
		ctx, f.DB, auth.ID, providerID,
		postgresrepo.FlexResultProcessing,
	)
	if err != nil {
		t.Fatal(err)
	}

	_, err = postgresrepo.ReconcileFlexAuthorizationRecovery(
		ctx, f.DB, auth.ID, providerID,
		postgresrepo.FlexResultFailed,
	)

	if !errors.Is(
		err,
		postgresrepo.ErrFlexAuthorizationInvalidState,
	) {
		t.Fatalf("expected FAILED outcome rejection, got %v", err)
	}
}

func TestFlexRecoveryReconcileConcurrent(t *testing.T) {
	f := setupFlexAuthorizationTest(t)
	ctx := context.Background()

	auth, err := createTestFlexAuthorization(ctx, f)
	if err != nil {
		t.Fatal(err)
	}

	_, claimed, err := postgresrepo.ClaimFlexAuthorizationProcessing(
		ctx, f.DB, auth.ID,
	)
	if err != nil || !claimed {
		t.Fatalf("claim: claimed=%v err=%v", claimed, err)
	}

	providerID := "pi_recovery_concurrent_" + auth.ID

	_, err = postgresrepo.ReconcileFlexAuthorization(
		ctx, f.DB, auth.ID, providerID,
		postgresrepo.FlexResultProcessing,
	)
	if err != nil {
		t.Fatal(err)
	}

	const workers = 8

	var wg sync.WaitGroup
	results := make(chan error, workers)

	for i := 0; i < workers; i++ {
		wg.Add(1)

		go func() {
			defer wg.Done()

			_, err := postgresrepo.ReconcileFlexAuthorizationRecovery(
				ctx, f.DB, auth.ID, providerID,
				postgresrepo.FlexResultAuthorized,
			)

			results <- err
		}()
	}

	wg.Wait()
	close(results)

	successes := 0
	conflicts := 0

	for err := range results {
		switch {
		case err == nil:
			successes++

		case errors.Is(
			err,
			postgresrepo.ErrFlexAuthorizationResultConflict,
		):
			conflicts++

		default:
			t.Fatalf("unexpected concurrent error: %v", err)
		}
	}

	if successes != 1 || conflicts != workers-1 {
		t.Fatalf(
			"unexpected concurrency outcome: successes=%d conflicts=%d",
			successes,
			conflicts,
		)
	}

	var status string

	err = f.DB.QueryRow(ctx, `
		SELECT status
		FROM flex_authorizations
		WHERE id = $1
	`, auth.ID).Scan(&status)
	if err != nil {
		t.Fatal(err)
	}

	if status != "AUTHORIZED" {
		t.Fatalf("unexpected final status: %s", status)
	}
}
