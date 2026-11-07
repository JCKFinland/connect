#!/usr/bin/env bash
set -euo pipefail

export LC_ALL=C

printf '\n===== POSTGRESQL EXECUTABLE SHADOWING REGRESSION =====\n'

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

IDENTITY="$ROOT/scripts/stripe_provisioning_database_identity_gate.sh"
PREFLIGHT="$ROOT/scripts/stripe_provisioning_migration_preflight.sh"

test -f "$IDENTITY" || {
    echo "STOP: database identity gate missing"
    exit 1
}

test -f "$PREFLIGHT" || {
    echo "STOP: migration preflight gate missing"
    exit 1
}

TMP="$(mktemp -d)"
trap 'rm -rf -- "$TMP"' EXIT

mkdir -p "$TMP/bin" "$TMP/gates"

cp "$IDENTITY" "$TMP/gates/identity.sh"
cp "$PREFLIGHT" "$TMP/gates/preflight.sh"

printf 'SANDBOX ONLY\n' > "$TMP/fake-ca.pem"

cat > "$TMP/bin/psql" <<'SHADOW'
#!/bin/sh
printf 'SHADOW_EXECUTED\n' >> "$CONNECT_PSQL_SHADOW_LOG"
cat > /dev/null
exit 0
SHADOW

cat > "$TMP/approved-psql" <<'APPROVED'
#!/bin/sh
printf 'APPROVED_EXECUTED\n' >> "$CONNECT_PSQL_APPROVED_LOG"
cat > /dev/null
exit 99
APPROVED

chmod +x "$TMP/bin/psql" "$TMP/approved-psql"

printf 'NOT EXECUTABLE\n' > "$TMP/not-executable"

PATH="$TMP/bin:$PATH" \
bash -c 'test "$(command -v psql)" = "$1"' \
    _ "$TMP/bin/psql" || {
    echo "STOP: fake psql not first in PATH"
    exit 1
}

printf 'PASS: fake psql is first in PATH\n'

for GATE in identity preflight; do
    printf '\n--- %s ---\n' "$GATE"

    for CASE in missing relative nonexistent nonexecutable approved; do
        SHADOW_LOG="$TMP/${GATE}-${CASE}-shadow.log"
        APPROVED_LOG="$TMP/${GATE}-${CASE}-approved.log"
        OUTPUT="$TMP/${GATE}-${CASE}-output"

        rm -f "$SHADOW_LOG" "$APPROVED_LOG"

        case "$CASE" in
            missing)
                PSQL_BIN=
                ;;
            relative)
                PSQL_BIN=psql
                ;;
            nonexistent)
                PSQL_BIN="$TMP/does-not-exist"
                ;;
            nonexecutable)
                PSQL_BIN="$TMP/not-executable"
                ;;
            approved)
                PSQL_BIN="$TMP/approved-psql"
                ;;
        esac

        set +e

        env \
            -u CONNECT_DEPLOYMENT_PSQL_BIN \
            APP_ENV=production \
            STRIPE_CONNECT_PROVISIONING_ENABLED=false \
            DB_HOST=invalid.example \
            DB_PORT=5432 \
            DB_USER=nonexistent \
            DB_NAME=connect_shadow_audit \
            DB_SSLMODE=verify-full \
            PGSSLROOTCERT="$TMP/fake-ca.pem" \
            CONNECT_DEPLOYMENT_DATABASE_NAME=connect_shadow_audit \
            CONNECT_DEPLOYMENT_CLUSTER_SYSTEM_IDENTIFIER=123456789 \
            CONNECT_DEPLOYMENT_DATABASE_OID=16384 \
            CONNECT_PSQL_SHADOW_LOG="$SHADOW_LOG" \
            CONNECT_PSQL_APPROVED_LOG="$APPROVED_LOG" \
            CONNECT_DEPLOYMENT_PSQL_BIN="$PSQL_BIN" \
            PATH="$TMP/bin:$PATH" \
            bash "$TMP/gates/${GATE}.sh" \
                > "$OUTPUT" 2>&1

        RESULT=$?

        set -e

        if [ "$RESULT" -eq 0 ]; then
            echo "STOP: false success: $GATE / $CASE"
            exit 1
        fi

        if [ -e "$SHADOW_LOG" ]; then
            echo "STOP: shadow psql executed: $GATE / $CASE"
            exit 1
        fi

        if [ "$CASE" = approved ]; then
            test "$RESULT" -eq 99 || {
                echo "STOP: approved executable failure not propagated"
                exit 1
            }

            test -f "$APPROVED_LOG" || {
                echo "STOP: approved executable not invoked"
                exit 1
            }

            test "$(grep -c '^APPROVED_EXECUTED$' "$APPROVED_LOG")" -eq 1 || {
                echo "STOP: unexpected approved invocation count"
                exit 1
            }
        else
            test ! -e "$APPROVED_LOG" || {
                echo "STOP: approved executable invoked unexpectedly"
                exit 1
            }
        fi

        printf 'PASS: %-10s %-14s exit=%s\n' \
            "$GATE" "$CASE" "$RESULT"
    done
done

printf '\nPOSTGRESQL EXECUTABLE SHADOWING REGRESSION PASSED\n'
