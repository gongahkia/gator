#!/usr/bin/env python3
"""Measure local outbox delivery at selected queue sizes without Google requests."""

from __future__ import annotations

import argparse
import json
import platform
import tempfile
from dataclasses import asdict
from pathlib import Path
from time import perf_counter
from typing import cast

from hcb.benchmarks import TimingSummary, _BenchmarkGateway, create_large_fixture
from hcb.google_client import GoogleGateway
from hcb.models import EntityType, MutationOperation, PendingMutation
from hcb.storage import Storage
from hcb.sync import SyncEngine


def measure(count: int) -> float:
    with tempfile.TemporaryDirectory(prefix="hcb-outbox-benchmark-") as folder:
        database = Path(folder) / "fixture.db"
        create_large_fixture(database, task_count=count, event_count=0)
        with Storage(database) as storage:
            with storage.transaction():
                for index in range(count):
                    storage.enqueue(
                        PendingMutation(
                            None,
                            "benchmark",
                            EntityType.TASK,
                            f"task-{index:05d}",
                            MutationOperation.UPDATE,
                            {"list_id": "inbox", "body": {"title": f"Updated task {index:05d}"}},
                        )
                    )
            started = perf_counter()
            result = SyncEngine(storage, cast(GoogleGateway, _BenchmarkGateway())).flush_outbox(
                "benchmark"
            )
            elapsed = perf_counter() - started
            if result.pushed != count or storage.pending_mutation_count("benchmark"):
                raise RuntimeError(f"outbox delivery was incomplete for {count} writes")
            return elapsed


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--counts", nargs="+", type=int, default=[250, 1000, 5000])
    parser.add_argument("--runs", type=int, default=3)
    args = parser.parse_args()
    if args.runs < 1 or any(count < 1 for count in args.counts):
        parser.error("runs and every count must be positive")

    results = []
    for count in args.counts:
        samples = [measure(count) for _ in range(args.runs)]
        results.append({"writes": count, **asdict(TimingSummary.from_samples(samples))})
    print(
        json.dumps(
            {
                "environment": {
                    "os": platform.system(),
                    "release": platform.release(),
                    "machine": platform.machine(),
                    "python": platform.python_version(),
                },
                "results": results,
            },
            indent=2,
            sort_keys=True,
        )
    )
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
