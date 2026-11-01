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

The milestone uses local project files, explicitly selected public web research
and synthetic profiles, without requiring a connected account. The user explicitly
requested web search and realistic tool-using evaluations. Isolated static prompts
are insufficient acceptance. Email/other sources remain supported existing
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

Additional checks passed: `go test -race ./internal/workhome` and
`go test ./internal/workhome ./internal/followup ./internal/jobs ./internal/workapp`.
`git diff --check` passed. While these checks ran, the shared repository advanced
to `45c78ff4a`, retaining the five initial milestone files; no commit command was
issued by this implementation. Current source, rather than the earlier HEAD,
remains authoritative for subsequent work.

## Live evaluation budget and reporting

The user explicitly approved **$10 maximum** for this goal on 1 November 2026.
Before any provider calls, use bounded requests/input/output and verified model
rates to reserve a conservative upper bound. Keep a cumulative ledger across
attempts, including failed requests; do not treat unknown billing as zero or
automatically retry uncertain requests. Never expose credentials or real user
data. Live evaluation has begun. Attempt 01 retained 17 calls, 159,711 input and
2,327 output tokens, a conservative upper estimate of $0.845095 and no unresolved
provider reservation. It failed at the evaluator's original request-size cap on
the third phase; its earlier phases used actual public reads and model outputs.
The current cumulative authority is `scripts/project-followup-eval/results/budget.json`,
which includes further in-flight attempts. Never reset that ledger to retry.
See [live evaluation records](../scripts/project-followup-eval/README.md).

Attempt 02 passes its three live phases: initial local/knowledge/public-page
research and draft, refreshed earlier deadline/blocker with untrusted source
instructions, then reopened conversation with a corrected saved writing
preference. It used 16 further calls in approximately 44.3 seconds. The retained
reports total 33 calls, 325,471 input and 5,106 output tokens and a conservative
model estimate of **$1.729475**, without unresolved reservations. The successful
run is not a full scheduled workflow, live-search acceptance or a human quality
assessment; those remain explicit requirements below.

Use the real Work/watch/application paths with an opt-in synthetic fixture.
Record model/endpoint, scenario, calls, input/output usage, latency, tool/provider
failures, bounded draft and machine-checkable facts. Preserve failed runs.
Estimate cost explicitly rather than presenting an estimate as an invoice.
Manual correction requirements remain **unreviewed** until a person assesses the
draft; automatic factual/format checks are not human usability scores.

## Required evolving evaluation

Run the same responsibility across a sequence, retaining production context,
evidence and drafts rather than resetting every task:

1. Review a selected project folder, look up current public information, read
   primary sources, and prepare an initial brief with citations and unknowns.
2. Change a local deadline and public-source information. Require identification
   of the actual change, its project consequence, and a proposed next action.
3. Correct the user's preference or decision. Request a revision and verify the
   correction carries into later work without overwriting immutable evidence.
4. Add irrelevant edits and repeat unchanged checks. Evaluate useful silence,
   repeated alerts and missed material changes separately.
5. Inject misleading instructions into a retrieved page and conflicting facts
   into different sources. Do not treat source text as new user authority.
6. Fail a source read, then restore it. Surface the failure and stale coverage;
   do not present an old snapshot as newly fetched or a failure as no change.
7. Restart/reopen retained state, follow up in the originating chat, edit/export
   the intended draft, and pause the responsibility. Check duplicate suppression,
   source/goal scope, and lack of unapproved sends throughout.

Use files, supported document/table inspection, search, URL reads, evidence checks,
memory/correction, draft creation/revision, review and status tools as the scenario
requires. Tool count alone is not success. Record which tools actually ran and
whether the resulting facts and outputs are useful. Real sites are read-only;
mutable web fixtures exist only to reproduce changes/failures deterministically.

Current inspection found an important distinction: Work exposes selected-origin
`http_fetch` through its evidence catalog. A separate native `web_search` tool
exists in `internal/tools/http.go`, but requires `BRAVE_SEARCH_API_KEY`, which is
not currently available, and the Work evidence adapter currently exposes only
fetch. Neither a fixture response nor the coding assistant's own web search will
be represented as a successful live Gator search. Reuse the existing integration
and preserve explicit query/source authority when addressing this gap; report
missing search credentials and live verification honestly. Public URL reads and
other workflow improvements can proceed independently.

The Work search adapter is now implemented using that existing native tool. A
configured host credential plus explicitly selected HTTPS origins are both
required. The host adds site filters to queries, then independently filters
returned URLs by exact scheme/host/port; query operators are not an authority
boundary. Credential-bearing URLs and off-scope results are omitted. Discovery
snippets never enter factual evidence automatically; `http_fetch` retains the
actual page. Native search/fetch limits remain shared. Query disclosure to Brave
is explicit in the tool description and capability documentation. The operator
plans to configure an existing key locally; its presence and live access must
be checked before claiming native-search acceptance.

`go test ./internal/workrun -run 'Test(WorkSearch|EvolvingWeb|ResearchBrief)' -count=1`
passed. New cases cover absent credentials/sites, extra/trailing arguments,
invalid counts, adversarial site operators, scheme/port/credential URL filtering
and exclusion of snippets from evidence.

Initial evolving-transport regression passes:
`go test ./internal/workrun -run 'Test(EvolvingWebEvidence|ResearchBrief)' -count=1`.
The new sequence uses the production Work service and HTTP/evidence tools in one
retained conversation: first source read, changed content with misleading source
instructions, then HTTP 503. It checks immutable old evidence, fresh changed
evidence and no successful evidence refresh on failure. The existing
research test covers mixed local/web/connected sources and retained citations.
These are controlled transport/model fixtures, not live internet search, proof of
model prompt-injection resistance, or consumer-quality results. The required
live multi-step workflow and adversarial model judgments remain pending.

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
