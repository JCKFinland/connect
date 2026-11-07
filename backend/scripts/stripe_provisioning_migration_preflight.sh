#!/usr/bin/env bash
set -euo pipefail

printf '\n===== CONNECT STRIPE PROVISIONING MIGRATION PREFLIGHT =====\n'

: "${DB_HOST:?STOP: DB_HOST required}"
: "${DB_PORT:?STOP: DB_PORT required}"
: "${DB_USER:?STOP: DB_USER required}"
: "${DB_NAME:?STOP: DB_NAME required}"
: "${DB_SSLMODE:?STOP: DB_SSLMODE required}"
: "${APP_ENV:?STOP: APP_ENV required}"
: "${STRIPE_CONNECT_PROVISIONING_ENABLED:?STOP: provisioning flag required}"

if [ "$APP_ENV" != production ]; then
    echo "STOP: production environment required"
    exit 1
fi

if [ "$DB_SSLMODE" != verify-full ]; then
    echo "STOP: production requires verify-full TLS"
    exit 1
fi

: "${PGSSLROOTCERT:?STOP: trusted PostgreSQL CA required}"

test -f "$PGSSLROOTCERT" || {
    echo "STOP: PostgreSQL CA file not found"
    exit 1
}

if [ "$STRIPE_CONNECT_PROVISIONING_ENABLED" != false ]; then
    echo "STOP: provisioning must remain disabled"
    exit 1
fi

case "$DB_NAME" in
    ""|postgres|template0|template1|connect_payout_destination_test|connect_settlement_test|connect_integration_test)
        echo "STOP: invalid production database target"
        exit 1
        ;;
esac

# Resolve PostgreSQL through an explicitly approved executable path.
# Never select the SQL verification client from inherited PATH.
: "${CONNECT_DEPLOYMENT_PSQL_BIN:?STOP: approved psql executable required}"

case "$CONNECT_DEPLOYMENT_PSQL_BIN" in
    /*) ;;
    *)
        echo "STOP: approved psql path must be absolute"
        exit 1
        ;;
esac

test -f "$CONNECT_DEPLOYMENT_PSQL_BIN" || {
    echo "STOP: approved psql executable missing"
    exit 1
}

test -x "$CONNECT_DEPLOYMENT_PSQL_BIN" || {
    echo "STOP: approved psql executable not executable"
    exit 1
}

printf '\n===== 1. DATABASE CONNECTION =====\n'

printf 'Host: %s\n' "$DB_HOST"
printf 'Port: %s\n' "$DB_PORT"
printf 'Database: %s\n' "$DB_NAME"
printf 'SSL mode: %s\n' "$DB_SSLMODE"

printf '\n===== 2. READ-ONLY SCHEMA PREFLIGHT =====\n'

PGSSLMODE="$DB_SSLMODE" \
"$CONNECT_DEPLOYMENT_PSQL_BIN" -X -v ON_ERROR_STOP=1 \
    -h "$DB_HOST" \
    -p "$DB_PORT" \
    -U "$DB_USER" \
    -d "$DB_NAME" <<'SQL'
BEGIN READ ONLY;

SELECT current_database() AS database_name;

SELECT version_id, is_applied
FROM public.goose_db_version
WHERE version_id BETWEEN 74 AND 79
ORDER BY version_id;

SELECT
    to_regclass('public.users') IS NOT NULL AS users_present,
    to_regclass('public.companies') IS NOT NULL AS companies_present,
    to_regclass('public.permissions') IS NOT NULL AS permissions_present;

SELECT
    to_regclass('public.stripe_connect_ownership_bindings')
        IS NOT NULL AS ownership_bindings_present,
    to_regclass('public.stripe_connect_account_link_sessions')
        IS NOT NULL AS link_sessions_present,
    to_regclass('public.stripe_connect_provisioning_intents')
        IS NOT NULL AS provisioning_intents_present;

DO $$
DECLARE
    latest_version BIGINT;
BEGIN
    SELECT MAX(version_id)
    INTO latest_version
    FROM public.goose_db_version
    WHERE is_applied;

    IF latest_version IS DISTINCT FROM 75 THEN
        RAISE EXCEPTION
            'STOP: expected production migration baseline 75, found %',
            latest_version;
    END IF;

    IF NOT EXISTS (
        SELECT 1 FROM public.goose_db_version
        WHERE version_id = 74 AND is_applied
    ) OR NOT EXISTS (
        SELECT 1 FROM public.goose_db_version
        WHERE version_id = 75 AND is_applied
    ) THEN
        RAISE EXCEPTION 'STOP: payout migration prerequisites missing';
    END IF;

    IF to_regclass('public.users') IS NULL
       OR to_regclass('public.companies') IS NULL
       OR to_regclass('public.permissions') IS NULL THEN
        RAISE EXCEPTION 'STOP: prerequisite tables missing';
    END IF;

    IF to_regclass('public.stripe_connect_ownership_bindings') IS NOT NULL
       OR to_regclass('public.stripe_connect_account_link_sessions') IS NOT NULL
       OR to_regclass('public.stripe_connect_provisioning_intents') IS NOT NULL THEN
        RAISE EXCEPTION
            'STOP: Stripe migration objects already present';
    END IF;
END
$$;

COMMIT;
SQL

printf '\n===== PREFLIGHT PASSED =====\n'
printf 'Baseline migration 75 verified\n'
printf 'Migrations 76-79 require controlled deployment\n'
printf 'No database writes or migrations executed\n'
printf 'Provisioning remains disabled\n'
