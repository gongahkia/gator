# Project follow-up: first consumer workflow

Status: implementation and acceptance in progress, 1 November 2026.
Starting source: `e4f2b33b1`, clean worktree. This is an active milestone,
not a claim of product readiness or completed user acceptance.

## Outcome

A solo user selects a project folder and asks Gator to track meaningful deadline,
decision and blocker changes. Gator reviews scope, timing, permissions and limits,
establishes a useful initial understanding, and prepares a short project update
when warranted. Updates return to the requesting conversation. The user can
correct context, review/edit the draft, and pause or change the agreement.
Unrelated edits and repeated concerns should stay quiet. Failed checks must never
look like evidence that nothing changed.

The first milestone uses local project files and synthetic profiles, without
requiring a connected account. Email/other sources remain supported existing
capabilities but are not prerequisites for first value. No terminal expansion,
new daemon, authority store, database, provider routing change or automatic send
is authorized by this milestone. Existing scheduling and execution lifetimes stay
intact.

## Existing implementation to reuse

- `internal/followup` reviewed agreements, local scope and per-check/day budgets.
- `cmd/gator/work_app_followup.go` conversational prepare/review/apply adapter.
- `cmd/gator/work_followup.go`, existing jobs/watch runner and local supervisor.
- Work source snapshots, evidence, artifact validation and retained revisions.
- Existing saved knowledge/corrections and scoped recall; no second memory system.
- Existing chat updates, reply binding, file preview/edit/export and receipt paths.

Older competitive audits describe some now-implemented features as missing.
Current code already includes managed sessions and originating-chat updates.
Do not rebuild these from an old checklist. Conversely, their existence does not
prove that a consumer gets a useful outcome without coaching.

## Acceptance requirements

| Product gap | Evidence required before completion | Current status |
| --- | --- | --- |
| Complete recurring workflow | Reviewed setup, initial result, material change, quiet/noise, repeat suppression, correct chat update, reviewed draft, correction, pause/recovery | Existing component coverage; integrated acceptance still required |
| First useful result | A discoverable path from selected files to a first useful output; measure steps/time in fixture and distinguish human setup time | Current setup exists; assess first-run friction and initial-check delay |
| Proactive judgment | Source-grounded change vs noise cases, repeated changes, stale inputs and injection; retain failed live attempts | Existing watch/quiet gates; new workflow-level evaluation required |
| Correct personal context | Correction survives a later check; prior/stale text cannot silently supersede it; unrelated goals excluded | Existing memory tests/live lifecycle; workflow-specific evidence required |
| Useful output | Concise update with correct dates, owners, unknowns and next action; review/edit/export correct version; report human repair as unreviewed until actually rated | Artifact mechanisms exist; actual draft content needs evaluation |
| Dependable availability | Last successful/attempted check, overdue/offline/ended states, actionable recovery; observation never starts work | First patch implemented and focused tests passing; broader verification pending |

## Initial concrete correction

The availability projection previously always displayed `schedule.Next(now)`
while online. Between cron boundaries, a missed `run_once` check could therefore
appear to be scheduled only in the future. It now reuses `jobs.Due` to expose the
actual due slot and a derived `DueNow`/“due since” indicator. It does not claim or
run the job, change its schedule or override skip/expiry/paused/offline policy.

The conversational follow-up view now includes last successful check, last
attempt, due-now state and the existing next action. Unknown timestamps are
omitted, allowing the agent to distinguish “never checked” from a real success.
No persisted schema or execution behavior changes.

Regression evidence so far:

```sh
go test ./internal/workhome ./cmd/gator -run 'Test(CheckHealth|CheckAvailability|ChatFollowup|ReviewedFollowup|ReviewedProjectFollowup|DailyFollowup)' -count=1
```

Both packages passed. New cases check overdue vs skip/offline semantics,
absence of side effects, health projection and omission of unobserved timestamps.
These tests prove health reporting, not the whole milestone.

## Live evaluation budget and reporting

The user explicitly approved **$10 maximum** for this goal on 1 November 2026.
Before any provider calls, use bounded requests/input/output and verified model
rates to reserve a conservative upper bound. Keep a cumulative ledger across
attempts, including failed requests; do not treat unknown billing as zero or
automatically retry uncertain requests. Never expose credentials or real user
data. No live calls have been made for this milestone yet; initial spend is $0.

Use the real Work/watch/application paths with an opt-in synthetic fixture.
Record model/endpoint, scenario, calls, input/output usage, latency, tool/provider
failures, bounded draft and machine-checkable facts. Preserve failed runs.
Estimate cost explicitly rather than presenting an estimate as an invoice.
Manual correction requirements remain **unreviewed** until a person assesses the
draft; automatic factual/format checks are not human usability scores.

## Remaining work and completion boundary

1. Exercise the complete current path to identify actual integration/UX gaps.
2. Implement improvements necessary for first useful result and useful updates.
3. Add a reproducible scenario sequence using production services, deterministic
   adversarial cases and bounded live-model evaluation.
4. Verify relevant Go/host/frontend paths and inspect actual rendered outputs.
5. Document actual results, remaining native-device/user acceptance and progress
   against all six gaps. Complete the goal only after every milestone requirement
   is evidenced; generate one copy-ready next goal at that point, not now.

Physical-phone, multi-week consumer retention, live connected-account behavior
and native platform acceptance are distinct from local fixture success. Nothing
in this record claims those occurred.
