# Personal-agent product implementation — November 2026

User-authorized scope: implement all fifteen findings from the 3 November audit,
verify each change, document its evidence and make atomic commits for rollback.
Starting checkout: `6814eabf2`. README files must remain untouched. This record
tracks the entire scope; completing a slice does not close its broader finding.

The user authorized **US$10 total for new live model/search evaluations** on
3 November. Existing October/November evaluation allowances are not reset or
reused. New calls need durable conservative reservations before dispatch, actual
usage afterward, and a cumulative ledger retaining failures and unknown charges.
Free fixtures do not establish live-model or user acceptance.

New allowance authority: `scripts/competitive-eval/authorization.json`; cumulative
ledger: `scripts/competitive-eval/results/budget.json`. The existing opt-in Work
evaluation transport is reused with this new directory, a US$9.50 admission
ceiling and durable unknown-request reservations. Conservative model ceilings
are US$6/M input and US$24/M output, covering cache-write and regional uplift.
The selected `gpt-5.6` alias and rates were checked against [the official model
page](https://developers.openai.com/api/docs/models/gpt-5.6-sol) and
[API pricing](https://developers.openai.com/api/docs/pricing) on 3 November.

## Requirements and acceptance

| Audit item | Implementation and required acceptance | State |
| --- | --- | --- |
| 1. Proactive usefulness | Fix pause/health and unnecessary-alert failures; complete evolving responsibility and coding repair/review journeys; unseen cases, human repair assessment and multi-day precision/miss evidence | In progress |
| 2. Availability | Supported always-on installation, discoverable enrollment, health and recovery; client-off work and host restart verified; precise local-resource ownership | Open |
| 3. Durable execution | Persist safe workflow boundaries, reconcile uncertain effects, resume eligible work without repeating confirmed effects; crash/update/provider-loss journeys | Open |
| 4. First value | Task-led defaults, model/account/search readiness and inline recovery; clean installation to useful result without developer intervention | Open |
| 5. Connected jobs | Complete calendar and email workflows, including threading, recipients, attachments, drafts, reviews, account scope, reconciliation and real-service acceptance | Open |
| 6. Research | Reviewed public discovery beyond preselected sites, sensible shared budgets and in-task scope expansion; unknown-source research with provenance and network protections | Open |
| 7. Browser | Bounded task/session permissions, private sign-in and useful session continuity; representative multi-page jobs with measured interruption count | Open |
| 8. Delivery | Durable notification outcomes/retry/reconciliation, channel choices and mobile delivery; disconnect, OS failure and actual-device acceptance | In progress: bounded OS handoff retry verified locally |
| 9. Conversation | Responsive questions and task routing during work, preserving owner-bound steering, approvals and resource limits | Open |
| 10. Memory/adoption | Scalable scoped reconciliation, outage/ambiguity behavior, realistic accumulated profiles, multilingual history and reviewed competitor-data import | Open |
| 11. Costs | Task/recurring money estimates, shared enforceable ceilings and visible unknown costs across model/search/delegation | Open |
| 12. Deliverables | Representative template revisions and independently reviewed briefs, sheets and decks; factual/layout defects and repair time recorded | Open |
| 13. Release/platform | Signed install/update/rollback lifecycle; supervisor drain/restart; acceptable Windows command isolation and native evidence | Open |
| 14. Privacy/assurance | Precise threat/data-flow model, platform credential protection, and adversarial cross-tool/content evaluations before expanding authority | In progress: notification command text isolated from program source |
| 15. Capability truth | Current shared capability descriptions for onboarding/help/model guidance; eliminate calendar contradictions and test user-visible claims | Open |

Acceptance depending on provider registrations, signing identities, physical
devices, deployment or independent human judgment stays explicitly unverified
until that evidence exists. Implement independent engineering work while those
acceptance steps remain open. Do not treat a green narrow test as product parity.

## Implementation evidence

### Paused scheduling and observed-success continuity (item 1)

The retained live watch report exposed two independent defects: `Due` ignored
`Enabled`, and pausing changed the digest used to match observed successful checks.
The scheduler and legacy claim path now reject disabled definitions. A separate
check-policy digest normalizes only the enable flag for evidence comparisons;
the reviewed-settings digest still binds pause/resume so concurrent edits are
rejected. Existing enabled-definition digests remain compatible.

Regression coverage uses persisted real job attempts and reopening stores. It
checks exact cron slots, missed-slot catch-up, stale claims, unchanged watermarks,
pause/resume success, and invalidation after substantive source/objective edits.
Validation: `go test ./internal/jobs ./internal/workhome` and both full package
race suites pass. Focused command integration tests matching
`Test(GoalWatch|ChangeOnly|ChatFollowup|ReviewedFollowup|DailyFollowup|Check)` pass.
`git diff --check` passes. No model or search requests were made for this slice.

Rollback: revert its code/documentation commit. No new persisted fields or data
migration are introduced. An already admitted check may finish after pause; this
change prevents future scheduler admission, not cancellation of active work.

### Current-run effects and source/citation continuity (item 1)

Three retained live attempts passed 18/19 checks and preserved the remaining
cosmetic-change failure. The model was already staying quiet and not rewriting
the draft. Inspection exposed host defects: all inherited artifacts forced
attention, replayed suggestions could do the same, and carried-forward evidence
could shadow the freshly read source at the same locator. A further mismatch
exposed a phantom final line in `read_file` after a terminating newline.

Work outcomes now expose the verified parent's artifact identities/root. Watches
compare output hashes/sizes and use current execution tool events, preserving
new/changed/removed output and fresh suggestions as reviewable effects. Existing
drafts stay retained without forcing another alert. Current file reads bind
evidence to current frozen bytes. Line references can use verified current/parent
drafts; every path, content hash and physical line bound is checked. The reader
reports physical lines while preserving the terminating newline and full byte
coverage. Invalid references still require review.

`goal_watch_revision_test.go` exercises real Work execution across a baseline,
cosmetic refresh and changed deadline, with actual artifacts, replay and evidence.
Additional regressions cover historical/current suggestions, removed output,
changed parent bytes and unowned/out-of-bounds references. Physical-line tests
also check full byte coverage needed by coding review.

The live run `watch-2026-11-03T05-00-54.138Z.json` passes **19/19 checks** in
approximately 52.2 seconds. Baseline and material changes surface; identical and
cosmetic changes stay quiet; source failures report honestly, recovery rechecks
without another concern, and pause/health checks pass. All three earlier failed
reports remain beside it. Cumulative new accounting is 70 model requests,
728,052 input / 5,339 output tokens, **US$4.496448 upper estimate**, zero search
charges/reservations and zero unresolved model reservations. These estimates
include failed attempts and conservative regional/cache-write rates; they are
not a provider invoice. No connected accounts or real user data were loaded.

The original engineering projection checkpoint was included in the user's
intervening commit `cbbf6d7ac`; it is preserved. This implementation extends it
with measured fixes. Item 1 remains open for the coding completion journey,
unseen cases, independent human draft-repair assessment and multi-day usefulness.
No README files are changed.

Verification for this slice: full `internal/tools` and `internal/workrun` race
suites pass; command watch/change-only race checks pass; physical-line,
multi-revision and retained-output reference regressions pass. Repository-wide
verification is also running for the combined current changes and will be
recorded separately. The live sequence tests real model/tool behavior but remains
an engineer-authored synthetic responsibility, not a consumer success-rate study.

### Notification handoff, bounded retry and command text (items 8 and 14)

The production sender now records pending/failed/accepted OS handoff separately
from chat delivery/read/resolution. Existing supervisor passes retry admitted
unread/unresolved notices with backoff, current per-goal policy checks, per-entry
leases and a five-attempt limit. Failed commands no longer mark handoff successful;
historical unread entries are not newly admitted. Inbox details expose the state.
The OS call does not hold the global inbox lock. Backup includes delivery state
and excludes process locks. Legacy notification receipts remain readable.

The old macOS command interpolated the model-authored summary into AppleScript.
Fixed program source plus argv now keeps that text inert; Linux likewise places
text after an option terminator. The same delivery path covers scheduled job
results, manual job results and local plan-review reminders. See
[notification delivery](NOTIFICATION_DELIVERY.md) for exact retry semantics,
rollback behavior, platform limits and the possible duplicate after a crash
between OS handoff and durable settlement.

Validation: full inbox/lifecycle/workapp race suites pass. Focused race tests
across those packages and the command adapter cover delivery, concurrent receipts,
backoff/restart, retry exhaustion, backup, projection, goal-review policy and
hostile-looking text through the native macOS argument parser. New backup and
projection regressions also pass separately. No actual OS notification, real
account action or paid model call was used for this slice. Physical display,
mobile push, channel escalation, and new field layout in a running Electron app
remain unverified. The broader items remain open.

## Verification checkpoint — 3 November

- `go test ./... -timeout 6m`: all packages pass on this macOS checkout. The
  command package completed in 331.6 seconds while other checks were concurrent.
- `go vet ./...`: passes.
- Full affected race suites pass for jobs, workhome, tools, workrun, inbox,
  lifecycle and workapp; focused command race checks pass. Tests added after the
  broad run began were additionally run in their affected focused suites.
- Live evolving watch: 19/19 checks; all failed attempts retained; US$4.496448
  accounted upper cost of the new US$10 allowance; no unknown reservations.
- No README files changed. No real account action, deployment, service activation,
  signing change or release publication was performed.

Completed commits from this session: `30a2f7880` (paused scheduling/evidence),
`a55ae7bd0` (new evaluation allowance), `80f7d1995` (current-run effects and
citations), `8edc672c4` (notification outcome/retry and command isolation).
The intervening user commit `cbbf6d7ac` is preserved. The complete fifteen-item
goal and subsequent responsiveness request remain active. Next engineering work
continues the unresolved acceptance rows, including complete connected workflows,
execution/availability, onboarding, research/browser authority, memory/costs,
deliverables and release/privacy assurance.

## Additional user request

After the current product/engineering implementation, profile startup to usable
chat, screen/chat switching, input latency, rendering and background resource
use in the actual app. Record reproducible measurements, improve the measured
bottlenecks and commit verified changes separately. This is additional scope;
it does not replace any of the fifteen requirements above.
