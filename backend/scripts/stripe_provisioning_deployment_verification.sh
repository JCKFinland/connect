#!/usr/bin/env bash
set -euo pipefail

printf '\n===== CONNECT STRIPE DEPLOYMENT VERIFICATION =====\n'

# Resolve the backend directory independently of the caller's CWD.
SCRIPT_DIR="$(
    cd -- "$(dirname -- "${BASH_SOURCE[0]}")" &&
    pwd -P
)"

BACKEND_DIR="$(
    cd -- "$SCRIPT_DIR/.." &&
    pwd -P
)"

cd "$BACKEND_DIR"

: "${APP_ENV:?STOP: APP_ENV required}"
: "${STRIPE_CONNECT_PROVISIONING_ENABLED:?STOP: provisioning flag required}"
: "${FLEX_SETTLEMENT_WORKER_ENABLED:?STOP: settlement worker flag required}"
: "${FLEX_AUTHORIZATION_RECOVERY_WORKER_ENABLED:?STOP: recovery worker flag required}"

if [ "$APP_ENV" != production ]; then
    echo "STOP: production environment required"
    exit 1
fi

if [ "$STRIPE_CONNECT_PROVISIONING_ENABLED" != false ]; then
    echo "STOP: Stripe provisioning must remain disabled"
    exit 1
fi

if [ "$FLEX_SETTLEMENT_WORKER_ENABLED" != false ]; then
    echo "STOP: settlement worker must remain disabled"
    exit 1
fi

if [ "$FLEX_AUTHORIZATION_RECOVERY_WORKER_ENABLED" != false ]; then
    echo "STOP: authorization recovery worker must remain disabled"
    exit 1
fi

GATES=(
    scripts/stripe_provisioning_deployment_approval_gate.sh
    scripts/stripe_provisioning_migration_gate.sh
    scripts/stripe_provisioning_database_identity_gate.sh
    scripts/stripe_provisioning_migration_preflight.sh
)

for gate in "${GATES[@]}"; do
    test -f "$gate" || {
        printf 'STOP: required gate missing: %s\n' "$gate"
        exit 1
    }

    bash -n "$gate" || {
        printf 'STOP: invalid gate syntax: %s\n' "$gate"
        exit 1
    }
done

printf '\n===== 1. DEPLOYMENT APPROVAL EVIDENCE =====\n'
bash "${GATES[0]}"

printf '\n===== 2. MIGRATION AUTHORIZATION AND INTEGRITY =====\n'
bash "${GATES[1]}"

printf '\n===== 3. POSTGRESQL DATABASE IDENTITY =====\n'
bash "${GATES[2]}"

printf '\n===== 4. READ-ONLY DATABASE SCHEMA PREFLIGHT =====\n'
bash "${GATES[3]}"

printf '\n===== DEPLOYMENT VERIFICATION COMPLETE =====\n'
printf 'Approval evidence gate passed\n'
printf 'Migration authorization and integrity gate passed\n'
printf 'Database identity gate passed\n'
printf 'Read-only schema preflight passed\n'
printf 'No migration executed\n'
printf 'No Stripe provisioning activated\n'
printf 'A separate approved migration work order is required\n'
