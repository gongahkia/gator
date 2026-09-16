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
from hcb.models import Conflict, ConflictStatus, EntityType
from hcb.paths import AppPaths
from hcb.storage import Storage
from hcb.task_recurrence import parse_task_recurrence_notes

ACCOUNT_ID = "benchmark"
UPDATED_TASK_TITLE = "Qt bridge interaction task updated"
UPDATED_TASK_NOTES = "Updated by the isolated Qt bridge acceptance"
UPDATED_EVENT_TITLE = "Qt bridge interaction event updated"
UPDATED_EVENT_DESCRIPTION = "Updated by the isolated Qt bridge acceptance"
UPDATED_EVENT_LOCATION = "Updated acceptance location"
UPDATED_TASK_LIST_TITLE = "Qt bridge acceptance list updated"
HIDDEN_TASK_TITLE = "Qt bridge acceptance hidden-list task"
UPDATED_CALENDAR_TITLE = "Qt bridge acceptance calendar updated"
UPDATED_CALENDAR_DESCRIPTION = "Updated by the isolated Qt bridge calendar acceptance"
SUBSCRIPTION_REMOTE_ID = "qt-bridge-acceptance-subscription"
HIERARCHY_PARENT_TASK_TITLE = "Qt bridge hierarchy parent"
HIERARCHY_FIRST_CHILD_TASK_TITLE = "Qt bridge hierarchy first child"
HIERARCHY_SECOND_CHILD_TASK_TITLE = "Qt bridge hierarchy second child"


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


def wait_for_json(
    path: Path,
    process: subprocess.Popen[str],
    *,
    label: str,
    timeout_seconds: float = 45,
) -> dict[str, object]:
    deadline = monotonic() + timeout_seconds
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
                raise RuntimeError(f"{label} failed (exit {exit_code}): {detail}; report={payload}")
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
        report,
        process,
        label="Qt bridge interaction" if acceptance else "Qt restart",
        timeout_seconds=75 if acceptance else 45,
    )


def require_string(payload: dict[str, object], key: str) -> str:
    value = payload.get(key)
    if not isinstance(value, str) or not value:
        raise RuntimeError(f"Qt report has no {key}")
    return value


def verify_persistence(
    descriptor: dict[str, str | int], report: dict[str, object], database_file: Path
) -> dict[str, int]:
    task_id = require_string(report, "task_id")
    hidden_task_id = require_string(report, "hidden_task_id")
    event_id = require_string(report, "event_id")
    task_list_id = require_string(report, "task_list_id")
    calendar_id = require_string(report, "calendar_id")
    recurrence_task_id = require_string(report, "recurrence_task_id")
    recurrence_successor_id = require_string(report, "recurrence_successor_id")
    split_recurrence_task_id = require_string(report, "split_recurrence_task_id")
    split_recurrence_successor_id = require_string(report, "split_recurrence_successor_id")
    hierarchy_parent_id = require_string(report, "hierarchy_parent_id")
    hierarchy_first_child_id = require_string(report, "hierarchy_first_child_id")
    hierarchy_second_child_id = require_string(report, "hierarchy_second_child_id")
    bulk_first_task_id = require_string(report, "bulk_first_task_id")
    bulk_second_task_id = require_string(report, "bulk_second_task_id")
    conflict_id = int(require_string(report, "conflict_id"))
    event_date = date.fromisoformat(require_string(report, "event_date"))

    def searched_task(title: str, task_id: str) -> dict[str, object]:
        results = bridge_data(
            descriptor,
            "/v1/accounts/benchmark/search?" + urlencode({"q": title, "limit": 50}),
        ).get("results")
        if not isinstance(results, list):
            raise RuntimeError("restarted bridge returned malformed recurrence task search results")
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
            raise RuntimeError("restarted bridge did not retain a Qt-managed recurrence task")
        return task

    recurring_source = searched_task("Qt bridge recurring task updated", recurrence_task_id)
    stopped_occurrence = searched_task("Qt bridge recurring task updated", recurrence_successor_id)
    split_source = searched_task("Qt bridge split recurring task", split_recurrence_task_id)
    split_occurrence = searched_task(
        "Qt bridge split recurring task", split_recurrence_successor_id
    )
    recurring_marker = parse_task_recurrence_notes(str(recurring_source.get("notes") or "")).marker
    stopped_notes = parse_task_recurrence_notes(str(stopped_occurrence.get("notes") or ""))
    split_source_marker = parse_task_recurrence_notes(str(split_source.get("notes") or "")).marker
    split_marker = parse_task_recurrence_notes(str(split_occurrence.get("notes") or "")).marker
    if (
        recurring_source.get("status") != "completed"
        or recurring_marker is None
        or (recurring_marker.frequency, recurring_marker.interval) != ("weekly", 2)
        or stopped_notes.state != "unmanaged"
        or split_source_marker is None
        or split_marker is None
        or split_marker.series_id == split_source_marker.series_id
        or split_marker.ordinal != 0
    ):
        raise RuntimeError("restarted bridge returned incomplete Qt recurrence mutations")

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
        or task.get("due") != event_date.isoformat()
    ):
        raise RuntimeError("restarted bridge returned an incomplete Qt task mutation")

    hidden_results = bridge_data(
        descriptor,
        "/v1/accounts/benchmark/search?" + urlencode({"q": HIDDEN_TASK_TITLE, "limit": 50}),
    ).get("results")
    if not isinstance(hidden_results, list):
        raise RuntimeError("restarted bridge returned malformed hidden-list task search results")
    hidden_task = next(
        (
            result.get("item")
            for result in hidden_results
            if isinstance(result, dict)
            and result.get("kind") == "task"
            and isinstance(result.get("item"), dict)
            and result["item"].get("id") == hidden_task_id
        ),
        None,
    )
    if not isinstance(hidden_task, dict) or hidden_task.get("title") != HIDDEN_TASK_TITLE:
        raise RuntimeError("restarted bridge did not retain the deselected-list task")

    hierarchy_parent = searched_task(HIERARCHY_PARENT_TASK_TITLE, hierarchy_parent_id)
    hierarchy_first_child = searched_task(
        HIERARCHY_FIRST_CHILD_TASK_TITLE, hierarchy_first_child_id
    )
    hierarchy_second_child = searched_task(
        HIERARCHY_SECOND_CHILD_TASK_TITLE, hierarchy_second_child_id
    )
    if (
        hierarchy_parent.get("list_id") != "inbox"
        or hierarchy_parent.get("parent_id") is not None
        or hierarchy_first_child.get("list_id") != task_list_id
        or hierarchy_first_child.get("parent_id") is not None
        or hierarchy_second_child.get("list_id") != "inbox"
        or hierarchy_second_child.get("parent_id") != hierarchy_parent_id
    ):
        raise RuntimeError("restarted bridge returned incomplete Qt task hierarchy changes")

    with Storage(database_file) as storage:
        for task_id in (bulk_first_task_id, bulk_second_task_id):
            task = storage.get_task(ACCOUNT_ID, task_id)
            if task is None or not task.metadata.deleted:
                raise RuntimeError("restarted core did not retain a Qt bulk task deletion")
        conflict = storage.get_conflict(ACCOUNT_ID, conflict_id)
        if conflict is None or conflict.status is not ConflictStatus.KEEP_REMOTE:
            raise RuntimeError("restarted core did not retain the Qt conflict resolution")

    conflicts = bridge_data(descriptor, "/v1/accounts/benchmark/conflicts").get("conflicts")
    if conflicts != []:
        raise RuntimeError("restarted bridge retained the resolved Qt conflict")

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
    start = event.get("start")
    end = event.get("end")
    if (
        not isinstance(start, dict)
        or not isinstance(end, dict)
        or start.get("kind") != "dateTime"
        or start.get("value") != f"{event_date.isoformat()}T10:00:00+00:00"
        or end.get("kind") != "dateTime"
        or end.get("value") != f"{event_date.isoformat()}T11:00:00+00:00"
    ):
        raise RuntimeError("restarted bridge returned incomplete Qt event timing changes")

    summary = bridge_data(descriptor, "/v1/accounts/benchmark/workspace").get("workspace")
    if not isinstance(summary, dict):
        raise RuntimeError("restarted bridge returned malformed workspace summary")
    task_lists = summary.get("task_lists")
    calendars = summary.get("calendars")
    if not isinstance(task_lists, list) or not isinstance(calendars, list):
        raise RuntimeError("restarted bridge omitted task-list or calendar summaries")
    task_list = next(
        (item for item in task_lists if isinstance(item, dict) and item.get("id") == task_list_id),
        None,
    )
    calendar = next(
        (item for item in calendars if isinstance(item, dict) and item.get("id") == calendar_id),
        None,
    )
    if not isinstance(task_list, dict) or (
        task_list.get("title") != UPDATED_TASK_LIST_TITLE or task_list.get("selected") is not False
    ):
        raise RuntimeError("restarted bridge returned incomplete Qt task-list changes")
    if not isinstance(calendar, dict) or (
        calendar.get("summary") != UPDATED_CALENDAR_TITLE
        or calendar.get("description") != UPDATED_CALENDAR_DESCRIPTION
        or calendar.get("time_zone") != "UTC"
        or calendar.get("color") != "#123456"
        or calendar.get("selected") is not False
        or calendar.get("hidden") is not True
    ):
        raise RuntimeError("restarted bridge returned incomplete Qt calendar changes")
    if any(
        isinstance(item, dict) and item.get("remote_id") == SUBSCRIPTION_REMOTE_ID
        for item in calendars
    ):
        raise RuntimeError("restarted bridge retained an unsubscribed calendar")
    return {"tasks": 6, "events": 1, "task_lists": 1, "calendars": 1}


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
        with Storage(paths.database_file) as storage, storage.transaction():
            storage.add_conflict(
                Conflict(
                    None,
                    ACCOUNT_ID,
                    EntityType.TASK,
                    "task-00000",
                    {"list_id": "inbox", "body": {"title": "Qt conflict fixture"}},
                    {"id": "remote-task-00000", "title": "Google conflict fixture"},
                )
            )

        descriptor_path = root / "bridge.json"
        bridge: subprocess.Popen[str] | None = None
        try:
            bridge, _descriptor = start_bridge(descriptor_path, environment)
            interaction = launch_qt(
                native, descriptor_path, environment, root / "qt-interaction.json", acceptance=True
            )
            if (
                interaction.get("ok") is not True
                or interaction.get("stage") != 35
                or not isinstance(interaction.get("hidden_task_id"), str)
                or not interaction["hidden_task_id"]
                or not all(
                    isinstance(interaction.get(key), str) and interaction[key]
                    for key in (
                        "recurrence_task_id",
                        "recurrence_successor_id",
                        "split_recurrence_task_id",
                        "split_recurrence_successor_id",
                        "hierarchy_parent_id",
                        "hierarchy_first_child_id",
                        "hierarchy_second_child_id",
                        "bulk_first_task_id",
                        "bulk_second_task_id",
                        "conflict_id",
                    )
                )
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
            persisted = verify_persistence(descriptor, interaction, paths.database_file)
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
                "task_list_create_rename_visibility": True,
                "task_list_visibility_hides_tasks": True,
                "calendar_create_settings": True,
                "calendar_subscribe_unsubscribe": True,
                "recurrence_create_reconfigure_complete_stop_split": True,
                "task_hierarchy_reparent_reorder_move": True,
                "bulk_task_complete_move_delete": True,
                "conflict_list_keep_remote": True,
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
