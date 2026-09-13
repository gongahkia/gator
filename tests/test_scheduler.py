from __future__ import annotations

from datetime import UTC, date, datetime, timedelta
from pathlib import Path

from hcb.errors import SyncBusyError
from hcb.models import (
    Account,
    Calendar,
    DateTimeKind,
    Event,
    EventDateTime,
    ReminderOverride,
    Task,
    TaskList,
)
from hcb.notifications import Notification, NotificationAction, NotificationPermissionError
from hcb.scheduler import ReminderScheduler, run_loop
from hcb.storage import Storage
from hcb.task_recurrence import TaskReminder, serialize_task_notes

NOW = datetime(2026, 8, 21, 9, tzinfo=UTC)


class Clock:
    def __init__(self, value: datetime = NOW) -> None:
        self.value = value

    def __call__(self) -> datetime:
        return self.value


class FakeNotifier:
    def __init__(self, *actions: NotificationAction | Exception) -> None:
        self.actions = list(actions)
        self.sent: list[Notification] = []

    def notify(self, notification: Notification) -> NotificationAction:
        self.sent.append(notification)
        action = self.actions.pop(0) if self.actions else NotificationAction.DELIVERED
        if isinstance(action, Exception):
            raise action
        return action


def seed(path: Path) -> None:
    with Storage(path) as store:
        store.upsert_account(Account("a", "private@example.test"))
        store.upsert_task_list(TaskList("list", "a", "Inbox"))
        store.upsert_calendar(
            Calendar(
                "cal",
                "a",
                "Private",
                time_zone="UTC",
                default_reminders=(ReminderOverride("popup", 10),),
            )
        )


def event(reminders: tuple[ReminderOverride, ...] = ()) -> Event:
    return Event(
        "event",
        "a",
        "cal",
        "Standup",
        EventDateTime(DateTimeKind.DATETIME, NOW + timedelta(minutes=10)),
        EventDateTime(DateTimeKind.DATETIME, NOW + timedelta(minutes=40)),
        reminder_use_default=not reminders,
        reminder_overrides=reminders,
    )


def test_default_popup_delivery_is_restart_idempotent_with_tui_closed(tmp_path: Path) -> None:
    path = tmp_path / "db.sqlite"
    seed(path)
    with Storage(path) as store:
        store.upsert_event(event())
    notifier = FakeNotifier()
    with Storage(path) as reopened:
        first = ReminderScheduler(reopened, notifier, now=lambda: NOW).run_once("a")
    with Storage(path) as restarted:
        second = ReminderScheduler(restarted, notifier, now=lambda: NOW).run_once("a")
    assert (first.delivered, second.delivered, len(notifier.sent)) == (1, 0, 1)


def test_email_is_ignored_and_catch_up_is_bounded(tmp_path: Path) -> None:
    path = tmp_path / "db.sqlite"
    seed(path)
    with Storage(path) as store:
        store.upsert_event(event((ReminderOverride("email", 10),)))
        notifier = FakeNotifier()
        result = ReminderScheduler(store, notifier, now=lambda: NOW).run_once("a")
        assert result.delivered == 0
        old = event((ReminderOverride("popup", 200),))
        store.upsert_event(old)
        result = ReminderScheduler(
            store, notifier, now=lambda: NOW, catch_up=timedelta(minutes=30)
        ).run_once("a")
        assert result.delivered == 0


def test_snooze_then_deliver_and_dismiss(tmp_path: Path) -> None:
    path = tmp_path / "db.sqlite"
    seed(path)
    clock = Clock()
    notifier = FakeNotifier(NotificationAction.SNOOZE, NotificationAction.DELIVERED)
    with Storage(path) as store:
        store.upsert_event(event())
        scheduler = ReminderScheduler(store, notifier, now=clock)
        assert scheduler.run_once("a").snoozed == 1
        clock.value += timedelta(minutes=9)
        assert scheduler.run_once("a").delivered == 0
        clock.value += timedelta(minutes=1)
        assert scheduler.run_once("a").delivered == 1

        store.upsert_event(
            Event(
                "other",
                "a",
                "cal",
                "Other",
                EventDateTime(DateTimeKind.DATETIME, clock.value + timedelta(minutes=10)),
                EventDateTime(DateTimeKind.DATETIME, clock.value + timedelta(minutes=20)),
            )
        )
        notifier.actions.append(NotificationAction.DISMISS)
        assert scheduler.run_once("a").dismissed == 1
        assert scheduler.run_once("a").dismissed == 0


def test_task_marker_and_permission_denial_retry(tmp_path: Path) -> None:
    path = tmp_path / "db.sqlite"
    seed(path)
    notes = serialize_task_notes("", reminder=TaskReminder("09:00", "UTC")).notes
    notifier = FakeNotifier(
        NotificationPermissionError("denied"),
        NotificationAction.DELIVERED,
    )
    with Storage(path) as store:
        store.upsert_task(Task("task", "a", "list", "Pay", notes=notes, due=date(2026, 8, 21)))
        scheduler = ReminderScheduler(store, notifier, now=lambda: NOW)
        assert scheduler.run_once("a").failed == 1
        assert scheduler.run_once("a").delivered == 1
        row = store.reminder_delivery_rows("a")[0]
        assert row["attempts"] == 2
        assert row["last_error"] is None


def test_busy_sync_does_not_delay_reminder_scans(tmp_path: Path) -> None:
    path = tmp_path / "db.sqlite"
    seed(path)
    delays: list[float] = []

    def busy_sync() -> None:
        raise SyncBusyError("another sync is active")

    with Storage(path) as store:
        run_loop(
            ReminderScheduler(store, FakeNotifier(), now=lambda: NOW),
            "a",
            interval=5,
            sync=busy_sync,
            sync_interval=1,
            jitter=0,
            random_value=lambda: 0,
            sleep=delays.append,
            stop=lambda: len(delays) == 2,
        )
    assert delays == [5, 5]
