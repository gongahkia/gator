from dataclasses import replace
from datetime import UTC, date, datetime
from pathlib import Path

import pytest

from hcb.application import ApplicationService
from hcb.errors import ConflictError, NotFoundError
from hcb.models import (
    Account,
    Calendar,
    Conflict,
    ConflictStatus,
    DateTimeKind,
    DriveFile,
    EntityType,
    Event,
    EventDateTime,
    Metadata,
    MutationOperation,
    NotesProjection,
    PendingMutation,
    ReminderOverride,
    SyncCursor,
    Task,
    TaskList,
    TaskPriority,
    TaskStatus,
)
from hcb.storage import Storage
from hcb.task_recurrence import (
    RecurrenceEnd,
    TaskRecurrenceMarker,
    parse_task_recurrence_notes,
    serialize_task_notes,
)


@pytest.fixture
def app(tmp_path: Path):
    with Storage(tmp_path / "application.db") as storage:
        storage.upsert_account(Account("a", "private@example.test"))
        storage.upsert_task_list(TaskList("inbox", "a", "Inbox", remote_id="remote-inbox"))
        storage.upsert_calendar(Calendar("cal", "a", "Primary", remote_id="remote-cal"))
        yield ApplicationService(storage)


def test_workspace_metadata_reads_are_available_through_application(
    app: ApplicationService,
) -> None:
    start = datetime(2026, 8, 21, tzinfo=UTC)
    end = start.replace(day=28)
    app.storage.replace_cached_instances("a", "cal", start, end, [])
    for index in range(101):
        app.create_task("a", "inbox", f"Task {index}", id=f"task-{index}")
    conflict_id = app.storage.add_conflict(
        Conflict(None, "a", EntityType.TASK, "task-0", {"title": "L"}, {"title": "R"})
    )

    snapshot = app.workspace("a")
    assert snapshot.pending == app.pending_count("a") == 101
    assert snapshot.instance_ranges == app.instance_ranges("a")
    assert app.instance_cache_status("a", "cal", start, end)["state"] == "fresh"
    assert [conflict.id for conflict in app.list_conflicts("a")] == [conflict_id]


def test_keep_remote_releases_dirty_event_and_resets_incremental_cursor(
    app: ApplicationService,
) -> None:
    start = EventDateTime(DateTimeKind.DATE, date(2026, 8, 21))
    end = EventDateTime(DateTimeKind.DATE, date(2026, 8, 22))
    app.storage.upsert_event(
        Event(
            "event",
            "a",
            "cal",
            "Local",
            start,
            end,
            remote_id="remote-event",
            metadata=Metadata(dirty=True),
        )
    )
    app.storage.set_cursor(SyncCursor("a", "events:remote-cal", "stale-token"))
    conflict_id = app.storage.add_conflict(
        Conflict(
            None,
            "a",
            EntityType.EVENT,
            "event",
            {"body": {"summary": "Local"}, "etag": "old"},
            {"status": 412},
        )
    )

    app.resolve_conflict("a", conflict_id, ConflictStatus.KEEP_REMOTE)

    assert not app.storage.get_event("a", "event").metadata.dirty
    assert app.storage.get_cursor("a", "events:remote-cal") is None
    assert app.storage.list_conflicts("a") == []


def test_keep_local_retries_without_stale_etag(app: ApplicationService) -> None:
    app.storage.upsert_task(
        Task("task", "a", "inbox", "Local", remote_id="remote-task", metadata=Metadata(dirty=True))
    )
    conflict_id = app.storage.add_conflict(
        Conflict(
            None,
            "a",
            EntityType.TASK,
            "task",
            {"list_id": "inbox", "body": {"title": "Local"}, "etag": "stale"},
            {"status": 412},
        )
    )

    app.resolve_conflict("a", conflict_id, ConflictStatus.KEEP_LOCAL)

    queued = app.storage.pending_mutations("a")
    assert len(queued) == 1
    assert queued[0].operation is MutationOperation.UPDATE
    assert queued[0].payload == {"list_id": "inbox", "body": {"title": "Local"}}


def test_keep_local_retry_precedes_later_queued_edit(app: ApplicationService) -> None:
    app.storage.upsert_task(
        Task("task", "a", "inbox", "Later", remote_id="remote-task", metadata=Metadata(dirty=True))
    )
    original_id = app.storage.enqueue(
        PendingMutation(
            None,
            "a",
            EntityType.TASK,
            "task",
            MutationOperation.UPDATE,
            {"list_id": "inbox", "body": {"title": "Original"}, "etag": "stale"},
        )
    )
    app.storage.complete_mutation("a", original_id)
    later_id = app.storage.enqueue(
        PendingMutation(
            None,
            "a",
            EntityType.TASK,
            "task",
            MutationOperation.UPDATE,
            {"list_id": "inbox", "body": {"title": "Later"}},
        )
    )
    conflict_id = app.storage.add_conflict(
        Conflict(
            None,
            "a",
            EntityType.TASK,
            "task",
            {"list_id": "inbox", "body": {"title": "Original"}, "etag": "stale"},
            {"status": 412, "mutation_id": original_id},
        )
    )

    app.resolve_conflict("a", conflict_id, ConflictStatus.KEEP_LOCAL)

    queued = app.storage.pending_mutations("a")
    assert [item.id for item in queued] == [original_id, later_id]
    assert queued[0].payload == {"list_id": "inbox", "body": {"title": "Original"}}
    assert queued[1].payload["body"]["title"] == "Later"


def test_keep_remote_preserves_later_queued_edits(app: ApplicationService) -> None:
    app.storage.upsert_task(
        Task("task", "a", "inbox", "Local", remote_id="remote-task", metadata=Metadata(dirty=True))
    )
    conflict_id = app.storage.add_conflict(
        Conflict(None, "a", EntityType.TASK, "task", {"body": {"title": "Local"}}, {"status": 412})
    )
    app.storage.enqueue(
        PendingMutation(
            None,
            "a",
            EntityType.TASK,
            "task",
            MutationOperation.UPDATE,
            {"body": {"title": "Later"}},
        )
    )

    with pytest.raises(ConflictError, match="Later queued changes"):
        app.resolve_conflict("a", conflict_id, ConflictStatus.KEEP_REMOTE)
    assert app.storage.get_task("a", "task").metadata.dirty
    assert len(app.storage.list_conflicts("a")) == 1


def test_workspace_lookups_and_freebusy_request_are_account_scoped(app: ApplicationService) -> None:
    app.storage.upsert_account(Account("b", "other@example.test"))
    app.storage.upsert_calendar(
        Calendar("hidden", "a", "Hidden", remote_id="other", selected=False)
    )
    drive_file = DriveFile("doc", "a", "Agenda")
    app.storage.upsert_drive_file(drive_file)
    event = app.create_event(
        "a",
        "cal",
        "Meeting",
        EventDateTime(DateTimeKind.DATE, date(2026, 8, 21)),
        EventDateTime(DateTimeKind.DATE, date(2026, 8, 22)),
    )
    event = replace(event, remote_id="remote-event")
    app.storage.upsert_event(event)

    assert [account.id for account in app.list_accounts()] == ["b", "a"]
    assert app.account("a") is not None
    assert app.calendar("a", "cal") is not None
    assert app.calendar("b", "cal") is None
    assert app.drive_file("a", "doc") == drive_file
    assert app.drive_file("b", "doc") is None
    assert app.event_by_remote_id("a", "remote-event") == event
    assert app.event_by_remote_id("b", "remote-event") is None
    assert app.freebusy_request(
        "a", datetime(2026, 8, 21, 9, tzinfo=UTC), datetime(2026, 8, 21, 10, tzinfo=UTC)
    ) == {
        "timeMin": "2026-08-21T09:00:00Z",
        "timeMax": "2026-08-21T10:00:00Z",
        "items": [{"id": "remote-cal"}],
    }


def test_task_write_is_optimistic_and_transactional(app: ApplicationService) -> None:
    task = app.create_task(
        "a",
        "inbox",
        "Pay bill",
        due=date(2026, 8, 22),
        due_time_zone="Asia/Singapore",
        priority=TaskPriority.HIGH,
    )
    assert app.storage.get_task("a", task.id) == task
    mutation = app.storage.pending_mutations("a")[0]
    assert mutation.entity_id == task.id
    assert mutation.payload["body"]["due"].startswith("2026-08-22T00:00:00")
    assert "priority" not in mutation.payload["body"]

    original = app.storage.enqueue

    def fail_enqueue(*_args, **_kwargs):
        raise RuntimeError("outbox unavailable")

    app.storage.enqueue = fail_enqueue  # type: ignore[method-assign]
    with pytest.raises(RuntimeError):
        app.create_task("a", "inbox", "Must roll back", id="rollback")
    app.storage.enqueue = original  # type: ignore[method-assign]
    assert app.storage.get_task("a", "rollback") is None


def test_notes_projection_and_date_only_constraint(app: ApplicationService) -> None:
    app.set_notes_projection("a", NotesProjection.DISABLED)
    task = app.create_task("a", "inbox", "Private", notes="secret")
    assert app.storage.get_task("a", task.id).notes == "secret"
    assert "notes" not in app.storage.pending_mutations("a")[-1].payload["body"]
    with pytest.raises(ValueError, match="date-only"):
        app.create_task("a", "inbox", "Bad due", due=datetime.now(UTC))  # type: ignore[arg-type]


def test_split_recurring_event_preserves_the_remaining_count(app: ApplicationService) -> None:
    series = app.create_event(
        "a",
        "cal",
        "Finite standup",
        EventDateTime(DateTimeKind.DATE, date(2026, 8, 1)),
        EventDateTime(DateTimeKind.DATE, date(2026, 8, 2)),
        recurrence=("RRULE:FREQ=DAILY;COUNT=3",),
        id="series",
    )
    app.storage.upsert_event(replace(series, remote_id="series-remote"))
    occurrence = Event(
        "occurrence",
        "a",
        "cal",
        "Finite standup",
        EventDateTime(DateTimeKind.DATE, date(2026, 8, 2)),
        EventDateTime(DateTimeKind.DATE, date(2026, 8, 3)),
        remote_id="occurrence-remote",
        canonical_id="series-remote",
        derived=True,
    )
    app.storage.upsert_event(occurrence)

    old, new = app.split_recurring_event("a", occurrence.id)

    assert old.recurrence == ("RRULE:FREQ=DAILY;UNTIL=20260801",)
    assert new.start.value == date(2026, 8, 2)
    assert new.recurrence == ("RRULE:FREQ=DAILY;COUNT=2",)


def test_split_recurring_event_handles_bysetpos_and_date_exceptions(
    app: ApplicationService,
) -> None:
    series = app.create_event(
        "a",
        "cal",
        "Last weekday",
        EventDateTime(DateTimeKind.DATETIME, datetime(2026, 1, 1, 9, tzinfo=UTC)),
        EventDateTime(DateTimeKind.DATETIME, datetime(2026, 1, 1, 10, tzinfo=UTC)),
        recurrence=(
            "RRULE:FREQ=MONTHLY;BYDAY=MO,TU,WE,TH,FR;BYSETPOS=-1;COUNT=4",
            "EXDATE:20260227T090000Z",
            "RDATE:20260302T090000Z",
        ),
        id="series",
    )
    app.storage.upsert_event(replace(series, remote_id="series-remote"))
    occurrence = Event(
        "occurrence",
        "a",
        "cal",
        "Last weekday",
        EventDateTime(DateTimeKind.DATETIME, datetime(2026, 3, 31, 9, tzinfo=UTC)),
        EventDateTime(DateTimeKind.DATETIME, datetime(2026, 3, 31, 10, tzinfo=UTC)),
        remote_id="occurrence-remote",
        canonical_id="series-remote",
        derived=True,
    )
    app.storage.upsert_event(occurrence)

    old, new = app.split_recurring_event("a", occurrence.id)

    assert old.recurrence == (
        "RRULE:FREQ=MONTHLY;BYDAY=MO,TU,WE,TH,FR;BYSETPOS=-1;UNTIL=20260331T085959Z",
        "EXDATE:20260227T090000Z",
        "RDATE:20260302T090000Z",
    )
    assert new.recurrence == ("RRULE:FREQ=MONTHLY;BYDAY=MO,TU,WE,TH,FR;BYSETPOS=-1;COUNT=2",)


def test_unexpanded_master_occurrence_can_be_overridden(app: ApplicationService) -> None:
    master = app.create_event(
        "a",
        "cal",
        "Daily standup",
        EventDateTime(DateTimeKind.DATETIME, datetime(2026, 8, 1, 9, tzinfo=UTC)),
        EventDateTime(DateTimeKind.DATETIME, datetime(2026, 8, 1, 10, tzinfo=UTC)),
        recurrence=("RRULE:FREQ=DAILY",),
    )

    updated, override = app.override_unexpanded_event_occurrence(
        "a",
        master.id,
        start=EventDateTime(DateTimeKind.DATETIME, datetime(2026, 8, 1, 11, tzinfo=UTC)),
        end=EventDateTime(DateTimeKind.DATETIME, datetime(2026, 8, 1, 12, tzinfo=UTC)),
    )

    assert updated.recurrence == ("RRULE:FREQ=DAILY", "EXDATE:20260801T090000Z")
    assert override.recurrence == ()
    assert override.start.value == datetime(2026, 8, 1, 11, tzinfo=UTC)
    assert [item.operation.value for item in app.storage.pending_mutations("a")[-2:]] == [
        "update",
        "create",
    ]


def test_batch_task_move_preflights_hierarchies_and_queues_in_order(
    app: ApplicationService,
) -> None:
    archive = app.create_task_list("a", "Archive")
    first = app.create_task("a", "inbox", "First", id="first")
    second = app.create_task("a", "inbox", "Second", id="second")
    parent = app.create_task("a", "inbox", "Parent", id="parent")
    child = app.create_task("a", "inbox", "Child", parent_id=parent.id, id="child")

    with pytest.raises(ValueError, match="both a parent and its subtask"):
        app.preview_task_move("a", [parent.id, child.id], archive.id)
    with pytest.raises(ValueError, match="tasks with subtasks"):
        app.preview_task_move("a", [parent.id], archive.id)
    with pytest.raises(NotFoundError, match="does not exist"):
        app.complete_tasks("a", [first.id, "missing"])
    assert app.storage.get_task("a", first.id).status.value == "needsAction"  # type: ignore[union-attr]

    preview = app.preview_task_move("a", [second.id, first.id], archive.id)
    assert [item.id for item in preview.items] == [second.id, first.id]
    moved = app.move_tasks("a", [second.id, first.id, second.id], archive.id)
    assert [item.id for item in moved] == [second.id, first.id]
    assert all(item.list_id == archive.id and item.parent_id is None for item in moved)
    moves = [item for item in app.storage.pending_mutations("a") if item.operation.value == "move"]
    assert [item.entity_id for item in moves] == [second.id, first.id]


def test_batch_actions_preflight_all_targets_before_mutating(app: ApplicationService) -> None:
    first = app.create_task("a", "inbox", "First", id="first")
    second = app.create_task("a", "inbox", "Second", id="second")
    before = len(app.storage.pending_mutations("a"))

    preview = app.preview_task_completion("a", [second.id, first.id], completed=True)
    assert preview.action == "complete"
    assert [item.id for item in preview.items] == [second.id, first.id]
    with pytest.raises(NotFoundError, match="does not exist"):
        app.preview_task_deletion("a", [first.id, "missing"])
    assert app.storage.get_task("a", first.id).status is TaskStatus.NEEDS_ACTION  # type: ignore[union-attr]
    assert len(app.storage.pending_mutations("a")) == before

    app.complete_tasks("a", [second.id, first.id])
    assert app.storage.get_task("a", first.id).status is TaskStatus.COMPLETED  # type: ignore[union-attr]


def test_large_task_batch_preserves_the_requested_serial_order(app: ApplicationService) -> None:
    tasks = [
        Task(f"task-{index}", "a", "inbox", f"Task {index}", remote_id=f"remote-{index}")
        for index in range(64)
    ]
    for task in tasks:
        app.storage.upsert_task(task)
    ids = [task.id for task in reversed(tasks)]

    preview = app.preview_task_completion("a", ids, completed=True)
    assert len(preview.items) == 64
    completed = app.complete_tasks("a", ids)

    assert [task.id for task in completed] == ids
    updates = [
        mutation.entity_id
        for mutation in app.storage.pending_mutations("a")
        if mutation.operation.value == "update"
    ]
    assert updates[-64:] == ids


def test_batch_event_move_rejects_google_unsupported_event_types(app: ApplicationService) -> None:
    destination = app.create_calendar("a", "Archive")
    event = app.create_event(
        "a",
        "cal",
        "Focus",
        EventDateTime(DateTimeKind.DATETIME, datetime(2026, 8, 21, 9, tzinfo=UTC)),
        EventDateTime(DateTimeKind.DATETIME, datetime(2026, 8, 21, 10, tzinfo=UTC)),
        event_type="focusTime",
    )
    with pytest.raises(ValueError, match="only moves default events"):
        app.move_events("a", [event.id], destination.id)
    assert app.storage.get_event("a", event.id).calendar_id == "cal"  # type: ignore[union-attr]


def test_batch_task_move_persists_offline_across_a_restart(tmp_path: Path) -> None:
    path = tmp_path / "batch-move.db"
    with Storage(path) as storage:
        storage.upsert_account(Account("a", "private@example.test"))
        storage.upsert_task_list(TaskList("inbox", "a", "Inbox"))
        storage.upsert_task_list(TaskList("archive", "a", "Archive"))
        service = ApplicationService(storage)
        first = service.create_task("a", "inbox", "First", id="first")
        second = service.create_task("a", "inbox", "Second", id="second")
        service.move_tasks("a", [first.id, second.id], "archive")
    with Storage(path) as restarted:
        assert [task.list_id for task in restarted.list_tasks("a")] == ["archive", "archive"]
        moves = [
            mutation
            for mutation in restarted.pending_mutations("a")
            if mutation.operation.value == "move"
        ]
        assert [mutation.entity_id for mutation in moves] == ["first", "second"]


def test_calendar_list_preferences_and_event_rich_clears(app: ApplicationService) -> None:
    calendar = app.update_calendar(
        "a",
        "cal",
        color="#112233",
        selected=False,
        notification_settings=({"type": "eventCreation", "method": "email"},),
        default_reminders=(ReminderOverride("popup", 10),),
    )
    assert calendar.color == "#112233"
    mutations = app.storage.pending_mutations("a")
    assert mutations[-1].payload["resource"] == "calendar-list"

    start = EventDateTime(DateTimeKind.DATETIME, datetime(2026, 8, 21, 9, tzinfo=UTC))
    end = EventDateTime(DateTimeKind.DATETIME, datetime(2026, 8, 21, 10, tzinfo=UTC))
    event = app.create_event(
        "a",
        "cal",
        "Review",
        start,
        end,
        description="Details",
        recurrence=("RRULE:FREQ=WEEKLY",),
        attendees=({"email": "guest@example.test"},),
    )
    updated = app.update_event(
        "a",
        event.id,
        description=None,
        recurrence=(),
        attendees=(),
        conference=None,
    )
    assert updated.description is None
    body = app.storage.pending_mutations("a")[-1].payload["body"]
    assert body["description"] is None
    assert body["recurrence"] == []
    assert body["attendees"] == []
    assert body["conferenceData"] is None


def test_recurring_event_changes_stale_cached_instance_ranges(app: ApplicationService) -> None:
    start = datetime(2026, 8, 21, tzinfo=UTC)
    app.storage.replace_cached_instances("a", "cal", start, start.replace(day=28), [])
    event = app.create_event(
        "a",
        "cal",
        "Standup",
        EventDateTime(DateTimeKind.DATETIME, start),
        EventDateTime(DateTimeKind.DATETIME, start.replace(hour=1)),
        recurrence=("RRULE:FREQ=DAILY",),
    )
    assert app.storage.list_instance_ranges("a", "cal")[0]["state"] == "stale"

    app.storage.replace_cached_instances("a", "cal", start, start.replace(day=28), [])
    app.update_event("a", event.id, summary="Daily standup")
    assert app.storage.list_instance_ranges("a", "cal")[0]["state"] == "stale"


def test_event_duplicate_invitations_and_cached_instance_agenda(app: ApplicationService) -> None:
    start = EventDateTime(DateTimeKind.DATETIME, datetime(2026, 8, 21, 9, tzinfo=UTC))
    end = EventDateTime(DateTimeKind.DATETIME, datetime(2026, 8, 21, 10, tzinfo=UTC))
    series = app.create_event(
        "a",
        "cal",
        "Standup",
        start,
        end,
        recurrence=("RRULE:FREQ=DAILY",),
        attendees=({"email": "guest@example.test"},),
        id="series",
    )
    copied = app.duplicate_event("a", series.id)
    assert copied.recurrence == ()
    assert copied.attendees == ()

    instance = app.storage.get_event("a", series.id)
    assert instance is not None
    app.storage.upsert_event(replace(instance, remote_id="series-remote"))
    instance = app.storage.get_event("a", series.id)
    assert instance is not None
    app.storage.upsert_event(
        replace(
            instance,
            id="instance",
            remote_id="instance-remote",
            canonical_id="series-remote",
            occurrence_id="2026-08-21T09:00:00Z",
            recurrence=(),
            derived=True,
        )
    )
    agenda = app.agenda_events(
        "a", start=date(2026, 8, 21), end=date(2026, 8, 22), calendar_id="cal"
    )
    assert {item.id for item in agenda} == {"instance", copied.id}


def test_schedule_search_orphan_and_undo_redo(app: ApplicationService) -> None:
    task = app.create_task("a", "inbox", "Deep work", priority="high")
    start = EventDateTime(
        DateTimeKind.DATETIME,
        datetime(2026, 8, 21, 9, tzinfo=UTC),
        "UTC",
    )
    end = EventDateTime(
        DateTimeKind.DATETIME,
        datetime(2026, 8, 21, 10, tzinfo=UTC),
        "UTC",
    )
    event, link = app.schedule_task("a", task.id, "cal", start, end)
    assert link.event_id == event.id
    assert app.search("a", "type:task priority:high Deep")[0].item.id == task.id

    app.delete_event("a", event.id)
    assert app.list_task_event_links("a", orphaned_only=True)[0].orphaned
    assert app.undo("a") is not None
    assert app.storage.get_event("a", event.id).metadata.deleted is False
    assert app.redo("a") is not None
    assert app.storage.get_event("a", event.id).metadata.deleted is True


def test_indexed_workspace_search_covers_every_user_facing_local_entity(
    app: ApplicationService,
) -> None:
    app.storage.upsert_task_list(TaskList("research", "a", "Research needle"))
    app.storage.upsert_calendar(
        Calendar("secondary", "a", "Calendar needle", description="calendarbodyneedle")
    )
    task = app.create_task("a", "research", "Task needle", notes="notebodyneedle")
    event = app.create_event(
        "a",
        "secondary",
        "Event needle",
        EventDateTime(DateTimeKind.DATETIME, datetime(2026, 8, 21, 9, tzinfo=UTC)),
        EventDateTime(DateTimeKind.DATETIME, datetime(2026, 8, 21, 10, tzinfo=UTC)),
        description="eventbodyneedle",
    )
    app.storage.upsert_drive_file(DriveFile("drive", "a", "Drive needle"))
    saved = app.save_search("a", "Saved needle", "task:needle")
    conflict_id = app.storage.add_conflict(
        Conflict(
            None,
            "a",
            EntityType.TASK,
            task.id,
            {"detail": "conflictbodyneedle"},
            {"detail": "remote"},
        )
    )

    results = app.search("a", "needle")
    assert {result.kind for result in results} == {
        "task",
        "event",
        "calendar",
        "task-list",
        "drive",
        "saved-search",
    }
    assert [result.item.id for result in app.search("a", "type:list needle")] == ["research"]
    assert [result.item.id for result in app.search("a", "body:notebodyneedle")] == [task.id]
    assert [result.item.id for result in app.search("a", "body:eventbodyneedle")] == [event.id]
    assert [result.item.id for result in app.search("a", "body:conflictbodyneedle")] == [
        conflict_id
    ]
    assert {result.kind for result in app.search("a", "source:local")} == {
        "conflict",
        "saved-search",
    }
    assert app.search("a", "type:saved needle")[0].item.id == saved.id

    app.update_task("a", task.id, title="Renamed result")
    assert not app.search("a", "Task needle")
    assert app.search("a", "Renamed result")[0].item.id == task.id
    app.delete_task("a", task.id)
    assert not app.search("a", "Renamed result")


def test_workspace_search_uses_its_index_instead_of_scanning_entity_tables(
    app: ApplicationService,
) -> None:
    task = app.create_task("a", "inbox", "Indexed needle")

    def fail_scan(*_args: object, **_kwargs: object) -> object:
        raise AssertionError("workspace search must not scan every entity")

    app.storage.list_tasks = fail_scan  # type: ignore[method-assign]
    app.storage.list_events = fail_scan  # type: ignore[method-assign]
    assert app.search("a", "needle")[0].item.id == task.id


def test_import_apply_is_atomic_and_diagnostics_are_redacted(
    app: ApplicationService,
) -> None:
    preview = app.preview_import(
        "tasks.json",
        '{"version":1,"records":[{"kind":"task","title":"Imported","priority":"low"}]}',
    )
    result = app.apply_import("a", preview, default_task_list_id="inbox")
    assert result.tasks[0].priority is TaskPriority.LOW
    diagnostics = app.diagnostics()
    assert diagnostics["path"] == "<redacted>"
    assert "private@example.test" not in str(diagnostics)


def test_event_fields_round_trip_through_storage_and_outbox(app: ApplicationService) -> None:
    start = EventDateTime(DateTimeKind.DATETIME, datetime(2026, 8, 21, 9, tzinfo=UTC), "UTC")
    end = EventDateTime(DateTimeKind.DATETIME, datetime(2026, 8, 21, 10, tzinfo=UTC), "UTC")
    event = app.create_event(
        "a",
        "cal",
        "Focus",
        start,
        end,
        attendees=({"email": "guest@example.test", "optional": True},),
        reminder_use_default=False,
        reminder_overrides=(ReminderOverride("popup", 10),),
        event_type="focusTime",
        visibility="private",
        transparency="opaque",
        color_id="5",
        attachments=({"fileId": "drive-1", "title": "Brief"},),
        conference={"createRequest": {"requestId": "stable-request"}},
        guests_can_modify=False,
        focus_time_properties={"autoDeclineMode": "declineAllConflictingInvitations"},
        send_updates="all",
        supports_attachments=True,
        conference_data_version=1,
    )
    assert app.storage.get_event("a", event.id) == event
    payload = app.storage.pending_mutations("a")[-1].payload
    assert payload["body"]["attendees"][0]["optional"] is True
    assert payload["body"]["reminders"]["overrides"] == [{"method": "popup", "minutes": 10}]
    assert payload["body"]["focusTimeProperties"]["autoDeclineMode"].startswith("decline")
    assert (
        payload["send_updates"],
        payload["supports_attachments"],
        payload["conference_data_version"],
    ) == ("all", True, 1)


def test_calendar_list_operations_notes_projection_and_schedule_idempotency(
    app: ApplicationService,
) -> None:
    subscribed = app.subscribe_calendar("a", "shared@example.test", summary="Shared")
    assert app.storage.pending_mutations("a")[-1].operation.value == "subscribe"
    app.remove_calendar_from_list("a", subscribed.id)
    assert app.storage.pending_mutations("a")[-1].operation.value == "remove"

    note = app.create_task("a", "inbox", "Undated root")
    dated = app.create_task("a", "inbox", "Dated", due=date(2026, 8, 22))
    app.set_notes_projection("a", "notes-only")
    assert {item.id for item in app.notes_listing("a")} == {note.id}
    assert {item.id for item in app.task_listing("a")} == {dated.id}

    start = EventDateTime(DateTimeKind.DATETIME, datetime(2026, 8, 21, 9, tzinfo=UTC), "UTC")
    end = EventDateTime(DateTimeKind.DATETIME, datetime(2026, 8, 21, 10, tzinfo=UTC), "UTC")
    first, _ = app.schedule_task("a", dated.id, "cal", start, end)
    moved, _ = app.schedule_task("a", dated.id, "cal", start, end)
    assert moved.id == first.id
    assert len([link for link in app.list_task_event_links("a") if link.task_id == dated.id]) == 1


def test_completed_recurring_task_creates_one_successor(app: ApplicationService) -> None:
    series = "3a21dc8d-2cb4-4b9a-980f-7aaf75ae2f43"
    marker = TaskRecurrenceMarker(
        series,
        f"{series}:0",
        0,
        "daily",
        1,
        "2026-08-21",
        "UTC",
        RecurrenceEnd("count", count=2),
        "",
        (),
        (),
        "Daily task",
        "2026-08-21",
        "none",
    )
    notes = serialize_task_notes("memo", marker).notes
    task = app.create_task(
        "a", "inbox", "Daily task", notes=notes, due=date(2026, 8, 21), due_time_zone="UTC"
    )
    result = app.complete_tasks_detailed("a", [task.id])
    assert [item.id for item in result.tasks] == [task.id]
    assert len(result.successors) == 1
    app.reconcile_task_recurrence("a")
    occurrences = [
        parse_task_recurrence_notes(item.notes or "").marker
        for item in app.storage.list_tasks("a")
        if parse_task_recurrence_notes(item.notes or "").marker
    ]
    assert [item.occurrence_id for item in occurrences].count(f"{series}:1") == 1


def test_undo_redo_unpushed_create_restores_projection_and_outbox(
    app: ApplicationService,
) -> None:
    task = app.create_task("a", "inbox", "Undo me")
    app.undo("a")
    assert app.storage.get_task("a", task.id) is None
    assert all(item.entity_id != task.id for item in app.storage.pending_mutations("a"))
    app.redo("a")
    assert app.storage.get_task("a", task.id).title == "Undo me"
    assert app.storage.pending_mutations("a")[-1].operation.value == "create"
