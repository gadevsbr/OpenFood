"""Restore a trusted dump, reconciling removal requests before atomic promotion.
Usage: python3 scripts/restore-compose.py backup.dump pre-restore.dump
Run from repository root. Requires Python 3, Compose and the current app image.
"""
import os
import secrets
import subprocess
import sys
import tempfile
from pathlib import Path


def compose(*args, **kwargs):
    return subprocess.run(["docker", "compose", *args], check=True, **kwargs)


def restore(source, safety):
    if source.resolve() == safety.resolve() or safety.exists():
        raise ValueError("Use a new, separate pre-restore backup path")
    if source.open("rb").read(5) != b"PGDMP":
        raise ValueError("Invalid dump format")
    stage = "openfood_validate_" + secrets.token_hex(6)
    compose("stop", "app")
    created = False
    try:
        ledger = compose("run", "--rm", "--no-deps", "-T", "app", "--privacy-export", stdout=subprocess.PIPE).stdout
        # Store the recovery ledger next to the safety backup for transfer/recovery.
        ledger_path = Path(str(safety) + ".privacy.json")
        with os.fdopen(os.open(ledger_path, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600), "wb") as f:
            f.write(ledger)
        with os.fdopen(os.open(safety, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600), "wb") as f:
            compose("exec", "-T", "db", "pg_dump", "-U", "openfood", "-d", "openfood", "-Fc", stdout=f)
        compose("exec", "-T", "db", "psql", "-v", "ON_ERROR_STOP=1", "-U", "openfood", "-d", "postgres", "-c", "CREATE DATABASE " + stage)
        created = True
        with source.open("rb") as f:
            compose("exec", "-T", "db", "pg_restore", "-U", "openfood", "-d", stage, "--exit-on-error", "--no-owner", "--no-acl", stdin=f)
        compose("run", "--rm", "--no-deps", "-T", "app", "--privacy-reapply", stage, input=ledger)
        with tempfile.TemporaryFile() as f:
            compose("exec", "-T", "db", "pg_dump", "-U", "openfood", "-d", stage, "-Fc", stdout=f)
            f.seek(0)
            compose("exec", "-T", "db", "pg_restore", "-U", "openfood", "-d", "openfood", "--clean", "--if-exists", "--single-transaction", "--exit-on-error", "--no-owner", "--no-acl", stdin=f)
    finally:
        try:
            if created:
                compose("exec", "-T", "db", "psql", "-v", "ON_ERROR_STOP=1", "-U", "openfood", "-d", "postgres", "-c", "DROP DATABASE " + stage + " WITH (FORCE)")
        finally:
            compose("start", "app")


if __name__ == "__main__":
    if len(sys.argv) != 3:
        sys.exit(__doc__)
    restore(Path(sys.argv[1]), Path(sys.argv[2]))
