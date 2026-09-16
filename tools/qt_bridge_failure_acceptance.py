#!/usr/bin/env python3
"""Exercise Qt bridge cancellation, failure recovery, and shutdown on a fake loopback bridge."""

from __future__ import annotations

import argparse
import json
import os
import stat
import subprocess
import tempfile
from contextlib import suppress
from http import HTTPStatus
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path
from threading import Lock, Thread
from time import monotonic, sleep
from urllib.parse import urlsplit

ACCOUNT_ID = "benchmark"
BRIDGE_TOKEN = "qt_bridge_failure_acceptance_token_20260914"


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument(
        "--native", type=Path, required=True, help="Built Hot Cross Buns Qt executable."
    )
    return parser.parse_args()


def isolated_environment(root: Path) -> dict[str, str]:
    environment = dict(os.environ)
    for name in tuple(environment):
        if name.startswith("HCB_GOOGLE_") or name.startswith("HCB_BRIDGE_"):
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


def stop_process(process: subprocess.Popen[str], *, timeout_seconds: float = 5) -> None:
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


def operation_snapshot(identifier: str, state: str) -> dict[str, object]:
    return {
        "id": identifier,
        "kind": "sync",
        "account_id": ACCOUNT_ID,
        "state": state,
        "cancellation_requested": state == "cancelled",
        "progress": [],
        "elapsed_ms": 0,
    }


def workspace_summary() -> dict[str, object]:
    metadata = {"etag": None, "local_updated_at": "2026-09-14T00:00:00+00:00", "deleted": False}
    return {
        "account": {"id": ACCOUNT_ID, "email": "bridge@example.test"},
        "pending": 0,
        "task_lists": [
            {
                "id": "inbox",
                "account_id": ACCOUNT_ID,
                "remote_id": None,
                "title": "Inbox",
                "position": 0,
                "selected": True,
                "metadata": metadata,
            }
        ],
        "calendars": [
            {
                "id": "primary",
                "account_id": ACCOUNT_ID,
                "remote_id": "primary",
                "summary": "Primary",
                "description": None,
                "time_zone": "UTC",
                "color": None,
                "selected": True,
                "hidden": False,
                "metadata": metadata,
            }
        ],
    }


class FailureBridge(ThreadingHTTPServer):
    def __init__(self) -> None:
        super().__init__(("127.0.0.1", 0), FailureBridgeHandler)
        self.lock = Lock()
        self.sync_starts = 0
        self.cancelled = False
        self.unexpected_requests: list[str] = []


class FailureBridgeHandler(BaseHTTPRequestHandler):
    server: FailureBridge

    def log_message(self, _format: str, *_args: object) -> None:
        return

    def send_payload(self, status: HTTPStatus, payload: dict[str, object]) -> None:
        encoded = json.dumps(payload, separators=(",", ":")).encode()
        try:
            self.send_response(status)
            self.send_header("Content-Type", "application/json")
            self.send_header("Content-Length", str(len(encoded)))
            self.end_headers()
            self.wfile.write(encoded)
        except BrokenPipeError:
            pass

    def data(self, value: dict[str, object], status: HTTPStatus = HTTPStatus.OK) -> None:
        self.send_payload(status, {"api_version": 1, "data": value})

    def error(self, status: HTTPStatus, message: str) -> None:
        self.send_payload(status, {"api_version": 1, "error": {"message": message}})

    def permitted(self) -> bool:
        if self.headers.get("Authorization") == f"Bearer {BRIDGE_TOKEN}":
            return True
        self.error(HTTPStatus.UNAUTHORIZED, "missing or invalid bridge token")
        return False

    def unexpected(self, method: str, path: str) -> None:
        with self.server.lock:
            self.server.unexpected_requests.append(f"{method} {path}")
        self.error(HTTPStatus.INTERNAL_SERVER_ERROR, "unexpected synthetic bridge request")

    def do_GET(self) -> None:  # noqa: N802 - required BaseHTTPRequestHandler API
        if not self.permitted():
            return
        request = urlsplit(self.path)
        path = request.path
        prefix = f"/v1/accounts/{ACCOUNT_ID}"
        if path == f"{prefix}/auth":
            self.data({"authentication": {"connected": False}})
            return
        if path == f"{prefix}/workspace":
            if request.query:
                self.data({"workspace": {"events": []}})
            else:
                self.data({"workspace": workspace_summary()})
            return
        if path == f"{prefix}/tasks":
            self.data({"page": {"tasks": [], "next_cursor": None}})
            return
        if path == f"{prefix}/conflicts":
            self.data({"conflicts": []})
            return
        if path == "/v1/operations/sync-cancel":
            with self.server.lock:
                cancelled = self.server.cancelled
            self.data(
                {
                    "operation": operation_snapshot(
                        "sync-cancel", "cancelled" if cancelled else "running"
                    )
                }
            )
            return
        if path == "/v1/operations/sync-mismatch":
            sleep(0.25)
            self.data({"operation": operation_snapshot("different-operation", "succeeded")})
            return
        if path == "/v1/operations/sync-shutdown":
            sleep(2)
            self.data({"operation": operation_snapshot("sync-shutdown", "running")})
            return
        self.unexpected("GET", path)

    def do_POST(self) -> None:  # noqa: N802 - required BaseHTTPRequestHandler API
        if not self.permitted():
            return
        path = urlsplit(self.path).path
        prefix = f"/v1/accounts/{ACCOUNT_ID}"
        if path == f"{prefix}/sync":
            with self.server.lock:
                self.server.sync_starts += 1
                sequence = self.server.sync_starts
            identifiers = {
                1: "sync-cancel",
                2: "sync-mismatch",
                3: "sync-invalid-start",
                4: "sync-shutdown",
            }
            identifier = identifiers.get(sequence)
            if identifier is None:
                self.unexpected("POST", path)
                return
            state = "invalid" if identifier == "sync-invalid-start" else "queued"
            self.data({"operation": operation_snapshot(identifier, state)}, HTTPStatus.ACCEPTED)
            return
        if path == f"{prefix}/tasks":
            self.error(HTTPStatus.CONFLICT, "synthetic task rejection")
            return
        if path == f"{prefix}/task-lists":
            self.data({"task_list": {}}, HTTPStatus.CREATED)
            return
        if path == f"{prefix}/calendars":
            self.error(HTTPStatus.SERVICE_UNAVAILABLE, "synthetic calendar rejection")
            return
        self.unexpected("POST", path)

    def do_DELETE(self) -> None:  # noqa: N802 - required BaseHTTPRequestHandler API
        if not self.permitted():
            return
        path = urlsplit(self.path).path
        if path == "/v1/operations/sync-cancel":
            with self.server.lock:
                self.server.cancelled = True
            self.data({"operation": operation_snapshot("sync-cancel", "cancelled")})
            return
        self.unexpected("DELETE", path)


def write_descriptor(path: Path, port: int) -> None:
    path.write_text(
        json.dumps(
            {"api_version": 1, "url": f"http://127.0.0.1:{port}", "token": BRIDGE_TOKEN},
            separators=(",", ":"),
        )
    )
    path.chmod(0o600)


def wait_for_report(path: Path, process: subprocess.Popen[str]) -> dict[str, object]:
    deadline = monotonic() + 35
    while monotonic() < deadline:
        if path.exists():
            payload = json.loads(path.read_text())
            if not isinstance(payload, dict):
                raise RuntimeError("Qt failure acceptance report is not a JSON object")
            try:
                exit_code = process.wait(timeout=10)
            except subprocess.TimeoutExpired as error:
                stop_process(process)
                raise RuntimeError(
                    "Qt process did not exit after writing its failure report"
                ) from error
            stdout, stderr = process.communicate()
            if exit_code != 0:
                raise RuntimeError(
                    f"Qt failure acceptance failed (exit {exit_code}): "
                    f"{payload.get('error') or stderr or stdout}; report={payload}"
                )
            return payload
        if process.poll() is not None:
            stdout, stderr = process.communicate()
            raise RuntimeError(f"Qt process exited before its failure report: {stderr or stdout}")
        sleep(0.05)
    stop_process(process)
    stdout, stderr = process.communicate()
    raise RuntimeError(f"Qt failure acceptance timed out: {stderr or stdout}")


def run_acceptance(native: Path) -> dict[str, bool]:
    if not native.is_file() or not os.access(native, os.X_OK):
        raise RuntimeError(f"native executable is unavailable: {native}")
    with tempfile.TemporaryDirectory(prefix="hcb-qt-bridge-failure-") as temporary:
        root = Path(temporary)
        environment = isolated_environment(root)
        Path(environment["HCB_ENV_FILE"]).touch(mode=0o600)
        bridge = FailureBridge()
        server_thread = Thread(target=bridge.serve_forever, daemon=True)
        server_thread.start()
        process: subprocess.Popen[str] | None = None
        try:
            descriptor = root / "bridge.json"
            write_descriptor(descriptor, bridge.server_port)
            if stat.S_IMODE(descriptor.stat().st_mode) != 0o600:
                raise RuntimeError("synthetic bridge descriptor is not owner-only")
            report = root / "qt-failure.json"
            environment["HCB_BRIDGE_FAILURE_ACCEPTANCE_FILE"] = str(report)
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
                env=environment,
                start_new_session=True,
            )
            report_payload = wait_for_report(report, process)
            if (
                report_payload.get("ok") is not True
                or report_payload.get("stage") != 9
                or any(
                    report_payload.get(key) is not True
                    for key in (
                        "polling_was_cancellable",
                        "cancellation_completed",
                        "mutation_failures_preserved_models",
                        "malformed_operation_rejected",
                        "malformed_start_rejected",
                        "shutdown_with_in_flight_poll",
                    )
                )
            ):
                raise RuntimeError(
                    f"Qt failure acceptance did not complete every stage: {report_payload}"
                )
            with bridge.lock:
                if bridge.unexpected_requests:
                    raise RuntimeError(
                        "synthetic bridge received unexpected requests: "
                        + ", ".join(bridge.unexpected_requests)
                    )
            return {
                "polling_was_cancellable": True,
                "cancellation_completed": True,
                "mutation_failures_preserved_models": True,
                "malformed_operation_rejected": True,
                "malformed_start_rejected": True,
                "shutdown_with_in_flight_poll": True,
            }
        finally:
            if process is not None:
                stop_process(process)
            bridge.shutdown()
            bridge.server_close()
            server_thread.join(timeout=5)


def main() -> int:
    args = parse_args()
    result = run_acceptance(args.native.resolve())
    print(json.dumps({"qt_bridge_failure_acceptance": result}, sort_keys=True))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
