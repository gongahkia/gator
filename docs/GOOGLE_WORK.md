# Google Workspace in Gator

Gator's Google Workspace connector provides Google Drive, Docs, Sheets,
Calendar event reads, and Gmail message reads to Gator Work. This supports a
daily brief that combines local notes, today's calendar, and selected inbox
messages, then drafts a local document or email for review. Gator does not
send email or change calendar events through this connector.

An ongoing goal can select this connector in a timed, read-only follow-up with
`gator goal schedule GOAL_ID --cron '0 9 * * *' --connector google-work --task 'Prepare the daily brief from selected mail and calendar events'`.
This runs only while the local supervisor is available. For a bounded connected
poll, use `gator goal watch GOAL_ID --connector google-work --gmail-query 'label:offsite newer_than:7d' --calendar-days 14`.
It checks about every five minutes, requires the needed read scopes, and keeps a
durable content hash. It does not provide push events, a provider event cursor,
or automatic email sending. A query with more than 100 Gmail matches or a
Calendar window with more than 100 events fails for review; narrow the scope.

## Set up from Work

After creating a Desktop OAuth client and enabling the Drive, Docs, Sheets,
Calendar, and Gmail
APIs, setup can remain inside `gator work`:

```text
/connector setup google-work YOUR_CLIENT_ID
/connector login google-work prompt
/connector test google-work
```

`/connector status google-work`, `/connector permission ...`, `/connector
logout google-work`, and `/connector delete google-work` cover the rest of the
connector lifecycle. Select it for a Work run with `--connector google-work`.

## User-owned Google desktop OAuth app

Create or select a **Desktop** OAuth client in your Google Cloud project, then
enable the Google Drive, Docs, Sheets, Calendar, and Gmail APIs:

```sh
gator provider connector add google-work --kind google --oauth-client-id YOUR_CLIENT_ID --daily-context
gator provider connector login google-work --oauth-client-secret-from-env GOOGLE_OAUTH_CLIENT_SECRET
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
Gmail read-only is a restricted Google scope,
so publishing a broadly available OAuth app may require Google's verification
process. Existing authorizations may retain previously granted scopes at
Google until the user revokes them.

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

Every mutation requires Work `act` mode and a fresh approval for the exact
operation, target, and payload hash. A batch contains at most 25 typed
operations and is never replayed automatically after an ambiguous outcome.

## Boundaries

Gator neither reads nor writes its former `STATE_DIR/gator/google-work/`
SQLite mirror. It does not delete that old local state automatically. There is
no offline Google browse mode, mutation queue, Hot Cross Buns migration path,
or credential reuse.
