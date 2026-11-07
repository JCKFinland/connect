#!/usr/bin/env bash
set -euo pipefail

# CONNECT deployment executable-shadowing regression.
# All executions occur against temporary sandbox gate copies.
# No production services or databases are contacted.

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)"
BACKEND_DIR="$(cd -- "$SCRIPT_DIR/../.." && pwd -P)"

APPROVAL="$BACKEND_DIR/scripts/stripe_provisioning_deployment_approval_gate.sh"
MIGRATION="$BACKEND_DIR/scripts/stripe_provisioning_migration_gate.sh"

test -f "$APPROVAL" || {
    echo "STOP: approval gate missing"
    exit 1
}

test -f "$MIGRATION" || {
    echo "STOP: migration gate missing"
    exit 1
}

bash -n "$APPROVAL"
bash -n "$MIGRATION"

TMP="$(mktemp -d)"
trap 'rm -rf -- "$TMP"' EXIT

mkdir -p "$TMP/bin" "$TMP/scripts" "$TMP/docs"

cp "$APPROVAL" "$TMP/scripts/approval.sh"
cp "$MIGRATION" "$TMP/scripts/migration.sh"

printf 'INVALID SANDBOX MANIFEST\n' \
    > "$TMP/docs/stripe_provisioning_migration_manifest.sha256"

cat > "$TMP/bin/git" <<'FAKEGIT'
#!/bin/sh
printf 'SHADOW_GIT_EXECUTED\n' >> "$CONNECT_SHADOW_AUDIT_LOG"
exit 0
FAKEGIT

cat > "$TMP/bin/sha256sum" <<'FAKESHA'
#!/bin/sh
printf 'SHADOW_SHA256_EXECUTED\n' >> "$CONNECT_SHADOW_AUDIT_LOG"
exit 0
FAKESHA

cat > "$TMP/approved-git" <<'APPROVEDGIT'
#!/bin/sh
printf 'APPROVED_GIT_EXECUTED\n' >> "$CONNECT_APPROVED_AUDIT_LOG"
exit 99
APPROVEDGIT

cat > "$TMP/approved-sha256sum" <<'APPROVEDSHA'
#!/bin/sh
printf 'APPROVED_SHA256_EXECUTED\n' >> "$CONNECT_APPROVED_AUDIT_LOG"
exit 99
APPROVEDSHA

printf 'not executable\n' > "$TMP/nonexecutable"

chmod +x \
    "$TMP/bin/git" \
    "$TMP/bin/sha256sum" \
    "$TMP/approved-git" \
    "$TMP/approved-sha256sum"

chmod -x "$TMP/nonexecutable"

SHADOW_PATH="$TMP/bin:$PATH"

test "$(PATH="$SHADOW_PATH" command -v git)" = "$TMP/bin/git" || {
    echo "STOP: fake Git not first in PATH"
    exit 1
}

test "$(PATH="$SHADOW_PATH" command -v sha256sum)" = "$TMP/bin/sha256sum" || {
    echo "STOP: fake SHA256 not first in PATH"
    exit 1
}

printf '\n===== GIT AND SHA256 EXECUTABLE SHADOWING REGRESSION =====\n'
echo "PASS: fake executables first in PATH"

run_case() {
    local gate="$1"
    local scenario="$2"
    local expected_exit="$3"
    local approved_path="$4"
    local output="$TMP/output"
    local result=0

    rm -f \
        "$TMP/shadow.log" \
        "$TMP/approved.log" \
        "$output"

    set +e

    (
        cd "$TMP"

        if [ "$gate" = approval ]; then
            env -u CONNECT_DEPLOYMENT_GIT_BIN \
                DB_HOST=invalid.example \
                DB_PORT=5432 \
                DB_NAME=connect_shadow_audit \
                CONNECT_DEPLOYMENT_RELEASE_COMMIT=aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa \
                CONNECT_DEPLOYMENT_DATABASE_NAME=connect_shadow_audit \
                CONNECT_DEPLOYMENT_DATABASE_HOST=invalid.example \
                CONNECT_DEPLOYMENT_DATABASE_PORT=5432 \
                CONNECT_DEPLOYMENT_OPERATOR=shadow-audit \
                CONNECT_DEPLOYMENT_CHANGE_REFERENCE=SHADOW-AUDIT \
                CONNECT_DEPLOYMENT_BACKUP_REFERENCE=SHADOW-AUDIT \
                CONNECT_DEPLOYMENT_RESTORE_TEST_REFERENCE=SHADOW-AUDIT \
                CONNECT_DEPLOYMENT_WINDOW_REFERENCE=SHADOW-AUDIT \
                CONNECT_DEPLOYMENT_INCIDENT_OWNER=shadow-audit \
                CONNECT_SHADOW_AUDIT_LOG="$TMP/shadow.log" \
                CONNECT_APPROVED_AUDIT_LOG="$TMP/approved.log" \
                PATH="$SHADOW_PATH" \
                CONNECT_DEPLOYMENT_GIT_BIN="$approved_path" \
                bash scripts/approval.sh > "$output" 2>&1
        else
            env -u CONNECT_DEPLOYMENT_SHA256SUM_BIN \
                APP_ENV=production \
                DB_NAME=connect_shadow_audit \
                STRIPE_CONNECT_PROVISIONING_ENABLED=false \
                CONNECT_STRIPE_MIGRATION_AUTHORIZED=true \
                CONNECT_STRIPE_MIGRATION_TARGET_VERSION=79 \
                CONNECT_SHADOW_AUDIT_LOG="$TMP/shadow.log" \
                CONNECT_APPROVED_AUDIT_LOG="$TMP/approved.log" \
                PATH="$SHADOW_PATH" \
                CONNECT_DEPLOYMENT_SHA256SUM_BIN="$approved_path" \
                bash scripts/migration.sh > "$output" 2>&1
        fi
    )

    result=$?

    set -e

    if [ "$result" -ne "$expected_exit" ]; then
        printf 'STOP: %s %s expected exit=%s got=%s\n' \
            "$gate" "$scenario" "$expected_exit" "$result"
        cat "$output"
        exit 1
    fi

    if [ -s "$TMP/shadow.log" ]; then
        printf 'STOP: %s %s invoked PATH-shadowed executable\n' \
            "$gate" "$scenario"
        exit 1
    fi

    if [ "$scenario" = approved ]; then
        test -s "$TMP/approved.log" || {
            echo "STOP: approved executable not invoked"
            exit 1
        }

        if [ "$gate" = approval ]; then
            grep -Fxq APPROVED_GIT_EXECUTED "$TMP/approved.log" || {
                echo "STOP: wrong approved Git invocation"
                exit 1
            }
        else
            grep -Fxq APPROVED_SHA256_EXECUTED "$TMP/approved.log" || {
                echo "STOP: wrong approved SHA256 invocation"
                exit 1
            }
        fi
    else
        if [ -s "$TMP/approved.log" ]; then
            echo "STOP: rejected path invoked an executable"
            exit 1
        fi
    fi

    printf 'PASS: %-10s %-14s exit=%s\n' \
        "$gate" "$scenario" "$result"
}

for GATE in approval migration; do
    printf '\n--- %s ---\n' "$GATE"

    if [ "$GATE" = approval ]; then
        APPROVED="$TMP/approved-git"
    else
        APPROVED="$TMP/approved-sha256sum"
    fi

    run_case "$GATE" missing 1 ""
    run_case "$GATE" relative 1 "relative/executable"
    run_case "$GATE" nonexistent 1 "$TMP/nonexistent"
    run_case "$GATE" nonexecutable 1 "$TMP/nonexecutable"
    run_case "$GATE" approved 99 "$APPROVED"
done

printf '\nGIT AND SHA256 EXECUTABLE SHADOWING REGRESSION PASSED\n'
