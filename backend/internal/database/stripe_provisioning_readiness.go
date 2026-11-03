package database

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrStripeProvisioningSchemaNotReady = errors.New(
	"stripe provisioning schema not ready",
)

// VerifyStripeProvisioningSchema checks durable provisioning storage.
//
// It does not apply migrations, create tables, contact Stripe,
// enable payouts, or register HTTP routes.
func VerifyStripeProvisioningSchema(
	ctx context.Context,
	db *pgxpool.Pool,
) error {
	if db == nil {
		return fmt.Errorf(
			"%w: nil database pool",
			ErrStripeProvisioningSchemaNotReady,
		)
	}

	var migrationApplied bool
	var ledgerPresent bool

	err := db.QueryRow(ctx, `
        SELECT
            EXISTS (
                SELECT 1
                FROM public.goose_db_version
                WHERE version_id = 79
                  AND is_applied = TRUE
            ),
            to_regclass(
                'public.stripe_connect_provisioning_intents'
            ) IS NOT NULL
    `).Scan(&migrationApplied, &ledgerPresent)

	if err != nil {
		return fmt.Errorf(
			"%w: database verification failed: %v",
			ErrStripeProvisioningSchemaNotReady,
			err,
		)
	}

	if !migrationApplied || !ledgerPresent {
		return fmt.Errorf(
			"%w: migration79=%t ledger=%t",
			ErrStripeProvisioningSchemaNotReady,
			migrationApplied,
			ledgerPresent,
		)
	}

	// The migration record and table alone do not establish
	// the ledger's required database-level safety protections.
	var constraintsPresent bool
	var triggerPresent bool

	err = db.QueryRow(ctx, `
        WITH required_constraints(name, kind) AS (
            VALUES
                ('uq_stripe_provisioning_driver_company', 'u'::char),
                ('uq_stripe_provisioning_idempotency', 'u'::char),
                ('uq_stripe_provisioning_account', 'u'::char),
                ('chk_stripe_provisioning_key', 'c'::char),
                ('chk_stripe_provisioning_account', 'c'::char),
                ('chk_stripe_provisioning_status', 'c'::char),
                ('chk_stripe_provisioning_lifecycle', 'c'::char),
                ('chk_stripe_provisioning_timestamps', 'c'::char)
        )
        SELECT
            NOT EXISTS (
                SELECT 1
                FROM required_constraints r
                WHERE NOT EXISTS (
                    SELECT 1
                    FROM pg_catalog.pg_constraint c
                    WHERE c.conrelid =
                        'public.stripe_connect_provisioning_intents'::regclass
                      AND c.conname = r.name
                      AND c.contype = r.kind
                      AND c.convalidated
                )
            ),
            EXISTS (
                SELECT 1
                FROM pg_catalog.pg_trigger t
                JOIN pg_catalog.pg_proc p
                  ON p.oid = t.tgfoid
                JOIN pg_catalog.pg_namespace n
                  ON n.oid = p.pronamespace
                WHERE t.tgrelid =
                    'public.stripe_connect_provisioning_intents'::regclass
                  AND t.tgname =
                    'trg_protect_stripe_connect_provisioning_intent'
                  AND NOT t.tgisinternal
                  AND t.tgenabled IN ('O', 'A')
                  AND t.tgtype = 31
                  AND p.proname =
                    'protect_stripe_connect_provisioning_intent'
                  AND n.nspname = 'public'
            )
    `).Scan(&constraintsPresent, &triggerPresent)

	if err != nil {
		return fmt.Errorf(
			"%w: integrity verification failed: %v",
			ErrStripeProvisioningSchemaNotReady,
			err,
		)
	}

	if !constraintsPresent || !triggerPresent {
		return fmt.Errorf(
			"%w: constraints=%t protective_trigger=%t",
			ErrStripeProvisioningSchemaNotReady,
			constraintsPresent,
			triggerPresent,
		)
	}

	return nil
}
