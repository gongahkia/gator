"""Read-only workspace views and local time-slot planning."""

from __future__ import annotations

from datetime import date, datetime, timedelta

from .application import Json, TimeSlot, WorkspaceSnapshot, _ApplicationServiceBase
from .errors import NotFoundError
from .models import (
    Account,
    Calendar,
    DateTimeKind,
    DriveFile,
    Event,
    EventStatus,
    NotesProjection,
    Task,
)


class WorkspaceServiceMixin(_ApplicationServiceBase):
    def list_accounts(self) -> tuple[Account, ...]:
        return tuple(self.storage.list_accounts())

    def account(self, account_id: str) -> Account | None:
        return self.storage.get_account(account_id)

    def calendar(self, account_id: str, calendar_id: str) -> Calendar | None:
        return self.storage.get_calendar(account_id, calendar_id)

    def drive_file(self, account_id: str, file_id: str) -> DriveFile | None:
        return self.storage.get_drive_file(account_id, file_id)

    def event_by_remote_id(self, account_id: str, remote_id: str) -> Event | None:
        return self.storage.get_event_by_remote(account_id, remote_id)

    def workspace(self, account_id: str) -> WorkspaceSnapshot:
        """Return one local-only snapshot for interactive clients."""
        account = self.storage.get_account(account_id)
        if account is None:
            raise NotFoundError(f"Account {account_id!r} does not exist")
        return WorkspaceSnapshot(
            account=account,
            tasks=tuple(self.storage.list_tasks(account_id)),
            events=tuple(self.storage.list_events(account_id)),
            task_lists=tuple(self.storage.list_task_lists(account_id)),
            calendars=tuple(self.storage.list_calendars(account_id)),
            instance_ranges=tuple(self.storage.list_instance_ranges(account_id)),
            pending=self.pending_count(account_id),
        )

    def pending_count(self, account_id: str) -> int:
        return self.storage.pending_mutation_count(account_id)

    def instance_ranges(self, account_id: str) -> tuple[dict[str, str], ...]:
        return tuple(self.storage.list_instance_ranges(account_id))

    def instance_cache_status(
        self,
        account_id: str,
        calendar_id: str,
        start: date | datetime,
        end: date | datetime,
    ) -> Json:
        return self.storage.instance_cache_status(account_id, calendar_id, start, end)

    def freebusy_request(self, account_id: str, start: datetime, end: datetime) -> Json:
        """Build an explicit remote availability request from selected cached calendars."""
        calendars = [
            {"id": calendar.remote_id}
            for calendar in self.storage.list_calendars(account_id)
            if calendar.selected and calendar.remote_id
        ]
        return {
            "timeMin": start.isoformat().replace("+00:00", "Z"),
            "timeMax": end.isoformat().replace("+00:00", "Z"),
            "items": calendars,
        }

    def find_time(
        self,
        account_id: str,
        day: date,
        *,
        duration_minutes: int = 30,
        day_start: int = 9,
        day_end: int = 17,
        step_minutes: int = 30,
    ) -> tuple[TimeSlot, ...]:
        """Find candidate wall-clock slots using only cached selected calendars."""
        self._account(account_id)
        if duration_minutes <= 0 or step_minutes <= 0:
            raise ValueError("duration and step must be positive")
        if not 0 <= day_start < day_end <= 24:
            raise ValueError("working hours must satisfy 0 <= start < end <= 24")
        window_start = datetime.combine(day, datetime.min.time()) + timedelta(hours=day_start)
        window_end = datetime.combine(day, datetime.min.time()) + timedelta(hours=day_end)
        selected = {item.id for item in self.storage.list_calendars(account_id) if item.selected}
        busy: list[tuple[datetime, datetime]] = []
        for event in self.storage.list_events(account_id, start=day, end=day + timedelta(days=1)):
            if event.calendar_id not in selected or event.status is EventStatus.CANCELLED:
                continue
            if event.start.kind is DateTimeKind.DATE:
                busy.append((window_start, window_end))
                continue
            start = event.start.value
            end = event.end.value
            assert isinstance(start, datetime) and isinstance(end, datetime)
            busy.append((start.replace(tzinfo=None), end.replace(tzinfo=None)))
        duration = timedelta(minutes=duration_minutes)
        step = timedelta(minutes=step_minutes)
        result: list[TimeSlot] = []
        cursor = window_start
        while cursor + duration <= window_end:
            end = cursor + duration
            if not any(cursor < busy_end and end > busy_start for busy_start, busy_end in busy):
                result.append(TimeSlot(cursor, end))
            cursor += step
        return tuple(result)

    def agenda_events(
        self,
        account_id: str,
        *,
        start: date | datetime,
        end: date | datetime,
        calendar_id: str | None = None,
    ) -> tuple[Event, ...]:
        """Return a local range view, preferring refreshed instances over series masters."""
        events = self.storage.list_events(account_id, calendar_id, start=start, end=end)
        expanded_series = {
            event.canonical_id for event in events if event.derived and event.canonical_id
        }
        return tuple(
            event
            for event in events
            if event.derived or not event.recurrence or event.remote_id not in expanded_series
        )

    def invitations(self, account_id: str) -> tuple[Event, ...]:
        """List cached events where the authenticated attendee has an RSVP state."""
        return tuple(
            event for event in self.storage.list_events(account_id) if event.attendee_response
        )

    def task_listing(self, account_id: str) -> tuple[Task, ...]:
        tasks = tuple(self.storage.list_tasks(account_id))
        if self.notes_projection(account_id) is NotesProjection.NOTES_ONLY:
            return tuple(
                task for task in tasks if task.due is not None or task.parent_id is not None
            )
        return tasks

    def notes_listing(self, account_id: str) -> tuple[Task, ...]:
        if self.notes_projection(account_id) is NotesProjection.DISABLED:
            return ()
        return tuple(
            task
            for task in self.storage.list_tasks(account_id)
            if task.due is None and task.parent_id is None
        )
