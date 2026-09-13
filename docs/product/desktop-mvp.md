# Desktop MVP and CLI/TUI parity map

This is the proposed scope for the first windowed Linux and macOS app. The
[development checklist](../../CHECKLIST.md) remains the source of truth for
implementation and release readiness. This map records what the current Python
CLI/TUI exposes and which of those workflows the first desktop app should carry.
It is a scope decision, not a claim that the desktop app exists or that live
Google behavior has passed acceptance.

## Product boundary

The first desktop app is a second frontend of the existing local Python core in
this repository. It uses the same account selection, configuration, credential
store, SQLite database, migrations, and durable outbox as `hcb`. Google remains
authoritative for synchronized data; reads and edits use the local mirror, and
network sync is explicit. The CLI/TUI remains available for workflows deferred
from the first desktop release. A hosted backend, a separate core repository,
and Windows support are outside this MVP.

The desktop app should work with an already configured account while offline.
It should also expose account connection through the existing Desktop OAuth
flow. Bring-your-own credentials are adequate for a prototype; the
distribution credential model is still an open [checklist item](../../CHECKLIST.md).

## Parity map

| Area | Current CLI/TUI | First desktop app | Later than MVP |
| --- | --- | --- | --- |
| Tasks, lists, and notes | Task/list CRUD, completion, moving, hierarchy, bulk actions, and notes projection; Tasks and Notes TUI surfaces. | Select a list; view, create, edit, complete, and delete tasks; set a date-only due date; view and edit notes using the existing projection setting. Show pending local edits. | Hierarchy editing, bulk actions, task scheduling as Calendar blocks, and full recurrence controls. |
| Calendars and events | Calendar management; Agenda, Day, Week, and Month TUI surfaces; event CRUD, recurrence, invitations, and instance refresh. | Select visible calendars; browse an agenda and week view; view, create, edit, and delete single events, including all-day events and time zones. Display cached recurring instances and their freshness state. | Day/month grids, recurring-series or occurrence editing, invitations/RSVP, calendar creation/subscription, and advanced event properties. |
| Search and capture | Local indexed search, saved searches, palette, and quick capture in TUI/CLI. | Search cached task and event titles, with an explicit body-search option; open a result; quick-capture a task or event through the shared parser. | Saved-search management, advanced search filters, Drive metadata search, and a global OS shortcut. |
| Sync and conflicts | Explicit sync, pending outbox, conflict list and resolution, uncertain-delivery reconciliation, explicit recurrence-instance refresh. | Show cached-data, last-sync, and pending state; start and cancel sync; show actionable errors and stale instance ranges; list and resolve conflicts, including uncertain delivery. Never hide a queued or ambiguous write. | Automatic background sync policy, if separately specified and validated. |
| Account and settings | Multi-account selection, Desktop OAuth, local config/profiles, notes mode, calendar selection, time zone, reminder settings, and extensive TUI theming. | Connect/select an account; show credential and sync status; edit time zone, calendar visibility, notes mode, and reminder enablement. | Theme/profile editor, custom keymap, terminal-specific appearance, and advanced daemon settings. |
| Reminders | Local reminder scheduler; macOS notification adapter and LaunchAgent; Linux currently prints to stderr. Event reminder overrides are editable; task reminder markers are stored in task notes. | Show event reminders and task reminder markers; allow reminder enablement and event popup-reminder edits. Show the current scheduler/notification status without promising background desktop notifications before platform integration is verified. | Task reminder editing, reliable Linux desktop notifications/background scheduling, and verified macOS post-exit notification behavior, tracked in the [release checklist](../../CHECKLIST.md). |

The first desktop app should use the core's existing date-only Google Tasks due
model. A timed task remains a linked Calendar event; the initial UI does not
need to create or edit that link. A recurring instance shown from cache must
not imply that its range is fresh. Editing a recurrence from the desktop UI
waits for an explicit scope workflow; the CLI/TUI remains available meanwhile.

Import/export, undo/redo, free/busy, bulk mutations, Drive metadata, custom
themes, and full CLI machine-output parity remain CLI/TUI workflows for the
first desktop release. These are deferrals for the desktop frontend, not
removals from the shared core.

## Acceptance for the first desktop release

- On Linux and macOS, open the same local account and cached data used by `hcb`,
  including after an app restart and while disconnected.
- Create and edit a task and a single event offline; see both in the relevant
  list/calendar and see their pending state; sync them explicitly after
  reconnecting and verify the resulting Google data during live acceptance.
- Surface a stale calendar-instance range and require explicit refresh before
  presenting it as fresh. Show sync cancellation, retryable failure, ordinary
  conflict, and uncertain delivery without silently dropping local intent.
- Keep the window responsive during list loading, search typing, calendar
  navigation, and sync. Measure those workflows against the performance targets
  when those targets are set in the checklist.
- Confirm account, configuration, keyring, SQLite migration, and outbox behavior
  with CLI/TUI and desktop processes coexisting. This remains a separate
  concurrency acceptance item in the checklist.

This scope was mapped from the current [CLI reference](../hcb-cli.md),
[`ApplicationService`](../../src/hcb/application.py),
[`HcbApp`](../../src/hcb/tui.py), and the reminder
[scheduler](../../src/hcb/scheduler.py) and
[notifier](../../src/hcb/notifications.py). It has not undergone live Google or
macOS acceptance.
