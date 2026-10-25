package flex_authorization

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	stripepayment "github.com/JCKFinland/connect/backend/internal/payments/stripe"
	postgresrepo "github.com/JCKFinland/connect/backend/internal/repository/postgres"
	"github.com/jackc/pgx/v5/pgxpool"
	stripego "github.com/stripe/stripe-go/v86"
)

// scopedRecoveryDiscovery delegates to the production PostgreSQL
// discovery implementation but exposes only this test's authorization.
//
// This prevents an integration test from executing recovery against
// unrelated financial records in the settlement test database.
type scopedRecoveryDiscovery struct {
	db              *pgxpool.Pool
	authorizationID string
}

func (d scopedRecoveryDiscovery) Discover(
	ctx context.Context,
	staleAfter time.Duration,
	batchSize int,
) ([]postgresrepo.FlexAuthorizationRecoveryCandidate, error) {
	candidates, err :=
		postgresrepo.DiscoverFlexAuthorizationRecoveryCandidates(
			ctx,
			d.db,
			staleAfter,
			postgresrepo.MaxFlexAuthorizationRecoveryBatchSize,
		)
	if err != nil {
		return nil, err
	}

	for _, candidate := range candidates {
		if candidate.AuthorizationID == d.authorizationID {
			return []postgresrepo.FlexAuthorizationRecoveryCandidate{
				candidate,
			}, nil
		}
	}

	return nil, nil
}

func TestAuthorizationRecoveryWorkerPostgresIntegration(
	t *testing.T,
) {
	if os.Getenv("APP_ENV") != "test" ||
		os.Getenv("DB_NAME") != "connect_settlement_test" ||
		os.Getenv("CONNECT_REQUIRE_SETTLEMENT_TEST_DB") != "1" ||
		os.Getenv("FLEX_SETTLEMENT_WORKER_ENABLED") != "false" ||
		os.Getenv("FLEX_AUTHORIZATION_RECOVERY_WORKER_ENABLED") != "false" {
		t.Fatal("isolated settlement test environment required")
	}

	f, authorizationID, providerID :=
		setupKnownProviderRecoveryTest(t)

	ctx := context.Background()

	var databaseName string
	if err := f.DB.QueryRow(
		ctx,
		"SELECT current_database()",
	).Scan(&databaseName); err != nil {
		t.Fatal(err)
	}

	if databaseName != "connect_settlement_test" {
		t.Fatalf("unsafe recovery database: %s", databaseName)
	}

	// Make only this fixture eligible for stale recovery.
	tag, err := f.DB.Exec(ctx, `
        UPDATE flex_authorizations
        SET updated_at = clock_timestamp() - INTERVAL '20 minutes'
        WHERE id = $1
          AND status = 'PROCESSING'
          AND provider_authorization_id = $2
    `, authorizationID, providerID)
	if err != nil {
		t.Fatal(err)
	}
	if tag.RowsAffected() != 1 {
		t.Fatal("failed to prepare stale recovery fixture")
	}

	retrievalCalls := 0

	adapter := recoveryReconcileAdapter{
		execute: func(
			ctx context.Context,
			request stripepayment.FlexAuthorizationRequest,
		) (*stripepayment.FlexAuthorizationResult, error) {
			retrievalCalls++

			if request.AuthorizationID != authorizationID ||
				request.ProviderAuthorizationID != providerID {
				return nil, errors.New(
					"recovery attempted an unexpected authorization",
				)
			}

			return &stripepayment.FlexAuthorizationResult{
				ProviderAuthorizationID: providerID,
				ProviderStatus:          stripego.PaymentIntentStatusRequiresCapture,
				Authorized:              true,
			}, nil
		},
	}

	service := NewRecoveryService(f.DB, adapter)

	worker, err := NewAuthorizationRecoveryWorker(
		scopedRecoveryDiscovery{
			db:              f.DB,
			authorizationID: authorizationID,
		},
		service,
		5*time.Minute,
		1,
	)
	if err != nil {
		t.Fatal(err)
	}

	var recoveryErrors []error

	err = worker.RunOnce(ctx, func(err error) {
		recoveryErrors = append(recoveryErrors, err)
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(recoveryErrors) != 0 {
		t.Fatalf("recovery errors: %v", recoveryErrors)
	}

	var status string
	var storedProviderID string

	err = f.DB.QueryRow(ctx, `
        SELECT status, provider_authorization_id
        FROM flex_authorizations
        WHERE id = $1
    `, authorizationID).Scan(&status, &storedProviderID)
	if err != nil {
		t.Fatal(err)
	}

	if status != "AUTHORIZED" ||
		storedProviderID != providerID {
		t.Fatalf(
			"unexpected recovery state: status=%s provider=%s",
			status,
			storedProviderID,
		)
	}

	if retrievalCalls != 1 {
		t.Fatalf(
			"expected one retrieval, got %d",
			retrievalCalls,
		)
	}

	// A terminal authorization must not be rediscovered.
	err = worker.RunOnce(ctx, func(err error) {
		recoveryErrors = append(recoveryErrors, err)
	})
	if err != nil {
		t.Fatal(err)
	}

	if len(recoveryErrors) != 0 {
		t.Fatalf("unexpected repeat recovery errors: %v", recoveryErrors)
	}

	if retrievalCalls != 1 {
		t.Fatalf(
			"terminal authorization retrieved again: %d",
			retrievalCalls,
		)
	}
}
