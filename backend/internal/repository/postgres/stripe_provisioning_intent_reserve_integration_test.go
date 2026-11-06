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

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestStripeProvisioningIntentConcurrentReservation(t *testing.T) {
	if os.Getenv("DB_NAME") != "connect_settlement_test" ||
		os.Getenv("CONNECT_REQUIRE_SETTLEMENT_TEST_DB") != "1" ||
		os.Getenv("FLEX_SETTLEMENT_WORKER_ENABLED") != "false" {
		t.Fatal("unsafe integration environment")
	}

	ctx, cancel := context.WithTimeout(
		context.Background(), 90*time.Second,
	)
	defer cancel()

	envOr := func(key, fallback string) string {
		if value := os.Getenv(key); value != "" {
			return value
		}
		return fallback
	}

	cfg, err := pgxpool.ParseConfig(fmt.Sprintf(
		"host=%s port=%s user=%s dbname=%s sslmode=disable",
		envOr("DB_HOST", "localhost"),
		envOr("DB_PORT", "5432"),
		envOr("DB_USER", "postgres"),
		"connect_settlement_test",
	))
	if err != nil {
		t.Fatal(err)
	}

	// pgx does not automatically read DB_PASSWORD.
	password := os.Getenv("DB_PASSWORD")
	if password == "" {
		t.Fatal("isolated database password unavailable")
	}
	cfg.ConnConfig.Password = password

	cfg.MaxConns = 5
	schema := "stripe_provision_test_" +
		strings.ReplaceAll(uuid.NewString(), "-", "")

	// Every connection sees the same isolated migration objects.
	cfg.AfterConnect = func(
		ctx context.Context, conn *pgx.Conn,
	) error {
		_, err := conn.Exec(
			ctx, "SET search_path TO "+schema+", public",
		)
		return err
	}

	db, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	var safe bool
	err = db.QueryRow(ctx, `
        SELECT current_database() = 'connect_settlement_test'
          AND (SELECT MAX(version_id)
               FROM public.goose_db_version
               WHERE is_applied) = 79
          AND to_regclass(
               'public.stripe_connect_provisioning_intents'
          ) IS NOT NULL
    `).Scan(&safe)
	if err != nil || !safe {
		t.Fatalf("unsafe database baseline: %v %v", safe, err)
	}

	// The schema is the only persistent test fixture.
	if _, err := db.Exec(
		ctx, "CREATE SCHEMA "+schema,
	); err != nil {
		t.Fatal(err)
	}

	var actorID, driverID, companyID string

	t.Cleanup(func() {
		defer db.Close()

		var dropErr error
		for attempt := 1; attempt <= 3; attempt++ {
			// Use a fresh timeout for each schema-drop attempt.
			dropCtx, stop := context.WithTimeout(
				context.Background(), 10*time.Second,
			)
			_, dropErr = db.Exec(
				dropCtx, "DROP SCHEMA "+schema+" CASCADE",
			)
			stop()
			if dropErr == nil {
				break
			}
			if attempt < 3 {
				time.Sleep(200 * time.Millisecond)
			}
		}
		if dropErr != nil {
			t.Errorf("isolated schema cleanup failed after retries: %v", dropErr)
			return
		}

		cleanupCtx, stop := context.WithTimeout(
			context.Background(), 20*time.Second,
		)
		defer stop()

		// Remove the isolated schema before its FK identities.
		if companyID != "" {
			if _, err := db.Exec(cleanupCtx,
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
			if _, err := db.Exec(cleanupCtx,
				"DELETE FROM public.users WHERE id = $1",
				userID,
			); err != nil {
				t.Errorf("synthetic user cleanup failed: %v", err)
			}
		}
	})

	migration, err := os.ReadFile(
		"../../../migrations/" +
			"00079_create_stripe_connect_provisioning_intents.sql",
	)
	if err != nil {
		t.Fatal(err)
	}

	text := string(migration)
	if strings.Count(text, "-- +goose Up") != 1 ||
		strings.Count(text, "-- +goose Down") != 1 {
		t.Fatal("invalid migration sections")
	}

	up := strings.SplitN(
		strings.SplitN(text, "-- +goose Up", 2)[1],
		"-- +goose Down", 2,
	)[0]

	up = strings.ReplaceAll(up, "-- +goose StatementBegin", "")
	up = strings.ReplaceAll(up, "-- +goose StatementEnd", "")

	if _, err := db.Exec(
		ctx, up, pgx.QueryExecModeSimpleProtocol,
	); err != nil {
		t.Fatalf("apply isolated migration: %v", err)
	}

	// Unique synthetic identities satisfy isolated migration FKs.
	actorID = uuid.NewString()
	driverID = uuid.NewString()
	companyID = uuid.NewString()

	_, err = db.Exec(ctx, `
		INSERT INTO public.users (
			id, email, password_hash, first_name, last_name
		) VALUES
			($1, $2, 'synthetic-test-only', 'Test', 'Actor'),
			($3, $4, 'synthetic-test-only', 'Test', 'Driver')
	`,
		actorID, "reservation-"+actorID+"@example.invalid",
		driverID, "reservation-"+driverID+"@example.invalid",
	)
	if err != nil {
		t.Fatalf("insert synthetic users: %v", err)
	}

	_, err = db.Exec(ctx, `
		INSERT INTO public.companies (
			id, name, legal_name, country_code, timezone
		) VALUES ($1, $2, $2, 'FI', 'Europe/Helsinki')
	`, companyID, "Reservation Test "+companyID)
	if err != nil {
		t.Fatalf("insert synthetic company: %v", err)
	}
	params := ReserveStripeProvisioningIntentParams{
		ActorUserID:  actorID,
		DriverUserID: driverID,
		CompanyID:    companyID,
	}

	type outcome struct {
		intent  *StripeProvisioningIntent
		created bool
		err     error
	}

	results := make(chan outcome, 2)
	start := make(chan struct{})
	var wg sync.WaitGroup

	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			intent, created, err :=
				ReserveStripeProvisioningIntent(ctx, db, params)
			results <- outcome{intent, created, err}
		}()
	}

	close(start)
	wg.Wait()
	close(results)

	createdCount := 0
	var first *StripeProvisioningIntent

	for result := range results {
		if result.err != nil || result.intent == nil {
			t.Fatalf("reservation failed: %v", result.err)
		}

		if result.created {
			createdCount++
		}

		if first == nil {
			first = result.intent
		} else if first.ID != result.intent.ID ||
			first.IdempotencyKey != result.intent.IdempotencyKey {
			t.Fatal("concurrent reservations diverged")
		}

		if result.intent.Status != "PENDING" {
			t.Fatal("unexpected reservation status")
		}
	}

	if createdCount != 1 {
		t.Fatalf("expected one creator, got %d", createdCount)
	}

	t.Log("PASS: one creator across concurrent reservations")

	replay, created, err :=
		ReserveStripeProvisioningIntent(ctx, db, params)

	if err != nil || created || replay == nil ||
		replay.ID != first.ID ||
		replay.IdempotencyKey != first.IdempotencyKey {
		t.Fatalf("replay changed reservation: %v", err)
	}

	t.Log("PASS: replay preserves durable identity")

	conflicting := params
	conflicting.ActorUserID = driverID

	rejected, created, err :=
		ReserveStripeProvisioningIntent(ctx, db, conflicting)

	if rejected != nil || created ||
		!errors.Is(err, ErrStripeProvisioningIntentConflict) {
		t.Fatalf("actor mismatch accepted: %v", err)
	}

	t.Log("PASS: actor mismatch rejected")

	var count int
	err = db.QueryRow(ctx, `
        SELECT count(*)
        FROM stripe_connect_provisioning_intents
        WHERE driver_user_id = $1::uuid
          AND company_id = $2::uuid
    `, driverID, companyID).Scan(&count)

	if err != nil || count != 1 {
		t.Fatalf("expected one persisted intent: %d %v", count, err)
	}

	t.Log("PASS: exactly one durable intent")

	// A reservation replay is read-only even after a provider
	// request has begun. It must never reset the lifecycle.
	_, err = db.Exec(ctx, `
        UPDATE stripe_connect_provisioning_intents
        SET provisioning_status = 'REQUESTED',
            request_started_at = clock_timestamp()
        WHERE id = $1::uuid
    `, first.ID)
	if err != nil {
		t.Fatalf("mark provisioning requested: %v", err)
	}

	assertReplay := func(expectedStatus, expectedAccount string) {
		t.Helper()

		previous, isNew, replayErr :=
			ReserveStripeProvisioningIntent(ctx, db, params)

		if replayErr != nil || isNew || previous == nil {
			t.Fatalf(
				"unsafe lifecycle replay %s: new=%v err=%v",
				expectedStatus, isNew, replayErr,
			)
		}

		if previous.ID != first.ID ||
			previous.IdempotencyKey != first.IdempotencyKey ||
			previous.ActorUserID != actorID ||
			previous.DriverUserID != driverID ||
			previous.CompanyID != companyID ||
			previous.Status != expectedStatus ||
			previous.StripeAccountID != expectedAccount {
			t.Fatalf(
				"lifecycle replay changed identity or state: %+v",
				previous,
			)
		}
	}

	assertReplay("REQUESTED", "")
	t.Log("PASS: REQUESTED replay does not restart provisioning")

	_, err = db.Exec(ctx, `
        UPDATE stripe_connect_provisioning_intents
        SET provisioning_status = 'REQUIRES_RECONCILIATION'
        WHERE id = $1::uuid
    `, first.ID)
	if err != nil {
		t.Fatalf("mark provisioning reconciliation: %v", err)
	}

	assertReplay("REQUIRES_RECONCILIATION", "")
	t.Log("PASS: reconciliation replay remains fenced")

	accountID := "acct_" +
		strings.ReplaceAll(uuid.NewString(), "-", "")

	_, err = db.Exec(ctx, `
        UPDATE stripe_connect_provisioning_intents
        SET provisioning_status = 'ACCOUNT_RECORDED',
            stripe_account_id = $2,
            account_recorded_at = clock_timestamp()
        WHERE id = $1::uuid
    `, first.ID, accountID)
	if err != nil {
		t.Fatalf("record provisioned account: %v", err)
	}

	assertReplay("ACCOUNT_RECORDED", accountID)

	var persistedCount int
	err = db.QueryRow(ctx, `
        SELECT count(*)
        FROM stripe_connect_provisioning_intents
        WHERE driver_user_id = $1::uuid
          AND company_id = $2::uuid
          AND provisioning_status = 'ACCOUNT_RECORDED'
          AND stripe_account_id = $3
    `, driverID, companyID, accountID).Scan(&persistedCount)

	if err != nil || persistedCount != 1 {
		t.Fatalf(
			"recorded account identity not preserved: count=%d err=%v",
			persistedCount, err,
		)
	}

	t.Log("PASS: lifecycle replay preserves terminal account")
}
