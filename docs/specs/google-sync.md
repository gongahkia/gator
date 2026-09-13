# Google sync specification

## Authentication

HCB uses a user-supplied Google OAuth **Desktop app** client, PKCE, and a
temporary `127.0.0.1` callback. Each account reads `HCB_GOOGLE_CLIENT_ID` and
an optional `HCB_GOOGLE_CLIENT_SECRET` from an owner-only local `.env` file.
The default file is `~/.config/hcb/personal.env`; `--env-file` and
`HCB_ENV_FILE` are process-level overrides.
Refresh tokens are encrypted in that file with a per-file Fernet key in the OS
keyring; access tokens remain in memory.

Scopes cover identity/email, Google Tasks, Google Calendar, and read-only Drive
metadata. Drive access is used only for user-initiated metadata search. Tokens
must never be stored in SQLite, configuration, output, logs, or diagnostics.

## Local-first flow

Google is authoritative for synchronized fields. Local mutations validate and
update SQLite optimistically, enter a durable outbox, and are delivered in
creation order before remote changes are pulled. Retryable transport,
rate-limit, and server failures retain queued work with backoff and jitter.
Conflicting or uncertain deliveries remain visible for explicit recovery.

Google Tasks synchronization uses per-list update watermarks with an overlap to
avoid boundary loss. Calendar synchronization saves pages transactionally and
stores `nextSyncToken` only after the final committed page. A Calendar
`410 Gone` clears only that calendar's mirror/checkpoint and starts a full pull.

Updates and deletes use entity tags where available. `409`, `410`, and `412`
mutation outcomes become recoverable conflicts rather than silent data loss.

## Mapping rules

Google Tasks backs task lists, title, notes, date-only due value, completion,
hierarchy, order, and deletion state. Local-only metadata includes search,
presentation state, recurrence markers, mutation state, and links to timed
Calendar blocks.

Google Calendar backs calendars, CalendarList presentation/preferences, events,
all-day/timed values, recurrence lines, resolved instances, reminders,
attendees, status, location, and description. HCB stores recurrence masters
unchanged. A named `events refresh-instances` operation uses Google's expanded
event listing for a requested range and persists only recurring instances as
derived rows. Range views read this cache; HCB never implements recurrence
expansion itself.

## Process model and validation

`hcb sync` is the explicit synchronization boundary. The optional reminder
process can perform configured periodic pull or full sync. The TUI and daemon
may coexist through SQLite WAL and bounded transactions. Remote sync, the
reminder process's pull-only sync, outbox recovery/delivery, and explicit
recurring-instance refresh hold a nonblocking advisory lock for the local
database. A competing sync reports that another sync is active before it
inspects `sending` rows; local reads and edits do not acquire this lock. The
lock file remains beside SQLite, and the operating system releases the held
lock when its owning process exits. This mechanism currently requires a local
filesystem with POSIX advisory locking. Desktop-process and macOS coexistence
still require acceptance testing.

Automated tests use fake transports for success and API failures. The live
Google TUI smoke procedure is separate. It was explicitly waived and not run
for the Python retirement pivot, so no live acceptance claim should be inferred
from the automated suite.
