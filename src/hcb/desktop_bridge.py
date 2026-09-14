"""Versioned, loopback-only bridge for native desktop frontends.

The bridge is deliberately small: it is a local client of the Python core, not
another source of account state. Each request gets an independent ``Runtime``
and SQLite connection, so it can coexist with the CLI, TUI, reminder process,
and a sync worker without sharing a connection across threads.
"""

from __future__ import annotations

import hmac
import json
import os
import secrets
import signal
import threading
from collections.abc import Callable
from contextlib import suppress
from dataclasses import dataclass, field
from datetime import date, datetime
from http import HTTPStatus
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path
from time import monotonic
from typing import Any, Final, Literal
from urllib.parse import parse_qs, unquote, urlsplit

from .application import ApplicationService
from .errors import (
    AuthenticationRequired,
    ConfigurationError,
    ConflictError,
    HcbError,
    NotFoundError,
    OfflineError,
    StorageError,
)
from .models import DateTimeKind, EventDateTime, TaskPriority
from .output import to_primitive
from .runtime import Runtime

BRIDGE_API_VERSION: Final = 1
BRIDGE_NAME: Final = "hcb-desktop-bridge"
MAX_JSON_BODY_BYTES: Final = 1_048_576
MAX_OPERATION_PROGRESS_ITEMS: Final = 32

Json = dict[str, Any]
OperationKind = Literal["oauth", "sync"]
OperationState = Literal["queued", "running", "succeeded", "failed", "cancelled"]


@dataclass(frozen=True, slots=True)
class BridgeDescriptor:
    """Connection data written only to a caller-selected owner-only file."""

    api_version: int
    url: str
    token: str


@dataclass(slots=True)
class _Operation:
    id: str
    kind: OperationKind
    account_id: str
    created_monotonic: float = field(default_factory=monotonic)
    state: OperationState = "queued"
    cancellation_requested: bool = False
    progress: list[str] = field(default_factory=list)
    result: Json | None = None
    error: Json | None = None
    _cancelled: threading.Event = field(default_factory=threading.Event, repr=False)
    _lock: threading.RLock = field(default_factory=threading.RLock, repr=False)

    def add_progress(self, message: str) -> None:
        with self._lock:
            if len(self.progress) == MAX_OPERATION_PROGRESS_ITEMS:
                self.progress.pop(0)
            self.progress.append(message[:512])

    def request_cancellation(self) -> None:
        with self._lock:
            if self.state in {"queued", "running"}:
                self.cancellation_requested = True
                self._cancelled.set()

    def is_cancelled(self) -> bool:
        return self._cancelled.is_set()

    def snapshot(self) -> Json:
        with self._lock:
            result: Json = {
                "id": self.id,
                "kind": self.kind,
                "account_id": self.account_id,
                "state": self.state,
                "cancellation_requested": self.cancellation_requested,
                "progress": list(self.progress),
                "elapsed_ms": int((monotonic() - self.created_monotonic) * 1_000),
            }
            if self.result is not None:
                result["result"] = self.result
            if self.error is not None:
                result["error"] = self.error
            return result


class BridgeOperations:
    """Bounded in-memory operation registry for one local bridge process."""

    def __init__(self, runtime_factory: Callable[[], Runtime]) -> None:
        self._runtime_factory = runtime_factory
        self._operations: dict[str, _Operation] = {}
        self._lock = threading.RLock()

    def get(self, operation_id: str) -> _Operation | None:
        with self._lock:
            return self._operations.get(operation_id)

    def start_sync(self, account_id: str) -> _Operation:
        def run(runtime: Runtime, operation: _Operation) -> Json:
            result = runtime.sync_account(
                account_id,
                progress=operation.add_progress,
                cancelled=operation.is_cancelled,
                cancel_hint="Cancel from the desktop app.",
            )
            return {"sync": to_primitive(result)}

        return self._start("sync", account_id, run)

    def start_oauth(self, account_id: str, expected_email: str) -> _Operation:
        def run(runtime: Runtime, operation: _Operation) -> Json:
            operation.add_progress("waiting for browser authorization")
            result = runtime.connect_account(account_id, expected_email=expected_email)
            # OAuthResult contains access and refresh tokens. Never cross the bridge.
            return {"connected": True, "granted_scopes": list(result.granted_scopes)}

        return self._start("oauth", account_id, run)

    def _start(
        self,
        kind: OperationKind,
        account_id: str,
        action: Callable[[Runtime, _Operation], Json],
    ) -> _Operation:
        operation = _Operation(secrets.token_urlsafe(18), kind, account_id)
        with self._lock:
            self._operations[operation.id] = operation
        worker = threading.Thread(
            target=self._run,
            args=(operation, action),
            name=f"hcb-bridge-{kind}-{operation.id[:8]}",
            daemon=True,
        )
        worker.start()
        return operation

    def _run(self, operation: _Operation, action: Callable[[Runtime, _Operation], Json]) -> None:
        with operation._lock:
            if operation.is_cancelled():
                operation.state = "cancelled"
                return
            operation.state = "running"
        runtime = self._runtime_factory()
        try:
            result = action(runtime, operation)
        except Exception as exc:  # convert errors to the stable operation contract
            with operation._lock:
                operation.state = "failed"
                operation.error = _error_payload(exc)
        else:
            with operation._lock:
                sync = result.get("sync")
                if operation.is_cancelled() or (
                    isinstance(sync, dict) and sync.get("cancelled") is True
                ):
                    operation.state = "cancelled"
                else:
                    operation.state = "succeeded"
                operation.result = result
        finally:
            runtime.close()


class DesktopBridge:
    """Own a loopback HTTP server and its short-lived connection descriptor."""

    def __init__(
        self,
        runtime_factory: Callable[[], Runtime] = Runtime,
        *,
        token: str | None = None,
        port: int = 0,
    ) -> None:
        if not 0 <= port <= 65_535:
            raise ValueError("bridge port must be between 0 and 65535")
        self._runtime_factory = runtime_factory
        self._token = token or secrets.token_urlsafe(32)
        if not self._token:
            raise ValueError("bridge token must not be empty")
        self.operations = BridgeOperations(runtime_factory)
        handler = _handler_type(self)
        self._server = ThreadingHTTPServer(("127.0.0.1", port), handler)
        self._server.daemon_threads = True
        self._ready_file: Path | None = None

    @property
    def descriptor(self) -> BridgeDescriptor:
        host, port = self._server.server_address[:2]
        return BridgeDescriptor(BRIDGE_API_VERSION, f"http://{host}:{port}", self._token)

    def serve_forever(self) -> None:
        self._server.serve_forever(poll_interval=0.2)

    def shutdown(self) -> None:
        self._server.shutdown()

    def close(self) -> None:
        self._server.server_close()
        if self._ready_file is not None:
            with suppress(OSError):
                self._ready_file.unlink(missing_ok=True)
            self._ready_file = None

    def write_descriptor(self, path: Path) -> None:
        """Atomically claim and write a private descriptor path supplied by a launcher."""
        if self._ready_file is not None:
            raise RuntimeError("bridge descriptor was already written")
        payload = json.dumps(
            to_primitive(self.descriptor), sort_keys=True, separators=(",", ":")
        ).encode()
        try:
            descriptor_fd = os.open(
                path, os.O_WRONLY | os.O_CREAT | os.O_EXCL | os.O_CLOEXEC, 0o600
            )
        except FileExistsError as exc:
            raise ValueError(f"bridge descriptor already exists: {path}") from exc
        except OSError as exc:
            raise ValueError(f"cannot create bridge descriptor: {exc}") from exc
        with os.fdopen(descriptor_fd, "wb") as descriptor:
            descriptor.write(payload)
            descriptor.flush()
        self._ready_file = path


def serve_from_cli(*, ready_file: Path, port: int = 0) -> None:
    """Run the process form used by a native helper launcher."""
    bridge = DesktopBridge(port=port)
    bridge.write_descriptor(ready_file)

    def stop(_signum: int, _frame: object) -> None:
        # BaseServer.shutdown() cannot run in the serve_forever thread.
        threading.Thread(target=bridge.shutdown, daemon=True).start()

    previous_int = signal.signal(signal.SIGINT, stop)
    previous_term = signal.signal(signal.SIGTERM, stop)
    try:
        bridge.serve_forever()
    finally:
        signal.signal(signal.SIGINT, previous_int)
        signal.signal(signal.SIGTERM, previous_term)
        bridge.close()


def _handler_type(bridge: DesktopBridge) -> type[BaseHTTPRequestHandler]:
    class BridgeHandler(BaseHTTPRequestHandler):
        protocol_version = "HTTP/1.1"

        def do_GET(self) -> None:  # noqa: N802
            self._dispatch("GET")

        def do_POST(self) -> None:  # noqa: N802
            self._dispatch("POST")

        def do_PATCH(self) -> None:  # noqa: N802
            self._dispatch("PATCH")

        def do_DELETE(self) -> None:  # noqa: N802
            self._dispatch("DELETE")

        def log_message(self, _format: str, *_args: object) -> None:
            # Request URLs can contain private account IDs. Do not log them.
            return

        def _dispatch(self, method: str) -> None:
            if not _authorized(self.headers.get("Authorization"), bridge.descriptor.token):
                self._write(
                    HTTPStatus.UNAUTHORIZED,
                    error={"code": "unauthorized", "message": "missing or invalid bridge token"},
                )
                return
            try:
                parsed = urlsplit(self.path)
                path = tuple(unquote(part) for part in parsed.path.split("/") if part)
                query = parse_qs(parsed.query, keep_blank_values=True, strict_parsing=True)
                body = self._body() if method in {"POST", "PATCH"} else {}
                status, data = self._route(method, path, query, body)
            except Exception as exc:
                status, error = _http_error(exc)
                self._write(status, error=error)
                return
            self._write(status, data=data)

        def _body(self) -> Json:
            content_type = self.headers.get("Content-Type", "")
            if not content_type.casefold().startswith("application/json"):
                raise ValueError("Content-Type must be application/json")
            length_text = self.headers.get("Content-Length")
            if length_text is None:
                raise ValueError("Content-Length is required")
            try:
                length = int(length_text)
            except ValueError as exc:
                raise ValueError("Content-Length must be an integer") from exc
            if not 0 <= length <= MAX_JSON_BODY_BYTES:
                raise ValueError(f"JSON body must be at most {MAX_JSON_BODY_BYTES} bytes")
            try:
                parsed = json.loads(self.rfile.read(length))
            except (UnicodeDecodeError, json.JSONDecodeError) as exc:
                raise ValueError("request body must be valid JSON") from exc
            if not isinstance(parsed, dict):
                raise ValueError("request body must be a JSON object")
            return parsed

        def _route(
            self, method: str, path: tuple[str, ...], query: dict[str, list[str]], body: Json
        ) -> tuple[HTTPStatus, Json]:
            if method == "GET" and path == ("v1", "health"):
                _no_query(query)
                return HTTPStatus.OK, {"service": BRIDGE_NAME, "api_version": BRIDGE_API_VERSION}
            if method == "GET" and path == ("v1", "accounts"):
                _no_query(query)
                return HTTPStatus.OK, {
                    "accounts": self._with_application(lambda app: app.list_accounts())
                }
            if len(path) >= 3 and path[:2] == ("v1", "accounts"):
                account_id = _account_id(path[2])
                return self._account_route(method, account_id, path[3:], query, body)
            if len(path) == 3 and path[:2] == ("v1", "operations"):
                _no_query(query)
                operation = bridge.operations.get(path[2])
                if operation is None:
                    raise NotFoundError(f"Operation {path[2]!r} does not exist")
                if method == "GET":
                    return HTTPStatus.OK, {"operation": operation.snapshot()}
                if method == "DELETE":
                    operation.request_cancellation()
                    return HTTPStatus.ACCEPTED, {"operation": operation.snapshot()}
            raise NotFoundError("bridge route does not exist")

        def _account_route(
            self,
            method: str,
            account_id: str,
            tail: tuple[str, ...],
            query: dict[str, list[str]],
            body: Json,
        ) -> tuple[HTTPStatus, Json]:
            if method == "GET" and tail == ("workspace",):
                return HTTPStatus.OK, {"workspace": self._workspace(account_id, query)}
            if method == "GET" and tail == ("search",):
                return HTTPStatus.OK, {"results": self._search(account_id, query)}
            if method == "POST" and tail == ("sync",):
                _empty_body(body)
                _no_query(query)
                operation = bridge.operations.start_sync(account_id)
                return HTTPStatus.ACCEPTED, {"operation": operation.snapshot()}
            if method == "POST" and tail == ("oauth",):
                _no_query(query)
                _keys(body, required={"expected_email"})
                email = _string(body["expected_email"], "expected_email")
                operation = bridge.operations.start_oauth(account_id, email)
                return HTTPStatus.ACCEPTED, {"operation": operation.snapshot()}
            if method == "POST" and tail == ("tasks",):
                _no_query(query)
                return HTTPStatus.CREATED, {"task": self._create_task(account_id, body)}
            if len(tail) == 2 and tail[0] == "tasks":
                task_id = tail[1]
                if method == "PATCH":
                    _no_query(query)
                    return HTTPStatus.OK, {"task": self._update_task(account_id, task_id, body)}
                if method == "DELETE":
                    _empty_body(body)
                    _no_query(query)
                    return HTTPStatus.OK, {
                        "task": self._with_application(
                            lambda app: app.delete_task(account_id, task_id)
                        )
                    }
            if method == "POST" and len(tail) == 3 and tail[0] == "tasks" and tail[2] == "complete":
                _no_query(query)
                _keys(body, optional={"completed"})
                completed = _bool(body["completed"], "completed") if "completed" in body else True
                return HTTPStatus.OK, {
                    "task": self._with_application(
                        lambda app: app.complete_task(account_id, tail[1], completed=completed)
                    )
                }
            if method == "POST" and tail == ("events",):
                _no_query(query)
                return HTTPStatus.CREATED, {"event": self._create_event(account_id, body)}
            if len(tail) == 2 and tail[0] == "events":
                event_id = tail[1]
                if method == "PATCH":
                    _no_query(query)
                    return HTTPStatus.OK, {"event": self._update_event(account_id, event_id, body)}
                if method == "DELETE":
                    _empty_body(body)
                    _no_query(query)
                    return HTTPStatus.OK, {
                        "event": self._with_application(
                            lambda app: app.delete_event(account_id, event_id)
                        )
                    }
            raise NotFoundError("bridge route does not exist")

        def _workspace(self, account_id: str, query: dict[str, list[str]]) -> Json:
            allowed = {"include", "start", "end", "calendar_id"}
            _query_keys(query, allowed)
            include = _include(query)
            if "events" in include and ("start" not in query or "end" not in query):
                raise ValueError("event workspace slices require start and end query parameters")

            def read(app: ApplicationService) -> Json:
                summary = to_primitive(app.workspace_summary(account_id))
                assert isinstance(summary, dict)
                if "tasks" in include:
                    summary["tasks"] = to_primitive(app.task_listing(account_id))
                if "events" in include:
                    start = _range_value(query["start"])
                    end = _range_value(query["end"])
                    if end <= start:
                        raise ValueError("event workspace end must be after start")
                    calendar_id = _single_query(query, "calendar_id")
                    summary["events"] = to_primitive(
                        app.agenda_events(account_id, start=start, end=end, calendar_id=calendar_id)
                    )
                return summary

            return self._with_application(read)

        def _search(self, account_id: str, query: dict[str, list[str]]) -> Any:
            _query_keys(query, {"q", "limit"})
            text = _single_query(query, "q")
            if text is None:
                raise ValueError("search requires q")
            limit = _query_int(query, "limit", default=50, minimum=1, maximum=200)
            return self._with_application(lambda app: app.search(account_id, text, limit=limit))

        def _create_task(self, account_id: str, body: Json) -> Any:
            _keys(
                body,
                required={"list_id", "title"},
                optional={"notes", "due", "due_time_zone", "priority", "parent_id", "position"},
            )
            return self._with_application(
                lambda app: app.create_task(
                    account_id,
                    _string(body["list_id"], "list_id"),
                    _string(body["title"], "title"),
                    notes=_nullable_string(body.get("notes"), "notes"),
                    due=_date_value(body.get("due"), "due"),
                    due_time_zone=_nullable_string(body.get("due_time_zone"), "due_time_zone"),
                    priority=_string(body.get("priority", TaskPriority.NONE.value), "priority"),
                    parent_id=_nullable_string(body.get("parent_id"), "parent_id"),
                    position=_nullable_string(body.get("position"), "position"),
                )
            )

        def _update_task(self, account_id: str, task_id: str, body: Json) -> Any:
            _keys(body, optional={"title", "notes", "due", "priority"})
            if not body:
                raise ValueError("task update requires at least one field")
            kwargs: dict[str, Any] = {}
            if "title" in body:
                kwargs["title"] = _string(body["title"], "title")
            if "notes" in body:
                kwargs["notes"] = _nullable_string(body["notes"], "notes")
            if "due" in body:
                if body["due"] is None:
                    kwargs["clear_due"] = True
                else:
                    kwargs["due"] = _date_value(body["due"], "due")
            if "priority" in body:
                kwargs["priority"] = _string(body["priority"], "priority")
            return self._with_application(
                lambda app: app.update_task(account_id, task_id, **kwargs)
            )

        def _create_event(self, account_id: str, body: Json) -> Any:
            _keys(
                body,
                required={"calendar_id", "summary", "start", "end"},
                optional={"description", "location", "recurrence"},
            )
            recurrence = _string_list(body.get("recurrence", []), "recurrence")
            return self._with_application(
                lambda app: app.create_event(
                    account_id,
                    _string(body["calendar_id"], "calendar_id"),
                    _string(body["summary"], "summary"),
                    _event_time(body["start"], "start"),
                    _event_time(body["end"], "end"),
                    description=_nullable_string(body.get("description"), "description"),
                    location=_nullable_string(body.get("location"), "location"),
                    recurrence=tuple(recurrence),
                )
            )

        def _update_event(self, account_id: str, event_id: str, body: Json) -> Any:
            _keys(body, optional={"summary", "start", "end", "description", "location"})
            if not body:
                raise ValueError("event update requires at least one field")
            kwargs: dict[str, Any] = {}
            if "summary" in body:
                kwargs["summary"] = _string(body["summary"], "summary")
            if "start" in body:
                kwargs["start"] = _event_time(body["start"], "start")
            if "end" in body:
                kwargs["end"] = _event_time(body["end"], "end")
            if "description" in body:
                kwargs["description"] = _nullable_string(body["description"], "description")
            if "location" in body:
                kwargs["location"] = _nullable_string(body["location"], "location")
            return self._with_application(
                lambda app: app.update_event(account_id, event_id, **kwargs)
            )

        def _with_application(self, action: Callable[[ApplicationService], Any]) -> Any:
            runtime = bridge._runtime_factory()
            try:
                return action(runtime.application)
            finally:
                runtime.close()

        def _write(
            self, status: HTTPStatus, *, data: Any | None = None, error: Json | None = None
        ) -> None:
            envelope: Json = {"api_version": BRIDGE_API_VERSION}
            if error is None:
                envelope["data"] = to_primitive(data)
            else:
                envelope["error"] = error
            encoded = json.dumps(envelope, ensure_ascii=False, separators=(",", ":")).encode(
                "utf-8"
            )
            self.send_response(status.value)
            self.send_header("Content-Type", "application/json; charset=utf-8")
            self.send_header("Cache-Control", "no-store")
            self.send_header("X-Content-Type-Options", "nosniff")
            self.send_header("Content-Length", str(len(encoded)))
            self.end_headers()
            self.wfile.write(encoded)

    return BridgeHandler


def _authorized(header: str | None, token: str) -> bool:
    return header is not None and hmac.compare_digest(header, f"Bearer {token}")


def _http_error(exc: Exception) -> tuple[HTTPStatus, Json]:
    payload = _error_payload(exc)
    status = {
        "not_found": HTTPStatus.NOT_FOUND,
        "conflict": HTTPStatus.CONFLICT,
        "authentication_required": HTTPStatus.UNAUTHORIZED,
        "offline": HTTPStatus.SERVICE_UNAVAILABLE,
        "storage_failure": HTTPStatus.SERVICE_UNAVAILABLE,
        "configuration_error": HTTPStatus.BAD_REQUEST,
        "invalid_request": HTTPStatus.BAD_REQUEST,
    }.get(str(payload["code"]), HTTPStatus.INTERNAL_SERVER_ERROR)
    return status, payload


def _error_payload(exc: Exception) -> Json:
    if isinstance(exc, NotFoundError):
        code = "not_found"
    elif isinstance(exc, ConflictError):
        code = "conflict"
    elif isinstance(exc, AuthenticationRequired):
        code = "authentication_required"
    elif isinstance(exc, OfflineError):
        code = "offline"
    elif isinstance(exc, StorageError):
        code = "storage_failure"
    elif isinstance(exc, ConfigurationError):
        code = "configuration_error"
    elif isinstance(exc, (TypeError, ValueError)):
        code = "invalid_request"
    elif isinstance(exc, HcbError):
        code = "hcb_error"
    else:
        code = "internal_error"
    message = exc.message if isinstance(exc, HcbError) else str(exc)
    result: Json = {"code": code, "message": message or code}
    if isinstance(exc, HcbError) and exc.hint:
        result["hint"] = exc.hint
    return result


def _account_id(value: str) -> str:
    if not value or len(value) > 256 or any(character.isspace() for character in value):
        raise ValueError("account id is invalid")
    return value


def _keys(
    body: Json,
    *,
    required: set[str] | None = None,
    optional: set[str] | None = None,
) -> None:
    required = required or set()
    optional = optional or set()
    missing = required.difference(body)
    unknown = set(body).difference(required | optional)
    if missing:
        raise ValueError(f"request body is missing: {', '.join(sorted(missing))}")
    if unknown:
        raise ValueError(f"request body has unknown fields: {', '.join(sorted(unknown))}")


def _empty_body(body: Json) -> None:
    _keys(body)


def _string(value: Any, name: str) -> str:
    if not isinstance(value, str) or not value:
        raise ValueError(f"{name} must be a non-empty string")
    return value


def _nullable_string(value: Any, name: str) -> str | None:
    if value is None:
        return None
    return _string(value, name)


def _bool(value: Any, name: str) -> bool:
    if type(value) is not bool:
        raise ValueError(f"{name} must be a boolean")
    return value


def _date_value(value: Any, name: str) -> date | None:
    if value is None:
        return None
    if not isinstance(value, str):
        raise ValueError(f"{name} must be an ISO date")
    try:
        return date.fromisoformat(value)
    except ValueError as exc:
        raise ValueError(f"{name} must be an ISO date") from exc


def _event_time(value: Any, name: str) -> EventDateTime:
    if not isinstance(value, dict):
        raise ValueError(f"{name} must be an event date/time object")
    _keys(value, required={"kind", "value"}, optional={"time_zone"})
    kind = _string(value["kind"], f"{name}.kind")
    raw = _string(value["value"], f"{name}.value")
    time_zone = _nullable_string(value.get("time_zone"), f"{name}.time_zone")
    if kind == DateTimeKind.DATE.value:
        parsed = _date_value(raw, f"{name}.value")
        assert parsed is not None
        return EventDateTime(DateTimeKind.DATE, parsed)
    if kind != DateTimeKind.DATETIME.value:
        raise ValueError(f"{name}.kind must be date or dateTime")
    try:
        parsed_datetime = datetime.fromisoformat(raw.replace("Z", "+00:00"))
    except ValueError as exc:
        raise ValueError(f"{name}.value must be an ISO date-time") from exc
    if parsed_datetime.tzinfo is None:
        raise ValueError(f"{name}.value must include an explicit UTC offset")
    return EventDateTime(DateTimeKind.DATETIME, parsed_datetime, time_zone)


def _string_list(value: Any, name: str) -> list[str]:
    if not isinstance(value, list) or not all(isinstance(item, str) for item in value):
        raise ValueError(f"{name} must be an array of strings")
    return value


def _query_keys(query: dict[str, list[str]], allowed: set[str]) -> None:
    unknown = set(query).difference(allowed)
    if unknown:
        raise ValueError(f"query has unknown fields: {', '.join(sorted(unknown))}")
    if any(len(values) != 1 for values in query.values()):
        raise ValueError("query fields may appear only once")


def _no_query(query: dict[str, list[str]]) -> None:
    _query_keys(query, set())


def _single_query(query: dict[str, list[str]], name: str) -> str | None:
    values = query.get(name)
    return values[0] if values else None


def _query_int(
    query: dict[str, list[str]], name: str, *, default: int, minimum: int, maximum: int
) -> int:
    value = _single_query(query, name)
    if value is None:
        return default
    try:
        parsed = int(value)
    except ValueError as exc:
        raise ValueError(f"{name} must be an integer") from exc
    if not minimum <= parsed <= maximum:
        raise ValueError(f"{name} must be between {minimum} and {maximum}")
    return parsed


def _include(query: dict[str, list[str]]) -> set[str]:
    raw = _single_query(query, "include")
    if raw is None or raw == "":
        return set()
    values = set(raw.split(","))
    allowed = {"tasks", "events"}
    if not values.issubset(allowed) or "" in values:
        raise ValueError("include may contain only tasks and events")
    return values


def _range_value(values: list[str]) -> date | datetime:
    raw = values[0]
    try:
        return date.fromisoformat(raw)
    except ValueError:
        try:
            parsed = datetime.fromisoformat(raw.replace("Z", "+00:00"))
        except ValueError as exc:
            raise ValueError("event range values must be ISO dates or date-times") from exc
        if parsed.tzinfo is None:
            raise ValueError("event range date-times must include an explicit UTC offset") from None
        return parsed
