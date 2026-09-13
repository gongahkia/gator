# Live Google TUI/CLI smoke

Status for the current validation: **full external gate not executed**. A read-only
preflight and a partial disposable-account Fedora smoke are recorded below. Neither
passes the full Linux/macOS acceptance gate. Update the redacted attestation only
after completing every step of the full procedure.

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
   cross-list move a non-recurring task and verify Google's
   [Tasks move operation](https://developers.google.com/workspace/tasks/reference/rest/v1/tasks/move)
   places the same remote task in the destination list without a duplicate.
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
   must remain visible throughout. Queue a later edit before the interruption and
   verify sync leaves it queued until reconciliation, then sends it afterward.

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

- OAuth/credential boundary: **incomplete**
- Tasks/recurrence/outbox: **incomplete**
- Calendar/tokens/rich fields: **incomplete**
- Free-busy/reminders/conflicts/offline: **incomplete**
- Disconnect/reset: **not executed**
- Overall live gate: **not passed**

## Disposable-account Fedora follow-up (partial)

Through 2026-09-13 15:28 UTC, automated CLI/API checks used the same isolated
disposable-account profile. The candidate checkout was `7662fe382` plus the
uncommitted sync and schema follow-up. The account identity was checked against Google's
UserInfo response before writes. The personal profile was not opened or changed.

- Manual Ask resolution in both directions and automatic Prefer Google and
  Prefer Local handling of stale Calendar ETags: **passed**. Google readback and
  local dirty/outbox state agreed after each synthetic edit.
- Abrupt process exit after Google accepted a synthetic Task create: **passed**.
  Restart showed uncertain delivery; verified `delivered` reconciliation kept
  one remote task and subsequently delivered a queued edit.
- Abrupt exit before the create request: **passed**. Google had no task;
  verified `retry` created one task and then delivered the queued edit.
- Invalid Calendar sync-token string: Google returned `400`, as distinct from
  expiry `410`; HCB surfaced the error and retained its cache. An injected
  `410` followed by real Google full-page reads: **passed**. The affected
  calendar rebuilt, an existing event retained its local ID, and the other
  calendar used its previous incremental token. Synthetic calendars were
  removed; Google returned deleted tombstones and no pending outbox rows.
- Managed daily Task recurrence with an exact local reminder: **passed**.
  Two completions produced one successor each, repeat sync produced no fourth
  task, and Google Tasks readback showed three markers and two completed tasks.
- Synthetic task hierarchy insert, top-level reparent, sibling reorder,
  cross-list move, and task-list rename: **passed** after a pull fix that maps
  Google parent IDs to local task IDs. The cross-list move retained one remote
  task ID in the destination; the source list had no active duplicate. Both
  synthetic lists were removed and no pending outbox row remained.
- Disabled, Notes-only, and Mirrored projections plus bulk completion:
  **passed** with remote readback. A first mixed-mode run exposed loss of a
  local-only Disabled-mode note after switching mode before sync. The local
  note record added in schema 10 preserved it on the repeat run; Google did
  not receive the Disabled-mode note. Both synthetic task lists were removed.
- Timed IANA event, all-day three-instance series, popup and email reminder
  fields, guest permissions, private visibility, changed and cancelled
  instances, explicit instance refresh, and remote free/busy: **passed** with
  Google readback. One test attempt used a derived-instance ID after its cache
  was replaced; the repeat refreshed the cache before editing. Both synthetic
  calendars were removed, with Google deletion tombstones and empty outbox
  verified. No attendees were added or notified.
- Moving one attendee-free default event between two synthetic calendars:
  **passed**. Google readback showed it in the destination, no active source
  copy, and a stable local event after two repeat syncs. Both calendars were
  removed and the outbox was empty.
- Creating an attendee-free event with a new Meet conference request:
  **passed**. Google returned a successful conference request and a video
  entry point; HCB retained conference data after repeat sync. The synthetic
  calendar was removed, and no attendee was invited.
- Injecting response loss after Google accepted an attendee-free event create:
  **passed**. The next sync reconciled the client-assigned event ID after a
  duplicate-create response. Google had one active event, and HCB had no
  pending outbox row or conflict. The synthetic calendar was removed. An
  earlier inspection attempt used a nonexistent outbox column; after correcting
  that test harness, the full sequence passed.
- Cleanup readback for all 11 synthetic calendars recorded in this isolated
  profile: **passed for calendar-list removal**. Every local fixture was marked
  deleted, none remained in Google's calendar list, and the outbox and conflict
  queue were empty. `calendars.get` still returned metadata for the three most
  recent removed calendars while their `calendarList.get` returned `404` and
  their event lists were empty. This check does not establish physical purge of
  Google calendar metadata.
- Headless TUI startup on the isolated disposable cache: **passed**; the
  workspace mounted with one cached row and no pending write. Physical terminal
  interaction and simultaneous CLI/TUI/reminder operation remain untested.
- CLI/TUI cross-process UI operation, notification delivery,
  OAuth revocation/reconnect, destructive reset, and
  macOS: **not executed**. Drive attachment, invitation RSVP, and installed-app
  checks also remain open. The disposable Drive metadata view contained files,
  but no file was verified as a synthetic fixture, so none was attached. No
  invitations or messages were sent.

The `410` run exposed that deleting every clean event on refresh changed local
IDs used by task-event links and reminders. The candidate implementation
preserves clean rows while the full pull runs, retains rows returned by Google,
and prunes missing rows only when the final page and cursor commit together.
A fake-gateway test covers an interrupted multi-page restart. This remains a
partial acceptance record, not release approval.

## Disposable-account Fedora partial smoke

This automated smoke used a separate Google test user and an isolated owner-only
credential file, configuration, cache, and SQLite database. It did not open the
personal HCB profile. Before any Google data writes, Google's UserInfo endpoint
confirmed the signed-in email matched the intended test account. A local scan
found no plaintext refresh token in the credential file, configuration, SQLite
dump, or diagnostics; the test credential file has a different keyring key from
the personal credential file.

- Candidate commit at live run: `9b99e7f17`; package version: `0.2.0`
- Platform: Fedora Linux 43, Linux `7.1.12-100.fc43.x86_64`, Python `3.12.13`
- Completed: `2026-09-13 09:07 UTC`; tester role: automated local smoke
- Initial pull and identity/credential isolation: **passed**
- Synthetic task-list create/delete and task create/edit/complete/delete,
  checked against Google Tasks readback: **passed**
- Synthetic calendar create/delete and timed event create/edit/delete, including
  IANA time-zone readback, checked against Google Calendar: **passed**
- Offline local task create/edit across CLI process restarts, one remote delivery,
  no repeat send on the second sync, and final empty outbox: **passed**
- Synthetic task lists, tasks, calendars, and events created by this smoke were
  removed and the removals checked remotely: **passed**
- Full recurrence, conflict policies, rich event fields, revocation/reconnect,
  TUI/manual workflows, and macOS acceptance: **not executed**

The OAuth account-email check added after this run has unit coverage, but its
browser authorization path has not been repeated live. No invitations or other
messages were sent to an external attendee.

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
