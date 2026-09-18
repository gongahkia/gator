# Google Work in Gator

Gator's Google Work connector is a Go-native port of the useful backend
workflows from Hot Cross Buns. It is a Work connector, not a separate Google
planner UI or a `gator google` command hierarchy.

## Set up a user-owned Google desktop OAuth app

Create or select a **Desktop** OAuth client in your Google Cloud project for
Gator. Enable the Google Tasks, Calendar, Drive, Docs, and Sheets APIs for that
project, then configure Gator with its client ID:

```sh
gator connector add google-work --kind google --oauth-client-id YOUR_CLIENT_ID
gator connector login google-work --oauth-client-secret-from-env GOOGLE_OAUTH_CLIENT_SECRET
```

The client secret is optional for clients that do not need one. If supplied it
is retained only in Gator's private `auth.json` alongside the refresh token;
it is never written to `config.json`, displayed by status, or put in Work
evidence. Google login uses PKCE and an ephemeral `127.0.0.1` loopback
callback. It is a new Gator authorization: Gator never reads a Hot Cross Buns
database, settings file, keychain entry, or credential.

`connector add --kind google` supplies Google's authorization/token endpoints
and these requested scopes by default:

- Tasks and Calendar read/write
- Drive metadata read
- Docs and Sheets read/write
- OpenID and email only for a live connection check

Select the connector for a Work run with `--connector google-work` or in the
Work TUI. The model receives only the explicitly selected, typed operations.

## Granular permissions

Each operation has its own existing Gator connector-permission key. For
example, a configuration may permit calendar reads while denying event
creation:

```sh
gator connector permission google-work events_list read allow
gator connector permission google-work events_create write deny
gator connector permission google-work tasks_create write ask
```

Available read operations cover task lists/tasks, calendar lists/events,
free/busy, Drive metadata, Docs, Sheets and bounded cell ranges. Mutating
operations cover task-list/task/calendar/event lifecycle, Docs batch updates,
Sheets creation/range/batch updates, and an ordered Google action batch.
Quick capture recognizes task/event, date, time, duration, priority, and
simple recurrence phrases and produces a reviewable preview; it never creates
a remote object by itself. `task_metadata_encode` similarly builds a bounded
`[GATOR-TASK v1]` notes block for task priority, date-only recurrence, and a
timezone-aware reminder; a later `tasks_create` or `tasks_update` still needs
its own fresh approval.

## Connected-only mirror

Google is always live-authoritative. Gator keeps a private per-connector
SQLite mirror under `STATE_DIR/gator/google-work/` only to make connected
searches, incremental comparisons, and batching efficient. A successful live
Google response is the only way data enters it.

`local_search` first makes a live Google health request. If authentication,
refresh, or that request fails, it returns no cached records. There is no
offline browse mode, offline mutation queue, cache-to-Work fallback, or
Hot Cross Buns migration path.

## Mutations and uncertainty

Every Google mutation requires Gator Work `act` mode plus a fresh approval for
the exact operation, target, and payload hash. The `batch` operation pins an
ordered canonical list of at most 25 typed Google mutations before a single
approval. Gator executes that list online without a local outbox.

If a request has an ambiguous result (for example a timeout after sending),
Gator records it as unknown and does not replay it automatically. Only a
bounded pre-send transport retry belongs inside an active user-approved action;
the local mirror is updated only after Google returns success.

## Deliberate boundaries

This port does not include Hot Cross Buns' Python/Textual, Swift/Qt/Electron
frontends, installer/theme formats, standalone CLI, SQLite migration, or
credential reuse. Existing HCB checkouts remain untouched. Its useful typed
domain behavior is being folded into Gator Work so HCB can be retired without
adding a competing product surface.
