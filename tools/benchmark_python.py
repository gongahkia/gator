#!/usr/bin/env python3
"""Generate a deterministic local Python-product performance report."""

from __future__ import annotations

import argparse
import json
import platform
import subprocess
import sys
import tempfile
from dataclasses import asdict
from pathlib import Path

from hcb.benchmarks import create_large_fixture, measure_large_fixture, measure_service_baseline


def _peak_rss_bytes() -> int | None:
    try:
        import resource
    except ImportError:
        return None
    peak = resource.getrusage(resource.RUSAGE_SELF).ru_maxrss
    if sys.platform == "darwin":
        return int(peak)
    if sys.platform.startswith("linux"):
        return int(peak) * 1024
    return None


def _measure_existing(database: Path, *, runs: int, outbox_writes: int) -> dict[str, object]:
    original = measure_large_fixture(database)
    services = measure_service_baseline(database, runs=runs, outbox_writes=outbox_writes)
    return {
        **original.as_dict(),
        "fixture": {
            "tasks": original.cached_tasks,
            "events": original.cached_events,
            "outbox_writes_per_run": outbox_writes,
            "database_bytes": database.stat().st_size,
        },
        "environment": {
            "os": platform.system(),
            "release": platform.release(),
            "machine": platform.machine(),
            "python": platform.python_version(),
        },
        "service_timings": {name: asdict(summary) for name, summary in services.items()},
        "process_peak_rss_bytes": _peak_rss_bytes(),
    }


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--database", type=Path)
    parser.add_argument("--keep-database", action="store_true")
    parser.add_argument("--tasks", type=int, default=10_000)
    parser.add_argument("--events", type=int, default=2_000)
    parser.add_argument("--runs", type=int, default=20)
    parser.add_argument("--outbox-writes", type=int, default=250)
    parser.add_argument("--measure-existing", action="store_true", help=argparse.SUPPRESS)
    args = parser.parse_args()

    if args.tasks < 1 or args.events < 0 or args.runs < 2 or args.outbox_writes < 1:
        parser.error(
            "tasks and outbox-writes must be positive, events nonnegative, runs at least two"
        )
    if args.outbox_writes > args.tasks:
        parser.error("--outbox-writes cannot exceed --tasks")
    if args.keep_database and args.database is None:
        parser.error("--keep-database requires --database")
    if args.measure_existing:
        if args.database is None or not args.database.is_file():
            parser.error("--measure-existing requires an existing --database")
        print(
            json.dumps(
                _measure_existing(args.database, runs=args.runs, outbox_writes=args.outbox_writes),
                indent=2,
                sort_keys=True,
            )
        )
        return 0

    temporary: tempfile.TemporaryDirectory[str] | None = None
    if args.database is None:
        temporary = tempfile.TemporaryDirectory(prefix="hcb-python-benchmark-")
        database = Path(temporary.name) / "fixture.db"
    else:
        database = args.database.expanduser().resolve()
        database.parent.mkdir(parents=True, exist_ok=True)

    if database.exists():
        parser.error(f"database already exists: {database}")
    try:
        create_large_fixture(database, task_count=args.tasks, event_count=args.events)
        completed = subprocess.run(
            [
                sys.executable,
                str(Path(__file__).resolve()),
                "--measure-existing",
                "--database",
                str(database),
                "--tasks",
                str(args.tasks),
                "--events",
                str(args.events),
                "--runs",
                str(args.runs),
                "--outbox-writes",
                str(args.outbox_writes),
            ],
            capture_output=True,
            text=True,
            check=False,
        )
        if completed.returncode:
            print(completed.stderr, file=sys.stderr, end="")
            return completed.returncode
        print(completed.stdout, end="")
        return 0
    finally:
        if temporary is not None:
            temporary.cleanup()


if __name__ == "__main__":
    raise SystemExit(main())
