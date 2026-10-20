#!/usr/bin/env python3
"""CONNECT isolated PostgreSQL backend regression runner."""

import os
import subprocess
import sys
from concurrent.futures import ThreadPoolExecutor
from datetime import datetime
from pathlib import Path


BACKEND = Path(__file__).resolve().parent.parent

DISPATCH_PACKAGE = (
    "github.com/JCKFinland/connect/backend/"
    "internal/services/dispatch"
)

DATABASES = {
    "dispatch": "connect_dispatch_test",
    "remaining_backend": "connect_integration_test",
}


def run_command(command, **kwargs):
    return subprocess.run(
        command,
        cwd=BACKEND,
        **kwargs,
    )


def controlled_database_environment(database_name):
    environment = os.environ.copy()

    for key in (
        "PGHOST",
        "PGHOSTADDR",
        "PGPORT",
        "PGUSER",
        "PGDATABASE",
        "PGSERVICE",
        "PGSERVICEFILE",
        "PGOPTIONS",
    ):
        environment.pop(key, None)

    environment["DB_HOST"] = "localhost"
    environment["DB_PORT"] = "5432"
    environment["DB_USER"] = "postgres"
    environment["DB_NAME"] = database_name
    environment["FLEX_SETTLEMENT_WORKER_ENABLED"] = "false"
    environment["CONNECT_REQUIRE_SETTLEMENT_TEST_DB"] = "0"

    return environment


def verify_database(name):
    environment = controlled_database_environment(name)

    result = run_command(
        [
            "psql",
            "-X",
            "-U", "postgres",
            "-h", "localhost",
            "-p", "5432",
            "-d", name,
            "-At",
            "-v", "ON_ERROR_STOP=1",
            "-c", "SELECT current_database();",
        ],
        capture_output=True,
        text=True,
        env=environment,
    )

    if result.returncode != 0:
        raise RuntimeError(
            f"Database verification failed for {name}: "
            f"{result.stderr.strip()}"
        )

    if result.stdout.strip() != name:
        raise RuntimeError(
            f"Unexpected database identity for {name}"
        )


def discover_packages():
    result = run_command(
        ["go", "list", "./..."],
        capture_output=True,
        text=True,
    )

    if result.returncode != 0:
        raise RuntimeError(
            f"Package discovery failed: {result.stderr.strip()}"
        )

    packages = result.stdout.splitlines()

    if packages.count(DISPATCH_PACKAGE) != 1:
        raise RuntimeError(
            "Dispatch package missing or duplicated."
        )

    remaining = [
        package
        for package in packages
        if package != DISPATCH_PACKAGE
    ]

    if not remaining:
        raise RuntimeError("No remaining backend packages.")

    return remaining


def run_test_group(name, packages, log_directory):
    environment = controlled_database_environment(DATABASES[name])

    log_path = log_directory / f"{name}.log"

    command = [
        "go",
        "test",
        *packages,
        "-p", "1",
        "-parallel", "1",
        "-count=1",
    ]

    with log_path.open("w", encoding="utf-8") as log:
        result = run_command(
            command,
            env=environment,
            stdout=log,
            stderr=subprocess.STDOUT,
        )

    return name, result.returncode, log_path


def print_result(name, status, log_path):
    print(f"\n===== RESULT: {name} =====", flush=True)
    print(f"Exit status: {status}")
    print(f"Log: {log_path}")

    lines = log_path.read_text(
        encoding="utf-8",
        errors="replace",
    ).splitlines()

    if status == 0:
        for line in lines[-35:]:
            print(line)
        return

    for index, line in enumerate(lines):
        if line.startswith("--- FAIL:") or line.startswith("FAIL"):
            for detail in lines[index:index + 7]:
                print(detail)

    print("\nLast 35 log lines:")
    for line in lines[-35:]:
        print(line)


def main():
    print(
        "\n===== CONNECT ISOLATED BACKEND REGRESSION =====",
        flush=True,
    )

    print("\n===== 1. VERIFY DATABASES =====", flush=True)

    for database in DATABASES.values():
        verify_database(database)
        print(f"Verified: {database}", flush=True)

    print("\n===== 2. DISCOVER PACKAGES =====", flush=True)

    remaining = discover_packages()

    print("Dispatch packages: 1")
    print(f"Remaining packages: {len(remaining)}")

    timestamp = datetime.now().strftime("%Y%m%d_%H%M%S")

    log_directory = (
        Path(os.environ.get("TEMP", "/tmp"))
        / "connect-regression"
        / timestamp
    )

    log_directory.mkdir(parents=True, exist_ok=True)

    print("\n===== 3. RUN TEST GROUPS =====", flush=True)

    jobs = [
        ("dispatch", [DISPATCH_PACKAGE], log_directory),
        ("remaining_backend", remaining, log_directory),
    ]

    with ThreadPoolExecutor(max_workers=2) as executor:
        futures = [
            executor.submit(run_test_group, *job)
            for job in jobs
        ]

        results = [
            future.result()
            for future in futures
        ]

    tests_passed = True

    for name, status, log_path in results:
        print_result(name, status, log_path)

        if status != 0:
            tests_passed = False

    print("\n===== 4. GO VET =====", flush=True)

    vet = run_command(["go", "vet", "./..."])

    print("Vet exit status:", vet.returncode)

    print("\n===== 5. DIFF INTEGRITY =====", flush=True)

    integrity = run_command(
        [
            "git",
            "-c", "core.safecrlf=false",
            "diff",
            "--check",
        ]
    )

    print("Diff check:", integrity.returncode)

    print("\n===== 6. STAGED FILES =====", flush=True)

    staged = run_command(
        ["git", "diff", "--cached", "--name-only"],
        capture_output=True,
        text=True,
    )

    staged_files = staged.stdout.strip()

    if staged_files:
        print(staged_files)
        print("WARNING: staged files detected.")
    elif staged.returncode == 0:
        print("Staging area: empty")
    else:
        print(staged.stderr.strip())

    print("\n===== FINAL RESULT =====", flush=True)

    success = (
        tests_passed
        and vet.returncode == 0
        and integrity.returncode == 0
        and staged.returncode == 0
        and not staged_files
    )

    print("PASS" if success else "FAIL")
    print("Logs:", log_directory)

    return 0 if success else 1


if __name__ == "__main__":
    try:
        sys.exit(main())
    except (RuntimeError, OSError) as error:
        print(f"\nSTOP: {error}", file=sys.stderr)
        sys.exit(1)
