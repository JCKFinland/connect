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

func TestStripeProvisioningIntentSingleWinnerClaim(t *testing.T) {
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

	// Invalid requests and canceled contexts must not claim
	// the durable intent or authorize provider execution.
	invalidClaim := claimParams
	invalidClaim.ActorUserID = "invalid-uuid"

	failureRejected, failureWon, failureErr :=
		ClaimStripeProvisioningIntent(ctx, db, invalidClaim)
	if failureRejected != nil || failureWon ||
		!errors.Is(failureErr, ErrStripeProvisioningIntentInvalid) {
		t.Fatalf("invalid UUID claim accepted: %v", failureErr)
	}

	invalidClaim = claimParams
	invalidClaim.IdempotencyKey = ""

	failureRejected, failureWon, failureErr =
		ClaimStripeProvisioningIntent(ctx, db, invalidClaim)
	if failureRejected != nil || failureWon ||
		!errors.Is(failureErr, ErrStripeProvisioningIntentInvalid) {
		t.Fatalf("empty key claim accepted: %v", failureErr)
	}

	failureRejected, failureWon, failureErr =
		ClaimStripeProvisioningIntent(nil, db, claimParams)
	if failureRejected != nil || failureWon ||
		!errors.Is(failureErr, ErrStripeProvisioningIntentInvalid) {
		t.Fatalf("nil context claim accepted: %v", failureErr)
	}

	failureRejected, failureWon, failureErr =
		ClaimStripeProvisioningIntent(ctx, nil, claimParams)
	if failureRejected != nil || failureWon ||
		!errors.Is(failureErr, ErrStripeProvisioningIntentInvalid) {
		t.Fatalf("nil pool claim accepted: %v", failureErr)
	}

	canceledCtx, cancelClaim := context.WithCancel(ctx)
	cancelClaim()

	failureRejected, failureWon, failureErr =
		ClaimStripeProvisioningIntent(
			canceledCtx, db, claimParams,
		)
	if failureRejected != nil || failureWon ||
		!errors.Is(failureErr, ErrStripeProvisioningIntentInvalid) {
		t.Fatalf("canceled context claim accepted: %v", failureErr)
	}

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
			"invalid request changed durable state: status=%s started=%v err=%v",
			beforeStatus, beforeStarted, err,
		)
	}

	t.Log("PASS: invalid and canceled claims preserve PENDING")

	type outcome struct {
		intent  *StripeProvisioningIntent
		claimed bool
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
			intent, claimed, claimErr :=
				ClaimStripeProvisioningIntent(
					ctx, db, claimParams,
				)
			results <- outcome{intent, claimed, claimErr}
		}()
	}

	close(start)
	wg.Wait()
	close(results)

	winners := 0
	for result := range results {
		if result.err != nil || result.intent == nil {
			t.Fatalf("claim failed: %v", result.err)
		}

		if result.claimed {
			winners++
		}

		if result.intent.ID != reserved.ID ||
			result.intent.IdempotencyKey != reserved.IdempotencyKey ||
			result.intent.Status != "REQUESTED" {
			t.Fatalf("claim identity or status changed: %+v",
				result.intent)
		}
	}

	if winners != 1 {
		t.Fatalf("expected exactly one claim winner, got %d",
			winners)
	}
	t.Log("PASS: exactly one concurrent claim winner")

	replay, claimed, err :=
		ClaimStripeProvisioningIntent(ctx, db, claimParams)
	if err != nil || claimed || replay == nil ||
		replay.ID != reserved.ID ||
		replay.IdempotencyKey != reserved.IdempotencyKey ||
		replay.Status != "REQUESTED" {
		t.Fatalf("REQUESTED replay unsafe: claimed=%v err=%v",
			claimed, err)
	}
	t.Log("PASS: REQUESTED replay cannot claim again")

	invalid := claimParams
	invalid.IdempotencyKey = "incorrect-idempotency-key"
	rejected, claimed, err :=
		ClaimStripeProvisioningIntent(ctx, db, invalid)
	if rejected != nil || claimed ||
		!errors.Is(err, ErrStripeProvisioningIntentConflict) {
		t.Fatalf("wrong idempotency key accepted: %v", err)
	}

	invalid = claimParams
	invalid.IntentID = uuid.NewString()
	rejected, claimed, err =
		ClaimStripeProvisioningIntent(ctx, db, invalid)
	if rejected != nil || claimed ||
		!errors.Is(err, ErrStripeProvisioningIntentConflict) {
		t.Fatalf("wrong intent ID accepted: %v", err)
	}

	invalid = claimParams
	invalid.ActorUserID = driverID
	rejected, claimed, err =
		ClaimStripeProvisioningIntent(ctx, db, invalid)
	if rejected != nil || claimed ||
		!errors.Is(err, ErrStripeProvisioningIntentConflict) {
		t.Fatalf("wrong actor accepted: %v", err)
	}
	t.Log("PASS: mismatched claim identities rejected")

	var count int
	var status string
	var requestStarted bool
	err = db.QueryRow(ctx, `
        SELECT count(*),
               max(provisioning_status),
               bool_and(request_started_at IS NOT NULL)
        FROM stripe_connect_provisioning_intents
        WHERE driver_user_id = $1::uuid
          AND company_id = $2::uuid
    `, driverID, companyID).Scan(
		&count, &status, &requestStarted,
	)
	if err != nil || count != 1 ||
		status != "REQUESTED" || !requestStarted {
		t.Fatalf(
			"durable claim not recorded: count=%d status=%s err=%v",
			count, status, err,
		)
	}
	t.Log("PASS: single durable REQUESTED intent")

	_, err = db.Exec(ctx, `
        UPDATE stripe_connect_provisioning_intents
        SET provisioning_status = 'REQUIRES_RECONCILIATION'
        WHERE id = $1::uuid
    `, reserved.ID)
	if err != nil {
		t.Fatalf("set reconciliation status: %v", err)
	}

	replay, claimed, err =
		ClaimStripeProvisioningIntent(ctx, db, claimParams)
	if err != nil || claimed || replay == nil ||
		replay.Status != "REQUIRES_RECONCILIATION" {
		t.Fatalf("reconciliation was reclaimed: %v", err)
	}
	t.Log("PASS: reconciliation cannot be reclaimed")

	accountID := "acct_" +
		strings.ReplaceAll(uuid.NewString(), "-", "")

	_, err = db.Exec(ctx, `
        UPDATE stripe_connect_provisioning_intents
        SET provisioning_status = 'ACCOUNT_RECORDED',
            stripe_account_id = $2,
            account_recorded_at = clock_timestamp()
        WHERE id = $1::uuid
    `, reserved.ID, accountID)
	if err != nil {
		t.Fatalf("record account: %v", err)
	}

	replay, claimed, err =
		ClaimStripeProvisioningIntent(ctx, db, claimParams)
	if err != nil || claimed || replay == nil ||
		replay.Status != "ACCOUNT_RECORDED" ||
		replay.StripeAccountID != accountID {
		t.Fatalf("recorded account was reclaimed: %v", err)
	}
	t.Log("PASS: recorded account remains terminal")
}
