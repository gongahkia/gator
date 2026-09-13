import select
import subprocess
import sys
from datetime import UTC, datetime
from functools import partial
from pathlib import Path
from threading import Barrier, Lock, Thread, get_ident
from threading import Event as ThreadEvent

import pytest

from hcb.application import ApplicationService
from hcb.errors import (
    AuthenticationRequired,
    GoogleApiError,
    RequestNotSentError,
    SyncBusyError,
    TransientTransportError,
)
from hcb.google_client import Page
from hcb.models import (
    Account,
    Calendar,
    DateTimeKind,
    EntityType,
    Event,
    EventDateTime,
    Metadata,
    MutationOperation,
    OutboxDeliveryState,
    PendingMutation,
    SyncCursor,
    Task,
    TaskList,
)
from hcb.storage import Storage
from hcb.sync import SyncEngine
from hcb.sync_pull import _FirstPagePrefetch

NOW = datetime(2026, 8, 21, 8, tzinfo=UTC)
EVENT = {
    "id": "event-r",
    "summary": "Standup",
    "start": {"dateTime": "2026-08-21T09:00:00Z"},
    "end": {"dateTime": "2026-08-21T09:30:00Z"},
    "updated": "2026-08-21T07:00:00Z",
    "etag": '"event-1"',
}


class FakeGateway:
    def __init__(self):
        self.task_list_pages = {None: Page(())}
        self.task_pages = {None: Page(())}
        self.calendar_pages = {None: Page(())}
        self.event_pages = {None: Page((), next_sync_token="sync-1")}
        self.calls = []
        self.fail = None
        self.created_task = {"id": "task-r", "etag": '"task-1"', "updated": "2026-08-21T08:00:00Z"}

    def list_task_lists(self, *, page_token=None):
        self.calls.append(("task-lists", page_token))
        return self.task_list_pages[page_token]

    def list_tasks(self, task_list_id, *, page_token=None, updated_min=None):
        self.calls.append(("tasks", task_list_id, page_token, updated_min))
        return self.task_pages[page_token]

    def list_calendars(self, *, page_token=None, sync_token=None):
        self.calls.append(("calendars", page_token, sync_token))
        return self.calendar_pages[page_token]

    def list_events(
        self,
        calendar_id,
        *,
        page_token=None,
        sync_token=None,
        time_min=None,
        time_max=None,
        single_events=False,
    ):
        self.calls.append(("events", calendar_id, page_token, sync_token, single_events))
        if self.fail:
            failure, self.fail = self.fail, None
            raise failure
        return self.event_pages[page_token]

    def create_task(self, task_list_id, body):
        self.calls.append(("create-task", task_list_id, body))
        if self.fail:
            failure, self.fail = self.fail, None
            raise failure
        return self.created_task

    def update_task(self, *args, **kwargs):
        if self.fail:
            raise self.fail
        return {"id": args[1]}

    def delete_task(self, *args, **kwargs):
        if self.fail:
            raise self.fail

    def move_task(self, *args, **kwargs):
        self.calls.append(("move-task", *args, kwargs))
        return {"id": args[1]}

    def create_task_list(self, body):
        return {"id": "list-r"}

    def update_task_list(self, task_list_id, body, *, etag=None):
        return {"id": task_list_id}

    def delete_task_list(self, task_list_id, *, etag=None):
        return None

    def create_calendar(self, body):
        return {"id": "cal-r"}

    def subscribe_calendar(self, calendar_id):
        self.calls.append(("subscribe-calendar", calendar_id))
        return {"id": calendar_id}

    def remove_calendar(self, calendar_id):
        self.calls.append(("remove-calendar", calendar_id))

    def update_calendar(self, calendar_id, body, *, etag=None):
        return {"id": calendar_id}

    def update_calendar_list(self, calendar_id, body, *, etag=None):
        self.calls.append(("update-calendar-list", calendar_id, body, etag))
        return {"id": calendar_id}

    def delete_calendar(self, calendar_id, *, etag=None):
        return None

    def calendar_colors(self):
        return {"calendar": {}, "event": {}}

    def create_event(self, calendar_id, body, **kwargs):
        self.calls.append(("create-event", calendar_id, body, kwargs))
        return {"id": "event-r"}

    def update_event(self, calendar_id, event_id, body, *, etag=None, **kwargs):
        return {"id": event_id}

    def delete_event(self, calendar_id, event_id, *, etag=None, **kwargs):
        return None

    def move_event(self, calendar_id, event_id, destination):
        return {"id": event_id}

    def respond_event(self, calendar_id, event_id, response_status, *, etag=None, **kwargs):
        return {"id": event_id}

    def freebusy(self, body):
        return {"calendars": {}}

    def drive_metadata(self, file_id, *, fields="*"):
        return {"id": file_id}

    def search_drive_metadata(self, query, *, page_token=None, page_size=100):
        return Page(({"id": "drive-1", "name": query},))


@pytest.fixture
def store(tmp_path: Path):
    with Storage(tmp_path / "sync.db") as result:
        result.upsert_account(Account("a", "a@example.test"))
        result.upsert_task_list(TaskList("list", "a", "Inbox", remote_id="list-r"))
        result.upsert_calendar(Calendar("cal", "a", "Primary", remote_id="cal-r"))
        yield result


def test_initial_and_incremental_task_sync_uses_overlap(store):
    gateway = FakeGateway()
    gateway.task_pages = {
        None: Page(
            (
                {
                    "id": "task-r",
                    "title": "One",
                    "status": "needsAction",
                    "updated": "2026-08-21T07:00:00Z",
                },
            )
        )
    }
    engine = SyncEngine(store, gateway, now=lambda: NOW)
    engine.sync_tasks("a", store.get_task_list("a", "list"))
    assert store.get_task("a", "task-r").title == "One"
    assert gateway.calls[-1][-1] is None

    engine.sync_tasks("a", store.get_task_list("a", "list"))
    assert gateway.calls[-1][-1] == "2026-08-21T07:55:00Z"


def test_unchanged_task_list_pull_does_not_rebuild_child_search_rows(store: Storage) -> None:
    store.upsert_task(Task("child", "a", "list", "Child", remote_id="child-r"))
    gateway = FakeGateway()
    gateway.task_list_pages = {
        None: Page(({"id": "list-r", "title": "Inbox", "etag": '"list-1"'},))
    }
    engine = SyncEngine(store, gateway)

    def child_document() -> tuple[int, str]:
        row = store.connection.execute(
            """SELECT id,list_name FROM workspace_search_documents
            WHERE account_id='a' AND kind='task' AND entity_id='child'"""
        ).fetchone()
        return row["id"], row["list_name"]

    engine.sync_task_lists("a")
    original_document = child_document()
    original_timestamp = store.get_task_list("a", "list").metadata.local_updated_at
    engine.sync_task_lists("a")
    assert child_document() == original_document
    assert store.get_task_list("a", "list").metadata.local_updated_at == original_timestamp

    gateway.task_list_pages = {
        None: Page(({"id": "list-r", "title": "Renamed", "etag": '"list-2"'},))
    }
    engine.sync_task_lists("a")
    assert child_document()[1] == "Renamed"
    assert store.get_task_list("a", "list").metadata.etag == '"list-2"'


def test_sync_reports_completed_stages(store):
    stages: list[str] = []

    SyncEngine(store, FakeGateway()).sync("a", progress=stages.append)

    assert stages == [
        "Sending local changes",
        "Fetching task lists",
        "Fetching tasks 1/1",
        "Fetching calendars",
        "Fetching calendar 1/1",
        "Finishing sync",
    ]


def test_sync_drains_more_than_two_outbox_pages_before_pull(store: Storage) -> None:
    gateway = FakeGateway()
    for index in range(205):
        task_id = f"task-{index:03}"
        store.upsert_task(Task(task_id, "a", "list", "Local", remote_id=task_id))
        store.enqueue(
            PendingMutation(
                None,
                "a",
                EntityType.TASK,
                task_id,
                MutationOperation.UPDATE,
                {"list_id": "list", "body": {"title": f"Updated {index}"}},
            )
        )

    def update_task(task_list_id, task_id, body, *, etag=None):
        gateway.calls.append(("update-task", task_id))
        return {"id": task_id}

    gateway.update_task = update_task
    result = SyncEngine(store, gateway).sync("a")

    assert result.pushed == 205
    assert gateway.calls[:205] == [("update-task", f"task-{index:03}") for index in range(205)]
    assert gateway.calls[205] == ("task-lists", None)
    assert store.pending_mutation_count("a") == 0


def test_cancellation_after_first_outbox_page_preserves_remaining_writes(store: Storage) -> None:
    gateway = FakeGateway()
    for index in range(105):
        task_id = f"task-{index:03}"
        store.upsert_task(Task(task_id, "a", "list", "Local", remote_id=task_id))
        store.enqueue(
            PendingMutation(
                None,
                "a",
                EntityType.TASK,
                task_id,
                MutationOperation.UPDATE,
                {"list_id": "list", "body": {"title": f"Updated {index}"}},
            )
        )

    attempts = 0

    def update_task(*args, **kwargs):
        nonlocal attempts
        attempts += 1
        if attempts == 101:
            raise GoogleApiError(503, "temporary")
        return {"id": args[1]}

    gateway.update_task = update_task
    result = SyncEngine(store, gateway, wait_for_retry=lambda _delay, _: False).flush_outbox("a")

    assert result.pushed == 100 and result.cancelled
    assert attempts == 101
    assert store.pending_mutation_count("a") == 5
    assert all(
        item.delivery_state is OutboxDeliveryState.PENDING for item in store.pending_mutations("a")
    )
    resumed = SyncEngine(store, FakeGateway()).flush_outbox("a")
    assert resumed.pushed == 5
    assert store.pending_mutation_count("a") == 0


def test_page_checkpoint_resumes_without_replaying_committed_page(store):
    gateway = FakeGateway()
    gateway.task_pages = {
        None: Page(({"id": "one", "title": "One"},), next_page_token="p2"),
        "p2": Page(({"id": "two", "title": "Two"},)),
    }
    original = gateway.list_tasks
    failed = False
    attempts: list[str | None] = []
    waits: list[float] = []

    def interrupted(task_list_id, *, page_token=None, updated_min=None):
        nonlocal failed
        attempts.append(page_token)
        if page_token == "p2" and not failed:
            failed = True
            raise GoogleApiError(503, "temporary")
        return original(task_list_id, page_token=page_token, updated_min=updated_min)

    gateway.list_tasks = interrupted
    engine = SyncEngine(
        store,
        gateway,
        now=lambda: NOW,
        random_source=lambda: 0.0,
        wait_for_retry=lambda delay, _: waits.append(delay) or True,
    )
    engine.sync_tasks("a", store.get_task_list("a", "list"))
    assert store.get_task("a", "one") is not None
    assert attempts == [None, "p2", "p2"]
    assert waits == [1.0]
    assert store.get_task("a", "two") is not None


def test_task_page_checkpoint_survives_database_reopen(tmp_path: Path) -> None:
    path = tmp_path / "task-pages.db"
    gateway = FakeGateway()
    gateway.task_pages = {
        None: Page(
            tuple({"id": f"task-{index}", "title": f"Task {index}"} for index in range(100)),
            next_page_token="p2",
        ),
        "p2": Page(({"id": "task-100", "title": "Task 100"},)),
    }
    original = gateway.list_tasks
    page_tokens: list[str | None] = []
    interrupted = False

    def fail_once(task_list_id: str, *, page_token=None, updated_min=None):
        nonlocal interrupted
        page_tokens.append(page_token)
        if page_token == "p2" and not interrupted:
            interrupted = True
            raise RuntimeError("stopped between task pages")
        return original(task_list_id, page_token=page_token, updated_min=updated_min)

    gateway.list_tasks = fail_once
    with Storage(path) as first:
        first.upsert_account(Account("a", "a@example.test"))
        first.upsert_task_list(TaskList("list", "a", "Inbox", remote_id="list-r"))
        task_list = first.get_task_list("a", "list")
        assert task_list is not None
        with pytest.raises(RuntimeError, match="stopped between task pages"):
            SyncEngine(first, gateway, now=lambda: NOW).sync_tasks("a", task_list)
        assert len(first.list_tasks("a")) == 100
        assert first.get_cursor("a", "tasks:list-r") is None
        assert first.resumable_checkpoint("a", "tasks:list-r")[1] == "p2"

    with Storage(path) as reopened:
        task_list = reopened.get_task_list("a", "list")
        assert task_list is not None
        result = SyncEngine(reopened, gateway, now=lambda: NOW).sync_tasks("a", task_list)
        assert result.pulled == 1
        assert len(reopened.list_tasks("a")) == 101
        assert reopened.get_cursor("a", "tasks:list-r").cursor == "2026-08-21T08:00:00Z"
    assert page_tokens == [None, "p2", "p2"]


def test_calendar_410_resets_only_expired_calendar_cursor(store):
    gateway = FakeGateway()
    store.set_cursor(SyncCursor("a", "events:cal-r", "old"))
    store.set_cursor(SyncCursor("a", "events:other", "keep"))
    gateway.fail = GoogleApiError(410, "expired")
    SyncEngine(store, gateway).sync_events("a", store.get_calendar("a", "cal"))
    event_calls = [call for call in gateway.calls if call[0] == "events"]
    assert [call[3] for call in event_calls] == ["old", None]
    assert store.get_cursor("a", "events:cal-r").cursor == "sync-1"
    assert store.get_cursor("a", "events:other").cursor == "keep"


def test_explicit_instance_refresh_caches_only_recurring_instances(store):
    gateway = FakeGateway()
    gateway.event_pages = {
        None: Page(
            (
                {
                    **EVENT,
                    "id": "instance-r",
                    "recurringEventId": "series-r",
                    "originalStartTime": {"dateTime": "2026-08-21T09:00:00Z"},
                },
                {**EVENT, "id": "ordinary-r"},
            )
        )
    }
    engine = SyncEngine(store, gateway)
    instances = engine.refresh_occurrences(
        "a",
        "cal",
        datetime(2026, 8, 21, tzinfo=UTC),
        datetime(2026, 8, 28, tzinfo=UTC),
    )
    assert [item.remote_id for item in instances] == ["instance-r"]
    assert store.get_event("a", "instance-r@2026-08-21T09:00:00Z").derived
    assert store.list_instance_ranges("a", "cal")


def test_remote_recurring_series_change_stales_cached_instance_ranges(store):
    gateway = FakeGateway()
    store.replace_cached_instances(
        "a",
        "cal",
        datetime(2026, 8, 21, tzinfo=UTC),
        datetime(2026, 8, 28, tzinfo=UTC),
        [],
    )
    gateway.event_pages = {
        None: Page(({**EVENT, "recurrence": ["RRULE:FREQ=DAILY"]},), next_sync_token="sync-2")
    }
    calendar = store.get_calendar("a", "cal")
    assert calendar is not None
    SyncEngine(store, gateway).sync_events("a", calendar)
    ranges = store.list_instance_ranges("a", "cal")
    assert ranges[0]["state"] == "stale"
    assert ranges[0]["stale_reason"] == "remote-recurring-event-changed"


def test_non_idempotent_task_create_is_quarantined_after_ambiguous_rate_limit(store):
    gateway = FakeGateway()
    store.upsert_task(Task("tmp", "a", "list", "Local"))
    mutation = PendingMutation(
        None,
        "a",
        EntityType.TASK,
        "tmp",
        MutationOperation.CREATE,
        {"list_id": "list", "body": {"title": "Local"}},
    )
    store.enqueue(mutation)
    gateway.fail = GoogleApiError(429, "slow", retry_after=9)
    result = SyncEngine(
        store,
        gateway,
        wait_for_retry=lambda _delay, _: pytest.fail("unsafe create must not be retried"),
    ).flush_outbox("a")
    assert result.conflicts == 1
    assert store.pending_mutations("a") == []
    assert store.list_conflicts("a")[0].local_payload["kind"] == "uncertain-delivery"


@pytest.mark.parametrize("status", [409, 410, 412])
def test_outbox_conflicts_are_recorded_without_dropping_later_writes(store, status):
    gateway = FakeGateway()
    store.upsert_task(Task("tmp", "a", "list", "Local"))
    for title in ("first", "second"):
        store.enqueue(
            PendingMutation(
                None,
                "a",
                EntityType.TASK,
                "tmp",
                MutationOperation.CREATE,
                {"list_id": "list", "body": {"title": title}},
            )
        )
    gateway.fail = GoogleApiError(status, "conflict")
    result = SyncEngine(store, gateway).flush_outbox("a")
    assert result.conflicts == 1 and result.pushed == 1
    assert len(store.list_conflicts("a")) == 1
    assert store.pending_mutations("a") == []


def test_pull_preserves_dirty_local_write_and_accounts_are_isolated(store):
    store.upsert_account(Account("b", "b@example.test"))
    store.upsert_task_list(TaskList("list", "b", "B", remote_id="list-b"))
    dirty = Task(
        "local",
        "a",
        "list",
        "Local",
        remote_id="task-r",
        metadata=Metadata(dirty=True),
    )
    store.upsert_task(dirty)
    gateway = FakeGateway()
    gateway.task_pages = {None: Page(({"id": "task-r", "title": "Remote"},))}
    SyncEngine(store, gateway).sync_tasks("a", store.get_task_list("a", "list"))
    assert store.get_task("a", "local").title == "Local"
    assert store.list_tasks("b") == []


def test_calendar_list_mutations_use_distinct_gateway_resources(store):
    gateway = FakeGateway()
    store.enqueue(
        PendingMutation(
            None,
            "a",
            EntityType.CALENDAR,
            "cal",
            MutationOperation.SUBSCRIBE,
            {"remote_id": "shared@example.test"},
        )
    )
    store.enqueue(
        PendingMutation(
            None,
            "a",
            EntityType.CALENDAR,
            "cal",
            MutationOperation.REMOVE,
            {"remote_id": "shared@example.test"},
        )
    )
    result = SyncEngine(store, gateway).flush_outbox("a")
    assert result.pushed == 2
    assert ("subscribe-calendar", "cal-r") in gateway.calls
    assert ("remove-calendar", "cal-r") in gateway.calls


def test_calendar_page_resume_survives_database_reopen(tmp_path: Path) -> None:
    path = tmp_path / "calendar-resume.db"
    gateway = FakeGateway()
    gateway.event_pages = {
        None: Page((EVENT,), next_page_token="p2"),
        "p2": Page(
            (
                {
                    **EVENT,
                    "id": "event-second",
                    "summary": "Second",
                    "etag": '"event-2"',
                },
            ),
            next_sync_token="sync-2",
        ),
    }
    original = gateway.list_events
    interrupted = False
    attempted_pages = []

    def crash_after_first_page(
        calendar_id,
        *,
        page_token=None,
        sync_token=None,
        time_min=None,
        time_max=None,
        single_events=False,
    ):
        nonlocal interrupted
        attempted_pages.append(page_token)
        if page_token == "p2" and not interrupted:
            interrupted = True
            raise RuntimeError("simulated process crash")
        return original(
            calendar_id,
            page_token=page_token,
            sync_token=sync_token,
            time_min=time_min,
            time_max=time_max,
            single_events=single_events,
        )

    gateway.list_events = crash_after_first_page
    with Storage(path) as first:
        first.upsert_account(Account("a", "a@example.test"))
        first.upsert_calendar(Calendar("cal", "a", "Primary", remote_id="cal-r"))
        with pytest.raises(RuntimeError, match="simulated process crash"):
            SyncEngine(first, gateway).sync_events("a", first.get_calendar("a", "cal"))
        assert first.get_event("a", "event-r") is not None
        assert first.get_cursor("a", "events:cal-r") is None
        assert first.resumable_checkpoint("a", "events:cal-r")[1] == "p2"

    with Storage(path) as reopened:
        SyncEngine(reopened, gateway).sync_events("a", reopened.get_calendar("a", "cal"))
        assert reopened.get_event("a", "event-second") is not None
        assert reopened.get_cursor("a", "events:cal-r").cursor == "sync-2"
    assert attempted_pages == [None, "p2", "p2"]


def test_paginated_incremental_calendar_list_keeps_sync_token(store: Storage) -> None:
    gateway = FakeGateway()
    gateway.calendar_pages = {
        None: Page((), next_page_token="p2"),
        "p2": Page((), next_sync_token="next-calendar-sync"),
    }
    store.set_cursor(SyncCursor("a", "calendar-list", "previous-calendar-sync"))

    SyncEngine(store, gateway).sync_calendars("a")

    assert [call for call in gateway.calls if call[0] == "calendars"] == [
        ("calendars", None, "previous-calendar-sync"),
        ("calendars", "p2", "previous-calendar-sync"),
    ]
    assert store.get_cursor("a", "calendar-list").cursor == "next-calendar-sync"


def test_paginated_incremental_events_keep_sync_token(store: Storage) -> None:
    gateway = FakeGateway()
    gateway.event_pages = {
        None: Page((), next_page_token="p2"),
        "p2": Page((), next_sync_token="next-events-sync"),
    }
    store.set_cursor(SyncCursor("a", "events:cal-r", "previous-events-sync"))

    SyncEngine(store, gateway).sync_events("a", store.get_calendar("a", "cal"))

    assert [call for call in gateway.calls if call[0] == "events"] == [
        ("events", "cal-r", None, "previous-events-sync", False),
        ("events", "cal-r", "p2", "previous-events-sync", False),
    ]
    assert store.get_cursor("a", "events:cal-r").cursor == "next-events-sync"


def test_parallel_pull_overlaps_reads_and_applies_rows_on_owner_thread(
    store: Storage, monkeypatch: pytest.MonkeyPatch
) -> None:
    store.upsert_task_list(TaskList("other-list", "a", "Other", remote_id="other-list-r"))
    store.upsert_calendar(Calendar("other-cal", "a", "Other", remote_id="other-cal-r"))
    owner_thread = get_ident()
    task_barrier = Barrier(2)
    event_barrier = Barrier(2)
    worker_threads: list[int] = []

    class ParallelGateway(FakeGateway):
        parallel_reads_safe = True

        def list_tasks(self, task_list_id, *, page_token=None, updated_min=None):
            worker_threads.append(get_ident())
            task_barrier.wait(timeout=3)
            return Page(({"id": f"task-{task_list_id}", "title": task_list_id},))

        def list_events(
            self,
            calendar_id,
            *,
            page_token=None,
            sync_token=None,
            time_min=None,
            time_max=None,
            single_events=False,
        ):
            worker_threads.append(get_ident())
            event_barrier.wait(timeout=3)
            return Page(({**EVENT, "id": f"event-{calendar_id}"},), next_sync_token="next")

    original_task_upsert = store.upsert_task
    original_event_upsert = store.upsert_event

    def upsert_task_on_owner(item):
        assert get_ident() == owner_thread
        original_task_upsert(item)

    def upsert_event_on_owner(item):
        assert get_ident() == owner_thread
        original_event_upsert(item)

    monkeypatch.setattr(store, "upsert_task", upsert_task_on_owner)
    monkeypatch.setattr(store, "upsert_event", upsert_event_on_owner)
    engine = SyncEngine(store, ParallelGateway(), pull_workers=2)

    task_result = engine.sync_task_lists("a")
    event_result = engine.sync_calendars("a")

    assert task_result.pulled == 2
    assert event_result.pulled == 2
    assert len(store.list_tasks("a")) == 2
    assert len(store.list_events("a")) == 2
    assert len(worker_threads) == 4
    assert all(thread != owner_thread for thread in worker_threads)


def test_parallel_first_page_failure_retries_without_replaying_other_lists(
    store: Storage,
) -> None:
    store.upsert_task_list(TaskList("other-list", "a", "Other", remote_id="other-list-r"))
    calls: list[str] = []
    lock = Lock()
    failed = False

    class FlakyGateway(FakeGateway):
        parallel_reads_safe = True

        def list_tasks(self, task_list_id, *, page_token=None, updated_min=None):
            nonlocal failed
            with lock:
                calls.append(task_list_id)
                if task_list_id == "list-r" and not failed:
                    failed = True
                    raise TransientTransportError("first request interrupted")
            return Page(({"id": f"task-{task_list_id}", "title": task_list_id},))

    engine = SyncEngine(
        store, FlakyGateway(), pull_workers=2, max_retries=1, wait_for_retry=lambda *_: True
    )

    assert engine.sync_task_lists("a").pulled == 2
    assert calls.count("list-r") == 2
    assert calls.count("other-list-r") == 1
    assert len(store.list_tasks("a")) == 2


def test_parallel_pull_skips_prefetch_for_resumable_task_page(store: Storage) -> None:
    for index in (2, 3):
        store.upsert_task_list(
            TaskList(f"list-{index}", "a", f"List {index}", remote_id=f"list-{index}-r")
        )
    checkpoint = store.start_checkpoint("a", "tasks:list-r")
    store.save_checkpoint_page(checkpoint, "p2")
    calls: list[tuple[str, str | None]] = []
    lock = Lock()

    class ResumeGateway(FakeGateway):
        parallel_reads_safe = True

        def list_tasks(self, task_list_id, *, page_token=None, updated_min=None):
            with lock:
                calls.append((task_list_id, page_token))
            return Page(({"id": f"task-{task_list_id}", "title": task_list_id},))

    assert SyncEngine(store, ResumeGateway(), pull_workers=2).sync_task_lists("a").pulled == 3
    assert sorted(calls) == [
        ("list-2-r", None),
        ("list-3-r", None),
        ("list-r", "p2"),
    ]
    assert store.resumable_checkpoint("a", "tasks:list-r") is None


def test_parallel_calendar_410_resets_only_expired_cursor(store: Storage) -> None:
    store.upsert_calendar(Calendar("other-cal", "a", "Other", remote_id="other-cal-r"))
    store.set_cursor(SyncCursor("a", "events:cal-r", "expired"))
    store.set_cursor(SyncCursor("a", "events:other-cal-r", "current"))
    calls: list[tuple[str, str | None]] = []
    lock = Lock()

    class ExpiredGateway(FakeGateway):
        parallel_reads_safe = True

        def list_events(
            self,
            calendar_id,
            *,
            page_token=None,
            sync_token=None,
            time_min=None,
            time_max=None,
            single_events=False,
        ):
            with lock:
                calls.append((calendar_id, sync_token))
            if calendar_id == "cal-r" and sync_token == "expired":
                raise GoogleApiError(410, "sync token expired")
            return Page((), next_sync_token=f"next-{calendar_id}")

    assert SyncEngine(store, ExpiredGateway(), pull_workers=2).sync_calendars("a").pages == 3
    assert calls.count(("cal-r", "expired")) == 1
    assert calls.count(("cal-r", None)) == 1
    assert calls.count(("other-cal-r", "current")) == 1
    assert store.get_cursor("a", "events:cal-r").cursor == "next-cal-r"
    assert store.get_cursor("a", "events:other-cal-r").cursor == "next-other-cal-r"


def test_parallel_pull_cancellation_leaves_remote_rows_unapplied(store: Storage) -> None:
    store.upsert_task_list(TaskList("other-list", "a", "Other", remote_id="other-list-r"))
    cancelled = False

    class ParallelGateway(FakeGateway):
        parallel_reads_safe = True

        def list_tasks(self, task_list_id, *, page_token=None, updated_min=None):
            return Page(({"id": f"task-{task_list_id}", "title": task_list_id},))

    def progress(message: str) -> None:
        nonlocal cancelled
        if message == "Fetching tasks 1/2":
            cancelled = True

    engine = SyncEngine(store, ParallelGateway(), pull_workers=2)
    result = engine.sync("a", progress=progress, cancelled=lambda: cancelled)

    assert result.cancelled
    assert store.list_tasks("a") == []
    assert store.get_cursor("a", "tasks:list-r") is None

    completed = engine.sync("a")
    assert not completed.cancelled
    assert len(store.list_tasks("a")) == 2


def test_prefetched_first_page_checkpoint_resumes_after_database_reopen(tmp_path: Path) -> None:
    path = tmp_path / "parallel-resume.db"
    calls: list[tuple[str, str | None]] = []
    lock = Lock()
    interrupted = False

    class InterruptedGateway(FakeGateway):
        parallel_reads_safe = True

        def list_tasks(self, task_list_id, *, page_token=None, updated_min=None):
            nonlocal interrupted
            with lock:
                calls.append((task_list_id, page_token))
                if task_list_id == "list-r" and page_token == "p2" and not interrupted:
                    interrupted = True
                    raise RuntimeError("stopped after prefetched first page")
            if task_list_id == "list-r" and page_token is None:
                return Page(({"id": "first", "title": "First"},), next_page_token="p2")
            if task_list_id == "list-r":
                return Page(({"id": "second", "title": "Second"},))
            return Page(({"id": "other", "title": "Other"},))

    gateway = InterruptedGateway()
    with Storage(path) as first:
        first.upsert_account(Account("a", "a@example.test"))
        first.upsert_task_list(TaskList("list", "a", "Inbox", remote_id="list-r"))
        first.upsert_task_list(TaskList("other-list", "a", "Other", remote_id="other-r"))
        with pytest.raises(RuntimeError, match="stopped after prefetched first page"):
            SyncEngine(first, gateway, pull_workers=2).sync_task_lists("a")
        assert first.get_task("a", "first") is not None
        assert first.get_task("a", "second") is None
        assert first.resumable_checkpoint("a", "tasks:list-r")[1] == "p2"

    with Storage(path) as reopened:
        SyncEngine(reopened, gateway, pull_workers=2).sync_task_lists("a")
        assert len(reopened.list_tasks("a")) == 3
        assert reopened.resumable_checkpoint("a", "tasks:list-r") is None
    assert calls.count(("list-r", None)) == 1
    assert calls.count(("list-r", "p2")) == 2


def test_parallel_prefetch_keeps_only_worker_count_pages_outstanding() -> None:
    started = ThreadEvent()
    release = ThreadEvent()
    first_batch_done = ThreadEvent()
    lock = Lock()
    requested: list[int] = []

    def fetch(index: int) -> Page:
        with lock:
            requested.append(index)
            if len(requested) == 4:
                first_batch_done.set()
        if index == 0:
            started.set()
            assert release.wait(timeout=3)
        return Page(())

    requests = {str(index): partial(fetch, index) for index in range(8)}
    try:
        with _FirstPagePrefetch(requests, 4) as prefetch:
            assert started.wait(timeout=3)
            assert first_batch_done.wait(timeout=3)
            with lock:
                assert sorted(requested) == [0, 1, 2, 3]
            release.set()
            assert prefetch.take("0").result() == Page(())
            prefetch.replenish()
            with lock:
                assert len(requested) <= 5
    finally:
        release.set()


def test_outbox_restart_and_completed_create_are_not_replayed(tmp_path: Path) -> None:
    path = tmp_path / "outbox-restart.db"
    with Storage(path) as first:
        first.upsert_account(Account("a", "a@example.test"))
        first.upsert_task_list(TaskList("list", "a", "Inbox", remote_id="list-r"))
        first.upsert_task(Task("tmp", "a", "list", "Persisted"))
        first.enqueue(
            PendingMutation(
                None,
                "a",
                EntityType.TASK,
                "tmp",
                MutationOperation.CREATE,
                {"list_id": "list", "body": {"title": "Persisted"}},
            )
        )

    gateway = FakeGateway()
    with Storage(path) as reopened:
        assert len(reopened.pending_mutations("a")) == 1
        result = SyncEngine(reopened, gateway).flush_outbox("a")
        assert result.pushed == 1
        assert reopened.pending_mutations("a") == []

    with Storage(path) as second_restart:
        result = SyncEngine(second_restart, gateway).flush_outbox("a")
        assert result.pushed == 0
        assert second_restart.get_task("a", "tmp").remote_id == "task-r"

    assert len([call for call in gateway.calls if call[0] == "create-task"]) == 1


def _seed_crash_database(path: Path, *, entity_type: EntityType) -> str:
    with Storage(path) as storage:
        storage.upsert_account(Account("a", "a@example.test"))
        if entity_type is EntityType.TASK:
            storage.upsert_task_list(TaskList("list", "a", "Inbox", remote_id="list-r"))
            storage.upsert_task(Task("local", "a", "list", "Crash-safe"))
            payload = {"list_id": "list", "body": {"title": "Crash-safe"}}
        else:
            storage.upsert_calendar(Calendar("cal", "a", "Primary", remote_id="cal-r"))
            event = {
                **EVENT,
                "id": "local",
                "summary": "Crash-safe event",
            }
            from hcb.sync import event_from_google

            storage.upsert_event(event_from_google("a", "cal", event, local_id="local"))
            payload = {
                "calendar_id": "cal",
                "body": {
                    "summary": "Crash-safe event",
                    "start": EVENT["start"],
                    "end": EVENT["end"],
                },
            }
        storage.enqueue(
            PendingMutation(
                None,
                "a",
                entity_type,
                "local",
                MutationOperation.CREATE,
                payload,
            )
        )
    return "local"


def test_active_delivery_is_not_recovered_by_a_second_sync(tmp_path: Path) -> None:
    path = tmp_path / "overlapping-sync.db"
    _seed_crash_database(path, entity_type=EntityType.TASK)
    started = ThreadEvent()
    release = ThreadEvent()
    failures: list[BaseException] = []

    class BlockingGateway(FakeGateway):
        def create_task(self, task_list_id, body):
            started.set()
            if not release.wait(5):
                raise TimeoutError("blocked Google request was not released")
            return super().create_task(task_list_id, body)

    def deliver() -> None:
        try:
            with Storage(path) as active:
                SyncEngine(active, BlockingGateway()).flush_outbox("a")
        except BaseException as error:
            failures.append(error)

    worker = Thread(target=deliver)
    worker.start()
    try:
        assert started.wait(5)
        with Storage(path) as competing:
            with pytest.raises(SyncBusyError):
                SyncEngine(competing, FakeGateway()).flush_outbox("a")
            assert competing.pending_mutations("a")[0].delivery_state is OutboxDeliveryState.SENDING
            assert competing.list_conflicts("a") == []
    finally:
        release.set()
        worker.join(5)
    assert not worker.is_alive()
    assert failures == []
    with Storage(path) as completed:
        assert completed.pending_mutations("a") == []
        assert completed.list_conflicts("a") == []


@pytest.mark.skipif(sys.platform == "win32", reason="POSIX advisory sync lock")
def test_sync_lock_is_process_wide_and_released_on_exit(tmp_path: Path) -> None:
    path = tmp_path / "process-sync.db"
    with Storage(path) as storage:
        child_code = """
import sys
from hcb.storage import Storage
from hcb.sync import SyncEngine

with Storage(sys.argv[1]) as child_storage:
    with SyncEngine(child_storage, object()).sync_ownership():
        print('locked', flush=True)
        sys.stdin.read(1)
"""
        child = subprocess.Popen(
            [sys.executable, "-c", child_code, str(path)],
            stdin=subprocess.PIPE,
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
            text=True,
        )
        try:
            assert child.stdout is not None
            assert select.select([child.stdout], [], [], 5)[0]
            assert child.stdout.readline().strip() == "locked"
            with pytest.raises(SyncBusyError), SyncEngine(storage, FakeGateway()).sync_ownership():
                pass
            child.terminate()
            child.wait(timeout=5)
            with SyncEngine(storage, FakeGateway()).sync_ownership():
                pass
        finally:
            if child.poll() is None:
                child.terminate()
                child.wait(timeout=5)


def test_interrupted_delivery_recovery_covers_every_page(store: Storage) -> None:
    for index in range(101):
        store.enqueue(
            PendingMutation(
                None,
                "a",
                EntityType.TASK,
                f"task-{index}",
                MutationOperation.CREATE,
                {"list_id": "list", "body": {"title": f"Task {index}"}},
                delivery_state=OutboxDeliveryState.SENDING,
            )
        )

    assert SyncEngine(store, FakeGateway()).recover_interrupted_deliveries("a") == 101
    assert store.pending_mutations("a") == []
    assert len(store.list_conflicts("a")) == 101


def test_full_task_cursor_reset_waits_for_sync_ownership(store: Storage) -> None:
    store.set_cursor(SyncCursor("a", "tasks:list-r", "watermark", NOW))
    with Storage(store.path) as competing, SyncEngine(store, FakeGateway()).sync_ownership():
        with pytest.raises(SyncBusyError):
            SyncEngine(competing, FakeGateway()).sync("a", full_tasks=True)
        assert store.get_cursor("a", "tasks:list-r").cursor == "watermark"


def test_reminder_pull_mode_respects_sync_ownership(store: Storage) -> None:
    gateway = FakeGateway()
    with (
        Storage(store.path) as competing,
        SyncEngine(store, FakeGateway()).sync_ownership(),
        pytest.raises(SyncBusyError),
    ):
        SyncEngine(competing, gateway).pull_only("a")
    assert gateway.calls == []


def test_crash_before_request_quarantines_non_idempotent_create_on_restart(
    tmp_path: Path,
) -> None:
    path = tmp_path / "before-request.db"
    _seed_crash_database(path, entity_type=EntityType.TASK)
    gateway = FakeGateway()

    def crash(phase: str, _mutation: PendingMutation) -> None:
        if phase == "before-request":
            raise RuntimeError("crash before request")

    with Storage(path) as storage:
        with pytest.raises(RuntimeError, match="crash before request"):
            SyncEngine(storage, gateway, crash_hook=crash).flush_outbox("a")
        assert storage.pending_mutations("a")[0].delivery_state is OutboxDeliveryState.SENDING
    assert not [call for call in gateway.calls if call[0] == "create-task"]

    with Storage(path) as restarted:
        result = SyncEngine(restarted, gateway).flush_outbox("a")
        assert result.conflicts == 1
        assert restarted.pending_mutations("a") == []
        conflict = restarted.list_conflicts("a")[0]
        assert conflict.local_payload["kind"] == "uncertain-delivery"


@pytest.mark.skipif(sys.platform == "win32", reason="POSIX advisory sync lock")
def test_abrupt_process_exit_releases_owner_and_preserves_uncertain_create(tmp_path: Path) -> None:
    path = tmp_path / "abrupt-exit.db"
    _seed_crash_database(path, entity_type=EntityType.TASK)
    child_code = """
import os
import sys
from hcb.storage import Storage
from hcb.sync import SyncEngine

def crash(phase, _mutation):
    if phase == 'before-request':
        os._exit(17)

with Storage(sys.argv[1]) as storage:
    SyncEngine(storage, object(), crash_hook=crash).flush_outbox('a')
"""
    child = subprocess.run(
        [sys.executable, "-c", child_code, str(path)],
        capture_output=True,
        text=True,
        timeout=5,
        check=False,
    )
    assert child.returncode == 17, child.stderr

    gateway = FakeGateway()
    with Storage(path) as restarted:
        assert restarted.pending_mutations("a")[0].delivery_state is OutboxDeliveryState.SENDING
        result = SyncEngine(restarted, gateway).flush_outbox("a")
        assert result.conflicts == 1 and result.retry_pending
        assert restarted.pending_mutations("a") == []
        assert restarted.list_conflicts("a")[0].local_payload["kind"] == "uncertain-delivery"
    assert not [call for call in gateway.calls if call[0] == "create-task"]


def test_crash_after_task_success_does_not_blindly_retry_and_can_mark_delivered(
    tmp_path: Path,
) -> None:
    path = tmp_path / "after-success.db"
    _seed_crash_database(path, entity_type=EntityType.TASK)
    gateway = FakeGateway()

    def crash(phase: str, _mutation: PendingMutation) -> None:
        if phase == "after-remote-success":
            raise RuntimeError("crash after remote success")

    with Storage(path) as storage, pytest.raises(RuntimeError, match="crash after remote success"):
        SyncEngine(storage, gateway, crash_hook=crash).flush_outbox("a")
    assert len([call for call in gateway.calls if call[0] == "create-task"]) == 1

    with Storage(path) as restarted:
        result = SyncEngine(restarted, gateway).flush_outbox("a")
        assert result.conflicts == 1
        assert len([call for call in gateway.calls if call[0] == "create-task"]) == 1
        conflict = restarted.list_conflicts("a")[0]
        application = ApplicationService(restarted)
        with pytest.raises(ValueError, match="remote-id"):
            application.resolve_uncertain_delivery("a", conflict.id, "delivered")
        application.resolve_uncertain_delivery("a", conflict.id, "delivered", remote_id="task-r")
        assert restarted.get_task("a", "local").remote_id == "task-r"
        assert restarted.pending_mutations("a") == []


def test_user_can_retry_create_after_verifying_it_was_not_delivered(tmp_path: Path) -> None:
    path = tmp_path / "retry-resolution.db"
    _seed_crash_database(path, entity_type=EntityType.TASK)
    gateway = FakeGateway()

    def crash(phase: str, _mutation: PendingMutation) -> None:
        if phase == "before-request":
            raise RuntimeError("stopped")

    with Storage(path) as storage, pytest.raises(RuntimeError):
        SyncEngine(storage, gateway, crash_hook=crash).flush_outbox("a")
    with Storage(path) as restarted:
        SyncEngine(restarted, gateway).recover_interrupted_deliveries("a")
        conflict = restarted.list_conflicts("a")[0]
        ApplicationService(restarted).resolve_uncertain_delivery("a", conflict.id, "retry")
        assert len(restarted.pending_mutations("a")) == 1
        assert SyncEngine(restarted, gateway).flush_outbox("a").pushed == 1
        assert restarted.pending_mutations("a") == []
    assert len([call for call in gateway.calls if call[0] == "create-task"]) == 1


def test_interrupted_create_holds_later_edits_until_reconciled(tmp_path: Path) -> None:
    path = tmp_path / "dependent-edits.db"
    _seed_crash_database(path, entity_type=EntityType.TASK)
    with Storage(path) as storage:
        original_create_id = storage.pending_mutations("a")[0].id
        storage.enqueue(
            PendingMutation(
                None,
                "a",
                EntityType.TASK,
                "local",
                MutationOperation.UPDATE,
                {"list_id": "list", "body": {"title": "Edited offline"}},
            )
        )
    gateway = FakeGateway()
    updates: list[tuple[object, ...]] = []

    def update_task(*args: object, **_kwargs: object) -> dict[str, str]:
        updates.append(args)
        return {"id": str(args[1])}

    gateway.update_task = update_task

    def crash(phase: str, _mutation: PendingMutation) -> None:
        if phase == "before-request":
            raise RuntimeError("stopped before request")

    with Storage(path) as active, pytest.raises(RuntimeError, match="stopped before request"):
        SyncEngine(active, gateway, crash_hook=crash).flush_outbox("a")

    with Storage(path) as restarted:
        result = SyncEngine(restarted, gateway).flush_outbox("a")
        assert result.conflicts == 1
        assert result.retry_pending
        assert updates == []
        assert [row.operation for row in restarted.pending_mutations("a")] == [
            MutationOperation.UPDATE
        ]
        assert SyncEngine(restarted, gateway).flush_outbox("a").retry_pending
        conflict = restarted.list_conflicts("a")[0]
        ApplicationService(restarted).resolve_uncertain_delivery("a", conflict.id, "retry")
        assert [row.id for row in restarted.pending_mutations("a")] == [
            original_create_id,
            original_create_id + 1,
        ]
        resumed = SyncEngine(restarted, gateway).flush_outbox("a")
        assert resumed.pushed == 2
        assert updates == [("list-r", "task-r", {"title": "Edited offline"})]
        assert restarted.pending_mutations("a") == []


def test_delivered_reconciliation_keeps_later_edits_dirty_until_sent(tmp_path: Path) -> None:
    path = tmp_path / "delivered-then-edited.db"
    _seed_crash_database(path, entity_type=EntityType.TASK)
    with Storage(path) as storage:
        storage.enqueue(
            PendingMutation(
                None,
                "a",
                EntityType.TASK,
                "local",
                MutationOperation.UPDATE,
                {"list_id": "list", "body": {"title": "Later edit"}},
            )
        )
    gateway = FakeGateway()

    def crash(phase: str, _mutation: PendingMutation) -> None:
        if phase == "after-remote-success":
            raise RuntimeError("response not recorded")

    with Storage(path) as active, pytest.raises(RuntimeError, match="response not recorded"):
        SyncEngine(active, gateway, crash_hook=crash).flush_outbox("a")
    with Storage(path) as restarted:
        assert SyncEngine(restarted, gateway).flush_outbox("a").retry_pending
        conflict = restarted.list_conflicts("a")[0]
        ApplicationService(restarted).resolve_uncertain_delivery(
            "a", conflict.id, "delivered", remote_id="task-r"
        )
        task = restarted.get_task("a", "local")
        assert task is not None and task.metadata.dirty
        assert SyncEngine(restarted, gateway).flush_outbox("a").pushed == 1
        assert restarted.pending_mutations("a") == []
    assert len([call for call in gateway.calls if call[0] == "create-task"]) == 1


def test_event_create_retries_same_google_id_after_success_crash(tmp_path: Path) -> None:
    path = tmp_path / "event-idempotency.db"
    _seed_crash_database(path, entity_type=EntityType.EVENT)

    class IdempotentEventGateway(FakeGateway):
        def __init__(self):
            super().__init__()
            self.event_ids = []

        def create_event(self, calendar_id, body, **kwargs):
            self.calls.append(("create-event", calendar_id, body, kwargs))
            event_id = body["id"]
            self.event_ids.append(event_id)
            if len(self.event_ids) > 1:
                raise GoogleApiError(409, "event id already exists")
            return {"id": event_id, "etag": '"created"'}

    gateway = IdempotentEventGateway()

    def crash(phase: str, _mutation: PendingMutation) -> None:
        if phase == "after-remote-success":
            raise RuntimeError("event response lost")

    with Storage(path) as storage, pytest.raises(RuntimeError, match="event response lost"):
        SyncEngine(storage, gateway, crash_hook=crash).flush_outbox("a")
    with Storage(path) as restarted:
        result = SyncEngine(restarted, gateway).flush_outbox("a")
        assert result.pushed == 1
        assert result.conflicts == 0
        assert restarted.pending_mutations("a") == []
        assert restarted.get_event("a", "local").remote_id == gateway.event_ids[0]
    assert gateway.event_ids[0] == gateway.event_ids[1]
    assert gateway.event_ids[0].startswith("hcb")


def test_known_request_not_sent_failure_remains_retryable(store: Storage) -> None:
    gateway = FakeGateway()
    store.upsert_task(Task("tmp-safe", "a", "list", "Retryable"))
    store.enqueue(
        PendingMutation(
            None,
            "a",
            EntityType.TASK,
            "tmp-safe",
            MutationOperation.CREATE,
            {"list_id": "list", "body": {"title": "Retryable"}},
        )
    )
    original = gateway.create_task
    failed = False

    def not_sent(task_list_id, body):
        nonlocal failed
        if not failed:
            failed = True
            raise RequestNotSentError("connection failed before send")
        return original(task_list_id, body)

    gateway.create_task = not_sent
    waits: list[float] = []
    result = SyncEngine(
        store,
        gateway,
        random_source=lambda: 0.0,
        wait_for_retry=lambda delay, _: waits.append(delay) or True,
    ).flush_outbox("a")
    assert result.pushed == 1
    assert waits == [1.0]
    assert store.pending_mutations("a") == []


@pytest.mark.parametrize(
    ("status", "reason", "retry_after"),
    [
        (408, None, None),
        (429, "rateLimitExceeded", 7.0),
        (503, None, None),
        (403, "userRateLimitExceeded", None),
    ],
)
def test_safe_outbox_writes_retry_only_transient_google_failures(
    store: Storage,
    status: int,
    reason: str | None,
    retry_after: float | None,
) -> None:
    gateway = FakeGateway()
    store.upsert_task(Task("remote", "a", "list", "Existing", remote_id="task-r"))
    store.enqueue(
        PendingMutation(
            None,
            "a",
            EntityType.TASK,
            "remote",
            MutationOperation.UPDATE,
            {"list_id": "list", "body": {"title": "Updated"}},
        )
    )
    attempts = 0
    waits: list[float] = []

    def flaky_update(*args, **kwargs):
        nonlocal attempts
        attempts += 1
        if attempts == 1:
            raise GoogleApiError(status, "temporary", reason=reason, retry_after=retry_after)
        return {"id": args[1]}

    gateway.update_task = flaky_update
    progress: list[str] = []
    result = SyncEngine(
        store,
        gateway,
        random_source=lambda: 0.25,
        wait_for_retry=lambda delay, _: waits.append(delay) or True,
    ).sync("a", progress=progress.append)

    assert result.pushed == 1
    assert attempts == 2
    assert waits == [retry_after or 1.25]
    assert any("retrying 1/4" in status for status in progress)
    assert store.pending_mutations("a") == []


@pytest.mark.parametrize(
    ("status", "reason"),
    [(400, None), (401, None), (403, "insufficientPermissions"), (409, None)],
)
def test_permanent_google_write_failures_are_not_retried(
    store: Storage, status: int, reason: str | None
) -> None:
    gateway = FakeGateway()
    store.upsert_task(Task("remote", "a", "list", "Existing", remote_id="task-r"))
    store.enqueue(
        PendingMutation(
            None,
            "a",
            EntityType.TASK,
            "remote",
            MutationOperation.UPDATE,
            {"list_id": "list", "body": {"title": "Updated"}},
        )
    )
    attempts = 0

    def rejected(*args, **kwargs):
        nonlocal attempts
        attempts += 1
        raise GoogleApiError(status, "permanent", reason=reason)

    gateway.update_task = rejected
    engine = SyncEngine(
        store,
        gateway,
        wait_for_retry=lambda _delay, _: pytest.fail("permanent failure must not be retried"),
    )
    if status == 409:
        result = engine.flush_outbox("a")
        assert result.conflicts == 1
    elif status in {401, 403}:
        with pytest.raises(AuthenticationRequired, match="Google authorization is required"):
            engine.flush_outbox("a")
    else:
        with pytest.raises(GoogleApiError, match="permanent"):
            engine.flush_outbox("a")
    assert attempts == 1


def test_retry_exhaustion_keeps_a_safe_write_queued_for_manual_resume(store: Storage) -> None:
    gateway = FakeGateway()
    store.upsert_task(Task("remote", "a", "list", "Existing", remote_id="task-r"))
    store.enqueue(
        PendingMutation(
            None,
            "a",
            EntityType.TASK,
            "remote",
            MutationOperation.UPDATE,
            {"list_id": "list", "body": {"title": "Updated"}},
        )
    )
    attempts = 0
    waits: list[float] = []

    def unavailable(*args, **kwargs):
        nonlocal attempts
        attempts += 1
        raise GoogleApiError(503, "unavailable")

    gateway.update_task = unavailable
    result = SyncEngine(
        store,
        gateway,
        max_retries=2,
        random_source=lambda: 0.0,
        wait_for_retry=lambda delay, _: waits.append(delay) or True,
    ).sync("a")

    assert result.retry_pending and result.retry_exhausted
    assert result.retry_message and "run sync to resume" in result.retry_message
    assert attempts == 3
    assert waits == [1.0, 2.0]
    mutation = store.pending_mutations("a")[0]
    assert mutation.delivery_state is OutboxDeliveryState.PENDING
    assert mutation.attempts == 1


def test_cancelling_a_retry_wait_preserves_the_queued_write(store: Storage) -> None:
    gateway = FakeGateway()
    store.upsert_task(Task("remote", "a", "list", "Existing", remote_id="task-r"))
    store.enqueue(
        PendingMutation(
            None,
            "a",
            EntityType.TASK,
            "remote",
            MutationOperation.UPDATE,
            {"list_id": "list", "body": {"title": "Updated"}},
        )
    )
    attempts = 0

    def unavailable(*args, **kwargs):
        nonlocal attempts
        attempts += 1
        raise GoogleApiError(503, "unavailable")

    gateway.update_task = unavailable
    result = SyncEngine(
        store,
        gateway,
        wait_for_retry=lambda _delay, _: False,
    ).sync("a")

    assert result.cancelled
    assert attempts == 1
    assert store.pending_mutations("a")[0].delivery_state is OutboxDeliveryState.PENDING


def test_ambiguous_transport_failure_quarantines_task_create_without_replay(store: Storage) -> None:
    gateway = FakeGateway()
    store.upsert_task(Task("tmp", "a", "list", "Local"))
    store.enqueue(
        PendingMutation(
            None,
            "a",
            EntityType.TASK,
            "tmp",
            MutationOperation.CREATE,
            {"list_id": "list", "body": {"title": "Local"}},
        )
    )
    attempts = 0

    def interrupted(*args, **kwargs):
        nonlocal attempts
        attempts += 1
        raise TransientTransportError("response lost")

    gateway.create_task = interrupted
    result = SyncEngine(
        store,
        gateway,
        wait_for_retry=lambda _delay, _: pytest.fail("ambiguous create must not be retried"),
    ).sync("a")

    assert result.conflicts == 1
    assert attempts == 1
    assert store.pending_mutations("a") == []


def test_interrupt_during_task_create_quarantines_unconfirmed_delivery(store: Storage) -> None:
    gateway = FakeGateway()
    store.upsert_task(Task("tmp", "a", "list", "Local"))
    store.enqueue(
        PendingMutation(
            None,
            "a",
            EntityType.TASK,
            "tmp",
            MutationOperation.CREATE,
            {"list_id": "list", "body": {"title": "Local"}},
        )
    )

    def interrupted(*args, **kwargs):
        raise KeyboardInterrupt

    gateway.create_task = interrupted
    with pytest.raises(KeyboardInterrupt):
        SyncEngine(store, gateway).flush_outbox("a")
    assert store.pending_mutations("a") == []
    assert store.list_conflicts("a")[0].local_payload["kind"] == "uncertain-delivery"


def test_calendar_event_create_retries_with_one_deterministic_id(store: Storage) -> None:
    gateway = FakeGateway()
    store.upsert_event(
        Event(
            "local-event",
            "a",
            "cal",
            "Planning",
            start=EventDateTime(DateTimeKind.DATETIME, NOW),
            end=EventDateTime(DateTimeKind.DATETIME, NOW),
        )
    )
    store.enqueue(
        PendingMutation(
            None,
            "a",
            EntityType.EVENT,
            "local-event",
            MutationOperation.CREATE,
            {"calendar_id": "cal", "body": {"summary": "Planning"}},
        )
    )
    event_ids: list[str] = []

    def flaky_event(calendar_id, body, **kwargs):
        event_ids.append(body["id"])
        if len(event_ids) == 1:
            raise GoogleApiError(503, "unavailable")
        raise GoogleApiError(409, "already exists")

    gateway.create_event = flaky_event
    result = SyncEngine(
        store,
        gateway,
        random_source=lambda: 0.0,
        wait_for_retry=lambda _delay, _: True,
    ).sync("a")

    assert result.pushed == 1
    assert event_ids[0] == event_ids[1]
    assert store.get_event("a", "local-event").remote_id == event_ids[0]


@pytest.mark.parametrize(
    ("entity_type", "operation", "payload"),
    [
        (
            EntityType.TASK,
            MutationOperation.CREATE,
            {"list_id": "list", "body": {"title": "Task"}},
        ),
        (
            EntityType.TASK_LIST,
            MutationOperation.CREATE,
            {"body": {"title": "List"}},
        ),
        (
            EntityType.CALENDAR,
            MutationOperation.CREATE,
            {"body": {"summary": "Calendar"}},
        ),
        (
            EntityType.TASK,
            MutationOperation.MOVE,
            {
                "source_list_id": "source",
                "list_id": "destination",
                "body": {"title": "Moved"},
            },
        ),
    ],
)
def test_every_non_idempotent_create_path_quarantines_interrupted_sending(
    store: Storage,
    entity_type: EntityType,
    operation: MutationOperation,
    payload: dict[str, object],
) -> None:
    store.enqueue(
        PendingMutation(
            None,
            "a",
            entity_type,
            f"uncertain-{entity_type.value}-{operation.value}",
            operation,
            payload,
            delivery_state=OutboxDeliveryState.SENDING,
        )
    )
    assert SyncEngine(store, FakeGateway()).recover_interrupted_deliveries("a") == 1
    assert store.pending_mutations("a") == []
    assert store.list_conflicts("a")[0].local_payload["kind"] == "uncertain-delivery"


def test_cross_list_task_move_uses_google_native_destination_move(store: Storage) -> None:
    store.upsert_task_list(TaskList("source", "a", "Source", remote_id="source-r"))
    store.upsert_task_list(TaskList("destination", "a", "Destination", remote_id="destination-r"))
    store.upsert_task(Task("parent", "a", "destination", "Parent", remote_id="parent-r"))
    store.upsert_task(
        Task(
            "previous",
            "a",
            "destination",
            "Previous",
            parent_id="parent",
            remote_id="previous-r",
        )
    )
    store.upsert_task(
        Task(
            "moved",
            "a",
            "destination",
            "Moved",
            parent_id="parent",
            position="previous",
            remote_id="moved-r",
        )
    )
    store.enqueue(
        PendingMutation(
            None,
            "a",
            EntityType.TASK,
            "moved",
            MutationOperation.MOVE,
            {
                "source_list_id": "source",
                "list_id": "destination",
                "parent": "parent",
                "previous": "previous",
                "remote_id": "moved-r",
            },
        )
    )

    gateway = FakeGateway()
    assert SyncEngine(store, gateway).flush_outbox("a").pushed == 1
    assert gateway.calls[-1] == (
        "move-task",
        "source-r",
        "moved-r",
        {
            "destination_task_list_id": "destination-r",
            "parent": "parent-r",
            "previous": "previous-r",
        },
    )
