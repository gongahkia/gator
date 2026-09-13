from __future__ import annotations

import json
from dataclasses import asdict
from pathlib import Path
from subprocess import CompletedProcess
from typing import Any

import pytest
import typer.completion
from typer.testing import CliRunner

from hcb import cli
from hcb.config import Theme, ThemeColors
from hcb.models import Account, Conflict, EntityType
from hcb.paths import AppPaths
from hcb.runtime import Runtime
from hcb.storage import Storage
from hcb.sync import SyncResult


class MemoryTokens:
    def __init__(self) -> None:
        self.values: dict[str, str] = {}

    def get(self, account_id: str) -> str | None:
        return self.values.get(account_id)

    def set(self, account_id: str, token: str) -> None:
        self.values[account_id] = token

    def delete(self, account_id: str) -> bool:
        return self.values.pop(account_id, None) is not None


@pytest.fixture
def cli_env(tmp_path: Path, monkeypatch: pytest.MonkeyPatch) -> tuple[CliRunner, AppPaths]:
    paths = AppPaths(tmp_path / "config", tmp_path / "data", tmp_path / "cache")
    tokens = MemoryTokens()
    monkeypatch.setattr(
        cli,
        "_runtime_factory",
        lambda: Runtime(
            paths,
            environ={},
            token_store=tokens,  # type: ignore[arg-type]
            credential_file=tmp_path / "google.env",
        ),
    )
    with Storage(paths.database_file) as storage:
        storage.upsert_account(Account("work", "me@example.com"))
    return CliRunner(), paths


def invoke(runner: CliRunner, args: list[str], input: str | None = None) -> Any:
    result = runner.invoke(cli.app, args, input=input)
    assert result.exception is None, result.output
    return result


def json_data(result: Any, command: str | None = None) -> Any:
    envelope = json.loads(result.stdout)
    assert envelope["schema_version"] == 1
    if command is not None:
        assert envelope["command"] == command
    return envelope["data"]


def seed_list(runner: CliRunner) -> str:
    result = invoke(runner, ["--json", "task-lists", "create", "Inbox"])
    return str(json_data(result, "task-lists.create")["id"])


def seed_calendar(runner: CliRunner) -> str:
    result = invoke(runner, ["--json", "calendars", "create", "Personal"])
    return str(json_data(result, "calendars.create")["id"])


def test_help_has_completion_and_full_command_groups(
    cli_env: tuple[CliRunner, AppPaths], monkeypatch: pytest.MonkeyPatch
) -> None:
    runner, _ = cli_env
    result = invoke(runner, ["--help"])
    for command in (
        "tasks",
        "task-lists",
        "notes",
        "events",
        "calendars",
        "saved-searches",
        "conflicts",
        "import",
        "export",
        "auth",
        "config",
        "themes",
        "doctor",
        "uninstall",
        "find-time",
        "daemon",
    ):
        assert command in result.stdout
    monkeypatch.setattr(typer.completion, "_get_shell_name", lambda: "bash")
    completion_option = runner.invoke(
        cli.app,
        ["--show-completion"],
        prog_name="hcb",
    )
    assert completion_option.exit_code == 0, completion_option.output
    assert "_hcb_completion" in completion_option.stdout
    completion = runner.invoke(
        cli.app,
        [],
        prog_name="hcb",
        env={"_HCB_COMPLETE": "source_bash"},
    )
    assert completion.exit_code == 0
    assert "_hcb_completion" in completion.stdout


def test_task_crud_json_tsv_and_notes(cli_env: tuple[CliRunner, AppPaths]) -> None:
    runner, _ = cli_env
    list_id = seed_list(runner)
    created = invoke(
        runner,
        [
            "--json",
            "tasks",
            "create",
            "Ship CLI",
            "--list",
            list_id,
            "--due",
            "2026-08-22",
            "--priority",
            "high",
        ],
    )
    task = json_data(created, "tasks.create")
    assert task["title"] == "Ship CLI"
    assert task["due"] == "2026-08-22"
    invoke(runner, ["notes", "set", task["id"], "-"], input="from stdin\n")
    shown = invoke(runner, ["--json", "notes", "show", task["id"]])
    assert json_data(shown, "notes.show")["notes"] == "from stdin\n"
    invoke(runner, ["tasks", "complete", task["id"]])
    listed = invoke(runner, ["--tsv", "tasks", "list", "--completed"])
    assert listed.stdout.startswith("id\tlist_id\ttitle\tstatus")
    assert "Ship CLI" in listed.stdout
    invoke(runner, ["tasks", "delete", task["id"], "--yes"])
    empty = invoke(runner, ["--json", "tasks", "list", "--completed"])
    assert json_data(empty, "tasks.list") == []


def test_destructive_commands_require_yes_when_not_tty(
    cli_env: tuple[CliRunner, AppPaths],
) -> None:
    runner, _ = cli_env
    list_id = seed_list(runner)
    result = runner.invoke(cli.app, ["task-lists", "delete", list_id])
    assert result.exit_code == 2
    assert "--yes" in result.stderr
    assert "Traceback" not in result.output


def test_batch_moves_show_a_complete_plan_and_require_confirmation(
    cli_env: tuple[CliRunner, AppPaths],
) -> None:
    runner, _ = cli_env
    inbox = seed_list(runner)
    archive = json_data(
        invoke(runner, ["--json", "task-lists", "create", "Archive"]), "task-lists.create"
    )["id"]
    task_ids = [
        json_data(
            invoke(runner, ["--json", "tasks", "create", title, "--list", inbox]),
            "tasks.create",
        )["id"]
        for title in ("First", "Second")
    ]
    denied = runner.invoke(cli.app, ["tasks", "move-many", *task_ids, "--list", str(archive)])
    assert denied.exit_code == 2
    assert "First" in denied.stderr and "Second" in denied.stderr
    assert "--yes" in denied.stderr

    moved = json_data(
        invoke(
            runner,
            ["--json", "tasks", "move-many", *task_ids, "--list", str(archive), "--yes"],
        ),
        "tasks.move-many",
    )
    assert [item["id"] for item in moved] == task_ids
    assert all(item["list_id"] == archive for item in moved)

    calendar = seed_calendar(runner)
    calendar_archive = json_data(
        invoke(runner, ["--json", "calendars", "create", "Calendar archive"]),
        "calendars.create",
    )["id"]
    event = json_data(
        invoke(
            runner,
            [
                "--json",
                "events",
                "create",
                "Move event",
                "--calendar",
                calendar,
                "--start",
                "2026-08-22",
                "--end",
                "2026-08-23",
                "--all-day",
            ],
        ),
        "events.create",
    )
    event_denied = runner.invoke(
        cli.app, ["events", "move-many", event["id"], "--calendar", str(calendar_archive)]
    )
    assert event_denied.exit_code == 2
    assert "Move event" in event_denied.stderr and "--yes" in event_denied.stderr
    event_moved = json_data(
        invoke(
            runner,
            [
                "--json",
                "events",
                "move-many",
                event["id"],
                "--calendar",
                str(calendar_archive),
                "--yes",
            ],
        ),
        "events.move-many",
    )
    assert event_moved[0]["calendar_id"] == calendar_archive


def test_batch_actions_show_exact_preflights_before_applying(
    cli_env: tuple[CliRunner, AppPaths],
) -> None:
    runner, _ = cli_env
    task_list = seed_list(runner)
    task_ids = [
        json_data(
            invoke(runner, ["--json", "tasks", "create", title, "--list", task_list]),
            "tasks.create",
        )["id"]
        for title in ("First", "Second")
    ]

    completed = invoke(runner, ["--json", "tasks", "complete-many", *task_ids])
    assert "Preflight: complete 2 task(s):" in completed.stderr
    assert "First" in completed.stderr and "needsAction → completed" in completed.stderr
    assert all(
        item["status"] == "completed" for item in json_data(completed, "tasks.complete-many")
    )

    denied_task_delete = runner.invoke(cli.app, ["tasks", "delete-many", *task_ids])
    assert denied_task_delete.exit_code == 2
    assert "First" in denied_task_delete.stderr and "active → deleted" in denied_task_delete.stderr
    assert "--yes" in denied_task_delete.stderr

    calendar = seed_calendar(runner)
    event = json_data(
        invoke(
            runner,
            [
                "--json",
                "events",
                "create",
                "Review me",
                "--calendar",
                calendar,
                "--start",
                "2026-08-22",
                "--end",
                "2026-08-23",
                "--all-day",
            ],
        ),
        "events.create",
    )
    responded = invoke(
        runner,
        ["--json", "events", "respond-many", event["id"], "--response", "accepted"],
    )
    assert "Preflight: RSVP accepted for 1 event(s):" in responded.stderr
    assert "Review me" in responded.stderr and "needsAction → accepted" in responded.stderr

    denied_event_delete = runner.invoke(cli.app, ["events", "delete-many", event["id"]])
    assert denied_event_delete.exit_code == 2
    assert (
        "Review me" in denied_event_delete.stderr
        and "active → deleted" in denied_event_delete.stderr
    )
    assert "--yes" in denied_event_delete.stderr


def test_events_search_saved_search_and_capture(cli_env: tuple[CliRunner, AppPaths]) -> None:
    runner, _ = cli_env
    list_id = seed_list(runner)
    calendar_id = seed_calendar(runner)
    event = invoke(
        runner,
        [
            "--json",
            "events",
            "create",
            "Planning",
            "--calendar",
            calendar_id,
            "--start",
            "2026-08-22",
            "--end",
            "2026-08-23",
            "--all-day",
        ],
    )
    event_id = json_data(event, "events.create")["id"]
    agenda = invoke(
        runner, ["--json", "events", "agenda", "--from", "2026-08-22", "--to", "2026-08-24"]
    )
    assert json_data(agenda, "events.agenda")[0]["id"] == event_id
    found = invoke(runner, ["--json", "search", "Planning"])
    assert json_data(found, "search")[0]["kind"] == "event"
    saved = invoke(runner, ["--json", "saved-searches", "save", "plan", "Planning"])
    saved_id = json_data(saved, "saved-searches.save")["id"]
    rerun = invoke(runner, ["--json", "saved-searches", "run", saved_id])
    assert json_data(rerun, "saved-searches.run")[0]["id"] == event_id
    captured = invoke(
        runner,
        ["--json", "capture", "-", "--list", list_id],
        input="task Buy milk tomorrow p1\n",
    )
    assert json_data(captured, "capture")["title"] == "Buy milk"


def test_calendar_preferences_and_event_recurrence_cli_parity(
    cli_env: tuple[CliRunner, AppPaths],
) -> None:
    runner, _ = cli_env
    calendar_id = seed_calendar(runner)
    event = json_data(
        invoke(
            runner,
            [
                "--json",
                "events",
                "create",
                "Standup",
                "--calendar",
                calendar_id,
                "--start",
                "2026-08-22T09:00:00+00:00",
                "--end",
                "2026-08-22T09:30:00+00:00",
                "--rrule",
                "FREQ=WEEKLY;BYDAY=MO",
            ],
        ),
        "events.create",
    )
    assert event["recurrence"] == ["RRULE:FREQ=WEEKLY;BYDAY=MO"]
    edited = json_data(
        invoke(
            runner,
            ["--json", "events", "edit", event["id"], "--clear-recurrence"],
        ),
        "events.edit",
    )
    assert edited["recurrence"] == []
    duplicate = json_data(
        invoke(runner, ["--json", "events", "duplicate", event["id"]]), "events.duplicate"
    )
    assert duplicate["id"] != event["id"]
    updated_calendar = json_data(
        invoke(
            runner,
            [
                "--json",
                "calendars",
                "set-list",
                calendar_id,
                "--color",
                "#123456",
                "--unselected",
                "--reminders-json",
                '[{"method":"popup","minutes":10}]',
            ],
        ),
        "calendars.set-list",
    )
    assert updated_calendar["color"] == "#123456"
    assert not updated_calendar["selected"]


def test_instance_cache_cli_reports_coverage_without_network(
    cli_env: tuple[CliRunner, AppPaths],
) -> None:
    runner, _ = cli_env
    calendar_id = seed_calendar(runner)
    instances = json_data(
        invoke(
            runner,
            [
                "--json",
                "events",
                "instances",
                "--calendar",
                calendar_id,
                "--from",
                "2026-08-21",
                "--to",
                "2026-08-28",
            ],
        ),
        "events.instances",
    )
    assert instances["instances"] == []
    assert instances["cache"][0]["state"] == "missing"
    ranges = json_data(
        invoke(runner, ["--json", "events", "instance-cache", "--calendar", calendar_id]),
        "events.instance-cache",
    )
    assert ranges == {"ranges": []}


def test_import_preview_apply_and_export(
    cli_env: tuple[CliRunner, AppPaths], tmp_path: Path
) -> None:
    runner, _ = cli_env
    list_id = seed_list(runner)
    source = tmp_path / "items.json"
    source.write_text(
        json.dumps(
            {
                "version": 1,
                "records": [
                    {"kind": "task", "title": "Imported", "list": list_id, "priority": "none"}
                ],
            }
        )
    )
    preview = invoke(runner, ["--json", "import", "preview", str(source)])
    assert json_data(preview, "import.preview")["rows"][0]["record"]["title"] == "Imported"
    invoke(runner, ["import", "apply", str(source), "--yes"])
    exported = invoke(runner, ["--json", "export", "--format", "json"])
    assert json_data(exported, "export")["content"]["records"][0]["title"] == "Imported"
    csv_output = tmp_path / "out.csv"
    invoke(runner, ["export", "--format", "csv", "--output", str(csv_output)])
    assert csv_output.read_text().startswith("kind,title,")


def test_config_commands_and_account_environment(
    tmp_path: Path, monkeypatch: pytest.MonkeyPatch
) -> None:
    paths = AppPaths(tmp_path / "config", tmp_path / "data", tmp_path / "cache")
    env = {"HCB_ACCOUNT": "second"}
    monkeypatch.setattr(cli, "_runtime_factory", lambda: Runtime(paths, environ=env))
    with Storage(paths.database_file) as storage:
        storage.upsert_account(Account("first", "first@example.com"))
        storage.upsert_account(Account("second", "second@example.com"))
    runner = CliRunner()
    invoke(runner, ["config", "init"])
    assert paths.config_file.name == "config.json"
    invoke(runner, ["config", "set", "preferences.week_starts_on", "6"])
    invoke(runner, ["config", "set", "theme.colors.focus", "cyan"])
    invoke(runner, ["config", "set", "theme.loader", "emoji.weather"])
    invoke(runner, ["config", "set", "preferences.editor", "hx"])
    invoke(runner, ["config", "set", "keys.external_editor", "ctrl+o"])
    shown = invoke(runner, ["--json", "config", "show"])
    assert json_data(shown, "config.show")["preferences"]["week_starts_on"] == 6
    assert json_data(shown, "config.show")["theme"]["loader"] == "emoji.weather"
    assert json_data(shown, "config.show")["preferences"]["editor"] == "hx"
    assert json_data(shown, "config.show")["keys"]["external_editor"] == "ctrl+o"
    assert (
        json_data(invoke(runner, ["--json", "config", "schema"]), "config.schema")["$schema"]
        == "https://json-schema.org/draft/2020-12/schema"
    )
    assert (
        json_data(invoke(runner, ["--json", "config", "show"]), "config.show")["theme"]["colors"][
            "focus"
        ]
        == "cyan"
    )
    invoke(
        runner,
        [
            "config",
            "set",
            "preferences.capture",
            '{"task_aliases":["todo"]}',
            "--json",
        ],
    )
    assert json_data(
        invoke(runner, ["--json", "config", "get", "preferences.capture.task_aliases"]),
        "config.get",
    )["value"] == ["todo"]
    assert (
        json_data(
            invoke(runner, ["--json", "config", "explain", "theme.stylesheet"]),
            "config.explain",
        )["reload"]
        == "restart required"
    )
    invoke(runner, ["config", "profiles", "create", "work"])
    assert (
        json_data(
            invoke(runner, ["--json", "config", "profiles", "select", "work"]),
            "config.profiles.select",
        )["active"]
        == "work"
    )
    assert invoke(runner, ["task-lists", "create", "Second"]).exit_code == 0
    with Storage(paths.database_file) as storage:
        assert len(storage.list_task_lists("second")) == 1
        assert storage.list_task_lists("first") == []


def test_theme_commands_apply_bundled_and_complete_custom_themes(
    cli_env: tuple[CliRunner, AppPaths], tmp_path: Path
) -> None:
    runner, paths = cli_env

    listed = json_data(invoke(runner, ["--json", "themes", "list"]), "themes.list")
    assert len(listed) == 100
    assert listed[0]["rank"] == 1
    assert listed[0]["name"] == "Dracula"
    assert listed[0]["colors"]["background"] == "#282a36"
    assert listed[-1]["rank"] == 100
    assert listed[-1]["name"] == "Xcode Light"

    shown = json_data(
        invoke(runner, ["--json", "themes", "show", "Catppuccin Latte"]), "themes.show"
    )
    assert shown["colors"]["background"] == "#eff1f5"
    applied = json_data(
        invoke(runner, ["--json", "themes", "apply", "Catppuccin Latte"]), "themes.apply"
    )
    assert applied["source"]["rank"] == 5
    assert applied["theme"]["preset"] == "Catppuccin Latte"
    assert json.loads(paths.config_file.read_text())["theme"]["preset"] == "Catppuccin Latte"

    ranked = json_data(invoke(runner, ["--json", "themes", "20"]), "themes.apply")
    assert ranked["source"]["rank"] == 20
    assert ranked["theme"]["preset"] == "Rose Pine"

    latest = json_data(invoke(runner, ["--json", "themes", "100"]), "themes.apply")
    assert latest["source"]["rank"] == 100
    assert latest["theme"]["preset"] == "Xcode Light"

    colors = ThemeColors(**{name: "#123456" for name in ThemeColors.__dataclass_fields__})
    custom = Theme(
        profile="light",
        density="compact",
        borders="unicode",
        focus="reverse",
        mouse=False,
        colors=colors,
    )
    custom_path = tmp_path / "custom-theme.json"
    custom_path.write_text(json.dumps(asdict(custom)), encoding="utf-8")
    loaded = json_data(
        invoke(runner, ["--json", "themes", "apply", "--file", str(custom_path)]),
        "themes.apply",
    )
    assert loaded["source"] == {"kind": "custom", "path": str(custom_path)}
    assert loaded["theme"] == asdict(custom)

    invoke(runner, ["config", "set", "theme.colors.accent", "cyan"])
    assert json.loads(paths.config_file.read_text())["theme"]["preset"] is None


def test_config_and_custom_theme_files_open_in_the_configured_editor(
    cli_env: tuple[CliRunner, AppPaths], tmp_path: Path, monkeypatch: pytest.MonkeyPatch
) -> None:
    runner, paths = cli_env
    commands: list[list[str]] = []
    custom_theme = tmp_path / "custom-theme.json"
    custom_theme.write_text('{"profile":"dark","colors":{"accent":"#aabbcc"}}\n')

    def run_editor(command: list[str], *, check: bool) -> CompletedProcess[str]:
        commands.append(command)
        assert check is False
        return CompletedProcess(command, 0)

    monkeypatch.setattr(cli.subprocess, "run", run_editor)
    invoke(runner, ["config", "set", "preferences.editor", "vim -n"])

    config_result = invoke(runner, ["--json", "config", "edit"])
    assert json_data(config_result, "config.edit") == {
        "path": str(paths.config_file),
        "valid": True,
    }

    theme_result = invoke(runner, ["--json", "themes", "edit", str(custom_theme)])
    assert json_data(theme_result, "themes.edit") == {"path": str(custom_theme), "valid": True}
    assert commands == [
        ["vim", "-n", str(paths.config_file)],
        ["vim", "-n", str(custom_theme)],
    ]


def test_local_reads_do_not_construct_google_or_keyring(
    tmp_path: Path, monkeypatch: pytest.MonkeyPatch
) -> None:
    paths = AppPaths(tmp_path / "config", tmp_path / "data", tmp_path / "cache")
    with Storage(paths.database_file) as storage:
        storage.upsert_account(Account("work", "me@example.com"))

    def explode() -> None:
        raise AssertionError("keyring was constructed")

    monkeypatch.setattr("hcb.runtime.TokenStore", explode)
    monkeypatch.setattr(cli, "_runtime_factory", lambda: Runtime(paths, environ={}))
    result = CliRunner().invoke(cli.app, ["tasks", "list"])
    assert result.exit_code == 0


def test_sync_is_explicit_network_boundary(
    cli_env: tuple[CliRunner, AppPaths], monkeypatch: pytest.MonkeyPatch
) -> None:
    runner, paths = cli_env
    calls: list[str] = []

    class Engine:
        def sync(self, account_id: str, **kwargs: object) -> SyncResult:
            calls.append(account_id)
            return SyncResult(pulled=2, pushed=1)

    class SyncRuntime(Runtime):
        def sync_engine(self, account_id: str) -> Engine:
            return Engine()

    monkeypatch.setattr(
        cli,
        "_runtime_factory",
        lambda: SyncRuntime(paths, environ={}, token_store=MemoryTokens()),  # type: ignore[arg-type]
    )
    result = invoke(runner, ["--json", "sync"])
    assert json_data(result, "sync")["pulled"] == 2
    assert calls == ["work"]


def test_sync_reports_bounded_retry_exhaustion_with_a_resume_hint(
    cli_env: tuple[CliRunner, AppPaths], monkeypatch: pytest.MonkeyPatch
) -> None:
    runner, paths = cli_env

    class Engine:
        def sync(self, account_id: str, **kwargs: object) -> SyncResult:
            return SyncResult(
                retry_pending=True,
                retry_exhausted=True,
                retry_message="Sending local change paused after bounded retries.",
            )

    class SyncRuntime(Runtime):
        def sync_engine(self, account_id: str) -> Engine:
            return Engine()

    monkeypatch.setattr(
        cli,
        "_runtime_factory",
        lambda: SyncRuntime(paths, environ={}, token_store=MemoryTokens()),  # type: ignore[arg-type]
    )
    result = runner.invoke(cli.app, ["sync"])
    assert result.exit_code == 6
    assert "bounded retries" in result.stderr
    assert "hcb sync to resume" in result.stderr


def test_sync_reports_uncertain_delivery_with_reconciliation_hint(
    cli_env: tuple[CliRunner, AppPaths], monkeypatch: pytest.MonkeyPatch
) -> None:
    runner, paths = cli_env

    class Engine:
        def sync(self, account_id: str, **kwargs: object) -> SyncResult:
            return SyncResult(
                retry_pending=True,
                retry_message="Sync paused for uncertain Google delivery.",
            )

    class SyncRuntime(Runtime):
        def sync_engine(self, account_id: str) -> Engine:
            return Engine()

    monkeypatch.setattr(
        cli,
        "_runtime_factory",
        lambda: SyncRuntime(paths, environ={}, token_store=MemoryTokens()),  # type: ignore[arg-type]
    )
    result = runner.invoke(cli.app, ["sync"])
    assert result.exit_code == 4
    assert "uncertain Google delivery" in result.stderr
    assert "hcb conflicts resolve-delivery" in result.stderr


def test_expected_not_found_uses_stable_exit_without_traceback(
    cli_env: tuple[CliRunner, AppPaths],
) -> None:
    runner, _ = cli_env
    result = runner.invoke(cli.app, ["tasks", "complete", "missing"])
    assert result.exit_code == 3
    assert "does not exist" in result.stderr
    assert "Traceback" not in result.output


def test_auth_status_is_local_and_handles_an_offline_account(
    cli_env: tuple[CliRunner, AppPaths],
) -> None:
    runner, _ = cli_env
    status = json_data(invoke(runner, ["--json", "auth", "status"]), "auth.status")
    assert len(status) == 1
    assert status[0]["id"] == "work"
    assert status[0]["authenticated"] is False


def test_advanced_cli_equivalence_for_tui_workflows(
    cli_env: tuple[CliRunner, AppPaths],
) -> None:
    runner, _ = cli_env
    list_id = seed_list(runner)
    calendar_id = seed_calendar(runner)
    task = json_data(
        invoke(runner, ["--json", "tasks", "create", "Block me", "--list", list_id]),
        "tasks.create",
    )
    event = json_data(
        invoke(
            runner,
            [
                "--json",
                "events",
                "create",
                "Specialist",
                "--calendar",
                calendar_id,
                "--start",
                "2026-08-22",
                "--end",
                "2026-08-23",
                "--all-day",
            ],
        ),
        "events.create",
    )
    properties = {
        "eventType": "focusTime",
        "visibility": "private",
        "attendees": [{"email": "guest@example.test"}],
        "focusTimeProperties": {"autoDeclineMode": "declineNone"},
    }
    invoke(
        runner,
        ["events", "set-properties", event["id"], json.dumps(properties)],
    )
    shown = json_data(invoke(runner, ["--json", "events", "show", event["id"]]), "events.show")
    assert shown["event_type"] == "focusTime"
    assert shown["focus_time_properties"]["autoDeclineMode"] == "declineNone"

    invoke(
        runner,
        [
            "tasks",
            "schedule",
            task["id"],
            "--calendar",
            calendar_id,
            "--start",
            "2026-08-22T09:00:00Z",
            "--end",
            "2026-08-22T10:00:00Z",
        ],
    )
    invoke(runner, ["tasks", "unschedule", task["id"], "--yes"])
    invoke(runner, ["tasks", "complete-many", task["id"]])
    invoke(runner, ["undo"])
    invoke(runner, ["redo"])
    invoke(runner, ["events", "delete-many", event["id"], "--yes"])


def test_json_schema_version_task_shape_and_tsv_columns(
    cli_env: tuple[CliRunner, AppPaths],
) -> None:
    runner, _ = cli_env
    assert invoke(runner, ["--json-schema-version"]).stdout.strip() == "1"
    list_id = seed_list(runner)
    task = json_data(
        invoke(runner, ["--json", "tasks", "create", "Schema", "--list", list_id]),
        "tasks.create",
    )
    assert set(task) == {
        "account_id",
        "completed_at",
        "due",
        "due_time_zone",
        "id",
        "list_id",
        "metadata",
        "notes",
        "parent_id",
        "position",
        "priority",
        "remote_id",
        "status",
        "title",
    }
    assert set(task["metadata"]) == {
        "deleted",
        "dirty",
        "etag",
        "local_updated_at",
        "remote_updated_at",
    }
    tsv = invoke(runner, ["--tsv", "tasks", "list"])
    assert tsv.stdout.splitlines()[0] == "id\tlist_id\ttitle\tstatus\tdue\tpriority"
    assert all(len(line.split("\t")) == 6 for line in tsv.stdout.splitlines())


def test_stable_usage_auth_and_not_found_exits_without_ansi_or_traceback(
    tmp_path: Path, monkeypatch: pytest.MonkeyPatch
) -> None:
    paths = AppPaths(tmp_path / "config", tmp_path / "data", tmp_path / "cache")
    monkeypatch.setattr(cli, "_runtime_factory", lambda: Runtime(paths, environ={"TERM": "dumb"}))
    runner = CliRunner()

    usage = runner.invoke(cli.app, ["events", "agenda", "--from", "not-a-date"])
    assert usage.exit_code == 2
    missing_auth = runner.invoke(cli.app, ["tasks", "list"])
    assert missing_auth.exit_code == 5

    with Storage(paths.database_file) as storage:
        storage.upsert_account(Account("work", "redacted@example.test"))
    missing = runner.invoke(cli.app, ["tasks", "complete", "missing"])
    assert missing.exit_code == 3

    for result in (usage, missing_auth, missing):
        assert "\x1b[" not in result.output
        assert "Traceback" not in result.output


def test_cli_resolves_uncertain_delivery_with_explicit_action(
    cli_env: tuple[CliRunner, AppPaths],
) -> None:
    runner, paths = cli_env
    list_id = seed_list(runner)
    task = json_data(
        invoke(runner, ["--json", "tasks", "create", "Uncertain", "--list", list_id]),
        "tasks.create",
    )
    with Storage(paths.database_file) as storage:
        storage.connection.execute(
            "DELETE FROM outbox WHERE account_id=? AND entity_id=?",
            ("work", task["id"]),
        )
        conflict_id = storage.add_conflict(
            Conflict(
                None,
                "work",
                EntityType.TASK,
                task["id"],
                {
                    "kind": "uncertain-delivery",
                    "mutation": {
                        "entity_type": "task",
                        "entity_id": task["id"],
                        "operation": "create",
                        "payload": {
                            "list_id": list_id,
                            "body": {"title": "Uncertain"},
                        },
                        "request_id": None,
                    },
                },
                {"kind": "delivery-status-unknown"},
            )
        )
    invoke(
        runner,
        [
            "conflicts",
            "resolve-delivery",
            str(conflict_id),
            "--action",
            "retry",
            "--yes",
        ],
    )
    with Storage(paths.database_file) as storage:
        retried = [
            item
            for item in storage.pending_mutations("work")
            if item.entity_type is EntityType.TASK and item.entity_id == task["id"]
        ]
        assert len(retried) == 1
        assert storage.list_conflicts("work") == []
