#!/usr/bin/env python3
"""Measure synthetic TUI startup in headless Textual and a real terminal session."""

from __future__ import annotations

import argparse
import asyncio
import fcntl
import json
import os
import platform
import pty
import resource
import select
import signal
import struct
import subprocess
import sys
import tempfile
import termios
from contextlib import suppress
from dataclasses import asdict
from pathlib import Path
from statistics import median
from time import perf_counter
from unittest.mock import patch


def _rss_bytes() -> int:
    peak = resource.getrusage(resource.RUSAGE_SELF).ru_maxrss
    return int(peak if sys.platform == "darwin" else peak * 1024)


def _headless_worker() -> None:
    started = perf_counter()
    from hcb.paths import AppPaths
    from hcb.runtime import Runtime
    from hcb.tui import HcbApp, WorkspaceTable

    imported = perf_counter() - started
    runtime = Runtime(AppPaths.discover())
    try:
        started = perf_counter()
        app = HcbApp(runtime)
        constructed = perf_counter() - started

        async def launch() -> tuple[float, float, int]:
            started = perf_counter()
            async with app.run_test(size=(120, 40)) as pilot:
                mounted = perf_counter() - started
                await pilot.pause()
                ready = perf_counter() - started
                rows = app.query_one("#content", WorkspaceTable).row_count
            return mounted, ready, rows

        mounted, ready, rows = asyncio.run(launch())
        print(
            json.dumps(
                {
                    "import_seconds": imported,
                    "construct_seconds": constructed,
                    "headless_mount_seconds": mounted,
                    "headless_ready_seconds": ready,
                    "headless_peak_rss_bytes": _rss_bytes(),
                    "visible_rows": rows,
                }
            )
        )
    finally:
        runtime.close()


def _fixture_environment(root: Path) -> dict[str, str]:
    environment = dict(os.environ)
    environment.pop("HCB_PROFILE", None)
    environment.pop("NO_COLOR", None)
    environment.update(
        {
            "XDG_CONFIG_HOME": str(root / "config"),
            "XDG_DATA_HOME": str(root / "data"),
            "XDG_CACHE_HOME": str(root / "cache"),
            "HCB_ENV_FILE": str(root / "no-credentials.env"),
            "HCB_ACCOUNT": "benchmark",
            "TERM": "xterm-256color",
            "COLUMNS": "120",
            "LINES": "40",
        }
    )
    return environment


def _create_fixture(environment: dict[str, str], tasks: int, events: int) -> None:
    from hcb.benchmarks import create_large_fixture
    from hcb.paths import AppPaths

    with patch.dict(os.environ, environment, clear=True):
        database = AppPaths.discover().database_file
    create_large_fixture(database, task_count=tasks, event_count=events)


def _headless_once(environment: dict[str, str], tasks: int) -> dict[str, float | int]:
    completed = subprocess.run(
        [sys.executable, str(Path(__file__).resolve()), "--headless-worker"],
        capture_output=True,
        text=True,
        env=environment,
        timeout=30,
        check=False,
    )
    if completed.returncode:
        raise RuntimeError(
            f"headless TUI exited {completed.returncode}: {completed.stderr.strip()}"
        )
    result: dict[str, float | int] = json.loads(completed.stdout)
    if result["visible_rows"] != tasks:
        raise RuntimeError(f"expected {tasks} task rows at startup")
    return result


def _process_rss_bytes(pid: int) -> int | None:
    with suppress(OSError, ValueError, subprocess.TimeoutExpired):
        completed = subprocess.run(
            ["ps", "-o", "rss=", "-p", str(pid)],
            capture_output=True,
            text=True,
            timeout=2,
            check=False,
        )
        if completed.returncode == 0:
            return int(completed.stdout.strip()) * 1024
    return None


def _terminal_once(environment: dict[str, str]) -> tuple[float, int | None]:
    master, slave = pty.openpty()
    fcntl.ioctl(slave, termios.TIOCSWINSZ, struct.pack("HHHH", 40, 120, 0, 0))
    started = perf_counter()
    try:
        process = subprocess.Popen(
            [sys.executable, "-c", "from hcb.cli import main; main()"],
            stdin=slave,
            stdout=slave,
            stderr=slave,
            env=environment,
            start_new_session=True,
        )
    finally:
        os.close(slave)
    output = bytearray()
    first_task: float | None = None
    rss: int | None = None
    try:
        while perf_counter() - started < 30:
            ready, _, _ = select.select([master], [], [], 0.2)
            if ready:
                try:
                    chunk = os.read(master, 65536)
                except OSError:
                    break
                if not chunk:
                    break
                output.extend(chunk)
                if b"Deterministic task" in output:
                    first_task = perf_counter() - started
                    rss = _process_rss_bytes(process.pid)
                    break
                if len(output) > 131072:
                    del output[:-65536]
            if process.poll() is not None:
                break
    finally:
        if process.poll() is None:
            with suppress(ProcessLookupError):
                os.killpg(process.pid, signal.SIGTERM)
        try:
            process.wait(timeout=5)
        except subprocess.TimeoutExpired:
            with suppress(ProcessLookupError):
                os.killpg(process.pid, signal.SIGKILL)
            process.wait(timeout=5)
        os.close(master)
    if first_task is None:
        raise RuntimeError(
            f"TUI did not emit a task-bearing terminal frame (exit {process.returncode})"
        )
    return first_task, rss


def _size_summary(samples: list[int]) -> dict[str, int | list[int]]:
    ordered = sorted(samples)
    return {
        "median_bytes": int(median(samples)),
        "p95_bytes": ordered[(95 * len(ordered) + 99) // 100 - 1],
        "samples_bytes": samples,
    }


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--counts", nargs="+", type=int, default=[1000, 10000])
    parser.add_argument("--runs", type=int, default=20)
    parser.add_argument("--headless-worker", action="store_true", help=argparse.SUPPRESS)
    args = parser.parse_args()
    if args.headless_worker:
        _headless_worker()
        return 0
    if args.runs < 1 or any(count < 1 for count in args.counts):
        parser.error("runs and every task count must be positive")

    from hcb.benchmarks import TimingSummary

    with tempfile.TemporaryDirectory(prefix="hcb-tui-startup-") as folder:
        environments = {}
        for count in args.counts:
            environment = _fixture_environment(Path(folder) / str(count))
            _create_fixture(environment, count, count // 5)
            environments[count] = environment
        results: dict[int, dict[str, list[float] | list[int]]] = {
            count: {
                "import_seconds": [],
                "construct_seconds": [],
                "headless_mount_seconds": [],
                "headless_ready_seconds": [],
                "terminal_first_task_seconds": [],
                "headless_peak_rss_bytes": [],
                "terminal_rss_at_first_task_bytes": [],
            }
            for count in args.counts
        }
        for _ in range(args.runs):
            for count in args.counts:
                samples = results[count]
                worker = _headless_once(environments[count], count)
                for metric in (
                    "import_seconds",
                    "construct_seconds",
                    "headless_mount_seconds",
                    "headless_ready_seconds",
                    "headless_peak_rss_bytes",
                ):
                    samples[metric].append(worker[metric])
                first_task, rss = _terminal_once(environments[count])
                samples["terminal_first_task_seconds"].append(first_task)
                if rss is not None:
                    samples["terminal_rss_at_first_task_bytes"].append(rss)

    report = []
    for count in args.counts:
        samples = results[count]
        report.append(
            {
                "tasks": count,
                "events": count // 5,
                "timings": {
                    metric: asdict(TimingSummary.from_samples(samples[metric]))
                    for metric in (
                        "import_seconds",
                        "construct_seconds",
                        "headless_mount_seconds",
                        "headless_ready_seconds",
                        "terminal_first_task_seconds",
                    )
                },
                "memory": {
                    metric: _size_summary(samples[metric])
                    for metric in (
                        "headless_peak_rss_bytes",
                        "terminal_rss_at_first_task_bytes",
                    )
                    if samples[metric]
                },
            }
        )
    print(
        json.dumps(
            {
                "environment": {
                    "os": platform.system(),
                    "release": platform.release(),
                    "machine": platform.machine(),
                    "python": platform.python_version(),
                    "terminal_columns": 120,
                    "terminal_rows": 40,
                },
                "results": report,
            },
            indent=2,
            sort_keys=True,
        )
    )
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
