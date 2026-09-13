"""Scriptable command-line interface for HCB."""

from __future__ import annotations

import json
import os
import plistlib
import shlex
import subprocess
import sys
from collections.abc import Callable, Sequence
from dataclasses import asdict, dataclass, replace
from datetime import date, datetime, timedelta
from pathlib import Path
from typing import Any, cast
from uuid import uuid4

import typer
from typer import _click as click

from .application import BatchActionPreview, BatchMovePreview, SearchResult
from .config import Config, ConfigError, load, loads, profile_path, save
from .config import schema as config_schema
from .errors import ConfigurationError, ConflictError, ExitCode, HcbError, OfflineError
from .import_export import (
    ImportedEvent,
    ImportedRecord,
    ImportedTask,
    export_csv,
    export_ics,
    export_json,
)
from .json_contract import (
    JSON_COMMANDS,
    JSON_SCHEMA_VERSION,
)
from .json_contract import (
    bundle as json_schema_bundle,
)
from .json_contract import (
    command_name as json_command_name,
)
from .json_contract import (
    command_schema as json_command_schema,
)
from .json_contract import (
    error as json_error,
)
from .json_contract import (
    success as json_success,
)
from .models import (
    Account,
    ConflictStatus,
    DateTimeKind,
    Event,
    EventDateTime,
    ReminderOverride,
    Task,
)
from .notifications import default_notifier
from .output import to_primitive
from .runtime import Runtime
from .scheduler import DaemonState, ReminderScheduler, run_loop
from .themes import apply_preset, load_custom_theme, preset, presets
from .uninstall import (
    RemovalProgress,
    UninstallPlan,
    build_plan,
    execute_plan,
    package_removal,
)


class HcbGroup(typer.core.TyperGroup):
    """Translate expected domain failures into stable, traceback-free exits."""

    def invoke(self, ctx: click.Context) -> Any:
        try:
            return super().invoke(ctx)
        except HcbError as exc:
            self._emit_error(
                ctx, exc.message, exc.hint, self._error_code(exc.exit_code), int(exc.exit_code)
            )
            raise click.exceptions.Exit(int(exc.exit_code)) from None
        except (ConfigError, ValueError) as exc:
            self._emit_error(ctx, str(exc), None, "invalid_request", int(ExitCode.USAGE))
            raise click.exceptions.Exit(int(ExitCode.USAGE)) from None

    @staticmethod
    def _emit_error(
        ctx: click.Context, message: str, hint: str | None, code: str, exit_code: int
    ) -> None:
        state = ctx.find_root().obj
        if isinstance(state, State) and state.json:
            click.echo(
                json.dumps(
                    json_error(
                        json_command_name(ctx),
                        code=code,
                        message=message,
                        hint=hint,
                        exit_code=exit_code,
                    ),
                    ensure_ascii=False,
                    sort_keys=True,
                )
            )
            return
        click.echo(f"Error: {message}", err=True)
        if hint:
            click.echo(f"Hint: {hint}", err=True)

    @staticmethod
    def _error_code(exit_code: ExitCode) -> str:
        return {
            ExitCode.NOT_FOUND: "not_found",
            ExitCode.CONFLICT: "conflict",
            ExitCode.AUTH_REQUIRED: "authentication_required",
            ExitCode.OFFLINE: "offline",
            ExitCode.REMOTE_FAILURE: "remote_failure",
            ExitCode.STORAGE_FAILURE: "storage_failure",
            ExitCode.CONFIGURATION: "configuration_error",
        }.get(exit_code, "hcb_error")


class ThemesGroup(HcbGroup):
    """Accept a preset rank as a concise alias for ``themes apply``."""

    def resolve_command(
        self, ctx: click.Context, args: list[str]
    ) -> tuple[str | None, click.Command | None, list[str]]:
        if args and args[0].isdigit():
            rank = int(args[0])
            selected = next((item for item in presets() if item.rank == rank), None)
            if selected is None:
                raise ValueError(f"unknown theme rank {rank}; run `hcb themes list`")
            args = ["apply", selected.name, *args[1:]]
        return super().resolve_command(ctx, args)


CONTEXT = {"help_option_names": ["-h", "--help"]}
app = typer.Typer(
    cls=HcbGroup,
    context_settings=CONTEXT,
    no_args_is_help=True,
    help="Local-first Google Tasks and Calendar client.",
)
tasks_app = typer.Typer(cls=HcbGroup, context_settings=CONTEXT, help="Manage tasks.")
lists_app = typer.Typer(cls=HcbGroup, context_settings=CONTEXT, help="Manage task lists.")
notes_app = typer.Typer(cls=HcbGroup, context_settings=CONTEXT, help="Manage task notes.")
events_app = typer.Typer(cls=HcbGroup, context_settings=CONTEXT, help="Manage events.")
calendars_app = typer.Typer(cls=HcbGroup, context_settings=CONTEXT, help="Manage calendars.")
saved_app = typer.Typer(cls=HcbGroup, context_settings=CONTEXT, help="Manage saved searches.")
conflicts_app = typer.Typer(cls=HcbGroup, context_settings=CONTEXT, help="Resolve sync conflicts.")
import_app = typer.Typer(cls=HcbGroup, context_settings=CONTEXT, help="Preview or apply imports.")
auth_app = typer.Typer(cls=HcbGroup, context_settings=CONTEXT, help="Manage authentication.")
config_app = typer.Typer(cls=HcbGroup, context_settings=CONTEXT, help="Manage configuration.")
config_profiles_app = typer.Typer(
    cls=HcbGroup, context_settings=CONTEXT, help="Manage configuration profiles."
)
themes_app = typer.Typer(cls=ThemesGroup, context_settings=CONTEXT, help="Manage visual themes.")
daemon_app = typer.Typer(
    cls=HcbGroup, context_settings=CONTEXT, help="Inspect sync daemon support."
)
drive_app = typer.Typer(cls=HcbGroup, context_settings=CONTEXT, help="Search Drive metadata.")
schema_app = typer.Typer(cls=HcbGroup, context_settings=CONTEXT, help="Inspect JSON contracts.")

app.add_typer(tasks_app, name="tasks")
app.add_typer(lists_app, name="task-lists")
app.add_typer(notes_app, name="notes")
app.add_typer(events_app, name="events")
app.add_typer(calendars_app, name="calendars")
app.add_typer(saved_app, name="saved-searches")
app.add_typer(conflicts_app, name="conflicts")
app.add_typer(import_app, name="import")
app.add_typer(auth_app, name="auth")
app.add_typer(config_app, name="config")
config_app.add_typer(config_profiles_app, name="profiles")
app.add_typer(themes_app, name="themes")
app.add_typer(daemon_app, name="daemon")
app.add_typer(drive_app, name="drive")
app.add_typer(schema_app, name="schema")


@dataclass(slots=True)
class State:
    runtime: Runtime
    account: str | None
    json: bool
    tsv: bool


_runtime_factory: Callable[[], Runtime] = Runtime


def _print_json_schema_version(value: bool) -> None:
    if value:
        typer.echo(JSON_SCHEMA_VERSION)
        raise typer.Exit()


@app.callback()
def root(
    ctx: typer.Context,
    account: str | None = typer.Option(
        None, "--account", "-a", envvar="HCB_ACCOUNT", help="Account id."
    ),
    json_output: bool = typer.Option(False, "--json", help="Emit JSON."),
    tsv: bool = typer.Option(False, "--tsv", help="Emit tab-separated records."),
    no_color: bool = typer.Option(False, "--no-color", help="Disable terminal color."),
    env_file: Path | None = typer.Option(  # noqa: B008
        None,
        "--env-file",
        envvar="HCB_ENV_FILE",
        help="Per-account Google credential .env file.",
    ),
    json_schema_version: bool = typer.Option(
        False,
        "--json-schema-version",
        help="Print the machine-readable output schema version and exit.",
        is_eager=True,
        callback=_print_json_schema_version,
    ),
) -> None:
    del json_schema_version
    if json_output and tsv:
        raise typer.BadParameter("--json and --tsv are mutually exclusive")
    if no_color or os.environ.get("NO_COLOR") is not None or os.environ.get("TERM") == "dumb":
        ctx.color = False
    runtime = _runtime_factory()
    if env_file is not None:
        runtime.credential_file_override = env_file
    ctx.obj = State(runtime, account, json_output, tsv)
    ctx.call_on_close(ctx.obj.runtime.close)


def _state(ctx: typer.Context) -> State:
    return cast(State, ctx.find_root().obj)


def _open_in_external_editor(
    runtime: Runtime, path: Path, *, fallback_editor: str | None = None
) -> None:
    """Open one user-requested text file with HCB's configured editor."""

    configured = runtime.environ.get("HCB_EDITOR")
    if not configured:
        try:
            configured = runtime.config.preferences.editor
        except ConfigurationError:
            if fallback_editor is None:
                raise
            configured = fallback_editor
    command = shlex.split(configured)
    if not command:
        raise ValueError("external editor command is empty")
    try:
        result = subprocess.run([*command, str(path)], check=False)
    except OSError as exc:
        raise ValueError(f"external editor failed: {exc}") from exc
    if result.returncode != 0:
        raise ValueError(f"external editor exited with status {result.returncode}")


def _account(ctx: typer.Context) -> str:
    state = _state(ctx)
    return state.runtime.account_id(state.account)


def _rows(value: Any) -> list[dict[str, Any]]:
    primitive = to_primitive(value)
    if isinstance(primitive, list):
        return [item if isinstance(item, dict) else {"value": item} for item in primitive]
    return [primitive if isinstance(primitive, dict) else {"value": primitive}]


def _emit(
    ctx: typer.Context,
    value: Any,
    *,
    fields: Sequence[str] = (),
    human: Callable[[Any], str] | None = None,
) -> None:
    state = _state(ctx)
    primitive = to_primitive(value)
    if state.json:
        typer.echo(
            json.dumps(
                json_success(json_command_name(ctx), primitive),
                ensure_ascii=False,
                indent=2,
                sort_keys=True,
            )
        )
        return
    if state.tsv:
        rows = _rows(value)
        selected = list(fields) or (list(rows[0]) if rows else [])
        typer.echo("\t".join(selected))
        for row in rows:
            typer.echo(
                "\t".join(
                    str(to_primitive(row.get(field)) if row.get(field) is not None else "")
                    .replace("\t", " ")
                    .replace("\n", " ")
                    for field in selected
                )
            )
        return
    values = value if isinstance(value, (list, tuple)) else (value,)
    for item in values:
        if human:
            typer.echo(human(item))
        elif isinstance(item, str):
            typer.echo(item)
        else:
            row = to_primitive(item)
            typer.echo("  ".join(f"{key}={val}" for key, val in row.items() if val is not None))


def _confirm(yes: bool, message: str) -> None:
    if yes:
        return
    if not sys.stdin.isatty():
        raise ValueError("destructive operation requires --yes when stdin is not a TTY")
    if not typer.confirm(message):
        raise typer.Abort()


def _confirm_uninstall(yes: bool) -> None:
    """Require a stronger acknowledgement before deleting every local HCB record."""

    if yes:
        return
    if not sys.stdin.isatty():
        raise ValueError("uninstall requires --yes when stdin is not a TTY")
    typer.echo("Type UNINSTALL to permanently remove HCB: ", nl=False)
    acknowledged = input()
    if acknowledged != "UNINSTALL":
        raise ValueError("uninstall cancelled; no HCB data was removed")


def _show_move_preflight(ctx: typer.Context, preview: BatchMovePreview) -> bool:
    """Write the complete local move plan before asking for confirmation."""
    state = _state(ctx)
    account = _account(ctx)
    if preview.entity_type == "task":
        tasks = tuple(item for item in preview.items if isinstance(item, Task))
        destination_list = state.runtime.storage.get_task_list(account, preview.destination_id)
        destination_title = destination_list.title if destination_list else preview.destination_id
        lines = [
            f"Preflight: move {len(tasks)} task(s) to {destination_title!r} as top-level tasks:"
        ]
        for task in tasks:
            source_list = state.runtime.storage.get_task_list(account, task.list_id)
            source_title = source_list.title if source_list else task.list_id
            lines.append(f"- {task.id} · {task.title}: {source_title!r} → {destination_title!r}")
        cross_destination = any(task.list_id != preview.destination_id for task in tasks)
    else:
        events = tuple(item for item in preview.items if isinstance(item, Event))
        destination_calendar = state.runtime.storage.get_calendar(account, preview.destination_id)
        destination_title = (
            destination_calendar.summary if destination_calendar else preview.destination_id
        )
        lines = [f"Preflight: move {len(events)} event(s) to {destination_title!r}:"]
        for event in events:
            source_calendar = state.runtime.storage.get_calendar(account, event.calendar_id)
            source_title = source_calendar.summary if source_calendar else event.calendar_id
            lines.append(
                f"- {event.id} · {event.summary}: {source_title!r} → {destination_title!r}"
            )
        cross_destination = any(event.calendar_id != preview.destination_id for event in events)
    click.echo("\n".join(lines), err=True)
    return cross_destination


def _show_batch_preflight(ctx: typer.Context, preview: BatchActionPreview) -> None:
    """Show each local change before invoking a batch operation."""
    if preview.entity_type == "task":
        tasks = tuple(item for item in preview.items if isinstance(item, Task))
        if preview.action == "delete":
            lines = [f"Preflight: delete {len(tasks)} task(s):"]
            lines.extend(f"- {task.id} · {task.title}: active → deleted" for task in tasks)
        else:
            target = "completed" if preview.action == "complete" else "needs action"
            lines = [f"Preflight: {preview.action} {len(tasks)} task(s):"]
            lines.extend(
                f"- {task.id} · {task.title}: {task.status.value} → {target}" for task in tasks
            )
    else:
        events = tuple(item for item in preview.items if isinstance(item, Event))
        if preview.action == "delete":
            lines = [f"Preflight: delete {len(events)} event(s):"]
            lines.extend(f"- {event.id} · {event.summary}: active → deleted" for event in events)
        else:
            target = preview.response_status or "needsAction"
            lines = [f"Preflight: RSVP {target} for {len(events)} event(s):"]
            lines.extend(
                f"- {event.id} · {event.summary}: "
                f"{event.attendee_response or 'needsAction'} → {target}"
                for event in events
            )
    click.echo("\n".join(lines), err=True)


def _event_point(raw: str, all_day: bool, zone: str | None) -> EventDateTime:
    if all_day:
        return EventDateTime(DateTimeKind.DATE, date.fromisoformat(raw))
    value = datetime.fromisoformat(raw.replace("Z", "+00:00"))
    return EventDateTime(DateTimeKind.DATETIME, value, zone)


def _recurrence_rules(values: list[str] | None) -> tuple[str, ...]:
    """Accept Google recurrence lines or concise RRULE values on the command line."""
    result: list[str] = []
    for value in values or ():
        rule = value.strip()
        if not rule:
            raise ValueError("recurrence rules must not be empty")
        if not rule.startswith(("RRULE:", "EXDATE;", "RDATE;")):
            rule = f"RRULE:{rule}"
        result.append(rule)
    return tuple(result)


def _range_datetime(value: str, *, end: bool = False) -> datetime:
    if len(value) == 10:
        parsed = datetime.combine(date.fromisoformat(value), datetime.min.time())
        return parsed + timedelta(days=1) if end else parsed
    return datetime.fromisoformat(value.replace("Z", "+00:00"))


def _task_record(task: Task, lists: dict[str, str]) -> ImportedTask:
    return ImportedTask(
        title=task.title,
        list=lists.get(task.list_id, task.list_id),
        due=task.due.isoformat() if task.due else None,
        notes=task.notes,
        priority=task.priority.value,
    )


def _event_record(event: Event, calendars: dict[str, str]) -> ImportedEvent:
    return ImportedEvent(
        title=event.summary,
        calendar=calendars.get(event.calendar_id, event.calendar_id),
        start=event.start.value.isoformat(),
        end=event.end.value.isoformat(),
        all_day=event.start.kind is DateTimeKind.DATE,
        time_zone=event.start.time_zone,
        description=event.description,
        location=event.location,
        recurrence=event.recurrence,
    )


# Tasks
@tasks_app.command("list")
def tasks_list(
    ctx: typer.Context,
    task_list: str | None = typer.Option(None, "--list"),
    completed: bool = typer.Option(False, "--completed"),
) -> None:
    application = _state(ctx).runtime.application
    items = list(application.task_listing(_account(ctx)))
    if task_list:
        items = [item for item in items if item.list_id == task_list]
    if not completed:
        items = [item for item in items if item.status.value != "completed"]
    _emit(
        ctx,
        items,
        fields=("id", "list_id", "title", "status", "due", "priority"),
        human=lambda item: f"{item.id}\t{item.title}",
    )


@tasks_app.command("create")
def tasks_create(
    ctx: typer.Context,
    title: str,
    task_list: str = typer.Option(..., "--list"),
    notes: str | None = typer.Option(None),
    due: str | None = typer.Option(None),
    priority: str = typer.Option("none"),
) -> None:
    item = _state(ctx).runtime.application.create_task(
        _account(ctx),
        task_list,
        title,
        notes=notes,
        due=date.fromisoformat(due) if due else None,
        priority=priority,
    )
    _emit(ctx, item, human=lambda value: f"Created task {value.id}: {value.title}")


@tasks_app.command("edit")
def tasks_edit(
    ctx: typer.Context,
    task_id: str,
    title: str | None = typer.Option(None),
    notes: str | None = typer.Option(None),
    due: str | None = typer.Option(None),
    clear_due: bool = typer.Option(False, "--clear-due"),
    priority: str | None = typer.Option(None),
) -> None:
    account = _account(ctx)
    state = _state(ctx)
    current = state.runtime.storage.get_task(account, task_id)
    if current is None:
        from .errors import NotFoundError

        raise NotFoundError(f"Task {task_id!r} does not exist")
    item = state.runtime.application.update_task(
        account,
        task_id,
        title=title,
        notes=current.notes if notes is None else notes,
        due=date.fromisoformat(due) if due else None,
        clear_due=clear_due,
        priority=priority,
    )
    _emit(ctx, item, human=lambda value: f"Updated task {value.id}: {value.title}")


@tasks_app.command("complete")
def tasks_complete(
    ctx: typer.Context,
    task_id: str,
    reopen: bool = typer.Option(False, "--reopen"),
) -> None:
    item = _state(ctx).runtime.application.complete_task(
        _account(ctx), task_id, completed=not reopen
    )
    _emit(ctx, item, human=lambda value: f"{value.id}\t{value.status.value}")


@tasks_app.command("delete")
def tasks_delete(
    ctx: typer.Context, task_id: str, yes: bool = typer.Option(False, "--yes", "-y")
) -> None:
    _confirm(yes, f"Delete task {task_id}?")
    item = _state(ctx).runtime.application.delete_task(_account(ctx), task_id)
    _emit(ctx, item, human=lambda value: f"Deleted task {value.id}")


@tasks_app.command("move")
def tasks_move(
    ctx: typer.Context,
    task_id: str,
    task_list: str | None = typer.Option(None, "--list"),
    parent: str | None = typer.Option(None),
    previous: str | None = typer.Option(None),
) -> None:
    item = _state(ctx).runtime.application.move_task(
        _account(ctx), task_id, list_id=task_list, parent_id=parent, previous_id=previous
    )
    _emit(ctx, item, human=lambda value: f"Moved task {value.id} to {value.list_id}")


@tasks_app.command("complete-many")
def tasks_complete_many(
    ctx: typer.Context, task_ids: list[str], reopen: bool = typer.Option(False, "--reopen")
) -> None:
    application = _state(ctx).runtime.application
    preview = application.preview_task_completion(_account(ctx), task_ids, completed=not reopen)
    _show_batch_preflight(ctx, preview)
    items = application.complete_tasks(_account(ctx), task_ids, completed=not reopen)
    _emit(ctx, items, human=lambda value: f"{value.id}\t{value.status.value}")


@tasks_app.command("delete-many")
def tasks_delete_many(
    ctx: typer.Context, task_ids: list[str], yes: bool = typer.Option(False, "--yes", "-y")
) -> None:
    application = _state(ctx).runtime.application
    preview = application.preview_task_deletion(_account(ctx), task_ids)
    _show_batch_preflight(ctx, preview)
    _confirm(yes, f"Delete {len(preview.items)} task(s)?")
    _emit(ctx, application.delete_tasks(_account(ctx), task_ids))


@tasks_app.command("move-many")
def tasks_move_many(
    ctx: typer.Context,
    task_ids: list[str],
    task_list: str = typer.Option(..., "--list"),
    yes: bool = typer.Option(False, "--yes", "-y"),
) -> None:
    account = _account(ctx)
    application = _state(ctx).runtime.application
    preview = application.preview_task_move(account, task_ids, task_list)
    if _show_move_preflight(ctx, preview):
        _confirm(yes, f"Move {len(preview.items)} task(s) to another list?")
    _emit(
        ctx,
        application.move_tasks(account, task_ids, task_list),
        human=lambda value: f"Moved task {value.id} to {value.list_id}",
    )


@tasks_app.command("schedule")
def tasks_schedule(
    ctx: typer.Context,
    task_id: str,
    calendar: str = typer.Option(...),
    start: str = typer.Option(...),
    end: str = typer.Option(...),
    time_zone: str | None = typer.Option(None, "--time-zone"),
) -> None:
    item = _state(ctx).runtime.application.schedule_task(
        _account(ctx),
        task_id,
        calendar,
        _event_point(start, False, time_zone),
        _event_point(end, False, time_zone),
    )
    _emit(ctx, item)


@tasks_app.command("unschedule")
def tasks_unschedule(
    ctx: typer.Context, task_id: str, yes: bool = typer.Option(False, "--yes", "-y")
) -> None:
    _confirm(yes, f"Delete the active calendar block for task {task_id}?")
    _emit(ctx, {"event": _state(ctx).runtime.application.unschedule_task(_account(ctx), task_id)})


@tasks_app.command("repair-schedule")
def tasks_repair_schedule(ctx: typer.Context, task_id: str, event_id: str) -> None:
    _emit(
        ctx,
        _state(ctx).runtime.application.repair_task_schedule(_account(ctx), task_id, event_id),
    )


@tasks_app.command("reconcile-recurrence")
def tasks_reconcile_recurrence(ctx: typer.Context) -> None:
    _emit(ctx, _state(ctx).runtime.application.reconcile_task_recurrence(_account(ctx)))


# Task lists
@lists_app.command("list")
def lists_list(ctx: typer.Context) -> None:
    items = _state(ctx).runtime.storage.list_task_lists(_account(ctx))
    _emit(
        ctx,
        items,
        fields=("id", "title", "position"),
        human=lambda item: f"{item.id}\t{item.title}",
    )


@lists_app.command("create")
def lists_create(ctx: typer.Context, title: str) -> None:
    item = _state(ctx).runtime.application.create_task_list(_account(ctx), title)
    _emit(ctx, item, human=lambda value: f"Created task list {value.id}: {value.title}")


@lists_app.command("edit")
def lists_edit(ctx: typer.Context, list_id: str, title: str = typer.Option(...)) -> None:
    item = _state(ctx).runtime.application.update_task_list(_account(ctx), list_id, title=title)
    _emit(ctx, item, human=lambda value: f"Updated task list {value.id}: {value.title}")


@lists_app.command("delete")
def lists_delete(
    ctx: typer.Context, list_id: str, yes: bool = typer.Option(False, "--yes", "-y")
) -> None:
    _confirm(yes, f"Delete task list {list_id} and its tasks?")
    item = _state(ctx).runtime.application.delete_task_list(_account(ctx), list_id)
    _emit(ctx, item, human=lambda value: f"Deleted task list {value.id}")


# Notes are task-note conveniences.
@notes_app.command("list")
def notes_list(ctx: typer.Context) -> None:
    items = _state(ctx).runtime.application.notes_listing(_account(ctx))
    _emit(ctx, items, fields=("id", "list_id", "title", "notes"))


@notes_app.command("mode")
def notes_mode(ctx: typer.Context, mode: str | None = typer.Argument(None)) -> None:
    application = _state(ctx).runtime.application
    value = (
        application.set_notes_projection(_account(ctx), mode)
        if mode is not None
        else application.notes_projection(_account(ctx))
    )
    _emit(ctx, {"mode": value.value})


@notes_app.command("show")
def notes_show(ctx: typer.Context, task_id: str) -> None:
    item = _state(ctx).runtime.storage.get_task(_account(ctx), task_id)
    if item is None:
        from .errors import NotFoundError

        raise NotFoundError(f"Task {task_id!r} does not exist")
    _emit(
        ctx,
        {"task_id": task_id, "notes": item.notes},
        fields=("task_id", "notes"),
        human=lambda value: value["notes"] or "",
    )


@notes_app.command("set")
def notes_set(ctx: typer.Context, task_id: str, text: str | None = typer.Argument(None)) -> None:
    value = sys.stdin.read() if text in (None, "-") else text
    item = _state(ctx).runtime.application.update_task(_account(ctx), task_id, notes=value)
    _emit(ctx, item, human=lambda task: f"Updated notes for {task.id}")


@notes_app.command("clear")
def notes_clear(
    ctx: typer.Context, task_id: str, yes: bool = typer.Option(False, "--yes", "-y")
) -> None:
    _confirm(yes, f"Clear notes for task {task_id}?")
    item = _state(ctx).runtime.application.update_task(_account(ctx), task_id, notes=None)
    _emit(ctx, item, human=lambda task: f"Cleared notes for {task.id}")


# Events
@events_app.command("agenda")
def events_agenda(
    ctx: typer.Context,
    start: str | None = typer.Option(None, "--from"),
    end: str | None = typer.Option(None, "--to"),
    calendar: str | None = typer.Option(None),
) -> None:
    start_date = date.fromisoformat(start) if start else date.today()
    stop = date.fromisoformat(end) if end else (start_date + timedelta(days=7))
    items = _state(ctx).runtime.application.agenda_events(
        _account(ctx), calendar_id=calendar, start=start_date, end=stop
    )
    _emit(
        ctx,
        items,
        fields=("id", "calendar_id", "summary", "start", "end"),
        human=lambda item: f"{item.start.value.isoformat()}\t{item.id}\t{item.summary}",
    )


@events_app.command("create")
def events_create(
    ctx: typer.Context,
    summary: str,
    start: str = typer.Option(...),
    end: str = typer.Option(...),
    calendar: str = typer.Option(...),
    all_day: bool = typer.Option(False, "--all-day"),
    time_zone: str | None = typer.Option(None, "--time-zone"),
    description: str | None = typer.Option(None),
    location: str | None = typer.Option(None),
    recurrence: list[str] | None = typer.Option(None, "--recurrence", "--rrule"),  # noqa: B008
    attendees_json: str | None = typer.Option(None, "--attendees-json"),
    reminders_json: str | None = typer.Option(None, "--reminders-json"),
    attachments_json: str | None = typer.Option(None, "--attachments-json"),
    event_type: str | None = typer.Option(None, "--event-type"),
    visibility: str | None = typer.Option(None),
    transparency: str | None = typer.Option(None),
    color_id: str | None = typer.Option(None, "--color-id"),
    meet: bool = typer.Option(False, "--meet"),
    send_updates: str = typer.Option("none", "--send-updates"),
) -> None:
    item = _state(ctx).runtime.application.create_event(
        _account(ctx),
        calendar,
        summary,
        _event_point(start, all_day, time_zone),
        _event_point(end, all_day, time_zone),
        description=description,
        location=location,
        recurrence=_recurrence_rules(recurrence),
        attendees=tuple(json.loads(attendees_json)) if attendees_json else (),
        reminder_use_default=reminders_json is None,
        reminder_overrides=tuple(
            ReminderOverride(item["method"], int(item["minutes"]))
            for item in (json.loads(reminders_json) if reminders_json else ())
        ),
        attachments=tuple(json.loads(attachments_json)) if attachments_json else (),
        event_type=event_type,
        visibility=visibility,
        transparency=transparency,
        color_id=color_id,
        conference=({"createRequest": {"requestId": uuid4().hex}} if meet else None),
        send_updates=send_updates,
        supports_attachments=bool(attachments_json),
        conference_data_version=1 if meet else 0,
    )
    _emit(ctx, item, human=lambda value: f"Created event {value.id}: {value.summary}")


@events_app.command("edit")
def events_edit(
    ctx: typer.Context,
    event_id: str,
    summary: str | None = typer.Option(None),
    start: str | None = typer.Option(None),
    end: str | None = typer.Option(None),
    all_day: bool = typer.Option(False, "--all-day"),
    time_zone: str | None = typer.Option(None, "--time-zone"),
    description: str | None = typer.Option(None),
    location: str | None = typer.Option(None),
    clear_description: bool = typer.Option(False, "--clear-description"),
    clear_location: bool = typer.Option(False, "--clear-location"),
    recurrence: list[str] | None = typer.Option(None, "--recurrence", "--rrule"),  # noqa: B008
    clear_recurrence: bool = typer.Option(False, "--clear-recurrence"),
    scope: str = typer.Option("this", "--scope"),
    send_updates: str = typer.Option("none", "--send-updates"),
) -> None:
    account = _account(ctx)
    state = _state(ctx)
    current = state.runtime.storage.get_event(account, event_id)
    if current is None:
        from .errors import NotFoundError

        raise NotFoundError(f"Event {event_id!r} does not exist")
    if recurrence is not None and clear_recurrence:
        raise ValueError("--recurrence and --clear-recurrence are mutually exclusive")
    changes: dict[str, Any] = {}
    if description is not None or clear_description:
        changes["description"] = None if clear_description else description
    if location is not None or clear_location:
        changes["location"] = None if clear_location else location
    if recurrence is not None or clear_recurrence:
        changes["recurrence"] = () if clear_recurrence else _recurrence_rules(recurrence)
    item = state.runtime.application.update_event(
        account,
        event_id,
        summary=summary,
        start=_event_point(start, all_day, time_zone) if start else None,
        end=_event_point(end, all_day, time_zone) if end else None,
        scope=scope,  # type: ignore[arg-type]
        send_updates=send_updates,
        **changes,
    )
    _emit(ctx, item, human=lambda value: f"Updated event {value.id}: {value.summary}")


@events_app.command("delete")
def events_delete(
    ctx: typer.Context,
    event_id: str,
    yes: bool = typer.Option(False, "--yes", "-y"),
    scope: str = typer.Option("this", "--scope"),
    send_updates: str = typer.Option("none", "--send-updates"),
) -> None:
    _confirm(yes, f"Delete event {event_id}?")
    item = _state(ctx).runtime.application.delete_event(
        _account(ctx),
        event_id,
        scope=scope,  # type: ignore[arg-type]
        send_updates=send_updates,
    )
    _emit(ctx, item, human=lambda value: f"Deleted event {value.id}")


@events_app.command("move")
def events_move(ctx: typer.Context, event_id: str, calendar: str = typer.Option(...)) -> None:
    item = _state(ctx).runtime.application.move_event(_account(ctx), event_id, calendar)
    _emit(ctx, item, human=lambda value: f"Moved event {value.id} to {value.calendar_id}")


@events_app.command("show")
def events_show(ctx: typer.Context, event_id: str) -> None:
    item = _state(ctx).runtime.storage.get_event(_account(ctx), event_id)
    if item is None:
        from .errors import NotFoundError

        raise NotFoundError(f"Event {event_id!r} does not exist")
    _emit(ctx, item)


@events_app.command("instances")
def events_instances(
    ctx: typer.Context,
    calendar: str | None = typer.Option(None, "--calendar"),
    start: str | None = typer.Option(None, "--from"),
    end: str | None = typer.Option(None, "--to"),
) -> None:
    """List locally cached recurring instances; this command performs no network I/O."""
    start_value = (
        _range_datetime(start) if start else datetime.combine(date.today(), datetime.min.time())
    )
    end_value = _range_datetime(end, end=True) if end else start_value + timedelta(days=7)
    items = [
        item
        for item in _state(ctx).runtime.storage.list_events(
            _account(ctx), calendar, start=start_value, end=end_value
        )
        if item.derived
    ]
    storage = _state(ctx).runtime.storage
    account = _account(ctx)
    calendar_ids = (
        (calendar,)
        if calendar is not None
        else tuple(item.id for item in storage.list_calendars(account))
    )
    cache = [
        storage.instance_cache_status(account, calendar_id, start_value, end_value)
        for calendar_id in calendar_ids
    ]
    if _state(ctx).json:
        _emit(ctx, {"instances": items, "cache": cache})
        return
    _emit(
        ctx,
        items,
        fields=("id", "calendar_id", "canonical_id", "summary", "start", "end"),
        human=lambda item: f"{item.start.value.isoformat()}\t{item.id}\t{item.summary}",
    )


@events_app.command("refresh-instances")
def events_refresh_instances(
    ctx: typer.Context,
    calendar: str = typer.Option(..., "--calendar"),
    start: str = typer.Option(..., "--from"),
    end: str = typer.Option(..., "--to"),
) -> None:
    """Explicitly fetch and cache Google-expanded recurring instances for one range."""
    state = _state(ctx)
    account = _account(ctx)
    items = state.runtime.sync_engine(account).refresh_occurrences(
        account, calendar, _range_datetime(start), _range_datetime(end, end=True)
    )
    start_value = _range_datetime(start)
    end_value = _range_datetime(end, end=True)
    _emit(
        ctx,
        {
            "calendar_id": calendar,
            "refreshed": len(items),
            "instances": items,
            "cache": state.runtime.storage.instance_cache_status(
                account, calendar, start_value, end_value
            ),
        },
    )


@events_app.command("instance-cache")
def events_instance_cache(
    ctx: typer.Context,
    calendar: str | None = typer.Option(None, "--calendar"),
) -> None:
    """Inspect raw local instance-cache ranges; this command performs no network I/O."""
    _emit(
        ctx,
        {"ranges": _state(ctx).runtime.storage.list_instance_ranges(_account(ctx), calendar)},
    )


@events_app.command("invitations")
def events_invitations(ctx: typer.Context) -> None:
    _emit(
        ctx,
        _state(ctx).runtime.application.invitations(_account(ctx)),
        fields=("id", "calendar_id", "summary", "attendee_response", "start"),
        human=lambda item: (
            f"{item.attendee_response}\t{item.start.value.isoformat()}\t{item.summary}"
        ),
    )


@events_app.command("duplicate")
def events_duplicate(
    ctx: typer.Context,
    event_id: str,
    calendar: str | None = typer.Option(None, "--calendar"),
    summary: str | None = typer.Option(None),
    start: str | None = typer.Option(None),
    end: str | None = typer.Option(None),
    all_day: bool = typer.Option(False, "--all-day"),
    time_zone: str | None = typer.Option(None, "--time-zone"),
    include_recurrence: bool = typer.Option(False, "--include-recurrence"),
    include_attendees: bool = typer.Option(False, "--include-attendees"),
    send_updates: str = typer.Option("none", "--send-updates"),
) -> None:
    if (start is None) != (end is None):
        raise ValueError("--start and --end must be supplied together")
    item = _state(ctx).runtime.application.duplicate_event(
        _account(ctx),
        event_id,
        calendar_id=calendar,
        summary=summary,
        start=_event_point(start, all_day, time_zone) if start else None,
        end=_event_point(end, all_day, time_zone) if end else None,
        include_recurrence=include_recurrence,
        include_attendees=include_attendees,
        send_updates=send_updates,
    )
    _emit(ctx, item, human=lambda value: f"Duplicated event {value.id}: {value.summary}")


@events_app.command("invite")
def events_invite(
    ctx: typer.Context,
    event_id: str,
    emails: list[str],
    send_updates: str = typer.Option("all", "--send-updates"),
) -> None:
    account = _account(ctx)
    current = _state(ctx).runtime.storage.get_event(account, event_id)
    if current is None:
        from .errors import NotFoundError

        raise NotFoundError(f"Event {event_id!r} does not exist")
    existing = {
        str(attendee.get("email", "")).casefold()
        for attendee in current.attendees
        if attendee.get("email")
    }
    additions = [{"email": email} for email in emails if email.casefold() not in existing]
    item = _state(ctx).runtime.application.update_event(
        account,
        event_id,
        attendees=tuple((*current.attendees, *additions)),
        send_updates=send_updates,
    )
    _emit(ctx, item, human=lambda value: f"Updated invitees for {value.id}")


@events_app.command("set-properties")
def events_set_properties(
    ctx: typer.Context,
    event_id: str,
    properties_json: str,
    send_updates: str = typer.Option("none", "--send-updates"),
) -> None:
    """Set retained specialist Google event fields from a structured JSON object."""
    value = json.loads(properties_json)
    if not isinstance(value, dict):
        raise ValueError("properties_json must be a JSON object")
    reminders = value.get("reminders")
    if reminders is not None and not isinstance(reminders, list):
        raise ValueError("reminders must be an array")
    changes: dict[str, Any] = {}
    if "attendees" in value:
        attendees = value["attendees"]
        if attendees is not None and not isinstance(attendees, list):
            raise ValueError("attendees must be an array or null")
        changes["attendees"] = tuple(attendees or ())
    if "reminderUseDefault" in value:
        changes["reminder_use_default"] = value["reminderUseDefault"]
    if "reminders" in value:
        changes["reminder_overrides"] = tuple(
            ReminderOverride(entry["method"], int(entry["minutes"])) for entry in (reminders or ())
        )
    for source, target in (
        ("eventType", "event_type"),
        ("transparency", "transparency"),
        ("visibility", "visibility"),
        ("colorId", "color_id"),
        ("conferenceData", "conference"),
        ("guestsCanInviteOthers", "guests_can_invite_others"),
        ("guestsCanModify", "guests_can_modify"),
        ("guestsCanSeeOtherGuests", "guests_can_see_other_guests"),
        ("anyoneCanAddSelf", "anyone_can_add_self"),
        ("focusTimeProperties", "focus_time_properties"),
        ("outOfOfficeProperties", "out_of_office_properties"),
        ("workingLocationProperties", "working_location_properties"),
    ):
        if source in value:
            changes[target] = value[source]
    if "attachments" in value:
        attachments = value["attachments"]
        if attachments is not None and not isinstance(attachments, list):
            raise ValueError("attachments must be an array or null")
        changes["attachments"] = tuple(attachments or ())
    item = _state(ctx).runtime.application.update_event(
        _account(ctx),
        event_id,
        send_updates=send_updates,
        supports_attachments="attachments" in changes,
        conference_data_version=1 if "conference" in changes else 0,
        **changes,
    )
    _emit(ctx, item)


@events_app.command("respond")
def events_respond(
    ctx: typer.Context,
    event_id: str,
    response: str,
    comment: str | None = typer.Option(None),
    send_updates: str = typer.Option("all", "--send-updates"),
) -> None:
    item = _state(ctx).runtime.application.respond_event(
        _account(ctx),
        event_id,
        response,  # type: ignore[arg-type]
        comment=comment,
        send_updates=send_updates,
    )
    _emit(ctx, item, human=lambda value: f"Queued {response} response for {value.entity_id}")


@events_app.command("delete-many")
def events_delete_many(
    ctx: typer.Context, event_ids: list[str], yes: bool = typer.Option(False, "--yes", "-y")
) -> None:
    application = _state(ctx).runtime.application
    preview = application.preview_event_deletion(_account(ctx), event_ids)
    _show_batch_preflight(ctx, preview)
    _confirm(yes, f"Delete {len(preview.items)} event(s)?")
    _emit(ctx, application.delete_events(_account(ctx), event_ids))


@events_app.command("respond-many")
def events_respond_many(
    ctx: typer.Context,
    event_ids: list[str],
    response: str = typer.Option(...),
    comment: str | None = typer.Option(None),
    send_updates: str = typer.Option("all", "--send-updates"),
) -> None:
    application = _state(ctx).runtime.application
    preview = application.preview_event_response(
        _account(ctx),
        event_ids,
        response,  # type: ignore[arg-type]
    )
    _show_batch_preflight(ctx, preview)
    _emit(
        ctx,
        application.respond_events(
            _account(ctx),
            event_ids,
            response,  # type: ignore[arg-type]
            comment=comment,
            send_updates=send_updates,
        ),
    )


@events_app.command("move-many")
def events_move_many(
    ctx: typer.Context,
    event_ids: list[str],
    calendar: str = typer.Option(...),
    yes: bool = typer.Option(False, "--yes", "-y"),
) -> None:
    account = _account(ctx)
    application = _state(ctx).runtime.application
    preview = application.preview_event_move(account, event_ids, calendar)
    if _show_move_preflight(ctx, preview):
        _confirm(yes, f"Move {len(preview.items)} event(s) to another calendar?")
    _emit(
        ctx,
        application.move_events(account, event_ids, calendar),
    )


@events_app.command("split")
def events_split(
    ctx: typer.Context, event_id: str, yes: bool = typer.Option(False, "--yes", "-y")
) -> None:
    _confirm(yes, "Split this recurring event and following instances?")
    _emit(ctx, _state(ctx).runtime.application.split_recurring_event(_account(ctx), event_id))


# Calendars
@calendars_app.command("list")
def calendars_list(ctx: typer.Context) -> None:
    items = _state(ctx).runtime.storage.list_calendars(_account(ctx))
    _emit(
        ctx,
        items,
        fields=("id", "summary", "time_zone", "selected"),
        human=lambda item: f"{item.id}\t{item.summary}",
    )


@calendars_app.command("create")
def calendars_create(
    ctx: typer.Context,
    summary: str,
    description: str | None = typer.Option(None),
    time_zone: str | None = typer.Option(None, "--time-zone"),
    location: str | None = typer.Option(None),
    color: str | None = typer.Option(None, "--color"),
) -> None:
    item = _state(ctx).runtime.application.create_calendar(
        _account(ctx),
        summary,
        description=description,
        time_zone=time_zone,
        location=location,
        color=color,
    )
    _emit(ctx, item, human=lambda value: f"Created calendar {value.id}: {value.summary}")


@calendars_app.command("subscribe")
def calendars_subscribe(
    ctx: typer.Context, remote_calendar_id: str, summary: str | None = typer.Option(None)
) -> None:
    item = _state(ctx).runtime.application.subscribe_calendar(
        _account(ctx), remote_calendar_id, summary=summary
    )
    _emit(ctx, item, human=lambda value: f"Subscribed to calendar {value.id}")


@calendars_app.command("edit")
def calendars_edit(
    ctx: typer.Context,
    calendar_id: str,
    summary: str | None = typer.Option(None),
    description: str | None = typer.Option(None),
    clear_description: bool = typer.Option(False, "--clear-description"),
    time_zone: str | None = typer.Option(None, "--time-zone"),
    clear_time_zone: bool = typer.Option(False, "--clear-time-zone"),
    location: str | None = typer.Option(None),
    clear_location: bool = typer.Option(False, "--clear-location"),
) -> None:
    if (
        (description is not None and clear_description)
        or (time_zone is not None and clear_time_zone)
        or (location is not None and clear_location)
    ):
        raise ValueError("a value and its --clear flag are mutually exclusive")
    changes: dict[str, Any] = {}
    if description is not None or clear_description:
        changes["description"] = None if clear_description else description
    if time_zone is not None or clear_time_zone:
        changes["time_zone"] = None if clear_time_zone else time_zone
    if location is not None or clear_location:
        changes["location"] = None if clear_location else location
    item = _state(ctx).runtime.application.update_calendar(
        _account(ctx), calendar_id, summary=summary, **changes
    )
    _emit(ctx, item, human=lambda value: f"Updated calendar {value.id}: {value.summary}")


@calendars_app.command("set-list")
def calendars_set_list(
    ctx: typer.Context,
    calendar_id: str,
    color: str | None = typer.Option(None, "--color"),
    clear_color: bool = typer.Option(False, "--clear-color"),
    foreground_color: str | None = typer.Option(None, "--foreground-color"),
    clear_foreground_color: bool = typer.Option(False, "--clear-foreground-color"),
    summary_override: str | None = typer.Option(None, "--summary-override"),
    clear_summary_override: bool = typer.Option(False, "--clear-summary-override"),
    selected: bool | None = typer.Option(None, "--selected/--unselected"),  # noqa: B008
    hidden: bool | None = typer.Option(None, "--hidden/--shown"),  # noqa: B008
    reminders_json: str | None = typer.Option(None, "--reminders-json"),
    notifications_json: str | None = typer.Option(None, "--notifications-json"),
) -> None:
    if any(
        (
            color is not None and clear_color,
            foreground_color is not None and clear_foreground_color,
            summary_override is not None and clear_summary_override,
        )
    ):
        raise ValueError("a value and its --clear flag are mutually exclusive")
    changes: dict[str, Any] = {}
    for key, value, clear in (
        ("color", color, clear_color),
        ("foreground_color", foreground_color, clear_foreground_color),
        ("summary_override", summary_override, clear_summary_override),
    ):
        if value is not None or clear:
            changes[key] = None if clear else value
    if selected is not None:
        changes["selected"] = selected
    if hidden is not None:
        changes["hidden"] = hidden
    if reminders_json is not None:
        parsed = json.loads(reminders_json)
        if not isinstance(parsed, list):
            raise ValueError("reminders_json must be a JSON array")
        changes["default_reminders"] = tuple(
            ReminderOverride(item["method"], int(item["minutes"])) for item in parsed
        )
    if notifications_json is not None:
        parsed = json.loads(notifications_json)
        if not isinstance(parsed, list) or not all(isinstance(item, dict) for item in parsed):
            raise ValueError("notifications_json must be a JSON array of objects")
        changes["notification_settings"] = tuple(parsed)
    item = _state(ctx).runtime.application.update_calendar(_account(ctx), calendar_id, **changes)
    _emit(ctx, item, human=lambda value: f"Updated calendar-list preferences for {value.id}")


@calendars_app.command("colors")
def calendars_colors(ctx: typer.Context) -> None:
    """Explicitly read Google's available calendar and event color IDs."""
    state = _state(ctx)
    _emit(ctx, state.runtime.sync_engine(_account(ctx)).gateway.calendar_colors())


@calendars_app.command("remove")
def calendars_remove(
    ctx: typer.Context, calendar_id: str, yes: bool = typer.Option(False, "--yes", "-y")
) -> None:
    _confirm(yes, f"Remove calendar {calendar_id} and its local events?")
    item = _state(ctx).runtime.application.remove_calendar_from_list(_account(ctx), calendar_id)
    _emit(ctx, item, human=lambda value: f"Removed calendar {value.id}")


@calendars_app.command("delete")
def calendars_delete(
    ctx: typer.Context, calendar_id: str, yes: bool = typer.Option(False, "--yes", "-y")
) -> None:
    _confirm(yes, f"Permanently delete calendar {calendar_id} from Google?")
    item = _state(ctx).runtime.application.delete_calendar(_account(ctx), calendar_id)
    _emit(ctx, item, human=lambda value: f"Deleted calendar {value.id}")


# Search and capture
@app.command("search")
def search(ctx: typer.Context, query: str, limit: int = typer.Option(50)) -> None:
    results = _state(ctx).runtime.application.search(_account(ctx), query, limit=limit)
    rows = [
        {"kind": result.kind, "score": result.score, **to_primitive(result.item)}
        for result in results
    ]
    _emit(
        ctx,
        rows,
        fields=("kind", "id", "score", "title", "summary"),
        human=lambda row: f"{row['kind']}\t{row['id']}\t{row.get('title') or row.get('summary')}",
    )


@app.command("find-time")
def find_time(
    ctx: typer.Context,
    day: str = typer.Option(..., "--date"),
    duration: int = typer.Option(30, "--duration"),
    day_start: int = typer.Option(9, "--day-start"),
    day_end: int = typer.Option(17, "--day-end"),
) -> None:
    """Find free slots using only locally cached selected calendars."""
    slots = _state(ctx).runtime.application.find_time(
        _account(ctx),
        date.fromisoformat(day),
        duration_minutes=duration,
        day_start=day_start,
        day_end=day_end,
    )
    _emit(
        ctx,
        slots,
        fields=("start", "end"),
        human=lambda slot: f"{slot.start.isoformat()}\t{slot.end.isoformat()}",
    )


@app.command("freebusy")
def remote_freebusy(
    ctx: typer.Context,
    start: str = typer.Option(...),
    end: str = typer.Option(...),
    calendars: list[str] | None = typer.Option(None, "--calendar"),  # noqa: B008
) -> None:
    """Explicitly query Google's remote free/busy endpoint."""
    state = _state(ctx)
    account = _account(ctx)
    selected = calendars or [
        item.remote_id
        for item in state.runtime.storage.list_calendars(account)
        if item.selected and item.remote_id
    ]
    body = {"timeMin": start, "timeMax": end, "items": [{"id": item} for item in selected]}
    _emit(ctx, state.runtime.sync_engine(account).gateway.freebusy(body))


@drive_app.command("search")
def drive_search(ctx: typer.Context, query: str) -> None:
    """Explicitly search remote Drive metadata and cache the result."""
    state = _state(ctx)
    account = _account(ctx)
    page = state.runtime.sync_engine(account).gateway.search_drive_metadata(query)
    items = state.runtime.application.cache_drive_metadata(account, list(page.items))
    _emit(ctx, items, fields=("id", "name", "mime_type", "web_view_link"))


@drive_app.command("list")
def drive_list(ctx: typer.Context) -> None:
    _emit(ctx, _state(ctx).runtime.storage.list_drive_files(_account(ctx)))


@saved_app.command("list")
def saved_list(ctx: typer.Context) -> None:
    items = _state(ctx).runtime.application.list_saved_searches(_account(ctx))
    _emit(
        ctx,
        items,
        fields=("id", "name", "query"),
        human=lambda item: f"{item.id}\t{item.name}\t{item.query}",
    )


@saved_app.command("save")
def saved_save(ctx: typer.Context, name: str, query: str) -> None:
    item = _state(ctx).runtime.application.save_search(_account(ctx), name, query)
    _emit(ctx, item, human=lambda value: f"Saved search {value.id}: {value.name}")


@saved_app.command("run")
def saved_run(ctx: typer.Context, search_id: str) -> None:
    results: tuple[SearchResult, ...] = _state(ctx).runtime.application.run_saved_search(
        _account(ctx), search_id
    )
    rows = [
        {"kind": result.kind, "score": result.score, **to_primitive(result.item)}
        for result in results
    ]
    _emit(ctx, rows, fields=("kind", "id", "score", "title", "summary"))


@saved_app.command("delete")
def saved_delete(
    ctx: typer.Context, search_id: str, yes: bool = typer.Option(False, "--yes", "-y")
) -> None:
    _confirm(yes, f"Delete saved search {search_id}?")
    _state(ctx).runtime.application.delete_saved_search(_account(ctx), search_id)
    _emit(
        ctx, {"deleted": search_id}, human=lambda value: f"Deleted saved search {value['deleted']}"
    )


@app.command("capture")
def capture(
    ctx: typer.Context,
    text: str | None = typer.Argument(None),
    kind: str = typer.Option("task"),
    task_list: str | None = typer.Option(None, "--list"),
    calendar: str | None = typer.Option(None),
    time_zone: str | None = typer.Option(None, "--time-zone"),
) -> None:
    raw = sys.stdin.read() if text in (None, "-") else text
    state = _state(ctx)
    item = state.runtime.application.quick_capture(
        _account(ctx),
        raw.strip(),
        kind,  # type: ignore[arg-type]
        task_list_id=task_list or state.runtime.config.preferences.default_task_list_id,
        calendar_id=calendar or state.runtime.config.preferences.default_calendar_id,
        preferences=state.runtime.config.preferences.capture,
        time_zone=time_zone or state.runtime.config.preferences.time_zone,
    )
    _emit(ctx, item, human=lambda value: f"Captured {value.id}")


# The only command boundary which constructs Google services.
@app.command("sync")
def sync(
    ctx: typer.Context,
    full_tasks: bool = typer.Option(False, "--full-tasks", help="Reset task cursors first."),
) -> None:
    state = _state(ctx)
    account = _account(ctx)
    try:
        result = state.runtime.sync_engine(account).sync(
            account,
            full_tasks=full_tasks,
            progress=lambda status: click.echo(status, err=True),
            cancel_hint="Press Ctrl+C to cancel.",
        )
    except KeyboardInterrupt as exc:
        raise OfflineError(
            "Sync cancelled. Local changes remain queued.", hint="Run hcb sync to resume."
        ) from exc
    if result.retry_pending and not result.retry_exhausted:
        raise ConflictError(
            result.retry_message or "Sync paused for uncertain delivery.",
            hint="Check Google, then run hcb conflicts list and hcb conflicts resolve-delivery.",
        )
    if result.cancelled or result.retry_exhausted:
        raise OfflineError(
            result.retry_message or "Sync paused. Local changes remain queued.",
            hint="Run hcb sync to resume.",
        )
    _emit(
        ctx,
        result,
        human=lambda value: (
            f"Sync complete: pulled={value.pulled} pushed={value.pushed} "
            f"conflicts={value.conflicts}"
        ),
    )


# Conflicts
@conflicts_app.command("list")
def conflicts_list(ctx: typer.Context, all_items: bool = typer.Option(False, "--all")) -> None:
    items = _state(ctx).runtime.storage.list_conflicts(_account(ctx), open_only=not all_items)
    _emit(
        ctx,
        items,
        fields=("id", "entity_type", "entity_id", "status"),
        human=lambda item: (
            f"{item.id}\t{item.entity_type.value}\t{item.entity_id}\t{item.status.value}"
        ),
    )


@conflicts_app.command("resolve")
def conflicts_resolve(
    ctx: typer.Context,
    conflict_id: int,
    resolution: str = typer.Option(...),
    merged_json: str | None = typer.Option(None, "--merged-json"),
    yes: bool = typer.Option(False, "--yes", "-y"),
) -> None:
    _confirm(yes, f"Resolve conflict {conflict_id} as {resolution}?")
    merged = json.loads(merged_json) if merged_json else None
    item = _state(ctx).runtime.application.resolve_conflict(
        _account(ctx), conflict_id, ConflictStatus(resolution), merged_payload=merged
    )
    _emit(ctx, item, human=lambda value: f"Resolved conflict {value.id}: {value.status.value}")


@conflicts_app.command("resolve-delivery")
def conflicts_resolve_delivery(
    ctx: typer.Context,
    conflict_id: int,
    action: str = typer.Option(..., help="retry or delivered"),
    remote_id: str | None = typer.Option(None, "--remote-id"),
    yes: bool = typer.Option(False, "--yes", "-y"),
) -> None:
    _confirm(yes, f"Resolve uncertain delivery {conflict_id} as {action}?")
    item = _state(ctx).runtime.application.resolve_uncertain_delivery(
        _account(ctx),
        conflict_id,
        action,
        remote_id=remote_id,
    )
    _emit(ctx, item, human=lambda value: f"Resolved delivery {value.id}: {value.status.value}")


# Import/export
def _source(path: str, filename: str | None) -> tuple[str, bytes]:
    if path == "-":
        if not filename:
            raise ValueError("--filename is required when importing from stdin")
        return filename, sys.stdin.buffer.read()
    target = Path(path)
    return target.name, target.read_bytes()


@import_app.command("preview")
def import_preview(
    ctx: typer.Context, path: str, filename: str | None = typer.Option(None)
) -> None:
    name, raw = _source(path, filename)
    preview = _state(ctx).runtime.application.preview_import(name, raw)
    _emit(
        ctx,
        preview,
        human=lambda value: (
            f"Import preview: rows={len(value.rows)} "
            f"errors={len(value.errors) + sum(bool(row.errors) for row in value.rows)}"
        ),
    )


@import_app.command("apply")
def import_apply(
    ctx: typer.Context,
    path: str,
    filename: str | None = typer.Option(None),
    task_list: str | None = typer.Option(None, "--list"),
    calendar: str | None = typer.Option(None),
    yes: bool = typer.Option(False, "--yes", "-y"),
) -> None:
    _confirm(yes, "Apply import to the local database?")
    name, raw = _source(path, filename)
    state = _state(ctx)
    preview = state.runtime.application.preview_import(name, raw)
    result = state.runtime.application.apply_import(
        _account(ctx),
        preview,
        default_task_list_id=task_list or state.runtime.config.preferences.default_task_list_id,
        default_calendar_id=calendar or state.runtime.config.preferences.default_calendar_id,
    )
    _emit(
        ctx,
        result,
        human=lambda value: f"Imported {len(value.tasks)} tasks and {len(value.events)} events",
    )


@app.command("export")
def export(
    ctx: typer.Context,
    format: str = typer.Option("json", "--format", "-f"),
    output: Path | None = typer.Option(None, "--output", "-o"),  # noqa: B008
) -> None:
    state = _state(ctx)
    account = _account(ctx)
    lists = {item.id: item.title for item in state.runtime.storage.list_task_lists(account)}
    calendars = {item.id: item.summary for item in state.runtime.storage.list_calendars(account)}
    records: list[ImportedRecord] = [
        *(_task_record(item, lists) for item in state.runtime.storage.list_tasks(account)),
        *(_event_record(item, calendars) for item in state.runtime.storage.list_events(account)),
    ]
    exporters = {"json": export_json, "csv": export_csv, "ics": export_ics}
    if format not in exporters:
        raise ValueError("export format must be json, csv, or ics")
    text = exporters[format](records)
    if state.json:
        if output:
            output.write_text(text, encoding="utf-8")
            _emit(ctx, {"format": format, "output": str(output)})
            return
        content: object = json.loads(text) if format == "json" else text
        _emit(ctx, {"format": format, "content": content})
        return
    if output:
        output.write_text(text, encoding="utf-8")
        typer.echo(str(output))
    else:
        typer.echo(text, nl=not text.endswith("\n"))


# Authentication
@auth_app.command("connect")
def auth_connect(
    ctx: typer.Context,
    account_id: str,
    email: str,
    no_browser: bool = typer.Option(False, "--no-browser"),
) -> None:
    state = _state(ctx)
    result = state.runtime.authenticator(account_id).connect(
        account_id, expected_email=email, open_browser=not no_browser
    )
    state.runtime.storage.upsert_account(Account(account_id, email))
    _emit(
        ctx,
        {"account_id": account_id, "email": email, "scopes": result.granted_scopes},
        human=lambda value: f"Connected {value['email']} ({value['account_id']})",
    )


@auth_app.command("status")
def auth_status(ctx: typer.Context) -> None:
    state = _state(ctx)
    rows = [
        {
            **to_primitive(item),
            "authenticated": state.runtime.token_store_for(item.id).get(item.id) is not None,
        }
        for item in state.runtime.storage.list_accounts()
    ]
    _emit(
        ctx,
        rows,
        fields=("id", "email", "enabled", "authenticated"),
        human=lambda item: (
            f"{item['id']}\t{item['email']}\t"
            f"{'connected' if item['authenticated'] else 'disconnected'}"
        ),
    )


@auth_app.command("disconnect")
def auth_disconnect(
    ctx: typer.Context,
    account_id: str | None = typer.Argument(None),
) -> None:
    target = account_id or _account(ctx)
    state = _state(ctx)
    removed = state.runtime.disconnect(target)
    _emit(
        ctx,
        {"account_id": target, "credentials_removed": removed},
        human=lambda value: f"Disconnected {value['account_id']}",
    )


@auth_app.command("reset")
def auth_reset(
    ctx: typer.Context,
    account_id: str | None = typer.Argument(None),
    yes: bool = typer.Option(False, "--yes", "-y"),
) -> None:
    target = account_id or _account(ctx)
    _confirm(yes, f"Permanently remove all cached data for {target}?")
    state = _state(ctx)
    removed = state.runtime.disconnect(target, reset_local_data=True)
    _emit(ctx, {"account_id": target, "credentials_removed": removed, "cache_removed": True})


@app.command("undo")
def undo(ctx: typer.Context) -> None:
    _emit(ctx, {"intent_id": _state(ctx).runtime.application.undo(_account(ctx))})


@app.command("redo")
def redo(ctx: typer.Context) -> None:
    _emit(ctx, {"intent_id": _state(ctx).runtime.application.redo(_account(ctx))})


# Configuration
@config_app.command("show")
def config_show(ctx: typer.Context) -> None:
    _emit(ctx, _state(ctx).runtime.config)


@config_app.command("path")
def config_path(ctx: typer.Context) -> None:
    _emit(
        ctx, {"path": str(_state(ctx).runtime.paths.config_file)}, human=lambda value: value["path"]
    )


@config_app.command("init")
def config_init(ctx: typer.Context, force: bool = typer.Option(False, "--force")) -> None:
    target = _state(ctx).runtime.paths.config_file
    if target.exists() and not force:
        raise ValueError(f"{target} already exists; use --force to replace it")
    save(_state(ctx).runtime.config, target)
    _emit(ctx, {"path": str(target)}, human=lambda value: value["path"])


@config_app.command("edit")
def config_edit(ctx: typer.Context) -> None:
    """Open ``config.json`` in the configured editor and validate it when it closes."""

    state = _state(ctx)
    target = state.runtime.paths.config_file
    if not target.exists():
        save(state.runtime.config, target)
    _open_in_external_editor(state.runtime, target, fallback_editor="nvim")
    load(target)
    _emit(
        ctx,
        {"path": str(target), "valid": True},
        human=lambda value: f"Validated {value['path']}",
    )


@config_app.command("schema")
def config_schema_show(ctx: typer.Context) -> None:
    """Print the Draft 2020-12 schema for the strict ``config.json`` file."""
    _emit(ctx, config_schema(), human=lambda value: json.dumps(value, indent=2, sort_keys=True))


def _coerce_config(current: Any, raw: str) -> Any:
    if isinstance(current, bool):
        if raw.lower() not in {"true", "false"}:
            raise ValueError("boolean configuration values must be true or false")
        return raw.lower() == "true"
    if isinstance(current, int):
        return int(raw)
    if current is None:
        return None if raw.lower() == "none" else raw
    return raw


def _config_value(document: dict[str, Any], key: str) -> Any:
    value: Any = document
    for part in key.split("."):
        if not isinstance(value, dict) or part not in value:
            raise ValueError(f"unknown configuration key {key!r}")
        value = value[part]
    return value


def _set_config_value(document: dict[str, Any], key: str, value: Any) -> None:
    parts = key.split(".")
    target: dict[str, Any] = document
    for part in parts[:-1]:
        current = target.get(part)
        if not isinstance(current, dict):
            raise ValueError(f"unknown configuration key {key!r}")
        target = current
    if parts[-1] not in target:
        raise ValueError(f"unknown configuration key {key!r}")
    target[parts[-1]] = value


@config_app.command("set")
def config_set(
    ctx: typer.Context,
    key: str,
    value: str,
    as_json: bool = typer.Option(False, "--json", help="Interpret VALUE as JSON."),
) -> None:
    state = _state(ctx)
    current = load(state.runtime.paths.config_file, resolve_profile=False)
    if key == "theme.preset":
        raise ValueError("apply a preset with `hcb themes apply NAME`, not config set")
    document = asdict(current)
    previous = _config_value(document, key)
    try:
        updated_value = json.loads(value) if as_json else _coerce_config(previous, value)
    except json.JSONDecodeError as exc:
        raise ValueError(f"invalid JSON value: {exc.msg}") from exc
    _set_config_value(document, key, updated_value)
    if key.startswith("theme."):
        document["theme"]["preset"] = None
    updated = loads(json.dumps(document))
    save(updated, state.runtime.paths.config_file)
    state.runtime.__dict__.pop("config", None)
    _emit(
        ctx,
        {"key": key, "value": updated_value},
        human=lambda row: f"{row['key']}={row['value']}",
    )


@config_app.command("get")
def config_get(ctx: typer.Context, key: str) -> None:
    value = _config_value(asdict(_state(ctx).runtime.config), key)
    _emit(ctx, {"key": key, "value": value}, human=lambda row: str(row["value"]))


@config_app.command("unset")
def config_unset(ctx: typer.Context, key: str) -> None:
    state = _state(ctx)
    current = load(state.runtime.paths.config_file, resolve_profile=False)
    document = asdict(current)
    default = asdict(Config())
    _set_config_value(document, key, _config_value(default, key))
    updated = loads(json.dumps(document))
    save(updated, state.runtime.paths.config_file)
    state.runtime.__dict__.pop("config", None)
    _emit(ctx, {"key": key, "value": _config_value(asdict(updated), key)})


@config_app.command("explain")
def config_explain(ctx: typer.Context, key: str) -> None:
    current = _config_value(asdict(_state(ctx).runtime.config), key)
    default = _config_value(asdict(Config()), key)
    _emit(
        ctx,
        {
            "key": key,
            "value": current,
            "default": default,
            "reload": _config_reload_behavior(key),
        },
    )


def _config_reload_behavior(key: str) -> str:
    if key == "theme.stylesheet":
        return "restart required"
    if key.startswith(("theme.", "keys.", "tui.")) or key in {
        "preferences.week_starts_on",
        "preferences.date_time_format",
        "preferences.time_zone",
        "preferences.editor",
    }:
        return "live in the running TUI"
    return "next command or daemon start"


@config_profiles_app.command("list")
def config_profiles_list(ctx: typer.Context) -> None:
    paths = _state(ctx).runtime.paths
    names = (
        sorted(path.stem for path in paths.profiles_dir.glob("*.json"))
        if paths.profiles_dir.exists()
        else []
    )
    _emit(ctx, {"active": _state(ctx).runtime.config.active_profile, "profiles": names})


@config_profiles_app.command("create")
def config_profiles_create(ctx: typer.Context, name: str) -> None:
    target = profile_path(_state(ctx).runtime.paths.config_file, name)
    if target.exists():
        raise ValueError(f"profile {name!r} already exists")
    target.parent.mkdir(parents=True, exist_ok=True)
    target.write_text("{}\n", encoding="utf-8")
    _emit(ctx, {"name": name, "path": str(target)})


@config_profiles_app.command("select")
def config_profiles_select(ctx: typer.Context, name: str) -> None:
    state = _state(ctx)
    current = load(state.runtime.paths.config_file, resolve_profile=False)
    if not profile_path(state.runtime.paths.config_file, name).exists():
        raise ValueError(f"profile {name!r} does not exist; create it first")
    updated = replace(current, active_profile=name)
    save(updated, state.runtime.paths.config_file)
    state.runtime.__dict__.pop("config", None)
    _emit(ctx, {"active": name})


@config_profiles_app.command("delete")
def config_profiles_delete(ctx: typer.Context, name: str) -> None:
    state = _state(ctx)
    target = profile_path(state.runtime.paths.config_file, name)
    if not target.exists():
        raise ValueError(f"profile {name!r} does not exist")
    target.unlink()
    current = load(state.runtime.paths.config_file, resolve_profile=False)
    if current.active_profile == name:
        save(replace(current, active_profile=None), state.runtime.paths.config_file)
        state.runtime.__dict__.pop("config", None)
    _emit(ctx, {"deleted": name})


# Themes
@themes_app.command("list")
def themes_list(ctx: typer.Context) -> None:
    """List all built-in Ghostty-derived visual presets."""
    _emit(
        ctx,
        presets(),
        fields=("rank", "name", "family", "profile"),
        human=lambda item: f"{item.rank:>2}  {item.name}  ·  {item.family} ({item.profile})",
    )


@themes_app.command("show")
def themes_show(ctx: typer.Context, name: str) -> None:
    """Show all semantic token values in one built-in preset."""
    _emit(ctx, preset(name))


@themes_app.command("apply")
def themes_apply(
    ctx: typer.Context,
    name: str | None = typer.Argument(None),
    file: Path | None = typer.Option(  # noqa: B008
        None, "--file", help="Strict standalone Theme JSON file."
    ),
) -> None:
    """Apply a bundled preset, or every visual setting from a custom theme file."""
    if (name is None) == (file is None):
        raise ValueError("provide exactly one preset NAME or --file PATH")
    state = _state(ctx)
    source: dict[str, object]
    if file is not None:
        theme = load_custom_theme(file)
        source = {"kind": "custom", "path": str(file)}
    else:
        selected = preset(cast(str, name))
        theme = apply_preset(state.runtime.config.theme, selected.name)
        source = {
            "kind": "builtin",
            "rank": selected.rank,
            "name": selected.name,
            "upstream_name": selected.upstream_name,
        }
    updated = state.runtime.update_theme(theme)
    _emit(ctx, {"source": source, "theme": updated.theme})


@themes_app.command("edit")
def themes_edit(ctx: typer.Context, file: Path) -> None:
    """Open an existing custom theme JSON file and validate it when it closes."""

    target = file.expanduser()
    if not target.is_file():
        raise ValueError(f"custom theme file must be an existing file: {target}")
    _open_in_external_editor(_state(ctx).runtime, target)
    load_custom_theme(target)
    _emit(
        ctx,
        {"path": str(target), "valid": True},
        human=lambda value: f"Validated {value['path']}",
    )


@schema_app.command("list")
def schema_list(ctx: typer.Context) -> None:
    """List stable `--json` command contracts bundled with this release."""
    _emit(
        ctx,
        {
            "schema_version": JSON_SCHEMA_VERSION,
            "schema_id": json_schema_bundle()["$id"],
            "commands": list(JSON_COMMANDS),
        },
    )


@schema_app.command("show")
def schema_show(ctx: typer.Context, command: str) -> None:
    """Print the Draft 2020-12 schema fragment for one dotted command name."""
    _emit(ctx, {"command": command, "schema": json_command_schema(command)})


@app.command("doctor")
def doctor(ctx: typer.Context) -> None:
    state = _state(ctx)
    diagnostics = state.runtime.application.diagnostics()
    diagnostics.update(
        {
            "config": state.runtime.paths.config_file.name,
            "config_valid": True,
            "color": not (
                os.environ.get("NO_COLOR") is not None or os.environ.get("TERM") == "dumb"
            ),
        }
    )
    _emit(
        ctx,
        diagnostics,
        human=lambda row: "\n".join(f"{key}: {value}" for key, value in row.items()),
    )


def _uninstall_plan_data(plan: UninstallPlan, *, keep_program: bool) -> dict[str, object]:
    return {
        "targets": [{"label": target.label, "path": str(target.path)} for target in plan.targets],
        "credential_key_files": [str(path) for path in plan.credential_key_files],
        "external_credential_file_preserved": (
            str(plan.excluded_credential_file) if plan.excluded_credential_file else None
        ),
        "running_daemon_pid": plan.running_daemon_pid,
        "package_removal": (
            {"manager": plan.package_removal.manager, "command": list(plan.package_removal.command)}
            if plan.package_removal
            else None
        ),
        "keep_program": keep_program,
        "google_data_unchanged": True,
    }


def _uninstall_plan_human(data: dict[str, object]) -> str:
    lines = ["HCB uninstall plan:"]
    for target in cast(list[dict[str, str]], data["targets"]):
        lines.append(f"- remove {target['label']}: {target['path']}")
    key_files = cast(list[str], data["credential_key_files"])
    if key_files:
        lines.append(f"- remove {len(key_files)} credential encryption key(s) from the OS keyring")
    preserved = cast(str | None, data["external_credential_file_preserved"])
    if preserved:
        lines.append(f"- preserve external credential file: {preserved}")
    package = cast(dict[str, object] | None, data["package_removal"])
    if package:
        lines.append(f"- remove the HCB executable through {package['manager']}")
    elif data["keep_program"]:
        lines.append("- keep the HCB executable by request")
    else:
        lines.append("- package manager could not be identified")
    lines.append("- Google Tasks, Calendar, and Drive data are not changed")
    return "\n".join(lines)


def _uninstall_result_human(value: dict[str, object]) -> str:
    package_status = value["package_removal"]
    if package_status == "removed":
        return "HCB has been removed. Google data was not changed."
    if package_status == "scheduled":
        log_path = value["deferred_log"]
        return f"HCB data has been removed; executable removal is scheduled. Log: {log_path}"
    return "HCB data has been removed; the executable was kept by request."


@app.command("uninstall")
def uninstall(
    ctx: typer.Context,
    yes: bool = typer.Option(False, "--yes", "-y", help="Skip the typed UNINSTALL confirmation."),
    dry_run: bool = typer.Option(False, "--dry-run", help="Show the exact teardown plan only."),
    package_manager: str = typer.Option(
        "auto",
        "--package-manager",
        help="auto, uv, or pipx; use this when auto detection is unavailable.",
    ),
    keep_program: bool = typer.Option(
        False, "--keep-program", help="Remove HCB data but leave the installed executable in place."
    ),
    include_env_file: bool = typer.Option(
        False,
        "--include-env-file",
        help="Also remove an explicit --env-file or HCB_ENV_FILE outside HCB's config directory.",
    ),
    no_animate: bool = typer.Option(
        False, "--no-animate", help="Disable the compact terminal removal animation."
    ),
) -> None:
    """Remove HCB-owned local state, credentials, reminders, and its managed executable."""

    state = _state(ctx)
    explicit_credential_file = bool(
        state.runtime.credential_file_override or state.runtime.environ.get("HCB_ENV_FILE")
    )
    package = None if keep_program else package_removal(package_manager)
    plan = build_plan(
        state.runtime.paths,
        active_credential_file=state.runtime.credential_file("uninstall"),
        credential_file_is_explicit=explicit_credential_file,
        include_explicit_credential_file=include_env_file,
        package=package,
    )
    plan_data = _uninstall_plan_data(plan, keep_program=keep_program)
    if dry_run:
        _emit(ctx, plan_data, human=_uninstall_plan_human)
        return
    if not keep_program and package is None:
        if not state.json:
            click.echo(_uninstall_plan_human(plan_data), err=True)
        raise ValueError(
            "HCB could not identify its package manager; rerun with --package-manager uv or "
            "--package-manager pipx, or use --keep-program"
        )
    if plan.running_daemon_pid is not None and sys.platform != "darwin":
        raise ValueError(
            f"HCB reminders appear to be running (pid {plan.running_daemon_pid}); "
            "stop that daemon first"
        )
    _confirm_uninstall(yes)
    state.runtime.close()
    result = execute_plan(
        plan,
        progress=RemovalProgress(enabled=not state.json, animate=not no_animate),
        keyring_store=state.runtime.keyring_store,
    )
    _emit(
        ctx,
        {
            "removed": list(result.removed),
            "skipped": list(result.skipped),
            "package_removal": result.package_removal,
            "deferred_log": str(result.deferred_log) if result.deferred_log else None,
            "google_data_unchanged": True,
        },
        human=_uninstall_result_human,
    )


@daemon_app.command("status")
def daemon_status(ctx: typer.Context) -> None:
    state = DaemonState(_state(ctx).runtime.paths.cache_dir)
    running = False
    pid: int | None = None
    if state.pid_file.exists():
        try:
            pid = int(state.pid_file.read_text(encoding="ascii"))
            os.kill(pid, 0)
            running = True
        except (OSError, ValueError):
            pass
    detail: dict[str, Any] = {}
    if state.status_file.exists():
        try:
            loaded = json.loads(state.status_file.read_text(encoding="utf-8"))
            if isinstance(loaded, dict):
                detail = {key: loaded[key] for key in ("status", "updated_at") if key in loaded}
        except (OSError, json.JSONDecodeError):
            detail = {"status": "invalid"}
    _emit(
        ctx,
        {"installed": _launch_agent_path().exists(), "running": running, "pid": pid, **detail},
        human=lambda value: (
            f"{'running' if value['running'] else 'stopped'}"
            f"{' (installed)' if value['installed'] else ''}"
        ),
    )


@daemon_app.command("install")
def daemon_install(ctx: typer.Context) -> None:
    if sys.platform != "darwin":
        raise HcbError("LaunchAgent installation is only available on macOS")
    import subprocess

    target = _launch_agent_path()
    target.parent.mkdir(parents=True, exist_ok=True)
    payload = {
        "Label": "com.hot-cross-buns.reminderd",
        "ProgramArguments": [sys.executable, "-m", "hcb.cli", "daemon", "run"],
        "RunAtLoad": True,
        "KeepAlive": True,
        "ProcessType": "Background",
        "StandardOutPath": str(Path.home() / "Library/Logs/hcb-reminderd.log"),
        "StandardErrorPath": str(Path.home() / "Library/Logs/hcb-reminderd.log"),
    }
    temporary = target.with_suffix(".tmp")
    temporary.write_bytes(plistlib.dumps(payload))
    temporary.replace(target)
    subprocess.run(
        ["launchctl", "bootstrap", f"gui/{os.getuid()}", str(target)],
        check=False,
        capture_output=True,
    )
    _emit(ctx, {"path": str(target), "installed": True}, human=lambda value: value["path"])


def _launch_agent_path() -> Path:
    return Path.home() / "Library/LaunchAgents/com.hot-cross-buns.reminderd.plist"


@daemon_app.command("uninstall")
def daemon_uninstall(ctx: typer.Context) -> None:
    if sys.platform != "darwin":
        raise HcbError("LaunchAgent installation is only available on macOS")
    import subprocess

    target = _launch_agent_path()
    subprocess.run(
        ["launchctl", "bootout", f"gui/{os.getuid()}", str(target)],
        check=False,
        capture_output=True,
    )
    target.unlink(missing_ok=True)
    _emit(ctx, {"uninstalled": True}, human=lambda _value: "uninstalled")


@daemon_app.command("run")
def daemon_run(
    ctx: typer.Context,
    once: bool = typer.Option(False, "--once", help="Scan and deliver once, then exit."),
) -> None:
    runtime = _state(ctx).runtime
    account = _account(ctx)
    preferences = runtime.config.preferences
    if not preferences.reminders_enabled:
        raise HcbError("local reminders are disabled in preferences")
    scheduler = ReminderScheduler(
        runtime.storage,
        default_notifier(),
        catch_up=timedelta(minutes=preferences.reminder_catch_up_minutes),
    )
    state = DaemonState(runtime.paths.cache_dir)
    state.write("running")
    try:
        if once:
            result = scheduler.run_once(account)
            state.write("idle")
            _emit(ctx, result)
            return
        sync_callback: Callable[[], object] | None = None
        if preferences.reminder_sync_interval_minutes and preferences.reminder_sync_mode != "off":
            engine = runtime.sync_engine(account)
            if preferences.reminder_sync_mode == "pull":

                def pull_sync() -> object:
                    return engine.pull_only(account)

                sync_callback = pull_sync
            else:

                def full_sync() -> object:
                    return engine.sync(account)

                sync_callback = full_sync
        run_loop(
            scheduler,
            account,
            interval=preferences.reminder_poll_seconds,
            jitter=preferences.reminder_jitter_seconds,
            sync=sync_callback,
            sync_interval=preferences.reminder_sync_interval_minutes * 60,
        )
    except KeyboardInterrupt:
        state.write("stopped")
    finally:
        state.clear()


def main() -> None:
    if len(sys.argv) == 1:
        if sys.stdin.isatty() and sys.stdout.isatty():
            from .tui import run_tui

            run_tui(_runtime_factory())
            return
        typer.echo(
            "HCB's interactive workspace requires a TTY. Run `hcb --help` for scriptable commands.",
            err=True,
        )
        return
    app()


if __name__ == "__main__":
    main()
