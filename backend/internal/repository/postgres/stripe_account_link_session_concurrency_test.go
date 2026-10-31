package postgres

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/JCKFinland/connect/backend/internal/payouts"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestStripeAccountLinkSessionConcurrentClaims(t *testing.T) {
	if os.Getenv("DB_NAME") != "connect_payout_destination_test" ||
		os.Getenv("FLEX_SETTLEMENT_WORKER_ENABLED") != "false" {
		t.Fatal("unsafe test environment")
	}

	ctx, cancel := context.WithTimeout(
		context.Background(), 45*time.Second,
	)
	defer cancel()

	envOr := func(key, fallback string) string {
		if value := os.Getenv(key); value != "" {
			return value
		}
		return fallback
	}

	cfg, err := pgxpool.ParseConfig(fmt.Sprintf(
		"host=%s port=%s user=%s dbname=connect_payout_destination_test sslmode=disable",
		envOr("DB_HOST", "localhost"),
		envOr("DB_PORT", "5432"),
		envOr("DB_USER", "postgres"),
	))
	if err != nil {
		t.Fatal(err)
	}
	cfg.MaxConns = 4

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	schema := "stripe_link_race_" + strings.ReplaceAll(
		uuid.NewString(), "-", "",
	)

	if _, err := pool.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatalf("create isolated schema: %v", err)
	}

	// Cleanup is deliberately ordered: remove the schema and its
	// foreign keys before deleting synthetic public identities.
	var actorID, driverID, companyID string
	defer func() {
		cleanupCtx, stop := context.WithTimeout(
			context.Background(), 15*time.Second,
		)
		defer stop()

		if _, err := pool.Exec(
			cleanupCtx, "DROP SCHEMA "+schema+" CASCADE",
		); err != nil {
			t.Errorf("isolated schema cleanup failed: %v", err)
			return
		}

		if companyID != "" {
			if _, err := pool.Exec(cleanupCtx,
				"DELETE FROM public.companies WHERE id = $1",
				companyID,
			); err != nil {
				t.Errorf("synthetic company cleanup failed: %v", err)
			}
		}

		for _, userID := range []string{actorID, driverID} {
			if userID == "" {
				continue
			}
			if _, err := pool.Exec(cleanupCtx,
				"DELETE FROM public.users WHERE id = $1",
				userID,
			); err != nil {
				t.Errorf("synthetic user cleanup failed: %v", err)
			}
		}
	}()

	migration, err := os.ReadFile(
		"../../../migrations/00078_create_stripe_connect_account_link_sessions.sql",
	)
	if err != nil {
		t.Fatal(err)
	}

	parts := strings.SplitN(
		string(migration), "-- +goose Down", 2,
	)
	if len(parts) != 2 {
		t.Fatal("invalid migration sections")
	}
	up := strings.SplitN(parts[0], "-- +goose Up", 2)
	if len(up) != 2 {
		t.Fatal("migration Up missing")
	}

	setup, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer setup.Release()

	if _, err := setup.Exec(
		ctx, "SET search_path TO "+schema+", public",
	); err != nil {
		t.Fatal(err)
	}

	if _, err := setup.Exec(
		ctx, up[1], pgx.QueryExecModeSimpleProtocol,
	); err != nil {
		t.Fatalf("apply isolated migration: %v", err)
	}

	actorID = uuid.NewString()
	driverID = uuid.NewString()
	companyID = uuid.NewString()
	accountID := "acct_" + strings.ReplaceAll(
		uuid.NewString(), "-", "",
	)

	_, err = setup.Exec(ctx, `
        INSERT INTO public.users (
            id, email, password_hash, first_name, last_name
        ) VALUES
            ($1, $2, 'synthetic-test-only', 'Test', 'Actor'),
            ($3, $4, 'synthetic-test-only', 'Test', 'Driver')
    `,
		actorID, "race-"+actorID+"@example.invalid",
		driverID, "race-"+driverID+"@example.invalid",
	)
	if err != nil {
		t.Fatalf("insert synthetic users: %v", err)
	}

	_, err = setup.Exec(ctx, `
        INSERT INTO public.companies (
            id, name, legal_name, country_code, timezone
        ) VALUES ($1, $2, $2, 'FI', 'Europe/Helsinki')
    `, companyID, "Race Test "+companyID)
	if err != nil {
		t.Fatalf("insert synthetic company: %v", err)
	}

	var sessionID string
	err = setup.QueryRow(ctx, `
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

	request := payouts.StripeConnectOwnershipRequest{
		ActorUserID:  actorID,
		DriverUserID: driverID,
		CompanyID:    companyID,
		AccountID:    accountID,
	}

	connections := make([]*pgxpool.Conn, 0, 2)
	for i := 0; i < 2; i++ {
		conn, err := pool.Acquire(ctx)
		if err != nil {
			t.Fatal(err)
		}
		connections = append(connections, conn)
		defer conn.Release()

		if _, err := conn.Exec(
			ctx, "SET search_path TO "+schema+", public",
		); err != nil {
			t.Fatal(err)
		}
	}

	type result struct {
		claimed bool
		err     error
	}

	results := make(chan result, 2)
	start := make(chan struct{})
	var wg sync.WaitGroup

	for _, connection := range connections {
		wg.Add(1)
		go func(conn *pgxpool.Conn) {
			defer wg.Done()
			<-start
			claimer := NewStripeAccountLinkSessionClaimer(conn)
			claimed, err := claimer.Claim(
				ctx, sessionID, request,
			)
			results <- result{claimed: claimed, err: err}
		}(connection)
	}

	close(start)
	wg.Wait()
	close(results)

	successes := 0
	rejections := 0

	for outcome := range results {
		if outcome.err != nil {
			t.Fatalf("concurrent claim error: %v", outcome.err)
		}
		if outcome.claimed {
			successes++
		} else {
			rejections++
		}
	}

	if successes != 1 || rejections != 1 {
		t.Fatalf(
			"expected exactly one winner: successes=%d rejections=%d",
			successes, rejections,
		)
	}

	var consumedAt *time.Time
	if err := setup.QueryRow(ctx, `
        SELECT consumed_at
        FROM stripe_connect_account_link_sessions
        WHERE id = $1
    `, sessionID).Scan(&consumedAt); err != nil {
		t.Fatal(err)
	}
	if consumedAt == nil {
		t.Fatal("winning claim did not consume session")
	}

	t.Log("PASS: two connections competed for one session")
	t.Log("PASS: exactly one claim succeeded")
	t.Log("PASS: losing claim rejected")
	t.Log("PASS: session consumed exactly once")

	_ = errors.Is
}
