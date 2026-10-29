# Durable jobs design

**Interface status:** The [terminal distribution](TERMINAL.md) exposes these
retained command families as `gator job ...`. Source-checkout examples below use
the Go runtime's explicit internal opt-in; see [development instructions](DEVELOPMENT.md).

## Implemented boundary

Gator does not turn the process-local terminal registry, TUI queue, or app
server into a scheduler. Those components own live resources in one process;
they do not provide durable ownership, missed-run policy, or restart safety.

Scheduled Work runs through a separate local supervisor backed by versioned job
definitions and immutable attempt records. The user can start it in a foreground
terminal with `gator job supervisor`. On macOS, `gator job launchagent install`
installs an opt-in per-user service; on Linux, `gator job systemd install` installs
a per-user systemd service. Both keep the supervisor running while the
host is on and the user manager is active. Gator does not stretch its process-local
TUI queue into a scheduler.

This is a product boundary. The supervisor reconciles interrupted attempts and
applies the configured missed-run policy after a restart. It does not run when
the machine is off. Reinstall a per-user service when the executable path
changes.

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
private file. Its version-2 shape extends the following legacy example with `snapshot_id`, captured project configuration, complete `code_policy`, `limits`, and `web_origins`:

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
provider/model selection, maximum turns, and connector IDs. New goal checks
using a Google OAuth connector store a SHA-256 pin of the Google account's
OIDC subject after a live identity check. Reauthorization of the same account
keeps the check valid; a different account stops it before connected reads.
Older definitions retain their refresh-grant pins and still fail closed on
grant changes. Secrets remain in Gator's credential store. A definition never embeds an API key,
bearer token, refresh token, or approval.

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
and running events remain available for inspection. Supervisor startup marks
interrupted attempts and publishes their history/inbox result without repeating
unknown effects.

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

`gator job add ... --change-only` and `gator goal schedule ID ... --change-only`
opt into a local input gate. Before each scheduled run, Gator hashes the
selected regular files, goal context and plan, visible notes, enabled skills,
and active memories. If they exactly match the most recent successful run
under the same job policy, it records an `unchanged` attempt and skips the
model call and notification. A failure or changed job policy runs normally.
The local gate rejects ordinary jobs with web origins or connectors, because those sources
can change independently of local files. Connected goal watches use a separate bounded
connector poll and combine its content digest with the local gate. Both require refreshed source
capture. The file scan is capped at 10,000 files, 16 MiB per file, and 64 MiB
total; an oversized or non-regular source fails the check rather than claiming
that nothing changed. `.git` is excluded. Use ordinary cron jobs for checks
whose result depends on time passing even when local inputs stay the same.

`gator goal watch ID [--task TEXT] [--artifact venue-reply.md]` creates one
active local watch for a goal. The supervisor polls its selected local files,
saved context, and plan once per minute while running. A due plan step or
`review-at` crossing also changes the watch input, even when files do not.
The first poll runs the model; later identical inputs are recorded as
`unchanged` without a model call. This is bounded polling, not a filesystem
event subscription, and it stops while the host or per-user service is off.
The watch resumes one missed check after a restart. Existing goal review
reminders are owned by the watch while it is enabled, so one due review does
not create a second inbox entry.

`gator goal notifications ID` shows that goal's desktop delivery policy.
`--desktop off` mutes desktop alerts for that goal, while `--quiet
22:00-08:00 --timezone Asia/Singapore` suppresses them during local quiet
hours. `--urgent-only on` sends desktop alerts only for failed jobs and goal
watch results explicitly marked urgent; ordinary completed jobs and due plan
reviews stay in the inbox. Use `--urgent-only off` to restore all eligible
desktop alerts. A watch's urgent label is a model judgment, not a verified
severity assessment; older watch responses without the field count as routine.
These settings only narrow desktop alerts when the supervisor's global
notification option is enabled. Completed results and failures still go to
the terminal inbox; quiet hours do not defer or drop them. The same policy
applies to due plan-review reminders. A reminder retained during quiet hours
is not replayed as a desktop notification when quiet hours end.

A connected watch can select up to four read surfaces with `--connector ID`.
An HTTP JSON connector polls its configured resource. A Google connector needs
`--gmail-query QUERY`, `--calendar-days N` (1-30), or both; Gmail and Calendar
watches each read up to 500 selected items across bounded pages. If either
source exceeds that limit or returns invalid pagination, the poll fails and
surfaces an error instead of claiming nothing changed. Connected watches poll
every five minutes while the local supervisor runs. Their combined input hash
persists across restarts. Gmail retains a mailbox history cursor to avoid
rereading unchanged message details; Calendar rereads its selected window on
every poll. Neither watch installs a push subscription. Connector
read permissions and credentials are checked on each poll. The model runs only
when the local or connected content hash changes, then reads current evidence
again in Work. A source can change between those two reads; the next poll will
detect it. Completed attempts also retain bounded item IDs and SHA-256
fingerprints, then pass an added/updated/removed item list to the next model
check. This metadata helps the model look at the right message or event; it
does not retain message bodies in scheduler state or prove that a change matters.
Unchanged attempts keep only the combined input digest. The host does not
verify whether a changed message is relevant.

A watch asks Work for an explicit `attention` decision and an evidence-backed
summary. A quiet completed run stays out of the inbox. A draft artifact or a
successful reviewed suggestion always surfaces, even if the model marks its
response quiet; malformed responses also surface. A watch can use selected
read-only connector material but cannot send mail or approve a connected action. With
`--artifact`, Work must create that draft on each model run, so use it for
tasks where each source change merits a draft. The user must still accept a
plan suggestion explicitly. Content relevance and model decisions require
live evaluation; the host only verifies the gating and review boundary.

Completed runs also retain a short final-response summary and an output
digest. The inbox suppresses exact repeated results under the same job policy;
it does not attempt semantic equivalence between different phrasings.
Use `gator job inbox show ENTRY_ID` to inspect a retained result and
`gator job inbox rate ENTRY_ID useful|noise|wrong|clear` to record or correct
your judgment of a surfaced job alert. `gator job inbox report GOAL_ID` counts
the shown job alerts and their ratings for one goal; `gator goal activity GOAL_ID`
shows the same count. Reading an alert does not rate it, and quiet checks do
not create an inbox entry. These ratings describe only alerts that were shown;
they cannot measure missed important changes or establish alert accuracy on
their own.

In interactive Gator, `ctrl+b` or `/inbox` opens a freshly loaded inbox panel.
Each row shows its entry ID, read state, and rating. Press `esc` to return to
chat, then use `/inbox show ENTRY_ID`, `/inbox read ENTRY_ID`, or
`/inbox rate ENTRY_ID useful|noise|wrong|clear`. The panel refreshes when reopened
and after an inbox action, including actions through `/jobs inbox ...`.
Urgent job alerts show an explicit marker in the CLI and terminal inbox; a
user rating is independent of that model-assigned label.

`gator job quiet-audit JOB_ID` lists the newest 50 completed goal-watch checks
where the model explicitly chose quiet, along with their attempt IDs, retained
connected-item change counts, summaries, and Work IDs. It counts every retained
quiet decision, even when only 50 rows are displayed. After inspecting the
underlying source and Work run, use
`gator job quiet-audit JOB_ID ATTEMPT_ID missed --note 'reason'`
to record a missed important change, or `... correct` to confirm that staying
quiet was appropriate. `... clear` removes a previous judgment. The audit
excludes unchanged-input skips, because no new model decision was made, and
excludes alerted and failed attempts. Unreviewed checks are not assumed correct
or missed. These user labels can support a supervised recall study only when
the relevant quiet checks are actually reviewed; counts alone are not recall.

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

Each run calls the typed Work service directly, which revalidates the
source root, outcome contract, provider configuration, connector descriptors,
and credential availability. Remote response content and credentials are not
written to scheduler lifecycle events.

## CLI

To route a CLI-created check's updates to an existing chat, use the terminal
launcher with that chat's exact ID:

```sh
gator job add 'Weekly review' --conversation work-EXISTING_ID \
  --schedule '0 9 * * 1' --timezone Asia/Singapore --mode inspect \
  -- 'Review this project and report changes needing attention'
```

The chat must exist in the selected profile. Its folder becomes the default
source; an explicit `--source` must identify that same directory. Later source
edits must preserve this binding. Updates appear in the originating chat in
Gator and the companion, with separate execution conversations for each run.
Opening an execution conversation does not grant it the originating chat's
follow-up controls. The command confirms the selected chat ID after saving.
Without `--conversation`, jobs retain their existing host inbox behavior; the
companion does not expose a global inbox for those unbound updates. This option
does not start the supervisor, configure remote access or send a notification
immediately. The configured execution host still needs to stay online.

```text
GATOR_INTERNAL_COMMANDS=1 go run ./cmd/gator job add NAME --schedule '0 9 * * 1' --timezone Asia/Singapore \
  --source ./briefing --artifact brief.docx -- 'Prepare the weekly brief'
GATOR_INTERNAL_COMMANDS=1 go run ./cmd/gator job list
GATOR_INTERNAL_COMMANDS=1 go run ./cmd/gator job show ID
GATOR_INTERNAL_COMMANDS=1 go run ./cmd/gator job edit ID --schedule '30 9 * * 1'
GATOR_INTERNAL_COMMANDS=1 go run ./cmd/gator job enable|disable ID
GATOR_INTERNAL_COMMANDS=1 go run ./cmd/gator job run ID
GATOR_INTERNAL_COMMANDS=1 go run ./cmd/gator job history ID
GATOR_INTERNAL_COMMANDS=1 go run ./cmd/gator job supervisor [--notify=false]
GATOR_INTERNAL_COMMANDS=1 go run ./cmd/gator job status
GATOR_INTERNAL_COMMANDS=1 go run ./cmd/gator job stop
GATOR_INTERNAL_COMMANDS=1 go run ./cmd/gator job inbox [--unread]
GATOR_INTERNAL_COMMANDS=1 go run ./cmd/gator job inbox show ENTRY_ID
GATOR_INTERNAL_COMMANDS=1 go run ./cmd/gator job inbox read ENTRY_ID
GATOR_INTERNAL_COMMANDS=1 go run ./cmd/gator job inbox rate ENTRY_ID useful|noise|wrong|clear
GATOR_INTERNAL_COMMANDS=1 go run ./cmd/gator job inbox report GOAL_ID
GATOR_INTERNAL_COMMANDS=1 go run ./cmd/gator job quiet-audit JOB_ID
GATOR_INTERNAL_COMMANDS=1 go run ./cmd/gator job quiet-audit JOB_ID ATTEMPT_ID missed --note 'The deadline changed'
GATOR_INTERNAL_COMMANDS=1 go run ./cmd/gator job quiet-audit JOB_ID ATTEMPT_ID correct
GATOR_INTERNAL_COMMANDS=1 go run ./cmd/gator job quiet-audit JOB_ID ATTEMPT_ID clear
GATOR_INTERNAL_COMMANDS=1 go run ./cmd/gator job remove ID --yes        # keeps immutable history by default
GATOR_INTERNAL_COMMANDS=1 go run ./cmd/gator job launchagent install    # macOS, opt-in per-user supervisor
GATOR_INTERNAL_COMMANDS=1 go run ./cmd/gator job launchagent status
GATOR_INTERNAL_COMMANDS=1 go run ./cmd/gator job launchagent remove --yes
GATOR_INTERNAL_COMMANDS=1 go run ./cmd/gator job systemd install        # Linux, opt-in per-user supervisor
GATOR_INTERNAL_COMMANDS=1 go run ./cmd/gator job systemd status
GATOR_INTERNAL_COMMANDS=1 go run ./cmd/gator job systemd remove --yes
```

Cron uses five fields and an explicit IANA timezone. `skip` is the default
missed-run behavior; `run_once` catches up once. Runs are non-overlapping,
retry transient pre-action failures at most three times, and never execute a
connected mutation or publish operation.

## Operational boundary

The supervisor runs in the foreground by default. The `launchagent` and
`systemd` commands install or remove per-user services on macOS and Linux,
respectively. Neither enables Linux lingering or runs while the machine is off.
`gator job stop` verifies
the private bearer token, literal loopback endpoint, PID, and instance nonce
before requesting shutdown. Stop cancels the active Work context and joins its
children. Startup reconciliation marks interrupted attempts for inspection; it
does not automatically repeat their effects.

## Frozen inputs and recoverable attempts

`job add --refresh-snapshot=false` captures source and selected project
configuration at creation. The default refreshes for each attempt. `--contract`
accepts the entire contract JSON. Code policy remains an internal concern.
Model request, token and wall limits and repeatable `--web-origin` grants
are retained. These values reach Work without a CLI-flag round trip.

`job edit ID --source PATH` captures the explicitly selected source again for a
frozen job, including when PATH is unchanged. Other edits preserve its capture.

The supervisor persists schedule intent before advancing its watermark. Each
bounded retry has a stable Work run reference retained before execution. Only
transient provider failures are retried (at most three attempts); policy, task,
validation and cancellation failures are not blindly retried. Completed or
interrupted attempts remain in history and inbox. See [state recovery and
migration](WORK_DEPTH_MIGRATIONS.md) for the crash boundary and old-state semantics.

### Health in the requesting chat

A check created from chat reports its current availability above that chat's
composer. Expand it to inspect the last successful check for the current
settings, request limits, unsuccessful attempts and observed supervisor gaps.
A heartbeat gap describes the background engine's availability; it cannot
reconstruct source changes during that interval.

Repeated unsuccessful polls share one saved health incident. A limit changing
to an execution failure, or the reverse, starts a different notice. Recovery is
saved even when the next successful check stays quiet. These health notices are
separate from alert-quality feedback, and recovery does not mark earlier notices
read or done. All individual attempts remain available for inspection.

OS notifications use an at-most-once handoff claim. A process stopping during
the OS handoff may omit a toast; the saved in-app update remains available.
