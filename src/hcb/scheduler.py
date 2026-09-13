"""Persistent, idempotent reminder scheduling independent of the TUI."""

from __future__ import annotations

import json
import os
import random
import time
from collections.abc import Callable
from dataclasses import dataclass
from datetime import UTC, date, datetime, timedelta
from datetime import time as day_time
from pathlib import Path
from zoneinfo import ZoneInfo

from .errors import SyncBusyError
from .models import DateTimeKind, Event, EventStatus, Task, TaskStatus, utc_now
from .notifications import Notification, NotificationAction, Notifier
from .storage import Storage
from .task_recurrence import parse_task_recurrence_notes


@dataclass(frozen=True, slots=True)
class SchedulerResult:
    discovered: int = 0
    delivered: int = 0
    snoozed: int = 0
    dismissed: int = 0
    failed: int = 0


class ReminderScheduler:
    def __init__(
        self,
        storage: Storage,
        notifier: Notifier,
        *,
        now: Callable[[], datetime] = utc_now,
        catch_up: timedelta = timedelta(hours=1),
    ) -> None:
        self.storage = storage
        self.notifier = notifier
        self.now = now
        self.catch_up = catch_up

    @staticmethod
    def _aware(value: datetime, zone: str | None = None) -> datetime:
        if value.tzinfo is None:
            return value.replace(tzinfo=ZoneInfo(zone or "UTC")).astimezone(UTC)
        return value.astimezone(UTC)

    def _event_start(self, event: Event, calendar_zone: str | None) -> datetime:
        if event.start.kind is DateTimeKind.DATE:
            assert isinstance(event.start.value, date)
            local = datetime.combine(event.start.value, day_time.min)
            return self._aware(local, event.start.time_zone or calendar_zone)
        assert isinstance(event.start.value, datetime)
        return self._aware(event.start.value, event.start.time_zone or calendar_zone)

    def _discover_events(self, account_id: str, earliest: datetime, now: datetime) -> int:
        found = 0
        calendars = {item.id: item for item in self.storage.list_calendars(account_id)}
        for event in self.storage.list_events(account_id):
            if (
                event.status is EventStatus.CANCELLED
                or event.metadata.deleted
                or event.attendee_response == "declined"
            ):
                continue
            calendar = calendars.get(event.calendar_id)
            reminders = (
                calendar.default_reminders
                if event.reminder_use_default and calendar is not None
                else event.reminder_overrides
            )
            start = self._event_start(event, calendar.time_zone if calendar else None)
            for reminder in reminders:
                if reminder.method != "popup":
                    continue
                trigger = start - timedelta(minutes=reminder.minutes)
                if earliest <= trigger <= now:
                    self.storage.ensure_reminder_delivery(
                        account_id,
                        "event",
                        event.id,
                        trigger,
                        occurrence_id=event.occurrence_id,
                    )
                    found += 1
        return found

    def _discover_tasks(self, account_id: str, earliest: datetime, now: datetime) -> int:
        found = 0
        for task in self.storage.list_tasks(account_id):
            if task.status is TaskStatus.COMPLETED or task.metadata.deleted or task.due is None:
                continue
            parsed = parse_task_recurrence_notes(task.notes or "")
            if parsed.reminder is None:
                continue
            hour, minute = (int(value) for value in parsed.reminder.time.split(":"))
            trigger = datetime.combine(task.due, day_time(hour, minute))
            trigger = self._aware(trigger, parsed.reminder.time_zone)
            if earliest <= trigger <= now:
                occurrence = parsed.marker.occurrence_id if parsed.marker else None
                self.storage.ensure_reminder_delivery(
                    account_id, "task", task.id, trigger, occurrence_id=occurrence
                )
                found += 1
        return found

    def _notification(self, row: dict[str, object]) -> Notification | None:
        account_id, source_id = str(row["account_id"]), str(row["source_id"])
        if row["source_type"] == "event":
            source: Event | Task | None = self.storage.get_event(account_id, source_id)
            if not isinstance(source, Event):
                return None
            title, body = "Calendar reminder", source.summary
        else:
            source = self.storage.get_task(account_id, source_id)
            if not isinstance(source, Task):
                return None
            title, body = "Task reminder", source.title
        identifier = ":".join(
            str(row[key])
            for key in ("account_id", "source_type", "source_id", "occurrence_id", "scheduled_at")
        )
        return Notification(title, body, identifier)

    def run_once(self, account_id: str) -> SchedulerResult:
        now = self._aware(self.now())
        earliest = now - self.catch_up
        discovered = self._discover_events(account_id, earliest, now)
        discovered += self._discover_tasks(account_id, earliest, now)
        delivered = snoozed = dismissed = failed = 0
        for row in self.storage.due_reminder_deliveries(account_id, now, earliest):
            notification = self._notification(row)
            if notification is None:
                self.storage.update_reminder_delivery(row, dismissed_at=now)
                dismissed += 1
                continue
            try:
                action = self.notifier.notify(notification)
            except Exception as exc:
                self.storage.update_reminder_delivery(row, error=type(exc).__name__)
                failed += 1
                continue
            if action is NotificationAction.SNOOZE:
                self.storage.update_reminder_delivery(
                    row, snoozed_until=now + timedelta(minutes=10)
                )
                snoozed += 1
            elif action is NotificationAction.DISMISS:
                self.storage.update_reminder_delivery(row, dismissed_at=now)
                dismissed += 1
            else:
                self.storage.update_reminder_delivery(row, delivered_at=now)
                delivered += 1
        return SchedulerResult(discovered, delivered, snoozed, dismissed, failed)


def run_loop(
    scheduler: ReminderScheduler,
    account_id: str,
    *,
    interval: float,
    jitter: float = 0,
    sync: Callable[[], object] | None = None,
    sync_interval: float = 0,
    sleep: Callable[[float], None] = time.sleep,
    random_value: Callable[[], float] = random.random,
    stop: Callable[[], bool] = lambda: False,
) -> None:
    failures = 0
    last_sync: float | None = None
    while not stop():
        try:
            scheduler.run_once(account_id)
            monotonic = time.monotonic()
            if (
                sync
                and sync_interval > 0
                and (last_sync is None or monotonic - last_sync >= sync_interval)
            ):
                sync()
                last_sync = monotonic
            failures = 0
        except SyncBusyError:
            failures = 0
        except Exception:
            failures = min(failures + 1, 6)
        delay = interval * (2**failures) + random_value() * jitter
        sleep(delay)


class DaemonState:
    """Atomic PID/status files used by LaunchAgent and manual runs."""

    def __init__(self, directory: Path) -> None:
        self.directory = directory
        self.pid_file = directory / "reminderd.pid"
        self.status_file = directory / "reminderd-status.json"

    def write(self, status: str, *, error: str | None = None) -> None:
        self.directory.mkdir(parents=True, exist_ok=True)
        self.pid_file.write_text(str(os.getpid()), encoding="ascii")
        payload = {"pid": os.getpid(), "status": status, "updated_at": utc_now().isoformat()}
        if error:
            payload["error"] = error
        temporary = self.status_file.with_suffix(".tmp")
        temporary.write_text(json.dumps(payload, sort_keys=True), encoding="utf-8")
        temporary.replace(self.status_file)

    def clear(self) -> None:
        self.pid_file.unlink(missing_ok=True)
