# HCB CLI

Running `hcb` with an interactive terminal opens the TUI. All other entry
points are scriptable:

```sh
hcb --help
hcb COMMAND --help
hcb --account ACCOUNT_ID COMMAND
hcb --json COMMAND
hcb --tsv COMMAND
```

Major command groups are `tasks`, `task-lists`, `notes`, `events`, `calendars`,
`saved-searches`, `conflicts`, `import`, `auth`, `config`, `themes`, `daemon`,
`drive`, and `schema`. Top-level operations include `capture`, `search`, `find-time`,
`freebusy`, `sync`, `export`, `undo`, `redo`, `doctor`, and `uninstall`.

The TUI’s `/` palette uses a local SQLite full-text index and never contacts
Google. It searches tasks/notes, task lists, calendars, events, cached Drive
files, saved searches, and conflicts. Bare terms are title-first; use
`body:QUERY` or `notes:QUERY` for descriptions and notes, and `type:task`,
`type:list`, `type:event`, `type:calendar`, `type:drive`, `type:saved`, or
`type:conflict` to narrow the result type.

Examples:

```sh
hcb auth connect personal you@example.com
hcb sync
hcb tasks list
hcb tasks create "Prepare agenda" --list LIST_ID --due 2026-08-24
hcb tasks complete TASK_ID
hcb events agenda --from 2026-08-21 --to 2026-08-28
hcb events refresh-instances --calendar CAL_ID --from 2026-08-21 --to 2026-09-21
hcb events instance-cache --calendar CAL_ID
hcb events duplicate EVENT_ID --include-attendees
hcb events invite EVENT_ID guest@example.test --send-updates all
hcb calendars set-list CAL_ID --color '#4285f4' --selected
hcb themes list
hcb themes show Dracula
hcb themes apply "Catppuccin Mocha"
hcb themes apply --file ~/.config/hcb/my-theme.json
hcb capture "Call Sam tomorrow"
hcb --json search "Sam"
hcb schema show events.instances
hcb import preview tasks.csv
hcb export --format ics --output calendar.ics
```

`--json` and `--tsv` are mutually exclusive. Human output goes to stdout and
actionable errors go to stderr. Destructive noninteractive commands require
`--yes`. `NO_COLOR` and `TERM=dumb` disable decoration.

`hcb uninstall --dry-run` prints the exact local teardown plan. `hcb uninstall`
requires typing `UNINSTALL` before removing HCB-owned configuration, data,
cache, credentials, known keyring keys, the macOS reminders agent, and the
managed `uv`/`pipx` executable. It never changes Google data. Use `--yes` only
for reviewed automation, `--no-animate` for plain logs, and
`--include-env-file` only when an explicit external credential file should be
deleted too. HCB refuses teardown while a manually supervised reminders daemon
appears active.

All `--json` success output has this envelope:

```json
{
  "schema_version": 1,
  "command": "events.instances",
  "data": {}
}
```

Expected JSON failures use `error` instead of `data`, with stable `code`,
`message`, nullable `hint`, and `exit_code`; they still exit non-zero. The
scalar `hcb --json-schema-version` intentionally remains `1`. `hcb schema list`
lists every public JSON command, and `hcb schema show COMMAND` prints its
self-contained Draft 2020-12 schema. The versioned schema is bundled in both
the wheel and source distribution.

Account selection uses `--account`, `HCB_ACCOUNT`, or
`preferences.default_account_id`; a sole configured account is selected
automatically. OAuth setup requires a local owner-only `.env` file containing
`HCB_GOOGLE_CLIENT_ID` and, when supplied by Google, `HCB_GOOGLE_CLIENT_SECRET`.
The default is `~/.config/hcb/personal.env`; `--env-file PATH` and
`HCB_ENV_FILE` override it for the process. The refresh token is encrypted in
that file and its encryption key is stored in the OS keyring.

`preferences.editor` stores the external editor command used by TUI text
fields (default `nvim`), and `keys.external_editor` stores its shortcut
(default `ctrl+g`). For the current process, `HCB_EDITOR` takes precedence over
`preferences.editor`; it does not use the account credential `.env` file.
For example:

```sh
hcb config set preferences.editor "nvim"
hcb config set keys.external_editor "ctrl+g"
hcb config set preferences.date_time_format friendly
```

`hcb config edit` creates `config.json` when needed, opens it in that editor,
and validates the result after the editor exits. `hcb themes edit PATH` opens
an existing custom theme JSON file and validates it without applying it. In the
TUI, the onboarding credential path and Import path display the configured
shortcut: it opens the actual credential file or existing import source rather
than a temporary text-field buffer. Re-preview an import after returning from
the editor.

Configuration uses schema version 2. `config.json` is the strict base file and
`profiles/NAME.json` is an optional strict overlay selected by `active_profile`
or the process-only `HCB_PROFILE` variable. Use `hcb config profiles list`,
`create`, `select`, and `delete` to manage profile files. `hcb config get KEY`,
`set KEY VALUE`, `set --json KEY JSON`, `unset KEY`, and `explain KEY` expose
the resolved values, defaults, and reload behavior. Profile TCSS stylesheets
are loaded after HCB's base TCSS at startup. Restart HCB after changing a
stylesheet path; an invalid stylesheet falls back to the base UI.

`preferences.date_time_format` accepts `friendly` (default), `friendly_24h`,
or `iso`; it controls the dates and times displayed by the TUI.

`preferences.conflict_policy` accepts `ask` (default), `prefer-google`, or
`prefer-local`; the TUI exposes the same choice in Settings. The policy applies
to stale-ETag (`412`) task and event updates/deletes. `prefer-google` drops the
conflicted local mutation and refreshes that list or calendar from Google.
`prefer-local` retries the mutation without the stale ETag. `ask` leaves an
open conflict for `hcb conflicts` to resolve. Other conflict responses, and
updates with later queued edits to the same entity, still require explicit
resolution. Sync pauses before sending later changes to a conflicted item until
that conflict is resolved. A remote win clears the local dirty flag before
pulling so the Google value can replace the cache. If later edits are queued,
choosing Google is rejected to preserve them; choosing Local requeues the
rejected edit ahead of them.

The Notes projection setting controls where undated root tasks appear. In
`disabled` mode, task notes created or explicitly edited locally are omitted
from Google writes and retained in a local-only record so later pulls and
projection changes do not erase them. A later task write in `notes-only` or
`mirrored` mode includes the note in Google Tasks and clears that local-only
record after Google accepts the write.

TUI text fields offer standard Rich emoji codes and aliases after `:`; use
`↑`/`↓` to select and `Tab` or `Enter` to insert the displayed emoji.

`events agenda`, `events instances`, and `events instance-cache` read SQLite
only. Normal `sync` updates canonical event records; `events refresh-instances
--calendar --from --to` is the explicit remote operation that expands a range
through Google and updates the derived instance cache. `events instances`
returns the queried occurrences together with `fresh`, `partial`, `stale`, or
`missing` cache coverage for the exact range. A local or remote recurring
series/instance change marks every cached range for that calendar stale; only a
named refresh marks coverage fresh again. `events edit` supports `--rrule` / `--recurrence`,
`--clear-recurrence`, `--clear-description`, and `--clear-location`.
`events set-properties` accepts a structured JSON object and honours explicit
JSON `null` and empty arrays as clears. `calendars edit` changes Calendar
resource fields, while `calendars set-list` changes CalendarList settings such
as selection, colors, default reminders, and notification preferences.

`themes list` returns HCB's 100 bundled Ghostty-derived presets in rank order.
`themes show NAME` returns one full semantic palette, and `themes apply NAME`
writes it to `config.json` while retaining the current density, border, focus,
mouse, and loader choices. `themes RANK` is an equivalent shorthand, so
`themes 20` applies Rose Pine. `themes apply --file PATH` accepts a strict standalone
`theme` JSON object: `profile`, `density`, `borders`, `focus`, `mouse`, `loader`, plus a
`colors` object with any or all of `background`, `surface`, `panel`, `overlay`,
`control`, `text`, `muted`, `border`, `focus`, `selection`, `accent`, `success`,
`warning`, and `danger`. Omitted fields use the terminal-minimal defaults.
Unknown fields, duplicate keys, and invalid Textual colors are rejected. A
custom file replaces every supplied visual setting; a bundled preset replaces
the full color palette and profile, but preserves UI layout and interaction
settings. Any direct `hcb config set theme.*` visual edit clears the preset
provenance label; use `hcb themes apply NAME` to select a bundled preset.

The reminder process is explicit:

```sh
hcb daemon run --once
hcb daemon run
hcb daemon install     # macOS only
hcb daemon status
hcb daemon uninstall
```

Use each command's `--help` as the authoritative option reference.
