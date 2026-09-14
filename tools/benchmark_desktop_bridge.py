#!/usr/bin/env python3
"""Measure the local desktop bridge against a deterministic SQLite mirror."""

from __future__ import annotations

import argparse
import json
import platform
import tempfile
import threading
from pathlib import Path
from statistics import median
from time import perf_counter
from urllib.request import Request, urlopen

from hcb.benchmarks import create_large_fixture
from hcb.desktop_bridge import DesktopBridge
from hcb.paths import AppPaths
from hcb.runtime import Runtime


def _request(bridge: DesktopBridge, path: str) -> tuple[float, int]:
    request = Request(
        bridge.descriptor.url + path,
        headers={"Authorization": f"Bearer {bridge.descriptor.token}"},
    )
    started = perf_counter()
    with urlopen(request, timeout=15) as response:  # noqa: S310 - loopback bridge created above
        payload = json.loads(response.read())
    return perf_counter() - started, len(json.dumps(payload, separators=(",", ":")))


def _summary(samples: list[tuple[float, int]]) -> dict[str, object]:
    seconds = [sample[0] for sample in samples]
    return {
        "runs": len(samples),
        "median_seconds": median(seconds),
        "max_seconds": max(seconds),
        "response_bytes": samples[-1][1],
        "samples_seconds": seconds,
    }


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--tasks", type=int, default=10_000)
    parser.add_argument("--events", type=int, default=2_000)
    parser.add_argument("--page-size", type=int, default=200)
    parser.add_argument("--runs", type=int, default=5)
    args = parser.parse_args()
    if args.tasks < 1 or args.events < 0 or args.runs < 1:
        parser.error("tasks and runs must be positive; events must be nonnegative")
    if not 1 <= args.page_size <= 500:
        parser.error("page-size must be between 1 and 500")

    with tempfile.TemporaryDirectory(prefix="hcb-desktop-bridge-benchmark-") as directory:
        root = Path(directory)
        paths = AppPaths(root / "config", root / "data", root / "cache")
        create_large_fixture(paths.database_file, task_count=args.tasks, event_count=args.events)
        bridge = DesktopBridge(lambda: Runtime(paths, environ={}))
        worker = threading.Thread(target=bridge.serve_forever, daemon=True)
        worker.start()
        try:
            timings = {
                "workspace_summary": [
                    _request(bridge, "/v1/accounts/benchmark/workspace") for _ in range(args.runs)
                ],
                "task_page": [
                    _request(bridge, f"/v1/accounts/benchmark/tasks?limit={args.page_size}")
                    for _ in range(args.runs)
                ],
                "search": [
                    _request(bridge, "/v1/accounts/benchmark/search?q=release-marker&limit=50")
                    for _ in range(args.runs)
                ],
                "event_range": [
                    _request(
                        bridge,
                        "/v1/accounts/benchmark/workspace?include=events"
                        "&start=2026-03-01&end=2026-04-01",
                    )
                    for _ in range(args.runs)
                ],
                "full_task_workspace": [
                    _request(bridge, "/v1/accounts/benchmark/workspace?include=tasks")
                    for _ in range(args.runs)
                ],
            }
        finally:
            bridge.shutdown()
            worker.join(timeout=5)
            bridge.close()

    report = {
        "fixture": {"tasks": args.tasks, "events": args.events, "task_page_size": args.page_size},
        "environment": {
            "os": platform.system(),
            "release": platform.release(),
            "machine": platform.machine(),
        },
        "timings": {name: _summary(samples) for name, samples in timings.items()},
    }
    print(json.dumps(report, indent=2, sort_keys=True))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
