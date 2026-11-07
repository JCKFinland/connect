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
MANIFEST="$BACKEND_DIR/docs/stripe_provisioning_migration_manifest.sha256"

test -f "$GATE"
test -f "$MANIFEST"

command -v python >/dev/null
command -v timeout >/dev/null
command -v sha256sum >/dev/null

TMP="$(mktemp -d)"
trap 'rm -rf -- "$TMP"' EXIT

mkdir -p \
    "$TMP/backend/scripts" \
    "$TMP/backend/docs" \
    "$TMP/backend/migrations" \
    "$TMP/bin"

cp "$GATE" "$TMP/backend/scripts/"
cp "$MANIFEST" "$TMP/backend/docs/"

python - "$BACKEND_DIR" "$MANIFEST" "$TMP/backend" <<'PY'
from pathlib import Path
import re
import shutil
import sys

root = Path(sys.argv[1])
manifest = Path(sys.argv[2])
destination = Path(sys.argv[3])

expected = {
    "00076_add_payout_destination_verification_permission.sql",
    "00077_create_stripe_connect_ownership_bindings.sql",
    "00078_create_stripe_connect_account_link_sessions.sql",
    "00079_create_stripe_connect_provisioning_intents.sql",
}

found = set()

for line in manifest.read_text(encoding="utf-8").splitlines():
    if not line.strip() or line.lstrip().startswith("#"):
        continue

    match = re.fullmatch(
        r"([0-9a-fA-F]{64}) [ *](\S+)",
        line,
    )

    if not match:
        raise SystemExit("FAIL: malformed manifest")

    relative = Path(match.group(2))

    if relative.is_absolute() or ".." in relative.parts:
        raise SystemExit("FAIL: unsafe manifest path")

    if relative.parent != Path("migrations"):
        raise SystemExit("FAIL: unexpected manifest directory")

    if relative.name not in expected:
        raise SystemExit("FAIL: unexpected migration")

    if relative.name in found:
        raise SystemExit("FAIL: duplicate migration")

    source = root / relative

    if not source.is_file():
        raise SystemExit("FAIL: missing migration")

    target = destination / relative
    target.parent.mkdir(parents=True, exist_ok=True)
    shutil.copyfile(source, target)

    found.add(relative.name)

if found != expected:
    raise SystemExit("FAIL: migration inventory mismatch")

print("PASS: four protected migration files copied")
PY

cat > "$TMP/bin/psql" <<'SH'
#!/usr/bin/env bash
echo "UNEXPECTED_DATABASE_CONNECTION"
exit 97
SH

cat > "$TMP/bin/goose" <<'SH'
#!/usr/bin/env bash
echo "UNEXPECTED_MIGRATION_EXECUTION"
exit 97
SH

cat > "$TMP/bin/stripe" <<'SH'
#!/usr/bin/env bash
echo "UNEXPECTED_STRIPE_EXECUTION"
exit 97
SH

chmod +x "$TMP/bin/psql" "$TMP/bin/goose" "$TMP/bin/stripe"

FIXTURE="$TMP/backend"

run_gate() {
    (
        cd "$FIXTURE"

        timeout 15s env -i \
            PATH="$TMP/bin:/usr/bin:/bin" \
            HOME="$TMP" \
            APP_ENV=production \
            DB_NAME=connect_production \
            STRIPE_CONNECT_PROVISIONING_ENABLED=false \
            CONNECT_STRIPE_MIGRATION_AUTHORIZED=true \
            CONNECT_STRIPE_MIGRATION_TARGET_VERSION=79 \
            bash scripts/stripe_provisioning_migration_gate.sh
    )
}

assert_no_prohibited_commands() {
    if grep -Eq \
        'UNEXPECTED_DATABASE_CONNECTION|UNEXPECTED_MIGRATION_EXECUTION|UNEXPECTED_STRIPE_EXECUTION' \
        "$1"; then
        echo "FAIL: prohibited external operation"
        exit 1
    fi
}

printf '\n===== CLEAN BASELINE =====\n'

(
    cd "$FIXTURE"
    sha256sum --check --strict \
        docs/stripe_provisioning_migration_manifest.sha256
)

run_gate > "$TMP/output" 2>&1 || {
    echo "FAIL: valid migration baseline rejected"
    cat "$TMP/output"
    exit 1
}

assert_no_prohibited_commands "$TMP/output"

echo "PASS: clean authorization baseline"

printf '\n===== MIGRATION TAMPER CASES =====\n'

for migration in \
    00076_add_payout_destination_verification_permission.sql \
    00077_create_stripe_connect_ownership_bindings.sql \
    00078_create_stripe_connect_account_link_sessions.sql \
    00079_create_stripe_connect_provisioning_intents.sql
do
    FILE="$FIXTURE/migrations/$migration"

    cp "$FILE" "$TMP/original.sql"

    printf '\n-- CONTROLLED TEST TAMPER\n' >> "$FILE"

    set +e
    run_gate > "$TMP/output" 2>&1
    result=$?
    set -e

    cp "$TMP/original.sql" "$FILE"

    if [ "$result" -eq 0 ] || [ "$result" -eq 124 ]; then
        echo "FAIL: unsafe result for $migration: $result"
        cat "$TMP/output"
        exit 1
    fi

    grep -Fq "$migration: FAILED" "$TMP/output" || {
        echo "FAIL: checksum-specific rejection missing"
        cat "$TMP/output"
        exit 1
    }

    assert_no_prohibited_commands "$TMP/output"

    printf 'PASS: %s tampering rejected\n' "$migration"
done

printf '\n===== RESTORED BASELINE =====\n'

(
    cd "$FIXTURE"
    sha256sum --check --strict \
        docs/stripe_provisioning_migration_manifest.sha256
)

run_gate > "$TMP/output" 2>&1 || {
    echo "FAIL: restored baseline rejected"
    cat "$TMP/output"
    exit 1
}

assert_no_prohibited_commands "$TMP/output"

echo "PASS: restored authorization baseline"
echo "MIGRATION INTEGRITY REGRESSION PASSED"
