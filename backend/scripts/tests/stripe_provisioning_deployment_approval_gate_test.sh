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

GATE="$BACKEND_DIR/scripts/stripe_provisioning_deployment_approval_gate.sh"

test -f "$GATE"
bash -n "$GATE"

TMP="$(mktemp -d)"
trap 'rm -rf -- "$TMP"' EXIT

mkdir -p "$TMP/bin"
cp "$GATE" "$TMP/approval_gate.sh"

cat > "$TMP/bin/git" <<'SH'
#!/usr/bin/env bash
# Match the approved, isolated release-identity Git invocation.
# Reject legacy or unexpected Git operations.
if [ "$#" -eq 6 ] &&
   [ "$1" = --no-optional-locks ] &&
   [ "$2" = -C ] &&
   [ -n "$3" ] &&
   [ "$4" = rev-parse ] &&
   [ "$5" = --verify ] &&
   [ "$6" = 'HEAD^{commit}' ]; then
    printf '%s\n' 'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa'
    exit 0
fi
echo "UNEXPECTED_GIT_OPERATION"
exit 97
SH

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

chmod +x "$TMP/bin/"*

BASH_EXE="$(command -v bash)"
MOCK_PATH="$TMP/bin:$PATH"
MOCK_HEAD=aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa

REQUIRED=(
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

BASE_ENV=(
    "PATH=$MOCK_PATH"
    "CONNECT_DEPLOYMENT_GIT_BIN=$TMP/bin/git"
    "HOME=$TMP"
    "LC_ALL=C"
    "DB_HOST=db.example.invalid"
    "DB_PORT=5432"
    "DB_NAME=connect_production"
    "CONNECT_DEPLOYMENT_RELEASE_COMMIT=$MOCK_HEAD"
    "CONNECT_DEPLOYMENT_DATABASE_NAME=connect_production"
    "CONNECT_DEPLOYMENT_DATABASE_HOST=db.example.invalid"
    "CONNECT_DEPLOYMENT_DATABASE_PORT=5432"
    "CONNECT_DEPLOYMENT_OPERATOR=operator-ref-001"
    "CONNECT_DEPLOYMENT_CHANGE_REFERENCE=change-ref-001"
    "CONNECT_DEPLOYMENT_BACKUP_REFERENCE=backup-ref-001"
    "CONNECT_DEPLOYMENT_RESTORE_TEST_REFERENCE=restore-ref-001"
    "CONNECT_DEPLOYMENT_WINDOW_REFERENCE=window-ref-001"
    "CONNECT_DEPLOYMENT_INCIDENT_OWNER=incident-ref-001"
)

CASE_COUNT=0

run_case() {
    local name="$1"
    local expected="$2"
    shift 2

    local -a environment=("${BASE_ENV[@]}")
    local change key value entry result
    local -a updated

    for change in "$@"; do
        key="${change%%=*}"
        value="${change#*=}"

        updated=()

        for entry in "${environment[@]}"; do
            if [ "${entry%%=*}" != "$key" ]; then
                updated+=("$entry")
            fi
        done

        environment=("${updated[@]}")

        if [ "$value" != "__UNSET__" ]; then
            environment+=("$key=$value")
        fi
    done

    set +e
    env -i "${environment[@]}" \
        "$BASH_EXE" "$TMP/approval_gate.sh" \
        > "$TMP/output" 2>&1
    result=$?
    set -e

    if grep -Eq \
        'UNEXPECTED_DATABASE_CONNECTION|UNEXPECTED_MIGRATION_EXECUTION|UNEXPECTED_STRIPE_EXECUTION|UNEXPECTED_GIT_OPERATION' \
        "$TMP/output"; then
        echo "FAIL: prohibited operation in $name"
        cat "$TMP/output"
        exit 1
    fi

    if [ "$expected" = "__SUCCESS__" ]; then
        if [ "$result" -ne 0 ] ||
           ! grep -Fq 'APPROVAL EVIDENCE PRESENT' "$TMP/output"; then
            echo "FAIL: valid evidence rejected in $name"
            printf 'Exit code: %s\n' "$result"
            cat "$TMP/output"
            exit 1
        fi
    else
        if [ "$result" -eq 0 ] ||
           ! grep -Fq "$expected" "$TMP/output"; then
            echo "FAIL: incorrect rejection in $name"
            printf 'Exit code: %s\n' "$result"
            cat "$TMP/output"
            exit 1
        fi
    fi

    CASE_COUNT=$((CASE_COUNT + 1))
    printf 'PASS: %s\n' "$name"
}

printf '\n===== VALID SYNTHETIC BASELINE =====\n'
run_case valid_baseline __SUCCESS__

printf '\n===== REQUIRED EVIDENCE =====\n'
for variable in "${REQUIRED[@]}"; do
    run_case \
        "missing_$variable" \
        "STOP: $variable required" \
        "$variable=__UNSET__"
done

printf '\n===== PLACEHOLDER EVIDENCE =====\n'
for placeholder in TODO TBD UNKNOWN PLACEHOLDER NONE N/A test dummy; do
    run_case \
        "placeholder_$placeholder" \
        'STOP: placeholder evidence: CONNECT_DEPLOYMENT_BACKUP_REFERENCE' \
        "CONNECT_DEPLOYMENT_BACKUP_REFERENCE=$placeholder"
done

printf '\n===== DATABASE IDENTITY MISMATCH =====\n'
run_case host_mismatch \
    'STOP: approved database host mismatch' \
    CONNECT_DEPLOYMENT_DATABASE_HOST=other.example.invalid

run_case port_mismatch \
    'STOP: approved database port mismatch' \
    CONNECT_DEPLOYMENT_DATABASE_PORT=5433

run_case name_mismatch \
    'STOP: approved database name mismatch' \
    CONNECT_DEPLOYMENT_DATABASE_NAME=another_production

printf '\n===== INVALID DATABASE PORTS =====\n'
for port in 0 65536 abc -1; do
    run_case \
        "invalid_port_$port" \
        'STOP: invalid database port' \
        "DB_PORT=$port" \
        "CONNECT_DEPLOYMENT_DATABASE_PORT=$port"
done

printf '\n===== UNSAFE DATABASE NAMES =====\n'
for name in \
    postgres template0 template1 \
    connect_test connect_test_extra test_connect
do
    run_case \
        "unsafe_database_$name" \
        'STOP: unsafe database name' \
        "DB_NAME=$name" \
        "CONNECT_DEPLOYMENT_DATABASE_NAME=$name"
done

printf '\n===== RELEASE COMMIT MISMATCH =====\n'
run_case release_commit_mismatch \
    'STOP: approved release commit mismatch' \
    CONNECT_DEPLOYMENT_RELEASE_COMMIT=bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb

printf '\n===== RESTORED VALID BASELINE =====\n'
run_case restored_baseline __SUCCESS__

test "$CASE_COUNT" -eq 34 || {
    echo "FAIL: unexpected test count: $CASE_COUNT"
    exit 1
}

printf '\n34 APPROVAL EVIDENCE REGRESSIONS PASSED\n'
