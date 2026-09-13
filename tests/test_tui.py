from __future__ import annotations

import asyncio
import json
from collections.abc import Awaitable, Callable, Iterator
from contextlib import contextmanager
from dataclasses import replace
from datetime import UTC, date, datetime
from pathlib import Path
from threading import Event as ThreadEvent
from time import sleep

import pytest
from rich.console import Console
from rich.style import Style
from rich.text import Text
from textual import events
from textual.app import SuspendNotSupported
from textual.color import Color
from textual.containers import Horizontal
from textual.widgets import Button, Input, Label, ListView, Select, Static

from hcb.config import Config, KeyBindings, Theme, ThemeColors, save
from hcb.environment import LocalEnvironment
from hcb.models import (
    Account,
    Conflict,
    DateTimeKind,
    DriveFile,
    EntityType,
    EventDateTime,
    Preferences,
    ReminderOverride,
    Task,
)
from hcb.paths import AppPaths
from hcb.runtime import Runtime
from hcb.storage import Storage
from hcb.themes import apply_preset, preset
from hcb.tui import (
    PALETTE_COMMANDS,
    BatchActionScreen,
    BatchMoveScreen,
    CalendarGrid,
    CalendarOverflowScreen,
    CalendarScreen,
    ConfirmScreen,
    ConflictScreen,
    EditorScreen,
    EntityRow,
    EventEditorScreen,
    FindTimeScreen,
    GoogleSetupScreen,
    HcbApp,
    HelpScreen,
    ImportScreen,
    ItemViewScreen,
    LoadingScreen,
    OnboardingScreen,
    PaletteScreen,
    RsvpScreen,
    ScheduleScreen,
    SettingsScreen,
    TerminalTextArea,
    WorkspaceRow,
    WorkspaceTable,
    emoji_suggestions,
    google_maps_url,
    linkify_urls,
    recurrence_frequency,
    recurrence_spec,
    recurrence_summary,
    recurrence_with_frequency,
    recurrence_with_spec,
    render_readonly_markup,
)


def run[T](coro: Awaitable[T]) -> T:
    return asyncio.run(coro)


def seeded_runtime(tmp_path: Path, *, environ: dict[str, str] | None = None) -> Runtime:
    paths = AppPaths(tmp_path / "config", tmp_path / "data", tmp_path / "cache")
    with Storage(paths.database_file) as storage:
        storage.upsert_account(Account("work", "me@example.com"))
    runtime = Runtime(paths, environ=environ or {})
    task_list = runtime.application.create_task_list("work", "Inbox")
    runtime.application.create_task("work", task_list.id, "Ship TUI", notes="Pilot coverage")
    runtime.application.create_calendar("work", "Personal")
    return runtime


def app_test(
    app: HcbApp,
    body: Callable[[object], Awaitable[None]],
    *,
    size: tuple[int, int] = (110, 34),
) -> None:
    async def scenario() -> None:
        async with app.run_test(size=size) as pilot:
            await pilot.pause()
            await body(pilot)

    run(scenario())


def workspace_rows(app: HcbApp) -> tuple[WorkspaceRow, ...]:
    return app.query_one("#content", WorkspaceTable).workspace_rows


async def activate_palette(pilot: object, app: HcbApp, query_text: str) -> None:
    await pilot.press("/")  # type: ignore[attr-defined]
    await pilot.pause()  # type: ignore[attr-defined]
    query = app.screen.query_one("#palette-query", Input)
    query.value = query_text
    await pilot.pause()  # type: ignore[attr-defined]
    await pilot.press("enter")  # type: ignore[attr-defined]
    await pilot.pause()  # type: ignore[attr-defined]


def test_startup_surface_switching_and_narrow_layout(tmp_path: Path) -> None:
    app = HcbApp(seeded_runtime(tmp_path))

    async def assertions(pilot: object) -> None:
        assert "Tasks" in str(app.query_one("#surface-title", Static).render())
        assert len(workspace_rows(app)) == 1
        await pilot.press("3")  # type: ignore[attr-defined]
        await pilot.pause()  # type: ignore[attr-defined]
        assert app.surface == "Agenda"
        await pilot.resize_terminal(58, 24)  # type: ignore[attr-defined]
        await pilot.pause()  # type: ignore[attr-defined]
        assert app.has_class("very-narrow")

    app_test(app, assertions)


def test_workspace_splitter_resizes_sidebar_and_respects_narrow_layout(tmp_path: Path) -> None:
    app = HcbApp(seeded_runtime(tmp_path))

    async def assertions(pilot: object) -> None:
        sidebar = app.query_one("#sidebar")
        sidebar_width = sidebar.size.width

        await pilot.press("ctrl+alt+right", "ctrl+alt+right")  # type: ignore[attr-defined]
        await pilot.pause()  # type: ignore[attr-defined]
        assert sidebar.size.width == sidebar_width + 4

        await pilot.mouse_down("#sidebar-resize", offset=(0, 4))  # type: ignore[attr-defined]
        await pilot.mouse_up("#center", offset=(5, 4))  # type: ignore[attr-defined]
        await pilot.pause()  # type: ignore[attr-defined]
        assert sidebar.size.width > sidebar_width + 4
        assert app._resize_target is None

        await pilot.resize_terminal(58, 24)  # type: ignore[attr-defined]
        await pilot.pause()  # type: ignore[attr-defined]
        assert app.query_one("#sidebar-resize", Static).display is False

    app_test(app, assertions, size=(120, 34))


def test_notes_surface_handles_tasks_without_notes(tmp_path: Path) -> None:
    runtime = seeded_runtime(tmp_path)
    task_list = runtime.storage.list_task_lists("work")[0]
    runtime.application.create_task("work", task_list.id, "Untitled note")
    app = HcbApp(runtime)

    async def assertions(pilot: object) -> None:
        app.action_surface("Notes")
        await pilot.pause()  # type: ignore[attr-defined]
        labels = [str(row.label) for row in workspace_rows(app)]
        assert any("Untitled note" in label for label in labels)

    app_test(app, assertions)


def test_agenda_uses_the_configured_friendly_date_time_format(tmp_path: Path) -> None:
    runtime = seeded_runtime(tmp_path)
    save(
        Config(preferences=Preferences(time_zone="Asia/Singapore", date_time_format="friendly")),
        runtime.paths.config_file,
    )
    calendar_id = runtime.storage.list_calendars("work")[0].id
    runtime.application.create_event(
        "work",
        calendar_id,
        "Evening meeting",
        EventDateTime(DateTimeKind.DATETIME, datetime(2026, 5, 26, 11, 23, tzinfo=UTC)),
        EventDateTime(DateTimeKind.DATETIME, datetime(2026, 5, 26, 12, 23, tzinfo=UTC)),
    )
    app = HcbApp(runtime, selected_date=date(2026, 5, 25))

    async def assertions(pilot: object) -> None:
        await pilot.press("3")  # type: ignore[attr-defined]
        await pilot.pause()  # type: ignore[attr-defined]
        labels = [str(row.label) for row in workspace_rows(app)]
        assert any("26 May 2026, 7:23pm" in label for label in labels), labels
        assert all("T11:23:00" not in label for label in labels)

    app_test(app, assertions)


def test_workspace_rows_place_colored_indicators_between_dates_and_titles(tmp_path: Path) -> None:
    runtime = seeded_runtime(tmp_path)
    theme = apply_preset(Theme(), "Flexoki Light")
    theme = replace(
        theme,
        roles=replace(
            theme.roles,
            completed_item=replace(theme.roles.completed_item, color="#d14d41"),
        ),
    )
    save(Config(theme=theme), runtime.paths.config_file)
    task_list = runtime.storage.list_task_lists("work")[0]
    dated_task = runtime.application.create_task(
        "work", task_list.id, "Dated task", due=date(2026, 8, 24)
    )
    runtime.application.complete_task("work", dated_task.id)
    note = runtime.application.create_task("work", task_list.id, "Undated note")
    calendar = runtime.application.create_calendar("work", "Color calendar", color="#123456")
    event = runtime.application.create_event(
        "work",
        calendar.id,
        "Colored event",
        EventDateTime(DateTimeKind.DATETIME, datetime(2026, 8, 24, 9, tzinfo=UTC)),
        EventDateTime(DateTimeKind.DATETIME, datetime(2026, 8, 24, 10, tzinfo=UTC)),
    )
    app = HcbApp(runtime, selected_date=date(2026, 8, 24))

    def indicator_style(item_id: str) -> Style:
        content = app.query_one("#content", WorkspaceTable)
        index = next(
            index for index, row in enumerate(content.workspace_rows) if row.item_id == item_id
        )
        label = content._rendered_row_label(index)
        offset = label.plain.index("●")
        return label.get_style_at_offset(Console(), offset)

    def indicator_color(item_id: str) -> str:
        color = indicator_style(item_id).color
        assert color is not None
        return color.get_truecolor().hex

    async def assertions(pilot: object) -> None:
        dated_row = next(row for row in workspace_rows(app) if row.item_id == dated_task.id)
        assert f"{app.format_date(dated_task.due)}  ●  {dated_task.title}" in dated_row.label.plain
        assert indicator_color(dated_task.id) == theme.colors.accent

        app.action_surface("Notes")
        await pilot.pause()  # type: ignore[attr-defined]
        note_row = next(row for row in workspace_rows(app) if row.item_id == note.id)
        assert f"●  {note.title}" in note_row.label.plain
        assert indicator_color(note.id) == theme.colors.accent

        app.action_surface("Agenda")
        await pilot.pause()  # type: ignore[attr-defined]
        event_row = next(row for row in workspace_rows(app) if row.item_id == event.id)
        assert (
            f"{app.format_date_time(event.start.value)}  ●  {event.summary}"
            in event_row.label.plain
        )
        content = app.query_one("#content", WorkspaceTable)
        event_index = next(
            index for index, row in enumerate(content.workspace_rows) if row.item_id == event.id
        )
        content.select_workspace_row(event_index, Style(bold=True, reverse=True))
        assert indicator_color(event.id) == "#123456"
        selected_indicator = indicator_style(event.id)
        assert not selected_indicator.reverse
        assert selected_indicator.bgcolor is not None
        assert selected_indicator.bgcolor.get_truecolor().hex == theme.colors.text

    app_test(app, assertions)


def test_day_surface_includes_timed_events_ending_on_the_selected_day(tmp_path: Path) -> None:
    runtime = seeded_runtime(tmp_path)
    calendar_id = runtime.storage.list_calendars("work")[0].id
    event = runtime.application.create_event(
        "work",
        calendar_id,
        "Evening climb",
        EventDateTime(DateTimeKind.DATETIME, datetime(2026, 8, 24, 17, 30, tzinfo=UTC)),
        EventDateTime(DateTimeKind.DATETIME, datetime(2026, 8, 24, 20, 30, tzinfo=UTC)),
    )
    app = HcbApp(runtime, selected_date=date(2026, 8, 24))

    async def assertions(pilot: object) -> None:
        app.action_surface("Day")
        await pilot.pause()  # type: ignore[attr-defined]
        assert event.id in {row.item_id for row in workspace_rows(app)}

    app_test(app, assertions)


def test_calendar_grids_include_due_tasks_create_from_slots_and_fallback_when_narrow(
    tmp_path: Path,
) -> None:
    runtime = seeded_runtime(tmp_path)
    task_list = runtime.storage.list_task_lists("work")[0]
    due_task = runtime.application.create_task(
        "work", task_list.id, "Calendar task", due=date(2026, 8, 24)
    )
    calendar_id = runtime.storage.list_calendars("work")[0].id
    runtime.application.create_event(
        "work",
        calendar_id,
        "Calendar event",
        EventDateTime(DateTimeKind.DATETIME, datetime(2026, 8, 24, 9, tzinfo=UTC)),
        EventDateTime(DateTimeKind.DATETIME, datetime(2026, 8, 24, 10, tzinfo=UTC)),
    )
    app = HcbApp(runtime, selected_date=date(2026, 8, 24))

    async def assertions(pilot: object) -> None:
        await pilot.press("4")  # type: ignore[attr-defined]
        await pilot.pause()  # type: ignore[attr-defined]
        grid = app.query_one("#calendar-content", CalendarGrid)
        assert grid.display
        assert {item.item_id for item in grid.items} >= {due_task.id}

        app.action_create_calendar_slot(date(2026, 8, 24), 9 * 60, all_day=False)
        await pilot.pause()  # type: ignore[attr-defined]
        assert isinstance(app.screen, EventEditorScreen)
        assert app.screen.query_one("#event-start", Input).value.startswith("2026-08-24T09:00")
        assert app.screen.query_one("#event-end", Input).value.startswith("2026-08-24T10:00")
        app.pop_screen()

        app.apply_calendar_item_change(
            next(item for item in grid.items if item.item_id == due_task.id),
            day=date(2026, 8, 25),
            minute=None,
            operation="move",
        )
        await pilot.pause()  # type: ignore[attr-defined]
        assert runtime.storage.get_task("work", due_task.id).due == date(2026, 8, 25)  # type: ignore[union-attr]

    app_test(app, assertions, size=(130, 34))

    narrow = HcbApp(Runtime(runtime.paths, environ={}), selected_date=date(2026, 8, 24))

    async def narrow_assertions(pilot: object) -> None:
        await pilot.press("6")  # type: ignore[attr-defined]
        await pilot.pause()  # type: ignore[attr-defined]
        assert not narrow.query_one("#calendar-content", CalendarGrid).display
        assert narrow.query_one("#content", WorkspaceTable).display

    app_test(narrow, narrow_assertions, size=(80, 34))


def test_calendar_drag_shows_live_valid_and_invalid_drop_feedback(tmp_path: Path) -> None:
    runtime = seeded_runtime(tmp_path)
    calendar_id = runtime.storage.list_calendars("work")[0].id
    event = runtime.application.create_event(
        "work",
        calendar_id,
        "Drag preview",
        EventDateTime(DateTimeKind.DATETIME, datetime(2026, 8, 24, 9, tzinfo=UTC)),
        EventDateTime(DateTimeKind.DATETIME, datetime(2026, 8, 24, 11, tzinfo=UTC)),
    )
    app = HcbApp(runtime, selected_date=date(2026, 8, 24))

    async def assertions(pilot: object) -> None:
        await pilot.press("4")  # type: ignore[attr-defined]
        await pilot.pause()  # type: ignore[attr-defined]
        grid = app.query_one("#calendar-content", CalendarGrid)
        hit = next(hit for hit in grid._hits if hit.block.item.item_id == event.id)
        grid.on_mouse_down(events.MouseDown(grid, hit.x, hit.y + 1, 0, 0, 1, False, False, False))

        target_y = grid._timed_start + 13 * 2
        grid.on_mouse_move(
            events.MouseMove(grid, grid._time_axis, target_y, 0, 0, 0, False, False, False)
        )
        assert grid._drag_preview is not None
        assert grid._drag_preview.valid
        assert "→ Drag preview" in grid._line(grid._drag_preview.y).plain

        grid.on_mouse_move(events.MouseMove(grid, 0, target_y, 0, 0, 0, False, False, False))
        assert grid._drag_preview is not None
        assert not grid._drag_preview.valid
        assert "× invalid drop" in grid._line(grid._drag_preview.y).plain

        grid.on_mouse_up(events.MouseUp(grid, 0, target_y, 0, 0, 1, False, False, False))
        assert grid._drag is None
        assert grid._drag_preview is None

    app_test(app, assertions, size=(130, 34))


def test_calendar_empty_slot_drag_creates_an_exact_event_range(tmp_path: Path) -> None:
    app = HcbApp(seeded_runtime(tmp_path), selected_date=date(2026, 8, 24))

    async def assertions(pilot: object) -> None:
        await pilot.press("4")  # type: ignore[attr-defined]
        await pilot.pause()  # type: ignore[attr-defined]
        grid = app.query_one("#calendar-content", CalendarGrid)
        start_y = grid._timed_start + 9 * 2
        end_y = grid._timed_start + 10 * 2
        grid.on_mouse_down(
            events.MouseDown(grid, grid._time_axis, start_y, 0, 0, 1, False, False, False)
        )
        grid.on_mouse_move(
            events.MouseMove(grid, grid._time_axis, end_y, 0, 0, 0, False, False, False)
        )
        assert grid._drag_preview is not None
        assert grid._drag_preview.label == "+ 09:00–10:30 (90m)"
        grid.on_mouse_up(events.MouseUp(grid, grid._time_axis, end_y, 0, 0, 1, False, False, False))
        await pilot.pause()  # type: ignore[attr-defined]
        assert isinstance(app.screen, EventEditorScreen)
        assert app.screen.query_one("#event-start", Input).value.startswith("2026-08-24T09:00")
        assert app.screen.query_one("#event-end", Input).value.startswith("2026-08-24T10:30")

    app_test(app, assertions, size=(130, 34))


def test_calendar_can_schedule_and_resize_task_blocks(tmp_path: Path) -> None:
    runtime = seeded_runtime(tmp_path)
    task_list = runtime.storage.list_task_lists("work")[0]
    task = runtime.application.create_task(
        "work", task_list.id, "Scheduled task", due=date(2026, 8, 24)
    )
    app = HcbApp(runtime, selected_date=date(2026, 8, 24))

    async def assertions(pilot: object) -> None:
        await pilot.press("4")  # type: ignore[attr-defined]
        await pilot.pause()  # type: ignore[attr-defined]
        grid = app.query_one("#calendar-content", CalendarGrid)
        due_item = next(item for item in grid.items if item.item_id == task.id)
        app.apply_calendar_item_change(
            due_item, day=date(2026, 8, 24), minute=9 * 60, operation="move"
        )
        await pilot.pause()  # type: ignore[attr-defined]
        link = next(
            link
            for link in runtime.application.list_task_event_links("work")
            if link.task_id == task.id
        )
        scheduled = runtime.storage.get_event("work", link.event_id)
        assert scheduled is not None
        assert scheduled.start.value == datetime(2026, 8, 24, 9, tzinfo=UTC)
        assert scheduled.end.value == datetime(2026, 8, 24, 10, tzinfo=UTC)

        timed_task = next(
            item for item in grid.items if item.item_id == task.id and not item.all_day
        )
        assert not any(
            item.kind == "event" and item.item_id == link.event_id for item in grid.items
        )
        app.apply_calendar_item_change(
            timed_task, day=date(2026, 8, 24), minute=11 * 60, operation="resize-end"
        )
        await pilot.pause()  # type: ignore[attr-defined]
        resized = runtime.storage.get_event("work", link.event_id)
        assert resized is not None
        assert resized.end.value == datetime(2026, 8, 24, 11, tzinfo=UTC)

    app_test(app, assertions, size=(130, 34))


def test_calendar_keyboard_cursor_creates_moves_and_resizes(tmp_path: Path) -> None:
    runtime = seeded_runtime(tmp_path)
    calendar_id = runtime.storage.list_calendars("work")[0].id
    event = runtime.application.create_event(
        "work",
        calendar_id,
        "Keyboard event",
        EventDateTime(DateTimeKind.DATETIME, datetime(2026, 8, 24, 9, tzinfo=UTC)),
        EventDateTime(DateTimeKind.DATETIME, datetime(2026, 8, 24, 10, tzinfo=UTC)),
    )
    app = HcbApp(runtime, selected_date=date(2026, 8, 24))

    async def assertions(pilot: object) -> None:
        await pilot.press("5")  # type: ignore[attr-defined]
        await pilot.pause()  # type: ignore[attr-defined]
        grid = app.query_one("#calendar-content", CalendarGrid)
        assert grid.has_focus

        await pilot.press("right", "down", "c")  # type: ignore[attr-defined]
        await pilot.pause()  # type: ignore[attr-defined]
        assert isinstance(app.screen, EventEditorScreen)
        assert app.screen.query_one("#event-start", Input).value.startswith("2026-08-25T09:30")
        app.pop_screen()
        await pilot.pause()  # type: ignore[attr-defined]

        grid.select_item(("event", event.id))
        grid._cursor = (date(2026, 8, 25), 11 * 60, False)
        grid.focus()
        await pilot.press("m")  # type: ignore[attr-defined]
        await pilot.pause()  # type: ignore[attr-defined]
        moved = runtime.storage.get_event("work", event.id)
        assert moved is not None
        assert moved.start.value == datetime(2026, 8, 25, 11, tzinfo=UTC)

        grid.select_item(("event", event.id))
        grid.focus()
        await pilot.press("shift+down")  # type: ignore[attr-defined]
        await pilot.pause()  # type: ignore[attr-defined]
        resized = runtime.storage.get_event("work", event.id)
        assert resized is not None
        assert resized.end.value == datetime(2026, 8, 25, 12, 30, tzinfo=UTC)

    app_test(app, assertions, size=(130, 34))


def test_calendar_day_cursor_advances_the_visible_date(tmp_path: Path) -> None:
    app = HcbApp(seeded_runtime(tmp_path), selected_date=date(2026, 8, 24))

    async def assertions(pilot: object) -> None:
        await pilot.press("4", "right")  # type: ignore[attr-defined]
        await pilot.pause()  # type: ignore[attr-defined]
        assert app.selected_date == date(2026, 8, 25)

    app_test(app, assertions, size=(130, 34))


def test_calendar_current_time_rule_refreshes_day_and_month_timelines(tmp_path: Path) -> None:
    today = date.today()
    app = HcbApp(seeded_runtime(tmp_path), selected_date=today)
    now = datetime.combine(today, datetime.min.time(), tzinfo=UTC).replace(hour=9, minute=15)

    async def assertions(pilot: object) -> None:
        await pilot.press("4")  # type: ignore[attr-defined]
        await pilot.pause()  # type: ignore[attr-defined]
        grid = app.query_one("#calendar-content", CalendarGrid)
        grid._refresh_clock(now)
        assert "●" in grid._line(grid._timed_start + 18).plain

        await pilot.press("6")  # type: ignore[attr-defined]
        await pilot.pause()  # type: ignore[attr-defined]
        grid._refresh_clock(now)
        day_index = (today - grid._range.start).days
        week = day_index // 7
        y = 1 + week * grid._month_cell_height + 1 + grid._month_all_day_height + 9
        assert "●" in grid._line(y).plain

    app_test(app, assertions, size=(130, 34))


def test_calendar_can_override_an_unexpanded_master_occurrence(tmp_path: Path) -> None:
    runtime = seeded_runtime(tmp_path)
    calendar_id = runtime.storage.list_calendars("work")[0].id
    master = runtime.application.create_event(
        "work",
        calendar_id,
        "Unexpanded daily",
        EventDateTime(DateTimeKind.DATETIME, datetime(2026, 8, 24, 9, tzinfo=UTC)),
        EventDateTime(DateTimeKind.DATETIME, datetime(2026, 8, 24, 10, tzinfo=UTC)),
        recurrence=("RRULE:FREQ=DAILY",),
    )
    app = HcbApp(runtime, selected_date=date(2026, 8, 24))

    async def assertions(pilot: object) -> None:
        await pilot.press("4")  # type: ignore[attr-defined]
        await pilot.pause()  # type: ignore[attr-defined]
        grid = app.query_one("#calendar-content", CalendarGrid)
        item = next(item for item in grid.items if item.item_id == master.id)
        app.apply_calendar_item_change(
            item,
            day=date(2026, 8, 24),
            minute=11 * 60,
            operation="move",
            scope="this",
        )
        await pilot.pause()  # type: ignore[attr-defined]
        updated = runtime.storage.get_event("work", master.id)
        assert updated is not None
        assert updated.recurrence[-1] == "EXDATE:20260824T090000Z"
        override = next(
            event
            for event in app.cache.events
            if event.id != master.id and event.summary == "Unexpanded daily"
        )
        assert override.start.value == datetime(2026, 8, 24, 11, tzinfo=UTC)
        assert master.id not in {item.item_id for item in grid.items}
        assert override.id in {item.item_id for item in grid.items}

    app_test(app, assertions, size=(130, 34))


def test_month_grid_exposes_hidden_items_through_more_dialog(tmp_path: Path) -> None:
    runtime = seeded_runtime(tmp_path)
    task_list = runtime.storage.list_task_lists("work")[0]
    for index in range(4):
        runtime.application.create_task(
            "work", task_list.id, f"Dense task {index}", due=date(2026, 8, 24)
        )
    app = HcbApp(runtime, selected_date=date(2026, 8, 24))

    async def assertions(pilot: object) -> None:
        await pilot.press("6")  # type: ignore[attr-defined]
        await pilot.pause()  # type: ignore[attr-defined]
        grid = app.query_one("#calendar-content", CalendarGrid)
        hidden = grid._overflow[date(2026, 8, 24)]
        app.on_calendar_grid_overflow_requested(
            CalendarGrid.OverflowRequested(grid, date(2026, 8, 24), hidden)
        )
        await pilot.pause()  # type: ignore[attr-defined]
        assert isinstance(app.screen, CalendarOverflowScreen)
        assert len(app.screen.query_one("#calendar-overflow-items", ListView).children) == len(
            hidden
        )
        await pilot.click("#calendar-overflow-day")  # type: ignore[attr-defined]
        await pilot.pause()  # type: ignore[attr-defined]
        assert app.surface == "Day"
        assert app.selected_date == date(2026, 8, 24)

    app_test(app, assertions, size=(130, 34))


def test_month_grid_places_timed_events_on_a_per_day_timeline(tmp_path: Path) -> None:
    runtime = seeded_runtime(tmp_path)
    calendar_id = runtime.storage.list_calendars("work")[0].id
    morning = runtime.application.create_event(
        "work",
        calendar_id,
        "Morning",
        EventDateTime(DateTimeKind.DATETIME, datetime(2026, 8, 24, 9, tzinfo=UTC)),
        EventDateTime(DateTimeKind.DATETIME, datetime(2026, 8, 24, 10, tzinfo=UTC)),
    )
    afternoon = runtime.application.create_event(
        "work",
        calendar_id,
        "Afternoon",
        EventDateTime(DateTimeKind.DATETIME, datetime(2026, 8, 24, 14, tzinfo=UTC)),
        EventDateTime(DateTimeKind.DATETIME, datetime(2026, 8, 24, 15, tzinfo=UTC)),
    )
    app = HcbApp(runtime, selected_date=date(2026, 8, 24))

    async def assertions(pilot: object) -> None:
        await pilot.press("6")  # type: ignore[attr-defined]
        await pilot.pause()  # type: ignore[attr-defined]
        grid = app.query_one("#calendar-content", CalendarGrid)
        morning_hit = next(hit for hit in grid._hits if hit.block.item.item_id == morning.id)
        afternoon_hit = next(hit for hit in grid._hits if hit.block.item.item_id == afternoon.id)
        assert afternoon_hit.y - morning_hit.y == 5
        assert grid._slot(morning_hit.x, morning_hit.y) == (date(2026, 8, 24), 9 * 60, False)
        assert grid._line(morning_hit.y).plain.startswith("09:00 ● M")
        assert grid._line(afternoon_hit.y).plain.startswith("14:00 ● A")

    app_test(app, assertions, size=(130, 34))


def test_links_in_workspace_text_open_only_safe_web_urls(tmp_path: Path) -> None:
    url = "https://example.test/guide"
    linked = linkify_urls(f"Read {url}.")
    assert linked.plain == f"Read {url}."
    assert [span.style.link for span in linked.spans] == [url]

    opened: list[str] = []
    app = HcbApp(seeded_runtime(tmp_path), url_opener=lambda target: opened.append(target) or True)
    plain_row = app._workspace_task_row(Task("plain", "work", "inbox", "Read offline"), "")
    assert plain_row.label.plain.endswith("Read offline")
    assert not any(span.style.link for span in plain_row.label.spans)
    linked_row = app._workspace_task_row(Task("linked", "work", "inbox", f"Read {url}."), "")
    assert [span.style.link for span in linked_row.label.spans if span.style.link] == [url]
    app.surface = "Notes"
    preview_row = app._workspace_task_row(
        Task("preview", "work", "inbox", "Reference", notes=f"See {url}.\nSecond line"), ""
    )
    assert [span.style.link for span in preview_row.label.spans if span.style.link] == [url]
    app.surface = "Tasks"

    async def assertions(_: object) -> None:
        app.on_click(events.Click(app, 0, 0, 0, 0, 1, False, False, False, style=Style(link=url)))
        app.on_click(
            events.Click(
                app, 0, 0, 0, 0, 1, False, False, False, style=Style(link="file:///tmp/no")
            )
        )
        assert opened == [url]

    app_test(app, assertions)


def test_item_view_links_event_attachments_and_drive_files(tmp_path: Path) -> None:
    runtime = seeded_runtime(tmp_path)
    calendar_id = runtime.storage.list_calendars("work")[0].id
    description_url = "https://example.test/event-notes"
    attachment_url = "https://drive.google.com/open?id=attachment"
    drive_url = "https://drive.google.com/open?id=file"
    event = runtime.application.create_event(
        "work",
        calendar_id,
        "Planning",
        EventDateTime(DateTimeKind.DATE, date(2026, 8, 24)),
        EventDateTime(DateTimeKind.DATE, date(2026, 8, 25)),
        description=f"Read {description_url}",
        attachments=({"title": "Planning brief", "fileUrl": attachment_url},),
    )
    runtime.storage.upsert_drive_file(
        DriveFile("brief", "work", "Planning brief", "application/pdf", drive_url)
    )
    app = HcbApp(runtime, selected_date=date(2026, 8, 24))

    def targets(screen: ItemViewScreen) -> set[str]:
        targets: set[str] = set()
        for widget in screen.query(Static):
            content = widget.content
            if not isinstance(content, Text):
                continue
            targets.update(
                span.style.link
                for span in content.spans
                if isinstance(span.style, Style) and span.style.link
            )
        return targets

    async def assertions(pilot: object) -> None:
        app.selected = ("event", event.id)
        app.action_view()
        await pilot.pause()  # type: ignore[attr-defined]
        assert isinstance(app.screen, ItemViewScreen)
        assert {description_url, attachment_url} <= targets(app.screen)

        await pilot.press("escape")  # type: ignore[attr-defined]
        app.selected = ("drive", "brief")
        app.action_view()
        await pilot.pause()  # type: ignore[attr-defined]
        assert isinstance(app.screen, ItemViewScreen)
        assert drive_url in targets(app.screen)

    app_test(app, assertions)


def test_item_view_renders_markdown_and_safe_html(tmp_path: Path) -> None:
    runtime = seeded_runtime(tmp_path)
    task_list = runtime.storage.list_task_lists("work")[0]
    task = runtime.application.create_task(
        "work",
        task_list.id,
        "## Task heading",
        notes="**Important**: _review_ the [brief](https://example.test/brief).",
    )
    calendar_id = runtime.storage.list_calendars("work")[0].id
    event = runtime.application.create_event(
        "work",
        calendar_id,
        "## Event heading",
        EventDateTime(DateTimeKind.DATE, date(2026, 8, 24)),
        EventDateTime(DateTimeKind.DATE, date(2026, 8, 25)),
        description=(
            "<p>Use <strong>bold</strong> and <em>italics</em>.</p>"
            "<ul><li>First step</li></ul>"
            "<script>do-not-render()</script>"
        ),
    )
    app = HcbApp(runtime, selected_date=date(2026, 8, 24))

    rendered = render_readonly_markup(
        '<h2>Plan</h2><p>Use **bold** and _italics_ <a href="https://example.test">docs</a> '
        "and [unsafe](javascript:bad).</p>"
    )
    assert rendered.plain == "Plan\n\nUse bold and italics docs and unsafe."
    assert any(span.style == "bold" for span in rendered.spans)
    assert any(span.style == "italic" for span in rendered.spans)
    assert any(
        isinstance(span.style, Style) and span.style.link == "https://example.test"
        for span in rendered.spans
    )

    def text_parts(screen: ItemViewScreen) -> tuple[Text, ...]:
        return tuple(
            widget.content for widget in screen.query(Static) if isinstance(widget.content, Text)
        )

    async def assertions(pilot: object) -> None:
        app.selected = ("event", event.id)
        app.action_view()
        await pilot.pause()  # type: ignore[attr-defined]
        assert isinstance(app.screen, ItemViewScreen)
        event_text = "\n".join(part.plain for part in text_parts(app.screen))
        assert "<p>" not in event_text
        assert "do-not-render" not in event_text
        assert "• First step" in event_text

        await pilot.press("escape")  # type: ignore[attr-defined]
        app.selected = ("task", task.id)
        app.action_view()
        await pilot.pause()  # type: ignore[attr-defined]
        assert isinstance(app.screen, ItemViewScreen)
        task_text = "\n".join(part.plain for part in text_parts(app.screen))
        assert "## Task heading" not in task_text
        assert "**Important**" not in task_text
        assert "Task heading" in task_text
        assert "Important" in task_text

    app_test(app, assertions)


def test_event_view_only_shows_relevant_specialist_details(tmp_path: Path) -> None:
    runtime = seeded_runtime(tmp_path)
    calendar_id = runtime.storage.list_calendars("work")[0].id
    event = runtime.application.create_event(
        "work",
        calendar_id,
        "Long reminder",
        EventDateTime(DateTimeKind.DATE, date(2026, 8, 24)),
        EventDateTime(DateTimeKind.DATE, date(2026, 8, 25)),
        reminder_use_default=False,
        reminder_overrides=(ReminderOverride("popup", 420), ReminderOverride("email", 90)),
        color_id="4",
        event_type="focusTime",
        visibility="private",
        transparency="opaque",
    )
    app = HcbApp(runtime, selected_date=date(2026, 8, 24))
    screen = ItemViewScreen(app, event)

    details = screen._event_details(event)
    assert "Reminders: popup 7 hr, email 1 hr 30 min" in details.plain
    assert "Status:" not in details.plain
    assert "Type:" not in details.plain
    assert "Visibility:" not in details.plain
    assert "Transparency:" not in details.plain
    assert "Conference:" not in details.plain
    assert "Guest permissions:" not in details.plain
    dot = next(span for span in details.spans if details.plain[span.start : span.end] == "●")
    assert isinstance(dot.style, Style)
    assert dot.style.color is not None
    assert dot.style.color.get_truecolor().hex == "#e67c73"

    relevant = runtime.application.update_event(
        "work",
        event.id,
        conference={"entryPoints": [{"uri": "https://meet.google.com/example"}]},
        guests_can_invite_others=False,
        guests_can_modify=True,
    )
    relevant_details = screen._event_details(relevant)
    assert "Conference: Open meeting" in relevant_details.plain
    assert "Guest permissions: invite=no, modify=yes" in relevant_details.plain
    assert any(
        isinstance(span.style, Style) and span.style.link == "https://meet.google.com/example"
        for span in relevant_details.spans
    )


def test_event_view_formats_reminder_offsets_as_readable_units() -> None:
    assert ItemViewScreen._reminder_offset(59) == "59 min"
    assert ItemViewScreen._reminder_offset(60) == "1 hr"
    assert ItemViewScreen._reminder_offset(90) == "1 hr 30 min"
    assert ItemViewScreen._reminder_offset(1500) == "1 day 1 hr"


def test_completed_tasks_are_struck_through_in_detail_and_workspace(tmp_path: Path) -> None:
    runtime = seeded_runtime(tmp_path)
    task = runtime.application.complete_task("work", runtime.storage.list_tasks("work")[0].id)
    app = HcbApp(runtime)
    screen = ItemViewScreen(app, task)

    assert "Status:" not in screen._task_details(task).plain
    title = screen._task_title(task)
    assert any(isinstance(span.style, Style) and span.style.strike for span in title.spans)

    async def assertions(pilot: object) -> None:
        row = next(item for item in workspace_rows(app) if item.item_id == task.id)
        label = row.label
        spans = getattr(label, "spans", ())
        assert any(getattr(span.style, "dim", False) for span in spans)
        assert any(getattr(span.style, "strike", False) for span in spans)

    app_test(app, assertions)


def test_content_selection_persistently_marks_the_active_row(tmp_path: Path) -> None:
    runtime = seeded_runtime(tmp_path)
    task_list = runtime.storage.list_task_lists("work")[0]
    runtime.application.create_task("work", task_list.id, "Second task")
    app = HcbApp(runtime)

    async def assertions(pilot: object) -> None:
        content = app.query_one("#content", WorkspaceTable)
        rows = workspace_rows(app)

        content.move_cursor(row=1, animate=False)
        await pilot.pause()  # type: ignore[attr-defined]

        assert app.selected == ("task", rows[1].item_id)

        app.query_one("#resources", ListView).focus()
        await pilot.pause()  # type: ignore[attr-defined]
        assert content.row_at(content.cursor_row) == rows[1]

    app_test(app, assertions)


def test_content_enter_opens_a_readonly_view_and_e_opens_the_editor(tmp_path: Path) -> None:
    app = HcbApp(seeded_runtime(tmp_path))

    async def assertions(pilot: object) -> None:
        content = app.query_one("#content", WorkspaceTable)
        content.focus()
        await pilot.press("enter")  # type: ignore[attr-defined]
        await pilot.pause()  # type: ignore[attr-defined]
        assert isinstance(app.screen, ItemViewScreen)
        assert app.screen.query_one("#item-view-edit", Button).label == "Edit"
        assert app.screen.query_one("#item-view-delete", Button).label == "Delete"
        await pilot.press("e")  # type: ignore[attr-defined]
        await pilot.pause()  # type: ignore[attr-defined]
        assert isinstance(app.screen, EditorScreen)
        await pilot.press("escape")  # type: ignore[attr-defined]
        await pilot.click("#content", offset=(4, 1))  # type: ignore[attr-defined]
        await pilot.pause()  # type: ignore[attr-defined]
        assert isinstance(app.screen, ItemViewScreen)

    app_test(app, assertions)


def test_event_editor_uses_readable_frequency_and_preserves_rrule_details(tmp_path: Path) -> None:
    runtime = seeded_runtime(tmp_path)
    calendar_id = runtime.storage.list_calendars("work")[0].id
    event = runtime.application.create_event(
        "work",
        calendar_id,
        "Renew membership",
        EventDateTime(DateTimeKind.DATE, date(2026, 8, 1)),
        EventDateTime(DateTimeKind.DATE, date(2026, 8, 2)),
        recurrence=("RRULE:FREQ=YEARLY;COUNT=3",),
    )
    app = HcbApp(runtime, selected_date=date(2026, 8, 1))

    assert recurrence_frequency(event.recurrence) == "yearly"
    assert recurrence_summary(event.recurrence) == "Every year · 3 times"
    assert recurrence_with_frequency(event.recurrence, "weekly") == ("RRULE:FREQ=WEEKLY;COUNT=3",)

    async def assertions(pilot: object) -> None:
        app.selected = ("event", event.id)
        app.action_edit()
        await pilot.pause()  # type: ignore[attr-defined]
        assert isinstance(app.screen, EventEditorScreen)
        assert app.screen.query_one("#event-frequency", Select).value == "yearly"
        assert "Every year · 3 times" in str(
            app.screen.query_one("#event-recurrence-summary", Static).render()
        )
        assert app.screen.query_one("#event-custom-recurrence", Button).display

        app.screen.query_one("#event-frequency", Select).value = "weekly"
        await pilot.pause()  # type: ignore[attr-defined]
        assert "Every week · 3 times" in str(
            app.screen.query_one("#event-recurrence-summary", Static).render()
        )
        await pilot.click("#event-save")  # type: ignore[attr-defined]
        await pilot.pause()  # type: ignore[attr-defined]
        updated = runtime.storage.get_event("work", event.id)
        assert updated is not None
        assert updated.recurrence == ("RRULE:FREQ=WEEKLY;COUNT=3",)

    app_test(app, assertions)


def test_event_editor_builds_custom_recurrence_with_contextual_controls(tmp_path: Path) -> None:
    runtime = seeded_runtime(tmp_path)
    calendar_id = runtime.storage.list_calendars("work")[0].id
    event = runtime.application.create_event(
        "work",
        calendar_id,
        "Custom interval",
        EventDateTime(DateTimeKind.DATE, date(2026, 8, 1)),
        EventDateTime(DateTimeKind.DATE, date(2026, 8, 2)),
    )
    app = HcbApp(runtime, selected_date=date(2026, 8, 1))

    async def assertions(pilot: object) -> None:
        app.selected = ("event", event.id)
        app.action_edit()
        await pilot.pause()  # type: ignore[attr-defined]
        app.screen.query_one("#event-frequency", Select).value = "custom"
        await pilot.pause()  # type: ignore[attr-defined]

        assert app.screen.query_one("#recurrence-weekdays", Horizontal).display
        app.screen.query_one("#recurrence-unit", Select).value = "monthly"
        await pilot.pause()  # type: ignore[attr-defined]
        assert not app.screen.query_one("#recurrence-weekdays", Horizontal).display
        assert app.screen.query_one("#recurrence-monthly", Horizontal).display
        app.screen.query_one("#recurrence-unit", Select).value = "weekly"
        await pilot.pause()  # type: ignore[attr-defined]
        app.screen.query_one("#recurrence-interval", Input).value = "2"
        await pilot.click("#recurrence-weekday-sa")  # type: ignore[attr-defined]
        await pilot.click("#recurrence-weekday-mo")  # type: ignore[attr-defined]
        await pilot.click("#recurrence-weekday-we")  # type: ignore[attr-defined]
        app.screen.query_one("#recurrence-end", Select).value = "after"
        await pilot.pause()  # type: ignore[attr-defined]
        assert app.screen.query_one("#recurrence-count", Input).display
        app.screen.query_one("#recurrence-count", Input).value = "8"
        await pilot.click("#recurrence-done")  # type: ignore[attr-defined]
        await pilot.pause()  # type: ignore[attr-defined]

        assert isinstance(app.screen, EventEditorScreen)
        assert "Every 2 weeks on Monday, Wednesday · 8 times" in str(
            app.screen.query_one("#event-recurrence-summary", Static).render()
        )
        await pilot.click("#event-save")  # type: ignore[attr-defined]
        await pilot.pause()  # type: ignore[attr-defined]
        updated = runtime.storage.get_event("work", event.id)
        assert updated is not None
        assert updated.recurrence == ("RRULE:FREQ=WEEKLY;INTERVAL=2;BYDAY=MO,WE;COUNT=8",)

    app_test(app, assertions)


def test_structured_recurrence_preserves_unmodeled_rrule_and_exdate_lines() -> None:
    original = (
        "RRULE:FREQ=WEEKLY;INTERVAL=2;BYDAY=MO,WE;COUNT=3;WKST=MO",
        "EXDATE:20260805",
    )
    spec = recurrence_spec(original, date(2026, 8, 3))

    assert spec.interval == 2
    assert spec.weekdays == ("MO", "WE")
    assert spec.count == 3

    updated = recurrence_with_spec(
        original,
        replace(spec, interval=4, weekdays=("TU",), end="on", until=date(2026, 12, 31)),
        all_day=False,
    )

    assert updated == (
        "RRULE:FREQ=WEEKLY;INTERVAL=4;BYDAY=TU;UNTIL=20261231T235959Z;WKST=MO",
        "EXDATE:20260805",
    )


def test_event_view_location_uses_a_clickable_google_maps_link(tmp_path: Path) -> None:
    runtime = seeded_runtime(tmp_path)
    calendar_id = runtime.storage.list_calendars("work")[0].id
    event = runtime.application.create_event(
        "work",
        calendar_id,
        "Lunch",
        EventDateTime(DateTimeKind.DATE, date(2026, 8, 24)),
        EventDateTime(DateTimeKind.DATE, date(2026, 8, 25)),
        location="Maxwell Food Centre, Singapore",
    )
    opened: list[str] = []
    app = HcbApp(
        runtime,
        selected_date=date(2026, 8, 24),
        url_opener=lambda target: opened.append(target) or True,
    )
    maps_url = google_maps_url(event.location or "")
    assert maps_url == (
        "https://www.google.com/maps/search/?api=1&query=Maxwell+Food+Centre%2C+Singapore"
    )

    async def assertions(pilot: object) -> None:
        app.selected = ("event", event.id)
        app.action_view()
        await pilot.pause()  # type: ignore[attr-defined]
        assert isinstance(app.screen, ItemViewScreen)
        location = app.screen.query_one(".item-view-location", Static).content
        assert isinstance(location, Text)
        assert location.plain.startswith("󰖟 Maxwell Food Centre")
        assert any(
            isinstance(span.style, Style) and span.style.link == maps_url for span in location.spans
        )
        await pilot.press("escape")  # type: ignore[attr-defined]
        app.action_edit()
        await pilot.pause()  # type: ignore[attr-defined]
        assert isinstance(app.screen, EventEditorScreen)
        editor_location = app.screen.query_one("#event-location-map", Static).content
        assert isinstance(editor_location, Text)
        assert editor_location.plain.startswith("󰖟 Maxwell Food Centre")
        app.on_click(
            events.Click(
                app,
                0,
                0,
                0,
                0,
                1,
                False,
                False,
                True,
                style=Style(link=maps_url),
            )
        )
        assert opened == [maps_url]

    app_test(app, assertions)


def test_content_rows_scroll_horizontally_for_long_titles(tmp_path: Path) -> None:
    runtime = seeded_runtime(tmp_path)
    task_list = runtime.storage.list_task_lists("work")[0]
    runtime.application.create_task("work", task_list.id, "Long title " * 30)
    app = HcbApp(runtime)

    async def assertions(pilot: object) -> None:
        content = app.query_one("#content", WorkspaceTable)
        assert content.max_scroll_x > 0
        content.scroll_to(content.max_scroll_x, animate=False, immediate=True)
        await pilot.pause()  # type: ignore[attr-defined]
        assert content.scroll_x == content.max_scroll_x

    app_test(app, assertions, size=(80, 24))


def test_topbar_uses_present_date_while_surface_uses_selected_date(tmp_path: Path) -> None:
    selected_date = date(2001, 1, 2)
    app = HcbApp(seeded_runtime(tmp_path), selected_date=selected_date)

    async def assertions(_: object) -> None:
        topbar = str(app.query_one("#topbar", Static).render())
        surface_title = str(app.query_one("#surface-title", Static).render())
        assert app.format_date(app._present_date()) in topbar
        assert app.format_date(selected_date) not in topbar
        assert "Tuesday, 02 January 2001" in surface_title

    app_test(app, assertions)


def test_resource_selection_persistently_marks_the_active_filter(tmp_path: Path) -> None:
    runtime = seeded_runtime(tmp_path)
    task_list = runtime.application.create_task_list("work", "Plans")
    calendar = runtime.application.create_calendar("work", "Work")
    app = HcbApp(runtime)

    async def assertions(pilot: object) -> None:
        resources = app.query_one("#resources", ListView)
        rows = tuple(resources.query(EntityRow))
        all_resources = next(row for row in rows if row.kind == "resource-all")
        plans = next(row for row in rows if row.item_id == task_list.id)
        work = next(row for row in rows if row.item_id == calendar.id)

        assert all_resources.has_class("hcb-selected")
        assert not plans.has_class("hcb-selected")

        app.resource_filter = ("task-list", task_list.id)
        app._render_chrome(refresh_resources=False)
        await pilot.pause()  # type: ignore[attr-defined]
        assert plans.has_class("hcb-selected")
        assert not all_resources.has_class("hcb-selected")

        app.resource_filter = ("calendar", calendar.id)
        app._render_chrome(refresh_resources=False)
        app.query_one("#content", WorkspaceTable).focus()
        await pilot.pause()  # type: ignore[attr-defined]
        assert work.has_class("hcb-selected")
        assert not plans.has_class("hcb-selected")

    app_test(app, assertions)


def test_text_editors_have_a_persistent_high_contrast_block_cursor(tmp_path: Path) -> None:
    app = HcbApp(seeded_runtime(tmp_path))

    async def assertions(pilot: object) -> None:
        await pilot.press("n")  # type: ignore[attr-defined]
        await pilot.pause()  # type: ignore[attr-defined]
        title = app.screen.query_one("#editor-title", Input)
        notes = app.screen.query_one("#editor-notes", TerminalTextArea)
        assert title.has_focus
        assert not title.cursor_blink
        assert not notes.cursor_blink
        cursor = title.get_component_rich_style("input--cursor")
        assert cursor.bgcolor is not None and cursor.bgcolor.number == 7
        assert cursor.color is not None and cursor.color.number == 0

    app_test(app, assertions)


def test_text_editors_complete_rich_emoji_names_and_aliases(tmp_path: Path) -> None:
    app = HcbApp(seeded_runtime(tmp_path))
    alias = emoji_suggestions(":+1", 3)
    assert alias is not None and ("+1", "👍") in alias[1]

    async def assertions(pilot: object) -> None:
        await pilot.press("n")  # type: ignore[attr-defined]
        await pilot.pause()  # type: ignore[attr-defined]
        title = app.screen.query_one("#editor-title", Input)
        await pilot.press(":", "s", "m", "i")  # type: ignore[attr-defined]
        await pilot.pause()  # type: ignore[attr-defined]
        popup = app.screen.query_one("#emoji-completion", Static)
        assert ":smile: 😄" in str(popup.render())
        assert app._emoji_selection == 0
        await pilot.press("down")  # type: ignore[attr-defined]
        await pilot.pause()  # type: ignore[attr-defined]
        assert app._emoji_selection == 1
        await pilot.press("tab")  # type: ignore[attr-defined]
        await pilot.pause()  # type: ignore[attr-defined]
        assert title.value == "😸"
        assert not popup.display

        title.value = ":heart"
        title.cursor_position = len(title.value)
        await pilot.pause()  # type: ignore[attr-defined]
        assert popup.display
        await pilot.press("escape")  # type: ignore[attr-defined]
        await pilot.pause()  # type: ignore[attr-defined]
        assert isinstance(app.screen, EditorScreen)
        assert title.value == ":heart"
        assert not popup.display

        notes = app.screen.query_one("#editor-notes", TerminalTextArea)
        notes.focus()
        notes.load_text("Before :heart")
        notes.cursor_location = notes.document.end
        await pilot.pause()  # type: ignore[attr-defined]
        await pilot.press("enter")  # type: ignore[attr-defined]
        await pilot.pause()  # type: ignore[attr-defined]
        assert notes.text == "Before ❤"

    app_test(app, assertions)


def test_external_editor_replaces_shared_input_widgets(tmp_path: Path) -> None:
    commands: list[list[str]] = []
    temporary_files: list[Path] = []
    contents = iter(("first\nsecond\n", "one\n\ntwo\n"))
    suspensions: list[bool] = []

    @contextmanager
    def suspended() -> Iterator[None]:
        suspensions.append(True)
        yield

    def run_editor(command: list[str]) -> int:
        commands.append(command)
        path = Path(command[-1])
        temporary_files.append(path)
        path.write_text(next(contents), encoding="utf-8")
        return 0

    app = HcbApp(
        seeded_runtime(tmp_path, environ={"HCB_EDITOR": "vim -n"}),
        editor_runner=run_editor,
        suspend=suspended,
    )

    async def assertions(pilot: object) -> None:
        app.action_external_editor()
        assert not commands
        await pilot.press("n")  # type: ignore[attr-defined]
        await pilot.pause()  # type: ignore[attr-defined]
        title = app.screen.query_one("#editor-title", Input)
        title.value = "original"
        await pilot.press("ctrl+g")  # type: ignore[attr-defined]
        await pilot.pause()  # type: ignore[attr-defined]
        assert title.value == "first second"
        notes = app.screen.query_one("#editor-notes", TerminalTextArea)
        notes.focus()
        await pilot.press("ctrl+g")  # type: ignore[attr-defined]
        await pilot.pause()  # type: ignore[attr-defined]
        assert notes.text == "one\n\ntwo\n"

    app_test(app, assertions)
    assert [command[:-1] for command in commands] == [["vim", "-n"], ["vim", "-n"]]
    assert len(suspensions) == 2
    assert all(not path.exists() for path in temporary_files)


def test_external_editor_failure_leaves_input_unchanged(tmp_path: Path) -> None:
    notifications: list[str] = []

    @contextmanager
    def suspended() -> Iterator[None]:
        yield

    def run_editor(command: list[str]) -> int:
        Path(command[-1]).write_text("changed", encoding="utf-8")
        return 9

    app = HcbApp(seeded_runtime(tmp_path), editor_runner=run_editor, suspend=suspended)

    async def assertions(pilot: object) -> None:
        original_notify = app.notify

        def record_notification(message: object, **kwargs: object) -> None:
            notifications.append(str(message))
            original_notify(message, **kwargs)

        app.notify = record_notification  # type: ignore[method-assign]
        await pilot.press("n")  # type: ignore[attr-defined]
        await pilot.pause()  # type: ignore[attr-defined]
        title = app.screen.query_one("#editor-title", Input)
        title.value = "original"
        await pilot.press("ctrl+g")  # type: ignore[attr-defined]
        await pilot.pause()  # type: ignore[attr-defined]
        assert title.value == "original"

    app_test(app, assertions)
    assert "External editor exited with status 9" in notifications


def test_external_editor_reports_unsupported_suspension(tmp_path: Path) -> None:
    notifications: list[str] = []

    def unsupported() -> None:
        raise SuspendNotSupported("unsupported")

    app = HcbApp(
        seeded_runtime(tmp_path),
        editor_runner=lambda command: (_ for _ in ()).throw(AssertionError(command)),
        suspend=unsupported,  # type: ignore[arg-type]
    )

    async def assertions(pilot: object) -> None:
        original_notify = app.notify

        def record_notification(message: object, **kwargs: object) -> None:
            notifications.append(str(message))
            original_notify(message, **kwargs)

        app.notify = record_notification  # type: ignore[method-assign]
        await pilot.press("n")  # type: ignore[attr-defined]
        await pilot.pause()  # type: ignore[attr-defined]
        title = app.screen.query_one("#editor-title", Input)
        title.value = "original"
        await pilot.press("ctrl+g")  # type: ignore[attr-defined]
        await pilot.pause()  # type: ignore[attr-defined]
        assert title.value == "original"

    app_test(app, assertions)
    assert "External editor is unavailable in this environment" in notifications


def test_event_surface_marks_stale_instance_cache_ranges(tmp_path: Path) -> None:
    runtime = seeded_runtime(tmp_path)
    calendar_id = runtime.storage.list_calendars("work")[0].id
    start = datetime(2026, 8, 21, tzinfo=UTC)
    runtime.storage.replace_cached_instances("work", calendar_id, start, start.replace(day=28), [])
    runtime.storage.mark_instance_ranges_stale(
        "work", calendar_id, reason="local-recurring-event-updated"
    )
    app = HcbApp(runtime, selected_date=date(2026, 8, 21))

    async def assertions(pilot: object) -> None:
        await pilot.press("3")  # type: ignore[attr-defined]
        await pilot.pause()  # type: ignore[attr-defined]
        cache_state = str(app.query_one("#sync-state", Static).render())
        assert "instances: stale" in cache_state
        assert "local recurring event updated" in cache_state
        assert "21 August 2026" in cache_state

    app_test(app, assertions)


def test_palette_shows_commands_and_title_first_results(tmp_path: Path) -> None:
    app = HcbApp(seeded_runtime(tmp_path))

    async def assertions(pilot: object) -> None:
        await pilot.press("/")  # type: ignore[attr-defined]
        await pilot.pause()  # type: ignore[attr-defined]
        assert isinstance(app.screen, PaletteScreen)
        results = app.screen.query_one("#palette-results", ListView)
        assert len(results.children) == len(PALETTE_COMMANDS)
        query = app.screen.query_one("#palette-query", Input)
        query.value = "Ship"
        await pilot.pause()  # type: ignore[attr-defined]
        labels = [str(row.query_one("Label").render()) for row in results.children]
        assert any(label.startswith("Ship TUI") for label in labels)
        await pilot.press("escape")  # type: ignore[attr-defined]

    app_test(app, assertions)


def test_help_uses_the_configured_keymap_and_lists_all_actions(tmp_path: Path) -> None:
    runtime = seeded_runtime(tmp_path)
    save(Config(keys=KeyBindings(help="f1")), runtime.paths.config_file)
    app = HcbApp(runtime)

    async def assertions(pilot: object) -> None:
        await pilot.press("f1")  # type: ignore[attr-defined]
        await pilot.pause()  # type: ignore[attr-defined]
        assert isinstance(app.screen, HelpScreen)
        content = app.screen.query_one("#help-content").query_one(Static)
        assert "Command palette" in str(content.render())
        assert "External editor" in str(content.render())
        await pilot.press("escape")  # type: ignore[attr-defined]
        await pilot.pause()  # type: ignore[attr-defined]
        assert not isinstance(app.screen, HelpScreen)

    app_test(app, assertions)


def test_custom_tcss_is_loaded_only_when_the_file_validates(tmp_path: Path) -> None:
    paths = AppPaths(tmp_path / "config", tmp_path / "data", tmp_path / "cache")
    paths.config_dir.mkdir()
    stylesheet = paths.config_dir / "custom.tcss"
    stylesheet.write_text("#topbar { color: red; }\n", encoding="utf-8")
    save(Config(theme=Theme(stylesheet="custom.tcss")), paths.config_file)
    valid = HcbApp(Runtime(paths, environ={}))
    assert valid._stylesheet_error is None
    assert valid._loaded_stylesheet == "custom.tcss"

    stylesheet.write_text("#topbar { color: ; }\n", encoding="utf-8")
    invalid = HcbApp(Runtime(paths, environ={}))
    assert invalid._stylesheet_error is not None
    assert invalid._loaded_stylesheet is None


def test_palette_searches_indexed_drive_files_and_opens_their_view(tmp_path: Path) -> None:
    runtime = seeded_runtime(tmp_path)
    runtime.storage.upsert_drive_file(
        DriveFile(
            "brief",
            "work",
            "Quarterly brief workspaceunique",
            "application/pdf",
            "https://example.test/brief",
        )
    )
    app = HcbApp(runtime)

    async def assertions(pilot: object) -> None:
        await pilot.press("/")  # type: ignore[attr-defined]
        await pilot.pause()  # type: ignore[attr-defined]
        query = app.screen.query_one("#palette-query", Input)
        query.value = "workspaceunique"
        await pilot.pause()  # type: ignore[attr-defined]
        labels = [
            str(row.query_one("Label").render())
            for row in app.screen.query_one("#palette-results", ListView).children
        ]
        assert labels == ["Quarterly brief workspaceunique  · drive"]
        await pilot.press("enter")  # type: ignore[attr-defined]
        await pilot.pause()  # type: ignore[attr-defined]
        assert app.selected == ("drive", "brief")
        assert isinstance(app.screen, ItemViewScreen)
        assert "application/pdf" in "\n".join(
            str(widget.render()) for widget in app.screen.query(Static)
        )

    app_test(app, assertions)


def test_palette_click_opens_task_note_and_event_results_in_their_views(tmp_path: Path) -> None:
    runtime = seeded_runtime(tmp_path)
    task_list = runtime.storage.list_task_lists("work")[0]
    task = runtime.application.create_task("work", task_list.id, "Palette task target")
    note = runtime.application.create_task("work", task_list.id, "Palette note target")
    calendar_id = runtime.storage.list_calendars("work")[0].id
    event = runtime.application.create_event(
        "work",
        calendar_id,
        "Palette event target",
        EventDateTime(DateTimeKind.DATE, date(2026, 8, 24)),
        EventDateTime(DateTimeKind.DATE, date(2026, 8, 25)),
    )
    app = HcbApp(runtime)

    async def open_result(pilot: object, query_text: str, expected_kind: str, item_id: str) -> None:
        await pilot.press("/")  # type: ignore[attr-defined]
        await pilot.pause()  # type: ignore[attr-defined]
        query = app.screen.query_one("#palette-query", Input)
        query.value = query_text
        await pilot.pause()  # type: ignore[attr-defined]
        results = app.screen.query_one("#palette-results", ListView)
        assert len(results.children) == 1
        await pilot.click(results.children[0])  # type: ignore[attr-defined]
        await pilot.pause()  # type: ignore[attr-defined]
        assert app.selected == (expected_kind, item_id)
        assert isinstance(app.screen, ItemViewScreen)
        await pilot.press("escape")  # type: ignore[attr-defined]
        await pilot.pause()  # type: ignore[attr-defined]

    async def assertions(pilot: object) -> None:
        await open_result(pilot, task.title, "task", task.id)
        await open_result(pilot, note.title, "task", note.id)
        await open_result(pilot, event.summary, "event", event.id)

    app_test(app, assertions)


def test_palette_workspace_result_navigation_for_lists_calendars_saved_searches_and_conflicts(
    tmp_path: Path,
) -> None:
    runtime = seeded_runtime(tmp_path)
    task_list = runtime.application.create_task_list("work", "Research")
    calendar = runtime.application.create_calendar("work", "Travel")
    saved = runtime.application.save_search(
        "work", "Open research", "list:Research completed:false"
    )
    runtime.storage.add_conflict(
        Conflict(None, "work", EntityType.TASK, "missing", {"title": "L"}, {"title": "R"})
    )
    app = HcbApp(runtime)

    async def assertions(pilot: object) -> None:
        app._palette_result(("task-list", task_list.id))
        assert app.surface == "Tasks"
        assert app.resource_filter == ("task-list", task_list.id)

        app._palette_result(("calendar", calendar.id))
        assert app.surface == "Agenda"
        assert app.resource_filter == ("calendar", calendar.id)

        app._palette_result(("saved-search", saved.id))
        await pilot.pause()  # type: ignore[attr-defined]
        assert isinstance(app.screen, PaletteScreen)
        assert app.screen.query_one("#palette-query", Input).value == saved.query
        await pilot.press("escape")  # type: ignore[attr-defined]
        await pilot.pause()  # type: ignore[attr-defined]

        app._palette_result(("conflict", "1"))
        await pilot.pause()  # type: ignore[attr-defined]
        assert isinstance(app.screen, ConflictScreen)

    app_test(app, assertions)


def test_task_create_edit_complete_and_delete(tmp_path: Path) -> None:
    runtime = seeded_runtime(tmp_path)
    database = runtime.paths.database_file
    app = HcbApp(runtime)

    async def actions(pilot: object) -> None:
        await pilot.press("n")  # type: ignore[attr-defined]
        await pilot.pause()  # type: ignore[attr-defined]
        app.screen.query_one("#editor-title", Input).value = "Created in TUI"
        await pilot.click("#save")  # type: ignore[attr-defined]
        await pilot.pause()  # type: ignore[attr-defined]
        rows = app.query_one("#content", WorkspaceTable)
        created_index = next(
            index
            for index, row in enumerate(workspace_rows(app))
            if "Created in TUI" in str(row.label)
        )
        rows.move_cursor(row=created_index, animate=False)
        await pilot.pause()  # type: ignore[attr-defined]
        await pilot.press("space")  # type: ignore[attr-defined]
        await pilot.pause()  # type: ignore[attr-defined]
        assert isinstance(app.screen, BatchActionScreen)
        await pilot.click("#batch-action-apply")  # type: ignore[attr-defined]
        await pilot.pause()  # type: ignore[attr-defined]
        await pilot.press("d")  # type: ignore[attr-defined]
        await pilot.pause()  # type: ignore[attr-defined]
        assert isinstance(app.screen, BatchActionScreen)
        await pilot.click("#batch-action-apply")  # type: ignore[attr-defined]
        await pilot.pause()  # type: ignore[attr-defined]

    app_test(app, actions)
    with Storage(database) as storage:
        assert all(task.title != "Created in TUI" for task in storage.list_tasks("work"))


def test_task_editor_uses_due_date_selects_and_can_delete_one_task(tmp_path: Path) -> None:
    runtime = seeded_runtime(tmp_path)
    app = HcbApp(runtime)

    async def actions(pilot: object) -> None:
        await pilot.press("e")  # type: ignore[attr-defined]
        await pilot.pause()  # type: ignore[attr-defined]
        assert isinstance(app.screen, EditorScreen)
        assert app.screen.query_one("#delete", Button).display
        app.screen.query_one("#editor-due-day", Select).value = "31"
        app.screen.query_one("#editor-due-month", Select).value = "12"
        app.screen.query_one("#editor-due-year", Select).value = "2027"
        await pilot.click("#save")  # type: ignore[attr-defined]
        await pilot.pause()  # type: ignore[attr-defined]
        assert app.cache.tasks[0].due == date(2027, 12, 31)

        await pilot.press("e")  # type: ignore[attr-defined]
        await pilot.pause()  # type: ignore[attr-defined]
        await pilot.click("#delete")  # type: ignore[attr-defined]
        await pilot.pause()  # type: ignore[attr-defined]
        assert isinstance(app.screen, ConfirmScreen)
        await pilot.press("y")  # type: ignore[attr-defined]
        await pilot.pause()  # type: ignore[attr-defined]
        assert not app.cache.tasks

    app_test(app, actions)


def test_no_account_onboarding_is_actionable_and_offline(
    tmp_path: Path, monkeypatch: pytest.MonkeyPatch
) -> None:
    notifications: list[str] = []

    def record_notification(_screen: OnboardingScreen, message: object, **_kwargs: object) -> None:
        notifications.append(str(message))

    monkeypatch.setattr(OnboardingScreen, "notify", record_notification)
    paths = AppPaths(tmp_path / "config", tmp_path / "data", tmp_path / "cache")
    app = HcbApp(
        Runtime(paths, environ={}),
        local_environment=LocalEnvironment(
            "macOS",
            "arm64",
            "Ghostty",
            terminal_theme="Rose Pine",
            suggested_preset="Rose Pine",
        ),
    )

    async def assertions(pilot: object) -> None:
        assert app.account_id is None
        assert isinstance(app.screen, OnboardingScreen)
        assert (
            app.screen.query_one("#onboard-env-file", Input).value == "~/.config/hcb/personal.env"
        )
        assert (
            str(app.screen.query_one("#onboard-env-editor-hint", Label).render())
            == "Ctrl+G to open this credential file in nvim"
        )
        labels = [str(label.render()) for label in app.screen.query(".onboarding-field Label")]
        assert labels == [
            "Credential .env path",
            "Suggested .env files",
            "Local account identifier",
            "Account email",
            "Time zone",
            "Reminders",
            "Appearance",
        ]
        assert app.screen.query_one("#onboard-timezone", Select).value == "UTC"
        assert app.screen.query_one("#onboard-reminders", Select).value == "true"
        assert app.screen.query_one("#onboard-theme", Select).value == "terminal"
        assert app.screen.query_one("#onboard-env-file-suggestion", Select).value == str(
            Path("~/.config/hcb/personal.env").expanduser()
        )
        assert ("Use detected Rose Pine", "Rose Pine") in app.screen._theme_options()
        assert len(app.screen.query("#onboarding-discovery")) == 0
        assert len(app.screen.query("#onboarding-appearance")) == 0
        assert app.screen.query_one("#onboard-offline", Button).display
        assert app.screen.query_one("#onboard-connect", Button).display
        state = str(app.query_one("#sync-state", Static).render())
        assert "no network activity" in state
        assert "hcb auth connect" in str(workspace_rows(app)[0].label)
        await pilot.press("escape")  # type: ignore[attr-defined]

    app_test(app, assertions, size=(58, 24))
    assert notifications == [
        "Detected Rose Pine in Ghostty. Use detected Rose Pine is available under Appearance."
    ]


def test_onboarding_credential_shortcut_opens_the_actual_credential_file(tmp_path: Path) -> None:
    paths = AppPaths(tmp_path / "config", tmp_path / "data", tmp_path / "cache")
    commands: list[list[str]] = []
    suspensions: list[bool] = []
    credential_file = tmp_path / "credentials" / "personal.env"

    @contextmanager
    def suspended() -> Iterator[None]:
        suspensions.append(True)
        yield

    def run_editor(command: list[str]) -> int:
        commands.append(command)
        return 0

    app = HcbApp(
        Runtime(paths, environ={}),
        editor_runner=run_editor,
        suspend=suspended,
    )

    async def assertions(pilot: object) -> None:
        field = app.screen.query_one("#onboard-env-file", Input)
        field.value = str(credential_file)
        await pilot.click("#onboard-env-file")  # type: ignore[attr-defined]
        await pilot.press("ctrl+g")  # type: ignore[attr-defined]
        await pilot.pause()  # type: ignore[attr-defined]
        assert credential_file.read_text(encoding="utf-8").endswith("HCB_GOOGLE_CLIENT_SECRET=\n")
        assert credential_file.stat().st_mode & 0o777 == 0o600

    app_test(app, assertions)
    assert commands == [["nvim", str(credential_file)]]
    assert suspensions == [True]


def test_onboarding_offline_saves_non_secret_config_without_auth(tmp_path: Path) -> None:
    paths = AppPaths(tmp_path / "config", tmp_path / "data", tmp_path / "cache")
    app = HcbApp(Runtime(paths, environ={}))
    credential_file = paths.config_dir / "personal.env"

    async def actions(pilot: object) -> None:
        assert isinstance(app.screen, OnboardingScreen)
        app.screen.query_one("#onboard-env-file", Input).value = str(credential_file)
        app.screen.query_one("#onboard-account", Input).value = "offline"
        app.screen.query_one("#onboard-email", Input).value = "offline@example.test"
        app.screen.query_one("#onboard-timezone", Select).value = "Asia/Singapore"
        app.screen.query_one("#onboard-reminders", Select).value = "false"
        app.screen.query_one("#onboard-theme", Select).value = "Rose Pine"
        await pilot.click("#onboard-offline")  # type: ignore[attr-defined]
        await pilot.pause()  # type: ignore[attr-defined]
        assert app.account_id == "offline"

    app_test(app, actions)
    reopened = Runtime(paths, environ={})
    assert reopened.config.preferences.time_zone == "Asia/Singapore"
    assert "google_client_json" not in paths.config_file.read_text()
    assert not reopened.config.preferences.reminders_enabled
    assert reopened.config.theme.preset == "Rose Pine"
    assert reopened.storage.get_account("offline") is not None
    assert credential_file.read_text().endswith("HCB_GOOGLE_CLIENT_SECRET=\n")
    assert credential_file.stat().st_mode & 0o777 == 0o600
    reopened.close()


def test_onboarding_connect_waits_for_explicit_confirmation(tmp_path: Path) -> None:
    paths = AppPaths(tmp_path / "config", tmp_path / "data", tmp_path / "cache")
    credential_file = tmp_path / "google.env"
    credential_file.write_text("HCB_GOOGLE_CLIENT_ID=public-client\n")
    credential_file.chmod(0o600)
    runtime = Runtime(paths, environ={})
    calls: list[str] = []

    class Authenticator:
        def connect(self, account_id: str, *, expected_email: str) -> None:
            assert expected_email == "google@example.test"
            calls.append(account_id)

    runtime.__dict__["authenticator"] = lambda _account: Authenticator()
    app = HcbApp(runtime)

    async def actions(pilot: object) -> None:
        app.screen.query_one("#onboard-env-file", Input).value = str(credential_file)
        app.screen.query_one("#onboard-account", Input).value = "google"
        app.screen.query_one("#onboard-email", Input).value = "google@example.test"
        await pilot.click("#onboard-connect")  # type: ignore[attr-defined]
        for _ in range(200):
            if isinstance(app.screen, ConfirmScreen):
                break
            await pilot.pause(0.01)  # type: ignore[attr-defined]
        assert calls == []
        assert isinstance(app.screen, ConfirmScreen)
        assert app.screen.query_one("#confirm", Button).label == "Connect"
        await pilot.press("y")  # type: ignore[attr-defined]
        for _ in range(200):
            if calls:
                break
            await pilot.pause(0.01)  # type: ignore[attr-defined]
        assert calls == ["google"]

    app_test(app, actions)
    assert "public-client" not in paths.config_file.read_text()


def test_onboarding_credential_file_suggestion_updates_the_editable_path(tmp_path: Path) -> None:
    paths = AppPaths(tmp_path / "config", tmp_path / "data", tmp_path / "cache")
    credential_file = paths.config_dir / "accounts" / "work.env"
    credential_file.parent.mkdir(parents=True)
    credential_file.write_text("HCB_GOOGLE_CLIENT_ID=client-id\n")
    app = HcbApp(Runtime(paths, environ={}))

    async def actions(pilot: object) -> None:
        selector = app.screen.query_one("#onboard-env-file-suggestion", Select)
        assert (f"Detected · {credential_file}", str(credential_file)) in (
            app.screen._credential_file_options()
        )
        selector.value = str(credential_file)
        await pilot.pause()  # type: ignore[attr-defined]
        assert app.screen.query_one("#onboard-env-file", Input).value == str(credential_file)

    app_test(app, actions)


def test_no_color_forces_mono_ascii_and_disables_mouse(tmp_path: Path) -> None:
    paths = AppPaths(tmp_path / "config", tmp_path / "data", tmp_path / "cache")
    save(Config(theme=Theme(profile="light", borders="unicode", mouse=True)), paths.config_file)
    app = HcbApp(Runtime(paths, environ={"NO_COLOR": "1"}))

    async def assertions(_: object) -> None:
        assert app.theme_mode == "mono"
        assert app.border_style == "ascii"
        assert not app.mouse_enabled
        assert app.has_class("theme-mono", "ascii", "no-mouse")

    app_test(app, assertions)


def test_visual_config_colors_controls_and_surfaces_from_semantic_tokens(tmp_path: Path) -> None:
    runtime = seeded_runtime(tmp_path)
    theme = apply_preset(Theme(), "Flexoki Light")
    save(Config(theme=theme), runtime.paths.config_file)
    app = HcbApp(runtime)

    async def assertions(pilot: object) -> None:
        colors = theme.colors
        assert app.query_one("#surface-tasks", Button).styles.background == Color.parse(
            colors.control
        )
        assert app.query_one("#surface-tasks", Button).styles.auto_color
        assert app.query_one("#resources", ListView).styles.background == Color.parse(colors.panel)
        assert app.query_one("#resources", ListView).styles.auto_color
        assert app.query_one("#content", WorkspaceTable).styles.background == Color.parse(
            colors.background
        )
        app.push_screen(HelpScreen(app))
        await pilot.pause()  # type: ignore[attr-defined]
        assert app.screen.styles.background == Color.parse(colors.overlay)
        assert app.screen.query_one("#help-dialog").styles.background == Color.parse(colors.surface)

    app_test(app, assertions)


def test_visual_config_live_reloads_and_invalid_edits_keep_last_theme(tmp_path: Path) -> None:
    runtime = seeded_runtime(tmp_path)
    app = HcbApp(runtime)
    notifications: list[str] = []

    async def assertions(pilot: object) -> None:
        original_theme = app.theme
        original_notify = app.notify

        def record_notification(message: object, **kwargs: object) -> None:
            notifications.append(str(message))
            original_notify(message, **kwargs)

        app.notify = record_notification  # type: ignore[method-assign]
        save(
            Config(
                theme=Theme(colors=ThemeColors(focus="cyan", border="yellow", accent="magenta"))
            ),
            runtime.paths.config_file,
        )
        await asyncio.sleep(2.2)
        await pilot.pause()  # type: ignore[attr-defined]
        assert app.theme != original_theme
        assert runtime.config.theme.colors.focus == "cyan"
        current_theme = app.theme

        runtime.paths.config_file.write_text('{"theme":{"colors":{"focus":"bad"}}}')
        await asyncio.sleep(2.2)
        await pilot.pause()  # type: ignore[attr-defined]
        assert app.theme == current_theme
        assert runtime.config.theme.colors.focus == "cyan"
        assert any("config.json not applied" in message for message in notifications)

    app_test(app, assertions)


def test_event_modal_create_edit_rsvp_and_delete(tmp_path: Path) -> None:
    runtime = seeded_runtime(tmp_path)
    database = runtime.paths.database_file
    calendar_id = runtime.application.workspace("work").calendars[0].id
    app = HcbApp(runtime, selected_date=date(2026, 8, 21))

    async def actions(pilot: object) -> None:
        await pilot.press("3")  # type: ignore[attr-defined]
        await pilot.press("n")  # type: ignore[attr-defined]
        await pilot.pause()  # type: ignore[attr-defined]
        assert isinstance(app.screen, EventEditorScreen)
        app.screen.query_one("#event-title", Input).value = "Planning"
        app.screen.query_one("#event-start", Input).value = "2026-08-21"
        app.screen.query_one("#event-end", Input).value = "2026-08-22"
        app.screen.query_one("#event-calendar", Input).value = calendar_id
        await pilot.click("#event-save")  # type: ignore[attr-defined]
        await pilot.pause()  # type: ignore[attr-defined]
        await pilot.press("e")  # type: ignore[attr-defined]
        await pilot.pause()  # type: ignore[attr-defined]
        app.screen.query_one("#event-title", Input).value = "Updated planning"
        await pilot.click("#event-save")  # type: ignore[attr-defined]
        await pilot.pause()  # type: ignore[attr-defined]
        await pilot.press("v")  # type: ignore[attr-defined]
        await pilot.pause()  # type: ignore[attr-defined]
        assert isinstance(app.screen, RsvpScreen)
        await pilot.click("#accepted")  # type: ignore[attr-defined]
        await pilot.pause()  # type: ignore[attr-defined]
        assert isinstance(app.screen, BatchActionScreen)
        await pilot.click("#batch-action-apply")  # type: ignore[attr-defined]
        await pilot.pause()  # type: ignore[attr-defined]
        assert app.cache.pending >= 2
        await pilot.press("e")  # type: ignore[attr-defined]
        await pilot.pause()  # type: ignore[attr-defined]
        assert app.screen.query_one("#event-delete", Button).display
        await pilot.click("#event-delete")  # type: ignore[attr-defined]
        await pilot.pause()  # type: ignore[attr-defined]
        assert isinstance(app.screen, ConfirmScreen)
        await pilot.press("y")  # type: ignore[attr-defined]
        await pilot.pause()  # type: ignore[attr-defined]

    app_test(app, actions)
    with Storage(database) as storage:
        assert storage.list_events("work") == []


def test_palette_calendar_and_settings_workflows(tmp_path: Path) -> None:
    runtime = seeded_runtime(tmp_path)
    config_path = runtime.paths.config_file
    app = HcbApp(runtime)

    async def actions(pilot: object) -> None:
        await activate_palette(pilot, app, "Calendars")
        assert isinstance(app.screen, CalendarScreen)
        app.screen.query_one("#calendar-name", Input).value = "Team"
        await pilot.click("#calendar-add")  # type: ignore[attr-defined]
        await pilot.pause()  # type: ignore[attr-defined]
        assert any(row[1] == "Team" for row in app.calendar_rows())
        calendar_list = app.screen.query_one("#calendar-list", ListView)
        calendar_list.index = len(calendar_list.children) - 1
        await pilot.pause()  # type: ignore[attr-defined]
        await pilot.click("#calendar-toggle")  # type: ignore[attr-defined]
        await pilot.pause()  # type: ignore[attr-defined]
        assert next(row[2] for row in app.calendar_rows() if row[1] == "Team") is False
        calendar_list.index = len(calendar_list.children) - 1
        await pilot.pause()  # type: ignore[attr-defined]
        await pilot.click("#calendar-delete")  # type: ignore[attr-defined]
        await pilot.press("y")  # type: ignore[attr-defined]
        await pilot.pause()  # type: ignore[attr-defined]
        assert all(row[1] != "Team" for row in app.calendar_rows())
        await pilot.press("escape")  # type: ignore[attr-defined]
        await activate_palette(pilot, app, "Settings")
        assert isinstance(app.screen, SettingsScreen)
        for selector in (
            "#setting-profile",
            "#setting-density",
            "#setting-borders",
            "#setting-focus",
            "#setting-mouse",
            "#setting-week",
            "#setting-date-time-format",
        ):
            assert isinstance(app.screen.query_one(selector), Select)
        app.screen.query_one("#setting-density", Select).value = "compact"
        app.screen.query_one("#setting-borders", Select).value = "ascii"
        app.screen.query_one("#setting-mouse", Select).value = "false"
        app.screen.query_one("#setting-date-time-format", Select).value = "friendly_24h"
        app.screen.query_one("#setting-editor", Input).value = "hx"
        app.screen.query_one("#setting-external-editor", Input).value = "ctrl+o"
        app.screen.query_one("#setting-loader", Select).value = "emoji.weather"
        app.screen.query_one("#setting-reminder-catch-up", Input).value = "90"
        app.screen.query_one("#setting-reminder-jitter", Input).value = "3"
        app.screen.query_one("#setting-reminder-sync-interval", Input).value = "15"
        app.screen.query_one("#setting-reminder-sync-mode", Select).value = "pull"
        await pilot.click("#settings-save")  # type: ignore[attr-defined]
        await pilot.pause()  # type: ignore[attr-defined]
        assert app.has_class("density-compact", "ascii", "no-mouse")

    app_test(app, actions)
    saved = Runtime(app.runtime.paths, environ={}).config
    assert config_path.exists()
    assert saved.theme.density == "compact"
    assert saved.theme.borders == "ascii"
    assert not saved.theme.mouse
    assert saved.theme.loader == "emoji.weather"
    assert saved.preferences.date_time_format == "friendly_24h"
    assert saved.preferences.editor == "hx"
    assert saved.keys.external_editor == "ctrl+o"
    assert saved.preferences.reminder_catch_up_minutes == 90
    assert saved.preferences.reminder_jitter_seconds == 3
    assert saved.preferences.reminder_sync_interval_minutes == 15
    assert saved.preferences.reminder_sync_mode == "pull"


def test_settings_theme_selector_offers_and_applies_detected_theme(tmp_path: Path) -> None:
    runtime = seeded_runtime(tmp_path)
    app = HcbApp(
        runtime,
        local_environment=LocalEnvironment(
            "macOS",
            "arm64",
            "Ghostty",
            terminal_theme="Rose Pine",
            suggested_preset="Rose Pine",
        ),
    )

    async def actions(pilot: object) -> None:
        await activate_palette(pilot, app, "Settings")
        assert isinstance(app.screen, SettingsScreen)
        assert ("Use detected Rose Pine", "Rose Pine") in app.screen._theme_options()
        app.screen.query_one("#setting-theme", Select).value = "Rose Pine"
        await pilot.pause()  # type: ignore[attr-defined]
        selected = preset("Rose Pine")
        assert app.screen.query_one("#setting-profile", Select).value == selected.profile
        colors = app.screen.query_one("#settings-colors", TerminalTextArea).text
        assert ThemeColors(**json.loads(colors)) == selected.colors
        await pilot.click("#settings-save")  # type: ignore[attr-defined]
        await pilot.pause()  # type: ignore[attr-defined]
        await activate_palette(pilot, app, "Settings")
        assert isinstance(app.screen, SettingsScreen)
        assert app.screen.query_one("#setting-theme", Select).value == "Rose Pine"
        await pilot.press("escape")  # type: ignore[attr-defined]

    app_test(app, actions, size=(80, 18))
    assert runtime.config.theme.preset == "Rose Pine"
    assert runtime.config.theme.profile == preset("Rose Pine").profile


def test_loading_surface_renders_the_selected_rattles_loader(tmp_path: Path) -> None:
    app = HcbApp(seeded_runtime(tmp_path))

    async def assertions(pilot: object) -> None:
        app.start_loading("Syncing with Google")
        await pilot.pause()  # type: ignore[attr-defined]
        assert isinstance(app.screen, LoadingScreen)
        loader = app.screen.query_one("#rattles-loader", Static)
        assert str(loader.render()).strip() in {"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
        app.update_loading("Fetching task lists")
        assert (
            str(app.screen.query_one("#loading-message", Label).render()) == "Fetching task lists"
        )
        app.stop_loading()
        await pilot.pause()  # type: ignore[attr-defined]
        assert not isinstance(app.screen, LoadingScreen)

    app_test(app, assertions)


def test_loading_progress_keeps_the_resource_list_and_selection_stable(tmp_path: Path) -> None:
    app = HcbApp(seeded_runtime(tmp_path))

    async def assertions(pilot: object) -> None:
        resources = app.query_one("#resources", ListView)
        before = tuple(resources.children)
        resources.index = 1
        app.resource_filter = ("task-list", before[1].item_id)

        app.start_loading("Syncing with Google")
        await pilot.pause()  # type: ignore[attr-defined]
        app.update_loading("Fetching task lists")
        app.stop_loading()
        await pilot.pause()  # type: ignore[attr-defined]

        assert tuple(resources.children) == before
        assert resources.index == 1
        assert app.resource_filter == ("task-list", before[1].item_id)

    app_test(app, assertions)


def test_unicode_chrome_keeps_a_content_row_and_single_edge_dividers(tmp_path: Path) -> None:
    runtime = seeded_runtime(tmp_path)
    save(Config(theme=Theme(borders="unicode")), runtime.paths.config_file)
    app = HcbApp(runtime)

    async def assertions(pilot: object) -> None:
        topbar = app.query_one("#topbar", Static)
        tabs = app.query_one("#tabs")
        sidebar_resize = app.query_one("#sidebar-resize", Static)

        assert topbar.content_size.height >= 1
        assert tabs.content_size.height >= 1
        assert topbar.styles.border_top[0] == ""
        assert topbar.styles.border_bottom[0] == "solid"
        assert sidebar_resize.styles.border_left[0] == "solid"
        assert sidebar_resize.styles.border_top[0] == ""

    app_test(app, assertions)


def test_ascii_chrome_keeps_a_visible_title_and_bottom_divider(tmp_path: Path) -> None:
    runtime = seeded_runtime(tmp_path)
    save(Config(theme=Theme(borders="ascii")), runtime.paths.config_file)
    app = HcbApp(runtime)

    async def assertions(pilot: object) -> None:
        topbar = app.query_one("#topbar", Static)

        assert topbar.content_size.height >= 1
        assert "HCB" in str(topbar.render())
        assert topbar.styles.border_top[0] == ""
        assert topbar.styles.border_bottom[0] == "ascii"

    app_test(app, assertions)


def test_mini_month_click_selects_a_day_without_rebuilding_resources(tmp_path: Path) -> None:
    runtime = seeded_runtime(tmp_path)
    save(Config(theme=Theme(borders="unicode")), runtime.paths.config_file)
    app = HcbApp(runtime, selected_date=date(2026, 8, 23))

    async def assertions(pilot: object) -> None:
        resources = app.query_one("#resources", ListView)
        before = tuple(resources.children)
        button_id = next(
            button_id
            for button_id, value in app._mini_month_days.items()
            if value == date(2026, 8, 3)
        )

        await pilot.click(f"#{button_id}")  # type: ignore[attr-defined]
        await pilot.pause()  # type: ignore[attr-defined]

        assert app.selected_date == date(2026, 8, 3)
        assert tuple(resources.children) == before
        assert "Monday, 03 August 2026" in str(app.query_one("#surface-title", Static).render())

    app_test(app, assertions)


def test_mini_month_visibly_marks_the_selected_day(tmp_path: Path) -> None:
    app = HcbApp(seeded_runtime(tmp_path), selected_date=date(2026, 8, 23))

    async def assertions(pilot: object) -> None:
        button_id = next(
            button_id
            for button_id, value in app._mini_month_days.items()
            if value == date(2026, 8, 23)
        )
        button = app.query_one(f"#{button_id}", Button)
        assert button.label == "23"
        assert button.has_class("mini-day-selected")

    app_test(app, assertions)


def test_mini_month_renders_at_narrow_terminal_widths(tmp_path: Path) -> None:
    app = HcbApp(seeded_runtime(tmp_path), selected_date=date(2026, 8, 23))

    async def assertions(_: object) -> None:
        button_id = next(
            button_id
            for button_id, value in app._mini_month_days.items()
            if value == date(2026, 8, 31)
        )
        button = app.query_one(f"#{button_id}", Button)
        assert button.size.width >= 2
        assert button.label == "31"

    app_test(app, assertions, size=(80, 34))


def test_sync_worker_uses_an_isolated_sqlite_connection(tmp_path: Path) -> None:
    runtime = seeded_runtime(tmp_path)
    original_storage = runtime.storage
    engines: list[object] = []

    class Result:
        pulled = 0
        pushed = 0
        conflicts = 0
        cancelled = False
        retry_pending = False
        retry_exhausted = False

    class Engine:
        def __init__(self) -> None:
            self.storage = original_storage

        def sync(
            self,
            account_id: str,
            *,
            progress: Callable[[str], None] | None = None,
            cancelled: Callable[[], bool] | None = None,
            cancel_hint: str | None = None,
        ) -> Result:
            assert self.storage is not original_storage
            assert self.storage.get_account(account_id) is not None
            assert progress is not None
            assert cancelled is not None
            assert cancel_hint == "Press Esc to cancel."
            progress("Fetching task lists")
            return Result()

    def sync_engine(_: str) -> Engine:
        engine = Engine()
        engines.append(engine)
        return engine

    runtime.sync_engine = sync_engine  # type: ignore[method-assign,assignment]
    app = HcbApp(runtime)

    async def assertions(pilot: object) -> None:
        app.action_sync()
        for _ in range(20):
            await asyncio.sleep(0.01)
            await pilot.pause()  # type: ignore[attr-defined]
            if engines and not isinstance(app.screen, LoadingScreen):
                break
        assert engines
        assert not isinstance(app.screen, LoadingScreen)

    app_test(app, assertions)


def test_sync_loading_surface_cancels_a_retry_wait(tmp_path: Path) -> None:
    runtime = seeded_runtime(tmp_path)
    started = ThreadEvent()

    class Result:
        pulled = 0
        pushed = 0
        cancelled = True
        retry_exhausted = False
        retry_message = "Sync cancelled. Local changes remain queued."

    class Engine:
        def __init__(self) -> None:
            self.storage = runtime.storage

        def sync(
            self,
            account_id: str,
            *,
            progress: Callable[[str], None] | None = None,
            cancelled: Callable[[], bool] | None = None,
            cancel_hint: str | None = None,
        ) -> Result:
            assert progress is not None and cancelled is not None
            progress("Sending local change temporarily failed; retrying 1/4 in 1.0s.")
            started.set()
            for _ in range(100):
                if cancelled():
                    return Result()
                sleep(0.01)
            raise AssertionError("sync cancellation was not delivered")

    runtime.sync_engine = lambda _: Engine()  # type: ignore[method-assign,assignment]
    app = HcbApp(runtime)

    async def assertions(pilot: object) -> None:
        app.action_sync()
        for _ in range(30):
            await asyncio.sleep(0.01)
            await pilot.pause()  # type: ignore[attr-defined]
            if started.is_set() and isinstance(app.screen, LoadingScreen):
                break
        assert isinstance(app.screen, LoadingScreen)
        assert app.screen.cancellable
        cancel_hint = app.screen.query_one("#loading-cancel-hint", Label)
        assert "Esc to cancel" in str(cancel_hint.render())
        assert cancel_hint.styles.content_align_horizontal == "center"
        await pilot.press("escape")  # type: ignore[attr-defined]
        for _ in range(30):
            await asyncio.sleep(0.01)
            await pilot.pause()  # type: ignore[attr-defined]
            if not isinstance(app.screen, LoadingScreen):
                break
        assert not isinstance(app.screen, LoadingScreen)

    app_test(app, assertions)


def test_sync_without_google_credentials_shows_recovery_steps(tmp_path: Path) -> None:
    runtime = seeded_runtime(tmp_path)
    runtime.credential_file_override = tmp_path / "missing-google.env"
    app = HcbApp(runtime)

    async def assertions(pilot: object) -> None:
        app.action_sync()
        for _ in range(30):
            await asyncio.sleep(0.01)
            await pilot.pause()  # type: ignore[attr-defined]
            if isinstance(app.screen, GoogleSetupScreen):
                break
        assert isinstance(app.screen, GoogleSetupScreen)
        guidance = str(app.screen.query_one("#google-setup-guidance", Static).render())
        assert "Desktop OAuth client" in guidance
        assert "HCB_GOOGLE_CLIENT_ID" in guidance
        assert str(runtime.credential_file("work")) in guidance
        assert "hcb --env-file" in guidance
        assert "auth connect work me@example.com" in guidance

    app_test(app, assertions)


def test_recovery_modal_centers_in_a_half_terminal(tmp_path: Path) -> None:
    runtime = seeded_runtime(tmp_path)
    app = HcbApp(runtime)

    async def assertions(pilot: object) -> None:
        app.push_screen(
            GoogleSetupScreen(
                operation="Sync",
                account_id="work",
                email="me@example.com",
                credential_file=runtime.credential_file("work"),
                reason="credentials are unavailable",
            )
        )
        await pilot.pause()  # type: ignore[attr-defined]
        dialog = app.screen.query_one("#google-setup-dialog")
        center_x = dialog.region.x + dialog.region.width / 2
        center_y = dialog.region.y + dialog.region.height / 2
        assert abs(center_x - app.size.width / 2) <= 1
        assert abs(center_y - app.size.height / 2) <= 1

    app_test(app, assertions, size=(80, 18))


def test_palette_find_time_uses_only_cached_events(tmp_path: Path) -> None:
    runtime = seeded_runtime(tmp_path)
    calendar_id = runtime.application.workspace("work").calendars[0].id
    runtime.application.create_event(
        "work",
        calendar_id,
        "Busy morning",
        EventDateTime(DateTimeKind.DATETIME, datetime(2026, 8, 21, 9, tzinfo=UTC)),
        EventDateTime(DateTimeKind.DATETIME, datetime(2026, 8, 21, 10, tzinfo=UTC)),
    )
    app = HcbApp(runtime, selected_date=date(2026, 8, 21))

    async def assertions(pilot: object) -> None:
        await activate_palette(pilot, app, "Find a time")
        assert isinstance(app.screen, FindTimeScreen)
        rows = app.screen.query_one("#find-results", ListView)
        labels = [str(row.query_one("Label").render()) for row in rows.children]
        assert labels[0].startswith("10:00")
        assert "never queried" in str(app.screen.query_one("#find-disclosure", Static).render())

    app_test(app, assertions)


def test_schedule_bulk_undo_redo_and_import_dialogs(
    tmp_path: Path,
) -> None:
    runtime = seeded_runtime(tmp_path)
    database = runtime.paths.database_file
    source = tmp_path / "import.json"
    source.write_text('{"version":1,"records":[{"kind":"task","title":"Imported in TUI"}]}')
    app = HcbApp(runtime)

    async def actions(pilot: object) -> None:
        task_id = app.cache.tasks[0].id
        app.selected = ("task", task_id)
        await activate_palette(pilot, app, "Schedule task")
        assert isinstance(app.screen, ScheduleScreen)
        app.screen.query_one("#schedule-start", Input).value = "2026-08-21T09:00:00Z"
        app.screen.query_one("#schedule-end", Input).value = "2026-08-21T10:00:00Z"
        await pilot.click("#schedule-save")  # type: ignore[attr-defined]
        await pilot.pause()  # type: ignore[attr-defined]
        assert runtime.application.list_task_event_links("work")
        await pilot.click("#schedule-remove")  # type: ignore[attr-defined]
        await pilot.press("y")  # type: ignore[attr-defined]
        await pilot.pause()  # type: ignore[attr-defined]
        await pilot.click("#schedule-close")  # type: ignore[attr-defined]
        await pilot.pause()  # type: ignore[attr-defined]

        app.selected = ("task", task_id)
        app.marked = {task_id}
        await activate_palette(pilot, app, "Bulk actions")
        await pilot.click("#bulk-complete")  # type: ignore[attr-defined]
        await pilot.pause()  # type: ignore[attr-defined]
        assert isinstance(app.screen, BatchActionScreen)
        await pilot.click("#batch-action-apply")  # type: ignore[attr-defined]
        await pilot.pause()  # type: ignore[attr-defined]
        assert runtime.storage.get_task("work", task_id).status.value == "completed"
        await pilot.press("u")  # type: ignore[attr-defined]
        await pilot.pause()  # type: ignore[attr-defined]
        assert runtime.storage.get_task("work", task_id).status.value == "needsAction"
        await pilot.press("ctrl+r")  # type: ignore[attr-defined]
        await pilot.pause()  # type: ignore[attr-defined]
        assert runtime.storage.get_task("work", task_id).status.value == "completed"

        await activate_palette(pilot, app, "Import")
        assert isinstance(app.screen, ImportScreen)
        app.screen.query_one("#import-path", Input).value = str(source)
        await pilot.click("#import-preview")  # type: ignore[attr-defined]
        await pilot.pause()  # type: ignore[attr-defined]
        assert "Accepted: 1" in str(app.screen.query_one("#import-summary", Static).render())
        await pilot.click("#import-apply")  # type: ignore[attr-defined]
        await pilot.press("y")  # type: ignore[attr-defined]
        await pilot.pause()  # type: ignore[attr-defined]

    app_test(app, actions)
    with Storage(database) as storage:
        assert any(task.title == "Imported in TUI" for task in storage.list_tasks("work"))


def test_import_path_shortcut_opens_the_source_and_discards_its_preview(tmp_path: Path) -> None:
    runtime = seeded_runtime(tmp_path)
    source = tmp_path / "import.json"
    source.write_text('{"version":1,"records":[{"kind":"task","title":"Imported"}]}')
    commands: list[list[str]] = []
    suspensions: list[bool] = []

    @contextmanager
    def suspended() -> Iterator[None]:
        suspensions.append(True)
        yield

    def run_editor(command: list[str]) -> int:
        commands.append(command)
        return 0

    app = HcbApp(runtime, editor_runner=run_editor, suspend=suspended)

    async def actions(pilot: object) -> None:
        await activate_palette(pilot, app, "Import")
        assert isinstance(app.screen, ImportScreen)
        field = app.screen.query_one("#import-path", Input)
        field.value = str(source)
        await pilot.click("#import-preview")  # type: ignore[attr-defined]
        await pilot.pause()  # type: ignore[attr-defined]
        assert app.screen.preview is not None
        assert (
            str(app.screen.query_one("#import-editor-hint", Label).render())
            == "Ctrl+G to open this import file in nvim"
        )
        await pilot.click("#import-path")  # type: ignore[attr-defined]
        await pilot.press("ctrl+g")  # type: ignore[attr-defined]
        await pilot.pause()  # type: ignore[attr-defined]
        assert app.screen.preview is None
        assert "choose Preview again" in str(
            app.screen.query_one("#import-summary", Static).render()
        )

    app_test(app, actions)
    assert commands == [["nvim", str(source)]]
    assert suspensions == [True]


def test_settings_can_open_and_apply_the_complete_config_file(tmp_path: Path) -> None:
    runtime = seeded_runtime(tmp_path)
    commands: list[list[str]] = []

    @contextmanager
    def suspended() -> Iterator[None]:
        yield

    def run_editor(command: list[str]) -> int:
        commands.append(command)
        Path(command[-1]).write_text(
            '{"preferences":{"date_time_format":"iso"},"theme":{"density":"compact"}}\n',
            encoding="utf-8",
        )
        return 0

    app = HcbApp(runtime, editor_runner=run_editor, suspend=suspended)

    async def actions(pilot: object) -> None:
        await activate_palette(pilot, app, "Settings")
        assert isinstance(app.screen, SettingsScreen)
        await pilot.click("#settings-edit-config")  # type: ignore[attr-defined]
        await pilot.pause()  # type: ignore[attr-defined]
        assert app.runtime.config.preferences.date_time_format == "iso"
        assert app.has_class("density-compact")

    app_test(app, actions)
    assert commands == [["nvim", str(runtime.paths.config_file)]]


def test_keyboard_marking_and_batch_move_review_keep_the_selection_visible(tmp_path: Path) -> None:
    runtime = seeded_runtime(tmp_path)
    archive = runtime.application.create_task_list("work", "Archive")
    source_calendar = runtime.storage.list_calendars("work")[0]
    calendar_archive = runtime.application.create_calendar("work", "Calendar archive")
    event = runtime.application.create_event(
        "work",
        source_calendar.id,
        "Move me",
        EventDateTime(DateTimeKind.DATETIME, datetime(2026, 8, 21, 9, tzinfo=UTC)),
        EventDateTime(DateTimeKind.DATETIME, datetime(2026, 8, 21, 10, tzinfo=UTC)),
    )
    app = HcbApp(runtime, selected_date=date(2026, 8, 21))

    async def actions(pilot: object) -> None:
        task_id = app.cache.tasks[0].id
        app.selected = ("task", task_id)
        await pilot.press("x")  # type: ignore[attr-defined]
        await pilot.pause()  # type: ignore[attr-defined]
        assert task_id in app.marked
        assert "selected: 1 task(s)" in str(app.query_one("#surface-title", Static).render())

        await activate_palette(pilot, app, "Bulk actions")
        await pilot.click("#bulk-move-tasks")  # type: ignore[attr-defined]
        await pilot.pause()  # type: ignore[attr-defined]
        assert isinstance(app.screen, BatchMoveScreen)
        app.screen.query_one("#batch-move-destination", Select).value = archive.id
        await pilot.click("#batch-move-review")  # type: ignore[attr-defined]
        await pilot.pause()  # type: ignore[attr-defined]
        review = str(app.screen.query_one("#batch-move-summary", Static).render())
        assert "Ship TUI" in review and "Inbox" in review and "Archive" in review
        assert app.screen.preview is not None
        assert not app.screen.query_one("#batch-move-confirm", Button).disabled
        assert app.screen.query_one("#batch-move-destination", Select).value == archive.id
        await pilot.click("#batch-move-confirm")  # type: ignore[attr-defined]
        await pilot.pause()  # type: ignore[attr-defined]
        assert runtime.storage.get_task("work", task_id).list_id == archive.id  # type: ignore[union-attr]
        assert task_id in app.marked

        app.surface = "Agenda"
        app.selected = ("event", event.id)
        app._render_surface()
        await pilot.pause()  # type: ignore[attr-defined]
        app.selected = ("event", event.id)
        app.action_mark()
        await pilot.pause()  # type: ignore[attr-defined]
        assert event.id in app.marked_events
        app.action_move_marked("event")
        await pilot.pause()  # type: ignore[attr-defined]
        assert isinstance(app.screen, BatchMoveScreen)
        app.screen.query_one("#batch-move-destination", Select).value = calendar_archive.id
        await pilot.click("#batch-move-review")  # type: ignore[attr-defined]
        await pilot.pause()  # type: ignore[attr-defined]
        await pilot.click("#batch-move-confirm")  # type: ignore[attr-defined]
        await pilot.pause()  # type: ignore[attr-defined]
        assert runtime.storage.get_event("work", event.id).calendar_id == calendar_archive.id  # type: ignore[union-attr]
        assert event.id in app.marked_events

        app.action_rsvp()
        await pilot.pause()  # type: ignore[attr-defined]
        assert isinstance(app.screen, RsvpScreen)
        await pilot.click("#accepted")  # type: ignore[attr-defined]
        await pilot.pause()  # type: ignore[attr-defined]
        assert isinstance(app.screen, BatchActionScreen)
        review = str(app.screen.query_one("#batch-action-summary", Static).render())
        assert "Move me" in review and "needsAction → accepted" in review
        await pilot.click("#batch-action-apply")  # type: ignore[attr-defined]
        await pilot.pause()  # type: ignore[attr-defined]
        assert event.id in app.marked_events

        app.marked.add(task_id)
        app.action_delete()
        await pilot.pause()  # type: ignore[attr-defined]
        assert not isinstance(app.screen, BatchActionScreen)
        assert task_id in app.marked and event.id in app.marked_events

    app_test(app, actions)


def test_pointer_marking_and_batch_review_show_exact_task_change(tmp_path: Path) -> None:
    runtime = seeded_runtime(tmp_path)
    app = HcbApp(runtime)

    async def actions(pilot: object) -> None:
        task = app.cache.tasks[0]
        content = app.query_one("#content", WorkspaceTable)
        await pilot.click(content, offset=(4, 0), control=True)  # type: ignore[attr-defined]
        await pilot.pause()  # type: ignore[attr-defined]
        assert task.id in app.marked
        assert "selected: 1 task(s)" in str(app.query_one("#surface-title", Static).render())

        app.action_complete()
        await pilot.pause()  # type: ignore[attr-defined]
        assert isinstance(app.screen, BatchActionScreen)
        review = str(app.screen.query_one("#batch-action-summary", Static).render())
        assert "Ship TUI" in review and "needsAction → completed" in review
        await pilot.click("#batch-action-apply")  # type: ignore[attr-defined]
        await pilot.pause()  # type: ignore[attr-defined]
        assert runtime.storage.get_task("work", task.id).status.value == "completed"
        assert task.id in app.marked

    app_test(app, actions)


def test_conflict_resolution_and_explicit_remote_freebusy(tmp_path: Path) -> None:
    runtime = seeded_runtime(tmp_path)
    runtime.storage.add_conflict(
        Conflict(None, "work", EntityType.TASK, "task", {"body": {}}, {"body": {}})
    )
    calls: list[dict[str, object]] = []

    class Gateway:
        def freebusy(self, body: dict[str, object]) -> dict[str, object]:
            calls.append(body)
            return {"calendars": {"primary": {"busy": [{"start": "a", "end": "b"}]}}}

    class Engine:
        gateway = Gateway()

    runtime.sync_engine = lambda _account: Engine()  # type: ignore[method-assign,assignment]
    app = HcbApp(runtime, selected_date=date(2026, 8, 21))

    async def actions(pilot: object) -> None:
        await activate_palette(pilot, app, "Conflicts")
        assert isinstance(app.screen, ConflictScreen)
        await pilot.click("#conflict-remote")  # type: ignore[attr-defined]
        await pilot.pause()  # type: ignore[attr-defined]
        assert runtime.storage.list_conflicts("work") == []

        await activate_palette(pilot, app, "Find a time")
        assert isinstance(app.screen, FindTimeScreen)
        await pilot.click("#find-remote")  # type: ignore[attr-defined]
        for _ in range(20):
            await pilot.pause()  # type: ignore[attr-defined]
            if calls:
                break
        assert calls
        assert isinstance(app.screen, FindTimeScreen)
        assert "Explicit Google free/busy" in str(
            app.screen.query_one("#find-disclosure", Static).render()
        )

    app_test(app, actions)


def test_resize_preserves_unicode_selection_and_restores_sidebar(tmp_path: Path) -> None:
    runtime = seeded_runtime(tmp_path)
    task_list_id = runtime.application.workspace("work").task_lists[0].id
    for index in range(35):
        runtime.application.create_task(
            "work",
            task_list_id,
            f"長い予定 🚆 item {index:02d}",
        )
    app = HcbApp(runtime)

    async def assertions(pilot: object) -> None:
        rows = app.query_one("#content", WorkspaceTable)
        rows.move_cursor(row=rows.row_count - 1, animate=False)
        await pilot.pause()  # type: ignore[attr-defined]
        selected = rows.row_at(rows.cursor_row)
        assert selected is not None
        selected_id = selected.item_id
        assert "長い予定" in str(selected.label)

        await pilot.resize_terminal(44, 18)  # type: ignore[attr-defined]
        await pilot.pause()  # type: ignore[attr-defined]
        assert app.has_class("very-narrow")
        assert rows.row_at(rows.cursor_row) is not None
        assert rows.row_at(rows.cursor_row).item_id == selected_id
        assert app.query_one("#sidebar").display is False

        await pilot.resize_terminal(120, 38)  # type: ignore[attr-defined]
        await pilot.pause()  # type: ignore[attr-defined]
        assert not app.has_class("narrow")
        assert rows.row_at(rows.cursor_row) is not None
        assert rows.row_at(rows.cursor_row).item_id == selected_id
        assert app.query_one("#sidebar").display is True

    app_test(app, assertions)


def test_tui_uncertain_delivery_requires_explicit_remote_id(tmp_path: Path) -> None:
    runtime = seeded_runtime(tmp_path)
    task = runtime.application.workspace("work").tasks[0]
    runtime.storage.connection.execute(
        "DELETE FROM outbox WHERE account_id=? AND entity_id=?",
        ("work", task.id),
    )
    runtime.storage.add_conflict(
        Conflict(
            None,
            "work",
            EntityType.TASK,
            task.id,
            {
                "kind": "uncertain-delivery",
                "mutation": {
                    "entity_type": "task",
                    "entity_id": task.id,
                    "operation": "create",
                    "payload": {
                        "list_id": task.list_id,
                        "body": {"title": task.title},
                    },
                    "request_id": None,
                },
            },
            {"kind": "delivery-status-unknown"},
        )
    )
    app = HcbApp(runtime)

    async def actions(pilot: object) -> None:
        await activate_palette(pilot, app, "Conflicts")
        assert isinstance(app.screen, ConflictScreen)
        app.screen.query_one("#conflict-remote-id", Input).value = "remote-task-id"
        await pilot.click("#conflict-remote")  # type: ignore[attr-defined]
        await pilot.pause()  # type: ignore[attr-defined]
        assert runtime.storage.list_conflicts("work") == []
        assert runtime.storage.get_task("work", task.id).remote_id == "remote-task-id"

    app_test(app, actions)
