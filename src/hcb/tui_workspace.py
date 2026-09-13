"""Workspace loading, rendering, and selection handlers."""

from __future__ import annotations

import calendar
from dataclasses import replace
from datetime import UTC, date, datetime, timedelta
from typing import TYPE_CHECKING, Any, cast
from zoneinfo import ZoneInfo

from rich.style import Style
from rich.text import Text
from textual import events
from textual.widgets import (
    Button,
    ListView,
    Static,
)

from .application import (
    SearchResult,
)
from .models import DriveFile, Event, Task, TaskStatus
from .tui import GOOGLE_EVENT_COLORS, linkify_urls, role_rich_style
from .tui_calendar import CalendarSurface, calendar_range
from .tui_components import (
    CachedWorkspace,
    CalendarGrid,
    CalendarOverflowResult,
    CalendarOverflowScreen,
    EntityRow,
    LoadingScreen,
    WorkspaceRow,
    WorkspaceTable,
)

if TYPE_CHECKING:
    pass


class WorkspaceMixin:
    loading_operation: str | None
    _loading_screen: LoadingScreen | None
    selected: tuple[str, str] | None
    resource_filter: tuple[str, str] | None
    surface: str
    _mini_month_render_key: tuple[date, int, str] | None
    _instance_badge_cache: dict[tuple[object, ...], str | None]

    def on_resize(self: Any, _: events.Resize) -> None:
        """Swap between grid and list projection when terminal width changes."""
        if self.surface in {"Day", "Week", "Month"}:
            self.call_after_refresh(self._render_surface)

    def refresh_workspace(self: Any) -> None:
        """Refresh the UI cache through the application controller boundary."""
        if self.account_id is None:
            self.cache = CachedWorkspace()
            self._render_onboarding()
            return
        snapshot = self.runtime.application.workspace(self.account_id)
        notes_enabled = (
            self.runtime.application.notes_projection(self.account_id).value != "disabled"
        )
        self.query_one("#surface-notes", Button).display = notes_enabled
        if not notes_enabled and self.surface == "Notes":
            self.surface = "Tasks"
        self.cache = CachedWorkspace(
            identity=snapshot.account.email,
            tasks=snapshot.tasks,
            events=snapshot.events,
            task_lists=tuple((item.id, item.title) for item in snapshot.task_lists),
            calendars=tuple((item.id, item.summary, item.selected) for item in snapshot.calendars),
            instance_ranges=snapshot.instance_ranges,
            pending=snapshot.pending,
        )
        self._instance_badge_cache.clear()
        self.marked.intersection_update(task.id for task in self.cache.tasks)
        self.marked_events.intersection_update(event.id for event in self.cache.events)
        self._render_chrome()
        self._render_surface()

    def _render_onboarding(self: Any) -> None:
        self.query_one("#topbar", Static).update("HCB  ·  offline workspace")
        self._render_mini_month()
        self.query_one("#resources", ListView).clear()
        self.query_one("#sync-state", Static).update("Not connected · no network activity")
        self.query_one("#surface-title", Static).update("Welcome")
        content = self.query_one("#content", WorkspaceTable)
        self.query_one("#calendar-content", CalendarGrid).display = False
        content.display = True
        content.replace_rows(
            (
                WorkspaceRow(
                    "onboarding",
                    "connect",
                    Text("No account configured. Run: hcb auth connect ACCOUNT EMAIL"),
                ),
            )
        )

    def _render_chrome(self: Any, *, refresh_resources: bool = True) -> None:
        """Refresh chrome; only rebuild resources when workspace data has changed."""
        self.query_one("#topbar", Static).update(
            f"HCB  ·  {self.cache.identity}  ·  {self.format_date(self._present_date())}"
            "  ·  / command palette"
        )
        self._render_mini_month()
        if refresh_resources:
            resources = self.query_one("#resources", ListView)
            resources.clear()
            resources.append(EntityRow("All resources", kind="resource-all", item_id="all"))
            for item_id, title in self.cache.task_lists:
                resources.append(EntityRow(f"☐ {title}", kind="task-list", item_id=item_id))
            for item_id, title, _ in self.cache.calendars:
                resources.append(EntityRow(f"□ {title}", kind="calendar", item_id=item_id))
        self._update_resource_selection()
        state = self.loading_operation or "offline cache"
        cache_badge = self._instance_cache_badge()
        suffix = f" · {cache_badge}" if cache_badge else ""
        self.query_one("#sync-state", Static).update(
            f"{state} · {self.cache.pending} pending{suffix}"
        )

    def start_loading(self: Any, operation: str, *, cancellable: bool = False) -> None:
        """Show the selected loader while one explicit remote operation is in progress."""
        self.loading_operation = operation
        if self._loading_screen is None:
            self._loading_screen = LoadingScreen(operation, cancellable=cancellable)
            self.push_screen(self._loading_screen)
        else:
            self._loading_screen.set_message(operation)
        self._render_chrome(refresh_resources=False)

    def cancel_sync(self: Any) -> None:
        """Ask the active sync to stop before its next Google request."""
        if not self._sync_lock.locked():
            return
        self._sync_cancel.set()
        self.update_loading("Cancelling sync after the current request")

    def update_loading(self: Any, status: str) -> None:
        """Display the current, concrete stage of the active remote operation."""
        self.loading_operation = status
        if self._loading_screen is not None:
            self._loading_screen.set_message(status)
        self._render_chrome(refresh_resources=False)

    def stop_loading(self: Any) -> None:
        """Remove the active loading surface after its worker has completed."""
        screen = self._loading_screen
        self.loading_operation = None
        self._loading_screen = None
        if screen is not None and self.screen is screen:
            self.pop_screen()
        self._render_chrome(refresh_resources=False)

    def _render_mini_month(self: Any) -> None:
        """Render date controls for the sidebar's current calendar month."""
        render_key = (
            self.selected_date,
            self.runtime.config.preferences.week_starts_on,
            self.border_style,
        )
        if render_key == self._mini_month_render_key:
            return
        self._mini_month_render_key = render_key
        cal = calendar.TextCalendar(self.runtime.config.preferences.week_starts_on)
        self.query_one("#mini-month-title", Static).update(f"{self.selected_date:%B %Y}")
        weekdays = cal.formatweekheader(2)
        if self.border_style != "ascii":
            weekdays = weekdays.replace(" ", " ")
        self.query_one("#mini-month-weekdays", Static).update(weekdays)
        weeks = calendar.Calendar(self.runtime.config.preferences.week_starts_on).monthdayscalendar(
            self.selected_date.year, self.selected_date.month
        )
        self._mini_month_days.clear()
        for week_index in range(6):
            days = weeks[week_index] if week_index < len(weeks) else (0,) * 7
            for weekday, day in enumerate(days):
                button = self.query_one(f"#mini-day-{week_index}-{weekday}", Button)
                # A standard Button adds one cell of implicit line padding on each side.
                # Day cells can be two columns wide in the narrow layout, so remove it.
                button.styles.line_pad = 0
                target = (
                    date(self.selected_date.year, self.selected_date.month, day) if day else None
                )
                button.label = str(day) if day else ""
                button.disabled = target is None
                button.set_class(target == self.selected_date, "mini-day-selected")
                if target is not None and button.id is not None:
                    self._mini_month_days[button.id] = target

    def _select_date(self: Any, value: date) -> None:
        """Apply a local date navigation without disturbing the resource list."""
        if value == self.selected_date:
            return
        self.selected_date = value
        self._render_chrome(refresh_resources=False)
        self._render_surface()

    def _present_date(self: Any) -> date:
        """Return today's date in the user's configured timezone."""
        return datetime.now(ZoneInfo(self.runtime.config.preferences.time_zone)).date()

    def format_date(self: Any, value: date) -> str:
        """Render a date using the selected user-facing display style."""
        if self.runtime.config.preferences.date_time_format == "iso":
            return value.isoformat()
        return f"{value.day} {value:%B %Y}"

    def format_time(self: Any, value: datetime) -> str:
        """Render an instant in the configured local timezone."""
        local = cast(datetime, self._local_time(value))
        style = self.runtime.config.preferences.date_time_format
        if style == "iso":
            return local.isoformat()
        if style == "friendly_24h":
            return f"{local.hour:02d}:{local.minute:02d}"
        hour = local.hour % 12 or 12
        suffix = "am" if local.hour < 12 else "pm"
        return f"{hour}:{local.minute:02d}{suffix}"

    def format_date_time(self: Any, value: date | datetime) -> str:
        """Render all-day values as dates and timed values with local clock time."""
        if not isinstance(value, datetime):
            return str(self.format_date(value))
        if self.runtime.config.preferences.date_time_format == "iso":
            return cast(datetime, self._local_time(value)).isoformat()
        local = cast(datetime, self._local_time(value))
        return f"{self.format_date(local.date())}, {self.format_time(local)}"

    def _local_time(self: Any, value: datetime) -> datetime:
        if value.tzinfo is None:
            value = value.replace(tzinfo=UTC)
        return value.astimezone(ZoneInfo(self.runtime.config.preferences.time_zone))

    def _task_rows(self: Any, tasks: tuple[Task, ...]) -> list[tuple[Task, str]]:
        children: dict[str | None, list[Task]] = {}
        for task in tasks:
            children.setdefault(task.parent_id, []).append(task)
        rows: list[tuple[Task, str]] = []
        seen: set[str] = set()

        def visit(task: Task, depth: int) -> None:
            if task.id in seen:
                return
            seen.add(task.id)
            rows.append((task, "  " * depth))
            for child in children.get(task.id, []):
                visit(child, depth + 1)

        for root in children.get(None, []):
            visit(root, 0)
        for task in tasks:
            visit(task, 0)
        return rows

    def _render_surface(self: Any) -> None:
        self._update_surface_title()
        content = self.query_one("#content", WorkspaceTable)
        calendar = self.query_one("#calendar-content", CalendarGrid)
        if self._calendar_grid_available():
            content.display = False
            calendar.display = True
            events = self._events_for_surface()
            scheduled_tasks = self._calendar_task_schedules()
            tasks = self._calendar_tasks_for_surface(scheduled_tasks)
            colors = {
                item_id: stored.color
                for item_id, _title, _selected in self.cache.calendars
                if (stored := self.runtime.application.calendar(self.account_id or "", item_id))
                if stored.color and stored.color.startswith("#")
            }
            calendar.set_calendar(
                cast(CalendarSurface, self.surface),
                self.selected_date,
                events,
                tasks,
                week_starts_on=self.runtime.config.preferences.week_starts_on,
                time_zone=self.runtime.config.preferences.time_zone,
                calendar_colors=colors,
                fallback_color=self.runtime.config.theme.colors.accent,
                selected=self.selected,
                scheduled_tasks=scheduled_tasks,
            )
            calendar.focus()
            # Keep the list projection synchronized underneath the grid. This
            # gives resize fallback an immediate readable surface and preserves
            # the existing programmatic WorkspaceTable integration.
            titles = {item_id: title for item_id, title, _selected in self.cache.calendars}
            scheduled_event_ids = {event.id for event in scheduled_tasks.values()}
            fallback_rows = [
                self._workspace_event_row(event, titles)
                for event in events
                if event.id not in scheduled_event_ids
            ]
            fallback_rows.extend(self._workspace_task_row(task, "") for task in tasks)
            content.replace_rows(
                tuple(fallback_rows)
                or (WorkspaceRow("empty", "empty", Text("No calendar items in this range")),),
                height=2 if self.density == "comfortable" else 1,
            )
            return
        calendar.display = False
        content.display = True
        rows: list[WorkspaceRow] = []
        if self.surface in {"Tasks", "Notes"}:
            tasks = self._surface_tasks()
            for task, indent in self._task_rows(tasks):
                rows.append(self._workspace_task_row(task, indent))
        else:
            events = self._events_for_surface()
            calendar_titles = {item_id: title for item_id, title, _selected in self.cache.calendars}
            for event in events:
                rows.append(self._workspace_event_row(event, calendar_titles))
            for task in self._calendar_tasks_for_surface(self._calendar_task_schedules()):
                rows.append(self._workspace_task_row(task, ""))
            if self.surface == "Month" and not events:
                rows.append(WorkspaceRow("empty", "month", Text("No events this month")))
        if not rows:
            rows.append(
                WorkspaceRow(
                    "empty",
                    "empty",
                    Text(f"No {self.surface.lower()} in the local cache"),
                )
            )
        content.replace_rows(tuple(rows), height=2 if self.density == "comfortable" else 1)
        selected_index = next(
            (index for index, row in enumerate(rows) if (row.kind, row.item_id) == self.selected),
            0,
        )
        content.move_cursor(row=selected_index, animate=False)
        selected = rows[selected_index]
        self.selected = (selected.kind, selected.item_id)
        content.select_workspace_row(
            selected_index, role_rich_style(self.runtime.config.theme.roles.selected_item)
        )

    def _calendar_grid_available(self: Any) -> bool:
        if self.surface not in {"Day", "Week", "Month"}:
            return False
        center_width = self.query_one("#center").size.width
        if not isinstance(center_width, int):
            return False
        return center_width >= CalendarGrid.minimum_width(cast(CalendarSurface, self.surface))

    def _calendar_task_schedules(self: Any) -> dict[str, Event]:
        """Return task IDs with an active linked calendar block in the local cache."""
        if self.account_id is None or (
            self.resource_filter and self.resource_filter[0] == "calendar"
        ):
            return {}
        events = {event.id: event for event in self.cache.events}
        return {
            link.task_id: event
            for link in self.runtime.application.list_task_event_links(self.account_id)
            if not link.orphaned and (event := events.get(link.event_id)) is not None
        }

    def _calendar_tasks_for_surface(
        self: Any, scheduled_tasks: dict[str, Event] | None = None
    ) -> tuple[Task, ...]:
        """Return due tasks for calendar surfaces without leaking calendar filters."""
        if self.resource_filter and self.resource_filter[0] == "calendar":
            return ()
        tasks = self.cache.tasks
        if self.resource_filter and self.resource_filter[0] == "task-list":
            tasks = tuple(task for task in tasks if task.list_id == self.resource_filter[1])
        scheduled_ids = set((scheduled_tasks or {}).keys())
        return tuple(task for task in tasks if task.due is not None or task.id in scheduled_ids)

    def _surface_tasks(self: Any) -> tuple[Task, ...]:
        """Return the task projection for the active task-oriented surface."""
        tasks = self.cache.tasks
        if self.resource_filter and self.resource_filter[0] == "task-list":
            tasks = tuple(task for task in tasks if task.list_id == self.resource_filter[1])
        projection = (
            self.runtime.application.notes_projection(self.account_id) if self.account_id else None
        )
        if self.surface == "Notes":
            return (
                tuple(task for task in tasks if task.due is None and task.parent_id is None)
                if projection is not None and projection.value != "disabled"
                else ()
            )
        if projection is not None and projection.value == "notes-only":
            return tuple(
                task for task in tasks if task.due is not None or task.parent_id is not None
            )
        return cast(tuple[Task, ...], tasks)

    @staticmethod
    def _task_cache_key(task: Task) -> tuple[str, bool, date, str]:
        """Match SQLite's workspace task ordering while keeping null due dates first."""
        return (task.status.value, task.due is not None, task.due or date.min, task.title)

    @staticmethod
    def _event_cache_key(event: Event) -> str:
        """Match the cached event query's ISO-encoded start ordering."""
        return event.start.value.isoformat()

    def _replace_cached_task(self: Any, task: Task) -> None:
        tasks = [item for item in self.cache.tasks if item.id != task.id]
        tasks.append(task)
        tasks.sort(key=self._task_cache_key)
        self.cache = replace(self.cache, tasks=tuple(tasks))

    def _replace_cached_event(self: Any, event: Event) -> None:
        events = [item for item in self.cache.events if item.id != event.id]
        events.append(event)
        events.sort(key=self._event_cache_key)
        self.cache = replace(self.cache, events=tuple(events))

    def _remove_cached_item(self: Any, kind: str, item_id: str) -> None:
        if kind == "task":
            self.cache = replace(
                self.cache, tasks=tuple(item for item in self.cache.tasks if item.id != item_id)
            )
        else:
            self.cache = replace(
                self.cache, events=tuple(item for item in self.cache.events if item.id != item_id)
            )

    def _refresh_mutation_chrome(self: Any, *, event_changed: bool = False) -> None:
        """Refresh only small chrome values after a local, singleton mutation."""
        if self.account_id is None:
            return
        cache_updates: dict[str, object] = {
            "pending": self.runtime.application.pending_count(self.account_id)
        }
        if event_changed:
            cache_updates["instance_ranges"] = self.runtime.application.instance_ranges(
                self.account_id
            )
        self.cache = replace(self.cache, **cache_updates)
        self._instance_badge_cache.clear()
        self._render_chrome(refresh_resources=False)

    def _empty_workspace_row(self: Any) -> WorkspaceRow:
        return WorkspaceRow("empty", "empty", Text(f"No {self.surface.lower()} in the local cache"))

    def _ensure_workspace_row_presence(self: Any) -> None:
        content = self.query_one("#content", WorkspaceTable)
        if content.row_count == 0:
            content.insert_workspace_row(0, self._empty_workspace_row())

    def _remove_empty_workspace_row(self: Any) -> None:
        content = self.query_one("#content", WorkspaceTable)
        if content.row_count == 1 and (row := content.row_at(0)) and row.kind == "empty":
            content.remove_workspace_row(0)

    def _select_first_workspace_row(self: Any) -> None:
        content = self.query_one("#content", WorkspaceTable)
        if (row := content.row_at(0)) is None:
            self.selected = None
            return
        self.selected = (row.kind, row.item_id)
        content.move_cursor(row=0, animate=False)
        content.select_workspace_row(
            0, role_rich_style(self.runtime.config.theme.roles.selected_item)
        )

    def _reconcile_task_workspace_row(self: Any, task: Task) -> None:
        """Update, insert, or reposition one task without rebuilding the surface."""
        content = self.query_one("#content", WorkspaceTable)
        current_index = content.index_of("task", task.id)
        target: tuple[int, str] | None = None
        if self.surface in {"Tasks", "Notes"}:
            for index, (candidate, indent) in enumerate(self._task_rows(self._surface_tasks())):
                if candidate.id == task.id:
                    target = (index, indent)
                    break
        if target is None:
            if current_index is not None:
                content.remove_workspace_row(current_index)
            if self.selected == ("task", task.id):
                self._select_first_workspace_row()
            self._ensure_workspace_row_presence()
            self._update_surface_title()
            return
        target_index, indent = target
        row = self._workspace_task_row(task, indent)
        self._remove_empty_workspace_row()
        if current_index is None:
            content.insert_workspace_row(target_index, row)
        elif current_index == target_index:
            content.update_workspace_row(current_index, row)
        else:
            content.remove_workspace_row(current_index)
            content.insert_workspace_row(target_index, row)
        if self.selected == ("task", task.id):
            content.move_cursor(row=target_index, animate=False)
            content.select_workspace_row(
                target_index, role_rich_style(self.runtime.config.theme.roles.selected_item)
            )
        elif self.selected is None or content.index_of(*self.selected) is None:
            self._select_first_workspace_row()
        self._update_surface_title()

    def _reconcile_event_workspace_row(self: Any, event: Event) -> None:
        """Update, insert, or reposition one event without rebuilding the surface."""
        content = self.query_one("#content", WorkspaceTable)
        current_index = content.index_of("event", event.id)
        target_index: int | None = None
        if self.surface not in {"Tasks", "Notes"}:
            target_index = next(
                (
                    index
                    for index, candidate in enumerate(self._events_for_surface())
                    if candidate.id == event.id
                ),
                None,
            )
        if target_index is None:
            if current_index is not None:
                content.remove_workspace_row(current_index)
            if self.selected == ("event", event.id):
                self._select_first_workspace_row()
            self._ensure_workspace_row_presence()
            self._update_surface_title()
            return
        calendar_titles = {item_id: title for item_id, title, _selected in self.cache.calendars}
        row = self._workspace_event_row(event, calendar_titles)
        self._remove_empty_workspace_row()
        if current_index is None:
            content.insert_workspace_row(target_index, row)
        elif current_index == target_index:
            content.update_workspace_row(current_index, row)
        else:
            content.remove_workspace_row(current_index)
            content.insert_workspace_row(target_index, row)
        if self.selected == ("event", event.id):
            content.move_cursor(row=target_index, animate=False)
            content.select_workspace_row(
                target_index, role_rich_style(self.runtime.config.theme.roles.selected_item)
            )
        elif self.selected is None or content.index_of(*self.selected) is None:
            self._select_first_workspace_row()
        self._update_surface_title()

    def apply_workspace_task_mutation(self: Any, task: Task) -> None:
        """Apply one locally-written task to cache and the active virtual surface."""
        self._replace_cached_task(task)
        if self._calendar_grid_available():
            self._render_surface()
            self._refresh_mutation_chrome()
            return
        self._reconcile_task_workspace_row(task)
        self._refresh_mutation_chrome()

    def apply_workspace_event_mutation(self: Any, event: Event) -> None:
        """Apply one locally-written event to cache and the active virtual surface."""
        self._replace_cached_event(event)
        if self._calendar_grid_available():
            self._render_surface()
            self._refresh_mutation_chrome(event_changed=True)
            return
        self._reconcile_event_workspace_row(event)
        self._refresh_mutation_chrome(event_changed=True)

    def remove_workspace_item(self: Any, kind: str, item_id: str) -> None:
        """Remove one local item from cache and the active virtual surface."""
        self._remove_cached_item(kind, item_id)
        if self._calendar_grid_available():
            if self.selected == (kind, item_id):
                self.selected = None
            self._render_surface()
            self._refresh_mutation_chrome(event_changed=kind == "event")
            return
        content = self.query_one("#content", WorkspaceTable)
        if (index := content.index_of(kind, item_id)) is not None:
            content.remove_workspace_row(index)
        if self.selected == (kind, item_id):
            self.selected = None
            self._select_first_workspace_row()
        self._ensure_workspace_row_presence()
        self._update_surface_title()
        self._refresh_mutation_chrome(event_changed=kind == "event")

    def _update_surface_title(self: Any) -> None:
        """Refresh the small title label without disturbing the virtual workspace list."""
        selection = self._selection_summary()
        self.query_one("#surface-title", Static).update(
            f"{self.surface}  ·  {self.selected_date:%A, %d %B %Y}{selection}"
        )

    def _workspace_task_row(self: Any, task: Task, indent: str) -> WorkspaceRow:
        marked = "*" if task.id in self.marked else " "
        status = "✓" if task.status is TaskStatus.COMPLETED else "·"
        due = (
            f"{self.format_date(task.due)}  "
            if task.due and self.runtime.config.tui.task_show_due
            else ""
        )
        note_lines = (task.notes or "").splitlines()
        notes = (
            f" — {note_lines[0]}"
            if self.surface == "Notes" and note_lines and self.runtime.config.tui.notes_show_preview
            else ""
        )
        dot_style = self._workspace_dot_style(self.runtime.config.theme.colors.accent)
        label = Text(f"{marked} {indent}{status} {due}")
        dot_start = len(label)
        label.append("●", style=dot_style)
        dot_end = len(label)
        label.append("  ")
        title_start = len(label)
        label.append(task.title)
        title_end = len(label)
        label.append(notes)
        if task.status is TaskStatus.COMPLETED:
            style = role_rich_style(self.runtime.config.theme.roles.completed_item)
            label.stylize(style)
            label.stylize(style, title_start, title_end)
            label.stylize(dot_style, dot_start, dot_end)
        return WorkspaceRow("task", task.id, linkify_urls(label))

    @staticmethod
    def _workspace_dot_style(color: str) -> Style:
        """Translate Textual's terminal-default tokens for Rich text rendering."""
        return Style(color="default" if color in {"ansi_default", "transparent"} else color)

    def _workspace_event_row(
        self: Any, event: Event, calendar_titles: dict[str, str]
    ) -> WorkspaceRow:
        when = self.format_date_time(event.start.value)
        marked = "*" if event.id in self.marked_events else " "
        extras: list[str] = []
        if self.runtime.config.tui.agenda_show_calendar and (
            calendar := calendar_titles.get(event.calendar_id)
        ):
            extras.append(calendar)
        if self.runtime.config.tui.agenda_show_location and event.location:
            extras.append(event.location)
        suffix = f"  · {' · '.join(extras)}" if extras else ""
        color = event.color_id
        if color is None:
            calendar = self.runtime.application.calendar(event.account_id, event.calendar_id)
            color = calendar.color if calendar is not None else None
        dot_color = (GOOGLE_EVENT_COLORS.get(color) if color is not None else None) or (
            color if color is not None and color.startswith("#") else None
        )
        label = Text(f"{marked} {when}  ")
        label.append(
            "●",
            style=self._workspace_dot_style(dot_color or self.runtime.config.theme.colors.accent),
        )
        label.append(f"  {event.summary}{suffix}")
        return WorkspaceRow("event", event.id, linkify_urls(label))

    def _refresh_marked_workspace_row(self: Any) -> None:
        """Patch only the marker and title summary after a local mark toggle."""
        self._update_surface_title()
        if self.selected is None:
            return
        content = self.query_one("#content", WorkspaceTable)
        for index, row in enumerate(content.workspace_rows):
            if (row.kind, row.item_id) != self.selected:
                continue
            marked = (
                row.item_id in self.marked
                if row.kind == "task"
                else row.item_id in self.marked_events
            )
            marker = "*" if marked else " "
            label = Text(f"{marker} ")
            label.append_text(row.label[2:])
            content.update_workspace_row(index, WorkspaceRow(row.kind, row.item_id, label))
            return

    def _selection_summary(self: Any) -> str:
        parts: list[str] = []
        if self.marked:
            parts.append(f"{len(self.marked)} task(s)")
        if self.marked_events:
            parts.append(f"{len(self.marked_events)} event(s)")
        return f"  ·  selected: {', '.join(parts)}" if parts else ""

    def _update_content_selection(self: Any) -> None:
        """Move the virtual-table cursor to the selected workspace identity."""
        content = self.query_one("#content", WorkspaceTable)
        if self.selected is None:
            return
        for index in range(content.row_count):
            row = content.row_at(index)
            if row is not None and (row.kind, row.item_id) == self.selected:
                content.move_cursor(row=index, animate=False)
                content.select_workspace_row(
                    index, role_rich_style(self.runtime.config.theme.roles.selected_item)
                )
                return

    def _update_resource_selection(self: Any) -> None:
        """Keep the active resource visible after focus moves to another pane."""
        resources = self.query_one("#resources", ListView)
        for row in resources.query(EntityRow):
            selected = (
                row.kind == "resource-all"
                if self.resource_filter is None
                else (row.kind, row.item_id) == self.resource_filter
            )
            row.set_class(selected, "hcb-selected")
            self._apply_selected_item_role(row, selected)

    def _apply_selected_item_role(self: Any, row: EntityRow, selected: bool) -> None:
        """Apply an optional semantic selection override without replacing base TCSS."""
        for rule in ("color", "background", "text_style"):
            row.styles.clear_rule(rule)
        if not selected:
            return
        role = self.runtime.config.theme.roles.selected_item
        if role.color not in {None, "ansi_default", "transparent"}:
            row.styles.color = role.color
        if role.background not in {None, "ansi_default", "transparent"}:
            row.styles.background = role.background
        row.styles.text_style = role.text_style

    def _event_surface_range(self: Any) -> tuple[date, date]:
        day = self.selected_date
        if self.surface in {"Day", "Week", "Month"}:
            visible = calendar_range(
                cast(CalendarSurface, self.surface),
                day,
                self.runtime.config.preferences.week_starts_on,
            )
            return visible.start, visible.end
        if self.surface == "Day":
            return day, day + timedelta(days=1)
        if self.surface == "Week":
            offset = (day.weekday() - self.runtime.config.preferences.week_starts_on) % 7
            start = day - timedelta(days=offset)
            return start, start + timedelta(days=7)
        if self.surface == "Month":
            start = day.replace(day=1)
            return start, (start.replace(day=28) + timedelta(days=4)).replace(day=1)
        return day, day + timedelta(days=self.runtime.config.tui.agenda_days)

    def _instance_cache_badge(self: Any) -> str | None:
        """Summarize the local-only occurrence-cache state for the visible range."""
        if self.account_id is None or self.surface in {"Tasks", "Notes"}:
            return None
        start, end = self._event_surface_range()
        calendar_ids = (
            (self.resource_filter[1],)
            if self.resource_filter and self.resource_filter[0] == "calendar"
            else tuple(item_id for item_id, _title, selected in self.cache.calendars if selected)
        )
        cache_key = (
            self.account_id,
            self.surface,
            start,
            end,
            self.resource_filter,
            calendar_ids,
            self.runtime.config.preferences.date_time_format,
            self.runtime.config.preferences.time_zone,
        )
        if cache_key in self._instance_badge_cache:
            return cast(str | None, self._instance_badge_cache[cache_key])
        if not calendar_ids:
            self._instance_badge_cache[cache_key] = "instances: missing"
            return "instances: missing"
        statuses = [
            self.runtime.application.instance_cache_status(self.account_id, item_id, start, end)
            for item_id in calendar_ids
        ]
        states = {item["state"] for item in statuses}
        state = (
            "fresh"
            if states == {"fresh"}
            else "partial"
            if "fresh" in states
            else ("stale" if "stale" in states else "missing")
        )
        ranges = [item for status in statuses for item in status["ranges"]]
        refreshed = max(
            (str(item["refreshed_at"]) for item in ranges if item.get("refreshed_at")),
            default=None,
        )
        stale_reason = next(
            (str(item["stale_reason"]) for item in ranges if item.get("state") == "stale"),
            None,
        )
        details = f"{self.format_date(start)}–{self.format_date(end)}"
        if refreshed:
            details += f", refreshed {self.format_date_time(datetime.fromisoformat(refreshed))}"
        if stale_reason:
            details += f", {stale_reason.replace('-', ' ')}"
        result = f"instances: {state} ({details})"
        self._instance_badge_cache[cache_key] = result
        return result

    def _events_for_surface(self: Any) -> tuple[Event, ...]:
        start, end = self._event_surface_range()
        events = tuple(
            event
            for event in self.cache.events
            if self._event_overlaps_range(event, start, end)
            if (
                not self.resource_filter
                or self.resource_filter[0] != "calendar"
                or event.calendar_id == self.resource_filter[1]
            )
        )
        expanded_series = {
            event.canonical_id for event in events if event.derived and event.canonical_id
        }
        return tuple(
            event
            for event in events
            if event.derived or not event.recurrence or event.remote_id not in expanded_series
        )

    def _event_overlaps_range(self: Any, event: Event, start: date, end: date) -> bool:
        """Use instant boundaries for timed events and exclusive ones for all-day dates."""
        event_start = event.start.value
        event_end = event.end.value
        if not isinstance(event_start, datetime) and not isinstance(event_end, datetime):
            return event_end > start and event_start < end
        if not isinstance(event_start, datetime) or not isinstance(event_end, datetime):
            return False
        zone = ZoneInfo(self.runtime.config.preferences.time_zone)
        range_start = datetime.combine(start, datetime.min.time(), tzinfo=zone)
        range_end = datetime.combine(end, datetime.min.time(), tzinfo=zone)
        return bool(
            self._local_time(event_end) > range_start and self._local_time(event_start) < range_end
        )

    def on_button_pressed(self: Any, event: Button.Pressed) -> None:
        button_id = event.button.id
        if button_id in self._mini_month_days:
            self._select_date(self._mini_month_days[button_id])
        elif button_id and button_id.startswith("surface-"):
            self.action_surface(button_id.removeprefix("surface-").title())

    def on_workspace_table_row_highlighted(self: Any, event: WorkspaceTable.RowHighlighted) -> None:
        if event.table.id != "content":
            return
        self.selected = (event.row.kind, event.row.item_id)
        event.table.select_workspace_row(
            event.index, role_rich_style(self.runtime.config.theme.roles.selected_item)
        )

    def on_list_view_selected(self: Any, event: ListView.Selected) -> None:
        if not isinstance(event.item, EntityRow):
            return
        row = event.item
        if event.list_view.id == "content":
            self.selected = (row.kind, row.item_id)
            self._update_content_selection()
            if row.kind in {"task", "event"}:
                self.action_view()
            return
        if event.list_view.id != "resources":
            return
        self.resource_filter = None if row.kind == "resource-all" else (row.kind, row.item_id)
        self._render_chrome(refresh_resources=False)
        self._render_surface()

    def on_workspace_table_row_selected(self: Any, event: WorkspaceTable.RowSelected) -> None:
        if event.table.id != "content":
            return
        self.selected = (event.row.kind, event.row.item_id)
        if event.row.kind in {"task", "event"}:
            self.action_view()

    def on_workspace_table_row_marked(self: Any, event: WorkspaceTable.RowMarked) -> None:
        self.selected = (event.row.kind, event.row.item_id)
        self.action_mark()

    def on_calendar_grid_item_selected(self: Any, event: CalendarGrid.ItemSelected) -> None:
        if event.grid.id != "calendar-content":
            return
        self.selected = (event.item.kind, event.item.item_id)
        event.grid.select_item(self.selected)

    def on_calendar_grid_item_activated(self: Any, event: CalendarGrid.ItemActivated) -> None:
        if event.grid.id != "calendar-content":
            return
        self.selected = (event.item.kind, event.item.item_id)
        event.grid.select_item(self.selected)
        self.action_view()

    def on_calendar_grid_slot_selected(self: Any, event: CalendarGrid.SlotSelected) -> None:
        if event.grid.id != "calendar-content":
            return
        self.action_create_calendar_slot(event.day, event.minute, all_day=event.all_day)

    def on_calendar_grid_slot_range_selected(
        self: Any, event: CalendarGrid.SlotRangeSelected
    ) -> None:
        if event.grid.id != "calendar-content":
            return
        self.action_create_calendar_range(
            event.start_day,
            event.start_minute,
            event.end_day,
            event.end_minute,
        )

    def on_calendar_grid_date_requested(self: Any, event: CalendarGrid.DateRequested) -> None:
        if event.grid.id != "calendar-content":
            return
        self.selected_date = event.day
        self._render_chrome(refresh_resources=False)
        self._render_surface()

    def on_calendar_grid_overflow_requested(
        self: Any, event: CalendarGrid.OverflowRequested
    ) -> None:
        if event.grid.id != "calendar-content":
            return
        self.push_screen(
            CalendarOverflowScreen(event.day, event.items), self._calendar_overflow_result
        )

    def _calendar_overflow_result(self: Any, result: CalendarOverflowResult | None) -> None:
        if result is None:
            return
        if result.action == "day":
            self.selected_date = result.day
            self.action_surface("Day")
            return
        if result.item is None:
            return
        self.selected = result.item
        self.action_view()

    def on_calendar_grid_item_changed(self: Any, event: CalendarGrid.ItemChanged) -> None:
        if event.grid.id != "calendar-content":
            return
        self.selected = (event.item.kind, event.item.item_id)
        self.apply_calendar_item_change(
            event.item,
            day=event.day,
            minute=event.minute,
            operation=event.operation,
        )

    def _selected_task(self: Any) -> Task | None:
        if not self.selected or self.selected[0] != "task":
            return None
        return next((item for item in self.cache.tasks if item.id == self.selected[1]), None)

    def _selected_event(self: Any) -> Event | None:
        if not self.selected or self.selected[0] != "event":
            return None
        return next((item for item in self.cache.events if item.id == self.selected[1]), None)

    def _selected_drive(self: Any) -> DriveFile | None:
        if not self.selected or self.selected[0] != "drive" or self.account_id is None:
            return None
        return cast(
            DriveFile | None,
            self.runtime.application.drive_file(self.account_id, self.selected[1]),
        )

    def search_local(self: Any, query: str) -> tuple[SearchResult, ...]:
        if self.account_id is None:
            return ()
        return cast(
            tuple[SearchResult, ...], self.runtime.application.search(self.account_id, query)
        )
