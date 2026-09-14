# Desktop bridge protocol

The desktop bridge lets a native frontend use the current local HCB core without
opening HCB's SQLite database, token store, OAuth client configuration, or Google
transport. It is a local process boundary, not a network service and not another
source of account state.

Start it from the same installed HCB version as the frontend:

```sh
hcb bridge serve --ready-file /private/launcher-directory/bridge.json
```

The launcher must create a new path in an owner-only directory. The command
binds only `127.0.0.1`, writes a new JSON descriptor with mode `0600`, and stays
in the foreground. The descriptor contains a version, loopback URL, and
random bearer token. It is removed during normal `SIGINT` or `SIGTERM` shutdown.
The bridge refuses to overwrite a descriptor path so a launcher cannot
accidentally connect to stale credentials.

Every request sends `Authorization: Bearer <descriptor token>`. Responses are
JSON envelopes:

```json
{"api_version": 1, "data": {}}
```

Failures use `{"api_version": 1, "error": {"code": "...", "message": "..."}}`.
The bridge does not emit Python tracebacks, OAuth access tokens, refresh tokens,
credential file paths, or database paths.

## Version 1 surface

All data uses the existing Python `to_primitive` representation. Names are
snake_case; timed event values are objects such as
`{"kind":"dateTime","value":"2026-09-14T09:00:00+00:00","time_zone":"UTC"}`.

| Method and path | Result |
| --- | --- |
| `GET /v1/health` | Bridge identity and protocol version. |
| `GET /v1/accounts` | Local account metadata. |
| `GET /v1/accounts/{account}/workspace` | Small account summary: account, task lists, calendars, cached instance ranges, and pending count. |
| `GET /v1/accounts/{account}/workspace?include=tasks` | Explicit full task mirror for bounded/small consumers. |
| `GET /v1/accounts/{account}/workspace?include=events&start=...&end=...` | Date-bounded event range. `calendar_id` is optional. |
| `GET /v1/accounts/{account}/tasks?limit=200&cursor=...&list_id=...` | Preferred virtual-list path. Limits are 1–500 and the opaque cursor resumes the stable local order. |
| `GET /v1/accounts/{account}/search?q=...&limit=...` | Existing local workspace search. |
| `POST /v1/accounts/{account}/tasks`, `PATCH`/`DELETE /tasks/{id}`, `POST /tasks/{id}/complete` | Optimistic task operations through `ApplicationService`. |
| `POST /v1/accounts/{account}/events`, `PATCH`/`DELETE /events/{id}` | Optimistic event operations through `ApplicationService`. |
| `POST /v1/accounts/{account}/sync` | Starts an asynchronous sync operation. |
| `POST /v1/accounts/{account}/oauth` | Starts the existing browser OAuth connection for an already configured account; body is `{"expected_email":"..."}`. |
| `GET`/`DELETE /v1/operations/{id}` | Poll an asynchronous operation or request cancellation. |

Task and event changes require an `Idempotency-Key` header with a frontend-
generated token. HCB stores the resulting HTTP status and response in the same
SQLite transaction as the core mutation. Retrying the same request after a
lost response returns that stored result; reusing the key with different method,
path, or payload returns `409`. Receipts expire after seven days.

`sync` cancellation is passed to the existing sync engine and reaches a
cancelled state when it stops. [Limit] OAuth can be cancelled before the bridge
opens the current browser/listener flow. The existing Google authenticator has
no cancellation hook once that flow is active, so a later cancellation request
is recorded but cannot interrupt it. A cancellable authenticator API is still
needed before promising OAuth cancellation in a desktop UI.

## Process and performance model

The bridge opens a fresh `Runtime` and SQLite connection for each ordinary
request. It never shares a SQLite connection across HTTP worker threads. A
mutation holds one outer SQLite transaction so its optimistic core update,
outbox item, and idempotency receipt commit together. Existing process-level
sync ownership and SQLite WAL locking remain the coordination mechanism for the
CLI, TUI, reminder process, sync workers, and bridge.

For a synthetic 10,000-task/2,000-event local mirror on the Fedora development
machine, the five-run median was about 7 ms for the summary and 35 ms for a
200-task page. The deliberately full task workspace response was about 4.25 MB
and took about 1.30 s. These are machine-specific observations, not release
budgets; use the reproducible command below before releases:

```sh
make benchmark-desktop-bridge
```

Desktop clients should load the summary, then pages and bounded event ranges;
they should not request the full task mirror during initial UI construction.
