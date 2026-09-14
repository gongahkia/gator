from datetime import UTC, date, datetime
from pathlib import Path

import pytest

from hcb import storage_sync
from hcb.models import (
    Account,
    Calendar,
    Conflict,
    ConflictStatus,
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
from hcb.storage import SCHEMA_VERSION, Storage


@pytest.fixture
def store(tmp_path: Path):
    with Storage(tmp_path / "hcb.db") as result:
        yield result


def seed(store: Storage, account_id: str = "a") -> None:
    store.upsert_account(Account(account_id, f"{account_id}@example.test"))
    store.upsert_task_list(TaskList("list", account_id, "Inbox"))
    store.upsert_calendar(Calendar("cal", account_id, "Primary"))


def test_schema_wal_and_account_partitioning(store: Storage) -> None:
    seed(store, "a")
    seed(store, "b")
    store.upsert_task(Task("same", "a", "list", "Alpha needle"))
    store.upsert_task(Task("same", "b", "list", "Beta"))
    assert store.get_task("a", "same").title == "Alpha needle"
    assert store.get_task("b", "same").title == "Beta"
    assert [task.id for task in store.search_tasks("a", "needle")] == ["same"]
    diagnostics = store.diagnostics()
    assert diagnostics["schema_version"] == SCHEMA_VERSION
    assert diagnostics["journal_mode"] == "wal"
    assert diagnostics["integrity"] == "ok"


def test_task_list_local_visibility_persists(store: Storage) -> None:
    store.upsert_account(Account("a", "a@example.test"))
    task_list = TaskList("hidden", "a", "Hidden", selected=False)
    store.upsert_task_list(task_list)

    assert store.get_task_list("a", "hidden") == task_list


def test_event_occurrences_and_range_queries(store: Storage) -> None:
    seed(store)
    event = Event(
        "occ",
        "a",
        "cal",
        "Planning",
        EventDateTime(DateTimeKind.DATETIME, datetime(2026, 8, 21, 9, tzinfo=UTC)),
        EventDateTime(DateTimeKind.DATETIME, datetime(2026, 8, 21, 10, tzinfo=UTC)),
        canonical_id="series",
        occurrence_id="2026-08-21T09:00:00+00:00",
        recurrence=("RRULE:FREQ=WEEKLY",),
    )
    store.upsert_event(event)
    assert store.get_event("a", "occ") == event
    assert store.get_event("a", "occ").is_occurrence
    assert len(store.list_events("a", start=date(2026, 8, 21), end=date(2026, 8, 22))) == 1
    assert store.search_events("a", "Plan") == [event]


def test_calendar_list_metadata_and_cached_instance_range(store: Storage) -> None:
    seed(store)
    calendar = Calendar(
        "cal",
        "a",
        "Primary",
        color="#123456",
        foreground_color="#ffffff",
        location="Singapore",
        summary_override="Work",
        hidden=True,
        selected=False,
        notification_settings=({"type": "eventCreation", "method": "email"},),
    )
    store.upsert_calendar(calendar)
    assert store.get_calendar("a", "cal") == calendar

    start = datetime(2026, 8, 21, 9, tzinfo=UTC)
    instance = Event(
        "instance",
        "a",
        "cal",
        "Standup",
        EventDateTime(DateTimeKind.DATETIME, start),
        EventDateTime(DateTimeKind.DATETIME, start.replace(hour=10)),
        remote_id="instance-remote",
        canonical_id="series",
        occurrence_id=start.isoformat(),
        derived=True,
    )
    with store.transaction():
        store.replace_cached_instances("a", "cal", start, start.replace(day=28), [instance])
    assert store.get_event("a", "instance").derived
    assert store.list_instance_ranges("a", "cal")[0]["start_value"] == start.isoformat()
    assert (
        store.instance_cache_status("a", "cal", start.replace(day=22), start.replace(day=23))[
            "state"
        ]
        == "fresh"
    )

    store.mark_instance_ranges_stale("a", "cal", reason="local-recurring-event-updated")
    stale = store.instance_cache_status("a", "cal", start.replace(day=22), start.replace(day=23))
    assert stale["state"] == "stale"
    assert stale["ranges"][0]["stale_reason"] == "local-recurring-event-updated"
    with store.transaction():
        store.replace_cached_instances("a", "cal", start.replace(day=22), start.replace(day=23), [])
    partial = store.instance_cache_status("a", "cal", start.replace(day=21), start.replace(day=24))
    assert partial["state"] == "partial"
    assert (
        store.instance_cache_status("a", "cal", start.replace(day=22), start.replace(day=23))[
            "state"
        ]
        == "fresh"
    )

    dirty = Event(
        "local",
        "a",
        "cal",
        "Local change",
        EventDateTime(DateTimeKind.DATETIME, start),
        EventDateTime(DateTimeKind.DATETIME, start.replace(hour=10)),
        metadata=Metadata(dirty=True),
    )
    store.upsert_event(dirty)
    store.clear_calendar_mirror("a", "cal")
    assert store.get_event("a", "instance") is None
    assert store.get_event("a", "local") == dirty


def test_outbox_cursor_conflict_reminder_and_transaction(store: Storage) -> None:
    seed(store)
    mutation_id = store.enqueue(
        PendingMutation(
            None, "a", EntityType.TASK, "task", MutationOperation.CREATE, {"title": "New"}
        )
    )
    store.fail_mutation("a", mutation_id, "offline")
    assert store.pending_mutations("a")[0].attempts == 1
    store.complete_mutation("a", mutation_id)
    assert store.pending_mutations("a") == []

    store.set_cursor(SyncCursor("a", "tasks", "next"))
    assert store.get_cursor("a", "tasks").cursor == "next"
    checkpoint = store.start_checkpoint("a", "tasks")
    store.finish_checkpoint(checkpoint, cursor="next")

    conflict_id = store.add_conflict(
        Conflict(None, "a", EntityType.TASK, "task", {"title": "L"}, {"title": "R"})
    )
    assert store.list_conflicts("a")[0].id == conflict_id
    store.resolve_conflict("a", conflict_id, ConflictStatus.KEEP_LOCAL)
    assert store.list_conflicts("a") == []

    now = datetime.now(UTC)
    store.set_reminder("a", "event", now)
    assert store.due_reminders("a", now)[0]["event_id"] == "event"

    with pytest.raises(RuntimeError), store.transaction():
        store.upsert_task(Task("rolled-back", "a", "list", "No"))
        raise RuntimeError("stop")
    assert store.get_task("a", "rolled-back") is None


def test_get_mutation_reads_later_pages_and_preserves_account_and_delivery_state(
    store: Storage, monkeypatch: pytest.MonkeyPatch
) -> None:
    seed(store, "a")
    seed(store, "b")
    mutation_id = 0
    for index in range(101):
        mutation_id = store.enqueue(
            PendingMutation(
                None,
                "a",
                EntityType.TASK,
                f"task-{index}",
                MutationOperation.UPDATE,
                {"body": {"title": f"Updated {index}"}},
            )
        )
    started_at = datetime.now(UTC)
    store.mark_mutation_sending("a", mutation_id, request_id="request-101", started_at=started_at)

    decoded_rows = 0
    original_loads = storage_sync.json.loads

    def counted_loads(value: str) -> object:
        nonlocal decoded_rows
        decoded_rows += 1
        return original_loads(value)

    with monkeypatch.context() as patcher:
        patcher.setattr(storage_sync.json, "loads", counted_loads)
        mutation = store.get_mutation("a", mutation_id)

    assert decoded_rows == 1
    assert mutation is not None
    assert mutation == store.pending_mutations("a", limit=101)[-1]
    assert mutation.delivery_state is OutboxDeliveryState.SENDING
    assert mutation.request_id == "request-101"
    assert mutation.sending_started_at == started_at
    assert store.get_mutation("b", mutation_id) is None
    assert store.get_mutation("a", mutation_id + 1) is None
