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

func TestStripeProvisioningReconciliationSafety(t *testing.T) {
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

	// Require credentials for the isolated test database.
	password := os.Getenv("DB_PASSWORD")
	if password == "" {
		t.Fatal("isolated database password unavailable")
	}
	cfg.ConnConfig.Password = password

	cfg.MaxConns = 5

	schema := "stripe_provision_test_" +
		strings.ReplaceAll(uuid.NewString(), "-", "")

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
		db.Close()
		t.Fatalf("unsafe database baseline: %v %v", safe, err)
	}

	if _, err := db.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		db.Close()
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

		// Drop schema references before synthetic identities.
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

	// Synthetic identities isolate this test from public fixtures.
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
		actorID, "reconcile-"+actorID+"@example.invalid",
		driverID, "reconcile-"+driverID+"@example.invalid",
	)
	if err != nil {
		t.Fatalf("insert synthetic users: %v", err)
	}

	_, err = db.Exec(ctx, `
		INSERT INTO public.companies (
			id, name, legal_name, country_code, timezone
		) VALUES ($1, $2, $2, 'FI', 'Europe/Helsinki')
	`, companyID, "Reconciliation Test "+companyID)
	if err != nil {
		t.Fatalf("insert synthetic company: %v", err)
	}
	reserved, created, err := ReserveStripeProvisioningIntent(
		ctx, db, ReserveStripeProvisioningIntentParams{
			ActorUserID:  actorID,
			DriverUserID: driverID,
			CompanyID:    companyID,
		},
	)
	if err != nil || !created || reserved == nil {
		t.Fatalf("reservation failed: %v", err)
	}

	claimParams := ClaimStripeProvisioningIntentParams{
		IntentID:       reserved.ID,
		ActorUserID:    actorID,
		DriverUserID:   driverID,
		CompanyID:      companyID,
		IdempotencyKey: reserved.IdempotencyKey,
	}

	params := MarkStripeProvisioningReconciliationParams{
		IntentID:       reserved.ID,
		ActorUserID:    actorID,
		DriverUserID:   driverID,
		CompanyID:      companyID,
		IdempotencyKey: reserved.IdempotencyKey,
	}

	expectInvalid := func(
		label string,
		callCtx context.Context,
		pool *pgxpool.Pool,
		candidate MarkStripeProvisioningReconciliationParams,
	) {
		t.Helper()

		result, changed, callErr :=
			MarkStripeProvisioningReconciliation(
				callCtx, pool, candidate,
			)

		if result != nil || changed ||
			!errors.Is(
				callErr, ErrStripeProvisioningIntentInvalid,
			) {
			t.Fatalf("%s accepted: %v", label, callErr)
		}
	}

	expectConflict := func(
		label string,
		candidate MarkStripeProvisioningReconciliationParams,
	) {
		t.Helper()

		result, changed, callErr :=
			MarkStripeProvisioningReconciliation(
				ctx, db, candidate,
			)

		if result != nil || changed ||
			!errors.Is(
				callErr, ErrStripeProvisioningIntentConflict,
			) {
			t.Fatalf("%s accepted: %v", label, callErr)
		}
	}

	// PENDING must never be marked uncertain.
	expectConflict("PENDING reconciliation", params)

	invalid := params
	invalid.IntentID = "invalid-uuid"
	expectInvalid("invalid intent UUID", ctx, db, invalid)

	invalid = params
	invalid.IdempotencyKey = ""
	expectInvalid("empty idempotency key", ctx, db, invalid)

	invalid = params
	invalid.IdempotencyKey = " " + params.IdempotencyKey
	expectInvalid("whitespace idempotency key", ctx, db, invalid)

	expectInvalid("nil context", nil, db, params)
	expectInvalid("nil pool", ctx, nil, params)

	canceledCtx, cancelCall := context.WithCancel(ctx)
	cancelCall()
	expectInvalid("canceled context", canceledCtx, db, params)

	var beforeStatus string
	var beforeStarted bool

	err = db.QueryRow(ctx, `
        SELECT provisioning_status,
               request_started_at IS NOT NULL
        FROM stripe_connect_provisioning_intents
        WHERE id = $1::uuid
    `, reserved.ID).Scan(&beforeStatus, &beforeStarted)

	if err != nil || beforeStatus != "PENDING" || beforeStarted {
		t.Fatalf(
			"invalid calls changed PENDING state: %s %v %v",
			beforeStatus, beforeStarted, err,
		)
	}

	t.Log("PASS: invalid and premature transitions rejected")

	claimedIntent, claimed, err :=
		ClaimStripeProvisioningIntent(ctx, db, claimParams)

	if err != nil || !claimed || claimedIntent == nil ||
		claimedIntent.Status != "REQUESTED" {
		t.Fatalf("claim fixture failed: %v", err)
	}

	invalid = params
	invalid.IdempotencyKey = "incorrect-key"
	expectConflict("wrong idempotency key", invalid)

	invalid = params
	invalid.IntentID = uuid.NewString()
	expectConflict("wrong intent ID", invalid)

	invalid = params
	invalid.ActorUserID = driverID
	expectConflict("wrong actor", invalid)

	invalid = params
	invalid.DriverUserID = actorID
	expectConflict("wrong driver", invalid)

	invalid = params
	invalid.CompanyID = uuid.NewString()
	expectConflict("wrong company", invalid)

	t.Log("PASS: immutable identity conflicts rejected")

	type outcome struct {
		intent  *StripeProvisioningIntent
		changed bool
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

			intent, changed, callErr :=
				MarkStripeProvisioningReconciliation(
					ctx, db, params,
				)

			results <- outcome{intent, changed, callErr}
		}()
	}

	close(start)
	wg.Wait()
	close(results)

	winners := 0

	for result := range results {
		if result.err != nil || result.intent == nil {
			t.Fatalf("concurrent reconciliation failed: %v",
				result.err)
		}

		if result.changed {
			winners++
		}

		if result.intent.ID != reserved.ID ||
			result.intent.ActorUserID != actorID ||
			result.intent.DriverUserID != driverID ||
			result.intent.CompanyID != companyID ||
			result.intent.IdempotencyKey != reserved.IdempotencyKey ||
			result.intent.Status != "REQUIRES_RECONCILIATION" ||
			result.intent.StripeAccountID != "" {
			t.Fatalf("concurrent result mismatch: %+v",
				result.intent)
		}
	}

	if winners != 1 {
		t.Fatalf(
			"expected exactly one reconciliation winner, got %d",
			winners,
		)
	}

	t.Log("PASS: exactly one concurrent reconciliation winner")

	var started, recorded bool
	var durableStatus, durableKey string
	var durableCount int

	err = db.QueryRow(ctx, `
        SELECT count(*),
               max(provisioning_status),
               max(idempotency_key),
               bool_and(request_started_at IS NOT NULL),
               bool_and(account_recorded_at IS NOT NULL)
        FROM stripe_connect_provisioning_intents
        WHERE driver_user_id = $1::uuid
          AND company_id = $2::uuid
    `, driverID, companyID).Scan(
		&durableCount, &durableStatus, &durableKey,
		&started, &recorded,
	)

	if err != nil || durableCount != 1 ||
		durableStatus != "REQUIRES_RECONCILIATION" ||
		durableKey != reserved.IdempotencyKey ||
		!started || recorded {
		t.Fatalf(
			"durable reconciliation invalid: count=%d status=%s err=%v",
			durableCount, durableStatus, err,
		)
	}

	t.Log("PASS: one durable unresolved intent")

	replay, changed, err :=
		MarkStripeProvisioningReconciliation(ctx, db, params)

	if err != nil || changed || replay == nil ||
		replay.Status != "REQUIRES_RECONCILIATION" ||
		replay.IdempotencyKey != reserved.IdempotencyKey {
		t.Fatalf("reconciliation replay unsafe: %v", err)
	}

	replayClaim, claimedAgain, err :=
		ClaimStripeProvisioningIntent(ctx, db, claimParams)

	if err != nil || claimedAgain || replayClaim == nil ||
		replayClaim.Status != "REQUIRES_RECONCILIATION" {
		t.Fatalf("reconciliation reclaimed: %v", err)
	}

	t.Log("PASS: reconciliation cannot restart provider request")

	accountID := "acct_" +
		strings.ReplaceAll(uuid.NewString(), "-", "")

	recordParams := RecordStripeProvisioningAccountParams{
		IntentID:        reserved.ID,
		ActorUserID:     actorID,
		DriverUserID:    driverID,
		CompanyID:       companyID,
		IdempotencyKey:  reserved.IdempotencyKey,
		StripeAccountID: accountID,
	}

	terminal, newlyRecorded, err :=
		RecordStripeProvisioningAccount(
			ctx, db, recordParams,
		)

	if err != nil || !newlyRecorded || terminal == nil ||
		terminal.Status != "ACCOUNT_RECORDED" ||
		terminal.StripeAccountID != accountID {
		t.Fatalf("terminal account recording failed: %v", err)
	}

	expectConflict("terminal reconciliation", params)

	terminalReplay, claimedAgain, err :=
		ClaimStripeProvisioningIntent(ctx, db, claimParams)

	if err != nil || claimedAgain || terminalReplay == nil ||
		terminalReplay.Status != "ACCOUNT_RECORDED" ||
		terminalReplay.StripeAccountID != accountID {
		t.Fatalf("terminal account reclaimed: %v", err)
	}

	var finalCount int
	var finalStatus, finalAccount, finalKey string
	var finalStarted, finalRecorded bool

	err = db.QueryRow(ctx, `
        SELECT count(*),
               max(provisioning_status),
               max(stripe_account_id),
               max(idempotency_key),
               bool_and(request_started_at IS NOT NULL),
               bool_and(account_recorded_at IS NOT NULL)
        FROM stripe_connect_provisioning_intents
        WHERE driver_user_id = $1::uuid
          AND company_id = $2::uuid
    `, driverID, companyID).Scan(
		&finalCount, &finalStatus, &finalAccount, &finalKey,
		&finalStarted, &finalRecorded,
	)

	if err != nil || finalCount != 1 ||
		finalStatus != "ACCOUNT_RECORDED" ||
		finalAccount != accountID ||
		finalKey != reserved.IdempotencyKey ||
		!finalStarted || !finalRecorded {
		t.Fatalf("terminal state changed: %v", err)
	}

	t.Log("PASS: terminal account cannot reopen or be reclaimed")
}
