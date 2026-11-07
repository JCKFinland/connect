#!/usr/bin/env bash
set -euo pipefail

export LC_ALL=C

SCRIPT_DIR="$(
    cd -- "$(dirname -- "${BASH_SOURCE[0]}")" &&
    pwd -P
)"

BACKEND_DIR="$(
    cd -- "$SCRIPT_DIR/../.." &&
    pwd -P
)"

GATE="$BACKEND_DIR/scripts/stripe_provisioning_migration_gate.sh"

test -f "$GATE" || {
    echo "STOP: migration authorization gate missing"
    exit 1
}

TMP="$(mktemp -d)"
trap 'rm -rf -- "$TMP"' EXIT

mkdir -p "$TMP/backend/scripts" "$TMP/backend/docs"

cp "$GATE" "$TMP/backend/scripts/migration.sh"

printf 'SYNTHETIC MANIFEST\n' \
    > "$TMP/backend/docs/stripe_provisioning_migration_manifest.sha256"

cat > "$TMP/approved-sha256sum" <<'SH'
#!/bin/sh
set -eu

# The approved executable must receive the exact minimal environment.
# No inherited variables or attacker-controlled executable search paths.

test "$#" -eq 2 || exit 91
test "$1" = --check || exit 92
test "$2" = docs/stripe_provisioning_migration_manifest.sha256 || exit 93

test "${PATH:-}" = /usr/bin:/bin || exit 94
test "${LC_ALL:-}" = C || exit 95

if [ -n "${CONNECT_SHA256_ATTACK_MARKER+x}" ]; then
    exit 96
fi

if [ -n "${BASH_ENV+x}" ]; then
    exit 97
fi

if [ -n "${ENV+x}" ]; then
    exit 98
fi

if [ -n "${LD_PRELOAD+x}" ]; then
    exit 99
fi

if [ -n "${HOME+x}" ]; then
    exit 100
fi

if [ -n "${TMPDIR+x}" ]; then
    exit 101
fi

# A successful synthetic checksum executable reports success.
exit 0
SH

chmod +x "$TMP/approved-sha256sum"

printf '\n===== SHA256 ENVIRONMENT ISOLATION REGRESSION =====\n'

(
    cd "$TMP/backend"

    env \
        APP_ENV=production \
        DB_NAME=connect_sha256_audit \
        STRIPE_CONNECT_PROVISIONING_ENABLED=false \
        CONNECT_STRIPE_MIGRATION_AUTHORIZED=true \
        CONNECT_STRIPE_MIGRATION_TARGET_VERSION=79 \
        CONNECT_DEPLOYMENT_SHA256SUM_BIN="$TMP/approved-sha256sum" \
        CONNECT_SHA256_ATTACK_MARKER=hostile \
        HOME="$TMP/attacker-home" \
        TMPDIR="$TMP" \
        bash scripts/migration.sh
) > "$TMP/output" 2>&1 || {
    echo "STOP: hostile environment reached approved SHA256 executable"
    cat "$TMP/output"
    exit 1
}

grep -Fq 'AUTHORIZATION GATE PASSED' "$TMP/output" || {
    echo "STOP: valid isolated checksum execution rejected"
    cat "$TMP/output"
    exit 1
}

echo "PASS: hostile environment excluded"
echo "PASS: approved checksum executable invoked"
echo "PASS: valid isolated authorization accepted"

printf '\nSHA256 ENVIRONMENT ISOLATION REGRESSION PASSED\n'
