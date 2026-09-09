# Durable jobs design

## Decision

Gator does not turn the process-local terminal registry, TUI queue, or app
server into a scheduler. Those components own live resources in one process;
they do not provide durable ownership, missed-run policy, or restart safety.

Scheduled Work will ship only as a separate local supervisor backed by
versioned job definitions and immutable execution records. Until that service
meets the gates below, automation should invoke `gator work` from an existing
supervisor such as launchd, systemd, or CI and retain the emitted bundle ID.

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
private file. The first schema should contain:

```json
{
  "version": 1,
  "id": "weekly-metrics",
  "enabled": true,
  "objective": "Prepare the weekly metrics brief.",
  "source": "/absolute/canonical/source",
  "mode": "draft",
  "external_actions": "draft",
  "artifacts": ["brief.md", "metrics.csv"],
  "connectors": ["metrics"],
  "schedule": {
    "cron": "0 9 * * 1",
    "timezone": "Asia/Singapore",
    "missed_runs": "run_once"
  },
  "concurrency": "forbid",
  "created_at": "2026-09-09T00:00:00Z",
  "updated_at": "2026-09-09T00:00:00Z"
}
```

The stored definition also needs the full normalized outcome contract, provider
selection, maximum turns, and a non-secret connector identity digest. Secrets
remain in Gator's credential store. A definition must never embed an API key,
bearer token, approval, model-generated prompt, or mutable remote tool schema.

Each invocation creates an immutable `intent.json` before model work starts,
appends lifecycle events to `events.jsonl`, and publishes one terminal
`result.json`:

```json
{
  "version": 1,
  "execution_id": "jobrun-...",
  "job_id": "weekly-metrics",
  "definition_sha256": "...",
  "scheduled_for": "2026-09-14T01:00:00Z",
  "started_at": "2026-09-14T01:00:03Z",
  "finished_at": "2026-09-14T01:04:12Z",
  "status": "completed",
  "work_id": "work-...",
  "manifest_sha256": "...",
  "attempt": 1
}
```

The result points to a normal sealed Work bundle; it does not create a second
artifact format. Status transitions in the event log are append-only (`queued`,
`running`, then one terminal state). A supervisor restart reconciles any
orphaned `running` execution by appending `interrupted` before scheduling
another attempt; it never rewrites history.

## State and ownership

The supervisor owns a private subtree below Gator's state directory:

```text
gator/jobs/
  definitions/<job-id>.json
  history/<job-id>/<execution-id>/intent.json
  history/<job-id>/<execution-id>/events.jsonl
  history/<job-id>/<execution-id>/result.json
  supervisor/identity.json
  supervisor/events.jsonl
```

Definitions and records are `0600`; directories are `0700`; publication uses
write, sync, rename, and parent-directory sync. IDs and directory entries are
bounded. Symlinks and non-regular state files are rejected. A single OS-level
exclusive lock owns the store. PID files alone are not process identity and
must never authorize stop or mutation.

The supervisor exposes an authenticated literal-loopback control socket with a
random private token and an instance nonce. CLI status/stop requests verify the
endpoint and nonce before signaling. This may share small transport primitives
with the app server, but it is a different lifecycle and state owner.

## Scheduling semantics

- Cron is evaluated in an explicit IANA timezone and stores the next UTC fire
  time. Daylight-saving transitions are tested as product behavior.
- `missed_runs` is either `skip` or `run_once`; replaying every missed interval
  is intentionally unsupported in the first version.
- `concurrency: forbid` is the only initial policy. If a previous execution is
  still live, the next occurrence becomes a recorded `skipped_overlap` event.
- Retries are opt-in, bounded, and use exponential backoff. `unknown` external
  action evidence is never automatically retried.
- Definition edits affect only future executions. Every execution binds the
  exact definition digest it used.
- Disabling or deleting a definition does not delete its run history or Work
  bundles. Retention is a separate explicit policy.

Before each run the supervisor revalidates the source root, outcome contract,
provider configuration, connector descriptors, and credential availability.
A failed preflight produces a durable failure record without invoking a model.
Remote response content and credentials are never written to scheduler logs.

## Intended CLI

```text
gator job add ID --file job.json
gator job list
gator job show ID
gator job enable|disable ID
gator job run ID                 # foreground, creates normal execution record
gator job history ID
gator job supervisor start|status|stop
gator job remove ID --yes        # keeps immutable history by default
```

`job run` is the first implementation slice because it proves definition
validation and execution records without pretending a daemon exists. The
supervisor comes only after restart, lock, time, and shutdown behavior is
tested.

## Release gates

Scheduled Work is usable only when all of the following are demonstrated:

1. schema round-trip and forward-version rejection;
2. atomic concurrent definition updates and single-supervisor ownership;
3. restart recovery for every non-terminal execution state;
4. timezone and missed-run fixtures, including DST boundaries;
5. overlap suppression and bounded retry fixtures;
6. missing/expired credential preflight with no model invocation;
7. immutable links from execution records to verified Work manifests;
8. safe stop during model work and during artifact sealing;
9. no path from a job definition to unattended `approve`; and
10. install, upgrade, log rotation, and uninstall behavior on macOS and Linux.

Until these gates pass, Gator should be explicit that it supports foreground
Work and externally supervised headless runs—not built-in persistent jobs.
