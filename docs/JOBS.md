# Durable jobs design

## Implemented boundary

Gator does not turn the process-local terminal registry, TUI queue, or app
server into a scheduler. Those components own live resources in one process;
they do not provide durable ownership, missed-run policy, or restart safety.

Scheduled Work runs through a separate local supervisor backed by versioned job
definitions and immutable attempt records. The user starts it in a foreground
terminal with `gator job supervisor`; Gator deliberately does not install an OS
service or stretch its process-local TUI queue into a scheduler.

This is a product boundary, not a missing alias. A command that merely sleeps
in the background would look persistent while losing jobs on logout, upgrade,
crash, or machine restart.

## Authority boundary

Unattended jobs may run only in `inspect` or `draft` mode. Their external-action
disposition is `forbid` or `draft`; it can never be `approve`. A scheduled run
may therefore prepare a reviewable webhook proposal, but it cannot send it.

Interactive approval is deliberately non-transferable:

- an approval from a foreground run cannot be stored on a job;
- an approval cannot be represented by an environment variable or CLI flag;
- a credential proves authentication, not authorization for a side effect;
- resuming or retrying a job does not inherit a prior action decision; and
- every eventual publish or connected mutation still passes through the
  foreground `action.Broker` against the exact target and payload digest.

## Durable model

A job definition is developer-owned configuration, written atomically as a
private file. Its version-1 shape includes:

```json
{
  "version": 1,
  "id": "job-...",
  "name": "weekly-metrics",
  "enabled": true,
  "schedule": "0 9 * * 1",
  "timezone": "Asia/Singapore",
  "missed": "run_once",
  "source_path": "/absolute/canonical/source",
  "refresh_snapshot": true,
  "objective": "Prepare the weekly metrics brief.",
  "mode": "draft",
  "contract": {
    "version": 1,
    "external_actions": "draft",
    "artifacts": [
      {"path": "brief.md"},
      {"path": "metrics.csv"}
    ]
  },
  "connector_ids": ["metrics"],
  "max_steps": 24,
  "created_at": "2026-09-09T00:00:00Z",
  "updated_at": "2026-09-09T00:00:00Z"
}
```

The stored definition includes the normalized outcome contract, optional
provider/model selection, maximum turns, and connector IDs. Secrets remain in
Gator's credential store. A definition never embeds an API key, bearer token,
or approval.

Each invocation creates an immutable `intent.json` before model work starts,
appends lifecycle events to `events.jsonl`, and publishes one terminal
`result.json`:

```json
{
  "version": 1,
  "id": "attempt-...",
  "job_id": "weekly-metrics",
  "definition_sha256": "...",
  "scheduled_at": "2026-09-14T01:00:00Z",
  "started_at": "2026-09-14T01:00:03Z",
  "finished_at": "2026-09-14T01:04:12Z",
  "status": "completed",
  "conversation_id": "workconv-...",
  "revision_id": "workrev-...",
  "bundle_path": "/private/.../work-...",
  "try": 1
}
```

The result points to a normal sealed Work bundle; it does not create a second
artifact format. Status transitions in the event log are append-only (`queued`,
`running`, then one terminal state). A terminal `result.json` is created once
and never overwritten. If the foreground process is killed mid-run, its intent
and running events remain available for inspection; automatic interrupted-run
reconciliation is not yet implemented.

## State and ownership

The supervisor owns a private subtree below Gator's state directory:

```text
gator/jobs/
  definitions/<job-id>.json
  history/<job-id>/<execution-id>/intent.json
  history/<job-id>/<execution-id>/events.jsonl
  history/<job-id>/<execution-id>/result.json
  supervisor.json
  supervisor.token
  supervisor.lock
```

Definitions and records are `0600`; directories are `0700`; definition
publication uses write, sync, rename, and parent-directory sync. IDs are
bounded, and state reads reject symlinks and non-regular files. A private
exclusive-create lock prevents two supervisors from owning the store. A stale
lock is removed only after its recorded PID is no longer live.

The supervisor exposes an authenticated literal-loopback control socket with a
random private token and an instance nonce. CLI status/stop requests verify the
endpoint and nonce before signaling. This may share small transport primitives
with the app server, but it is a different lifecycle and state owner.

## Scheduling semantics

- Cron is evaluated in an explicit IANA timezone and claimed in UTC.
- `missed` is either `skip` or `run_once`; replaying every missed interval
  is intentionally unsupported in the first version.
- Runs execute sequentially in one supervisor, so they do not overlap.
- A run retries transient failures at most three times with bounded backoff.
  Errors reporting an unknown or uncertain outcome are never retried.
- Definition edits affect only future executions. Every execution binds the
  exact definition digest it used.
- Disabling or deleting a definition does not delete its run history or Work
  bundles. Retention is a separate explicit policy.

Each run starts a normal `gator work --json` subprocess, which revalidates the
source root, outcome contract, provider configuration, connector descriptors,
and credential availability. Remote response content and credentials are not
written to scheduler lifecycle events.

## CLI

```text
gator job add NAME --schedule '0 9 * * 1' --timezone Asia/Singapore \
  --source ./briefing --artifact brief.docx -- 'Prepare the weekly brief'
gator job list
gator job show ID
gator job edit ID --schedule '30 9 * * 1'
gator job enable|disable ID
gator job run ID
gator job history ID
gator job supervisor [--notify=false]
gator job status
gator job stop
gator inbox [--unread]
gator inbox read ENTRY_ID
gator job remove ID --yes        # keeps immutable history by default
```

Cron uses five fields and an explicit IANA timezone. `skip` is the default
missed-run behavior; `run_once` catches up once. Runs are non-overlapping,
retry transient pre-action failures at most three times, and never execute a
connected mutation or publish operation.

## Operational boundary

The supervisor is intentionally foreground-only: it does not install a launchd
unit, systemd unit, login item, or background daemon. `gator job stop` verifies
the private bearer token, literal loopback endpoint, PID, and instance nonce
before requesting shutdown. Stop waits for the active Work subprocess because
mid-run cancellation and restart reconciliation are deliberately deferred.
