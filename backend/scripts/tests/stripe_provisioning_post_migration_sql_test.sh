#!/usr/bin/env bash
set -euo pipefail

# CONNECT post-migration SQL behavioral regression.
# Requires a separately created, owned, disposable PostgreSQL 16
# container with migrations 1-79 already applied.
# Never creates or modifies a production database.

export LC_ALL=C

SCRIPT_DIR="$(
    cd -- "$(dirname -- "${BASH_SOURCE[0]}")" &&
    pwd -P
)"

BACKEND_DIR="$(
    cd -- "$SCRIPT_DIR/../.." &&
    pwd -P
)"

cd "$BACKEND_DIR"

VERIFIER=scripts/stripe_provisioning_post_migration_verify.sh
CONTAINER=connect-stripe-postmigration-regression
OWNER=postmigration-regression
BASE=connect_stripe_ci_regression

: "${CONNECT_POSTMIGRATION_SQL_TEST:?STOP: explicit test authorization required}"

test "$CONNECT_POSTMIGRATION_SQL_TEST" = 1 || {
    echo "STOP: invalid test authorization"
    exit 1
}

test -f "$VERIFIER" && test ! -L "$VERIFIER" || {
    echo "STOP: verifier missing or symlinked"
    exit 1
}

test "$(docker inspect -f '{{.Name}}' "$CONTAINER")" = "/$CONTAINER" || {
    echo "STOP: unexpected container identity"
    exit 1
}

test "$(docker inspect -f '{{index .Config.Labels "connect.test-suite"}}' "$CONTAINER")" = "$OWNER" || {
    echo "STOP: disposable ownership label missing"
    exit 1
}

test "$(docker inspect -f '{{.Config.Image}}' "$CONTAINER")" = postgres:16 || {
    echo "STOP: unexpected PostgreSQL image"
    exit 1
}

test "$(docker inspect -f '{{.State.Status}}' "$CONTAINER")" = running || {
    echo "STOP: disposable container not running"
    exit 1
}

test "$(docker port "$CONTAINER" 5432/tcp)" = 127.0.0.1:55433 || {
    echo "STOP: unexpected PostgreSQL port binding"
    exit 1
}

CONTAINER_ID="$(docker inspect -f '{{.Id}}' "$CONTAINER")"

TMP="$(mktemp -d)"
trap 'rm -rf -- "$TMP"' EXIT

python - "$VERIFIER" "$TMP/verifier.sql" <<'EXTRACT'
from pathlib import Path
import re
import sys

source = Path(sys.argv[1]).read_text(encoding="utf-8")

matches = re.findall(
    r"<<'SQL'\r?\n(.*?)\r?\nSQL(?:\r?\n|$)",
    source,
    flags=re.DOTALL,
)

if len(matches) != 1:
    raise SystemExit("STOP: expected one authoritative SQL block")

sql = matches[0] + "\n"

for marker in (
    "BEGIN READ ONLY;",
    "COMMIT;",
    "goose_db_version",
    "required_constraints",
    "ownership_trigger_present",
    "link_trigger_present",
    "provisioning_trigger_present",
):
    if marker not in sql:
        raise SystemExit("STOP: missing SQL contract: " + marker)

if sql.count("BEGIN READ ONLY;") != 1:
    raise SystemExit("STOP: unexpected transaction count")

Path(sys.argv[2]).write_text(
    sql, encoding="utf-8", newline="\n"
)
EXTRACT

run_psql() {
    local database="$1"
    shift

    docker exec -i "$CONTAINER_ID" \
        psql -X -v ON_ERROR_STOP=1 \
        -U postgres -d "$database" "$@"
}

printf '\n===== 1. VERIFY DISPOSABLE BASELINE =====\n'

VERSION="$(
    docker exec "$CONTAINER_ID" \
        psql -X -At -v ON_ERROR_STOP=1 \
        -U postgres -d "$BASE" \
        -c "SELECT MAX(version_id) FROM public.goose_db_version WHERE is_applied"
)"

test "$VERSION" = 79 || {
    echo "STOP: expected migration 79"
    exit 1
}

SERVER_VERSION="$(
    docker exec "$CONTAINER_ID" \
        psql -X -At -v ON_ERROR_STOP=1 \
        -U postgres -d "$BASE" \
        -c 'SHOW server_version'
)"

case "$SERVER_VERSION" in
    16.*) ;;
    *)
        echo "STOP: PostgreSQL 16 required"
        exit 1
        ;;
esac

run_psql "$BASE" \
    < "$TMP/verifier.sql" \
    > "$TMP/baseline.log" 2>&1 || {
    cat "$TMP/baseline.log"
    echo "STOP: valid schema rejected"
    exit 1
}

grep -Fq \
    'PASS: migration 79, three ledgers, fourteen constraints, three exact protective triggers' \
    "$TMP/baseline.log" || {
    echo "STOP: valid schema completion notice missing"
    exit 1
}

echo "PASS: valid schema accepted"

printf '\n===== 2. CREATE ISOLATED NEGATIVE DATABASES =====\n'

DATABASES=(
    connect_stripe_regression_constraint
    connect_stripe_regression_disabled
    connect_stripe_regression_function
    connect_stripe_regression_event
    connect_stripe_regression_ownership
)

for database in "${DATABASES[@]}"; do
    exists="$(
        docker exec "$CONTAINER_ID" \
            psql -X -At -v ON_ERROR_STOP=1 \
            -U postgres -d postgres \
            -c "SELECT COUNT(*) FROM pg_catalog.pg_database WHERE datname = '$database'"
    )"

    test "$exists" = 0 || {
        echo "STOP: negative test database already exists: $database"
        exit 1
    }

    docker exec "$CONTAINER_ID" \
        psql -X -v ON_ERROR_STOP=1 \
        -U postgres -d postgres \
        -c "CREATE DATABASE $database TEMPLATE $BASE" \
        >/dev/null
done

echo "PASS: five isolated schema copies"

expect_rejection() {
    local database="$1"
    local expected="$2"
    local output="$TMP/$database.log"

    if run_psql "$database" \
        < "$TMP/verifier.sql" \
        > "$output" 2>&1; then
        echo "STOP: invalid schema accepted: $database"
        exit 1
    fi

    grep -Fq "$expected" "$output" || {
        cat "$output"
        echo "STOP: expected rejection not observed: $database"
        exit 1
    }

    echo "PASS: $database rejected"
}

printf '\n===== 3. MISSING CONSTRAINT =====\n'

run_psql connect_stripe_regression_constraint \
    -c 'ALTER TABLE public.stripe_connect_provisioning_intents DROP CONSTRAINT chk_stripe_provisioning_key' \
    >/dev/null

expect_rejection \
    connect_stripe_regression_constraint \
    'STOP: required Stripe constraint missing or invalid'

printf '\n===== 4. DISABLED TRIGGER =====\n'

run_psql connect_stripe_regression_disabled \
    -c 'ALTER TABLE public.stripe_connect_provisioning_intents DISABLE TRIGGER trg_protect_stripe_connect_provisioning_intent' \
    >/dev/null

expect_rejection \
    connect_stripe_regression_disabled \
    'STOP: Stripe protective trigger contract invalid'

printf '\n===== 5. WRONG TRIGGER FUNCTION =====\n'

run_psql connect_stripe_regression_function <<'SQL'
CREATE FUNCTION public.regression_wrong_trigger()
RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    RETURN NEW;
END;
$$;

ALTER TRIGGER trg_protect_stripe_connect_provisioning_intent
ON public.stripe_connect_provisioning_intents
RENAME TO regression_original_trigger;

CREATE TRIGGER trg_protect_stripe_connect_provisioning_intent
BEFORE INSERT OR UPDATE OR DELETE
ON public.stripe_connect_provisioning_intents
FOR EACH ROW
EXECUTE FUNCTION public.regression_wrong_trigger();
SQL

expect_rejection \
    connect_stripe_regression_function \
    'STOP: Stripe protective trigger contract invalid'

printf '\n===== 6. WRONG TRIGGER EVENT =====\n'

run_psql connect_stripe_regression_event <<'SQL'
ALTER TRIGGER trg_protect_stripe_connect_provisioning_intent
ON public.stripe_connect_provisioning_intents
RENAME TO regression_original_trigger;

CREATE TRIGGER trg_protect_stripe_connect_provisioning_intent
BEFORE INSERT
ON public.stripe_connect_provisioning_intents
FOR EACH ROW
EXECUTE FUNCTION public.protect_stripe_connect_provisioning_intent();
SQL

expect_rejection \
    connect_stripe_regression_event \
    'STOP: Stripe protective trigger contract invalid'

printf '\n===== 7. WRONG CONSTRAINT OWNERSHIP =====\n'

run_psql connect_stripe_regression_ownership <<'SQL'
ALTER TABLE public.stripe_connect_provisioning_intents
DROP CONSTRAINT chk_stripe_provisioning_key;

ALTER TABLE public.stripe_connect_ownership_bindings
ADD CONSTRAINT chk_stripe_provisioning_key CHECK (TRUE);
SQL

expect_rejection \
    connect_stripe_regression_ownership \
    'STOP: required Stripe constraint missing or invalid'

printf '\n===== PERMANENT POST-MIGRATION SQL REGRESSION PASSED =====\n'

echo "PASS: valid schema"
echo "PASS: missing constraint"
echo "PASS: disabled trigger"
echo "PASS: wrong trigger function"
echo "PASS: wrong trigger event"
echo "PASS: wrong constraint ownership"
echo "No production database connections"
echo "No Stripe provisioning activated"
