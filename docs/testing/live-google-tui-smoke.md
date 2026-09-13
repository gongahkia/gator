# Live Google TUI/CLI smoke

Status for the current validation: **full external gate not executed**. A limited
read-only preflight is recorded below; it does not pass the disposable-account
acceptance gate. Update only the redacted attestation section after completing
every step of the full procedure.

Use a disposable Google account and user-owned Cloud project. Never record client
IDs, client secrets, tokens, authorization codes, account email/subject, remote IDs,
titles, notes, attendees, file names, request/response bodies, database paths,
screenshots, or raw logs.

## Safe setup

Follow the [bring-your-own OAuth setup guide](../oauth-setup.md) before this
acceptance run; preparing credentials does not itself pass the live gate.

1. Enable Google Tasks, Calendar, and Drive APIs. Create an OAuth **Desktop app**
   client and add the disposable account as a test user when required.
2. Create an owner-only account `.env` outside the repository with
   `HCB_GOOGLE_CLIENT_ID` and optional `HCB_GOOGLE_CLIENT_SECRET`; set
   `HCB_ENV_FILE` to its path when not using HCB's default personal path. Do not
   print or copy its contents.
3. Use an isolated config/data/cache directory and OS credential store. Confirm the
   requested scopes are Tasks, Calendar, and Drive metadata read-only.
4. Create one disposable task list, calendar, Drive file, and external test attendee.
   Use neutral data that contains no personal information.

## OAuth and credential boundary

1. Run onboarding and connect. Verify PKCE loopback opens only after confirmation,
   no client-secret field exists, and denied consent/occupied callback port produces
   an actionable error without a traceback.
2. Restart and sync without authorizing again. Search config, a SQLite dump,
   diagnostics, and any candidate support archive for known token sentinels. Tokens
   must exist only as encrypted values in the account `.env`; their decryption key
   must exist only in the OS credential store.
3. Revoke consent, verify sync requests reconnection while preserving cache, then
   reconnect.

## Tasks, recurrence, and outbox

1. Create/rename/delete a list; create/edit/complete/delete/reorder/reparent tasks;
   cross-list move a task and verify Google receives one destination create and one
   source delete.
2. Exercise disabled, notes-only, and mirrored projections with an undated root task.
3. Create a managed recurring task and exact reminder through HCB. Complete two
   occurrences, verify one successor each, marker preservation in notes, and no
   duplicate successor after repeated sync.
4. Go offline, queue task/list edits, quit, restart offline, then reconnect and sync
   twice. Verify each remote mutation is applied once and the outbox empties.
5. During a disposable create, terminate the process only after diagnostics show the
   outbox row entered `sending`. For Tasks, task lists, and calendars, restart must
   surface an **uncertain delivery** action and must not issue another create. Check
   Google manually, then exercise both resolutions: provide the remote ID when the
   object exists, or choose retry only after verifying it does not. The local intent
   must remain visible throughout.

## Calendar tokens and rich fields

1. Perform initial multi-page sync, interrupt it between pages, restart, and verify
   resume without duplicate canonical rows. Make a remote edit and verify subsequent
   incremental sync uses the stored per-calendar token.
2. Cause or inject `410 Gone` for one disposable calendar. Verify only that
   calendar's canonical cache/token rebuilds and other calendar tokens remain.
3. Verify timed events preserve selected IANA wall time and all-day dates remain
   stable. Run `events refresh-instances` for a bounded range. Test recurrence plus
   one changed and one cancelled instance; recurrence lines must round-trip
   unchanged, cached instances must appear in `events instances`, and Google
   instances remain authoritative.
4. Create/edit/delete/move an event with popup and email reminders, attendees,
   guest permissions, RSVP comment, send-updates choice, Meet conference, and Drive
   metadata attachment. Verify email reminders never become local notifications,
   Meet works, and Drive file content is never read or uploaded.
5. Exercise invitation RSVP accepted/tentative/declined, Focus time, Out of office,
   Working location, calendar create/subscribe/remove-from-list, and bulk actions.
6. Terminate one Calendar event create after Google accepts it but before local
   completion, then restart and sync. Verify the retry uses the same client-assigned
   event ID, a Google `409 already exists` is reconciled as success, exactly one
   remote event exists, and the outbox clears.

## Free/busy, reminders, conflicts, and offline reads

1. Run cached find-time and confirm it makes no request. Then explicitly run remote
   free/busy, inspect candidate slots, cancel once, and confirm no event was written.
2. Enable the local scheduler, close the TUI, and verify a popup reminder fires.
   Snooze ten minutes and verify no early repeat; dismiss and verify no repeat after
   scheduler restart. Deny notification permission and verify diagnostics, not a
   sync crash.
3. For Prefer Google, Prefer Local, and Ask, queue an offline edit and make a
   conflicting remote edit. Restore connectivity and verify the chosen result.
   For Ask, resolve each direction once through both CLI and TUI.
4. Disable networking and restart. Confirm tasks, notes, agenda, search, saved
   searches, and diagnostics read only SQLite and make no hidden Google request.

## Disconnect and reset

1. Disconnect. Confirm OS credentials are removed while cache remains readable and
   diagnostics contain no identity/token material.
2. Reconnect once, then run destructive reset only after its explicit confirmation.
   Confirm credentials, account cache, outbox, conflicts, reminder state, and
   account-scoped settings are removed.

## Redacted attestation

Record only: candidate commit, package version, OS/Python versions, UTC completion
time, `passed`/`failed` per section, and tester role. Do not paste command output.

Current result:

- OAuth/credential boundary: **not executed**
- Tasks/recurrence/outbox: **not executed**
- Calendar/tokens/rich fields: **not executed**
- Free-busy/reminders/conflicts/offline: **not executed**
- Disconnect/reset: **not executed**
- Overall live gate: **not passed**

## Read-only preflight

This limited run used an existing account that is **not disposable**. No Google
create, update, delete, revocation, or reset operation was run. Its results do
not change the full-gate statuses above.

- Candidate commit: `821107ca1`
- Package version: `0.2.0`
- Platform: Fedora Linux 43, Linux `7.1.12-100.fc43.x86_64`, Python `3.12.13`
- Completed: `2026-09-13 05:59 UTC`
- Tester role: automated local read-only preflight
- Credential parsing, permissions, keyring decryption, refresh, and requested scopes: **passed**
- Google Tasks, Calendar, and Drive read-only API reachability: **passed**
- Isolated initial and incremental pull with no queued writes: **passed**
- Offline cached task, agenda, search, and TUI Tasks/Agenda reads: **passed**
- Plaintext credential scan of local config, diagnostics, and SQLite dump: **passed**
- Disposable-account writes, conflict recovery, revocation, reset, and macOS acceptance: **not executed**
