import asyncio
import json
import subprocess
import sys
from datetime import date
from pathlib import Path
from time import perf_counter

import pytest

from hcb.benchmarks import create_large_fixture, measure_large_fixture
from hcb.models import Account, Calendar, DateTimeKind, Event, EventDateTime, Task, TaskList
from hcb.paths import AppPaths
from hcb.runtime import Runtime
from hcb.storage import Storage
from hcb.tui import HcbApp, WorkspaceTable


def test_benchmark_report_measures_repeated_services_and_large_outbox() -> None:
    tool = Path(__file__).resolve().parents[1] / "tools/benchmark_python.py"
    completed = subprocess.run(
        [
            sys.executable,
            str(tool),
            "--tasks",
            "120",
            "--events",
            "10",
            "--runs",
            "2",
            "--outbox-writes",
            "105",
        ],
        capture_output=True,
        text=True,
        check=True,
    )
    report = json.loads(completed.stdout)

    assert report["fixture"]["tasks"] == 120
    assert report["fixture"]["outbox_writes_per_run"] == 105
    assert report["service_timings"].keys() == {
        "cli_startup",
        "search",
        "workspace",
        "outbox_flush",
    }
    for summary in report["service_timings"].values():
        assert summary["runs"] == 2
        assert len(summary["samples_seconds"]) == 2
        assert summary["max_seconds"] >= summary["median_seconds"] > 0


def test_outbox_benchmark_reports_completed_delivery_across_page_boundary() -> None:
    tool = Path(__file__).resolve().parents[1] / "tools/benchmark_outbox.py"
    completed = subprocess.run(
        [sys.executable, str(tool), "--counts", "101", "250", "--runs", "2"],
        capture_output=True,
        text=True,
        check=True,
    )
    report = json.loads(completed.stdout)

    assert [item["writes"] for item in report["results"]] == [101, 250]
    for item in report["results"]:
        assert item["runs"] == 2
        assert len(item["samples_seconds"]) == 2
        assert item["median_seconds"] > 0


def test_pull_benchmark_reports_initial_and_incremental_page_counts() -> None:
    tool = Path(__file__).resolve().parents[1] / "tools/benchmark_pull.py"
    completed = subprocess.run(
        [
            sys.executable,
            str(tool),
            "--tasks",
            "101",
            "--events",
            "11",
            "--task-lists",
            "1",
            "--calendars",
            "1",
            "--task-page-size",
            "100",
            "--event-page-size",
            "10",
            "--runs",
            "2",
        ],
        capture_output=True,
        text=True,
        check=True,
    )
    report = json.loads(completed.stdout)
    assert report["fixture"]["tasks"] == 101
    assert report["runs"] == 2
    for sample in report["samples"]:
        assert sample["rows"] == {"tasks": 101, "events": 11}
        assert sample["stages"]["initial"]["pages"] == {
            "task_lists": 1,
            "tasks": 2,
            "calendar_list": 1,
            "events": 2,
        }
        assert sample["stages"]["incremental"]["pages"] == {
            "task_lists": 1,
            "tasks": 1,
            "calendar_list": 1,
            "events": 1,
        }
    assert report["timings"]["initial"]["task_seconds"]["runs"] == 2


@pytest.mark.skipif(sys.platform == "win32", reason="the terminal benchmark requires a Unix PTY")
def test_tui_startup_benchmark_reaches_a_real_task_frame() -> None:
    tool = Path(__file__).resolve().parents[1] / "tools/benchmark_tui_startup.py"
    completed = subprocess.run(
        [sys.executable, str(tool), "--counts", "20", "--runs", "1"],
        capture_output=True,
        text=True,
        check=True,
    )
    result = json.loads(completed.stdout)["results"][0]

    assert result["tasks"] == 20 and result["events"] == 4
    for metric in ("import_seconds", "headless_ready_seconds", "terminal_first_task_seconds"):
        summary = result["timings"][metric]
        assert summary["runs"] == 1
        assert summary["median_seconds"] > 0
    assert result["memory"]["headless_peak_rss_bytes"]["median_bytes"] > 0


def test_deterministic_large_fixture_local_performance(tmp_path: Path) -> None:
    database = tmp_path / "large.db"
    create_large_fixture(database)
    result = measure_large_fixture(database)

    assert result.search_results == 11
    assert result.agenda_results > 100
    assert result.cached_tasks == 10_000
    assert result.cached_events == 2_000
    assert result.calendar_week_blocks > 1_000
    assert result.calendar_month_blocks > 8_000

    # These are regression tripwires, not microbenchmarks. The generous limits
    # tolerate shared CI runners while still detecting accidental quadratic work.
    assert result.cold_open_seconds < 5.0
    assert result.search_10k_seconds < 0.25
    assert result.agenda_seconds < 5.0
    assert result.tui_cache_load_seconds < 5.0
    assert result.calendar_week_layout_seconds < 2.0
    assert result.calendar_month_layout_seconds < 3.0


def test_virtual_workspace_switches_ten_thousand_rows_without_row_widgets(tmp_path: Path) -> None:
    """Keep the large-workspace path on the virtual-table scalability boundary."""

    paths = AppPaths(tmp_path / "config", tmp_path / "data", tmp_path / "cache")
    with Storage(paths.database_file) as storage, storage.transaction():
        storage.upsert_account(Account("work", "performance@example.test"))
        storage.upsert_task_list(TaskList("inbox", "work", "Inbox"))
        for index in range(10_000):
            storage.upsert_task(Task(f"task-{index}", "work", "inbox", f"Task {index:05d}"))

    async def scenario() -> tuple[float, float, int, int]:
        app = HcbApp(Runtime(paths, environ={}))
        async with app.run_test(size=(120, 40)) as pilot:
            await pilot.pause()
            content = app.query_one("#content", WorkspaceTable)
            mounted_before = len(tuple(content.walk_children()))
            started = perf_counter()
            app.action_surface("Agenda")
            await pilot.pause()
            to_agenda = perf_counter() - started
            started = perf_counter()
            app.action_surface("Tasks")
            await pilot.pause()
            to_tasks = perf_counter() - started
            return to_agenda, to_tasks, content.row_count, mounted_before

    to_agenda, to_tasks, row_count, mounted_before = asyncio.run(scenario())
    assert row_count == 10_000
    assert mounted_before < 20
    # Generous CI tripwires: local development should be materially faster.
    assert to_agenda < 1.0
    assert to_tasks < 1.0


def test_singleton_workspace_mutations_patch_ten_thousand_rows(tmp_path: Path) -> None:
    """Keep editor-save equivalents off the full workspace-refresh path."""

    paths = AppPaths(tmp_path / "config", tmp_path / "data", tmp_path / "cache")
    with Storage(paths.database_file) as storage, storage.transaction():
        storage.upsert_account(Account("work", "performance@example.test"))
        storage.upsert_task_list(TaskList("inbox", "work", "Inbox"))
        for index in range(10_000):
            storage.upsert_task(Task(f"task-{index}", "work", "inbox", f"Task {index:05d}"))

    async def scenario() -> tuple[float, float, float, int, int]:
        app = HcbApp(Runtime(paths, environ={}))
        async with app.run_test(size=(120, 40)) as pilot:
            await pilot.pause()
            content = app.query_one("#content", WorkspaceTable)
            content.move_cursor(row=5_000, animate=False)
            await pilot.pause()
            selected = app._selected_task()
            assert selected is not None

            updated = app.runtime.application.update_task(
                "work", selected.id, title="A updated performance task"
            )
            started = perf_counter()
            app.apply_workspace_task_mutation(updated)
            await pilot.pause()
            update_seconds = perf_counter() - started
            assert content.index_of("task", updated.id) is not None

            created = app.runtime.application.create_task(
                "work", "inbox", "A created performance task"
            )
            started = perf_counter()
            app.apply_workspace_task_mutation(created)
            await pilot.pause()
            create_seconds = perf_counter() - started
            assert content.index_of("task", created.id) is not None

            completed = app.runtime.application.complete_task("work", created.id)
            started = perf_counter()
            app.apply_workspace_task_mutation(completed)
            await pilot.pause()
            complete_seconds = perf_counter() - started
            return (
                update_seconds,
                create_seconds,
                complete_seconds,
                content.row_count,
                len(tuple(content.walk_children())),
            )

    update_seconds, create_seconds, complete_seconds, row_count, mounted = asyncio.run(scenario())
    assert row_count == 10_001
    assert mounted < 20
    # Regression tripwires, intentionally looser than an interactive local run.
    assert update_seconds < 0.8
    assert create_seconds < 0.8
    assert complete_seconds < 0.8


def test_singleton_event_workspace_mutations_patch_two_thousand_rows(tmp_path: Path) -> None:
    """Keep event editor saves and creates on the same incremental path."""

    paths = AppPaths(tmp_path / "config", tmp_path / "data", tmp_path / "cache")
    start = EventDateTime(DateTimeKind.DATE, date(2026, 8, 30))
    end = EventDateTime(DateTimeKind.DATE, date(2026, 8, 31))
    with Storage(paths.database_file) as storage, storage.transaction():
        storage.upsert_account(Account("work", "performance@example.test"))
        storage.upsert_calendar(Calendar("calendar", "work", "Calendar"))
        for index in range(2_000):
            storage.upsert_event(
                Event(f"event-{index}", "work", "calendar", f"Event {index:05d}", start, end)
            )

    async def scenario() -> tuple[float, float, int]:
        app = HcbApp(Runtime(paths, environ={}), selected_date=date(2026, 8, 30))
        async with app.run_test(size=(120, 40)) as pilot:
            await pilot.pause()
            app.action_surface("Agenda")
            await pilot.pause()
            content = app.query_one("#content", WorkspaceTable)
            event = app.cache.events[1_000]

            updated = app.runtime.application.update_event(
                "work", event.id, summary="Updated performance event"
            )
            started = perf_counter()
            app.apply_workspace_event_mutation(updated)
            await pilot.pause()
            update_seconds = perf_counter() - started
            assert content.index_of("event", updated.id) is not None

            created = app.runtime.application.create_event(
                "work", "calendar", "Created performance event", start, end
            )
            started = perf_counter()
            app.apply_workspace_event_mutation(created)
            await pilot.pause()
            create_seconds = perf_counter() - started
            assert content.index_of("event", created.id) is not None
            return update_seconds, create_seconds, content.row_count

    update_seconds, create_seconds, row_count = asyncio.run(scenario())
    assert row_count == 2_001
    assert update_seconds < 0.8
    assert create_seconds < 0.8
