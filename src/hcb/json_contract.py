"""Versioned machine-output envelopes and the bundled JSON Schema contract."""

from __future__ import annotations

import json
from importlib.resources import files
from typing import Any, cast

JSON_SCHEMA_VERSION = 1
SCHEMA_RESOURCE = "hcb-json-v1.schema.json"

# This registry is deliberately explicit. Contract tests compare it with the
# Click command tree, so adding a public command requires assigning it a JSON
# contract before release.
JSON_COMMANDS = (
    "auth.connect",
    "auth.disconnect",
    "auth.reset",
    "auth.status",
    "calendars.colors",
    "calendars.create",
    "calendars.delete",
    "calendars.edit",
    "calendars.list",
    "calendars.remove",
    "calendars.set-list",
    "calendars.subscribe",
    "capture",
    "config.edit",
    "config.explain",
    "config.get",
    "config.init",
    "config.path",
    "config.profiles.create",
    "config.profiles.delete",
    "config.profiles.list",
    "config.profiles.select",
    "config.schema",
    "config.set",
    "config.show",
    "config.unset",
    "conflicts.list",
    "conflicts.resolve",
    "conflicts.resolve-delivery",
    "bridge.serve",
    "daemon.install",
    "daemon.run",
    "daemon.status",
    "daemon.uninstall",
    "doctor",
    "drive.list",
    "drive.search",
    "events.agenda",
    "events.create",
    "events.delete",
    "events.delete-many",
    "events.duplicate",
    "events.edit",
    "events.instance-cache",
    "events.instances",
    "events.invitations",
    "events.invite",
    "events.move",
    "events.move-many",
    "events.refresh-instances",
    "events.respond",
    "events.respond-many",
    "events.set-properties",
    "events.show",
    "events.split",
    "export",
    "find-time",
    "freebusy",
    "import.apply",
    "import.preview",
    "notes.clear",
    "notes.list",
    "notes.mode",
    "notes.set",
    "notes.show",
    "redo",
    "saved-searches.delete",
    "saved-searches.list",
    "saved-searches.run",
    "saved-searches.save",
    "schema.list",
    "schema.show",
    "search",
    "sync",
    "task-lists.create",
    "task-lists.delete",
    "task-lists.edit",
    "task-lists.list",
    "tasks.complete",
    "tasks.complete-many",
    "tasks.create",
    "tasks.delete",
    "tasks.delete-many",
    "tasks.edit",
    "tasks.list",
    "tasks.move",
    "tasks.move-many",
    "tasks.reconcile-recurrence",
    "tasks.repair-schedule",
    "tasks.schedule",
    "tasks.unschedule",
    "themes.apply",
    "themes.edit",
    "themes.list",
    "themes.show",
    "uninstall",
    "undo",
)


def command_name(ctx: Any) -> str:
    """Return the stable dotted command name for a Click command context."""
    path = str(ctx.command_path).split()
    if path:
        path = path[1:]
    invoked = getattr(ctx, "invoked_subcommand", None)
    if invoked and (not path or path[-1] != invoked):
        path.append(invoked)
    return ".".join(path) or "root"


def success(command: str, data: Any) -> dict[str, Any]:
    return {"schema_version": JSON_SCHEMA_VERSION, "command": command, "data": data}


def error(
    command: str, *, code: str, message: str, hint: str | None, exit_code: int
) -> dict[str, Any]:
    return {
        "schema_version": JSON_SCHEMA_VERSION,
        "command": command,
        "error": {"code": code, "message": message, "hint": hint, "exit_code": exit_code},
    }


def bundle() -> dict[str, Any]:
    document = json.loads(
        files("hcb.schemas").joinpath(SCHEMA_RESOURCE).read_text(encoding="utf-8")
    )
    if not isinstance(document, dict):
        raise RuntimeError("bundled JSON schema must be an object")
    return cast(dict[str, Any], document)


def command_schema(command: str) -> dict[str, Any]:
    if command not in JSON_COMMANDS:
        raise ValueError(f"unknown JSON command {command!r}; run `hcb schema list`")
    document = bundle()
    return {
        "$schema": "https://json-schema.org/draft/2020-12/schema",
        "$id": f"https://hot-cross-buns.dev/schemas/json/v1/{command}.schema.json",
        "title": f"HCB JSON v1: {command}",
        "$defs": document["$defs"],
        "allOf": [
            {"$ref": "#/$defs/successEnvelope"},
            {
                "properties": {
                    "command": {"const": command},
                    "data": {"$ref": "#/$defs/jsonValue"},
                }
            },
        ],
    }
