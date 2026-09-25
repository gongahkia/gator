# Google Workspace in Gator

Gator's Google Workspace connector provides Google Drive, Docs, and Sheets to
Gator Work. It is not a planner: Google Tasks, Google Calendar, reminders,
quick capture, and local Google mirrors are not part of this product.

## Set up from Work

After creating a Desktop OAuth client and enabling the Drive, Docs, and Sheets
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
enable the Google Drive, Docs, and Sheets APIs:

```sh
gator provider connector add google-work --kind google --oauth-client-id YOUR_CLIENT_ID
gator provider connector login google-work --oauth-client-secret-from-env GOOGLE_OAUTH_CLIENT_SECRET
```

The client secret is optional for public clients. When supplied, it is retained
only in Gator's private `auth.json` beside the refresh token; it is never
written to `config.json`, shown by status, or added to Work evidence. Login
uses PKCE and an ephemeral `127.0.0.1` loopback callback.

The default OAuth scopes are OpenID and email for a live connection check,
Drive metadata read, and Docs/Sheets read-write. Gator does not request Tasks
or Calendar scopes. Existing authorizations may retain previously granted
scopes at Google until the user revokes them; reconnecting uses Gator's reduced
scope set.

## Operations and approvals

Read operations cover a live connection check, Drive metadata search and file
metadata, Google Docs, Google Sheets, and bounded Sheet ranges. Mutations cover
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
