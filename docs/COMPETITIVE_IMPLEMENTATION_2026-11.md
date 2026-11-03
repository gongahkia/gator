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
| 5. Connected jobs | Complete calendar and email workflows, including threading, recipients, attachments, drafts, reviews, account scope, reconciliation and real-service acceptance | In progress: rich Gmail sends, selected original uploads, thread reads and native send-permission controls |
| 6. Research | Reviewed public discovery beyond preselected sites, sensible shared budgets and in-task scope expansion; unknown-source research with provenance and network protections | In progress: task-only public/site grants, shared 32-attempt/eight-search limits, bounded evidence search and live primary-source acceptance |
| 7. Browser | Bounded task/session permissions, private sign-in and useful session continuity; representative multi-page jobs with measured interruption count | Open |
| 8. Delivery | Durable notification outcomes/retry/reconciliation, channel choices and mobile delivery; disconnect, OS failure and actual-device acceptance | In progress: bounded OS handoff retry verified locally |
| 9. Conversation | Responsive questions and task routing during work, preserving owner-bound steering, approvals and resource limits | Open |
| 10. Memory/adoption | Scalable scoped reconciliation, outage/ambiguity behavior, realistic accumulated profiles, multilingual history and reviewed competitor-data import | Open |
| 11. Costs | Task/recurring money estimates, shared enforceable ceilings and visible unknown costs across model/search/delegation | Open |
| 12. Deliverables | Representative template revisions and independently reviewed briefs, sheets and decks; factual/layout defects and repair time recorded | Open |
| 13. Release/platform | Signed install/update/rollback lifecycle; supervisor drain/restart; acceptable Windows command isolation and native evidence | Open |
| 14. Privacy/assurance | Precise threat/data-flow model, platform credential protection, and adversarial cross-tool/content evaluations before expanding authority | In progress: notification command text isolated from program source |
| 15. Capability truth | Current shared capability descriptions for onboarding/help/model guidance; eliminate calendar contradictions and test user-visible claims | In progress: shared connected-account/availability guide and Google operation parity check |

Acceptance depending on provider registrations, signing identities, physical
devices, deployment or independent human judgment stays explicitly unverified
until that evidence exists. Implement independent engineering work while those
acceptance steps remain open. Do not treat a green narrow test as product parity.

## Implementation evidence

### Reviewed public research and bounded evidence lookup (items 6, 9 and 15)

Foreground Work can now discover sources beyond manually selected origins.
`web_access` opens a native review of public-web or additional-site scope, actual
search readiness, data sharing and task-only limits. Eight searches fit within
32 total admitted web attempts, shared across the manager and connected/custom
researchers. Workers cannot ask for wider grants or reset the counters. Existing
site selection remains bounded; background checks cannot acquire a new grant.
Denial, revocation, cancellation, invalidated plan reviews and later requests
do not inherit authority. The journal retains scope decisions, and manifests
retain historical usage without making it a reusable capability.

The first real-model/public-search attempt found the right primary page but
failed at the unchanged evaluator context ceiling while paging raw HTML. A
bounded `search_evidence` tool now finds literal terms in verified retained bytes
with exact UTF-8-safe contexts/offsets and first/last matching ranges. It avoids
guessing local paths for web evidence. Concurrent captures use atomic publication
so multiple researchers cannot observe partial content-addressed files.

The actual Electron journey also exposed a review binding defect: both host and
renderer inferred the active approval from displayed `Request` fields. Reviews
now carry a private host-owned message identity and use the exact projected
message index; custom scope/limit layouts keep their approval controls without
weakening the live interaction token/owner checks. The web grant journey passes
in 6.0 seconds; Gmail selected-original sends and chat follow-up approval/save/edit/
pause journeys pass in 8.0 and 16.1 seconds. Shared help describes public research
and no longer equates an empty connected-account selection with local-only access.

Live report `research-2026-11-03T07-23-01.419Z.json` passes 8/8 assertions in
19.5 seconds, with five model requests, one actual TinyFish search, one public
page fetch and three local evidence searches. It starts without selected sites
and returns a retained-source citation to the official Go documentation. The
failed first report remains alongside it. These two attempts add twelve model
requests, 162,505 input / 719 output tokens, US$0.992286 estimated model ceiling
and US$0.16 conservatively retained search allowance.

New cumulative accounting: **98 model requests, 986,397 input / 7,042 output
tokens, US$6.087390 estimated model ceiling plus US$0.16 search reservation =
US$6.247390 total upper allowance usage**, with no unresolved model reservation.
The US$10 total authorization is unchanged. Search reservations are not billed
charges; TinyFish's current pricing/docs disagree about behavior beyond free
limits, so the conservative allowance remains retained. See
[web research](WEB-SEARCH.md) for behavior, source references, reproduction and
privacy limits. Public reads preserve DNS pinning, private-address checks,
credential separation, no automatic redirects and bounded responses. They do
not establish semantic filtering of every possible private query or URL payload.

The user captured the initial engineering changes in intervening commits
`61b6b60f4` and `7389df4e0`; those histories are preserved. Roll back the complete
research feature and its UI/prompt/test consumers together, including the
follow-up verification commit. Historical optional manifest usage and interaction
records grant no continuing access. README files remain untouched. Item 6 retains
limited live acceptance rather than a claimed general quality benchmark; the
full fifteen-item goal and later performance audit remain active.

### Conversational dynamic specialists (additional user request)

The user requested on-the-fly subagents and named specialists during the broader
implementation. `create_agent` now defines a task-specific role from an available
native template and optionally starts it immediately. `agent_profiles` manages
explicitly saved reusable definitions through chat. This uses the existing task
supervisor and its inspect/await/steer/answer/cancel controls, shared budget and
three-worker concurrency ceiling. See
[subagent behavior and boundaries](SUBAGENTS_AND_CLI_INTEROP.md#create-a-specialist-in-conversation).

The host binds specialization to actual template tools/model/source permissions,
captures the definition/digest in task evidence, rejects stale revisions and
keeps running tasks immutable across profile edits/deletion. No recursive
delegation, new provider selection, expanded research scope, automatic scheduling
or background host is introduced. Saving/deleting requires foreground operation
and current-user quotation evidence; semantic intent still depends on manager
instruction following. Task-only reuse is explicitly selected from the same
conversation lineage; saved roles use each later task's selected resources.

The real Electron fixture passed create/run/save, restart, new-chat reuse and
deletion. The retained live report
`scripts/competitive-eval/results/agents-2026-11-03T06-52-02.326Z.json` passed all
eight assertions across creation and reuse on two different project sources,
with no tool errors. It added 16 requests, 95,840 input tokens and 984 output
tokens, conservatively estimated at US$0.598656. The cumulative new allowance is
now **86 requests, 823,892 input / 6,323 output tokens, US$5.095104 upper estimate**,
with zero unknown-request or search reservations. The total authorization remains
US$10. This narrow acceptance does not close any of the fifteen audit findings.

Validation: `go test ./...`, `go vet ./...`, `go build ./cmd/gator`, the full
orchestrator/workrun/lifecycle race suites, TypeScript checking, desktop build,
desktop/application-host tests, and the Electron dynamic-agent journey pass.
Additional focused race tests cover actual narrowed step limits, promotion after
a first assignment, task-only revisions after owner restart, and reviewed-plan
denial of custom coding aliases. Desktop/application-host tests retain two
existing environment/platform skips. The UI build retains existing CSS,
dependency-directive and bundle-size warnings; startup/navigation profiling
remains queued under the separate user request. README files are unchanged.

The core implementation was captured in intervening commit `43e5cf8b8` while
verification continued; its history is preserved. The follow-up commit records
the remaining admission/restart checks, help and verification documentation.

Rollback: revert the dynamic-agent code, UI labels, tests and documentation
together. User-created profiles remain private retained content and can be
removed through knowledge deletion; historical task snapshots remain history.

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

Include cold/warm process launch to a usable composer, navigation to each major
screen, short/long conversation switching, typing and scrolling while output
streams, file-preview opening, and idle/background CPU/memory. Measure the full
interaction through the next rendered frame, retain environment/data-size/run
counts and distributions, and compare the same journeys before and after each
fix. Use isolated profiles so profiling cannot alter the user's conversations.

## Connected email checkpoint — 3 November

Gmail now supports explicitly listed To/Cc/Bcc recipients, reply threading and
attachments from this run's selected source/output/previous file mounts. The
host freezes attachment bytes before exact review and includes names, sizes and
SHA-256 identities in the approved payload. Draft-only and denied requests make
no send request. Full thread reads inherit existing message-content restrictions.
Rich envelope receipts survive sealed-result and interrupted-action projection
without copying bodies or file bytes into chat receipt summaries. See
[connected email](CONNECTED_EMAIL.md) for limits, rollback and open requirements.

Independent MIME parsing checks Unicode, binary content, recipient folding,
thread headers, frozen-buffer mutation and aggregate attachment limits. Real
Work executor tests exercise generated and selected original files through the
action broker/journal in approved, denied and draft-only modes. Full race suites
for connector, workapp and workrun pass; command receipt/connection regressions
pass. `go test ./... -timeout 6m` passes for commit `90f4c22d6` (command package
236.3 seconds under concurrent checks); `go vet ./...` passes.

No paid request or real account action was used. Native send-permission upgrades,
provider drafts, original upload selection, calendar and real-service acceptance
remain open. Item 5 is in progress, not complete. The responsiveness audit remains
queued after the active product work as requested.

## Native email permission checkpoint — 3 November

Connections now offers reviewed **Enable email sending** and **Turn off email
sending** controls. The enable flow reauthorizes the existing installed-app
scope set plus sending, checks explicitly reported API/identity scopes and a new
offline grant, then publishes expanded settings. It preserves the old local
connection on partial/missing grants, denial, cancellation and stale
account/configuration. Failed configuration publication attempts a CAS restore
without overwriting a concurrently replaced account. Local disable preserves
reading and credentials, explicitly applies to new tasks, and does not claim to
revoke Google's grant. Active tasks keep their reviewed connection snapshot.

The tests also exposed an existing callback-close race that could show the
browser an EOF instead of the result page. Pages are now length-delimited and
flushed before the completion signal permits closing the listener. Success,
denial, state mismatch and missing-code page bodies are verified over real local
HTTP connections. Permission tests exercise the production controls, PKCE
callback, token exchange and private stores. The Google workflow guide no longer
advertises removed Calendar operations/scopes in its setup/operation sections.

Validation: affected native permission/connection and callback race checks pass;
full auth race tests pass; TypeScript check, desktop production build and
`go vet ./...` pass. `go test ./... -timeout 6m` passes again with the native
controls included (command package 222.5 seconds). The actual Electron
`connections.spec.ts` journey passes in 16.4 seconds: it reviews the permission,
cancels, rejects a partial grant, accepts a complete grant and turns off sending
again, checking saved scopes and renderer credential privacy. An initial test
selector missed the footer's keyboard-shortcut suffix; it was corrected without
changing the interface or weakening the grant assertions. The native account
screen was also inspected at 980 × 780.

The browser fixtures use no real account, send no email and incur no new
model/search spend. The cumulative evaluation upper estimate remains
US$4.496448 of the US$10 allowance. No README files changed. Item 5 remains open
for provider drafts, original upload selection, calendar and real-service
acceptance. General reconnect scope handling and the rest of the broader
requirements are not certified by these permission-upgrade tests. The complete
fifteen-item goal remains active; the subsequent responsiveness audit is retained
above, with reproducible before/after measurements required.

## Shared capability guide checkpoint — 3 November

`internal/productguide/capabilities.json` now supplies the runtime's embedded
self-knowledge, desktop onboarding, empty source selection, both help surfaces
and the pilot's starting instruction. It removes unsupported Calendar offers
and the unconditional user-owned-client requirement, describes native sending
and the original-upload limitation, and reconciles the obsolete blanket claim
that no remote companion exists with its experimental, separately hosted
implementation. Nothing in the guide selects an account or grants authority.

The guide's Google operation inventory is checked against production tool
registration so future additions/removals require a corresponding review.
Named JSON imports keep the desktop from bundling model-only guidance. See
[shared capability descriptions](PRODUCT_CAPABILITY_GUIDE.md) for exact scope
and rollback. Broader navigation/voice/file/release descriptions still need
production-backed consistency checks; this does not close item 15.

Validation: productguide/workrun/workapp full race suites pass; focused command
pilot/app/connection tests pass; affected-package vet, TypeScript check and
desktop runtime/UI builds pass. The existing first-task Electron journey is
verified in 7.9 seconds with an isolated profile: it retains a file and draft
through model setup/reload and submits only on request. Its model endpoint is a
local fixture. No paid model/search call or real external action was used.

The preceding native email permission change is commit `9075ae6d9`; rich message
composition is `90f4c22d6`. The fifteen-item goal and the requested subsequent
responsiveness work remain active. README files stay untouched.

## Original input bytes in reviewed emails — 3 November

The desktop Work loader preserves original bytes for only the files selected
on the admitted request, deriving model input from the same bounded reads. Office
extraction remains the provider view; original DOCX/XLSX/PPTX/ODT bytes stay
separate. Per-task `list_input_files` returns names, media type hints, sizes,
hashes and exact `input/...` references. The connector's resolver can look up
only that frozen selection, never arbitrary uploads or filesystem paths.

The existing exact send preview includes original-file metadata and the private
bytes are frozen again before review. Source/staging/caller mutation cannot
change an approved send. A later task without files cannot borrow the original
from a retained tool result. The initial loader refactor and implementation were
captured by the user's intervening commits `9f36591a3` and `e0f17d35d`; they are
preserved. This checkpoint verifies and documents that implementation and adds
the cross-request regression. See [connected email](CONNECTED_EMAIL.md) for
combined input/output limits, lifetime and rollback boundaries.

Verification includes independently parsed MIME from production Work and the
actual Electron paperclip → task → exact approval → Gmail fixture route. The
expanded Electron journey picks DOCX, XLSX and PPTX together and checks every
received file against its selected bytes and approval hash/size; it passes in
8.0 seconds. The production Work regression additionally checks caller/staging
mutation, extracted model views, unselected references, denial and cross-request
isolation. Existing attachment draft/reload/queue/validation journeys pass
(three Electron cases, 55.5 seconds before expanding the email case).

`go test ./... -timeout 6m` passes (command package 239.2 seconds under concurrent
checks). Full attachment/tools/connector/workrun race suites pass; the later
cross-request regression also passes under race separately. `go vet ./...`,
TypeScript check and desktop runtime/UI builds pass. No real Gmail action or
paid evaluation was used; the US$10 ledger remains at US$4.496448 upper estimate.

Next connected-work requirements remain substantial: provider draft management,
calendar and real-account acceptance. Google's draft API uses a stable draft ID
but replaces its underlying message on edit, and requires `gmail.compose` for
draft sends rather than the existing `gmail.send` permission. Implement native
scope review, content/version-bound edits and exact send bytes, preserving
existing draft attachments and uncertain outcomes. A mutable draft ID alone is
insufficient authority to send its contents. Checked against
[Google's draft guide](https://developers.google.com/workspace/gmail/api/guides/drafts)
and [draft send contract](https://developers.google.com/workspace/gmail/api/reference/rest/v1/users.drafts/send).
The whole fifteen-item goal remains active; responsiveness profiling stays
queued after the current product work.
