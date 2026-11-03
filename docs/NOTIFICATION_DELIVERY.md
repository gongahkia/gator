# Desktop notification delivery

Implemented as a slice of competitive items 8 and 14, November 2026.

An Inbox update, OS notification handoff, display, reading, and resolution are
different events. A successful OS command confirms handoff only. Inbox details
show the current delivery state; opening those details does not retry delivery
or mark an update delivered, read or resolved.

## Durable handoff and retry

The sender persists `pending` before contacting the OS and records `handed_off`
only when the bounded OS command succeeds. Errors leave `failed`; a process
interruption can leave `pending`. The legacy `desktop_notified_at` timestamp is
written only on confirmed handoff for newly tracked deliveries. Existing legacy
timestamps remain respected without retroactively asserting display success.

Already admitted, unread and unresolved notifications may retry through the
existing job supervisor. Retries use delays of one minute, five minutes, fifteen
minutes and one hour, with a maximum of five attempts total. At most four due
deliveries are attempted in a supervisor pass. Current per-goal mute, urgent-only
and quiet-hour policies apply to retries. Disabling supervisor notifications
also disables its retry pass. Old unread entries without a delivery record are
not newly admitted on upgrade. Reading or resolving an entry stops future retries.

Per-entry process-held leases prevent concurrent handoff. The global inbox lock
is released while the OS sender runs, so receipts remain usable. Each production
OS call has a five-second context limit and a one-second process wait bound.
Outcome settlement reloads the record and preserves concurrent receipt changes.
Raw OS errors are not persisted because they can contain private content.

Retries concern notifications only: no model request, job, email, connector
mutation or approval is replayed. A crash after actual OS handoff but before its
durable settlement can produce a duplicate notification. This is bounded retry,
not exactly-once display. After the fifth unconfirmed attempt, the update stays
available in Inbox with an explicit unconfirmed status.

Delivery metadata participates in existing backup/deletion with its Inbox entry.
Process lock files are excluded from portable backups. The optional JSON field
does not replace or reinterpret the independent chat delivery/read/resolution
receipts. Older binaries ignore the new metadata; rolling back may lose retry
tracking if an older binary rewrites an entry, so back up before a downgrade.

## Native command boundary

On macOS the AppleScript source is fixed and notification text is supplied as
argv. Previously the summary was interpolated into a quoted script literal.
On Linux `notify-send` uses an option terminator before text arguments. Both
paths now return failures rather than silently claiming successful delivery.
Windows native desktop notification support remains unimplemented.

## Evidence and remaining acceptance

Tests cover failed/successful handoff, backoff across store reopening,
concurrent delivery, writable receipts during a held sender, interrupted pending
state, retry exhaustion, reading/resolution, retained backups without leases,
and truthful Inbox projection. The command adapter also tests hostile-looking
text against the real macOS AppleScript argument parser using a return-only
program; it displays no notification and executes no supplied program text.

Passing commands: full race suites for `internal/inbox`, `internal/lifecycle`
and `internal/workapp`; focused command/race checks matching desktop delivery,
notification, Inbox projection, read/resolution and goal-review behavior. The
multi-package focused race run also includes the added backup/projection tests.

Physical OS notification display, permission denial, Focus/Do Not Disturb,
Linux desktop-session delivery, mobile push and channel escalation remain
separate acceptance work. This implementation does not close competitive item 8.
