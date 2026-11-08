#!/usr/bin/env bash
set -euo pipefail

printf '\n===== CONNECT STRIPE POST-MIGRATION VERIFICATION =====\n'

: "${DB_HOST:?STOP: DB_HOST required}"
: "${DB_PORT:?STOP: DB_PORT required}"
: "${DB_USER:?STOP: DB_USER required}"
: "${DB_NAME:?STOP: DB_NAME required}"
: "${DB_SSLMODE:?STOP: DB_SSLMODE required}"
: "${APP_ENV:?STOP: APP_ENV required}"
: "${STRIPE_CONNECT_PROVISIONING_ENABLED:?STOP: provisioning flag required}"
: "${FLEX_SETTLEMENT_WORKER_ENABLED:?STOP: settlement worker flag required}"
: "${FLEX_AUTHORIZATION_RECOVERY_WORKER_ENABLED:?STOP: recovery worker flag required}"
: "${CONNECT_DEPLOYMENT_PSQL_BIN:?STOP: approved psql required}"

if [ "$APP_ENV" != production ]; then
    echo "STOP: production environment required"
    exit 1
fi

if [ "$DB_SSLMODE" != verify-full ]; then
    echo "STOP: verify-full TLS required"
    exit 1
fi

: "${PGSSLROOTCERT:?STOP: PostgreSQL CA required}"

test -f "$PGSSLROOTCERT" || {
    echo "STOP: PostgreSQL CA missing"
    exit 1
}

for variable in \
    STRIPE_CONNECT_PROVISIONING_ENABLED \
    FLEX_SETTLEMENT_WORKER_ENABLED \
    FLEX_AUTHORIZATION_RECOVERY_WORKER_ENABLED; do

    if [ "${!variable}" != false ]; then
        printf 'STOP: %s must be false\n' "$variable"
        exit 1
    fi
done

case "$DB_NAME" in
    ""|postgres|template0|template1|connect_payout_destination_test|connect_settlement_test|connect_integration_test)
        echo "STOP: unsafe database target"
        exit 1
        ;;
esac

case "$CONNECT_DEPLOYMENT_PSQL_BIN" in
    /*) ;;
    *)
        echo "STOP: approved psql must be absolute"
        exit 1
        ;;
esac

test -f "$CONNECT_DEPLOYMENT_PSQL_BIN" || {
    echo "STOP: approved psql missing"
    exit 1
}

test -x "$CONNECT_DEPLOYMENT_PSQL_BIN" || {
    echo "STOP: approved psql not executable"
    exit 1
}

PSQL_ENV=(
    "PATH=/usr/bin:/bin"
    "PGSSLMODE=$DB_SSLMODE"
    "PGSSLROOTCERT=$PGSSLROOTCERT"
)

if [ -n "${PGPASSWORD:-}" ]; then
    PSQL_ENV+=("PGPASSWORD=$PGPASSWORD")
fi

if [ -n "${PGPASSFILE:-}" ]; then
    PSQL_ENV+=("PGPASSFILE=$PGPASSFILE")
fi

if [ -n "${PGCONNECT_TIMEOUT:-}" ]; then
    PSQL_ENV+=("PGCONNECT_TIMEOUT=$PGCONNECT_TIMEOUT")
fi

printf '\n===== VERIFIED DATABASE IDENTITY =====\n'

: "${CONNECT_DEPLOYMENT_DATABASE_NAME:?STOP: approved database name required}"
: "${CONNECT_DEPLOYMENT_CLUSTER_SYSTEM_IDENTIFIER:?STOP: approved cluster identifier required}"
: "${CONNECT_DEPLOYMENT_DATABASE_OID:?STOP: approved database OID required}"

if [ "$DB_NAME" != "$CONNECT_DEPLOYMENT_DATABASE_NAME" ]; then
    echo "STOP: approved database name mismatch"
    exit 1
fi

# Reuse the existing fail-closed database identity gate.
# It verifies the database name, cluster system identifier,
# and database OID using a read-only PostgreSQL transaction.
IDENTITY_GATE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/stripe_provisioning_database_identity_gate.sh"

test -f "$IDENTITY_GATE" && test ! -L "$IDENTITY_GATE" || {
    echo "STOP: database identity gate unavailable"
    exit 1
}

bash "$IDENTITY_GATE"

printf '\n===== READ-ONLY POST-MIGRATION SCHEMA CHECK =====\n'

env -i "${PSQL_ENV[@]}" \
"$CONNECT_DEPLOYMENT_PSQL_BIN" \
    -X -v ON_ERROR_STOP=1 \
    -h "$DB_HOST" \
    -p "$DB_PORT" \
    -U "$DB_USER" \
    -d "$DB_NAME" <<'SQL'

BEGIN READ ONLY;

DO $$
DECLARE
    latest_version bigint;
    required_count integer;
    ledger_count integer;
    constraints_present boolean;
    ownership_trigger_present boolean;
    link_trigger_present boolean;
    provisioning_trigger_present boolean;
BEGIN
    SELECT MAX(version_id)
    INTO latest_version
    FROM public.goose_db_version
    WHERE is_applied;

    IF latest_version IS DISTINCT FROM 79 THEN
        RAISE EXCEPTION
            'STOP: expected migration version 79, found %',
            latest_version;
    END IF;

    SELECT COUNT(DISTINCT version_id)
    INTO required_count
    FROM public.goose_db_version
    WHERE is_applied
      AND version_id BETWEEN 76 AND 79;

    IF required_count <> 4 THEN
        RAISE EXCEPTION
            'STOP: migrations 76-79 not all applied';
    END IF;

    SELECT COUNT(*)
    INTO ledger_count
    FROM pg_catalog.pg_class
    WHERE oid IN (
        to_regclass('public.stripe_connect_ownership_bindings'),
        to_regclass('public.stripe_connect_account_link_sessions'),
        to_regclass('public.stripe_connect_provisioning_intents')
    )
      AND relkind = 'r';

    IF ledger_count <> 3 THEN
        RAISE EXCEPTION
            'STOP: expected three Stripe ledger tables';
    END IF;

    -- Match each required constraint to its authoritative
    -- table, name, type, and validated status.
    WITH required_constraints(table_oid, name, kind) AS (
        VALUES
        ('public.stripe_connect_ownership_bindings'::regclass,
         'chk_stripe_ownership_account', 'c'::char),
        ('public.stripe_connect_ownership_bindings'::regclass,
         'chk_stripe_ownership_status', 'c'::char),
        ('public.stripe_connect_ownership_bindings'::regclass,
         'chk_stripe_ownership_evidence', 'c'::char),
        ('public.stripe_connect_account_link_sessions'::regclass,
         'chk_stripe_link_account_identity', 'c'::char),
        ('public.stripe_connect_account_link_sessions'::regclass,
         'chk_stripe_link_lifetime', 'c'::char),
        ('public.stripe_connect_account_link_sessions'::regclass,
         'chk_stripe_link_consumption', 'c'::char),
        ('public.stripe_connect_provisioning_intents'::regclass,
         'uq_stripe_provisioning_driver_company', 'u'::char),
        ('public.stripe_connect_provisioning_intents'::regclass,
         'uq_stripe_provisioning_idempotency', 'u'::char),
        ('public.stripe_connect_provisioning_intents'::regclass,
         'uq_stripe_provisioning_account', 'u'::char),
        ('public.stripe_connect_provisioning_intents'::regclass,
         'chk_stripe_provisioning_key', 'c'::char),
        ('public.stripe_connect_provisioning_intents'::regclass,
         'chk_stripe_provisioning_account', 'c'::char),
        ('public.stripe_connect_provisioning_intents'::regclass,
         'chk_stripe_provisioning_status', 'c'::char),
        ('public.stripe_connect_provisioning_intents'::regclass,
         'chk_stripe_provisioning_lifecycle', 'c'::char),
        ('public.stripe_connect_provisioning_intents'::regclass,
         'chk_stripe_provisioning_timestamps', 'c'::char)
    )
    SELECT NOT EXISTS (
        SELECT 1
        FROM required_constraints r
        WHERE NOT EXISTS (
            SELECT 1
            FROM pg_catalog.pg_constraint c
            WHERE c.conrelid = r.table_oid
              AND c.conname = r.name
              AND c.contype = r.kind
              AND c.convalidated
        )
    )
    INTO constraints_present;

    IF NOT constraints_present THEN
        RAISE EXCEPTION
            'STOP: required Stripe constraint missing or invalid';
    END IF;

    -- BEFORE INSERT OR UPDATE OR DELETE, FOR EACH ROW:
    -- PostgreSQL trigger type bitmask = 31.
    -- Verify exact table, trigger, function, schema,
    -- enabled state, and event contract.

    SELECT EXISTS (
        SELECT 1
        FROM pg_catalog.pg_trigger t
        JOIN pg_catalog.pg_proc p ON p.oid = t.tgfoid
        JOIN pg_catalog.pg_namespace n ON n.oid = p.pronamespace
        WHERE t.tgrelid = 'public.stripe_connect_ownership_bindings'::regclass
          AND t.tgname = 'trg_protect_stripe_connect_ownership_binding'
          AND NOT t.tgisinternal
          AND t.tgenabled IN ('O', 'A')
          AND t.tgtype = 31
          AND p.proname = 'protect_stripe_connect_ownership_binding'
          AND n.nspname = 'public'
    )
    INTO ownership_trigger_present;

    SELECT EXISTS (
        SELECT 1
        FROM pg_catalog.pg_trigger t
        JOIN pg_catalog.pg_proc p ON p.oid = t.tgfoid
        JOIN pg_catalog.pg_namespace n ON n.oid = p.pronamespace
        WHERE t.tgrelid = 'public.stripe_connect_account_link_sessions'::regclass
          AND t.tgname = 'trg_protect_stripe_connect_account_link_session'
          AND NOT t.tgisinternal
          AND t.tgenabled IN ('O', 'A')
          AND t.tgtype = 31
          AND p.proname = 'protect_stripe_connect_account_link_session'
          AND n.nspname = 'public'
    )
    INTO link_trigger_present;

    SELECT EXISTS (
        SELECT 1
        FROM pg_catalog.pg_trigger t
        JOIN pg_catalog.pg_proc p ON p.oid = t.tgfoid
        JOIN pg_catalog.pg_namespace n ON n.oid = p.pronamespace
        WHERE t.tgrelid = 'public.stripe_connect_provisioning_intents'::regclass
          AND t.tgname = 'trg_protect_stripe_connect_provisioning_intent'
          AND NOT t.tgisinternal
          AND t.tgenabled IN ('O', 'A')
          AND t.tgtype = 31
          AND p.proname = 'protect_stripe_connect_provisioning_intent'
          AND n.nspname = 'public'
    )
    INTO provisioning_trigger_present;

    IF NOT ownership_trigger_present
       OR NOT link_trigger_present
       OR NOT provisioning_trigger_present THEN
        RAISE EXCEPTION
            'STOP: Stripe protective trigger contract invalid';
    END IF;

    RAISE NOTICE
        'PASS: migration 79, three ledgers, fourteen constraints, three exact protective triggers';
END
$$;

COMMIT;
SQL

printf '\n===== POST-MIGRATION SCHEMA VERIFICATION PASSED =====\n'
printf 'Read-only verification completed\n'
printf 'No migration executed\n'
printf 'No Stripe provisioning activated\n'
