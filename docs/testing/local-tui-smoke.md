# Local Python TUI/CLI smoke

Use this procedure on a candidate wheel with networking disabled. It complements,
but does not replace, the automated suite or the live Google procedure.

## Isolated install

1. Build with `python -m build`.
2. Create a new virtual environment outside the repository.
3. Install only the generated wheel with `python -m pip install dist/*.whl`.
4. Run `hcb --json-schema-version`, `hcb --help`, and `TERM=dumb NO_COLOR=1 hcb --help`.
   The schema version must be `1`; help must contain no escape sequences.

## Fresh and cached startup

1. Point `XDG_CONFIG_HOME`, `XDG_DATA_HOME`, and `XDG_CACHE_HOME` at an empty
   temporary directory and start `hcb`.
2. Confirm first-run setup says Google is optional until an explicit connection,
   asks for an optional local credential `.env` path, account, IANA timezone,
   and reminders. Do not enter OAuth values into `config.json` or the repository.
3. Save an offline account, create a task list, task, calendar, and event using the
   CLI, then restart the TUI with the network disabled. Confirm cached rows appear.
4. Switch Tasks, Notes, Agenda, Day, Week, and Month. Exercise create, edit,
   complete, delete confirmation, scheduling, undo/redo, import preview, settings,
   conflicts, calendar management, and command-palette actions.

## Terminal behavior

1. Resize from at least 120x38 to 44x18 and back. The sidebar must collapse and
   restore, selection must remain stable, and long titles must remain horizontally
   scroll-reachable.
2. Repeat with CJK text and emoji in selected titles.
3. Run with `NO_COLOR=1` and with the terminal profile. Status must remain understandable
   without color, borders must be ASCII, and mouse use must be optional.
4. Pipe JSON and TSV output to files. No full-screen TUI or ANSI sequence may open
   in a pipe. TSV rows must retain a fixed column count.

## Persistence and diagnostics

1. Queue offline creates, quit, reopen, and confirm the pending count and records
   remain. Undo an unpushed create and confirm both row and mutation disappear.
2. Run `hcb doctor --json`. It may contain counts, enums, schema/integrity state,
   and redacted paths only—never email, subject, titles, notes, OAuth material, or
   local absolute paths.
3. Disconnect and confirm cache remains. Run the separately confirmed reset command
   and confirm account cache and credentials are removed.
4. Run `hcb config init`, edit a semantic color in `config.json`, and confirm the
   running TUI reloads it. Then introduce an unknown token and confirm the TUI keeps
   the previous appearance while reporting the local configuration error.

## Automated scale check

Run `make benchmark`. The default deterministic fixture contains 10,000 tasks,
2,000 events, and 250 queued writes. The report includes repeated CLI startup,
search, workspace, and local outbox timings with median and p95 values, plus
one-shot database and calendar-layout timings. See the
[performance baseline](performance-baseline.md) for the measured results and
their limits. The broad regression tripwires in `tests/test_performance.py`
are intended to be stable on shared CI, not to advertise latency.

## Calendar terminal mouse smoke

Run `make calendar-mouse-smoke` on Linux. It starts a real Textual terminal
driver inside a pseudo-terminal, sends raw SGR left-button press/motion/release
input, and verifies that a Day-grid event moves from 09:00 to 13:00. This covers
the terminal input parser and captured drag path, unlike unit tests that post
`MouseDown` and `MouseMove` objects directly.

For a physical-pointer check in each supported terminal emulator, open Day view,
drag an event by its centre from 09:00 to 13:00, then resize it from its lower
edge. Confirm the live target preview follows the pointer, an invalid target is
shown as invalid, and the persisted event has the expected start and end times.
