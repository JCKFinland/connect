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

ORCHESTRATOR="$BACKEND_DIR/scripts/stripe_provisioning_deployment_verification.sh"

test -f "$ORCHESTRATOR"

command -v timeout >/dev/null
command -v python >/dev/null

TMP="$(mktemp -d)"
trap 'rm -rf -- "$TMP"' EXIT

mkdir -p "$TMP/backend/scripts"

cp "$ORCHESTRATOR" \
    "$TMP/backend/scripts/stripe_provisioning_deployment_verification.sh"

GATES=(
    stripe_provisioning_deployment_approval_gate.sh
    stripe_provisioning_migration_gate.sh
    stripe_provisioning_database_identity_gate.sh
    stripe_provisioning_migration_preflight.sh
)

for i in "${!GATES[@]}"; do
    number=$((i + 1))
    gate="${GATES[$i]}"

    cat > "$TMP/backend/scripts/$gate" <<SH
#!/usr/bin/env bash
set -euo pipefail
printf 'GATE_EXECUTED:%s\n' '$number'
if [ "\${FAIL_GATE:-0}" = '$number' ]; then
    printf 'SIMULATED_GATE_FAILURE:%s\n' '$number'
    exit 42
fi
exit 0
SH

    chmod +x "$TMP/backend/scripts/$gate"
    bash -n "$TMP/backend/scripts/$gate"
done

FIXTURE="$TMP/backend/scripts/stripe_provisioning_deployment_verification.sh"

bash -n "$FIXTURE"

run_orchestrator() {
    timeout 15s env -i \
        PATH="/usr/bin:/bin" \
        HOME="$TMP" \
        APP_ENV=production \
        STRIPE_CONNECT_PROVISIONING_ENABLED=false \
        FLEX_SETTLEMENT_WORKER_ENABLED=false \
        FLEX_AUTHORIZATION_RECOVERY_WORKER_ENABLED=false \
        CONNECT_DEPLOYMENT_GIT_BIN="$BASH" \
        CONNECT_DEPLOYMENT_SHA256SUM_BIN="$BASH" \
        CONNECT_DEPLOYMENT_PSQL_BIN="$BASH" \
        FAIL_GATE="$1" \
        bash "$FIXTURE"
}

for failed in 1 2 3 4; do
    set +e
    run_orchestrator "$failed" > "$TMP/output" 2>&1
    result=$?
    set -e

    test "$result" -eq 42 || {
        echo "FAIL: unexpected exit for gate $failed: $result"
        cat "$TMP/output"
        exit 1
    }

    python - "$TMP/output" "$failed" <<'PY'
from pathlib import Path
import sys

output = Path(sys.argv[1]).read_text()
failed = int(sys.argv[2])

actual = [
    line.strip()
    for line in output.splitlines()
    if line.startswith("GATE_EXECUTED:")
]

expected = [
    f"GATE_EXECUTED:{i}"
    for i in range(1, failed + 1)
]

if actual != expected:
    raise SystemExit("FAIL: gate execution order")

if output.count(f"SIMULATED_GATE_FAILURE:{failed}") != 1:
    raise SystemExit("FAIL: missing failure marker")

if "DEPLOYMENT VERIFICATION COMPLETE" in output:
    raise SystemExit("FAIL: false success after gate failure")

print(f"PASS: gate {failed} fails closed")
PY
done

run_orchestrator 0 > "$TMP/output" 2>&1

python - "$TMP/output" <<'PY'
from pathlib import Path
import sys

output = Path(sys.argv[1]).read_text()

actual = [
    line.strip()
    for line in output.splitlines()
    if line.startswith("GATE_EXECUTED:")
]

expected = [
    "GATE_EXECUTED:1",
    "GATE_EXECUTED:2",
    "GATE_EXECUTED:3",
    "GATE_EXECUTED:4",
]

if actual != expected:
    raise SystemExit("FAIL: success-path gate order")

if "DEPLOYMENT VERIFICATION COMPLETE" not in output:
    raise SystemExit("FAIL: missing success marker")

print("PASS: success-path gate order")
PY


printf '\n===== PRODUCTION SAFETY FLAG REGRESSION =====\n'

SAFETY_VARS=(
    APP_ENV
    STRIPE_CONNECT_PROVISIONING_ENABLED
    FLEX_SETTLEMENT_WORKER_ENABLED
    FLEX_AUTHORIZATION_RECOVERY_WORKER_ENABLED
)

SAFETY_PASS_COUNT=0

for TARGET_VAR in "${SAFETY_VARS[@]}"; do
    for CASE in missing unsafe; do

        ENV_ARGS=(
            PATH=/usr/bin:/bin
            HOME="$TMP"
            APP_ENV=production
            STRIPE_CONNECT_PROVISIONING_ENABLED=false
            FLEX_SETTLEMENT_WORKER_ENABLED=false
            FLEX_AUTHORIZATION_RECOVERY_WORKER_ENABLED=false
            CONNECT_DEPLOYMENT_GIT_BIN="$BASH"
            CONNECT_DEPLOYMENT_SHA256SUM_BIN="$BASH"
            CONNECT_DEPLOYMENT_PSQL_BIN="$BASH"
            FAIL_GATE=0
        )

        FILTERED=()

        for ENTRY in "${ENV_ARGS[@]}"; do
            case "$ENTRY" in
                "$TARGET_VAR="*)
                    if [ "$CASE" = missing ]; then
                        continue
                    fi

                    if [ "$TARGET_VAR" = APP_ENV ]; then
                        FILTERED+=("$TARGET_VAR=development")
                    else
                        FILTERED+=("$TARGET_VAR=true")
                    fi
                    ;;
                *)
                    FILTERED+=("$ENTRY")
                    ;;
            esac
        done

        case "$TARGET_VAR:$CASE" in
            APP_ENV:missing)
                EXPECTED='STOP: APP_ENV required'
                ;;
            APP_ENV:unsafe)
                EXPECTED='STOP: production environment required'
                ;;
            STRIPE_CONNECT_PROVISIONING_ENABLED:missing)
                EXPECTED='STOP: provisioning flag required'
                ;;
            STRIPE_CONNECT_PROVISIONING_ENABLED:unsafe)
                EXPECTED='STOP: Stripe provisioning must remain disabled'
                ;;
            FLEX_SETTLEMENT_WORKER_ENABLED:missing)
                EXPECTED='STOP: settlement worker flag required'
                ;;
            FLEX_SETTLEMENT_WORKER_ENABLED:unsafe)
                EXPECTED='STOP: settlement worker must remain disabled'
                ;;
            FLEX_AUTHORIZATION_RECOVERY_WORKER_ENABLED:missing)
                EXPECTED='STOP: recovery worker flag required'
                ;;
            FLEX_AUTHORIZATION_RECOVERY_WORKER_ENABLED:unsafe)
                EXPECTED='STOP: authorization recovery worker must remain disabled'
                ;;
        esac

        set +e

        timeout 15s env -i "${FILTERED[@]}" \
            bash "$FIXTURE" > "$TMP/safety_output" 2>&1

        RESULT=$?

        set -e

        if [ "$RESULT" -eq 0 ] || [ "$RESULT" -eq 124 ]; then
            echo "FAIL: unsafe safety flag accepted or timed out"
            echo "VARIABLE: $TARGET_VAR"
            echo "CASE: $CASE"
            cat "$TMP/safety_output"
            exit 1
        fi

        if ! grep -Fq "$EXPECTED" "$TMP/safety_output"; then
            echo "FAIL: expected safety rejection missing"
            echo "VARIABLE: $TARGET_VAR"
            echo "CASE: $CASE"
            echo "EXPECTED: $EXPECTED"
            cat "$TMP/safety_output"
            exit 1
        fi

        if grep -Fq 'GATE_EXECUTED:' "$TMP/safety_output"; then
            echo "FAIL: deployment gate executed for unsafe flag"
            cat "$TMP/safety_output"
            exit 1
        fi

        if grep -Fq 'DEPLOYMENT VERIFICATION COMPLETE' \
            "$TMP/safety_output"; then
            echo "FAIL: false deployment verification success"
            cat "$TMP/safety_output"
            exit 1
        fi

        printf 'PASS: %-45s %-8s rejected\n' \
            "$TARGET_VAR" "$CASE"

        SAFETY_PASS_COUNT=$((SAFETY_PASS_COUNT + 1))
    done
done

test "$SAFETY_PASS_COUNT" -eq 8 || {
    echo "FAIL: incomplete safety regression matrix"
    exit 1
}

echo "EIGHT PRODUCTION SAFETY FLAG REGRESSIONS PASSED"

echo "DEPLOYMENT ORCHESTRATOR REGRESSION PASSED"
