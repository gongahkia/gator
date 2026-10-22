# Google Workspace in Gator

**Interface status:** Gator is a desktop app. Shell examples below are source-checkout developer operations with an explicit internal opt-in; see [development instructions](DEVELOPMENT.md). They do not install or enable a public CLI.

Gator's Google Workspace connector provides Google Drive, Docs, Sheets,
Calendar event reads, and Gmail message reads to Gator Work. This supports a
daily brief that combines local notes, today's calendar, and selected inbox
messages, then drafts a local document or email for review. A separately
enabled Gmail send operation can send one plain-text message to one recipient
only in a foreground `act` run after exact approval. Gator does not change
calendar events through this connector.

Ask Gator in chat to watch selected mail or calendar changes, then review the
sources, timing, end date and notification condition before saving. The
**Following → Manage checks** form remains available for precise edits. Checks
run only while the local supervisor is available; the agreement reports that
availability. A bounded mail/calendar poll uses the schedule you review.
It requires the needed read scopes and keeps a
durable content hash. Gmail watches retain a mailbox history cursor alongside
selected message fingerprints. They still list the selected search query on
every poll, so messages entering or leaving a time-relative query remain
visible. Gmail history identifies which existing message details need rereading;
an expired cursor, too much history, or a 24-hour full-sync interval causes a
full detail scan. The cursor is mailbox-wide and does not make the watch a push
subscription. A query with more than 500 Gmail matches or a Calendar window
with more than 500 events fails for review; narrow the scope. Calendar's moving
time window cannot use the [`events.list` sync token](https://developers.google.com/workspace/calendar/api/v3/reference/events/list), so the watch reads each
bounded page of that window on every poll. There is no automatic email sending.

Gmail's [history API](https://developers.google.com/workspace/gmail/api/reference/rest/v1/users.history/list)
can return HTTP 404 for an expired cursor; Gator then performs a full selected
scan and obtains a new cursor from the
[profile API](https://developers.google.com/workspace/gmail/api/reference/rest/v1/users/getProfile).
The selected query is still fetched on every poll because mailbox history has
no search-query filter. A failed or incomplete scan does not advance the
retained cursor. Gator binds a reusable cursor to the connector endpoint and
query. Authenticated recurring checks need an account or grant pin before they
reuse cached message fingerprints; older unpinned checks continue with full
detail scans so a changed account cannot inherit another account's cache.

## Set up in the desktop app

1. Open **Settings → Connections → Connect Google email & calendar**.
2. Enter a unique connection ID, a display name and your Google Cloud **Desktop app** OAuth client ID. Enable the Gmail and Calendar APIs in that project. Gator does not yet operate a managed Google OAuth application, so this is still an advanced prerequisite.
3. Review and save. Choose **Sign in**, optionally enter the client secret required by your own OAuth app, and review the requested scopes.
4. Choose **Continue in browser**. Finish Google consent and return to Gator. Sign-in waits up to five minutes; you can cancel it or switch to another chat and return through Connections.
5. Choose **Check account & read access**. Gator verifies the account identity and makes bounded reads for the configured email, calendar and Drive metadata scopes. A failure on one service is reported as a failed check. A verified email is shown only if Google marks it verified.
6. Select the connection in chat settings or in a follow-up agreement. Saving credentials does not select it for a chat or authorize writes.

The native email/calendar setup requests OpenID, email, Gmail read and Calendar
event read scopes. It does not request document editing or email sending.
Existing Google connections with other scopes can also be signed in and managed
here. The advanced developer configuration below can create those connections.

**Reconnect account** replaces credentials after browser authorization. A
scheduled check pinned to a different account still needs a reviewed agreement
edit. **Disconnect account** removes local credentials; it does not revoke the
grant at Google. Remove that grant in your Google account settings when needed.
**Remove connection** also removes its local configuration; dependent checks
retain their source references and report unavailable access.

Credential status is local state, not proof of current access. Live checks
report only the surfaces they actually read, never successful writes. Tokens,
refresh tokens and client secrets stay in the private credential store; the
renderer receives a temporary authorization URL and status. Late sign-in or
refresh results cannot overwrite credentials changed since the operation began.

The native browser flow uses PKCE and an ephemeral loopback callback, matching
Google's [installed-app OAuth guidance](https://developers.google.com/identity/protocols/oauth2/native-app).
A managed app still needs provider verification and an operational owner; a
client-ID form does not remove those requirements.

## User-owned Google desktop OAuth app

Create or select a **Desktop** OAuth client in your Google Cloud project, then
enable the Google Drive, Docs, Sheets, Calendar, and Gmail APIs:

```sh
GATOR_INTERNAL_COMMANDS=1 go run ./cmd/gator provider connector add google-work --kind google --oauth-client-id YOUR_CLIENT_ID --daily-context
GATOR_INTERNAL_COMMANDS=1 go run ./cmd/gator provider connector login google-work --oauth-client-secret-from-env GOOGLE_OAUTH_CLIENT_SECRET
```

The client secret is optional for public clients. When supplied, it is retained
only in Gator's private `auth.json` beside the refresh token; it is never
written to `config.json`, shown by status, or added to Work evidence. Login
uses PKCE and an ephemeral `127.0.0.1` loopback callback.

The default OAuth scopes are OpenID and email for a live connection check,
Drive metadata read, and Docs/Sheets read-write. `--daily-context` explicitly
adds Calendar event read and Gmail read-only scopes. Without it, those operations
are absent from the agent tool list. For an existing connection, add a separate
Google connector with `--daily-context` and run `connector login` for it.
To enable sending, add `--send-email` during setup and log in with that new
scope. It requests only `gmail.send` for the send operation; `--daily-context`
is still needed if Gator should read Gmail and Calendar. Existing credentials
do not silently gain the send scope. Google classifies `gmail.send` as a
sensitive scope, so a published OAuth app may need verification.
Gmail read-only is a restricted Google scope,
so publishing a broadly available OAuth app may require Google's verification
process. Existing authorizations may retain previously granted scopes at
Google until the user revokes them.

New goal schedules and watches using a Google OAuth connector make a live
[OpenID UserInfo](https://developers.google.com/identity/openid-connect/reference)
request and pin a SHA-256 digest of the account's stable `sub`
identifier. Gator checks that identity before a scheduled run and against the
token used for each connected request. Reauthorizing the same account keeps
the check valid; switching accounts stops it before connected data is read.
If identity cannot be checked, the read fails closed. `gator goal activity ID`
shows that the account is pinned, while `gator goal access ID` makes a separate
live probe. To intentionally change accounts, confirm the new account, then
use `gator goal unschedule GOAL_ID JOB_ID` and recreate the check. Existing
refresh-grant-pinned checks retain their stricter behavior and stop on any
grant replacement; older unpinned checks still appear as unbound in activity.
The job definition contains only a digest, not the subject or token. This
identity check does not establish that a connected result is relevant or safe.

## Operations and approvals

Read operations cover a live connection check, Drive metadata search and file
metadata, Google Docs, Google Sheets, bounded Sheet ranges, a bounded Calendar
event window, and Gmail search plus message detail. Calendar reads require
`time_min` and `time_max` in RFC3339; Gmail list defaults to the last day of
inbox messages and returns IDs that can be opened with `gmail_get`. Gator
records each read with connector provenance and enforces the selected
connector read permission policy. Mutations cover
Docs create/batch update and Sheets create/range/batch update, plus an ordered
batch of those typed mutations. Update payloads may include a Google `etag`;
Gator binds it to the approved payload and sends it as `If-Match`.
For `sheets_values_update`, Gator includes `valueInputOption` in the exact
approval preview and sends it as the required URL query parameter. It defaults
to `RAW` if omitted; a user may explicitly choose `USER_ENTERED`. The option
is removed from the JSON `ValueRange` body. This follows Google's
[`spreadsheets.values.update` request contract](https://developers.google.com/workspace/sheets/api/reference/rest/v4/spreadsheets.values/update).

Sheet value writes accept booleans, numbers, and strings. Gator rejects `null`
cells because [Google skips them](https://developers.google.com/workspace/sheets/api/reference/rest/v4/spreadsheets.values#ValueRange);
use an empty string when the intent is to clear a cell.

For an individual Docs or Sheets action, the Work action record retains the
resource ID returned by Google and a SHA-256 digest of the response. An update
whose returned resource ID is missing or differs from the approved target is
recorded as uncertain, so it must be inspected before any retry. The resource
ID identifies a document or spreadsheet, not a unique edit receipt; the digest
does not prove the requested change is visible in a later read. Ordered batches
retain each attempted step's status and, when Google confirms a step, its
resource ID and response digest. If a later step fails before its request,
the batch is still marked uncertain because earlier steps may already have
changed remote state. Batches are not transactional and are never retried
automatically; a response digest is not a provider-issued mutation ID.

`gmail_send` accepts exactly `from`, `to`, `subject`, and `body` in its action
payload. It creates a plain-text MIME message for one recipient, with no
attachments, Cc, or Bcc. The `from` address must be a sender Gmail accepts for
the connected account. Gator shows the exact fields and target for approval;
an unattended goal check cannot use this operation. A successful send retains
Gmail's message ID in the action record in the Work bundle. Every prepared send
also has a unique reconciliation ID, embedded in its MIME `Message-ID` header
and retained with the action record. It distinguishes two separately approved
sends with identical text. If the API outcome is uncertain, Gator records that
uncertainty and does not retry automatically. For an action marked `unknown`,
use the IDs shown by `gator work review WORK_ID` to check Sent:

```sh
GATOR_INTERNAL_COMMANDS=1 go run ./cmd/gator work reconcile WORK_ID CORRELATION_ID
```

This read-only check searches the same Google connector's Sent mailbox, opens
the matching message, and checks its `SENT` label and exact `Message-ID` header.
It requires the connector's Gmail read-only scope and allowed list/detail
reads. Gator stores the latest check under
`STATE_DIR/gator/reconciliations/WORK_ID/CORRELATION_ID.json` and leaves the
sealed Work action as `unknown`. A match confirms a Sent mailbox copy, not
recipient delivery. No match is inconclusive because indexing can lag, and
multiple matches require manual inspection. The check never sends or retries.
Google's [messages.list API](https://developers.google.com/workspace/gmail/api/reference/rest/v1/users.messages/list)
supports an `rfc822msgid:` query with a Gmail read scope. The `gmail.send`
scope alone cannot perform that search, and a missing result must not be
interpreted as proof that a timed-out send failed.

Every mutation requires Work `act` mode and a fresh approval for the exact
operation, target, and payload hash. A batch contains at most 25 typed
operations and is never replayed automatically after an ambiguous outcome.
Gmail sends follow the same no-automatic-retry rule. This operation follows
Google's [MIME and base64url send guide](https://developers.google.com/workspace/gmail/api/guides/sending)
and uses the separate [Gmail send scope](https://developers.google.com/workspace/gmail/api/auth/scopes).

## Boundaries

Gator neither reads nor writes its former `STATE_DIR/gator/google-work/`
SQLite mirror. It does not delete that old local state automatically. There is
no offline Google browse mode, mutation queue, Hot Cross Buns migration path,
or credential reuse.
