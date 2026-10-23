package postgres_test

import (
	"context"
	"errors"
	"os"
	"testing"

	postgresrepo "github.com/JCKFinland/connect/backend/internal/repository/postgres"
	"github.com/google/uuid"
)

func requireFlexRecoveryLookupTestDB(t *testing.T) {
	t.Helper()

	if os.Getenv("DB_NAME") != "connect_settlement_test" ||
		os.Getenv("CONNECT_REQUIRE_SETTLEMENT_TEST_DB") != "1" ||
		os.Getenv("FLEX_SETTLEMENT_WORKER_ENABLED") != "false" {
		t.Fatal("isolated FLEX settlement test database guard is required")
	}
}

func TestFlexAuthorizationRecoveryLookupEligible(t *testing.T) {
	requireFlexRecoveryLookupTestDB(t)

	f := setupFlexAuthorizationTest(t)
	ctx := context.Background()

	authorization, err := createTestFlexAuthorization(ctx, f)
	if err != nil {
		t.Fatal(err)
	}

	_, claimed, err := postgresrepo.ClaimFlexAuthorizationProcessing(
		ctx, f.DB, authorization.ID,
	)
	if err != nil || !claimed {
		t.Fatalf("claim authorization: claimed=%v err=%v", claimed, err)
	}

	providerID := "pi_flex_recovery_lookup_" + uuid.NewString()

	_, err = postgresrepo.ReconcileFlexAuthorization(
		ctx,
		f.DB,
		authorization.ID,
		providerID,
		postgresrepo.FlexResultProcessing,
	)
	if err != nil {
		t.Fatal(err)
	}

	recovered, err := postgresrepo.GetFlexAuthorizationForRecovery(
		ctx, f.DB, authorization.ID,
	)
	if err != nil {
		t.Fatal(err)
	}

	if recovered == nil ||
		recovered.ID != authorization.ID ||
		recovered.AuctionID != authorization.AuctionID ||
		recovered.OfferID != authorization.OfferID ||
		recovered.PassengerID != authorization.PassengerID ||
		recovered.AmountCents != authorization.AmountCents ||
		recovered.Currency != authorization.Currency ||
		recovered.IdempotencyKey != authorization.IdempotencyKey ||
		recovered.Provider != "STRIPE" ||
		recovered.Status != "PROCESSING" ||
		recovered.ProviderAuthorizationID == nil ||
		*recovered.ProviderAuthorizationID != providerID {
		t.Fatalf("unexpected recovery authorization: %+v", recovered)
	}

	var persistedStatus string
	var persistedProviderID string

	err = f.DB.QueryRow(ctx, `
		SELECT status, provider_authorization_id
		FROM flex_authorizations
		WHERE id = $1
	`, authorization.ID).Scan(
		&persistedStatus,
		&persistedProviderID,
	)
	if err != nil {
		t.Fatal(err)
	}

	if persistedStatus != "PROCESSING" ||
		persistedProviderID != providerID {
		t.Fatal("recovery lookup unexpectedly changed authorization")
	}
}

func TestFlexAuthorizationRecoveryLookupRejectsIneligible(t *testing.T) {
	requireFlexRecoveryLookupTestDB(t)

	ctx := context.Background()

	t.Run("pending authorization", func(t *testing.T) {
		f := setupFlexAuthorizationTest(t)

		authorization, err := createTestFlexAuthorization(ctx, f)
		if err != nil {
			t.Fatal(err)
		}

		_, err = postgresrepo.GetFlexAuthorizationForRecovery(
			ctx, f.DB, authorization.ID,
		)
		if !errors.Is(err, postgresrepo.ErrFlexAuthorizationNotFound) {
			t.Fatalf("expected ineligible lookup rejection, got %v", err)
		}
	})

	t.Run("processing without provider identity", func(t *testing.T) {
		f := setupFlexAuthorizationTest(t)

		authorization, err := createTestFlexAuthorization(ctx, f)
		if err != nil {
			t.Fatal(err)
		}

		_, claimed, err := postgresrepo.ClaimFlexAuthorizationProcessing(
			ctx, f.DB, authorization.ID,
		)
		if err != nil || !claimed {
			t.Fatalf("claim authorization: claimed=%v err=%v", claimed, err)
		}

		_, err = postgresrepo.GetFlexAuthorizationForRecovery(
			ctx, f.DB, authorization.ID,
		)
		if !errors.Is(err, postgresrepo.ErrFlexAuthorizationNotFound) {
			t.Fatalf("expected unknown-provider rejection, got %v", err)
		}
	})

	t.Run("authorized terminal state", func(t *testing.T) {
		f := setupFlexAuthorizationTest(t)

		authorization, err := createTestFlexAuthorization(ctx, f)
		if err != nil {
			t.Fatal(err)
		}

		_, claimed, err := postgresrepo.ClaimFlexAuthorizationProcessing(
			ctx, f.DB, authorization.ID,
		)
		if err != nil || !claimed {
			t.Fatalf("claim authorization: claimed=%v err=%v", claimed, err)
		}

		_, err = postgresrepo.ReconcileFlexAuthorization(
			ctx,
			f.DB,
			authorization.ID,
			"pi_flex_recovery_terminal_"+uuid.NewString(),
			postgresrepo.FlexResultAuthorized,
		)
		if err != nil {
			t.Fatal(err)
		}

		_, err = postgresrepo.GetFlexAuthorizationForRecovery(
			ctx, f.DB, authorization.ID,
		)
		if !errors.Is(err, postgresrepo.ErrFlexAuthorizationNotFound) {
			t.Fatalf("expected terminal rejection, got %v", err)
		}
	})

	t.Run("invalid authorization UUID", func(t *testing.T) {
		f := setupFlexAuthorizationTest(t)

		_, err := postgresrepo.GetFlexAuthorizationForRecovery(
			ctx, f.DB, "invalid-uuid",
		)
		if !errors.Is(err, postgresrepo.ErrFlexAuthorizationNotFound) {
			t.Fatalf("expected invalid UUID rejection, got %v", err)
		}
	})
}

func TestFlexAuthorizationRecoveryLookupRejectsInvalidDependencies(t *testing.T) {
	ctx := context.Background()

	if _, err := postgresrepo.GetFlexAuthorizationForRecovery(
		ctx, nil, uuid.NewString(),
	); err == nil {
		t.Fatal("expected nil database rejection")
	}

	if _, err := postgresrepo.GetFlexAuthorizationForRecovery(
		nil, nil, uuid.NewString(),
	); err == nil {
		t.Fatal("expected nil context rejection")
	}
}
