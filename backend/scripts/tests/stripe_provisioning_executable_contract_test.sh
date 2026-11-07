#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(
    cd -- "$(dirname -- "${BASH_SOURCE[0]}")" &&
    pwd -P
)"

BACKEND_DIR="$(
    cd -- "$SCRIPT_DIR/../.." &&
    pwd -P
)"

SOURCE="$BACKEND_DIR/scripts/stripe_provisioning_deployment_verification.sh"

test -f "$SOURCE"

REAL_BASH="$BASH"

case "$REAL_BASH" in
    /*) ;;
    *) echo "STOP: absolute Bash path required"; exit 1 ;;
esac

test -f "$REAL_BASH" && test -x "$REAL_BASH"

TMP="$(mktemp -d)"
trap 'rm -rf -- "$TMP"' EXIT

mkdir -p "$TMP/backend/scripts" "$TMP/bin"

cp "$SOURCE" \
    "$TMP/backend/scripts/stripe_provisioning_deployment_verification.sh"

FIXTURE="$TMP/backend/scripts/stripe_provisioning_deployment_verification.sh"

for gate in \
    stripe_provisioning_deployment_approval_gate.sh \
    stripe_provisioning_migration_gate.sh \
    stripe_provisioning_database_identity_gate.sh \
    stripe_provisioning_migration_preflight.sh; do

    cat > "$TMP/backend/scripts/$gate" <<'GATE'
#!/usr/bin/env bash
printf 'CONTRACT_GATE_EXECUTED\n'
exit 99
GATE
done

cat > "$TMP/bin/approved" <<'APPROVED'
#!/usr/bin/env bash
exit 0
APPROVED

cp "$TMP/bin/approved" "$TMP/bin/nonexecutable"

chmod +x "$TMP/bin/approved"
chmod -x "$TMP/bin/nonexecutable"

# Windows/Git Bash may report executable permission despite chmod -x.
# Never claim this negative case passed unless -x is actually false.
NONEXECUTABLE_SUPPORTED=true

if [ -x "$TMP/bin/nonexecutable" ]; then
    NONEXECUTABLE_SUPPORTED=false
    echo "INFO: filesystem cannot represent nonexecutable fixture"
fi

APPROVED="$TMP/bin/approved"
NONEXECUTABLE="$TMP/bin/nonexecutable"
NONEXISTENT="$TMP/bin/does-not-exist"

VARIABLES=(
    CONNECT_DEPLOYMENT_GIT_BIN
    CONNECT_DEPLOYMENT_SHA256SUM_BIN
    CONNECT_DEPLOYMENT_PSQL_BIN
)

run_case() {
    local variable="$1"
    local mode="$2"
    local value=""
    local expected=1
    local result

    case "$mode" in
        missing) value="" ;;
        relative) value="relative/tool" ;;
        nonexistent) value="$NONEXISTENT" ;;
        nonexecutable) value="$NONEXECUTABLE" ;;
        approved)
            value="$APPROVED"
            expected=99
            ;;
        *)
            echo "STOP: invalid test case"
            exit 1
            ;;
    esac

    local git_bin="$APPROVED"
    local sha_bin="$APPROVED"
    local psql_bin="$APPROVED"

    case "$variable" in
        CONNECT_DEPLOYMENT_GIT_BIN) git_bin="$value" ;;
        CONNECT_DEPLOYMENT_SHA256SUM_BIN) sha_bin="$value" ;;
        CONNECT_DEPLOYMENT_PSQL_BIN) psql_bin="$value" ;;
        *) echo "STOP: invalid variable"; exit 1 ;;
    esac

    set +e

    timeout 15s env -i \
        PATH="/usr/bin:/bin" \
        HOME="$TMP" \
        APP_ENV=production \
        STRIPE_CONNECT_PROVISIONING_ENABLED=false \
        FLEX_SETTLEMENT_WORKER_ENABLED=false \
        FLEX_AUTHORIZATION_RECOVERY_WORKER_ENABLED=false \
        CONNECT_DEPLOYMENT_GIT_BIN="$git_bin" \
        CONNECT_DEPLOYMENT_SHA256SUM_BIN="$sha_bin" \
        CONNECT_DEPLOYMENT_PSQL_BIN="$psql_bin" \
        "$REAL_BASH" "$FIXTURE" \
        > "$TMP/output" 2>&1

    result=$?

    set -e

    if [ "$result" -ne "$expected" ]; then
        printf 'FAIL: %s %s expected=%s actual=%s\n' \
            "$variable" "$mode" "$expected" "$result"
        cat "$TMP/output"
        exit 1
    fi

    if [ "$mode" = approved ]; then
        grep -q '^CONTRACT_GATE_EXECUTED$' "$TMP/output" || {
            echo "STOP: approved contract did not reach first gate"
            exit 1
        }
    else
        if grep -q 'CONTRACT_GATE_EXECUTED' "$TMP/output"; then
            echo "STOP: gate executed with invalid contract"
            exit 1
        fi

        grep -Fq "STOP: approved executable" "$TMP/output" || {
            echo "STOP: expected contract failure missing"
            cat "$TMP/output"
            exit 1
        }

        grep -Fq "$variable" "$TMP/output" || {
            echo "STOP: expected variable not identified"
            cat "$TMP/output"
            exit 1
        }
    fi

    if grep -q 'DEPLOYMENT VERIFICATION COMPLETE' "$TMP/output"; then
        echo "STOP: false deployment success"
        exit 1
    fi

    printf 'PASS: %-38s %-14s exit=%s\n' \
        "$variable" "$mode" "$result"
}

printf '\n===== ORCHESTRATOR EXECUTABLE CONTRACT REGRESSION =====\n'

for variable in "${VARIABLES[@]}"; do
    for mode in \
        missing \
        relative \
        nonexistent \
        nonexecutable \
        approved; do

        if [ "$mode" = nonexecutable ] &&
           [ "$NONEXECUTABLE_SUPPORTED" = false ]; then
            printf 'SKIP: %-38s %-14s unsupported filesystem permissions\n' \
                "$variable" "$mode"
            continue
        fi

        run_case "$variable" "$mode"
    done
done

if [ "$NONEXECUTABLE_SUPPORTED" = true ]; then
    echo "15 ORCHESTRATOR EXECUTABLE CONTRACT REGRESSIONS PASSED"
else
    echo "12 ORCHESTRATOR EXECUTABLE CONTRACT REGRESSIONS PASSED"
    echo "3 nonexecutable cases skipped: filesystem limitation"
fi
