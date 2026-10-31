package postgres

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/JCKFinland/connect/backend/internal/payouts"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestStripeAccountLinkSessionCreatePostgresRollback(t *testing.T) {
	const databaseName = "connect_payout_destination_test"

	if os.Getenv("DB_NAME") != databaseName ||
		os.Getenv("FLEX_SETTLEMENT_WORKER_ENABLED") != "false" ||
		os.Getenv("CONNECT_REQUIRE_SETTLEMENT_TEST_DB") != "0" {
		t.Fatal("unsafe integration test environment")
	}

	ctx, cancel := context.WithTimeout(
		context.Background(), 60*time.Second,
	)
	defer cancel()

	envOr := func(name, fallback string) string {
		if value := os.Getenv(name); value != "" {
			return value
		}
		return fallback
	}

	cfg, err := pgxpool.ParseConfig(fmt.Sprintf(
		"host=%s port=%s user=%s dbname=%s sslmode=disable",
		envOr("DB_HOST", "localhost"),
		envOr("DB_PORT", "5432"),
		envOr("DB_USER", "postgres"),
		databaseName,
	))
	if err != nil {
		t.Fatal(err)
	}

	cfg.MaxConns = 2

	db, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	checkBaseline := func() {
		t.Helper()

		var safe bool
		err := db.QueryRow(ctx, `
            SELECT current_database() = $1
              AND (
                  SELECT MAX(version_id)
                  FROM goose_db_version
                  WHERE is_applied
              ) = 75
              AND to_regclass(
                  'public.stripe_connect_ownership_bindings'
              ) IS NULL
              AND to_regclass(
                  'public.stripe_connect_account_link_sessions'
              ) IS NULL
        `, databaseName).Scan(&safe)

		if err != nil || !safe {
			t.Fatalf("unsafe database baseline: %v %v", safe, err)
		}
	}

	checkBaseline()

	migration, err := os.ReadFile(
		"../../../migrations/00078_create_stripe_connect_account_link_sessions.sql",
	)
	if err != nil {
		t.Fatal(err)
	}

	parts := strings.SplitN(
		string(migration), "-- +goose Up", 2,
	)
	if len(parts) != 2 {
		t.Fatal("missing goose Up marker")
	}

	upParts := strings.SplitN(parts[1], "-- +goose Down", 2)
	if len(upParts) != 2 {
		t.Fatal("missing goose Down marker")
	}

	tx, err := db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		t.Fatal(err)
	}

	rolledBack := false
	defer func() {
		if !rolledBack {
			cleanupCtx, stop := context.WithTimeout(
				context.Background(), 15*time.Second,
			)
			defer stop()

			if err := tx.Rollback(cleanupCtx); err != nil &&
				!errors.Is(err, pgx.ErrTxClosed) {
				t.Errorf("rollback failed: %v", err)
			}
		}
	}()

	if _, err := tx.Exec(
		ctx, "SET LOCAL statement_timeout = '20s'",
	); err != nil {
		t.Fatal(err)
	}

	if _, err := tx.Exec(
		ctx, "SET LOCAL lock_timeout = '3s'",
	); err != nil {
		t.Fatal(err)
	}

	if _, err := tx.Exec(
		ctx, upParts[0], pgx.QueryExecModeSimpleProtocol,
	); err != nil {
		t.Fatalf("apply transactional migration: %v", err)
	}

	actorID := uuid.NewString()
	driverID := uuid.NewString()
	companyID := uuid.NewString()
	accountID := "acct_" + strings.ReplaceAll(uuid.NewString(), "-", "")

	_, err = tx.Exec(ctx, `
        INSERT INTO users (
            id, email, password_hash, first_name, last_name
        ) VALUES
            ($1, $2, 'synthetic-test-only', 'Test', 'Actor'),
            ($3, $4, 'synthetic-test-only', 'Test', 'Driver')
    `,
		actorID, "create-"+actorID+"@example.invalid",
		driverID, "create-"+driverID+"@example.invalid",
	)
	if err != nil {
		t.Fatalf("insert users: %v", err)
	}

	_, err = tx.Exec(ctx, `
        INSERT INTO companies (
            id, name, legal_name, country_code, timezone
        ) VALUES ($1, $2, $2, 'FI', 'Europe/Helsinki')
    `, companyID, "Create Test "+companyID)
	if err != nil {
		t.Fatalf("insert company: %v", err)
	}

	request := payouts.StripeConnectOwnershipRequest{
		ActorUserID:  actorID,
		DriverUserID: driverID,
		CompanyID:    companyID,
		AccountID:    accountID,
	}

	creator := NewStripeAccountLinkSessionCreator(tx)
	claimer := NewStripeAccountLinkSessionClaimer(tx)

	sessionID, err := creator.Create(ctx, request)
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	if !validStripeLinkClaimUUID(sessionID) {
		t.Fatalf("invalid returned session ID: %q", sessionID)
	}

	var (
		storedActor         string
		storedDriver        string
		storedCompany       string
		storedAccount       string
		lifetimeSeconds     float64
		initiallyUnconsumed bool
	)

	err = tx.QueryRow(ctx, `
        SELECT
            actor_user_id,
            driver_user_id,
            company_id,
            stripe_account_id,
            EXTRACT(EPOCH FROM (expires_at - created_at)),
            consumed_at IS NULL
        FROM stripe_connect_account_link_sessions
        WHERE id = $1::uuid
    `, sessionID).Scan(
		&storedActor,
		&storedDriver,
		&storedCompany,
		&storedAccount,
		&lifetimeSeconds,
		&initiallyUnconsumed,
	)
	if err != nil {
		t.Fatalf("read persisted session: %v", err)
	}

	if storedActor != actorID ||
		storedDriver != driverID ||
		storedCompany != companyID ||
		storedAccount != accountID ||
		lifetimeSeconds != 300 ||
		!initiallyUnconsumed {
		t.Fatal("persisted session identity or lifetime mismatch")
	}

	invalidRequests := []payouts.StripeConnectOwnershipRequest{
		{
			ActorUserID:  "",
			DriverUserID: driverID,
			CompanyID:    companyID,
			AccountID:    accountID,
		},
		{
			ActorUserID:  actorID,
			DriverUserID: uuid.Nil.String(),
			CompanyID:    companyID,
			AccountID:    accountID,
		},
		{
			ActorUserID:  actorID,
			DriverUserID: driverID,
			CompanyID:    "not-a-uuid",
			AccountID:    accountID,
		},
		{
			ActorUserID:  actorID,
			DriverUserID: driverID,
			CompanyID:    companyID,
			AccountID:    "invalid-account",
		},
	}

	for index, invalid := range invalidRequests {
		created, err := creator.Create(ctx, invalid)
		if created != "" ||
			!errors.Is(
				err,
				payouts.ErrStripeConnectOwnershipNotVerified,
			) {
			t.Fatalf(
				"invalid request %d accepted: id=%q err=%v",
				index, created, err,
			)
		}
	}

	var count int
	if err := tx.QueryRow(ctx, `
        SELECT COUNT(*)
        FROM stripe_connect_account_link_sessions
    `).Scan(&count); err != nil {
		t.Fatal(err)
	}

	if count != 1 {
		t.Fatalf("invalid requests created sessions: %d", count)
	}

	claimed, err := claimer.Claim(ctx, sessionID, request)
	if err != nil || !claimed {
		t.Fatalf("valid claim failed: claimed=%v err=%v", claimed, err)
	}

	claimed, err = claimer.Claim(ctx, sessionID, request)
	if err != nil || claimed {
		t.Fatalf("replay accepted: claimed=%v err=%v", claimed, err)
	}

	if err := tx.Rollback(ctx); err != nil {
		t.Fatalf("rollback failed: %v", err)
	}
	rolledBack = true

	checkBaseline()

	t.Log("session creation validated, single-use claim enforced, transaction rolled back")
}
