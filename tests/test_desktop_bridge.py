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
from hcb.models import Account, Calendar, DateTimeKind, Event, EventDateTime, Task, TaskList
from hcb.paths import AppPaths
from hcb.runtime import Runtime
from hcb.storage import Storage
from hcb.sync import SyncResult


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
