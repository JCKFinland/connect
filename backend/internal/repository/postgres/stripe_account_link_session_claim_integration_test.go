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

func TestStripeAccountLinkSessionClaimPostgresRollback(t *testing.T) {
	const databaseName = "connect_payout_destination_test"

	if os.Getenv("DB_NAME") != databaseName {
		t.Fatal("isolated test database is required")
	}
	if os.Getenv("FLEX_SETTLEMENT_WORKER_ENABLED") != "false" {
		t.Fatal("settlement worker must be disabled")
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
			t.Fatalf("unsafe database baseline: safe=%v err=%v", safe, err)
		}
	}
	checkBaseline()

	migration, err := os.ReadFile(
		"../../../migrations/00078_create_stripe_connect_account_link_sessions.sql",
	)
	if err != nil {
		t.Fatal(err)
	}
	source := string(migration)
	if strings.Count(source, "-- +goose Up") != 1 ||
		strings.Count(source, "-- +goose Down") != 1 {
		t.Fatal("invalid migration sections")
	}

	up := strings.SplitN(
		strings.SplitN(source, "-- +goose Up", 2)[1],
		"-- +goose Down", 2,
	)[0]

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

	if _, err := tx.Exec(ctx, "SET LOCAL statement_timeout = '20s'"); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, "SET LOCAL lock_timeout = '3s'"); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(
		ctx, up, pgx.QueryExecModeSimpleProtocol,
	); err != nil {
		t.Fatalf("transactional migration failed: %v", err)
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
		actorID, "link-"+actorID+"@example.invalid",
		driverID, "link-"+driverID+"@example.invalid",
	)
	if err != nil {
		t.Fatalf("insert users: %v", err)
	}

	_, err = tx.Exec(ctx, `
        INSERT INTO companies (
            id, name, legal_name, country_code, timezone
        ) VALUES ($1, $2, $2, 'FI', 'Europe/Helsinki')
    `, companyID, "Link Test "+companyID)
	if err != nil {
		t.Fatalf("insert company: %v", err)
	}

	request := payouts.StripeConnectOwnershipRequest{
		ActorUserID:  actorID,
		DriverUserID: driverID,
		CompanyID:    companyID,
		AccountID:    accountID,
	}

	insertSession := func() string {
		t.Helper()
		var sessionID string
		err := tx.QueryRow(ctx, `
            INSERT INTO stripe_connect_account_link_sessions (
                actor_user_id,
                driver_user_id,
                company_id,
                stripe_account_id,
                expires_at
            ) VALUES (
                $1, $2, $3, $4,
                clock_timestamp() + INTERVAL '5 minutes'
            )
            RETURNING id
        `, actorID, driverID, companyID, accountID).Scan(&sessionID)
		if err != nil {
			t.Fatalf("insert session: %v", err)
		}
		return sessionID
	}

	claimer := NewStripeAccountLinkSessionClaimer(tx)

	assertRejected := func(
		name, sessionID string,
		current payouts.StripeConnectOwnershipRequest,
	) {
		t.Helper()
		claimed, err := claimer.Claim(ctx, sessionID, current)
		if err != nil || claimed {
			t.Fatalf(
				"%s must reject: claimed=%v err=%v",
				name, claimed, err,
			)
		}
	}

	sessionID := insertSession()

	wrong := request
	wrong.ActorUserID = driverID
	assertRejected("wrong actor", sessionID, wrong)

	wrong = request
	wrong.DriverUserID = actorID
	assertRejected("wrong driver", sessionID, wrong)

	wrong = request
	wrong.CompanyID = uuid.NewString()
	assertRejected("wrong company", sessionID, wrong)

	wrong = request
	wrong.AccountID = "acct_" + strings.ReplaceAll(
		uuid.NewString(), "-", "",
	)
	assertRejected("wrong account", sessionID, wrong)

	assertRejected("wrong session", uuid.NewString(), request)

	var consumedAt *time.Time
	if err := tx.QueryRow(ctx, `
        SELECT consumed_at
        FROM stripe_connect_account_link_sessions
        WHERE id = $1
    `, sessionID).Scan(&consumedAt); err != nil {
		t.Fatal(err)
	}
	if consumedAt != nil {
		t.Fatal("mismatched claims consumed session")
	}

	claimed, err := claimer.Claim(ctx, sessionID, request)
	if err != nil || !claimed {
		t.Fatalf("valid claim failed: claimed=%v err=%v", claimed, err)
	}

	assertRejected("replay", sessionID, request)

	if err := tx.QueryRow(ctx, `
        SELECT consumed_at
        FROM stripe_connect_account_link_sessions
        WHERE id = $1
    `, sessionID).Scan(&consumedAt); err != nil {
		t.Fatal(err)
	}
	if consumedAt == nil {
		t.Fatal("successful claim did not persist consumption in transaction")
	}

	secondSession := insertSession()
	malformed := request
	malformed.AccountID = "not-a-stripe-account"
	malformedClaimed, malformedErr := claimer.Claim(
		ctx, secondSession, malformed,
	)
	if malformedClaimed ||
		!errors.Is(
			malformedErr,
			payouts.ErrStripeConnectOwnershipNotVerified,
		) {
		t.Fatalf(
			"malformed account not rejected: claimed=%v err=%v",
			malformedClaimed, malformedErr,
		)
	}

	malformedClaimed, malformedErr = claimer.Claim(
		ctx, "not-a-uuid", request,
	)
	if malformedClaimed ||
		!errors.Is(
			malformedErr,
			payouts.ErrStripeConnectOwnershipNotVerified,
		) {
		t.Fatalf(
			"malformed session ID not rejected: claimed=%v err=%v",
			malformedClaimed, malformedErr,
		)
	}

	if err := tx.QueryRow(ctx, `
        SELECT consumed_at
        FROM stripe_connect_account_link_sessions
        WHERE id = $1
    `, secondSession).Scan(&consumedAt); err != nil {
		t.Fatal(err)
	}
	if consumedAt != nil {
		t.Fatal("malformed request consumed session")
	}

	// A session must remain unconsumed after its expiry deadline.
	var expiredSessionID string
	err = tx.QueryRow(ctx, `
        INSERT INTO stripe_connect_account_link_sessions (
            actor_user_id,
            driver_user_id,
            company_id,
            stripe_account_id,
            expires_at
        ) VALUES (
            $1, $2, $3, $4,
            clock_timestamp() + INTERVAL '2 seconds'
        )
        RETURNING id
    `, actorID, driverID, companyID, accountID).Scan(&expiredSessionID)
	if err != nil {
		t.Fatalf("insert short-lived session: %v", err)
	}

	if _, err := tx.Exec(ctx, "SELECT pg_sleep(3)"); err != nil {
		t.Fatalf("wait for session expiry: %v", err)
	}

	assertRejected("expired session", expiredSessionID, request)

	var expiredConsumedAt *time.Time
	if err := tx.QueryRow(ctx, `
        SELECT consumed_at
        FROM stripe_connect_account_link_sessions
        WHERE id = $1
    `, expiredSessionID).Scan(&expiredConsumedAt); err != nil {
		t.Fatalf("read expired session: %v", err)
	}
	if expiredConsumedAt != nil {
		t.Fatal("expired session was consumed")
	}

	if err := tx.Rollback(ctx); err != nil {
		t.Fatalf("rollback failed: %v", err)
	}
	rolledBack = true

	checkBaseline()

	t.Log("PASS: exact matching session consumed once")
	t.Log("PASS: replay and identity mismatches rejected")
	t.Log("PASS: malformed requests fail closed")
	t.Log("PASS: migration and fixtures rolled back")
}
