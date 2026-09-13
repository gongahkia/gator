"""UI-independent, optimistic application services.

All provider-backed writes in this module update the local projection and append
an outbox record inside one SQLite transaction.
"""

from __future__ import annotations

import json
from dataclasses import dataclass, replace
from datetime import UTC, datetime
from typing import Any, Final, Literal
from uuid import uuid4
from zoneinfo import ZoneInfo, ZoneInfoNotFoundError

from .errors import NotFoundError
from .models import (
    Account,
    Calendar,
    Conflict,
    DateTimeKind,
    DriveFile,
    EntityType,
    Event,
    EventDateTime,
    Metadata,
    MutationOperation,
    NotesProjection,
    PendingMutation,
    Task,
    TaskList,
    utc_now,
)
from .storage import Storage

Json = dict[str, Any]
ResponseStatus = Literal["accepted", "declined", "tentative", "needsAction"]


class _Unset:
    pass


_UNSET: Final = _Unset()


@dataclass(frozen=True, slots=True)
class SavedSearch:
    id: str
    account_id: str
    name: str
    query: str
    created_at: datetime


@dataclass(frozen=True, slots=True)
class TaskEventLink:
    account_id: str
    task_id: str
    event_id: str
    created_at: datetime
    orphaned: bool = False


@dataclass(frozen=True, slots=True)
class SearchResult:
    kind: Literal["task", "event", "calendar", "task-list", "drive", "saved-search", "conflict"]
    item: Task | Event | Calendar | TaskList | DriveFile | SavedSearch | Conflict
    score: int

    @property
    def display_title(self) -> str:
        if isinstance(self.item, (Task, TaskList)):
            return self.item.title
        if isinstance(self.item, Event | Calendar):
            return self.item.summary
        if isinstance(self.item, DriveFile):
            return self.item.name
        if isinstance(self.item, SavedSearch):
            return self.item.name
        return f"{self.item.entity_type.value.title()} conflict · {self.item.entity_id}"


@dataclass(frozen=True, slots=True)
class ImportApplyResult:
    tasks: tuple[Task, ...] = ()
    events: tuple[Event, ...] = ()


@dataclass(frozen=True, slots=True)
class WorkspaceSnapshot:
    account: Account
    tasks: tuple[Task, ...]
    events: tuple[Event, ...]
    task_lists: tuple[TaskList, ...]
    calendars: tuple[Calendar, ...]
    instance_ranges: tuple[dict[str, str], ...]
    pending: int


@dataclass(frozen=True, slots=True)
class TimeSlot:
    start: datetime
    end: datetime


@dataclass(frozen=True, slots=True)
class BatchMovePreview:
    """Validated local plan for a serial task or event move."""

    entity_type: Literal["task", "event"]
    destination_id: str
    items: tuple[Task | Event, ...]


@dataclass(frozen=True, slots=True)
class BatchActionPreview:
    """Validated local plan for a batch action before it changes the mirror."""

    entity_type: Literal["task", "event"]
    action: Literal["complete", "reopen", "delete", "respond"]
    items: tuple[Task | Event, ...]
    response_status: ResponseStatus | None = None


@dataclass(frozen=True, slots=True)
class TaskCompletionResult:
    """Local task completion records, including recurring successors when present."""

    tasks: tuple[Task, ...]
    successors: tuple[Task, ...] = ()


def _id() -> str:
    return uuid4().hex


def _dirty(metadata: Metadata, *, deleted: bool | None = None) -> Metadata:
    return replace(
        metadata,
        dirty=True,
        deleted=metadata.deleted if deleted is None else deleted,
        local_updated_at=utc_now(),
    )


def _event_point(value: EventDateTime) -> Json:
    if value.kind is DateTimeKind.DATE:
        return {"date": value.value.isoformat()}
    result: Json = {"dateTime": value.value.isoformat()}
    if value.time_zone:
        result["timeZone"] = value.time_zone
    return result


class _ApplicationServiceBase:
    """Optimistic local domain boundary used by every UI."""

    def __init__(
        self,
        storage: Storage,
        *,
        notes_projection: NotesProjection | str | None = None,
    ) -> None:
        self.storage = storage
        self._notes_projection = NotesProjection(notes_projection) if notes_projection else None

    def _account(self, account_id: str) -> None:
        if self.storage.get_account(account_id) is None:
            raise NotFoundError(f"Account {account_id!r} does not exist")

    def notes_projection(self, account_id: str) -> NotesProjection:
        if self._notes_projection is not None:
            return self._notes_projection
        row = self.storage.connection.execute(
            "SELECT value FROM app_settings WHERE account_id=? AND key='notes_projection'",
            (account_id,),
        ).fetchone()
        return NotesProjection(row["value"]) if row else NotesProjection.MIRRORED

    def set_notes_projection(
        self, account_id: str, projection: NotesProjection | str
    ) -> NotesProjection:
        self._account(account_id)
        value = NotesProjection(projection)
        with self.storage.transaction():
            self.storage.connection.execute(
                """INSERT INTO app_settings(account_id,key,value) VALUES (?,?,?)
                ON CONFLICT(account_id,key) DO UPDATE SET value=excluded.value""",
                (account_id, "notes_projection", value.value),
            )
        return value

    def _enqueue(
        self,
        account_id: str,
        entity_type: EntityType,
        entity_id: str,
        operation: MutationOperation,
        payload: Json,
    ) -> int:
        return self.storage.enqueue(
            PendingMutation(None, account_id, entity_type, entity_id, operation, payload)
        )

    def _snapshot(self, table: str, account_id: str, entity_id: str) -> Json | None:
        row = self.storage.connection.execute(
            f"SELECT * FROM {table} WHERE account_id=? AND id=?",  # noqa: S608
            (account_id, entity_id),
        ).fetchone()
        return dict(row) if row else None

    def _intent(
        self,
        account_id: str,
        action: str,
        entity_type: EntityType,
        entity_id: str,
        before: Json | None,
        after: Json | None,
    ) -> None:
        self.storage.connection.execute(
            "DELETE FROM intents WHERE account_id=? AND state='undone'", (account_id,)
        )
        self.storage.connection.execute(
            """INSERT INTO intents(account_id,action,entity_type,entity_id,before_payload,
            after_payload,state,created_at) VALUES (?,?,?,?,?,?,?,?)""",
            (
                account_id,
                action,
                entity_type.value,
                entity_id,
                json.dumps(before, sort_keys=True) if before is not None else None,
                json.dumps(after, sort_keys=True) if after is not None else None,
                "applied",
                utc_now().isoformat(),
            ),
        )

    @staticmethod
    def _task_body(task: Task, projection: NotesProjection) -> Json:
        body: Json = {"title": task.title, "status": task.status.value}
        if projection is not NotesProjection.DISABLED and task.notes is not None:
            body["notes"] = task.notes
        if task.due is not None:
            body["due"] = datetime.combine(task.due, datetime.min.time(), UTC).isoformat()
        if task.completed_at is not None:
            body["completed"] = task.completed_at.isoformat()
        return body

    @staticmethod
    def _event_body(event: Event) -> Json:
        body: Json = {
            "summary": event.summary,
            "start": _event_point(event.start),
            "end": _event_point(event.end),
            "status": event.status.value,
        }
        if event.description is not None:
            body["description"] = event.description
        if event.location is not None:
            body["location"] = event.location
        if event.recurrence:
            body["recurrence"] = list(event.recurrence)
        body["reminders"] = {
            "useDefault": event.reminder_use_default,
            "overrides": [
                {"method": item.method, "minutes": item.minutes}
                for item in event.reminder_overrides
            ],
        }
        for key, value in (
            ("attendees", list(event.attendees)),
            ("eventType", event.event_type),
            ("transparency", event.transparency),
            ("visibility", event.visibility),
            ("colorId", event.color_id),
            ("attachments", list(event.attachments)),
            ("conferenceData", event.conference),
            ("guestsCanInviteOthers", event.guests_can_invite_others),
            ("guestsCanModify", event.guests_can_modify),
            ("guestsCanSeeOtherGuests", event.guests_can_see_other_guests),
            ("anyoneCanAddSelf", event.anyone_can_add_self),
            ("focusTimeProperties", event.focus_time_properties),
            ("outOfOfficeProperties", event.out_of_office_properties),
            ("workingLocationProperties", event.working_location_properties),
        ):
            if value not in (None, [], ()):
                body[key] = value
        return body

    @staticmethod
    def _validate_zone(value: str | None) -> None:
        if value is None:
            return
        try:
            ZoneInfo(value)
        except (ZoneInfoNotFoundError, ValueError) as exc:
            raise ValueError(f"invalid time zone {value!r}") from exc

    @classmethod
    def _validate_event_times(cls, start: EventDateTime, end: EventDateTime) -> None:
        if start.kind is not end.kind:
            raise ValueError("event start and end must both be dates or both be date-times")
        cls._validate_zone(start.time_zone)
        cls._validate_zone(end.time_zone)
        if start.kind is DateTimeKind.DATE:
            if start.time_zone or end.time_zone:
                raise ValueError("all-day events cannot have a time zone")
        else:
            start_value = start.value
            end_value = end.value
            assert isinstance(start_value, datetime) and isinstance(end_value, datetime)
            if (start_value.tzinfo is None) != (end_value.tzinfo is None):
                raise ValueError("timed event endpoints must use matching timezone awareness")
            if start_value.tzinfo is None and not (start.time_zone and end.time_zone):
                raise ValueError("naive timed events require explicit IANA time zones")
        if end.value <= start.value:
            raise ValueError("event end must be after start")

    @staticmethod
    def _validate_event_options(
        send_updates: str, supports_attachments: bool, conference_data_version: int
    ) -> None:
        if send_updates not in {"none", "all", "externalOnly"}:
            raise ValueError("send_updates must be none, all, or externalOnly")
        if not isinstance(supports_attachments, bool):
            raise ValueError("supports_attachments must be boolean")
        if conference_data_version not in {0, 1}:
            raise ValueError("conference_data_version must be 0 or 1")


# The public service is intentionally a small composition root. The mixins are
# domain modules; this class remains stable for CLI, TUI, and third-party callers.
from .application_history import HistoryServiceMixin  # noqa: E402
from .application_search import SearchServiceMixin  # noqa: E402
from .application_workflows import WorkflowServiceMixin  # noqa: E402
from .application_workspace import WorkspaceServiceMixin  # noqa: E402


class ApplicationService(
    WorkspaceServiceMixin,
    SearchServiceMixin,
    WorkflowServiceMixin,
    HistoryServiceMixin,
):
    """Optimistic local domain boundary used by every UI."""


Application = ApplicationService
