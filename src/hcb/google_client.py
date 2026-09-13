"""Typed boundary around the Google Tasks, Calendar, and Drive APIs."""

from __future__ import annotations

import json
import socket
from collections.abc import Mapping
from contextlib import suppress
from dataclasses import dataclass
from datetime import UTC, datetime
from email.utils import parsedate_to_datetime
from threading import current_thread
from typing import Any, Protocol, cast

from .errors import GoogleApiError, RequestNotSentError, TransientTransportError

Json = dict[str, Any]


@dataclass(frozen=True, slots=True)
class Page:
    items: tuple[Json, ...]
    next_page_token: str | None = None
    next_sync_token: str | None = None


class GoogleGateway(Protocol):
    """Network contract used by sync; fakes can implement it without Google packages."""

    def list_task_lists(self, *, page_token: str | None = None) -> Page: ...
    def list_tasks(
        self, task_list_id: str, *, page_token: str | None = None, updated_min: str | None = None
    ) -> Page: ...
    def list_calendars(
        self, *, page_token: str | None = None, sync_token: str | None = None
    ) -> Page: ...
    def list_events(
        self,
        calendar_id: str,
        *,
        page_token: str | None = None,
        sync_token: str | None = None,
        time_min: str | None = None,
        time_max: str | None = None,
        single_events: bool = False,
    ) -> Page: ...

    def create_task_list(self, body: Json) -> Json: ...
    def update_task_list(
        self, task_list_id: str, body: Json, *, etag: str | None = None
    ) -> Json: ...
    def delete_task_list(self, task_list_id: str, *, etag: str | None = None) -> None: ...
    def create_task(
        self,
        task_list_id: str,
        body: Json,
        *,
        parent: str | None = None,
        previous: str | None = None,
    ) -> Json: ...
    def update_task(
        self, task_list_id: str, task_id: str, body: Json, *, etag: str | None = None
    ) -> Json: ...
    def delete_task(self, task_list_id: str, task_id: str, *, etag: str | None = None) -> None: ...
    def move_task(
        self,
        task_list_id: str,
        task_id: str,
        *,
        destination_task_list_id: str | None = None,
        parent: str | None = None,
        previous: str | None = None,
    ) -> Json: ...

    def create_event(
        self,
        calendar_id: str,
        body: Json,
        *,
        send_updates: str = "none",
        supports_attachments: bool = False,
        conference_data_version: int = 0,
    ) -> Json: ...
    def update_event(
        self,
        calendar_id: str,
        event_id: str,
        body: Json,
        *,
        etag: str | None = None,
        send_updates: str = "none",
        supports_attachments: bool = False,
        conference_data_version: int = 0,
    ) -> Json: ...
    def delete_event(
        self,
        calendar_id: str,
        event_id: str,
        *,
        etag: str | None = None,
        send_updates: str = "none",
    ) -> None: ...
    def move_event(self, calendar_id: str, event_id: str, destination: str) -> Json: ...
    def respond_event(
        self,
        calendar_id: str,
        event_id: str,
        response_status: str,
        *,
        etag: str | None = None,
        comment: str | None = None,
        send_updates: str = "all",
    ) -> Json: ...

    def create_calendar(self, body: Json) -> Json: ...
    def subscribe_calendar(self, calendar_id: str) -> Json: ...
    def remove_calendar(self, calendar_id: str) -> None: ...
    def update_calendar(self, calendar_id: str, body: Json, *, etag: str | None = None) -> Json: ...
    def update_calendar_list(
        self, calendar_id: str, body: Json, *, etag: str | None = None
    ) -> Json: ...
    def delete_calendar(self, calendar_id: str, *, etag: str | None = None) -> None: ...
    def calendar_colors(self) -> Json: ...
    def freebusy(self, body: Json) -> Json: ...
    def drive_metadata(self, file_id: str, *, fields: str = "*") -> Json: ...
    def search_drive_metadata(
        self, query: str, *, page_token: str | None = None, page_size: int = 100
    ) -> Page: ...


def _error_from_http(exc: BaseException) -> GoogleApiError:
    response = getattr(exc, "resp", None)
    status = int(getattr(response, "status", 0) or 0)
    retry_after: float | None = None
    headers = response if isinstance(response, Mapping) else {}
    raw_retry = headers.get("retry-after") or headers.get("Retry-After")
    if raw_retry is not None:
        with suppress(TypeError, ValueError):
            retry_after = float(raw_retry)
        if retry_after is None and isinstance(raw_retry, str):
            with suppress(TypeError, ValueError, IndexError):
                retry_at = parsedate_to_datetime(raw_retry)
                if retry_at.tzinfo is None:
                    retry_at = retry_at.replace(tzinfo=UTC)
                retry_after = max(0.0, (retry_at - datetime.now(UTC)).total_seconds())
    reason: str | None = None
    message = f"Google API request failed ({status or 'unknown status'})"
    content = getattr(exc, "content", b"")
    try:
        payload = json.loads(content.decode() if isinstance(content, bytes) else content)
        error = payload.get("error", payload)
        message = str(error.get("message") or message)
        errors = error.get("errors") or ()
        if errors:
            reason = errors[0].get("reason")
        reason = reason or error.get("status")
    except (AttributeError, TypeError, ValueError, json.JSONDecodeError):
        pass
    return GoogleApiError(status, message, reason=reason, retry_after=retry_after)


class GoogleApiClient:
    """google-api-python-client adapter. Construction itself performs no network I/O."""

    def __init__(
        self,
        credentials: Any | None = None,
        *,
        tasks_service: Any | None = None,
        calendar_service: Any | None = None,
        drive_service: Any | None = None,
    ) -> None:
        if tasks_service is None or calendar_service is None or drive_service is None:
            from googleapiclient.discovery import build  # type: ignore[import-untyped]

            tasks_service = tasks_service or build(
                "tasks", "v1", credentials=credentials, cache_discovery=False
            )
            calendar_service = calendar_service or build(
                "calendar", "v3", credentials=credentials, cache_discovery=False
            )
            drive_service = drive_service or build(
                "drive", "v3", credentials=credentials, cache_discovery=False
            )
        self.tasks = tasks_service
        self.calendar = calendar_service
        self.drive = drive_service
        self._credentials = credentials
        self._owner_thread = current_thread()
        self.parallel_reads_safe = credentials is not None

    @staticmethod
    def _execute(request: Any, *, etag: str | None = None, http: Any | None = None) -> Json:
        if etag:
            request.headers["If-Match"] = etag
        try:
            return cast(Json, request.execute(http=http) if http is not None else request.execute())
        except Exception as exc:
            if hasattr(exc, "resp"):
                raise _error_from_http(exc) from exc
            if isinstance(exc, (ConnectionRefusedError, socket.gaierror)):
                raise RequestNotSentError(
                    "Google request was not sent because the connection could not be established"
                ) from exc
            if isinstance(
                exc, (BrokenPipeError, ConnectionResetError, TimeoutError, socket.timeout)
            ):
                raise TransientTransportError(
                    "Google request was interrupted before a response was received"
                ) from exc
            raise

    def _read_execute(self, request: Any) -> Json:
        if current_thread() is self._owner_thread or self._credentials is None:
            return self._execute(request)
        from google_auth_httplib2 import AuthorizedHttp  # type: ignore[import-untyped]
        from googleapiclient.http import build_http  # type: ignore[import-untyped]

        http = AuthorizedHttp(self._credentials, http=build_http())
        try:
            return self._execute(request, http=http)
        finally:
            http.close()

    @classmethod
    def _void(cls, request: Any, *, etag: str | None = None) -> None:
        cls._execute(request, etag=etag)

    @staticmethod
    def _page(response: Json) -> Page:
        return Page(
            tuple(response.get("items", ())),
            response.get("nextPageToken"),
            response.get("nextSyncToken"),
        )

    def list_task_lists(self, *, page_token: str | None = None) -> Page:
        return self._page(self._read_execute(self.tasks.tasklists().list(pageToken=page_token)))

    def list_tasks(
        self, task_list_id: str, *, page_token: str | None = None, updated_min: str | None = None
    ) -> Page:
        request = self.tasks.tasks().list(
            tasklist=task_list_id,
            pageToken=page_token,
            updatedMin=updated_min,
            maxResults=100,
            showCompleted=True,
            showDeleted=True,
            showHidden=True,
        )
        return self._page(self._read_execute(request))

    def list_calendars(
        self, *, page_token: str | None = None, sync_token: str | None = None
    ) -> Page:
        return self._page(
            self._read_execute(
                self.calendar.calendarList().list(
                    pageToken=page_token,
                    syncToken=sync_token,
                    showDeleted=True,
                    showHidden=True,
                )
            )
        )

    def list_events(
        self,
        calendar_id: str,
        *,
        page_token: str | None = None,
        sync_token: str | None = None,
        time_min: str | None = None,
        time_max: str | None = None,
        single_events: bool = False,
    ) -> Page:
        if sync_token is not None and (time_min is not None or time_max is not None):
            raise ValueError("Google events syncToken cannot be combined with time bounds")
        response = self._read_execute(
            self.calendar.events().list(
                calendarId=calendar_id,
                pageToken=page_token,
                syncToken=sync_token,
                timeMin=time_min,
                timeMax=time_max,
                maxResults=1000,
                singleEvents=single_events,
                showDeleted=True,
            )
        )
        return self._page(response)

    def create_task_list(self, body: Json) -> Json:
        return self._execute(self.tasks.tasklists().insert(body=body))

    def update_task_list(self, task_list_id: str, body: Json, *, etag: str | None = None) -> Json:
        return self._execute(
            self.tasks.tasklists().patch(tasklist=task_list_id, body=body), etag=etag
        )

    def delete_task_list(self, task_list_id: str, *, etag: str | None = None) -> None:
        self._void(self.tasks.tasklists().delete(tasklist=task_list_id), etag=etag)

    def create_task(
        self,
        task_list_id: str,
        body: Json,
        *,
        parent: str | None = None,
        previous: str | None = None,
    ) -> Json:
        return self._execute(
            self.tasks.tasks().insert(
                tasklist=task_list_id, body=body, parent=parent, previous=previous
            )
        )

    def update_task(
        self, task_list_id: str, task_id: str, body: Json, *, etag: str | None = None
    ) -> Json:
        return self._execute(
            self.tasks.tasks().patch(tasklist=task_list_id, task=task_id, body=body), etag=etag
        )

    def delete_task(self, task_list_id: str, task_id: str, *, etag: str | None = None) -> None:
        self._void(self.tasks.tasks().delete(tasklist=task_list_id, task=task_id), etag=etag)

    def move_task(
        self,
        task_list_id: str,
        task_id: str,
        *,
        destination_task_list_id: str | None = None,
        parent: str | None = None,
        previous: str | None = None,
    ) -> Json:
        return self._execute(
            self.tasks.tasks().move(
                tasklist=task_list_id,
                task=task_id,
                destinationTasklist=destination_task_list_id,
                parent=parent,
                previous=previous,
            )
        )

    def create_event(
        self,
        calendar_id: str,
        body: Json,
        *,
        send_updates: str = "none",
        supports_attachments: bool = False,
        conference_data_version: int = 0,
    ) -> Json:
        return self._execute(
            self.calendar.events().insert(
                calendarId=calendar_id,
                body=body,
                sendUpdates=send_updates,
                supportsAttachments=supports_attachments,
                conferenceDataVersion=conference_data_version,
            )
        )

    def update_event(
        self,
        calendar_id: str,
        event_id: str,
        body: Json,
        *,
        etag: str | None = None,
        send_updates: str = "none",
        supports_attachments: bool = False,
        conference_data_version: int = 0,
    ) -> Json:
        return self._execute(
            self.calendar.events().patch(
                calendarId=calendar_id,
                eventId=event_id,
                body=body,
                sendUpdates=send_updates,
                supportsAttachments=supports_attachments,
                conferenceDataVersion=conference_data_version,
            ),
            etag=etag,
        )

    def delete_event(
        self,
        calendar_id: str,
        event_id: str,
        *,
        etag: str | None = None,
        send_updates: str = "none",
    ) -> None:
        self._void(
            self.calendar.events().delete(
                calendarId=calendar_id, eventId=event_id, sendUpdates=send_updates
            ),
            etag=etag,
        )

    def move_event(self, calendar_id: str, event_id: str, destination: str) -> Json:
        return self._execute(
            self.calendar.events().move(
                calendarId=calendar_id,
                eventId=event_id,
                destination=destination,
            )
        )

    def respond_event(
        self,
        calendar_id: str,
        event_id: str,
        response_status: str,
        *,
        etag: str | None = None,
        comment: str | None = None,
        send_updates: str = "all",
    ) -> Json:
        attendee: Json = {"self": True, "responseStatus": response_status}
        if comment is not None:
            attendee["comment"] = comment
        body = {"attendees": [attendee]}
        return self.update_event(calendar_id, event_id, body, etag=etag, send_updates=send_updates)

    def create_calendar(self, body: Json) -> Json:
        return self._execute(self.calendar.calendars().insert(body=body))

    def subscribe_calendar(self, calendar_id: str) -> Json:
        return self._execute(self.calendar.calendarList().insert(body={"id": calendar_id}))

    def remove_calendar(self, calendar_id: str) -> None:
        self._void(self.calendar.calendarList().delete(calendarId=calendar_id))

    def update_calendar(self, calendar_id: str, body: Json, *, etag: str | None = None) -> Json:
        return self._execute(
            self.calendar.calendars().patch(calendarId=calendar_id, body=body), etag=etag
        )

    def update_calendar_list(
        self, calendar_id: str, body: Json, *, etag: str | None = None
    ) -> Json:
        return self._execute(
            self.calendar.calendarList().patch(calendarId=calendar_id, body=body), etag=etag
        )

    def delete_calendar(self, calendar_id: str, *, etag: str | None = None) -> None:
        self._void(self.calendar.calendars().delete(calendarId=calendar_id), etag=etag)

    def calendar_colors(self) -> Json:
        return self._execute(self.calendar.colors().get())

    def freebusy(self, body: Json) -> Json:
        return self._execute(self.calendar.freebusy().query(body=body))

    def drive_metadata(self, file_id: str, *, fields: str = "*") -> Json:
        return self._execute(self.drive.files().get(fileId=file_id, fields=fields))

    def search_drive_metadata(
        self, query: str, *, page_token: str | None = None, page_size: int = 100
    ) -> Page:
        if not query.strip():
            raise ValueError("Drive metadata search query is required")
        escaped = query.replace("\\", "\\\\").replace("'", "\\'")
        response = self._execute(
            self.drive.files().list(
                q=f"name contains '{escaped}' and trashed = false",
                fields=("nextPageToken,files(id,name,mimeType,webViewLink,iconLink,modifiedTime)"),
                pageToken=page_token,
                pageSize=max(1, min(page_size, 1000)),
                orderBy="modifiedTime desc",
            )
        )
        return Page(tuple(response.get("files", ())), response.get("nextPageToken"))


def rfc3339(value: datetime) -> str:
    """Google accepts ISO 8601; normalize UTC's suffix for stable requests."""
    return value.isoformat().replace("+00:00", "Z")
