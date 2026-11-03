package database

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestStripeProvisioningSchemaDisposableDatabase(t *testing.T) {
	if os.Getenv("DB_NAME") != "connect_payout_destination_test" ||
		os.Getenv("CONNECT_REQUIRE_SETTLEMENT_TEST_DB") != "0" ||
		os.Getenv("FLEX_SETTLEMENT_WORKER_ENABLED") != "false" ||
		os.Getenv("DB_HOST") != "localhost" ||
		os.Getenv("DB_PORT") != "5432" ||
		os.Getenv("DB_USER") != "postgres" ||
		os.Getenv("DB_SSLMODE") != "disable" {
		t.Skip("requires explicitly guarded local PostgreSQL test environment")
	}

	ctx, cancel := context.WithTimeout(
		context.Background(), 120*time.Second,
	)
	defer cancel()

	sourceDB := "connect_payout_destination_test"
	name := "connect_stripe_readiness_" +
		strings.ReplaceAll(uuid.NewString(), "-", "")

	run := func(input []byte, command string, args ...string) []byte {
		t.Helper()

		cmd := exec.CommandContext(ctx, command, args...)
		if input != nil {
			cmd.Stdin = bytes.NewReader(input)
		}

		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("%s failed: %v: %s", command, err, output)
		}
		return output
	}

	baseArgs := []string{
		"-h", "localhost", "-p", "5432", "-U", "postgres",
	}

	source, err := pgxpool.New(
		ctx,
		"host=localhost port=5432 user=postgres "+
			"dbname="+sourceDB+" sslmode=disable",
	)
	if err != nil {
		t.Fatal(err)
	}
	defer source.Close()

	var safe bool
	err = source.QueryRow(ctx, `
        SELECT current_database() = 'connect_payout_destination_test'
          AND (SELECT MAX(version_id)
               FROM public.goose_db_version
               WHERE is_applied) = 75
          AND to_regclass(
               'public.stripe_connect_provisioning_intents'
          ) IS NULL
    `).Scan(&safe)
	if err != nil || !safe {
		t.Fatalf("unsafe source database: %v %v", safe, err)
	}

	dumpArgs := append(
		append([]string{}, baseArgs...),
		"--schema-only", "--no-owner",
		"--no-privileges", "--no-comments",
		"-d", sourceDB,
	)
	dump := run(nil, "pg_dump", dumpArgs...)

	for _, marker := range []string{
		"CREATE TABLE public.users",
		"CREATE TABLE public.companies",
		"CREATE TABLE public.goose_db_version",
	} {
		if !bytes.Contains(dump, []byte(marker)) {
			t.Fatalf("schema dump missing %s", marker)
		}
	}
	if bytes.Contains(dump, []byte("COPY public.")) {
		t.Fatal("schema dump unexpectedly contains data")
	}

	// UUID-based name is generated exclusively by this test.
	// Refuse any collision; never reuse an existing database.
	var exists bool
	err = source.QueryRow(ctx,
		"SELECT EXISTS (SELECT 1 FROM pg_database WHERE datname=$1)",
		name,
	).Scan(&exists)
	if err != nil || exists {
		t.Fatalf("disposable database collision: %v %v", exists, err)
	}

	createArgs := append(
		append([]string{}, baseArgs...),
		"-T", "template0", name,
	)
	run(nil, "createdb", createArgs...)

	// Cleanup is registered immediately after successful creation.
	// Only this invocation's randomly generated database is removed.
	t.Cleanup(func() {
		cleanupCtx, stop := context.WithTimeout(
			context.Background(), 30*time.Second,
		)
		defer stop()

		cmd := exec.CommandContext(
			cleanupCtx, "dropdb",
			"-h", "localhost", "-p", "5432",
			"-U", "postgres", "--if-exists", name,
		)
		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Errorf("disposable database cleanup failed: %v: %s",
				err, output)
		}
	})

	restoreArgs := append(
		append([]string{}, baseArgs...),
		"-X", "-w", "-v", "ON_ERROR_STOP=1", "-d", name,
	)
	run(dump, "psql", restoreArgs...)

	pool, err := pgxpool.New(
		ctx,
		"host=localhost port=5432 user=postgres "+
			"dbname="+name+" sslmode=disable",
	)
	if err != nil {
		t.Fatal(err)
	}
	// Close connections before the database cleanup callback.
	t.Cleanup(pool.Close)

	var count int
	err = pool.QueryRow(ctx, `
        SELECT
          (SELECT count(*) FROM public.users) +
          (SELECT count(*) FROM public.companies) +
          (SELECT count(*) FROM public.goose_db_version)
    `).Scan(&count)
	if err != nil || count != 0 {
		t.Fatalf("schema-only clone contains data: %d %v", count, err)
	}

	migration, err := os.ReadFile(
		"../../migrations/00079_create_stripe_connect_provisioning_intents.sql",
	)
	if err != nil {
		t.Fatal(err)
	}

	text := string(migration)
	if strings.Count(text, "-- +goose Up") != 1 ||
		strings.Count(text, "-- +goose Down") != 1 {
		t.Fatal("migration 79 sections invalid")
	}

	up := strings.SplitN(
		strings.SplitN(text, "-- +goose Up", 2)[1],
		"-- +goose Down", 2,
	)[0]
	up = strings.ReplaceAll(up, "-- +goose StatementBegin", "")
	up = strings.ReplaceAll(up, "-- +goose StatementEnd", "")

	if _, err := pool.Exec(
		ctx, up, pgx.QueryExecModeSimpleProtocol,
	); err != nil {
		t.Fatalf("apply migration 79 to disposable database: %v", err)
	}

	// A schema-only restore has no Goose history rows.
	// Record only migration 79 in this disposable database.
	_, err = pool.Exec(ctx, `
        INSERT INTO public.goose_db_version
            (id, version_id, is_applied, tstamp)
        VALUES (1, 79, TRUE, NOW())
    `)
	if err != nil {
		t.Fatalf("record disposable migration 79: %v", err)
	}

	assertReady := func() {
		t.Helper()
		if err := VerifyStripeProvisioningSchema(ctx, pool); err != nil {
			t.Fatalf("intact ledger rejected: %v", err)
		}
	}

	assertRejected := func(reason string) {
		t.Helper()
		err := VerifyStripeProvisioningSchema(ctx, pool)
		if !errors.Is(err, ErrStripeProvisioningSchemaNotReady) {
			t.Fatalf("%s: expected fail-closed rejection, got %v",
				reason, err)
		}
	}

	t.Run("intact_ledger_accepted", func(t *testing.T) {
		assertReady()
	})

	constraint := "uq_stripe_provisioning_driver_company"

	t.Run("missing_unique_constraint_rejected", func(t *testing.T) {
		_, err := pool.Exec(ctx, fmt.Sprintf(
			`ALTER TABLE public.stripe_connect_provisioning_intents
             RENAME CONSTRAINT %s TO readiness_temporarily_renamed`,
			constraint,
		))
		if err != nil {
			t.Fatal(err)
		}

		// Restore before proceeding to the next test case.
		defer func() {
			restoreCtx, stop := context.WithTimeout(
				context.Background(), 10*time.Second,
			)
			defer stop()
			_, restoreErr := pool.Exec(restoreCtx, fmt.Sprintf(
				`ALTER TABLE public.stripe_connect_provisioning_intents
                 RENAME CONSTRAINT readiness_temporarily_renamed TO %s`,
				constraint,
			))
			if restoreErr != nil {
				t.Errorf("restore constraint: %v", restoreErr)
			}
		}()

		assertRejected("missing unique constraint")
	})

	assertReady()

	trigger := "trg_protect_stripe_connect_provisioning_intent"

	t.Run("disabled_protective_trigger_rejected", func(t *testing.T) {
		_, err := pool.Exec(ctx, fmt.Sprintf(
			`ALTER TABLE public.stripe_connect_provisioning_intents
             DISABLE TRIGGER %s`,
			trigger,
		))
		if err != nil {
			t.Fatal(err)
		}

		defer func() {
			restoreCtx, stop := context.WithTimeout(
				context.Background(), 10*time.Second,
			)
			defer stop()
			_, restoreErr := pool.Exec(restoreCtx, fmt.Sprintf(
				`ALTER TABLE public.stripe_connect_provisioning_intents
                 ENABLE TRIGGER %s`,
				trigger,
			))
			if restoreErr != nil {
				t.Errorf("restore trigger: %v", restoreErr)
			}
		}()

		assertRejected("disabled protective trigger")
	})

	assertReady()

	t.Log("disposable migration-79 readiness checks passed")
}
