#!/usr/bin/env python3
"""Exercise Qt bridge search and mutations against a disposable Python-owned core."""

from __future__ import annotations

import argparse
import json
import os
import stat
import subprocess
import sys
import tempfile
from contextlib import suppress
from datetime import date, timedelta
from pathlib import Path
from time import monotonic, sleep
from unittest.mock import patch
from urllib.parse import urlencode
from urllib.request import Request, urlopen

from hcb.benchmarks import create_large_fixture
from hcb.paths import AppPaths

ACCOUNT_ID = "benchmark"
UPDATED_TASK_TITLE = "Qt bridge interaction task updated"
UPDATED_TASK_NOTES = "Updated by the isolated Qt bridge acceptance"
UPDATED_EVENT_TITLE = "Qt bridge interaction event updated"
UPDATED_EVENT_DESCRIPTION = "Updated by the isolated Qt bridge acceptance"
UPDATED_EVENT_LOCATION = "Updated acceptance location"


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument(
        "--native", type=Path, required=True, help="Built Hot Cross Buns Qt executable."
    )
    return parser.parse_args()


def isolated_environment(root: Path) -> dict[str, str]:
    environment = dict(os.environ)
    for name in tuple(environment):
        if name.startswith("HCB_GOOGLE_"):
            environment.pop(name)
    for name in ("HCB_ACCOUNT", "HCB_ENV_FILE", "HCB_PROFILE", "NO_COLOR"):
        environment.pop(name, None)
    environment.update(
        {
            "XDG_CONFIG_HOME": str(root / "config"),
            "XDG_DATA_HOME": str(root / "data"),
            "XDG_CACHE_HOME": str(root / "cache"),
            "HCB_ACCOUNT": ACCOUNT_ID,
            "HCB_ENV_FILE": str(root / "no-credentials.env"),
            "QT_QPA_PLATFORM": "offscreen",
            "QT_QUICK_BACKEND": "software",
        }
    )
    return environment


def discovered_paths(environment: dict[str, str]) -> AppPaths:
    with patch.dict(os.environ, environment, clear=False):
        return AppPaths.discover()


def cli_command(*arguments: str) -> list[str]:
    return [sys.executable, "-m", "hcb.cli", *arguments]


def stop_process(process: subprocess.Popen[object], *, timeout_seconds: float = 5) -> None:
    if process.poll() is not None:
        return
    with suppress(ProcessLookupError):
        process.terminate()
    try:
        process.wait(timeout=timeout_seconds)
    except subprocess.TimeoutExpired:
        with suppress(ProcessLookupError):
            process.kill()
        process.wait(timeout=timeout_seconds)


def wait_for_descriptor(path: Path, process: subprocess.Popen[str]) -> dict[str, str | int]:
    deadline = monotonic() + 10
    while monotonic() < deadline:
        if path.exists():
            if stat.S_IMODE(path.stat().st_mode) != 0o600:
                raise RuntimeError("bridge descriptor is not owner-only")
            payload = json.loads(path.read_text())
            if (
                isinstance(payload, dict)
                and payload.get("api_version") == 1
                and isinstance(payload.get("token"), str)
                and isinstance(payload.get("url"), str)
                and payload["url"].startswith("http://127.0.0.1:")
            ):
                return payload
            raise RuntimeError("bridge descriptor is malformed")
        if process.poll() is not None:
            stdout, stderr = process.communicate()
            raise RuntimeError(
                f"bridge process exited before writing its descriptor: {stderr or stdout}"
            )
        sleep(0.05)
    raise RuntimeError("bridge process did not write its descriptor")


def start_bridge(
    descriptor: Path, environment: dict[str, str]
) -> tuple[subprocess.Popen[str], dict[str, str | int]]:
    process = subprocess.Popen(
        cli_command("bridge", "serve", "--ready-file", str(descriptor)),
        cwd=Path.cwd(),
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        text=True,
        env=environment,
        start_new_session=True,
    )
    return process, wait_for_descriptor(descriptor, process)


def bridge_data(descriptor: dict[str, str | int], path: str) -> dict[str, object]:
    url = descriptor["url"]
    token = descriptor["token"]
    if not isinstance(url, str) or not isinstance(token, str):
        raise RuntimeError("bridge descriptor is malformed")
    request = Request(url + path, headers={"Authorization": f"Bearer {token}"})
    with urlopen(request, timeout=10) as response:  # noqa: S310 - disposable loopback bridge
        payload = json.loads(response.read())
    if not isinstance(payload, dict) or payload.get("api_version") != 1:
        raise RuntimeError("bridge response is malformed")
    data = payload.get("data")
    if not isinstance(data, dict):
        raise RuntimeError("bridge response has no data payload")
    return data


def wait_for_json(path: Path, process: subprocess.Popen[str], *, label: str) -> dict[str, object]:
    deadline = monotonic() + 45
    while monotonic() < deadline:
        if path.exists():
            try:
                payload = json.loads(path.read_text())
            except json.JSONDecodeError as error:
                raise RuntimeError(f"{label} wrote malformed JSON") from error
            if not isinstance(payload, dict):
                raise RuntimeError(f"{label} report must be a JSON object")
            try:
                exit_code = process.wait(timeout=10)
            except subprocess.TimeoutExpired as error:
                stop_process(process)
                raise RuntimeError(f"{label} did not exit after writing its report") from error
            stdout, stderr = process.communicate()
            if exit_code != 0:
                detail = payload.get("error") or stderr or stdout
                raise RuntimeError(f"{label} failed (exit {exit_code}): {detail}")
            return payload
        if process.poll() is not None:
            stdout, stderr = process.communicate()
            raise RuntimeError(f"{label} exited before its report: {stderr or stdout}")
        sleep(0.05)
    stop_process(process)
    stdout, stderr = process.communicate()
    raise RuntimeError(f"{label} timed out: {stderr or stdout}")


def launch_qt(
    native: Path,
    descriptor: Path,
    environment: dict[str, str],
    report: Path,
    *,
    acceptance: bool,
) -> dict[str, object]:
    qt_environment = dict(environment)
    qt_environment[
        "HCB_BRIDGE_INTERACTION_ACCEPTANCE_FILE" if acceptance else "HCB_BRIDGE_READY_FILE"
    ] = str(report)
    process = subprocess.Popen(
        [
            str(native),
            "--bridge-descriptor",
            str(descriptor),
            "--bridge-account",
            ACCOUNT_ID,
        ],
        cwd=Path.cwd(),
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        text=True,
        env=qt_environment,
        start_new_session=True,
    )
    return wait_for_json(
        report, process, label="Qt bridge interaction" if acceptance else "Qt restart"
    )


def require_string(payload: dict[str, object], key: str) -> str:
    value = payload.get(key)
    if not isinstance(value, str) or not value:
        raise RuntimeError(f"Qt report has no {key}")
    return value


def verify_persistence(
    descriptor: dict[str, str | int], report: dict[str, object]
) -> dict[str, int]:
    task_id = require_string(report, "task_id")
    event_id = require_string(report, "event_id")
    event_date = date.fromisoformat(require_string(report, "event_date"))

    search_path = "/v1/accounts/benchmark/search?" + urlencode(
        {"q": UPDATED_TASK_TITLE, "limit": 50}
    )
    results = bridge_data(descriptor, search_path).get("results")
    if not isinstance(results, list):
        raise RuntimeError("restarted bridge returned malformed task search results")
    task = next(
        (
            result.get("item")
            for result in results
            if isinstance(result, dict)
            and result.get("kind") == "task"
            and isinstance(result.get("item"), dict)
            and result["item"].get("id") == task_id
        ),
        None,
    )
    if not isinstance(task, dict):
        raise RuntimeError("restarted bridge did not retain the Qt-created task")
    if (
        task.get("title") != UPDATED_TASK_TITLE
        or task.get("notes") != UPDATED_TASK_NOTES
        or task.get("status") != "completed"
    ):
        raise RuntimeError("restarted bridge returned an incomplete Qt task mutation")

    workspace_path = "/v1/accounts/benchmark/workspace?" + urlencode(
        {
            "include": "events",
            "start": event_date.isoformat(),
            "end": (event_date + timedelta(days=1)).isoformat(),
        }
    )
    workspace = bridge_data(descriptor, workspace_path).get("workspace")
    if not isinstance(workspace, dict) or not isinstance(workspace.get("events"), list):
        raise RuntimeError("restarted bridge returned malformed event workspace")
    event = next(
        (
            item
            for item in workspace["events"]
            if isinstance(item, dict) and item.get("id") == event_id
        ),
        None,
    )
    if not isinstance(event, dict):
        raise RuntimeError("restarted bridge did not retain the Qt-created event")
    if (
        event.get("summary") != UPDATED_EVENT_TITLE
        or event.get("description") != UPDATED_EVENT_DESCRIPTION
        or event.get("location") != UPDATED_EVENT_LOCATION
    ):
        raise RuntimeError("restarted bridge returned an incomplete Qt event mutation")
    return {"tasks": 1, "events": 1}


def run_acceptance(native: Path) -> dict[str, object]:
    if not native.is_file() or not os.access(native, os.X_OK):
        raise RuntimeError(f"native executable is unavailable: {native}")
    with tempfile.TemporaryDirectory(prefix="hcb-qt-bridge-interaction-") as temporary:
        root = Path(temporary)
        environment = isolated_environment(root)
        Path(environment["HCB_ENV_FILE"]).touch(mode=0o600)
        paths = discovered_paths(environment)
        paths.ensure()
        create_large_fixture(paths.database_file, task_count=1_000, event_count=1_500)

        descriptor_path = root / "bridge.json"
        bridge: subprocess.Popen[str] | None = None
        try:
            bridge, _descriptor = start_bridge(descriptor_path, environment)
            interaction = launch_qt(
                native, descriptor_path, environment, root / "qt-interaction.json", acceptance=True
            )
            if (
                interaction.get("ok") is not True
                or interaction.get("stage") != 7
                or not isinstance(interaction.get("initial_search_results"), int)
                or interaction["initial_search_results"] < 1
                or not isinstance(interaction.get("refreshed_search_results"), int)
                or interaction["refreshed_search_results"] < 1
            ):
                raise RuntimeError("Qt interaction report did not complete every bridge stage")

            stop_process(bridge)
            bridge = None
            if descriptor_path.exists():
                raise RuntimeError("bridge descriptor remained after the first clean shutdown")

            bridge, descriptor = start_bridge(descriptor_path, environment)
            persisted = verify_persistence(descriptor, interaction)
            restart = launch_qt(
                native, descriptor_path, environment, root / "qt-restart.json", acceptance=False
            )
            if not all(isinstance(restart.get(key), int) for key in ("tasks", "events")):
                raise RuntimeError("restarted Qt bridge readiness report is malformed")
            if restart["tasks"] < 1_001 or restart["events"] < 1:
                raise RuntimeError("restarted Qt bridge did not load the persisted workspace")

            return {
                "initial_search": True,
                "search_refresh": True,
                "task_create_update_complete": True,
                "event_create_update": True,
                "bridge_restart": True,
                "qt_restart": True,
                "persisted": persisted,
                "qt_restart_tasks": restart["tasks"],
                "qt_restart_events": restart["events"],
            }
        finally:
            if bridge is not None:
                stop_process(bridge)


def main() -> int:
    args = parse_args()
    result = run_acceptance(args.native.resolve())
    print(json.dumps({"qt_bridge_interaction_acceptance": result}, sort_keys=True))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
