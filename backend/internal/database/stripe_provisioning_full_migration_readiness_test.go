package database

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

func TestStripeProvisioningFullMigrationReadinessCI(t *testing.T) {
	if os.Getenv("CONNECT_STRIPE_FULL_MIGRATION_CI") != "1" {
		t.Skip("isolated Stripe migration CI fixture not enabled")
	}

	const databaseName = "connect_stripe_ci_regression"

	if os.Getenv("PGHOST") != "127.0.0.1" ||
		os.Getenv("PGPORT") != "55433" ||
		os.Getenv("PGUSER") != "postgres" ||
		os.Getenv("PGDATABASE") != databaseName ||
		os.Getenv("PGSSLMODE") != "disable" ||
		os.Getenv("PGPASSWORD") == "" {
		t.Fatal("STOP: unsafe PostgreSQL CI environment")
	}

	port, err := strconv.Atoi(os.Getenv("PGPORT"))
	if err != nil || port != 55433 {
		t.Fatal("STOP: invalid isolated PostgreSQL port")
	}

	ctx, cancel := context.WithTimeout(
		context.Background(), 180*time.Second,
	)
	defer cancel()

	dsn := fmt.Sprintf(
		"host=127.0.0.1 port=%d user=postgres dbname=%s sslmode=disable connect_timeout=5",
		port, databaseName,
	)

	config, err := pgx.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}

	config.Password = os.Getenv("PGPASSWORD")

	db := stdlib.OpenDB(*config)
	defer db.Close()

	if err := db.PingContext(ctx); err != nil {
		t.Fatal(err)
	}

	var actualName, version string
	err = db.QueryRowContext(ctx, `
        SELECT current_database(),
               current_setting('server_version')
    `).Scan(&actualName, &version)

	if err != nil {
		t.Fatal(err)
	}

	if actualName != databaseName ||
		!strings.HasPrefix(version, "16.") {
		t.Fatal("STOP: unexpected database identity or version")
	}

	var existingHistory int
	err = db.QueryRowContext(ctx, `
        SELECT COUNT(*)
        FROM pg_catalog.pg_tables
        WHERE schemaname = 'public'
          AND tablename = 'goose_db_version'
    `).Scan(&existingHistory)

	if err != nil {
		t.Fatal(err)
	}

	if existingHistory != 0 {
		t.Fatal("STOP: migration history already exists")
	}

	migrationDir := filepath.Join("..", "..", "migrations")

	manifestPath := filepath.Join(
		"..", "..", "docs",
		"stripe_provisioning_migration_manifest.sha256",
	)

	manifest, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}

	verified := map[string]bool{}

	for _, line := range strings.Split(
		strings.TrimSpace(string(manifest)), "\n",
	) {
		fields := strings.Fields(line)
		if len(fields) != 2 {
			t.Fatal("STOP: invalid Stripe migration manifest")
		}

		filename := fields[1]

		if !strings.HasPrefix(filename, "migrations/0007") ||
			!strings.HasSuffix(filename, ".sql") {
			t.Fatal("STOP: unexpected Stripe manifest path")
		}

		if verified[filename] {
			t.Fatal("STOP: duplicate Stripe manifest entry")
		}

		blob, err := os.ReadFile(
			filepath.Join("..", "..", filepath.FromSlash(filename)),
		)
		if err != nil {
			t.Fatal(err)
		}

		normalized := bytes.ReplaceAll(blob, []byte("\r\n"), []byte("\n"))

		if bytes.Contains(normalized, []byte("\r")) {
			t.Fatalf("STOP: unexpected migration line ending: %s", filename)
		}

		digest := sha256.Sum256(normalized)

		if hex.EncodeToString(digest[:]) != fields[0] {
			t.Fatalf("STOP: Stripe migration hash mismatch: %s", filename)
		}

		verified[filename] = true
	}

	for _, filename := range []string{
		"migrations/00076_add_payout_destination_verification_permission.sql",
		"migrations/00077_create_stripe_connect_ownership_bindings.sql",
		"migrations/00078_create_stripe_connect_account_link_sessions.sql",
		"migrations/00079_create_stripe_connect_provisioning_intents.sql",
	} {
		if !verified[filename] {
			t.Fatalf("STOP: missing Stripe manifest entry: %s", filename)
		}
	}

	if len(verified) != 4 {
		t.Fatal("STOP: unexpected Stripe manifest entries")
	}

	if err := goose.SetDialect("postgres"); err != nil {
		t.Fatal(err)
	}

	verifyHistory := func(expected int) {
		t.Helper()

		var latest, count int
		err := db.QueryRowContext(ctx, `
            SELECT COALESCE(MAX(version_id), 0),
                   COUNT(DISTINCT version_id)
            FROM public.goose_db_version
            WHERE is_applied
              AND version_id BETWEEN 1 AND $1
        `, expected).Scan(&latest, &count)

		if err != nil {
			t.Fatal(err)
		}

		if latest != expected || count != expected {
			t.Fatalf(
				"STOP: migration history mismatch: latest=%d count=%d expected=%d",
				latest, count, expected,
			)
		}
	}

	if err := goose.UpToContext(
		ctx, db, migrationDir, 75,
	); err != nil {
		t.Fatal(err)
	}

	verifyHistory(75)

	var absent bool
	err = db.QueryRowContext(ctx, `
        SELECT
            to_regclass('public.stripe_connect_ownership_bindings') IS NULL
            AND to_regclass('public.stripe_connect_account_link_sessions') IS NULL
            AND to_regclass('public.stripe_connect_provisioning_intents') IS NULL
    `).Scan(&absent)

	if err != nil {
		t.Fatal(err)
	}

	if !absent {
		t.Fatal("STOP: Stripe schema exists at baseline 75")
	}

	t.Log("PASS: clean baseline migrations 1-75")

	if err := goose.UpToContext(
		ctx, db, migrationDir, 79,
	); err != nil {
		t.Fatal(err)
	}

	verifyHistory(79)

	poolConfig, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}

	poolConfig.ConnConfig.Password = os.Getenv("PGPASSWORD")

	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		t.Fatal(err)
	}

	if err := VerifyStripeProvisioningSchema(ctx, pool); err != nil {
		t.Fatalf("STOP: CONNECT readiness failed: %v", err)
	}

	var tables, triggers int
	err = db.QueryRowContext(ctx, `
        SELECT
            (
                SELECT COUNT(*)
                FROM pg_class
                WHERE oid IN (
                    to_regclass('public.stripe_connect_ownership_bindings'),
                    to_regclass('public.stripe_connect_account_link_sessions'),
                    to_regclass('public.stripe_connect_provisioning_intents')
                )
            ),
            (
                SELECT COUNT(*)
                FROM pg_trigger
                WHERE NOT tgisinternal
                  AND tgenabled <> 'D'
                  AND tgrelid IN (
                    to_regclass('public.stripe_connect_ownership_bindings'),
                    to_regclass('public.stripe_connect_account_link_sessions'),
                    to_regclass('public.stripe_connect_provisioning_intents')
                )
            )
    `).Scan(&tables, &triggers)

	if err != nil {
		t.Fatal(err)
	}

	if tables != 3 || triggers < 3 {
		t.Fatalf(
			"STOP: Stripe schema contract failed: tables=%d triggers=%d",
			tables, triggers,
		)
	}

	t.Log("PASS: all 79 migration versions")
	t.Log("PASS: CONNECT application readiness")
	t.Log("PASS: three Stripe ledgers and protective triggers")
}
