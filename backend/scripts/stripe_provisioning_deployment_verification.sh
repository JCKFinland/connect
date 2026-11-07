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

# Execute every deployment gate with the interpreter running this
# orchestrator, rather than resolving another Bash through PATH.
#
# BASH is maintained by Bash and identifies the active interpreter.
# An inherited PATH must not select the gate interpreter.
TRUSTED_BASH="$BASH"

case "$TRUSTED_BASH" in
    /*) ;;
    *)
        echo "STOP: trusted Bash interpreter path unavailable"
        exit 1
        ;;
esac

test -f "$TRUSTED_BASH" || {
    echo "STOP: trusted Bash interpreter missing"
    exit 1
}

test -x "$TRUSTED_BASH" || {
    echo "STOP: trusted Bash interpreter not executable"
    exit 1
}

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

# Validate all approved executable contracts before executing any gate.
# Individual gates independently enforce their own requirements.
for variable in \
    CONNECT_DEPLOYMENT_GIT_BIN \
    CONNECT_DEPLOYMENT_SHA256SUM_BIN \
    CONNECT_DEPLOYMENT_PSQL_BIN; do

    approved_path="${!variable:-}"

    if [ -z "$approved_path" ]; then
        printf 'STOP: approved executable required: %s\n' "$variable"
        exit 1
    fi

    case "$approved_path" in
        /*) ;;
        *)
            printf 'STOP: approved executable path must be absolute: %s\n' "$variable"
            exit 1
            ;;
    esac

    if [ ! -f "$approved_path" ]; then
        printf 'STOP: approved executable missing: %s\n' "$variable"
        exit 1
    fi

    if [ ! -x "$approved_path" ]; then
        printf 'STOP: approved executable not executable: %s\n' "$variable"
        exit 1
    fi
done

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

    "$TRUSTED_BASH" -n "$gate" || {
        printf 'STOP: invalid gate syntax: %s\n' "$gate"
        exit 1
    }
done

printf '\n===== 1. DEPLOYMENT APPROVAL EVIDENCE =====\n'
"$TRUSTED_BASH" "${GATES[0]}"

printf '\n===== 2. MIGRATION AUTHORIZATION AND INTEGRITY =====\n'
"$TRUSTED_BASH" "${GATES[1]}"

printf '\n===== 3. POSTGRESQL DATABASE IDENTITY =====\n'
"$TRUSTED_BASH" "${GATES[2]}"

printf '\n===== 4. READ-ONLY DATABASE SCHEMA PREFLIGHT =====\n'
"$TRUSTED_BASH" "${GATES[3]}"

printf '\n===== DEPLOYMENT VERIFICATION COMPLETE =====\n'
printf 'Approval evidence gate passed\n'
printf 'Migration authorization and integrity gate passed\n'
printf 'Database identity gate passed\n'
printf 'Read-only schema preflight passed\n'
printf 'No migration executed\n'
printf 'No Stripe provisioning activated\n'
printf 'A separate approved migration work order is required\n'
