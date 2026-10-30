# Agent continuity implementation

Implemented 30 October 2026. Gator remains a general-purpose CLI agent with
native local tracking; none of these features requires a connector.

## Durable next actions

Goal watches can retain a checked `ready`, `waiting`, `blocked`, or `idle`
continuation, an existing plan step, and a wake time. The existing supervisor
honors that wake even when input fingerprints have not changed. Reopening the
store preserves the decision; quiet polls no longer hide it in goal activity.
The same reviewed objective, tools, permissions, daily budgets and end date apply.

Wakes must be 5 minutes to 7 days ahead and run at the first eligible poll.
A ready step cannot be completed, blocked, or have unresolved dependencies.
Three successive ready decisions for the same step with unchanged inputs hold
continuation for review. A changed policy or intervening failure cannot reuse
an obsolete wake. Draft creation never implies delivery or user task completion.

Use `gator goal activity GOAL_ID` to inspect retained decisions and checks.
The existing goal watch/supervisor commands remain the activation path. This is
bounded coordination through the existing Work runner, not an unrestricted
background agent or a guarantee that a long-running goal will succeed.

## Cross-record conflict review

`suggest_change` supports `kind: conflict`: 2–8 distinct notes/plans, exact
quotes, source identities and revisions, plus the discrepancy explanation.
The host checks scope, excluded notes, revisions and quote presence. Pending
identical findings reuse their review identity. Changed sources invalidate
acceptance. `gator suggestion show ID` includes the current source check.

Accepting a finding acknowledges review **without editing either source**.
Corrections use separate guarded note/plan proposals. Quote integrity is not
proof of semantic contradiction or evidence that one source is true. Detection
still depends on the model inspecting relevant records; it is not an exhaustive
consistency engine across every stored fact. Scoped goal runs cannot inspect
another goal's notes through these references.

## Native recurrence and dependencies

Steps can depend on another project's step using `GOAL_ID/STEP_ID`. Local IDs
still work. A graph lock and validation reject cross-project cycles, missing
links, and deleting a referenced step. A scoped watch gets the explicitly linked
step's status, not another project's files or notes. A model must inspect both
plans from the main chat before proposing a new cross-project link.

Daily/weekly blocks use an explicit IANA timezone and interval 1–52. Start/end
form an elapsed duration up to 24 hours; local start time is preserved through
DST. Nonexistent spring-forward times are skipped. Ambiguous fall-back times
use Go's timezone resolution, with the original anchor instant preserved.
An optional `until` bounds nominal starts. A recurring block cannot also have a
separate deadline; use a separate milestone step for that deadline.

Examples (replace IDs with your retained records):

```sh
gator goal plan GOAL_ID add --title 'Weekly preparation' \
  --start 2026-11-02T09:00:00+08:00 --end 2026-11-02T10:00:00+08:00 \
  --repeat weekly --interval 1 --timezone Asia/Singapore
gator goal plan GOAL_ID occurrence STEP_ID 2026-11-02 done
gator goal plan GOAL_ID occurrence STEP_ID 2026-11-09 skipped
gator goal plan GOAL_ID occurrence STEP_ID 2026-11-16 pending \
  --start 2026-11-17T09:00:00+08:00 --end 2026-11-17T10:00:00+08:00
gator goal plan GOAL_ID step STEP_ID pending --depends-on OTHER_GOAL_ID/OTHER_STEP_ID
```

Occurrence edits affect one nominal local date. Completing the base step ends
the series. Explicit overrides remain in the plan (maximum 128 per step);
clearing recurrence requires clearing those overrides explicitly first.
Expansion is bounded to 32 days; agenda queries allow 31 days and show current
and upcoming occurrences. Past occurrences remain derivable from the rule and
overrides. These are native scheduling records, not automatically created
reminders, recurrence-specific dependency edges, or inferred completions.
The existing plan limits (32 steps and 64 KiB) still apply.

## Companion recovery

Authenticated reads can restart an idle crashed backend, bounded to 3 attempts
per 5 minutes with a 10-second backoff. Active/queued work, pending approvals,
handoffs and unconfirmed actions require host review. No action is automatically
replayed. An exact stable message admission can be retried on a live backend;
other mutations stay blocked after an unconfirmed action.

`node companion/doctor.mjs ORIGIN OWNER_TOKEN_FILE` checks authenticated listener
readiness without opening chats or calling a model. It reports counts without
identities/secrets. Exit 2 flags stopped sessions, exit 1 a failed probe. Existing
Linux/macOS service preparation remains explicit and non-activating. See
[companion setup](../companion/README.md#listener-readiness-and-idle-recovery).

The listener probe does not establish provider or job-supervisor health.
Actual remote host deployment, physical device-off operation, native Windows
service/ACL acceptance and power-loss testing remain unverified by this work.

## Validation and rollback

- Affected Go packages passed: goals, jobs, reconciliation, suggestions, tools,
  Work runner, Work application and CLI; the CLI suite took about 212 seconds.
- Focused restart/quiet-poll and activity tests passed after the final wake-view fix.
- Companion browser scenarios passed (6 Chromium cases), plus real Go transport,
  revocation, replay prevention and recovery checks. Three opt-in service/boot
  tests were skipped. The health integration probe initially timed out; its 3s
  deadline was shorter than backend startup/catalog limits. It passed after
  alignment to 25s, preserving all state and zero-model-call assertions.
- Live OpenAI synthetic reports and runner instructions are in
  [internal-agent evals](../scripts/internal-agent-eval/README.md).
- The local terminal runtime was rebuilt. Native CI includes the new recurrence,
  reconciliation, continuation and companion recovery coverage.

Code changes are separate commits for rollback. Persisted plans now contain
recurrence/occurrence fields; attempts and proposals also have new optional
fields. Older binaries with strict readers may reject these newer records.
Back up the state profile before downgrading; Git rollback alone does not revert
user data or accepted plan changes. No real user records were changed by evals.

These checks demonstrate specific mechanics, not product parity with Dot/Muse,
longitudinal memory quality, notification precision, or production reliability.
