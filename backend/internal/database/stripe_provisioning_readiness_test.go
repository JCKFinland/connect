package database

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestStripeProvisioningSchemaRejectsNilPool(t *testing.T) {
	err := VerifyStripeProvisioningSchema(
		context.Background(),
		nil,
	)

	if !errors.Is(err, ErrStripeProvisioningSchemaNotReady) {
		t.Fatalf("expected schema readiness rejection, got %v", err)
	}
}

func TestStripeProvisioningSchemaRejectsMissingMigration79(
	t *testing.T,
) {
	if os.Getenv("DB_NAME") != "connect_payout_destination_test" ||
		os.Getenv("CONNECT_REQUIRE_SETTLEMENT_TEST_DB") != "0" ||
		os.Getenv("FLEX_SETTLEMENT_WORKER_ENABLED") != "false" {
		t.Skip("requires explicitly guarded isolated database")
	}

	ctx, cancel := context.WithTimeout(
		context.Background(),
		10*time.Second,
	)
	defer cancel()

	pool, err := pgxpool.New(
		ctx,
		"host=localhost port=5432 user=postgres "+
			"dbname=connect_payout_destination_test sslmode=disable",
	)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	var databaseName string
	var version int64
	var ledgerPresent bool

	err = pool.QueryRow(ctx, `
        SELECT
            current_database(),
            COALESCE((
                SELECT MAX(version_id)
                FROM public.goose_db_version
                WHERE is_applied = TRUE
            ), 0),
            to_regclass(
                'public.stripe_connect_provisioning_intents'
            ) IS NOT NULL
    `).Scan(&databaseName, &version, &ledgerPresent)

	if err != nil {
		t.Fatal(err)
	}

	if databaseName != "connect_payout_destination_test" ||
		version != 75 ||
		ledgerPresent {
		t.Fatalf(
			"unsafe test database baseline: db=%s version=%d ledger=%t",
			databaseName,
			version,
			ledgerPresent,
		)
	}

	err = VerifyStripeProvisioningSchema(ctx, pool)
	if !errors.Is(err, ErrStripeProvisioningSchemaNotReady) {
		t.Fatalf(
			"expected missing migration rejection, got %v",
			err,
		)
	}

	var afterVersion int64
	err = pool.QueryRow(ctx, `
        SELECT COALESCE(MAX(version_id), 0)
        FROM public.goose_db_version
        WHERE is_applied = TRUE
    `).Scan(&afterVersion)
	if err != nil {
		t.Fatal(err)
	}

	if afterVersion != version {
		t.Fatalf(
			"schema gate changed migration history: %d -> %d",
			version,
			afterVersion,
		)
	}
}

// TestStripeProvisioningSchemaIntegrityContract guards the
// catalog checks against accidentally dropping a required
// constraint or the protective lifecycle trigger.
func TestStripeProvisioningSchemaIntegrityContract(t *testing.T) {
	source, err := os.ReadFile("stripe_provisioning_readiness.go")
	if err != nil {
		t.Fatal(err)
	}

	required := []string{
		"uq_stripe_provisioning_driver_company",
		"uq_stripe_provisioning_idempotency",
		"uq_stripe_provisioning_account",
		"chk_stripe_provisioning_key",
		"chk_stripe_provisioning_account",
		"chk_stripe_provisioning_status",
		"chk_stripe_provisioning_lifecycle",
		"chk_stripe_provisioning_timestamps",
		"trg_protect_stripe_connect_provisioning_intent",
		"protect_stripe_connect_provisioning_intent",
		"c.convalidated",
		"t.tgenabled IN ('O', 'A')",
		"t.tgtype = 31",
	}

	for _, marker := range required {
		if !strings.Contains(string(source), marker) {
			t.Fatalf("missing schema integrity check: %s", marker)
		}
	}
}
