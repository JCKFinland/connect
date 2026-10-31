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

func runStripeProvisioningRecordingCase(t *testing.T, reconcile bool) {
	if os.Getenv("DB_NAME") != "connect_payout_destination_test" ||
		os.Getenv("CONNECT_REQUIRE_SETTLEMENT_TEST_DB") != "0" ||
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
		"connect_payout_destination_test",
	))
	if err != nil {
		t.Fatal(err)
	}

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
        SELECT current_database() = 'connect_payout_destination_test'
          AND (SELECT MAX(version_id)
               FROM public.goose_db_version
               WHERE is_applied) = 75
          AND to_regclass(
               'public.stripe_connect_provisioning_intents'
          ) IS NULL
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

	t.Cleanup(func() {
		defer db.Close()

		cleanupCtx, stop := context.WithTimeout(
			context.Background(), 20*time.Second,
		)
		defer stop()

		if _, err := db.Exec(
			cleanupCtx, "DROP SCHEMA "+schema+" CASCADE",
		); err != nil {
			t.Errorf("isolated schema cleanup failed: %v", err)
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

	// Existing test identities serve only as FK references.
	// No production identity or authorization is inferred.
	var actorID, driverID, companyID string
	err = db.QueryRow(ctx, `
        SELECT
          (SELECT id::text FROM public.users
           ORDER BY id LIMIT 1),
          (SELECT id::text FROM public.users
           ORDER BY id OFFSET 1 LIMIT 1),
          (SELECT id::text FROM public.companies
           ORDER BY id LIMIT 1)
    `).Scan(&actorID, &driverID, &companyID)
	if err != nil {
		t.Fatalf("test identities unavailable: %v", err)
	}

	params := ReserveStripeProvisioningIntentParams{
		ActorUserID:  actorID,
		DriverUserID: driverID,
		CompanyID:    companyID,
	}

	reserved, created, err :=
		ReserveStripeProvisioningIntent(ctx, db, params)
	if err != nil || !created || reserved == nil {
		t.Fatalf("reserve claim fixture: created=%v err=%v",
			created, err)
	}

	claimParams := ClaimStripeProvisioningIntentParams{
		IntentID:       reserved.ID,
		ActorUserID:    actorID,
		DriverUserID:   driverID,
		CompanyID:      companyID,
		IdempotencyKey: reserved.IdempotencyKey,
	}

	accountID := "acct_" +
		strings.ReplaceAll(uuid.NewString(), "-", "")
	otherAccountID := "acct_" +
		strings.ReplaceAll(uuid.NewString(), "-", "")

	recordParams := RecordStripeProvisioningAccountParams{
		IntentID:        reserved.ID,
		ActorUserID:     actorID,
		DriverUserID:    driverID,
		CompanyID:       companyID,
		IdempotencyKey:  reserved.IdempotencyKey,
		StripeAccountID: accountID,
	}

	// A PENDING reservation cannot be recorded before claiming.
	pending, pendingRecorded, pendingErr :=
		RecordStripeProvisioningAccount(ctx, db, recordParams)
	if pending != nil || pendingRecorded ||
		!errors.Is(pendingErr, ErrStripeProvisioningIntentConflict) {
		t.Fatalf("PENDING recording accepted: %v", pendingErr)
	}

	// Invalid input must not mutate the ledger.
	invalid := recordParams
	invalid.StripeAccountID = "invalid-account"
	rejected, recorded, recordErr :=
		RecordStripeProvisioningAccount(ctx, db, invalid)
	if rejected != nil || recorded ||
		!errors.Is(recordErr, ErrStripeProvisioningIntentInvalid) {
		t.Fatalf("invalid account accepted: %v", recordErr)
	}

	invalid = recordParams
	invalid.IdempotencyKey = ""
	rejected, recorded, recordErr =
		RecordStripeProvisioningAccount(ctx, db, invalid)
	if rejected != nil || recorded ||
		!errors.Is(recordErr, ErrStripeProvisioningIntentInvalid) {
		t.Fatalf("empty idempotency key accepted: %v", recordErr)
	}

	claimedIntent, claimed, err :=
		ClaimStripeProvisioningIntent(ctx, db, claimParams)
	if err != nil || !claimed || claimedIntent == nil ||
		claimedIntent.Status != "REQUESTED" {
		t.Fatalf("claim recording fixture: %v", err)
	}

	if reconcile {
		_, err = db.Exec(ctx, `
            UPDATE stripe_connect_provisioning_intents
            SET provisioning_status = 'REQUIRES_RECONCILIATION'
            WHERE id = $1::uuid
        `, reserved.ID)
		if err != nil {
			t.Fatalf("enter reconciliation: %v", err)
		}
	}

	type outcome struct {
		intent   *StripeProvisioningIntent
		recorded bool
		err      error
	}

	results := make(chan outcome, 2)
	start := make(chan struct{})
	var wg sync.WaitGroup

	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			intent, won, callErr :=
				RecordStripeProvisioningAccount(
					ctx, db, recordParams,
				)
			results <- outcome{intent, won, callErr}
		}()
	}

	close(start)
	wg.Wait()
	close(results)

	winners := 0
	for result := range results {
		if result.err != nil || result.intent == nil {
			t.Fatalf("concurrent recording failed: %v", result.err)
		}
		if result.recorded {
			winners++
		}
		if result.intent.ID != reserved.ID ||
			result.intent.IdempotencyKey != reserved.IdempotencyKey ||
			result.intent.Status != "ACCOUNT_RECORDED" ||
			result.intent.StripeAccountID != accountID {
			t.Fatalf("concurrent result mismatch: %+v", result.intent)
		}
	}

	if winners != 1 {
		t.Fatalf("expected one recording winner, got %d", winners)
	}
	t.Log("PASS: exactly one concurrent recording winner")

	replay, recorded, err :=
		RecordStripeProvisioningAccount(ctx, db, recordParams)
	if err != nil || recorded || replay == nil ||
		replay.Status != "ACCOUNT_RECORDED" ||
		replay.StripeAccountID != accountID {
		t.Fatalf("same-account replay unsafe: %v", err)
	}
	t.Log("PASS: identical-account replay is read-only")

	conflict := recordParams
	conflict.StripeAccountID = otherAccountID

	rejected, recorded, recordErr =
		RecordStripeProvisioningAccount(ctx, db, conflict)
	if rejected != nil || recorded ||
		!errors.Is(recordErr, ErrStripeProvisioningIntentConflict) {
		t.Fatalf("conflicting account accepted: %v", recordErr)
	}

	conflict = recordParams
	conflict.IntentID = uuid.NewString()
	rejected, recorded, recordErr =
		RecordStripeProvisioningAccount(ctx, db, conflict)
	if rejected != nil || recorded ||
		!errors.Is(recordErr, ErrStripeProvisioningIntentConflict) {
		t.Fatalf("conflicting intent accepted: %v", recordErr)
	}

	conflict = recordParams
	conflict.ActorUserID = driverID
	rejected, recorded, recordErr =
		RecordStripeProvisioningAccount(ctx, db, conflict)
	if rejected != nil || recorded ||
		!errors.Is(recordErr, ErrStripeProvisioningIntentConflict) {
		t.Fatalf("conflicting actor accepted: %v", recordErr)
	}
	t.Log("PASS: conflicting account and identities rejected")

	var count int
	var status string
	var persistedAccount string
	var started, finished bool

	err = db.QueryRow(ctx, `
        SELECT count(*),
               max(provisioning_status),
               max(stripe_account_id),
               bool_and(request_started_at IS NOT NULL),
               bool_and(account_recorded_at IS NOT NULL)
        FROM stripe_connect_provisioning_intents
        WHERE driver_user_id = $1::uuid
          AND company_id = $2::uuid
    `, driverID, companyID).Scan(
		&count, &status, &persistedAccount,
		&started, &finished,
	)

	if err != nil || count != 1 ||
		status != "ACCOUNT_RECORDED" ||
		persistedAccount != accountID ||
		!started || !finished {
		t.Fatalf(
			"durable recording invalid: count=%d status=%s account=%s err=%v",
			count, status, persistedAccount, err,
		)
	}
	t.Log("PASS: one durable terminal account recorded")

	replayClaim, claimedAgain, err :=
		ClaimStripeProvisioningIntent(ctx, db, claimParams)
	if err != nil || claimedAgain || replayClaim == nil ||
		replayClaim.Status != "ACCOUNT_RECORDED" ||
		replayClaim.StripeAccountID != accountID {
		t.Fatalf("recorded account reclaimed: %v", err)
	}
	t.Log("PASS: terminal account cannot be reclaimed")
}

func TestStripeProvisioningAccountRecordingRequested(t *testing.T) {
	runStripeProvisioningRecordingCase(t, false)
}

func TestStripeProvisioningAccountRecordingReconciliation(t *testing.T) {
	runStripeProvisioningRecordingCase(t, true)
}
