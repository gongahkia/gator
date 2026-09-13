# Hot Cross Buns

Hot Cross Buns (`hcb`) is a local-first terminal interface and scriptable CLI
for Google Tasks and Google Calendar. It keeps an SQLite mirror on your device,
queues offline changes, and keeps a local Google credential file on the local
machine.

## Install

HCB is currently distributed from its canonical Git source, not a package
registry. The bootstrapper uses an isolated `uv` environment when available,
or `pipx` as a fallback; the installed application still requires Python 3.12
or newer. See the [full installation guide](docs/installation.md) for security,
accessibility, update, removal, and troubleshooting details.

Download the installer, inspect it if desired, then run it:

```sh
curl --proto '=https' --tlsv1.2 -fL \
  https://raw.githubusercontent.com/gongahkia/hot-cross-buns/main/scripts/install-hcb.py \
  -o hcb-install.py
python3 hcb-install.py
```

It shows a compact staged progress card in an interactive terminal,
keeps package-manager output in a temporary log, and prints that log's path on
failure. Use `--no-animate`, `--no-color`, or `NO_COLOR=1` for accessible or
automated environments. Installation does not launch HCB, connect Google, add
credentials, or install the optional reminders daemon.

The installer accepts `--ref BRANCH_OR_COMMIT` to select a canonical Git ref;
use `--source .` to install a local checkout. `--dry-run` shows the exact
package-manager command without changing anything. `--demo` previews the full
terminal animation without invoking a package manager or modifying files.

For development from a source checkout:

```sh
uv sync --extra dev
uv run hcb --help
```

Run `hcb` in a terminal to open the Textual TUI. Run `hcb --help` to discover
scriptable commands.

In a wide terminal, drag the vertical divider to resize the sidebar for the
current session. `Ctrl+Alt+←` / `Ctrl+Alt+→` resize the sidebar.

Day, Week, and Month grids accept keyboard input: arrow keys move the calendar
cursor, `Enter` opens an item or creates one in an empty slot, `c` creates an
event, and `m` moves the selected item to the cursor. For timed events and
scheduled task blocks, `Shift+↑` / `Shift+↓` resize the end and `Ctrl+↑` /
`Ctrl+↓` resize the start. A timed task block is a linked Calendar event, so it
continues to respect Google Tasks' date-only due-date model.

Click a task, note, or event—or select one and press `Enter`—to open its
read-only detail view. Press `e` there to edit it.

The `/` palette is a local, indexed workspace search. It searches task titles
and event summaries by default; `body:QUERY` (or `notes:QUERY`) also searches
task notes, event details, and cached structured metadata. It covers tasks and
notes, task lists, calendars, events, cached Drive files, saved searches, and
conflicts—without contacting Google. Use `type:task`, `type:list`,
`type:event`, `type:calendar`, `type:drive`, `type:saved`, or `type:conflict`
to narrow results.

## Text editing

Every TUI text field supports Slack-style emoji completion: type a colon and an
emoji code such as `:smile` or alias such as `:+1`, then use `↑`/`↓` and `Tab`
or `Enter` to insert the selected emoji. `Escape` dismisses the suggestions.

Press `Ctrl+G` in a text field to edit its value in `nvim` by default. HCB
suspends its terminal UI, passes a private temporary file directly to the
editor command, then restores the changed value. Multiline fields retain their
text; single-line fields join edited lines with spaces. In onboarding, the
credential-path field opens the actual credential file (creating its
owner-only template when needed); the Import path opens its existing CSV, JSON,
or ICS source and requires a fresh preview when you return. Configure the
command and shortcut in `config.json` or through the CLI:

```sh
hcb config set preferences.editor "nvim"
hcb config set keys.external_editor "ctrl+g"
hcb config set preferences.date_time_format friendly
```

`hcb config edit` opens the complete configuration in that editor and validates
it when you close it. `hcb themes edit PATH` similarly opens and validates an
existing standalone theme JSON file without applying it.

Configuration v2 keeps a strict base `config.json` and may overlay one named
profile from `profiles/NAME.json`. Use `hcb config profiles create NAME` and
`hcb config profiles select NAME`, or set `HCB_PROFILE=NAME` for one process.
`hcb config get`, `set`, `set --json`, `unset`, and `explain` make every saved
setting inspectable and scriptable. The terminal Settings screen exposes
appearance, behavior, capture aliases, keymap, layout, profile, and advanced
stylesheet controls. An optional per-profile TCSS file is validated and loaded
after HCB's built-in stylesheet at startup; changing its path requires restart.

`HCB_EDITOR` overrides `preferences.editor` for the current process. It is a
normal environment variable, separate from HCB's account `.env` credential
file.

## Links

Web links in task and note titles or text, event titles and descriptions, and
Google Drive attachments are underlined in the TUI. Click one to open it in
your default browser. HCB opens only `http` and `https` URLs.

`preferences.date_time_format` controls TUI timestamps: `friendly` (the
default, `26 May 2026, 7:23pm`), `friendly_24h` (`26 May 2026, 19:23`), or
`iso`.

## Desktop OAuth setup

1. In Google Cloud Console, create or select a project.
2. Enable the Google Calendar API, Google Tasks API, and Google Drive API.
3. Configure the OAuth consent screen and add your Google account as a test user
   when the app is in testing mode.
4. Create an OAuth client with application type **Desktop app**.
5. Create HCB's owner-only default credential file at `~/.config/hcb/personal.env`.
   Pass `--env-file` or set `HCB_ENV_FILE` to use another local path.

```sh
credential_file="$HOME/.config/hcb/personal.env"
mkdir -p "${credential_file%/*}"
$EDITOR "$credential_file"
# HCB_GOOGLE_CLIENT_ID=...apps.googleusercontent.com
# HCB_GOOGLE_CLIENT_SECRET=...  # optional for a Desktop client
chmod 600 "$credential_file"

hcb --env-file "$credential_file" auth connect personal you@example.com
hcb sync
hcb
```

The file is never read from the repository or `config.json`. On first
connection HCB adds an encrypted refresh-token value to that same file; its
per-file encryption key remains in the operating-system keyring. HCB rejects a
credential file that group or other users can read or write. Do not commit it.

## CLI examples

```sh
hcb task-lists list
hcb tasks create "Book dentist" --list LIST_ID --due 2026-08-31
hcb events agenda --from 2026-08-21 --to 2026-08-28
hcb events create "Standup" --calendar CAL_ID --start 2026-08-24T09:00:00+08:00 --end 2026-08-24T09:15:00+08:00 --rrule 'FREQ=WEEKLY;BYDAY=MO'
hcb events refresh-instances --calendar CAL_ID --from 2026-08-21 --to 2026-09-21
hcb events instances --calendar CAL_ID --from 2026-08-21 --to 2026-09-21
hcb events instance-cache --calendar CAL_ID
hcb calendars set-list CAL_ID --color '#4285f4' --selected
hcb capture "Submit report tomorrow"
hcb search "report"
hcb --json tasks list
hcb schema list
hcb schema show events.instances
hcb config schema
hcb themes list
hcb themes apply "Catppuccin Mocha"
hcb export --format json --output hcb-export.json
hcb doctor
```

Use `--account`, `HCB_ACCOUNT`, or `preferences.default_account_id` when more
than one account is configured.

## Machine-readable output

Every successful `--json` command emits a versioned envelope with
`schema_version`, `command`, and `data`. Expected `--json` failures emit the
same version and command plus `error.code`, `error.message`, `error.hint`, and
`error.exit_code`, then return a non-zero process status. `hcb --json-schema-version`
remains a scalar `1` for lightweight capability checks.

HCB bundles its Draft 2020-12 contract in the installed package. Use
`hcb schema list` to enumerate covered commands and `hcb schema show COMMAND`
to print a self-contained schema for one command. The schema is intended for
shell and program integrations; the human and TSV output formats are separate
interfaces.

`hcb config init` writes the complete strict JSON configuration to the path from
`hcb config path`. `hcb config schema` prints its Draft 2020-12 schema. The TUI
uses its semantic theme tokens and reloads valid visual changes from that file;
invalid edits leave the active appearance in place and report an error.

## Themes

HCB ships 100 Ghostty-derived visual presets, selected from Ghostty's bundled
theme collection using family-level community adoption and recurrent terminal
theme curation. `hcb themes list` shows the stable ranked set, `hcb themes show
NAME` prints every semantic token, and `hcb themes apply NAME` writes it to
`config.json`. `hcb themes RANK` is a shorthand that applies that numbered
preset, so `hcb themes 20` applies Rose Pine. The running TUI notices that edit
and reloads it.

On first-run onboarding, HCB identifies the local platform, architecture, and
terminal, then checks the active terminal's standard local configuration paths
for a named theme. Ghostty, Windows Terminal, WezTerm, and Kitty are currently
supported. A matching bundled palette is offered as an explicit choice; **Keep
terminal defaults** remains selected unless the user chooses otherwise. This
read-only check never changes the terminal's configuration or connects to
Google.

The Settings dialog also offers every bundled palette, including **Use detected
NAME** when HCB finds a matching local terminal theme. Choosing a palette updates
the pending profile and semantic colors; it takes effect only after **Save**.

For a fully custom appearance, apply a strict standalone theme JSON document:

```sh
hcb themes apply --file my-theme.json
```

The file accepts every visual input field: `profile`, `density`, `borders`,
`focus`, `mouse`, `loader`, and all 14 semantic `colors` tokens. The Settings
dialog includes a searchable picker for all 56 [Rattles](https://github.com/vyfor/rattles)
loaders; `hcb config set theme.loader braille.dots` selects one noninteractively.
The selected loader is used consistently while HCB is connecting, syncing,
refreshing recurring instances, or querying Google free/busy.
`hcb config set theme.colors.TOKEN VALUE` is useful for a single override and
clears the preset provenance label. The complete Ghostty source inventory,
selection method, and pinned upstream revision are in
[the bundled-theme inventory](docs/architecture/ghostty-bundled-themes.md).

## Privacy and local data

Google remains authoritative for synchronized tasks and calendars. HCB stores
its SQLite mirror, mutation queue, sync checkpoints, configuration, and reminder
state in platform-appropriate user data directories. `hcb config path` prints
the configuration path. Refresh tokens are encrypted in the owner-only account
environment file; only the encryption key is stored through the OS keyring.
Access tokens stay in memory. HCB does not operate a web service or cloud backend.

`hcb auth disconnect` removes local credentials and keeps cached data.
`hcb auth reset --yes` also permanently removes that account's local data.

## Reminders and daemon

The reminder process is opt-in and independent of the TUI:

```sh
hcb config set preferences.reminders_enabled true
hcb daemon run --once
hcb daemon install   # macOS LaunchAgent
hcb daemon status
hcb daemon uninstall
```

Opening the TUI never installs a daemon. Automatic LaunchAgent installation is
currently available only on macOS; `hcb daemon run` can be supervised manually
on other platforms.

## Development

```sh
uv sync --extra dev
make format
make lint
make test
make build
make benchmark
```

See [the development checklist](CHECKLIST.md) for the authoritative roadmap,
[the documentation index](docs/README.md), the
[local smoke test](docs/testing/local-tui-smoke.md), and the
[live Google smoke procedure](docs/testing/live-google-tui-smoke.md).

## Acceptance status

The live Google acceptance gate for the Python pivot was explicitly waived and
was not executed before the legacy Qt, web, and self-hosted products were
retired. Automated and local TUI checks do not substitute for that live-account
acceptance. Run the documented live smoke procedure before treating Google
integration as release-accepted.
