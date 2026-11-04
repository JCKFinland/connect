#!/usr/bin/env bash
set -euo pipefail

printf '\n===== CONNECT STRIPE DATABASE IDENTITY GATE =====\n'

: "${DB_HOST:?STOP: DB_HOST required}"
: "${DB_PORT:?STOP: DB_PORT required}"
: "${DB_USER:?STOP: DB_USER required}"
: "${DB_NAME:?STOP: DB_NAME required}"
: "${DB_SSLMODE:?STOP: DB_SSLMODE required}"
: "${APP_ENV:?STOP: APP_ENV required}"

case "$APP_ENV" in
    production)
        if [ "$DB_SSLMODE" != verify-full ]; then
            echo "STOP: production requires verify-full TLS"
            exit 1
        fi

        : "${PGSSLROOTCERT:?STOP: trusted PostgreSQL CA required}"

        test -f "$PGSSLROOTCERT" || {
            echo "STOP: PostgreSQL CA file not found"
            exit 1
        }
        ;;
    test)
        if [ "$DB_HOST" != localhost ] &&
           [ "$DB_HOST" != 127.0.0.1 ] &&
           [ "$DB_HOST" != ::1 ]; then
            echo "STOP: test mode requires localhost"
            exit 1
        fi

        if [ "$DB_NAME" != connect_payout_destination_test ]; then
            echo "STOP: test mode requires isolated test database"
            exit 1
        fi
        ;;
    *)
        echo "STOP: unsupported environment"
        exit 1
        ;;
esac

: "${CONNECT_DEPLOYMENT_DATABASE_NAME:?STOP: approved database name required}"
: "${CONNECT_DEPLOYMENT_CLUSTER_SYSTEM_IDENTIFIER:?STOP: approved cluster identifier required}"
: "${CONNECT_DEPLOYMENT_DATABASE_OID:?STOP: approved database OID required}"

if [ "$DB_NAME" != "$CONNECT_DEPLOYMENT_DATABASE_NAME" ]; then
    echo "STOP: approved database name mismatch"
    exit 1
fi

case "$DB_NAME" in
    postgres|template0|template1)
        echo "STOP: unsafe database target"
        exit 1
        ;;
esac

if ! [[ "$DB_PORT" =~ ^[0-9]+$ ]] ||
   [ "$DB_PORT" -lt 1 ] ||
   [ "$DB_PORT" -gt 65535 ]; then
    echo "STOP: invalid database port"
    exit 1
fi

if ! [[ "$CONNECT_DEPLOYMENT_CLUSTER_SYSTEM_IDENTIFIER" =~ ^[0-9]+$ ]]; then
    echo "STOP: invalid cluster identifier"
    exit 1
fi

if ! [[ "$CONNECT_DEPLOYMENT_DATABASE_OID" =~ ^[0-9]+$ ]]; then
    echo "STOP: invalid database OID"
    exit 1
fi

command -v psql >/dev/null || {
    echo "STOP: psql unavailable"
    exit 1
}

printf '\n===== READ-ONLY DATABASE IDENTITY VERIFICATION =====\n'

PGSSLMODE="$DB_SSLMODE" \
psql -X -v ON_ERROR_STOP=1 \
    -h "$DB_HOST" \
    -p "$DB_PORT" \
    -U "$DB_USER" \
    -d "$DB_NAME" \
    -v expected_name="$CONNECT_DEPLOYMENT_DATABASE_NAME" \
    -v expected_cluster="$CONNECT_DEPLOYMENT_CLUSTER_SYSTEM_IDENTIFIER" \
    -v expected_oid="$CONNECT_DEPLOYMENT_DATABASE_OID" <<'SQL'

BEGIN READ ONLY;

SELECT
    set_config(
        'connect.expected_database_name',
        :'expected_name',
        true
    ),
    set_config(
        'connect.expected_cluster_identifier',
        :'expected_cluster',
        true
    ),
    set_config(
        'connect.expected_database_oid',
        :'expected_oid',
        true
    );

DO $$
DECLARE
    actual_name text;
    actual_cluster text;
    actual_oid text;
BEGIN
    SELECT current_database()
    INTO actual_name;

    SELECT system_identifier::text
    INTO STRICT actual_cluster
    FROM pg_catalog.pg_control_system();

    SELECT oid::text
    INTO STRICT actual_oid
    FROM pg_catalog.pg_database
    WHERE datname = current_database();

    IF actual_name IS DISTINCT FROM
       current_setting('connect.expected_database_name', true) THEN
        RAISE EXCEPTION 'STOP: database name identity mismatch';
    END IF;

    IF actual_cluster IS DISTINCT FROM
       current_setting('connect.expected_cluster_identifier', true) THEN
        RAISE EXCEPTION 'STOP: PostgreSQL cluster identity mismatch';
    END IF;

    IF actual_oid IS DISTINCT FROM
       current_setting('connect.expected_database_oid', true) THEN
        RAISE EXCEPTION 'STOP: PostgreSQL database OID mismatch';
    END IF;
END
$$;

SELECT
    current_database() AS verified_database_name,
    system_identifier::text AS verified_cluster_identifier,
    (
        SELECT oid::text
        FROM pg_catalog.pg_database
        WHERE datname = current_database()
    ) AS verified_database_oid
FROM pg_catalog.pg_control_system();

COMMIT;

SQL

printf '\n===== DATABASE IDENTITY QUERY COMPLETE =====\n'
printf 'Read-only database identity information collected\n'
printf 'No migration executed\n'
