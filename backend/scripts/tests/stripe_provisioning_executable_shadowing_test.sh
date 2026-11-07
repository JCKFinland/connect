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

test -f "$SOURCE" || {
    echo "STOP: deployment orchestrator missing"
    exit 1
}

REAL_BASH="$BASH"

case "$REAL_BASH" in
    /*) ;;
    *)
        echo "STOP: active Bash interpreter path unavailable"
        exit 1
        ;;
esac

test -x "$REAL_BASH" || {
    echo "STOP: active Bash interpreter not executable"
    exit 1
}

TMP="$(mktemp -d)"
trap 'rm -rf -- "$TMP"' EXIT

mkdir -p "$TMP/backend/scripts" "$TMP/bin"

cp "$SOURCE" \
    "$TMP/backend/scripts/stripe_provisioning_deployment_verification.sh"

GATES=(
    stripe_provisioning_deployment_approval_gate.sh
    stripe_provisioning_migration_gate.sh
    stripe_provisioning_database_identity_gate.sh
    stripe_provisioning_migration_preflight.sh
)

for GATE in "${GATES[@]}"; do
    cat > "$TMP/backend/scripts/$GATE" <<'GATE_SCRIPT'
#!/usr/bin/env bash
printf 'ACTUAL_GATE_EXECUTED\n'
exit 99
GATE_SCRIPT
done

cat > "$TMP/bin/bash" <<'SHADOW_SCRIPT'
#!/bin/sh
printf '%s\n' "$*" >> "$CONNECT_SHADOW_AUDIT_LOG"
exit 0
SHADOW_SCRIPT

chmod +x "$TMP/bin/bash"

"$REAL_BASH" -n \
    "$TMP/backend/scripts/stripe_provisioning_deployment_verification.sh"

PATH="$TMP/bin:$PATH" \
"$REAL_BASH" -c '
    test "$(command -v bash)" = "$1"
' _ "$TMP/bin/bash" || {
    echo "STOP: fake Bash not first in PATH"
    exit 1
}

printf '\n===== EXECUTABLE SHADOWING REGRESSION =====\n'

set +e

APP_ENV=production \
STRIPE_CONNECT_PROVISIONING_ENABLED=false \
FLEX_SETTLEMENT_WORKER_ENABLED=false \
FLEX_AUTHORIZATION_RECOVERY_WORKER_ENABLED=false \
CONNECT_DEPLOYMENT_GIT_BIN="$REAL_BASH" \
CONNECT_DEPLOYMENT_SHA256SUM_BIN="$REAL_BASH" \
CONNECT_DEPLOYMENT_PSQL_BIN="$REAL_BASH" \
CONNECT_SHADOW_AUDIT_LOG="$TMP/shadow.log" \
PATH="$TMP/bin:$PATH" \
"$REAL_BASH" \
    "$TMP/backend/scripts/stripe_provisioning_deployment_verification.sh" \
    > "$TMP/output" 2>&1

RESULT=$?

set -e

python - "$TMP/output" "$TMP/shadow.log" "$RESULT" <<'PY'
from pathlib import Path
import sys

output = Path(sys.argv[1]).read_text(
    encoding="utf-8",
    errors="replace",
)

shadow = Path(sys.argv[2])
result = int(sys.argv[3])

if result != 99:
    raise SystemExit(
        f"STOP: expected failing dummy gate exit 99, got {result}"
    )

if output.count("ACTUAL_GATE_EXECUTED") != 1:
    raise SystemExit(
        "STOP: real dummy gate did not execute exactly once"
    )

if "DEPLOYMENT VERIFICATION COMPLETE" in output:
    raise SystemExit(
        "STOP: false deployment verification success"
    )

if shadow.exists() and shadow.read_bytes():
    raise SystemExit(
        "STOP: substituted Bash intercepted execution"
    )

print("PASS: substituted Bash was not invoked")
print("PASS: actual dummy gate executed")
print("PASS: gate failure propagated")
print("PASS: false success prevented")
print("EXECUTABLE SHADOWING REGRESSION PASSED")
PY
