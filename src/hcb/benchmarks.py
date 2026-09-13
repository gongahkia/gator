"""Deterministic release-scale fixture and local performance measurements."""

from __future__ import annotations

import os
import subprocess
import sys
from collections.abc import Callable
from dataclasses import asdict, dataclass
from datetime import UTC, date, datetime, timedelta
from pathlib import Path
from statistics import median
from time import perf_counter
from typing import cast
from zoneinfo import ZoneInfo

from .application import ApplicationService
from .google_client import GoogleGateway, Json
from .models import (
    Account,
    Calendar,
    DateTimeKind,
    EntityType,
    Event,
    EventDateTime,
    MutationOperation,
    PendingMutation,
    Task,
    TaskList,
)
from .storage import Storage
from .sync import SyncEngine
from .tui_calendar import all_day_blocks, calendar_items, calendar_range, timed_blocks


@dataclass(frozen=True, slots=True)
class BenchmarkResult:
    cold_open_seconds: float
    search_10k_seconds: float
    agenda_seconds: float
    tui_cache_load_seconds: float
    search_results: int
    agenda_results: int
    cached_tasks: int
    cached_events: int
    calendar_week_layout_seconds: float
    calendar_month_layout_seconds: float
    calendar_week_blocks: int
    calendar_month_blocks: int

    def as_dict(self) -> dict[str, float | int]:
        return asdict(self)


@dataclass(frozen=True, slots=True)
class TimingSummary:
    runs: int
    median_seconds: float
    p95_seconds: float
    min_seconds: float
    max_seconds: float
    samples_seconds: tuple[float, ...]

    @classmethod
    def from_samples(cls, samples: list[float]) -> TimingSummary:
        if not samples:
            raise ValueError("at least one timing sample is required")
        ordered = sorted(samples)
        # nearest-rank p95 keeps the observed tail instead of interpolating it.
        p95_index = (95 * len(ordered) + 99) // 100 - 1
        return cls(
            len(samples),
            median(samples),
            ordered[p95_index],
            ordered[0],
            ordered[-1],
            tuple(samples),
        )


class _BenchmarkGateway:
    """Acknowledge local writes without network latency or credentials."""

    def update_task(
        self, task_list_id: str, task_id: str, body: Json, *, etag: str | None = None
    ) -> Json:
        return {"id": task_id}


def measure_service_baseline(
    path: Path, *, runs: int = 20, outbox_writes: int = 250
) -> dict[str, TimingSummary]:
    """Time real application and delivery paths against a deterministic fixture."""
    if runs < 2 or outbox_writes < 1:
        raise ValueError("runs must be at least two and outbox_writes must be positive")
    with Storage(path) as storage:
        task_count = storage.connection.execute(
            "SELECT COUNT(*) FROM tasks WHERE account_id=?", ("benchmark",)
        ).fetchone()[0]
        if outbox_writes > task_count:
            raise ValueError("outbox_writes exceeds fixture task count")
        application = ApplicationService(storage)
        engine = SyncEngine(storage, cast(GoogleGateway, _BenchmarkGateway()))
        samples: dict[str, list[float]] = {
            "cli_startup": [],
            "search": [],
            "workspace": [],
            "outbox_flush": [],
        }

        def timed(operation: Callable[[], object]) -> float:
            started = perf_counter()
            operation()
            return perf_counter() - started

        def cli_help() -> None:
            environment = {**os.environ, "NO_COLOR": "1", "TERM": "dumb"}
            completed = subprocess.run(
                [sys.executable, "-c", "from hcb.cli import main; main()", "--help"],
                stdout=subprocess.DEVNULL,
                stderr=subprocess.PIPE,
                text=True,
                env=environment,
                check=False,
            )
            if completed.returncode:
                raise RuntimeError(f"CLI startup failed: {completed.stderr.strip()}")

        for _ in range(runs):
            samples["cli_startup"].append(timed(cli_help))
            samples["search"].append(
                timed(lambda: application.search("benchmark", "release-marker", limit=50))
            )
            samples["workspace"].append(timed(lambda: application.workspace("benchmark")))

            with storage.transaction():
                for index in range(outbox_writes):
                    task_id = f"task-{index:05d}"
                    storage.enqueue(
                        PendingMutation(
                            None,
                            "benchmark",
                            EntityType.TASK,
                            task_id,
                            MutationOperation.UPDATE,
                            {"list_id": "inbox", "body": {"title": f"Updated task {index:05d}"}},
                        )
                    )
            started = perf_counter()
            result = engine.flush_outbox("benchmark")
            samples["outbox_flush"].append(perf_counter() - started)
            if result.pushed != outbox_writes or storage.pending_mutation_count("benchmark"):
                raise RuntimeError("benchmark outbox was not fully delivered")
        return {name: TimingSummary.from_samples(values) for name, values in samples.items()}


def create_large_fixture(
    path: Path,
    *,
    task_count: int = 10_000,
    event_count: int = 2_000,
) -> None:
    """Create a stable, realistic local cache without network or random input."""
    with Storage(path) as storage, storage.transaction():
        storage.upsert_account(Account("benchmark", "redacted@example.test"))
        storage.upsert_task_list(TaskList("inbox", "benchmark", "Inbox", remote_id="inbox-r"))
        storage.upsert_calendar(Calendar("primary", "benchmark", "Primary"))
        for index in range(task_count):
            marker = " release-marker" if index % 997 == 0 else ""
            storage.upsert_task(
                Task(
                    f"task-{index:05d}",
                    "benchmark",
                    "inbox",
                    f"Deterministic task {index:05d}{marker}",
                    notes=f"Local fixture note {index:05d}",
                    remote_id=f"remote-task-{index:05d}",
                )
            )
        origin = datetime(2026, 1, 1, 9, tzinfo=UTC)
        for index in range(event_count):
            start = origin + timedelta(hours=6 * index)
            storage.upsert_event(
                Event(
                    f"event-{index:05d}",
                    "benchmark",
                    "primary",
                    f"Fixture event {index:05d}",
                    EventDateTime(DateTimeKind.DATETIME, start, "UTC"),
                    EventDateTime(DateTimeKind.DATETIME, start + timedelta(minutes=45), "UTC"),
                )
            )


def measure_large_fixture(path: Path) -> BenchmarkResult:
    started = perf_counter()
    storage = Storage(path)
    cold_open = perf_counter() - started
    try:
        application = ApplicationService(storage)

        started = perf_counter()
        search = application.search("benchmark", "release-marker", limit=50)
        search_elapsed = perf_counter() - started

        started = perf_counter()
        agenda = storage.list_events(
            "benchmark",
            start=date(2026, 3, 1),
            end=date(2026, 4, 1),
        )
        agenda_elapsed = perf_counter() - started

        started = perf_counter()
        snapshot = application.workspace("benchmark")
        cache_elapsed = perf_counter() - started
        calendar = measure_calendar_layout()
        return BenchmarkResult(
            cold_open,
            search_elapsed,
            agenda_elapsed,
            cache_elapsed,
            len(search),
            len(agenda),
            len(snapshot.tasks),
            len(snapshot.events),
            calendar.week_seconds,
            calendar.month_seconds,
            calendar.week_blocks,
            calendar.month_blocks,
        )
    finally:
        storage.close()


@dataclass(frozen=True, slots=True)
class CalendarLayoutResult:
    """Pure calendar-layout work measured without terminal repaint variance."""

    week_seconds: float
    month_seconds: float
    week_blocks: int
    month_blocks: int


def measure_calendar_layout(
    *, event_count: int = 5_000, task_count: int = 5_000
) -> CalendarLayoutResult:
    """Measure Day/Week/Month geometry with 10,000 calendar-visible records.

    The fixture spreads due tasks and timed events over a complete six-week
    Month while retaining overlapping timed events. It exercises normalisation,
    all-day lane allocation, and timed collision packing without depending on
    a terminal emulator's paint speed.
    """
    origin = date(2026, 8, 3)
    zone = ZoneInfo("UTC")
    events = tuple(
        Event(
            f"calendar-event-{index}",
            "benchmark",
            "primary",
            f"Calendar fixture event {index}",
            EventDateTime(
                DateTimeKind.DATETIME,
                datetime.combine(origin + timedelta(days=index % 42), datetime.min.time(), UTC)
                + timedelta(minutes=(index * 30) % (24 * 60)),
                "UTC",
            ),
            EventDateTime(
                DateTimeKind.DATETIME,
                datetime.combine(origin + timedelta(days=index % 42), datetime.min.time(), UTC)
                + timedelta(minutes=(index * 30) % (24 * 60) + 45),
                "UTC",
            ),
        )
        for index in range(event_count)
    )
    tasks = tuple(
        Task(
            f"calendar-task-{index}",
            "benchmark",
            "inbox",
            f"Calendar fixture task {index}",
            due=origin + timedelta(days=index % 42),
        )
        for index in range(task_count)
    )

    def layout(surface: str) -> tuple[int, int]:
        visible = calendar_range(surface, date(2026, 8, 24), 0)  # type: ignore[arg-type]
        items = calendar_items(
            events,
            tasks,
            visible=visible,
            zone=zone,
            calendar_colors={"primary": "#4285f4"},
            fallback_color="#fbbc04",
        )
        return len(all_day_blocks(items, visible)), len(timed_blocks(items, visible))

    started = perf_counter()
    week_all_day, week_timed = layout("Week")
    week_seconds = perf_counter() - started
    started = perf_counter()
    month_all_day, month_timed = layout("Month")
    month_seconds = perf_counter() - started
    return CalendarLayoutResult(
        week_seconds,
        month_seconds,
        week_all_day + week_timed,
        month_all_day + month_timed,
    )
