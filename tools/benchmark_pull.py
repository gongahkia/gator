#!/usr/bin/env python3
"""Profile credential-free, paginated Tasks and Calendar pull sync."""

from __future__ import annotations

import argparse
import json
import platform
import subprocess
import sys
import tempfile
from collections import Counter
from contextlib import contextmanager
from dataclasses import asdict
from datetime import UTC, datetime
from pathlib import Path
from statistics import median
from time import perf_counter
from typing import cast

from hcb.benchmarks import TimingSummary
from hcb.google_client import GoogleGateway, Page
from hcb.models import Account
from hcb.storage import Storage
from hcb.sync import SyncEngine


class TimedStorage(Storage):
    def __init__(self, path: Path) -> None:
        self.measure_transactions = False
        self.transaction_seconds = 0.0
        super().__init__(path)

    @contextmanager
    def transaction(self):
        started = perf_counter()
        with super().transaction() as connection:
            yield connection
        if self.measure_transactions:
            self.transaction_seconds += perf_counter() - started


class SyntheticPullGateway:
    def __init__(
        self,
        *,
        tasks: int,
        events: int,
        task_lists: int,
        calendars: int,
        task_page_size: int,
        event_page_size: int,
    ) -> None:
        self.tasks = tasks
        self.events = events
        self.task_lists = task_lists
        self.calendars = calendars
        self.task_page_size = task_page_size
        self.event_page_size = event_page_size
        self.calls: Counter[str] = Counter()
        self.generation_seconds = 0.0

    @staticmethod
    def _bounds(total: int, buckets: int, index: int) -> tuple[int, int]:
        return total * index // buckets, total * (index + 1) // buckets

    def list_task_lists(self, *, page_token: str | None = None) -> Page:
        assert page_token is None
        self.calls["task_lists"] += 1
        return Page(
            tuple(
                {"id": f"list-{index}", "title": f"Task list {index}"}
                for index in range(self.task_lists)
            )
        )

    def list_tasks(
        self,
        task_list_id: str,
        *,
        page_token: str | None = None,
        updated_min: str | None = None,
    ) -> Page:
        self.calls["tasks"] += 1
        if updated_min is not None:
            return Page(())
        started = perf_counter()
        index = int(task_list_id.removeprefix("list-"))
        first, end = self._bounds(self.tasks, self.task_lists, index)
        offset = int(page_token or 0)
        last = min(first + offset + self.task_page_size, end)
        items = tuple(
            {
                "id": f"task-{number:05d}",
                "title": f"Synthetic task {number:05d}",
                "notes": f"Synthetic note {number:05d}",
                "updated": "2026-09-13T00:00:00Z",
            }
            for number in range(first + offset, last)
        )
        self.generation_seconds += perf_counter() - started
        next_token = str(last - first) if last < end else None
        return Page(items, next_page_token=next_token)

    def list_calendars(
        self, *, page_token: str | None = None, sync_token: str | None = None
    ) -> Page:
        assert page_token is None
        self.calls["calendar_list"] += 1
        if sync_token is not None:
            return Page((), next_sync_token="calendar-list-sync-2")
        return Page(
            tuple(
                {"id": f"calendar-{index}", "summary": f"Calendar {index}"}
                for index in range(self.calendars)
            ),
            next_sync_token="calendar-list-sync-1",
        )

    def list_events(
        self,
        calendar_id: str,
        *,
        page_token: str | None = None,
        sync_token: str | None = None,
        time_min: str | None = None,
        time_max: str | None = None,
        single_events: bool = False,
    ) -> Page:
        assert time_min is None and time_max is None and not single_events
        self.calls["events"] += 1
        if sync_token is not None:
            return Page((), next_sync_token="events-sync-2")
        started = perf_counter()
        index = int(calendar_id.removeprefix("calendar-"))
        first, end = self._bounds(self.events, self.calendars, index)
        offset = int(page_token or 0)
        last = min(first + offset + self.event_page_size, end)
        items = tuple(
            {
                "id": f"event-{number:05d}",
                "summary": f"Synthetic event {number:05d}",
                "description": f"Synthetic description {number:05d}",
                "start": {"dateTime": "2026-09-13T09:00:00Z"},
                "end": {"dateTime": "2026-09-13T10:00:00Z"},
                "updated": "2026-09-13T00:00:00Z",
            }
            for number in range(first + offset, last)
        )
        self.generation_seconds += perf_counter() - started
        next_token = str(last - first) if last < end else None
        return Page(
            items,
            next_page_token=next_token,
            next_sync_token="events-sync-1" if next_token is None else None,
        )


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


def measure_worker(args: argparse.Namespace) -> dict[str, object]:
    with tempfile.TemporaryDirectory(prefix="hcb-pull-benchmark-") as folder:
        database = Path(folder) / "fixture.db"
        gateway = SyntheticPullGateway(
            tasks=args.tasks,
            events=args.events,
            task_lists=args.task_lists,
            calendars=args.calendars,
            task_page_size=args.task_page_size,
            event_page_size=args.event_page_size,
        )
        with TimedStorage(database) as storage:
            storage.upsert_account(Account("benchmark", "synthetic@example.test"))
            engine = SyncEngine(
                storage,
                cast(GoogleGateway, gateway),
                now=lambda: datetime(2026, 9, 13, 12, tzinfo=UTC),
            )
            stages: dict[str, dict[str, object]] = {}
            with engine.sync_ownership():
                for name in ("initial", "incremental"):
                    gateway.calls.clear()
                    gateway.generation_seconds = 0.0
                    storage.transaction_seconds = 0.0
                    storage.measure_transactions = True
                    started = perf_counter()
                    task_result = engine.sync_task_lists("benchmark")
                    task_seconds = perf_counter() - started
                    started = perf_counter()
                    calendar_result = engine.sync_calendars("benchmark")
                    calendar_seconds = perf_counter() - started
                    storage.measure_transactions = False
                    stages[name] = {
                        "task_seconds": task_seconds,
                        "calendar_seconds": calendar_seconds,
                        "page_apply_seconds": storage.transaction_seconds,
                        "fake_page_generation_seconds": gateway.generation_seconds,
                        "pages": dict(gateway.calls),
                        "pulled": task_result.pulled + calendar_result.pulled,
                    }
            task_rows = storage.connection.execute("SELECT COUNT(*) FROM tasks").fetchone()[0]
            event_rows = storage.connection.execute("SELECT COUNT(*) FROM events").fetchone()[0]
            event_tokens = [
                storage.get_cursor("benchmark", f"events:calendar-{index}").cursor
                for index in range(args.calendars)
            ]
            if task_rows != args.tasks or event_rows != args.events:
                raise RuntimeError("synthetic pull did not persist every canonical row")
            if any(token != "events-sync-2" for token in event_tokens):
                raise RuntimeError("incremental pull did not persist final Calendar sync tokens")
            if stages["initial"]["pulled"] != (
                args.tasks + args.events + args.task_lists + args.calendars
            ):
                raise RuntimeError("initial pull returned an unexpected item count")
            if stages["incremental"]["pulled"] != args.task_lists:
                raise RuntimeError("unchanged incremental pull returned unexpected items")
        return {
            "stages": stages,
            "rows": {"tasks": task_rows, "events": event_rows},
            "database_bytes": database.stat().st_size,
            "peak_rss_bytes": _peak_rss_bytes(),
        }


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--tasks", type=int, default=10_000)
    parser.add_argument("--events", type=int, default=2_000)
    parser.add_argument("--task-lists", type=int, default=5)
    parser.add_argument("--calendars", type=int, default=2)
    parser.add_argument("--task-page-size", type=int, default=100)
    parser.add_argument("--event-page-size", type=int, default=250)
    parser.add_argument("--runs", type=int, default=5)
    parser.add_argument("--worker", action="store_true", help=argparse.SUPPRESS)
    args = parser.parse_args()
    if (
        args.tasks < 1
        or args.events < 0
        or args.task_lists < 1
        or args.calendars < 1
        or not 1 <= args.task_page_size <= 100
        or args.event_page_size < 1
        or args.runs < 1
    ):
        parser.error("counts must be positive, events nonnegative, task pages 1–100")
    if args.worker:
        print(json.dumps(measure_worker(args), sort_keys=True))
        return 0

    samples: list[dict[str, object]] = []
    for _ in range(args.runs):
        command = [sys.executable, str(Path(__file__).resolve()), "--worker"]
        for option in (
            "tasks",
            "events",
            "task_lists",
            "calendars",
            "task_page_size",
            "event_page_size",
        ):
            command.extend((f"--{option.replace('_', '-')}", str(getattr(args, option))))
        completed = subprocess.run(command, capture_output=True, text=True, check=False)
        if completed.returncode:
            raise RuntimeError(f"pull worker failed: {completed.stderr.strip()}")
        samples.append(json.loads(completed.stdout))

    timings = {
        stage: {
            metric: asdict(
                TimingSummary.from_samples(
                    [float(sample["stages"][stage][metric]) for sample in samples]
                )
            )
            for metric in (
                "task_seconds",
                "calendar_seconds",
                "page_apply_seconds",
                "fake_page_generation_seconds",
            )
        }
        for stage in ("initial", "incremental")
    }
    memory_samples = [sample["peak_rss_bytes"] for sample in samples]
    measured_memory = [value for value in memory_samples if value is not None]
    print(
        json.dumps(
            {
                "environment": {
                    "os": platform.system(),
                    "release": platform.release(),
                    "machine": platform.machine(),
                    "python": platform.python_version(),
                },
                "fixture": {
                    "tasks": args.tasks,
                    "events": args.events,
                    "task_lists": args.task_lists,
                    "calendars": args.calendars,
                    "task_page_size": args.task_page_size,
                    "event_page_size": args.event_page_size,
                },
                "runs": args.runs,
                "timings": timings,
                "samples": samples,
                "peak_rss_bytes": {
                    "median": median(measured_memory) if measured_memory else None,
                    "samples": memory_samples,
                },
            },
            indent=2,
            sort_keys=True,
        )
    )
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
