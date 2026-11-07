#!/usr/bin/env bash
set -euo pipefail

printf '\n===== CONNECT STRIPE MIGRATION AUTHORIZATION GATE =====\n'

EXPECTED_TARGET_VERSION=79
MANIFEST=docs/stripe_provisioning_migration_manifest.sha256

: "${APP_ENV:?STOP: APP_ENV required}"
: "${DB_NAME:?STOP: DB_NAME required}"
: "${STRIPE_CONNECT_PROVISIONING_ENABLED:?STOP: provisioning flag required}"
: "${CONNECT_STRIPE_MIGRATION_AUTHORIZED:?STOP: authorization flag required}"
: "${CONNECT_STRIPE_MIGRATION_TARGET_VERSION:?STOP: target version required}"

if [ "$APP_ENV" != production ]; then
    echo "STOP: production environment required"
    exit 1
fi

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

if [ "$CONNECT_STRIPE_MIGRATION_AUTHORIZED" != true ]; then
    echo "STOP: explicit migration authorization required"
    exit 1
fi

if [ "$CONNECT_STRIPE_MIGRATION_TARGET_VERSION" != "$EXPECTED_TARGET_VERSION" ]; then
    echo "STOP: migration target must be exactly 79"
    exit 1
fi

test -f "$MANIFEST" || {
    echo "STOP: migration integrity manifest missing"
    exit 1
}

# Use only the explicitly approved checksum executable.
# Never resolve migration integrity verification through inherited PATH.
: "${CONNECT_DEPLOYMENT_SHA256SUM_BIN:?STOP: approved SHA256 executable required}"

case "$CONNECT_DEPLOYMENT_SHA256SUM_BIN" in
    /*) ;;
    *)
        echo "STOP: approved SHA256 path must be absolute"
        exit 1
        ;;
esac

test -f "$CONNECT_DEPLOYMENT_SHA256SUM_BIN" || {
    echo "STOP: approved SHA256 executable missing"
    exit 1
}

test -x "$CONNECT_DEPLOYMENT_SHA256SUM_BIN" || {
    echo "STOP: approved SHA256 executable not executable"
    exit 1
}

printf '\n===== VERIFY MIGRATION INTEGRITY =====\n'

"$CONNECT_DEPLOYMENT_SHA256SUM_BIN" --check "$MANIFEST"

printf '\n===== AUTHORIZATION GATE PASSED =====\n'
printf 'Target migration version: %s\n' "$EXPECTED_TARGET_VERSION"
printf 'Migration file integrity verified\n'
printf 'No database connection attempted\n'
printf 'No migration executed\n'
printf 'Production deployment still requires a separate approved work order\n'
