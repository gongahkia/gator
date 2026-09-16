from __future__ import annotations

import json
import os
import subprocess
import sys
import threading
import time
from collections.abc import Callable, Iterator
from concurrent.futures import ThreadPoolExecutor
from datetime import UTC, date, datetime
from pathlib import Path
from time import perf_counter
from urllib.error import HTTPError
from urllib.request import Request, urlopen
from uuid import uuid4

import pytest

from hcb.benchmarks import create_large_fixture
from hcb.desktop_bridge import BRIDGE_API_VERSION, DesktopBridge
from hcb.models import (
    Account,
    Calendar,
    Conflict,
    ConflictStatus,
    DateTimeKind,
    EntityType,
    Event,
    EventDateTime,
    Task,
    TaskList,
)
from hcb.paths import AppPaths
from hcb.runtime import Runtime
from hcb.storage import Storage
from hcb.sync import SyncResult
from hcb.task_recurrence import parse_task_recurrence_notes


@pytest.fixture
def bridge_env(tmp_path: Path) -> Iterator[tuple[AppPaths, DesktopBridge]]:
    paths = AppPaths(tmp_path / "config", tmp_path / "data", tmp_path / "cache")
    credential_file = tmp_path / "test-account.env"
    credential_file.write_text("HCB_GOOGLE_CLIENT_ID=test-client-id\n")
    os.chmod(credential_file, 0o600)
    with Storage(paths.database_file) as storage, storage.transaction():
        storage.upsert_account(Account("work", "work@example.test", display_name="Work"))
        storage.upsert_task_list(TaskList("inbox", "work", "Inbox"))
        storage.upsert_calendar(Calendar("cal", "work", "Primary", time_zone="UTC"))
        storage.upsert_task(Task("seed-task", "work", "inbox", "Seed task", due=date(2026, 9, 14)))
        storage.upsert_event(
            Event(
                "seed-event",
                "work",
                "cal",
                "Seed event",
                EventDateTime(DateTimeKind.DATETIME, datetime(2026, 9, 14, 9, tzinfo=UTC)),
                EventDateTime(DateTimeKind.DATETIME, datetime(2026, 9, 14, 10, tzinfo=UTC)),
            )
        )
    bridge = DesktopBridge(
        lambda: Runtime(paths, environ={}, credential_file=credential_file),
        token="bridge-test-token",
    )
    worker = threading.Thread(target=bridge.serve_forever, daemon=True)
    worker.start()
    try:
        yield paths, bridge
    finally:
        bridge.shutdown()
        worker.join(timeout=5)
        assert not worker.is_alive()
        bridge.close()


def _request(
    bridge: DesktopBridge,
    method: str,
    path: str,
    *,
    body: dict[str, object] | None = None,
    token: str | None = "bridge-test-token",
    idempotency_key: str | None = None,
) -> tuple[int, dict[str, object]]:
    data = json.dumps(body).encode() if body is not None else None
    headers: dict[str, str] = {}
    if token is not None:
        headers["Authorization"] = f"Bearer {token}"
    if method in {"POST", "PATCH", "DELETE"}:
        headers["Idempotency-Key"] = uuid4().hex if idempotency_key is None else idempotency_key
    if data is not None:
        headers["Content-Type"] = "application/json"
    request = Request(bridge.descriptor.url + path, data=data, headers=headers, method=method)
    try:
        with urlopen(request, timeout=5) as response:  # noqa: S310 - loopback test server
            return response.status, json.loads(response.read())
    except HTTPError as error:
        return error.code, json.loads(error.read())


def _data(response: dict[str, object]) -> dict[str, object]:
    assert response["api_version"] == BRIDGE_API_VERSION
    data = response["data"]
    assert isinstance(data, dict)
    return data


def _wait_for(bridge: DesktopBridge, operation_id: str, expected: set[str]) -> dict[str, object]:
    deadline = time.monotonic() + 5
    while time.monotonic() < deadline:
        status, response = _request(bridge, "GET", f"/v1/operations/{operation_id}")
        assert status == 200
        operation = _data(response)["operation"]
        assert isinstance(operation, dict)
        if operation["state"] in expected:
            return operation
        time.sleep(0.01)
    raise AssertionError(f"operation {operation_id} did not reach {expected}")


def test_descriptor_authentication_and_health_are_loopback_private(
    bridge_env: tuple[AppPaths, DesktopBridge], tmp_path: Path
) -> None:
    _paths, bridge = bridge_env
    descriptor_path = tmp_path / "bridge.json"
    bridge.write_descriptor(descriptor_path)

    descriptor = json.loads(descriptor_path.read_text())
    assert descriptor == {
        "api_version": 1,
        "token": "bridge-test-token",
        "url": bridge.descriptor.url,
    }
    assert descriptor_path.stat().st_mode & 0o777 == 0o600

    status, unauthenticated = _request(bridge, "GET", "/v1/health", token=None)
    assert status == 401
    assert unauthenticated == {
        "api_version": 1,
        "error": {"code": "unauthorized", "message": "missing or invalid bridge token"},
    }

    status, response = _request(bridge, "GET", "/v1/health")
    assert status == 200
    assert _data(response) == {"service": "hcb-desktop-bridge", "api_version": 1}


def test_workspace_slices_are_account_scoped_and_range_bounded(
    bridge_env: tuple[AppPaths, DesktopBridge],
) -> None:
    _paths, bridge = bridge_env

    status, response = _request(bridge, "GET", "/v1/accounts/work/workspace")
    assert status == 200
    workspace = _data(response)["workspace"]
    assert isinstance(workspace, dict)
    assert workspace["account"]["id"] == "work"  # type: ignore[index]
    assert "tasks" not in workspace and "events" not in workspace

    status, response = _request(bridge, "GET", "/v1/accounts/work/auth")
    assert status == 200
    assert _data(response) == {"authentication": {"connected": False}}

    status, response = _request(
        bridge,
        "GET",
        "/v1/accounts/work/workspace?include=tasks,events&start=2026-09-14&end=2026-09-15",
    )
    assert status == 200
    workspace = _data(response)["workspace"]
    assert isinstance(workspace, dict)
    tasks = workspace["tasks"]
    events = workspace["events"]
    assert isinstance(tasks, list) and [task["id"] for task in tasks] == ["seed-task"]
    assert isinstance(events, list) and [event["id"] for event in events] == ["seed-event"]

    status, failure = _request(bridge, "GET", "/v1/accounts/work/workspace?include=events")
    assert status == 400
    assert failure["error"]["code"] == "invalid_request"  # type: ignore[index]

    status, failure = _request(bridge, "GET", "/v1/accounts/other/workspace")
    assert status == 404
    assert failure["error"]["code"] == "not_found"  # type: ignore[index]


def test_search_uses_the_indexed_core_result_contract(
    bridge_env: tuple[AppPaths, DesktopBridge],
) -> None:
    _paths, bridge = bridge_env

    status, response = _request(bridge, "GET", "/v1/accounts/work/search?q=Seed%20task&limit=1")
    assert status == 200
    results = _data(response)["results"]
    assert isinstance(results, list) and len(results) == 1
    result = results[0]
    assert isinstance(result, dict)
    assert result["kind"] == "task"
    assert result["score"] == 100
    item = result["item"]
    assert isinstance(item, dict)
    assert item["id"] == "seed-task"
    assert item["account_id"] == "work"
    assert item["title"] == "Seed task"

    status, failure = _request(bridge, "GET", "/v1/accounts/work/search?q=Seed&limit=201")
    assert status == 400
    assert failure["error"]["code"] == "invalid_request"  # type: ignore[index]


def test_authentication_state_is_reported_without_credential_material(tmp_path: Path) -> None:
    paths = AppPaths(tmp_path / "config", tmp_path / "data", tmp_path / "cache")
    with Storage(paths.database_file) as storage, storage.transaction():
        storage.upsert_account(Account("work", "work@example.test"))

    class AuthenticatedRuntime(Runtime):
        def has_stored_refresh_token(self, account_id: str) -> bool:
            assert account_id == "work"
            return True

    bridge = DesktopBridge(
        lambda: AuthenticatedRuntime(paths, environ={}), token="bridge-test-token"
    )
    worker = threading.Thread(target=bridge.serve_forever, daemon=True)
    worker.start()
    try:
        status, response = _request(bridge, "GET", "/v1/accounts/work/auth")
        assert status == 200
        assert _data(response) == {"authentication": {"connected": True}}
    finally:
        bridge.shutdown()
        worker.join(timeout=5)
        assert not worker.is_alive()
        bridge.close()


def test_task_and_event_mutations_use_the_python_optimistic_core(
    bridge_env: tuple[AppPaths, DesktopBridge],
) -> None:
    paths, bridge = bridge_env

    status, response = _request(
        bridge,
        "POST",
        "/v1/accounts/work/tasks",
        body={"list_id": "inbox", "title": "Bridge task", "due": "2026-09-15", "priority": "high"},
    )
    assert status == 201
    task = _data(response)["task"]
    assert isinstance(task, dict)
    task_id = task["id"]
    assert task["metadata"]["dirty"] is True  # type: ignore[index]

    status, response = _request(
        bridge,
        "PATCH",
        f"/v1/accounts/work/tasks/{task_id}",
        body={"notes": "Queued locally", "due": None},
    )
    assert status == 200
    assert _data(response)["task"]["notes"] == "Queued locally"  # type: ignore[index]
    assert _data(response)["task"]["due"] is None  # type: ignore[index]

    status, response = _request(
        bridge,
        "POST",
        f"/v1/accounts/work/tasks/{task_id}/complete",
        body={},
    )
    assert status == 200
    assert _data(response)["task"]["status"] == "completed"  # type: ignore[index]

    timing = {"kind": "dateTime", "value": "2026-09-16T09:00:00+00:00", "time_zone": "UTC"}
    ending = {"kind": "dateTime", "value": "2026-09-16T10:00:00+00:00", "time_zone": "UTC"}
    status, response = _request(
        bridge,
        "POST",
        "/v1/accounts/work/events",
        body={"calendar_id": "cal", "summary": "Bridge event", "start": timing, "end": ending},
    )
    assert status == 201
    event = _data(response)["event"]
    assert isinstance(event, dict)
    event_id = event["id"]

    status, response = _request(
        bridge,
        "PATCH",
        f"/v1/accounts/work/events/{event_id}",
        body={"summary": "Edited bridge event", "location": "Desk"},
    )
    assert status == 200
    assert _data(response)["event"]["summary"] == "Edited bridge event"  # type: ignore[index]

    status, response = _request(bridge, "DELETE", f"/v1/accounts/work/events/{event_id}")
    assert status == 200
    assert _data(response)["event"]["metadata"]["deleted"] is True  # type: ignore[index]

    with Storage(paths.database_file) as storage:
        assert storage.get_task("work", str(task_id)) is not None
        assert storage.get_event("work", str(event_id)).metadata.deleted
        # Two updates plus one create per entity use the same core as the CLI/TUI.
        assert storage.pending_mutation_count("work") == 6


def test_task_move_reparent_and_reorder_use_the_python_core(
    bridge_env: tuple[AppPaths, DesktopBridge],
) -> None:
    paths, bridge = bridge_env

    status, response = _request(
        bridge,
        "POST",
        "/v1/accounts/work/task-lists",
        body={"title": "Archive", "selected": False},
    )
    assert status == 201
    archive = _data(response)["task_list"]
    assert isinstance(archive, dict)
    archive_id = str(archive["id"])

    def create_task(title: str, *, parent_id: str | None = None) -> dict[str, object]:
        body: dict[str, object] = {"list_id": "inbox", "title": title}
        if parent_id is not None:
            body["parent_id"] = parent_id
        created_status, created_response = _request(
            bridge, "POST", "/v1/accounts/work/tasks", body=body
        )
        assert created_status == 201
        created = _data(created_response)["task"]
        assert isinstance(created, dict)
        return created

    parent = create_task("Move parent")
    parent_id = str(parent["id"])
    first_child = create_task("Move first child", parent_id=parent_id)
    first_child_id = str(first_child["id"])
    second_child = create_task("Move second child", parent_id=parent_id)
    second_child_id = str(second_child["id"])

    status, response = _request(
        bridge,
        "POST",
        f"/v1/accounts/work/tasks/{second_child_id}/move",
        body={"previous_id": None},
    )
    assert status == 200
    reordered_first = _data(response)["task"]
    assert isinstance(reordered_first, dict)
    assert reordered_first["parent_id"] == parent_id
    assert reordered_first["position"] is None

    status, response = _request(bridge, "GET", "/v1/accounts/work/tasks?list_id=inbox")
    assert status == 200
    page = _data(response)["page"]
    assert isinstance(page, dict)
    ordered_ids = [str(item["id"]) for item in page["tasks"]]  # type: ignore[index]
    assert ordered_ids.index(second_child_id) < ordered_ids.index(first_child_id)

    status, response = _request(
        bridge,
        "POST",
        f"/v1/accounts/work/tasks/{first_child_id}/move",
        body={"parent_id": None},
    )
    assert status == 200
    reparented_root = _data(response)["task"]
    assert isinstance(reparented_root, dict)
    assert reparented_root["list_id"] == "inbox"
    assert reparented_root["parent_id"] is None
    assert reparented_root["position"] is None

    status, response = _request(
        bridge,
        "POST",
        f"/v1/accounts/work/tasks/{first_child_id}/move",
        body={"parent_id": parent_id, "previous_id": second_child_id},
    )
    assert status == 200
    reparented_child = _data(response)["task"]
    assert isinstance(reparented_child, dict)
    assert reparented_child["parent_id"] == parent_id
    assert reparented_child["position"] == second_child_id

    status, failure = _request(
        bridge,
        "POST",
        f"/v1/accounts/work/tasks/{parent_id}/move",
        body={"parent_id": first_child_id},
    )
    assert status == 400
    assert failure["error"]["code"] == "invalid_request"  # type: ignore[index]

    idempotency_key = "cross-list-task-move"
    status, response = _request(
        bridge,
        "POST",
        f"/v1/accounts/work/tasks/{first_child_id}/move",
        body={"list_id": archive_id},
        idempotency_key=idempotency_key,
    )
    assert status == 200
    moved = _data(response)["task"]
    assert isinstance(moved, dict)
    assert moved["list_id"] == archive_id
    assert moved["parent_id"] is None
    assert moved["position"] is None

    status, retried = _request(
        bridge,
        "POST",
        f"/v1/accounts/work/tasks/{first_child_id}/move",
        body={"list_id": archive_id},
        idempotency_key=idempotency_key,
    )
    assert status == 200
    assert _data(retried) == _data(response)

    with Storage(paths.database_file) as storage:
        persisted = storage.get_task("work", first_child_id)
        assert persisted is not None
        assert persisted.list_id == archive_id
        assert persisted.parent_id is None
        assert persisted.position is None


def test_atomic_bulk_task_mutations_use_the_python_core(
    bridge_env: tuple[AppPaths, DesktopBridge],
) -> None:
    paths, bridge = bridge_env

    status, response = _request(
        bridge,
        "POST",
        "/v1/accounts/work/task-lists",
        body={"title": "Bulk archive"},
    )
    assert status == 201
    archive = _data(response)["task_list"]
    assert isinstance(archive, dict)
    archive_id = str(archive["id"])

    def create_task(title: str, *, parent_id: str | None = None) -> str:
        body: dict[str, object] = {"list_id": "inbox", "title": title}
        if parent_id is not None:
            body["parent_id"] = parent_id
        created_status, created_response = _request(
            bridge, "POST", "/v1/accounts/work/tasks", body=body
        )
        assert created_status == 201
        task = _data(created_response)["task"]
        assert isinstance(task, dict)
        return str(task["id"])

    first_id = create_task("Bulk first")
    second_id = create_task("Bulk second")
    parent_id = create_task("Bulk parent")
    create_task("Bulk child", parent_id=parent_id)
    task_ids = [first_id, second_id]

    completion_key = "bulk-task-completion"
    status, response = _request(
        bridge,
        "POST",
        "/v1/accounts/work/tasks/bulk/complete",
        body={"task_ids": task_ids},
        idempotency_key=completion_key,
    )
    assert status == 200
    completed = _data(response)
    assert [task["id"] for task in completed["tasks"]] == task_ids  # type: ignore[index]
    assert all(task["status"] == "completed" for task in completed["tasks"])  # type: ignore[index]
    assert completed["successors"] == []

    status, retry = _request(
        bridge,
        "POST",
        "/v1/accounts/work/tasks/bulk/complete",
        body={"task_ids": task_ids},
        idempotency_key=completion_key,
    )
    assert status == 200
    assert _data(retry) == completed

    status, failure = _request(
        bridge,
        "POST",
        "/v1/accounts/work/tasks/bulk/move",
        body={"task_ids": [parent_id], "list_id": archive_id},
    )
    assert status == 400
    assert failure["error"]["code"] == "invalid_request"  # type: ignore[index]
    with Storage(paths.database_file) as storage:
        parent = storage.get_task("work", parent_id)
        assert parent is not None
        assert parent.list_id == "inbox"

    status, response = _request(
        bridge,
        "POST",
        "/v1/accounts/work/tasks/bulk/move",
        body={"task_ids": task_ids, "list_id": archive_id},
        idempotency_key="bulk-task-move",
    )
    assert status == 200
    moved = _data(response)["tasks"]
    assert [task["id"] for task in moved] == task_ids  # type: ignore[index]
    assert all(task["list_id"] == archive_id for task in moved)  # type: ignore[index]
    assert all(task["parent_id"] is None for task in moved)  # type: ignore[index]

    status, response = _request(
        bridge,
        "POST",
        "/v1/accounts/work/tasks/bulk/delete",
        body={"task_ids": task_ids},
        idempotency_key="bulk-task-delete",
    )
    assert status == 200
    deleted = _data(response)["tasks"]
    assert [task["id"] for task in deleted] == task_ids  # type: ignore[index]
    assert all(task["metadata"]["deleted"] is True for task in deleted)  # type: ignore[index]

    with Storage(paths.database_file) as storage:
        for task_id in task_ids:
            task = storage.get_task("work", task_id)
            assert task is not None and task.metadata.deleted


def test_conflict_choices_use_the_python_core_without_exposing_payloads(
    bridge_env: tuple[AppPaths, DesktopBridge],
) -> None:
    paths, bridge = bridge_env
    with Storage(paths.database_file) as storage, storage.transaction():
        conflict_id = storage.add_conflict(
            Conflict(
                None,
                "work",
                EntityType.TASK,
                "seed-task",
                {"list_id": "inbox", "body": {"title": "Keep local"}},
                {"id": "google-seed-task", "title": "Keep Google"},
            )
        )
        uncertain_id = storage.add_conflict(
            Conflict(
                None,
                "work",
                EntityType.TASK,
                "uncertain-task",
                {"kind": "uncertain-delivery"},
                {},
            )
        )

    status, response = _request(bridge, "GET", "/v1/accounts/work/conflicts")
    assert status == 200
    conflicts = _data(response)["conflicts"]
    assert isinstance(conflicts, list)
    normal = next(conflict for conflict in conflicts if conflict["id"] == str(conflict_id))
    uncertain = next(conflict for conflict in conflicts if conflict["id"] == str(uncertain_id))
    assert normal == {
        "id": str(conflict_id),
        "resource": "Task",
        "status": "open",
        "message": "Local and Google changes need a choice.",
        "can_keep_local": True,
        "can_keep_remote": True,
        "resolved_at": None,
    }
    assert uncertain["can_keep_local"] is False
    assert uncertain["can_keep_remote"] is False
    assert "local_payload" not in normal and "remote_payload" not in normal

    resolution_key = "conflict-keep-remote"
    status, response = _request(
        bridge,
        "POST",
        f"/v1/accounts/work/conflicts/{conflict_id}/resolve",
        body={"resolution": "keep_remote"},
        idempotency_key=resolution_key,
    )
    assert status == 200
    resolved = _data(response)["conflict"]
    assert isinstance(resolved, dict)
    assert resolved["id"] == str(conflict_id)
    assert resolved["status"] == "keep_remote"
    assert isinstance(resolved["resolved_at"], str)

    status, retry = _request(
        bridge,
        "POST",
        f"/v1/accounts/work/conflicts/{conflict_id}/resolve",
        body={"resolution": "keep_remote"},
        idempotency_key=resolution_key,
    )
    assert status == 200
    assert _data(retry) == _data(response)

    status, failure = _request(
        bridge,
        "POST",
        f"/v1/accounts/work/conflicts/{uncertain_id}/resolve",
        body={"resolution": "keep_remote"},
        idempotency_key="uncertain-conflict-resolution",
    )
    assert status == 400
    assert failure["error"]["code"] == "invalid_request"  # type: ignore[index]
    assert "uncertain delivery" in failure["error"]["message"]  # type: ignore[index]

    status, response = _request(bridge, "GET", "/v1/accounts/work/conflicts")
    assert status == 200
    remaining = _data(response)["conflicts"]
    assert isinstance(remaining, list)
    assert [conflict["id"] for conflict in remaining] == [str(uncertain_id)]
    with Storage(paths.database_file) as storage:
        conflict = storage.get_conflict("work", conflict_id)
        assert conflict is not None and conflict.status is ConflictStatus.KEEP_REMOTE


def test_managed_task_recurrence_uses_the_durable_python_core(
    bridge_env: tuple[AppPaths, DesktopBridge],
) -> None:
    _paths, bridge = bridge_env
    recurrence = {
        "frequency": "daily",
        "interval": 1,
        "end": {"kind": "count", "count": 3},
    }
    status, response = _request(
        bridge,
        "POST",
        "/v1/accounts/work/tasks",
        body={
            "list_id": "inbox",
            "title": "Recurring bridge task",
            "notes": "Keep this context",
            "due": "2026-09-15",
            "due_time_zone": "UTC",
            "priority": "high",
            "recurrence": recurrence,
        },
    )
    assert status == 201
    created = _data(response)["task"]
    assert isinstance(created, dict)
    created_id = str(created["id"])
    created_marker = parse_task_recurrence_notes(str(created["notes"])).marker
    assert created_marker is not None
    assert created_marker.ordinal == 0
    assert created_marker.template_title == "Recurring bridge task"

    status, response = _request(
        bridge,
        "PATCH",
        f"/v1/accounts/work/tasks/{created_id}",
        body={
            "title": "Reconfigured recurring task",
            "due": "2026-09-16",
            "due_time_zone": "UTC",
            "recurrence": {
                "frequency": "weekly",
                "interval": 2,
                "end": {"kind": "count", "count": 3},
            },
        },
    )
    assert status == 200
    reconfigured = _data(response)["task"]
    assert isinstance(reconfigured, dict)
    reconfigured_marker = parse_task_recurrence_notes(str(reconfigured["notes"])).marker
    assert reconfigured_marker is not None
    assert (reconfigured_marker.frequency, reconfigured_marker.interval) == ("weekly", 2)
    assert reconfigured_marker.template_due_date == "2026-09-16"

    status, response = _request(
        bridge,
        "POST",
        f"/v1/accounts/work/tasks/{created_id}/complete",
        body={},
    )
    assert status == 200
    completed = _data(response)
    assert completed["task"]["status"] == "completed"  # type: ignore[index]
    successor = completed["successor"]
    assert isinstance(successor, dict)
    successor_id = str(successor["id"])
    successor_marker = parse_task_recurrence_notes(str(successor["notes"])).marker
    assert successor_marker is not None
    assert successor_marker.ordinal == 1

    status, response = _request(
        bridge,
        "POST",
        f"/v1/accounts/work/tasks/{successor_id}/recurrence/stop",
        body={"scope": "this"},
        idempotency_key="stop-recurring-task",
    )
    assert status == 200
    stopped = _data(response)["tasks"]
    assert isinstance(stopped, list)
    stopped_successor = next(item for item in stopped if item["id"] == successor_id)
    assert parse_task_recurrence_notes(str(stopped_successor["notes"])).state == "unmanaged"

    status, retry = _request(
        bridge,
        "POST",
        f"/v1/accounts/work/tasks/{successor_id}/recurrence/stop",
        body={"scope": "this"},
        idempotency_key="stop-recurring-task",
    )
    assert status == 200
    assert _data(retry) == _data(response)

    status, response = _request(
        bridge,
        "POST",
        "/v1/accounts/work/tasks",
        body={
            "list_id": "inbox",
            "title": "Split recurring bridge task",
            "due": "2026-09-15",
            "due_time_zone": "UTC",
            "recurrence": recurrence,
        },
    )
    assert status == 201
    split_source = _data(response)["task"]
    assert isinstance(split_source, dict)
    split_source_id = str(split_source["id"])
    source_marker = parse_task_recurrence_notes(str(split_source["notes"])).marker
    assert source_marker is not None
    status, response = _request(
        bridge,
        "POST",
        f"/v1/accounts/work/tasks/{split_source_id}/complete",
        body={},
    )
    assert status == 200
    split_successor = _data(response)["successor"]
    assert isinstance(split_successor, dict)
    split_successor_id = str(split_successor["id"])
    status, response = _request(
        bridge,
        "POST",
        f"/v1/accounts/work/tasks/{split_successor_id}/recurrence/split",
        body={},
    )
    assert status == 200
    split = _data(response)["tasks"]
    assert isinstance(split, list) and len(split) == 1
    split_marker = parse_task_recurrence_notes(str(split[0]["notes"])).marker
    assert split_marker is not None
    assert split_marker.series_id != source_marker.series_id
    assert split_marker.ordinal == 0


def test_task_list_and_calendar_management_stays_in_the_python_core(
    bridge_env: tuple[AppPaths, DesktopBridge],
) -> None:
    paths, bridge = bridge_env

    status, response = _request(
        bridge,
        "POST",
        "/v1/accounts/work/task-lists",
        body={"title": "Bridge list", "selected": False},
        idempotency_key="bridge-list-create",
    )
    assert status == 201
    task_list = _data(response)["task_list"]
    assert isinstance(task_list, dict)
    task_list_id = str(task_list["id"])
    assert task_list["selected"] is False

    status, response = _request(
        bridge,
        "POST",
        "/v1/accounts/work/task-lists",
        body={"title": "Bridge list", "selected": False},
        idempotency_key="bridge-list-create",
    )
    assert status == 201
    assert _data(response)["task_list"]["id"] == task_list_id  # type: ignore[index]

    status, failure = _request(
        bridge,
        "POST",
        "/v1/accounts/work/task-lists",
        body={"title": "Different bridge list"},
        idempotency_key="bridge-list-create",
    )
    assert status == 409
    assert failure["error"]["code"] == "conflict"  # type: ignore[index]

    status, response = _request(
        bridge,
        "PATCH",
        f"/v1/accounts/work/task-lists/{task_list_id}",
        body={"title": "Renamed bridge list", "selected": True},
    )
    assert status == 200
    assert _data(response)["task_list"]["title"] == "Renamed bridge list"  # type: ignore[index]
    assert _data(response)["task_list"]["selected"] is True  # type: ignore[index]
    with Storage(paths.database_file) as storage:
        # The rename is queued, while the visibility preference stays local.
        assert storage.pending_mutation_count("work") == 2

    status, response = _request(
        bridge,
        "POST",
        "/v1/accounts/work/calendars",
        body={
            "summary": "Bridge calendar",
            "description": "Created through the bridge",
            "time_zone": "UTC",
            "selected": False,
        },
    )
    assert status == 201
    calendar = _data(response)["calendar"]
    assert isinstance(calendar, dict)
    calendar_id = str(calendar["id"])
    assert calendar["selected"] is False

    status, response = _request(
        bridge,
        "PATCH",
        f"/v1/accounts/work/calendars/{calendar_id}",
        body={
            "summary": "Renamed bridge calendar",
            "description": None,
            "color": "#123456",
            "hidden": True,
            "selected": True,
        },
    )
    assert status == 200
    updated_calendar = _data(response)["calendar"]
    assert updated_calendar["summary"] == "Renamed bridge calendar"  # type: ignore[index]
    assert updated_calendar["description"] is None  # type: ignore[index]
    assert updated_calendar["color"] == "#123456"  # type: ignore[index]
    assert updated_calendar["hidden"] is True  # type: ignore[index]
    assert updated_calendar["selected"] is True  # type: ignore[index]

    status, response = _request(
        bridge,
        "POST",
        "/v1/accounts/work/calendar-subscriptions",
        body={"remote_calendar_id": "synthetic-subscription", "summary": "Subscribed fixture"},
    )
    assert status == 201
    subscription = _data(response)["calendar"]
    assert isinstance(subscription, dict)
    subscription_id = str(subscription["id"])
    assert subscription["remote_id"] == "synthetic-subscription"

    status, response = _request(
        bridge,
        "DELETE",
        f"/v1/accounts/work/calendar-subscriptions/{subscription_id}",
    )
    assert status == 200
    assert _data(response)["calendar"]["metadata"]["deleted"] is True  # type: ignore[index]

    status, response = _request(bridge, "GET", "/v1/accounts/work/workspace")
    assert status == 200
    workspace = _data(response)["workspace"]
    assert isinstance(workspace, dict)
    listed_task = next(item for item in workspace["task_lists"] if item["id"] == task_list_id)
    listed_calendar = next(item for item in workspace["calendars"] if item["id"] == calendar_id)
    assert listed_task["selected"] is True
    assert listed_calendar["hidden"] is True

    status, response = _request(bridge, "DELETE", f"/v1/accounts/work/task-lists/{task_list_id}")
    assert status == 200
    assert _data(response)["task_list"]["metadata"]["deleted"] is True  # type: ignore[index]
    status, response = _request(bridge, "DELETE", f"/v1/accounts/work/calendars/{calendar_id}")
    assert status == 200
    assert _data(response)["calendar"]["metadata"]["deleted"] is True  # type: ignore[index]

    with Storage(paths.database_file) as storage:
        assert storage.get_task_list("work", task_list_id).metadata.deleted
        assert storage.get_calendar("work", calendar_id).metadata.deleted
        assert storage.get_calendar("work", subscription_id).metadata.deleted
        # Selection alone is local; title and calendar writes retain normal outbox behavior.
        assert storage.pending_mutation_count("work") == 10


def test_mutation_idempotency_is_durable_and_rejects_key_reuse(
    bridge_env: tuple[AppPaths, DesktopBridge],
) -> None:
    paths, bridge = bridge_env
    body = {"list_id": "inbox", "title": "Retry-safe task"}

    status, response = _request(
        bridge,
        "POST",
        "/v1/accounts/work/tasks",
        body=body,
        idempotency_key="retry-safe-task",
    )
    assert status == 201
    first_task = _data(response)["task"]

    status, response = _request(
        bridge,
        "POST",
        "/v1/accounts/work/tasks",
        body=body,
        idempotency_key="retry-safe-task",
    )
    assert status == 201
    assert _data(response)["task"] == first_task
    with Storage(paths.database_file) as storage:
        assert storage.pending_mutation_count("work") == 1

    status, failure = _request(
        bridge,
        "POST",
        "/v1/accounts/work/tasks",
        body={"list_id": "inbox", "title": "Different task"},
        idempotency_key="retry-safe-task",
    )
    assert status == 409
    assert failure["error"]["code"] == "conflict"  # type: ignore[index]

    status, failure = _request(
        bridge,
        "POST",
        "/v1/accounts/work/tasks",
        body=body,
        idempotency_key="",
    )
    assert status == 400
    assert failure["error"]["code"] == "invalid_request"  # type: ignore[index]


def test_operations_cancel_sync_and_redact_oauth_tokens(tmp_path: Path) -> None:
    paths = AppPaths(tmp_path / "config", tmp_path / "data", tmp_path / "cache")
    sync_started = threading.Event()

    class OperationRuntime(Runtime):
        def sync_account(
            self,
            account_id: str,
            *,
            progress: Callable[[str], None] | None = None,
            cancelled: Callable[[], bool] | None = None,
            cancel_hint: str = "",
        ) -> SyncResult:
            assert account_id == "work" and cancel_hint
            assert progress is not None and cancelled is not None
            progress("sync started")
            sync_started.set()
            while not cancelled():
                time.sleep(0.01)
            progress("sync cancelled")
            return SyncResult(cancelled=True)

        def connect_account(
            self,
            account_id: str,
            *,
            expected_email: str,
            cancelled: Callable[[], bool] | None = None,
        ):
            assert account_id == "work" and expected_email == "work@example.test"
            assert cancelled is not None

            class Result:
                refresh_token = "never-expose-refresh-token"
                access_token = "never-expose-access-token"
                granted_scopes = ("scope-a", "scope-b")

            return Result()

    bridge = DesktopBridge(lambda: OperationRuntime(paths, environ={}), token="bridge-test-token")
    worker = threading.Thread(target=bridge.serve_forever, daemon=True)
    worker.start()
    try:
        status, response = _request(bridge, "POST", "/v1/accounts/work/sync", body={})
        assert status == 202
        operation = _data(response)["operation"]
        assert isinstance(operation, dict)
        assert sync_started.wait(5)
        operation_id = str(operation["id"])

        status, response = _request(bridge, "DELETE", f"/v1/operations/{operation_id}")
        assert status == 202
        final = _wait_for(bridge, operation_id, {"cancelled"})
        assert final["progress"] == ["sync started", "sync cancelled"]

        status, response = _request(
            bridge,
            "POST",
            "/v1/accounts/work/oauth",
            body={"expected_email": "work@example.test"},
        )
        assert status == 202
        oauth = _data(response)["operation"]
        assert isinstance(oauth, dict)
        final = _wait_for(bridge, str(oauth["id"]), {"succeeded"})
        assert final["result"] == {
            "connected": True,
            "granted_scopes": ["scope-a", "scope-b"],
        }
        assert "token" not in json.dumps(final)
    finally:
        bridge.shutdown()
        worker.join(timeout=5)
        assert not worker.is_alive()
        bridge.close()


def test_parallel_bridge_reads_coexist_with_cli_core_writes(
    bridge_env: tuple[AppPaths, DesktopBridge],
) -> None:
    paths, bridge = bridge_env

    def read_workspace(_index: int) -> int:
        status, response = _request(bridge, "GET", "/v1/accounts/work/workspace?include=tasks")
        assert status == 200
        workspace = _data(response)["workspace"]
        assert isinstance(workspace, dict)
        tasks = workspace["tasks"]
        assert isinstance(tasks, list)
        return len(tasks)

    def cli_write(_index: int) -> int:
        runtime = Runtime(paths, environ={})
        try:
            for index in range(20):
                runtime.application.create_task("work", "inbox", f"Concurrent task {index}")
            return runtime.application.pending_count("work")
        finally:
            runtime.close()

    with ThreadPoolExecutor(max_workers=9) as executor:
        reads = [executor.submit(read_workspace, index) for index in range(32)]
        write = executor.submit(cli_write, 0)
        observed_sizes = [future.result(timeout=10) for future in reads]
        pending_after_write = write.result(timeout=10)

    assert min(observed_sizes) >= 1
    assert max(observed_sizes) <= 21
    assert pending_after_write == 20
    with Storage(paths.database_file) as storage:
        assert storage.pending_mutation_count("work") == 20


def test_bridge_large_workspace_slices_avoid_hydration_until_requested(tmp_path: Path) -> None:
    paths = AppPaths(tmp_path / "config", tmp_path / "data", tmp_path / "cache")
    create_large_fixture(paths.database_file)
    bridge = DesktopBridge(lambda: Runtime(paths, environ={}), token="bridge-test-token")
    worker = threading.Thread(target=bridge.serve_forever, daemon=True)
    worker.start()
    try:
        started = perf_counter()
        status, response = _request(bridge, "GET", "/v1/accounts/benchmark/workspace")
        summary_seconds = perf_counter() - started
        assert status == 200
        summary = _data(response)["workspace"]
        assert isinstance(summary, dict)
        assert "tasks" not in summary and "events" not in summary

        started = perf_counter()
        status, response = _request(bridge, "GET", "/v1/accounts/benchmark/tasks?limit=200")
        first_page_seconds = perf_counter() - started
        assert status == 200
        page = _data(response)["page"]
        assert isinstance(page, dict)
        first_page = page["tasks"]
        cursor = page["next_cursor"]
        assert isinstance(first_page, list) and len(first_page) == 200
        assert isinstance(cursor, str)

        status, response = _request(
            bridge, "GET", f"/v1/accounts/benchmark/tasks?limit=200&cursor={cursor}"
        )
        assert status == 200
        second_page = _data(response)["page"]
        assert isinstance(second_page, dict)
        assert first_page[-1]["id"] != second_page["tasks"][0]["id"]  # type: ignore[index]

        started = perf_counter()
        status, response = _request(
            bridge,
            "GET",
            "/v1/accounts/benchmark/workspace?include=tasks,events&start=2026-03-01&end=2026-04-01",
        )
        full_slice_seconds = perf_counter() - started
        assert status == 200
        workspace = _data(response)["workspace"]
        assert isinstance(workspace, dict)
        assert len(workspace["tasks"]) == 10_000  # type: ignore[arg-type]
        assert len(workspace["events"]) > 100  # type: ignore[arg-type]

        # Regression tripwires, deliberately looser than local development.
        assert summary_seconds < 1.0
        assert first_page_seconds < 0.25
        assert full_slice_seconds < 5.0
    finally:
        bridge.shutdown()
        worker.join(timeout=5)
        assert not worker.is_alive()
        bridge.close()


def test_cli_bridge_process_writes_then_removes_its_descriptor(tmp_path: Path) -> None:
    ready_file = tmp_path / "bridge.json"
    environment = {
        **os.environ,
        "XDG_CONFIG_HOME": str(tmp_path / "config"),
        "XDG_DATA_HOME": str(tmp_path / "data"),
        "XDG_CACHE_HOME": str(tmp_path / "cache"),
        "PYTHONDONTWRITEBYTECODE": "1",
    }
    process = subprocess.Popen(
        [sys.executable, "-m", "hcb.cli", "bridge", "serve", "--ready-file", str(ready_file)],
        cwd=Path(__file__).resolve().parents[1],
        env=environment,
        stdin=subprocess.DEVNULL,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        text=True,
    )
    try:
        deadline = time.monotonic() + 5
        while not ready_file.exists() and time.monotonic() < deadline:
            time.sleep(0.01)
        assert ready_file.exists()
        descriptor = json.loads(ready_file.read_text())
        request = Request(
            descriptor["url"] + "/v1/health",
            headers={"Authorization": f"Bearer {descriptor['token']}"},
        )
        with urlopen(request, timeout=5) as response:  # noqa: S310 - launched loopback helper
            assert response.status == 200
            assert json.loads(response.read())["data"]["service"] == "hcb-desktop-bridge"
    finally:
        process.terminate()
        process.wait(timeout=5)

    assert process.returncode == 0
    assert not ready_file.exists()
