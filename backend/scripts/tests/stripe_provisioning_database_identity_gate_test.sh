#!/usr/bin/env bash
set -euo pipefail

# CONNECT Stripe database identity regression.
# Requires the isolated local PostgreSQL test container.
# Never targets production.

SCRIPT_DIR="$(
    cd -- "$(dirname -- "${BASH_SOURCE[0]}")" &&
    pwd -P
)"

BACKEND_DIR="$(
    cd -- "$SCRIPT_DIR/../.." &&
    pwd -P
)"

cd "$BACKEND_DIR"

SOURCE=scripts/stripe_provisioning_database_identity_gate.sh
CONTAINER=connect-financial-test-postgres
TEST_DB=connect_settlement_test

export PGCONNECT_TIMEOUT=5

: "${PGPASSWORD:?STOP: isolated PostgreSQL password required}"

test -f "$SOURCE" || {
    echo "STOP: identity gate missing"
    exit 1
}

bash -n "$SOURCE"

test "$(docker inspect -f '{{.State.Running}}' "$CONTAINER")" = true || {
    echo "STOP: isolated PostgreSQL container unavailable"
    exit 1
}

test "$(docker port "$CONTAINER" 5432/tcp)" = 127.0.0.1:55432 || {
    echo "STOP: unsafe PostgreSQL port binding"
    exit 1
}

TMP="$(mktemp -d)"
trap 'rm -rf -- "$TMP"' EXIT

printf '\n===== DATABASE IDENTITY REGRESSION =====\n'

DB_ACTUAL="$(
    psql -X -w -v ON_ERROR_STOP=1 \
        -h 127.0.0.1 -p 55432 \
        -U postgres -d "$TEST_DB" \
        -Atc 'SELECT current_database()'
)"

CLUSTER="$(
    psql -X -w -v ON_ERROR_STOP=1 \
        -h 127.0.0.1 -p 55432 \
        -U postgres -d "$TEST_DB" \
        -Atc \
        'SELECT system_identifier::text FROM pg_catalog.pg_control_system()'
)"

OID="$(
    psql -X -w -v ON_ERROR_STOP=1 \
        -h 127.0.0.1 -p 55432 \
        -U postgres -d "$TEST_DB" \
        -Atc \
        'SELECT oid::text FROM pg_catalog.pg_database WHERE datname=current_database()'
)"

test "$DB_ACTUAL" = "$TEST_DB" || {
    echo "STOP: unexpected test database"
    exit 1
}

[[ "$CLUSTER" =~ ^[0-9]+$ ]] || {
    echo "STOP: invalid cluster identifier"
    exit 1
}

[[ "$OID" =~ ^[0-9]+$ ]] || {
    echo "STOP: invalid database OID"
    exit 1
}

test "$CLUSTER" != 0
test "$OID" != 0

python - "$SOURCE" "$TMP/identity_gate.sh" <<'PYTHON'
from pathlib import Path
import sys

source = Path(sys.argv[1]).read_text(encoding="utf-8")

old = '[ "$DB_NAME" != connect_payout_destination_test ]'
new = '[ "$DB_NAME" != connect_settlement_test ]'

if source.count(old) != 1:
    raise SystemExit("STOP: unexpected test database guard")

modified = source.replace(old, new, 1)

if modified.replace(new, old, 1) != source:
    raise SystemExit("STOP: unexpected gate modification")

Path(sys.argv[2]).write_text(modified, encoding="utf-8")
PYTHON

bash -n "$TMP/identity_gate.sh"

run_gate() {
    local expected_cluster="$1"
    local expected_oid="$2"
    local output="$3"

    APP_ENV=test \
    DB_HOST=127.0.0.1 \
    DB_PORT=55432 \
    DB_USER=postgres \
    DB_NAME="$TEST_DB" \
    DB_SSLMODE=disable \
    CONNECT_DEPLOYMENT_DATABASE_NAME="$TEST_DB" \
    CONNECT_DEPLOYMENT_CLUSTER_SYSTEM_IDENTIFIER="$expected_cluster" \
    CONNECT_DEPLOYMENT_DATABASE_OID="$expected_oid" \
    bash "$TMP/identity_gate.sh" > "$output" 2>&1
}

printf '\n===== 1. CLUSTER MISMATCH =====\n'

if run_gate 0 "$OID" "$TMP/cluster-rejection"; then
    echo "STOP: incorrect cluster identifier accepted"
    exit 1
fi

grep -Fq \
    'STOP: PostgreSQL cluster identity mismatch' \
    "$TMP/cluster-rejection" || {
        echo "STOP: cluster mismatch not observed"
        cat "$TMP/cluster-rejection"
        exit 1
    }

echo "PASS: cluster mismatch rejected"

printf '\n===== 2. DATABASE OID MISMATCH =====\n'

if run_gate "$CLUSTER" 0 "$TMP/oid-rejection"; then
    echo "STOP: incorrect database OID accepted"
    exit 1
fi

grep -Fq \
    'STOP: PostgreSQL database OID mismatch' \
    "$TMP/oid-rejection" || {
        echo "STOP: database OID mismatch not observed"
        cat "$TMP/oid-rejection"
        exit 1
    }

echo "PASS: database OID mismatch rejected"

printf '\n===== 3. CORRECT IDENTITY =====\n'

run_gate "$CLUSTER" "$OID" "$TMP/acceptance" || {
    echo "STOP: correct identity rejected"
    cat "$TMP/acceptance"
    exit 1
}

grep -Fq \
    'DATABASE IDENTITY QUERY COMPLETE' \
    "$TMP/acceptance" || {
        echo "STOP: identity completion marker missing"
        cat "$TMP/acceptance"
        exit 1
    }

echo "PASS: correct identity accepted"

printf '\nDATABASE IDENTITY REGRESSION PASSED\n'
