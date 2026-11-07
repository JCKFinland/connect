#!/usr/bin/env bash
set -euo pipefail

printf '\n===== GIT ENVIRONMENT ISOLATION REGRESSION =====\n'

ROOT="$(
    cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../.." &&
    pwd -P
)"

SOURCE="$ROOT/scripts/stripe_provisioning_deployment_approval_gate.sh"

TMP="$(mktemp -d)"
trap 'rm -rf -- "$TMP"' EXIT

GIT_BIN="$(command -v git)"

case "$GIT_BIN" in
    /*) ;;
    *) echo "STOP: Git executable must be absolute"; exit 1 ;;
esac

test -f "$GIT_BIN" && test -x "$GIT_BIN" || {
    echo "STOP: Git executable unavailable"
    exit 1
}

mkdir -p "$TMP/backend/scripts"

cp "$SOURCE" \
    "$TMP/backend/scripts/stripe_provisioning_deployment_approval_gate.sh"

"$GIT_BIN" init -q "$TMP/backend"

"$GIT_BIN" -C "$TMP/backend" \
    -c user.name=Regression \
    -c user.email=regression@example.invalid \
    commit --allow-empty -q -m "approved release"

APPROVED_HEAD="$(
    "$GIT_BIN" -C "$TMP/backend" rev-parse HEAD
)"

"$GIT_BIN" init -q "$TMP/attacker"

"$GIT_BIN" -C "$TMP/attacker" \
    -c user.name=Regression \
    -c user.email=regression@example.invalid \
    commit --allow-empty -q -m "attacker release"

ATTACKER_HEAD="$(
    "$GIT_BIN" -C "$TMP/attacker" rev-parse HEAD
)"

test "$APPROVED_HEAD" != "$ATTACKER_HEAD" || {
    echo "STOP: fixture commit collision"
    exit 1
}

run_gate() {
    (
        cd "$TMP/backend"

        APP_ENV=production \
        DB_HOST=approved.example.invalid \
        DB_PORT=5432 \
        DB_NAME=connect_production \
        CONNECT_DEPLOYMENT_RELEASE_COMMIT="$1" \
        CONNECT_DEPLOYMENT_DATABASE_NAME=connect_production \
        CONNECT_DEPLOYMENT_DATABASE_HOST=approved.example.invalid \
        CONNECT_DEPLOYMENT_DATABASE_PORT=5432 \
        CONNECT_DEPLOYMENT_OPERATOR=regression \
        CONNECT_DEPLOYMENT_CHANGE_REFERENCE=change-1 \
        CONNECT_DEPLOYMENT_BACKUP_REFERENCE=backup-1 \
        CONNECT_DEPLOYMENT_RESTORE_TEST_REFERENCE=restore-1 \
        CONNECT_DEPLOYMENT_WINDOW_REFERENCE=window-1 \
        CONNECT_DEPLOYMENT_INCIDENT_OWNER=regression \
        CONNECT_DEPLOYMENT_GIT_BIN="$GIT_BIN" \
        GIT_DIR="$TMP/attacker/.git" \
        GIT_WORK_TREE="$TMP/attacker" \
        GIT_CONFIG_COUNT=1 \
        GIT_CONFIG_KEY_0=core.repositoryformatversion \
        GIT_CONFIG_VALUE_0=0 \
        bash scripts/stripe_provisioning_deployment_approval_gate.sh
    )
}

printf '\n===== 1. APPROVED RELEASE WITH HOSTILE GIT ENVIRONMENT =====\n'

run_gate "$APPROVED_HEAD" > "$TMP/approved.log" 2>&1 || {
    cat "$TMP/approved.log"
    echo "STOP: approved release rejected under hostile environment"
    exit 1
}

grep -Fq "Release commit: $APPROVED_HEAD" "$TMP/approved.log" || {
    cat "$TMP/approved.log"
    echo "STOP: approved release identity not verified"
    exit 1
}

echo "PASS: approved checkout identity preserved"

printf '\n===== 2. ATTACKER RELEASE MUST BE REJECTED =====\n'

if run_gate "$ATTACKER_HEAD" > "$TMP/attacker.log" 2>&1; then
    cat "$TMP/attacker.log"
    echo "STOP: attacker-controlled release identity accepted"
    exit 1
fi

grep -Fq \
    "STOP: approved release commit mismatch" \
    "$TMP/attacker.log" || {
    cat "$TMP/attacker.log"
    echo "STOP: expected fail-closed rejection missing"
    exit 1
}

echo "PASS: attacker release identity rejected"

printf '\n===== GIT ENVIRONMENT ISOLATION REGRESSION PASSED =====\n'
