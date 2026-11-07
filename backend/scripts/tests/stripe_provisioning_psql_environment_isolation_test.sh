#!/usr/bin/env bash
set -euo pipefail
export LC_ALL=C

printf '\n===== POSTGRESQL ENVIRONMENT ISOLATION REGRESSION =====\n'

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

IDENTITY="$ROOT/scripts/stripe_provisioning_database_identity_gate.sh"
PREFLIGHT="$ROOT/scripts/stripe_provisioning_migration_preflight.sh"

TMP="$(mktemp -d)"
trap 'rm -rf -- "$TMP"' EXIT

mkdir -p "$TMP/gates"

PYTHON_BIN="$(command -v python)" || {
    echo "STOP: test Python unavailable"
    exit 1
}

case "$PYTHON_BIN" in
    /*) ;;
    *)
        echo "STOP: test Python path not absolute"
        exit 1
        ;;
esac

test -f "$PYTHON_BIN" && test -x "$PYTHON_BIN" || {
    echo "STOP: test Python executable invalid"
    exit 1
}

cp "$IDENTITY" "$TMP/gates/identity.sh"
cp "$PREFLIGHT" "$TMP/gates/preflight.sh"

printf 'TEST CA\n' > "$TMP/ca.pem"

cat > "$TMP/approved-psql" <<'FAKE'
#!/usr/bin/env bash
set -euo pipefail

__APPROVED_PYTHON__ - <<'PY'
import os
from pathlib import Path

allowed_pg = {
    "PGSSLMODE",
    "PGSSLROOTCERT",
    "PGPASSWORD",
    "PGPASSFILE",
    "PGCONNECT_TIMEOUT",
}

environment = dict(os.environ)

unexpected_pg = sorted(
    name
    for name in environment
    if name.startswith("PG") and name not in allowed_pg
)

if unexpected_pg:
    raise SystemExit(
        "STOP: unauthorized PostgreSQL environment: "
        + ",".join(unexpected_pg)
    )

# Git Bash on Windows may add runtime variables when launching
# shell scripts. These are not libpq connection overrides.
for name in ("PGOPTIONS", "PGSERVICE", "PGHOSTADDR", "PGSSLCERT",
             "PGSSLKEY", "PGSERVICEFILE", "PGTARGETSESSIONATTRS"):
    if name in environment:
        raise SystemExit(
            "STOP: inherited PostgreSQL override: " + name
        )

required = {
    "PGSSLMODE": "verify-full",
    "PGPASSWORD": "isolated-password",
    "PGCONNECT_TIMEOUT": "5",
}

for name, expected in required.items():
    if environment.get(name) != expected:
        raise SystemExit(
            f"STOP: incorrect {name}"
        )

if environment.get("PGOPTIONS"):
    raise SystemExit("STOP: inherited PGOPTIONS")

if environment.get("PGSERVICE"):
    raise SystemExit("STOP: inherited PGSERVICE")

log_path = Path(os.environ["PGPASSFILE"]).with_suffix(".audit")

with log_path.open("a", encoding="utf-8") as handle:
    handle.write("APPROVED_PSQL_EXECUTED\n")
PY

cat > /dev/null
exit 99
FAKE

python - "$TMP/approved-psql" "$PYTHON_BIN" <<'PYTHON'
from pathlib import Path
import sys

path = Path(sys.argv[1])
python_bin = sys.argv[2]

source = path.read_text(encoding="utf-8")

if source.count("__APPROVED_PYTHON__") != 1:
    raise SystemExit("STOP: unexpected Python placeholder")

if "'" in python_bin or "\n" in python_bin:
    raise SystemExit("STOP: unsafe Python executable path")

source = source.replace(
    "__APPROVED_PYTHON__",
    "'" + python_bin + "'",
    1,
)

path.write_text(source, encoding="utf-8")
PYTHON

chmod +x "$TMP/approved-psql"

for GATE in identity preflight; do
    printf '\n--- %s ---\n' "$GATE"

    LOG="$TMP/$GATE.log"
    OUTPUT="$TMP/$GATE.output"

    set +e

    APP_ENV=production \
    STRIPE_CONNECT_PROVISIONING_ENABLED=false \
    DB_HOST=invalid.example \
    DB_PORT=5432 \
    DB_USER=nonexistent \
    DB_NAME=connect_environment_audit \
    DB_SSLMODE=verify-full \
    PGSSLROOTCERT="$TMP/ca.pem" \
    PGPASSWORD=isolated-password \
    PGCONNECT_TIMEOUT=5 \
    PGOPTIONS='-c search_path=attacker' \
    PGSERVICE=attacker \
    PGHOSTADDR=203.0.113.10 \
    PGSSLCERT="$TMP/unauthorized-client.crt" \
    PGTARGETSESSIONATTRS=read-write \
    CONNECT_DEPLOYMENT_DATABASE_NAME=connect_environment_audit \
    CONNECT_DEPLOYMENT_CLUSTER_SYSTEM_IDENTIFIER=123456789 \
    CONNECT_DEPLOYMENT_DATABASE_OID=16384 \
    CONNECT_DEPLOYMENT_PSQL_BIN="$TMP/approved-psql" \
    PGPASSFILE="$TMP/$GATE.pass" \
    bash "$TMP/gates/$GATE.sh" > "$OUTPUT" 2>&1

    RESULT=$?

    set -e

    if [ "$RESULT" -ne 99 ]; then
        echo "STOP: unexpected gate exit: $GATE / $RESULT"
        cat "$OUTPUT"
        exit 1
    fi

    grep -Fxq APPROVED_PSQL_EXECUTED "$TMP/$GATE.audit" || {
        echo "STOP: isolated client did not execute: $GATE"
        cat "$OUTPUT"
        exit 1
    }

    echo "PASS: $GATE isolated PostgreSQL environment"
done

printf '\nPOSTGRESQL ENVIRONMENT ISOLATION REGRESSION PASSED\n'
