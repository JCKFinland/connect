#!/usr/bin/env bash
set -euo pipefail

printf '\n===== CONNECT STRIPE DEPLOYMENT APPROVAL EVIDENCE =====\n'

require_evidence() {
    local variable="$1"
    local value="${!variable:-}"

    if [ -z "$value" ]; then
        printf 'STOP: %s required\n' "$variable"
        exit 1
    fi

    case "$value" in
        TODO|TBD|UNKNOWN|PLACEHOLDER|NONE|N/A|test|dummy)
            printf 'STOP: placeholder evidence: %s\n' "$variable"
            exit 1
            ;;
    esac
}

required=(
    CONNECT_DEPLOYMENT_RELEASE_COMMIT
    CONNECT_DEPLOYMENT_DATABASE_NAME
    CONNECT_DEPLOYMENT_DATABASE_HOST
    CONNECT_DEPLOYMENT_DATABASE_PORT
    CONNECT_DEPLOYMENT_OPERATOR
    CONNECT_DEPLOYMENT_CHANGE_REFERENCE
    CONNECT_DEPLOYMENT_BACKUP_REFERENCE
    CONNECT_DEPLOYMENT_RESTORE_TEST_REFERENCE
    CONNECT_DEPLOYMENT_WINDOW_REFERENCE
    CONNECT_DEPLOYMENT_INCIDENT_OWNER
)

for variable in "${required[@]}"; do
    require_evidence "$variable"
done

: "${DB_HOST:?STOP: DB_HOST required}"
: "${DB_PORT:?STOP: DB_PORT required}"
: "${DB_NAME:?STOP: DB_NAME required}"

if [ "$CONNECT_DEPLOYMENT_DATABASE_HOST" != "$DB_HOST" ]; then
    echo "STOP: approved database host mismatch"
    exit 1
fi

if [ "$CONNECT_DEPLOYMENT_DATABASE_PORT" != "$DB_PORT" ]; then
    echo "STOP: approved database port mismatch"
    exit 1
fi

if [ "$CONNECT_DEPLOYMENT_DATABASE_NAME" != "$DB_NAME" ]; then
    echo "STOP: approved database name mismatch"
    exit 1
fi

if ! [[ "$DB_PORT" =~ ^[0-9]+$ ]] ||
   [ "$DB_PORT" -lt 1 ] ||
   [ "$DB_PORT" -gt 65535 ]; then
    echo "STOP: invalid database port"
    exit 1
fi

case "$DB_NAME" in
    postgres|template0|template1|*_test|*_test_*|test_*)
        echo "STOP: unsafe database name"
        exit 1
        ;;
esac

# Use only the explicitly approved Git executable.
# Never resolve release identity verification through inherited PATH.
: "${CONNECT_DEPLOYMENT_GIT_BIN:?STOP: approved Git executable required}"

case "$CONNECT_DEPLOYMENT_GIT_BIN" in
    /*) ;;
    *)
        echo "STOP: approved Git path must be absolute"
        exit 1
        ;;
esac

test -f "$CONNECT_DEPLOYMENT_GIT_BIN" || {
    echo "STOP: approved Git executable missing"
    exit 1
}

test -x "$CONNECT_DEPLOYMENT_GIT_BIN" || {
    echo "STOP: approved Git executable not executable"
    exit 1
}

# Resolve release identity from the intended checkout without
# inheriting Git repository, configuration or executable overrides.
#
# -C binds the lookup to the current checkout directory.
# env -i removes inherited GIT_DIR, GIT_WORK_TREE, GIT_CONFIG_*
# and other process-environment overrides.
# --no-optional-locks prevents unnecessary optional Git locks.
actual_commit="$(
    env -i PATH=/usr/bin:/bin \
        "$CONNECT_DEPLOYMENT_GIT_BIN" \
        --no-optional-locks \
        -C "$PWD" \
        rev-parse --verify HEAD^{commit}
)"

if [ "$CONNECT_DEPLOYMENT_RELEASE_COMMIT" != "$actual_commit" ]; then
    echo "STOP: approved release commit mismatch"
    exit 1
fi

printf '\n===== APPROVAL EVIDENCE PRESENT =====\n'
printf 'Release commit: %s\n' "$actual_commit"
printf 'Database: %s\n' "$DB_NAME"
printf 'Database host: %s\n' "$DB_HOST"
printf 'Database port: %s\n' "$DB_PORT"
printf 'Change reference: %s\n' "$CONNECT_DEPLOYMENT_CHANGE_REFERENCE"
printf 'Operator: %s\n' "$CONNECT_DEPLOYMENT_OPERATOR"
printf 'Backup reference: %s\n' "$CONNECT_DEPLOYMENT_BACKUP_REFERENCE"
printf 'Restore test reference: %s\n' "$CONNECT_DEPLOYMENT_RESTORE_TEST_REFERENCE"
printf 'Window reference: %s\n' "$CONNECT_DEPLOYMENT_WINDOW_REFERENCE"
printf 'Incident owner: %s\n' "$CONNECT_DEPLOYMENT_INCIDENT_OWNER"
printf 'Evidence presence verified; authenticity not independently verified\n'
printf 'Database identity not independently authenticated\n'
printf 'No database connection attempted\n'
printf 'No migration executed\n'
