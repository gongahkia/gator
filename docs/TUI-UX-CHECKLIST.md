# Gator terminal experience completion checklist

This tracks the 15-item product/interaction goal from the audit of `7750df414`.
Completion requires behavior and verification, including the Go/host boundaries;
an available command alone is not evidence of an enjoyable end-to-end flow.

| # | Acceptance criteria | Status | Evidence / commit |
|---|---|---|---|
| 1 | Conversation remains available during execution; ordinary messages can steer a specific live task or start independent work; ambiguous routing is clarified without losing task identity, files, or drafts. | In progress | `92cd471f4`/`485cd007c` plus result continuity: ordinary messages reply, steer, clarify, delegate and stop specific independent owners without moving focus. Pending-review text corrections now revise the same task asynchronously, preserve its frozen inputs/limits, and require fresh approval; failures/cancellation and restart retain the outcome. Preparing/reviewed plans can be cancelled conversationally. `871cb971a`/`af242b3f8` also apply ordinary corrections during initial preparation in the same execution owner, retain interrupted proposals for explicit retry, and require a fresh exact plan review. A first unapproved proposal may finish during routing; later edits, approvals and dispatch invalidate the old correction identity. Recent original-task summaries reach follow-up conversation/work/planning without acquiring controls. Explicit queues remain available. Additional corrections during an in-flight revision, corrections during plan-bound execution, changes to pending exact actions/new inputs, delegated exact-approval journeys, broader failure/account/goal journeys and live-model routing quality remain open. |
| 2 | Following unifies commitments with outcome/state/next action, next and last successful checks, sources/timezone, pause/edit, and truthful foreground/service/offline limits. | Implemented; final journey pending | `4b9edf091`/`a95289265`: commitment cards and exact check controls. Active goal cards open their exact canonical picker row and prepare a check for that goal's current workspace, including duplicate titles, other-chat origins and busy owners. Focused Go/host/native integration checks pass. `08bc8aea1`/`1bb9e8994`: active/paused goals now expose canonical inspected plans and reviewed pause/resume controls. They bind exact saved goal identity, preserve other edits/check definitions, retain selection through refreshed ordering, and keep task ownership/drafts during real-Go controlled-provider work. Read cancellation retires late results without changing pages or stopping work. `883a536e7`/`b38ea945e` add reviewed daily/weekday/hourly/custom timing, timezone, catch-up and end-date edits for older schedules, preserving their full canonical policies and original task ownership. `e2f5195f9` adds visible workspace and status-appropriate next actions to unscheduled cards, retains same-title selection through reordered refreshes, and keeps unknown check coverage explicit. Native four-size checks pass. The final combined journey remains. |
| 3 | Relevant answers expose supplied memory/provenance/correction; noise feedback offers a reviewed, scoped check/notification change and reports its actual outcome. | Implemented; final journey pending | `cb095fcc4`/`08c12a19d`/`bda11baf3`: original, dynamic and carried memory are inspectable and correctable; noise feedback leads to scoped reviewed changes through the authoritative check controller. Continuations now receive a bounded current-status snapshot for previously referenced knowledge, preserving historical messages while supplying corrections and withdrawal/omission/scope/review status. Private replay retains validated IDs through compaction/restart. Exact refresh input is sealed and inspectable; it cannot widen a reviewed plan or spend an extra model request. Native four-size provider/input, correction, forgetting, privacy, snapshot inspection and restart checks pass. Final combined acceptance journey remains. |
| 4 | First task leads through only missing setup; host connectivity and model readiness are distinct; local/cloud processing, capabilities, and work budget are explained. | In progress | `d0fd58e48` provides Go-owned prerequisite/processing/budget projection. Ordinary first submissions offer missing setup while preserving private files/text and plan intent; Host connectivity and model setup have separate labels. Native fresh-profile → invalid provider form → repair → persisted selection → inspect context/limits → explicit useful task passes at all four sizes. Missing and subsequently changed credentials recover without replaying/admitting the rejected task. Request-specific connected-account/access setup handoffs and recovery after provider rejection during an admitted task remain; final integrated journey and live-account/model verification remain. |
| 5 | Workspace/account/source scope is inspectable near input; conversational/manual settings agree, including terminal preferences, with confirmed outcomes. | In progress | Ctrl+T and /request-context inspect workspace/model processing, selected accounts, inputs, mode and actual Go budgets. Missing account display metadata retains the selected identity. Appearance, navigation and recent-chat grouping now share Go configuration across manual controls, conversational tools, cached owners, CLI theme changes and restart. Legacy frontend values import once; failures do not report saved values. Native four-size settings journeys plus unreadable-import recovery and live-task preservation pass. Full permission explanations, broader account/settings integration and live-model/account verification remain; final combined acceptance journey pending. |
| 6 | Results/files/updates expose keyboard contextual actions to open/edit/reply/inspect evidence/mark done while retaining original identities and conversation context. | In progress | Configurable Ctrl+E and /actions expose original-item result/file/update controls. Answers join deliverables only by exact run identity. Native four-size journeys inspect retained evidence, copy answers, edit/save working file versions without changing sealed originals, reject other-chat targets, mark done with a correlated Go outcome, restore the original owner and explicitly send an update-bound reply while retaining the newer draft/review intent. Delayed source/file reads now close without reopening on late success/failure; exact file-action Back and explicit read retry pass at all four sizes, including an unrelated executing Go task. Original-answer request receipts now share /receipts controls, including exact-account Sent checks, permission repair, cancelled-read completion and restart at all four sizes. Conversation replies and delegated summaries now expose exact-turn/task contextual inspection, copying, retained inputs and original-owner navigation; four-size real-Go journeys preserve the parent draft/review intent without new model work. Structured connected-item actions and the final combined journey remain. |
| 7 | Input and useful transcript remain visible at 110×34, 80×24, 60×20, 42×14 with combined attachments/reply/plan/slash states and multiline drafts. | Implemented; final journey pending | `204386564`; `composer-budget.test.tsx`: 8 file names + plan/reply + multiline input and slash completion, ≥4 transcript rows at all four sizes. |
| 8 | Approval surface shows exact live action/account/destination/content/consequences; long reviews scroll; changed/stale tokens cannot execute; edits require fresh review. | Implemented; final journey pending | `8ba3a9cb8`; `approval-review.test.tsx`: four sizes, scrolling/exact data, stale and missing previews; `approvals.test.tsx`: real Go token protection and controlled connected send. |
| 9 | Root command search includes local/backend descriptions/names/aliases, grouped browsing remains usable, and commands requiring values have meaningful typed controls. | In progress | `5f54f7f17`; root search and common permission/theme/source/attachment controls verified in `commands.test.tsx`. `883a536e7`/`b38ea945e` provide typed schedule presets, timezone, catch-up and end-date controls with computed review. Other advanced command families still expose explicit advanced syntax; typed controls remain required. |
| 10 | Back restores menu/filter/selection, draft and reading position consistently; backend pages and file workflows retain context; rail marks the active page. | In progress | Host-managed navigation waits for the Go view acknowledgement; failed views retire and original work/focus is restored. Next-message plan-first intent now persists with text/files/reply context, including empty drafts, restart, commands and newer-draft admission races. Native restart → keyboard submission prepares review without starting work. Shared frontend menus and read-only inspections now restore parent query, selected choice and list position, including refreshed/reordered results at all four sizes. New commands/context owners start fresh navigation; successful check changes retire obsolete controls. Context list/detail/edit/review now retain local filter/reading positions through presentation-only IDs (`bda11baf3`); item-based transcript reading anchors now persist through the existing host across chat switching, page navigation, layout changes and restart; other typed-editor/file-editor and backend-page restoration across all flows remain open; active-page rail tests already pass. |
| 11 | Ctrl+C cancels/stops without implicit quit; Ctrl+Q quits through guards; help/hints reflect configured bindings; no function-key shortcuts are introduced. | In progress | Idle stop/draft preservation, explicit quit, remapped help/composer/rail verified. Tab now explicitly queues a draft after the active task in its current owner; Enter remains conversational and routes clear corrections. Slash completion/forms retain Tab. The queue shortcut uses the same Go settings backend; four sizes, Ctrl remapping and real-Go queued attachments are checked. Saved-context actions use Ctrl+E/Ctrl+X/Ctrl+O, and context refresh is correctly labeled. Ctrl+L latest uses the same Go shortcut settings and configured hints (`614d89a1a`); controlled Kitty checks exercise a custom Ctrl+Shift+L. Other voice/approval/contextual hints and terminal modifier limitations remain. |
| 12 | Disconnected/error states offer contextual reconnect/recovery with intact drafts/request identity; safe admission retries and uncertain external effects stay distinct. | Implemented; final journey pending | `d00952219`; `interaction-recovery.test.tsx`: real Go accepted request + lost acknowledgement retries once, reconnect keeps draft/owner, no model replay; receipts tests preserve unknown external outcomes. |
| 13 | New-content/latest/catch-up affordances preserve stable reading anchors across navigation/restart; long-history rendering is benchmarked and optimized when justified. | Implemented; final journey pending | `614d89a1a`: New-content counts, /catch-up, /latest and the configurable Ctrl+L latest binding share the composer hint row. Item identities and signed line offsets persist by profile/chat; layout changes, inserted history, chat switching, streaming revisions and restart preserve reading context in native fixtures. Original indexes/run IDs remain authoritative for actions. Cached immutable fingerprints and stable local-field projections avoid routine hint rebuilds. Fresh 5,000-message single samples: append 371→267 ms, startup 3,944→4,098 ms, scroll 361→395 ms, resize 597→750 ms; reproducible reports in `docs/benchmarks/tui-transcript-reading-{before,after}.json`. `03f743fb5` now mounts an 80-item native window above 160 retained items, with measured/estimated spacers and original full-history search/action identities. Selected native consumers stay alive until selection clears. The unchanged benchmark reports 5,000-message startup 4,098→218 ms, scroll 395→39 ms, append 267→35 ms, resize 750→76 ms; RSS 841→316 MiB in single sequential samples (`docs/benchmarks/tui-transcript-windowed.json`). Native four-size journeys cover continuous scrolling, distant search/actions, selection lifetime, resizing, exact off-window review, disconnected state, view paging and restart. The complete regression at `cf930c0c9` passed (178 pass, 3 existing skips, 0 failures, 181 tests / 78 files); subsequent features have the focused gates recorded below. Final combined acceptance and physical/live verification remain pending. |
| 14 | Voice has direct record/stop/cancel/transcribe/edit flow, visible processing destination and explicit consent; transcript is editable before sending. | Implemented; final journey pending | `09e74ef39`; `voice-controls.test.tsx`: all four sizes, native VoiceSession with controlled capture/transcription, route/chat consent, changed-route rejection, editable draft/no send, recording/transcription cancellation. Physical audio remains unverified on this host. |
| 15 | Shared native typography/spacing/selection/focus/disabled treatments and readable summaries work with terminal colors, NO_COLOR and reduced motion; exact technical details remain inspectable. | In progress | `20e9b7cde`: Markdown cleanup destroys native consumers before their borrowed syntax style, including theme changes. Shared immutable field subscriptions prevent routine snapshots from invalidating unchanged native content; the reproduced conversation render stall clears without changing deadlines or renderer. Exact review objects remain frozen. Detail views reserve their scrollbar column and page by viewport height, retaining readable processing/budget text at all four sizes. Shared visual/state audit and full acceptance journey remain. |

## Backend-only completion ledger (13 November scope change)

The active goal is now the backend objective in the supplied goal attachment.
The original fifteen findings above remain intact and are not claimed complete.
Terminal presentation work and the previously queued mobile implementation are
outside this goal. Preserve the CLI/TUI as a client; no Swift UI is being built.

| Backend workstream | Original findings | Current evidence and remaining acceptance |
|---|---|---|
| Conversational control | 1, 8, 12 | Initial preparation corrections exist. Pending external-action correction now has controlled Go/service and local-client evidence: same operation, exact old review withdrawn, fresh approval required; delegated control is verified with concurrent original work and parent-draft continuity. Repeated in-flight plan corrections now have local owner and controlled-client evidence; delegated/failure/restart coverage remains. Plan-bound pause/replan/review, reviewed input changes, delegated/race/failure/restart coverage remain. |
| Request-specific setup/recovery | 4, 12 | Existing provider setup/admission safeguards remain. Account/source/permission handoffs and provider rejection after admission need end-to-end verification and demonstrated gaps resolved; preserve request inputs and reject uncertain-effect replay. |
| Authoritative settings/scope | 5, 8 | Existing canonical terminal/model settings remain. Inventory broader account/source/permission/processing/budget contracts, saved outcomes, cached-owner propagation, restart and current/future/background effects; reject changed identities and unauthorized expansion. |
| Structured connected-item actions | 6, 8 | Existing local files/evidence/receipts retained. Complete supported provider item references and inspect/open/edit/reply/done paths with version/account/permission review and uncertain outcomes. |
| Typed workflows | 9 | Inventory opaque public workflows; complete supported Go/host typed fields, defaults, choices, validation, reviews, operation results and recovery without a new command engine. |
| Continuity/lifecycle | 1, 10, 11, 12, 13 | Prior draft, request, reading and receipt checks are a baseline. Verify affected workflow/file/chat/disconnect/restart boundaries, cancelled asynchronous results and foreground versus durable commitment semantics. |
| Integrated verification | 2, 3, 4, 5, 6, 8, 12, 14 | Full backend journey and relevant regression checks remain. Recheck existing memory/noise/scheduling/voice/history with new work; separate controlled providers and process restart from physical microphone and live account/model evidence. |

No workstream is complete merely because its contract or router exists. The final
journey must cover setup/access, persisted scope, commitment, unrelated dialogue,
task correction, pending-action correction, stale rejection, fresh exact approval,
connected-item results and restart/disconnection. Physical/live prerequisites
remain to be inspected; prior fixture checks do not establish them.

## Implementation rules

- Reuse Solid/OpenTUI styling and Go/host safeguards; keep CLI/TUI only, optional rail, no mascot or Electron.
- Preserve data, permissions, immutable evidence, recovery, drafts, and unrelated work. README changes require an essential correction.
- Make atomic local commits with relevant checks; do not push or publish.
- Distinguish native fixture rendering, protocol integration with controlled providers,
  physical terminal interaction, and live model/account verification.
- Track blockers precisely; continue independent items. Do not mark the goal complete with missing criteria.

## Queued follow-on: mobile access

The user's 9 November follow-up requests mobile research and implementation
**after these fifteen criteria are finished**. The recommendation and researched
implementation scope are in [mobile access](MOBILE-ACCESS-RESEARCH.md): reuse the
existing optional companion for a responsive/installable PWA; keep Telegram as an
optional messaging channel. The terminal remains the primary local application,
with no Electron restoration. This does not replace or narrow any row above.
Mobile implementation is queued; research is not implementation or completion.
Preserve both phases across continuations and do not declare the expanded work
finished after only the terminal phase or a mobile scaffold.

## Verification journal

- Repeated corrections during revision (13 November): the canonical store admits
  a later correction against the exact in-flight plan epoch, carries all earlier
  instructions forward, and invalidates the obsolete proposal before cancellation.
  The owner waits for that call to release its execution lease, then starts only
  the latest retained edit in the same task. Further corrections while draining
  do not start concurrent calls. Superseded attempts receive durable interrupted
  receipts; completed/failed outcomes remain intact. Initial-preparation handoff
  also selects the latest edit. Keyboard stop while draining cancels the latest
  retained proposal instead of launching it. Local receipt projections refresh
  after reconciliation. No reviewed-plan enforcement or approval gate is removed.
  Controlled Go owner checks pass for three corrections and cancellation in
  `/tmp/gator-backend-20261113-repeat-owner.log`; initial-cutover and latest owner
  checks pass in `/tmp/gator-backend-20261113-repeat-cutover-final.log`, and focused
  race checks pass in `/tmp/gator-backend-20261113-repeat-race.log`. The native
  host/client with a controlled model confirms ordinary second correction,
  earlier instruction text, unchanged sources/limits/uploads/task, responsive
  unrelated dialogue, superseded receipt and fresh unapproved result (1 pass,
  15 assertions): `/tmp/gator-backend-20261113-repeat-client-final.log`.
  TypeScript and runtime builds pass. Full session/app/run regression passes
  (70.238 / 64.355 / 118.976 seconds) in
  `/tmp/gator-backend-20261113-repeat-full.log`; this preceded the final small
  receipt-refresh/initial-handoff changes covered by the focused checks above.
  The full native plan-correction file is running in
  `/tmp/gator-backend-20261113-repeat-plan-client-full.log`. Delegated repeated
  correction, provider failure/restart and broader integrated journeys remain
  open. This is controlled-provider/process evidence, not live account/model
  quality or physical audio/terminal verification.

- The prior correction-history full session/app/run regression also passed,
  observed exit 0 (67.062 / 55.241 / 120.941 seconds), in
  `/tmp/gator-backend-20261113-history-full.log`.

- Backend correction-history retention (13 November): later sequential plan
  corrections now retain earlier user instructions, including failed attempts,
  in the canonical digest-bound plan. Earlier dialogue outcomes remain intact;
  retaining an instruction does not relabel its failed attempt as successful.
  Planning receives instruction text without private parent/turn controls, and
  reviewed execution receives the retained context with current-plan precedence.
  Histories are bounded to 32 prior instructions and 64 KiB combined text;
  overflow is rejected explicitly rather than silently dropping constraints.
  Clone isolation, persistence/reload, tamper rejection, stale approval, failed
  correction/retry, duplicate identity and size guards pass in
  `/tmp/gator-backend-20261113-correction-history-verified.log` and
  `/tmp/gator-backend-20261113-history-guards.log`. These are controlled Go checks;
  no physical/live verification is claimed. Full session/app/run regression is
  running in `/tmp/gator-backend-20261113-history-full.log`. Active-revision
  correction admission and serialization remain open; this fixes the instruction
  retention gap needed for that transition without claiming it implemented.

- Backend pending-action corrections (13 November, implementation in progress):
  `f44026a12`: the original operation accepts a correction only for its single exact pending
  external action, with unchanged plan restrictions. It denies the old proposal,
  enqueues guidance without blocking concurrent remote ingress, and requires a
  fresh exact replacement review. Router inputs never receive review tokens;
  historical task references strip correction capabilities. Explicit denials now
  remain in the action manifest as denied proposals, while the external dispatch
  journal continues to contain only admitted attempts, with no fabricated provider
  receipt. Race-detector checks cover approval versus correction (100 races),
  stale/missing identities, wrong review kinds, multiple reviews, reviewed plans,
  stopped operations, full queues and invalid instructions. The real service
  loopback Google test proves zero sends before fresh approval, one corrected
  send, same run, original/replacement decisions and retained manifest records:
  `/tmp/gator-backend-20261113-action-safety.log` (pass). Host regression:
  `/tmp/gator-backend-20261113-action-host-full.log` (134 pass, 1 existing skip).
  Go/host/client local-process journey and exact-approval regression:
  `/tmp/gator-backend-20261113-action-native-final.log` (2 pass, 30 assertions),
  including ordinary unrelated dialogue, correction, old-token rejection,
  corrected send, retained draft and restart without replay. These are controlled
  loopback providers and process checks, not physical terminal/audio or live
  account/model evidence. Delegated/local client journeys also pass in
  `/tmp/gator-backend-20261113-action-delegated.log` (2 pass, 39 assertions):
  the original requesting owner keeps running, the background owner receives the
  correction, fresh exact approval reaches that owner, and parent draft/restart
  continuity survives. TypeScript checks, bundle/runtime builds, focused CLI
  task-plan reference checks and focused race checks pass. The final full Go
  regression passes (`workapp` cached; `workrun` 118.940 seconds), observed exit 0,
  in `/tmp/gator-backend-20261113-action-go-verified.log`. The focused combined
  race gate passes in `/tmp/gator-backend-20261113-action-race-final.log`.
  All seven workstreams remain open.

- Complete initial-correction regression (13 November): the isolated checkout
  at `7c909f99a` (same implementation as main `af242b3f8`) finishes with
  **196 pass, 3 existing skips, 0 failures**, 4,046 assertions across 199 tests /
  78 files, 607.16 seconds, in
  `/tmp/gator-ux-20261113-preparing-full-tui.log`. The process was observed to exit
  0, and all 1,963 captured source, dependency and runtime hashes match the
  pre-run manifest. Main's runtime rebuild also passes. This verifies the full
  terminal regression through initial-plan corrections; it does not complete
  the all-fifteen combined journey or physical/live verification.

- Initial preparation corrections (`871cb971a`, `af242b3f8`, 13 November):
  ordinary composer messages correct an initially preparing plan in its original
  task. The canonical store persists the correction before its owner cancels
  the obsolete tool-free proposal. That owner releases its execution lease
  before the existing revision callback starts. Task identity, frozen text,
  uploaded inputs, sources, budgets and binding remain intact; neither proposal
  can implement work before fresh exact review. Interrupted empty proposals can
  be retried explicitly in the same task after provider failure or restart.
  Prior correction outcomes are retained before their references are replaced.
  Host capacity allows replacement of an already-running preparation without
  allocating an extra task slot; an idle owner still requires free capacity.
  The automatic transition to the first unapproved proposal can complete during
  tool-free routing without making a same-task correction spuriously stale.
  This exception applies only to correction, not stop, guidance or approval;
  manual edits, approvals, dispatch and later revisions retain exact epochs.
  The metadata remains private to host control and is excluded from model
  authority/reference inputs. Delegated preparation projects as running rather
  than prematurely ready for review. Keyboard stop during the handoff cancels
  only its exact pending correction, without launching replacement work.
  Native affected-flow gate under NO_COLOR/reduced motion: **33 pass, 0 failures,
  664 assertions**, six files, 90.86 seconds, in
  `/tmp/gator-ux-20261112-preparing-owner-native-final.log`, observed exit 0.
  Four-size journeys cover original preparation held at a controlled provider,
  ordinary correction/conversation, fresh keyboard approval and actual execution
  with the original private brief while excluding future-draft inputs. They also
  cover cancellation, provider failure, explicit retry, draft preservation,
  isolated-runtime SIGKILL, restart and no automatic replay. Separate native
  delegated preparing/review cases retain original owners and return the actual
  result without disturbing independent work. Existing concurrency, queues,
  approvals and lost-admission recovery pass alongside these cases.
  Full Go package regressions pass: worksession 89.533 seconds, workrun 140.017,
  workapp 80.599 (`preparing-go-full.log`); relevant CLI checks pass in 73.649
  seconds (`preparing-command-regression.log`). Final keyboard/epoch/owner checks
  pass after the last safeguards (workapp 12.730 seconds,
  `preparing-owner-projection-go.log`). Host full regression: **133 pass,
  1 existing skip, 0 failures**, 134 tests / 31.225 seconds. Typecheck, runtime
  build and bundle pass. Logs use the same `/tmp/gator-ux-20261112-` prefix.
  This is native rendering, real Go/store, controlled-provider and isolated
  process-crash evidence; it does not establish physical terminal/audio or live
  account/model-routing quality. Row 1 stays open for the remaining states and
  journeys listed in its table entry. All fifteen criteria stay in scope.

- Complete schedule regression and unscheduled commitments (12 November):
  the full terminal suite at `cf930c0c9` was polled to successful exit:
  **178 pass, 3 existing skips, 0 failures**, 3,713 assertions across 181 tests /
  78 files, 668.58 seconds, in `/tmp/gator-ux-20261112-timing-full-tui.log`.
  Production source and tests stayed unchanged throughout the run. The existing
  image-paste, old dictation and managed-browser skips were not changed.
  Subsequent commitment-card work used a separate checkout and its own copied
  dependencies/runtime, then entered this worktree only after that suite ended.
  `e2f5195f9` shows workspace and status-appropriate next actions on unscheduled
  goals. A paused goal offers retained-plan inspection or resumption; unavailable
  check health asks for refresh rather than implying verified coverage. Saved
  intent remains distinct from enabled monitoring, including when the service
  is online. Native tests retain the original same-title goal through reordered
  refreshes, cancel resumption without a write, inspect its exact workspace, and
  preserve a multiline/plan draft at all four sizes. Following gate: **4 pass,
  80 assertions**, 10.02 seconds, in `unscheduled-native.log` beneath the same
  prefix. Typecheck and bundle pass; the first isolated typecheck required
  copying the host dependencies used by an existing direct-import test, without
  changing source or weakening that check. Row 2 is implemented with final
  combined acceptance pending. This is native fixture and prior real-Go/
  controlled-provider evidence, not physical terminal/audio or live-account
  verification. All fifteen criteria remain in scope.

- Reviewed timing for older schedules (`883a536e7`, `b38ea945e`, 12 November):
  Following exposes daily, weekday and hourly presets, an explained five-field
  custom schedule, IANA timezone, missed-check choices and optional end date.
  Go validates the proposed timing and computes upcoming occurrences before a
  separate confirmation can save. Invalid previews retain entered fields; Back
  from review restores those values and root Back restores the exact check
  details. Forms and reviews bind their original check and canonical policy.
  Saving copies the existing definition and changes only the selected timing
  fields through `jobs.SaveReviewed`, preserving current scheduler bookkeeping.
  Source snapshots, account/credential pins, model, contracts, budgets, linked
  goal, destination chat and enabled state remain intact. No task, supervisor or
  account action starts. A successful save refreshes Following from the store.
  Stale reviews reject concurrent changes and require explicit refreshed review.
  Long backend reviews now reserve the scrollbar column and allow line/page/end
  reading without selecting or executing an action, including multiple-action
  reviews. Success/warning states use the shared terminal colors and protocol.
  Native four-size timing journeys cover invalid input, computed review, Back,
  stale rejection after an independent CLI edit, actual weekday/timezone and
  catch-up persistence, paused state, same-title peer isolation, management-view
  edits during independently running work, original-owner completion and restart
  with a multiline/plan draft. The controlled provider distinguishes the normal
  request-classification call from the actual execution request: exactly one of
  each, with neither repeated by controls or restart. The initial fixture held
  the classifier instead of execution; its failed log was retained and the
  fixture corrected rather than removing the no-extra-work assertion.
  Final focused native gate: **12 pass, 303 assertions**, two files, 28.77 seconds,
  in `/tmp/gator-ux-20261112-timing-native-clean-final.log`, observed exit 0 under
  NO_COLOR and reduced motion. This includes four-size multiple-action reading
  checks. The broader affected-flow gate passed **25 tests, 653 assertions**,
  six files, 135.44 seconds, in `timing-native-final.log`; subsequent changes
  expanded Ctrl reading to multiple-action reviews and were checked by the final
  focused gate. Go application/scheduler full regressions pass (39.709/4.697
  seconds), and CLI goal/follow-up/control/shortcut/timing checks pass (41.379
  seconds), in `timing-go-full.log` / `timing-command-regression.log` beneath the
  same `/tmp/gator-ux-20261112-` prefix. Go tests also verify frozen snapshot and
  account/contract preservation, scheduler-counter updates during review,
  cancellation, malformed/impossible schedules, stale and missing identities,
  and daylight-saving transitions. Typecheck, runtime build and bundle pass.
  These are native fixture, real Go/store and controlled-provider checks, not
  physical terminal/audio or live account/model-quality verification. The full
  post-change terminal regression and final combined journey are still pending.
  Row 2 remains open for unscheduled-card workspace/next-action clarity and that
  final journey; all fifteen original criteria remain in scope.

- Complete regression after reviewed goal controls (12 November): the interrupted
  observation was revalidated against the terminal log and process table. The
  runner is no longer live and its final summary reports **170 pass, 3 existing
  skips, 0 failures**, 3,437 assertions across 173 tests / 77 files, 553.49
  seconds, in `/tmp/gator-ux-20261112-goal-controls-full-tui.log`. Source and tests
  stayed unchanged during the run. This includes the new real-Go goal controls
  and cancelled-read journeys alongside the existing terminal regression. It
  does not replace the final combined acceptance journey, physical audio, or
  live account/model verification. All fifteen criteria remain in scope.

- Reviewed goal controls in Following (`08bc8aea1`, `1bb9e8994`, 12 November):
  active and paused goals expose retained-plan inspection and reviewed status
  changes through the canonical Go goal store. A saved record digest binds the
  responsibility, workspace, linked chat, notifications and current status.
  All existing goal-record writers share one process-held lease, so status
  changes cannot overwrite concurrent context/notification/conversation edits.
  Missing, malformed, stale and wrong-target identities are rejected; current
  records that change again after saving are not presented as a confirmed current
  status. Completed goals cannot be resumed through this control. Canonical
  uppercase hex identities remain supported without renaming existing records.
  The manual bridge accepts only inspect/pause/resume; no source, account,
  replacement policy, execution or approval grant travels in that frame.
  Inspection reads current outcomes, workspace, plan steps, deadlines,
  dependencies, evidence, recurrence and check health. It remains read-only in
  the executing owner; its separate busy state does not take queue/task/approval
  ownership or clear another control's busy state. Mutations during work reuse
  the established management view. Status changes retain saved check definitions;
  confirmations explain existing work may finish, other goals stay unchanged,
  and resuming a goal does not start a task/service or re-enable paused/ended
  check agreements. Post-save health failures explicitly report the saved status
  and unavailable refresh rather than implying nothing changed.
  Goal inspection reuses the cancellable read view: closing it retires late
  success/failure, cannot reopen a dialog/change pages, and does not stop work.
  Host runtime retention, reconnect/profile-maintenance and update guards include
  its pending read. Scoped recovery clears only the earlier matching goal error.
  Goal and check menus have distinct titles, preventing a same-title check's
  filter from hiding the goal controls. Escape from the new status confirmation
  restores the parent menu/filter. Following retains selected commitment identity
  through reordered refreshes and keeps a tall card's title in view.
  Native verification under NO_COLOR and reduced motion: **12 pass, 326
  assertions**, four files, 40.07 seconds, in
  `/tmp/gator-ux-20261112-goal-controls-native-safe-read.log`, observed exit 0.
  Real Go / controlled-provider journeys at all four sizes cover paused-plan
  inspection, cancelled and confirmed resume, a stale pause rejected after an
  independent CLI correction, fresh pause, restart, multiline/plan drafts,
  inspection during held work, management-view mutation, original task completion
  and one provider request. Peer-goal and check-definition bytes and the retained
  plan stay unchanged. Separate native fixtures at all four sizes close delayed
  successful and failed inspections with Escape/Ctrl+C, then navigate away;
  results stay retired and an unrelated issue/draft/running owner are preserved.
  Full Go goal/store and workapp regressions pass (4.882/26.700 seconds); CLI
  goal/follow-up/shortcut checks pass (35.091 seconds). Focused final Go checks
  also verify canonical identity compatibility and current-status confirmation.
  Host full regression: **133 pass, 1 existing skip, 0 failures**, 134 tests,
  26.64 seconds; final policy/session ownership recheck: **40 pass**, 0 failures.
  Typecheck, frontend bundle and final runtime build pass. This is native fixture,
  real Go/store and controlled-provider evidence, not physical terminal/audio or
  live account/model-quality verification. Advanced non-project schedule editing
  is still open in row 2. The complete terminal regression now passes as recorded
  above; the final combined acceptance journey remains pending. All fifteen
  criteria stay in scope.

- Bounded long-history rendering (`03f743fb5`, 12 November): history remains in the original
  immutable snapshot and global transcript search. Only the native content
  window mounts: 80 items once history exceeds 160 items. Measured/estimated
  spacers support ordinary scrolling and scrollbar jumps, not a truncated latest
  page. Distant search reveals mount the original item before aligning it;
  contextual actions retain its original backend message index/run/account data.
  Ctrl+Home reaches earliest retained content; configured latest and catch-up
  mount their actual target. Short histories retain their established native
  scroll behavior. An active text selection pins its native consumers and lets
  the mounted range expand; clearing it prunes back to the normal window.
  Width changes, asynchronous Markdown sizing and shrinking/paged snapshots
  preserve item anchors. The window owns estimated-geometry restoration; the
  retained reader owns initial bookmark restoration. This prevents the old
  reader from undoing a deliberate scroll while a window settles. Native
  content/style lifetime uses the existing explicit cleanup, with no renderer
  fallback or hidden history removal.
  At all four sizes, native fixture journeys with 1,000 retained messages verify
  earliest content, scrollbar movement into unmeasured history, distant search,
  exact item actions, selected text/consumer identity across a window boundary,
  pruning after clear, resizing, original off-window approval review, disconnected
  state, paged snapshot reindexing, new content, restart and catch-up, while keeping
  the original multiline/plan draft. Native approval rendering here is fixture
  evidence; actual Go token enforcement remains covered by the existing
  approval integration tests. The first fixture assertion explicitly waits on
  the real native Markdown worker's clock (bounded by two seconds inside the
  unchanged 20-second journey deadline), rather than exhausting synchronous
  frame passes before the worker finishes. No data or keyboard assertion is
  removed. Existing short-history/composer journeys pass alongside it.
  The unchanged reproducible benchmark passes its actual-readable-latest and
  exact reading-row-on-append assertions. Report:
  `docs/benchmarks/tui-transcript-windowed.json`, compared with
  `tui-transcript-reading-after.json`. Five thousand retained messages:
  startup 4,098→218 ms, scroll 395→39 ms, append 267→35 ms, resize 750→76 ms;
  JS heap 323→63 MiB, RSS 841→316 MiB. Single sequential synthetic samples, not
  a statistical or physical-terminal claim; initial native/Markdown warmup
  affects the first dataset. Selection can intentionally expand the normal
  native window to preserve the selected content. Focused native regression:
  **12 pass, 355 assertions**, four files, 35.83 seconds, under NO_COLOR and
  reduced motion, in `/tmp/gator-ux-20261112-window-native-final.log`.
  After clamping spacer indexes for shrinking snapshots and strengthening the
  draft to multiline, final window journeys pass again: **4 pass, 156 assertions**,
  22.63 seconds, in `/tmp/gator-ux-20261112-window-native-multiline.log`.
  Both processes were observed to exit 0. Typecheck and frontend bundle pass in
  `window-check-verified.log` / `window-build-verified.log`. Go/host code and
  protocol/approval enforcement are unchanged by this commit. The complete current terminal regression passes: **161 pass, 3 existing skips,
  0 failures**, 3,193 assertions across 164 tests / 75 files, 510.34 seconds,
  in `/tmp/gator-ux-20261112-window-full-tui.log`, polled to exit 0. Production
  source and tests stayed unchanged during that run. Row 13 is implemented;
  final combined acceptance remains pending. All fifteen criteria
  remain in scope; this does not complete the goal.

- Durable reading and catch-up (`614d89a1a`, 12 November): `/catch-up` starts at the first
  item since the last latest-view boundary; revisions of the same streamed
  answer count once. `/latest` and the configurable `jump_latest` binding
  (Ctrl+L by default, with Ctrl+End retained) return to newest content. The
  configured hint and catch-up command share the existing composer hint row;
  entering/leaving earlier reading does not resize the transcript. Combined
  attachment/reply/plan/slash/error states retain at least four transcript rows
  at the four required sizes. Global command search includes the descriptions.
  A host-owned presentation store retains only opaque item/content fingerprints,
  an index hint, a signed line offset and the last latest-view boundary. It
  stores no message text, account data, approvals, drafts, settings or task state.
  Profile/chat files are bounded, private, atomic and reject linked/malformed
  inputs. Host read/write failures explain recovery without changing history.
  View identities do not confer authority: contextual actions still use the
  original backend indexes, run IDs, sources and owners. A completed run's new
  revision ID does not change that run's reading identity. Switching owners
  cannot capture the incoming chat as the departing chat's position.
  Native fixtures exercise inserted earlier history, growth above/below the
  reader, size changes, chat/page navigation, restart, two new answers, catch-up,
  configured Ctrl+Shift+L under the controlled Kitty keyboard protocol, and
  successive fragments of one answer, with preserved multiline/plan drafts.
  The native benchmark exposed a lost margin offset and then invalid assumptions
  about the order of native slot placeholders. Both are fixed: signed offsets
  retain blank gaps, and geometry search includes only actual timeline boxes.
  The original exact-row append assertion remains unchanged and passes.
  Native bounds lookup is logarithmic rather than reading every item's bounds
  each frame. Weak caches hash unchanged timeline items once. Stable local
  route/draft/voice/error projections stop a reading-status change from rebuilding
  the global key registry and thousands of unchanged message hints.
  Fresh before/after reports are in `docs/benchmarks/tui-transcript-reading-*.json`.
  They are single samples, not a statistical performance claim: 5,000-message
  append improves 371→267 ms; startup/scroll/resize remain expensive and all
  messages still mount. Bounded rendering and the final integrated journey stay
  open. Final affected-flow regression under NO_COLOR and reduced motion:
  **58 pass, 0 failures**, 1,303 assertions, 18 files, 237.61 seconds; polled to
  exit 0 in `/tmp/gator-ux-20261112-reading-native-regression.log`. This includes
  ordinary conversation during execution, settings persistence, exact approval,
  original-result editing/replies, memory provenance, queues, voice controls,
  connection recovery, menu restoration and all four reading/composer sizes.
  Typecheck and frontend bundle pass. The Go runtime rebuild passes; config,
  shortcut-controller and affected help checks pass. The shortcut conflict
  fixture now uses free Ctrl+K because Ctrl+L has a product action; it still
  rejects the same concurrent claim, and the controller test now persists and
  inspects a custom latest binding. Host full regression: **131 pass, 1 existing
  skip, 0 failures**, 132 tests, 18.24 seconds, in
  `/tmp/gator-ux-20261112-reading-host-full.log`; after adding signed gap offsets,
  the two reading-store checks pass again in `reading-store-verified.log`.
  Native, host and Go processes were observed to successful exit, and source
  stayed unchanged during the final native run. The complete 155-test terminal
  run recorded below predates this feature; this entry does not present the
  18-file affected-flow run as another complete suite. This is native
  fixture/render and real host-store/config evidence, not
  physical terminal/audio or live-model/account verification. All 15 criteria
  remain in scope; this does not complete row 13.

- Correction precedence and withdrawal for continuations (12 November): before
  the next model request, the existing execution owner resolves previously
  referenced saved knowledge against the authoritative scoped stores. The
  bounded current-status input supplements historical messages; it does not
  rewrite them, change the reviewed request/plan, grant tools, or make another
  model call. It distinguishes current, omitted, missing/forgotten, expired,
  outside-scope, review-needed, unavailable and explicit-read-needed records.
  No unavailable/private value is included in the refresh. Source labels and
  text are bounded, Unicode stays intact, and JSON escaping counts against a
  16 KB input budget. Up to 128 validated private reference IDs survive
  compaction/restart; 32 current statuses are supplied, with overflow and
  truncated/full-read needs disclosed. The scoped memory list is resolved once
  per request rather than rescanned for each ID.
  The exact current-status payload is sealed in the same evidence catalogue and
  exposed beside the original reads/searches in answer context inspection.
  Refresh entries have readable status messages and an explicit saved-item ID.
  The source inspector identifies a request-start status snapshot separately
  from a model-invoked tool result.
  Controlled real-host reproduction now reports `same_owner=true`,
  `saved_correction=true`, `correction_supplied_to_followup=true`,
  `older_tool_response_supplied=true`, three provider requests. Log:
  `/tmp/gator-ux-20261112-memory-precedence-fixed.log`.
  Native regression: **11 pass, 351 assertions**, five files, 67.44 seconds,
  covering initial/dynamic knowledge, correction, forgetting, privacy and
  restart at all four sizes. Final native refresh run with NO_COLOR and reduced
  motion: **4 pass, 216 assertions**, 33.87 seconds; actual outbound requests
  contain the correction and withdrawal, the older tool message remains, and
  users inspect both historical and refreshed inputs. Logs:
  `/tmp/gator-ux-20261112-knowledge-refresh-native-verified.log` and
  `knowledge-refresh-native-final.log`.
  Focused Go checks cover actual context compaction, subsequent reopening,
  historical byte preservation, corrupt/missing/changed-source/expired/foreign/
  omitted states, escaped-input budgets and private-ID validation. A real
  reviewed read-only plan receives the corrected fact but still rejects an
  attempted artifact write; its digest stays unchanged. Full workrun/workapp
  regressions pass (94.287/32.765 seconds); full conversation persistence passes
  (38.548 seconds); focused context integration passes. Host: **129 pass,
  1 existing skip, 0 failures**, 130 tests, 24.97 seconds. Typecheck passes.
  After `994e57920`, the complete terminal regression passes: **152 pass,
  3 unchanged skips, 0 failures**, 2,894 assertions across 155 tests / 73 files,
  501.74 seconds. Log:
  `/tmp/gator-ux-20261112-knowledge-refresh-full-tui.log`, polled to exit 0.
  Production source and tests stayed unchanged during the complete run. This is
  controlled-provider/native-render and Go/store evidence, not physical
  terminal/audio or live-account/model-quality evidence. All fifteen criteria
  remain in scope. Row 3 is implemented with final combined journey pending.

- Dynamic answer memory provenance (`bda11baf3`, 12 November): successful knowledge-search
  and read tool messages are retained after lifecycle hooks in the existing
  sealed evidence catalogue. Locators bind the originating run and call; raw
  response text is not copied into history/telemetry metadata. Search previews
  record their actual scope and bounded source labels; full reads keep the exact
  original bytes. Context reconstructs those responses only after digest and run
  checks. It distinguishes full reads, search previews and earlier responses
  still present in the current revision's retained model context. Copying a
  source forward alone does not claim it was supplied after compaction. Carried
  excerpts from another goal stay inspectable without current-record access or
  correction authority. Missing/changed evidence produces an explicit failure.
  Current saved text/revision stays separate from historical input; corrections
  and forgetting use the existing store guards. Sources can inspect the retained
  response through its exact evidence ID. Long context lists show short summaries
  while details preserve the complete supplied body and support keyboard scroll.
  Context list/detail/edit/review frames have presentation-only reading identities;
  Back restores the list filter/position without reusing action authority or form
  secrets. Refresh cannot make Enter activate a hidden filtered row. Explicit
  transcript reveal now handles an item exactly as tall as the viewport, fixing
  wrong-item actions at 42×14. Context refresh is no longer mislabeled as review.
  Native/real-Go/controlled-provider journeys: **8 pass, 252 assertions**, 45.62
  seconds, at all four required sizes, including run-start and dynamic context,
  correction, reviewed note forgetting, long-note keyboard reading, restored
  filters, hidden-row safety, carried context, preserved drafts and restart.
  Log: `/tmp/gator-ux-20261112-dynamic-context-native-final.log`.
  Focused Go transaction/context/tool checks pass. Full `workrun` and `workapp`
  regressions pass (107.090 and 39.559 seconds). The initial tools regression
  exposed the valid escaped-metadata bound bug fixed by `86c1b562f`; the full
  tools/knowledge rerun passes in `dynamic-context-tools-verified.log` (tools
  10.416 seconds). Host: **128 pass, 1 existing skip, 0 failures**, 129 tests,
  22.75 seconds. Typecheck passes. The complete native regression exits with
  **151 pass, 3 existing skips, 1 failure**, 2,830 assertions across 155 tests /
  73 files, 484.40 seconds, in
  `/tmp/gator-ux-20261112-dynamic-context-full-tui.log`. Source/test files stayed
  frozen during that run. The one failure was the older saved-knowledge journey
  pressing plain E/O/F after those visible controls became Ctrl+E/Ctrl+O/Ctrl+X.
  It now exercises the actual Ctrl controls with every original data/conflict/
  omission/forget/restart assertion and its deadline preserved. Related native
  recheck: **14 pass, 0 failures, 150 assertions**, four files, 18.11 seconds, in
  `/tmp/gator-ux-20261112-knowledge-ctrl-verified.log`, polled to exit 0. This fixes
  the failing check; it is not presented as a second fully green complete run.
  These are native-render/controlled-provider and Go/store checks, not physical
  terminal/audio or live model/account verification. All fifteen criteria remain.

- Verified remaining correction-precedence gap (12 November): an isolated real
  Go host with a controlled model read a global fact through `knowledge_read`,
  corrected it in the authoritative memory store, then continued in the same
  owner. The saved correction succeeded, but the follow-up's model messages
  included the old tool response and omitted the corrected fact. Diagnostic
  result: `same_owner=true`, `saved_correction=true`,
  `correction_supplied_to_followup=false`, `older_tool_response_supplied=true`,
  three provider requests. Log:
  `/tmp/gator-ux-20261112-memory-precedence-diagnostic.log`; reproducible isolated
  driver: `/tmp/gator-memory-precedence-check.ts`. This is controlled transport
  evidence, not a model-quality claim. Next work must make subsequent requests
  receive authoritative scoped corrections/withdrawals, respecting exclusions,
  provenance, goal boundaries and budgets while preserving historical evidence.
  Row 3 remains in progress; successful persistence/provenance alone is not enough.

- Saved-note metadata bounds (`86c1b562f`, 12 November): a new source-preview regression
  exposed that sixteen valid escaped source labels could be written successfully
  but exceed the 16 KB metadata reader limit. The reader now permits bounded
  64 KB metadata, enough for the validated label contract and JSON escaping.
  Body/label limits, regular-file and directory/symlink checks remain enforced.
  Maximum escaped labels round-trip through loading and correction without
  truncation; oversized metadata still fails. `go test ./internal/knowledge`
  passes in 2.882 seconds. Log:
  `/tmp/gator-ux-20261112-knowledge-metadata-go.log`.

- Reviewed noise feedback (`08c12a19d`, 12 November): only a successfully acknowledged
  noise rating opens the original update's linked check controls. The user can
  edit notification conditions/timing in the canonical form, review a pause,
  inspect the affected agreement, or keep feedback alone. Missing check linkage
  explains that behavior stayed unchanged. Exact check digests and context are
  revalidated; a failed or stale mutation cannot report success. The shared pause
  review names its check, source scope, timing, current-run limits and effect on
  future checks. Conversational follow-up reviews show the old/proposed
  notification condition, changed frequency and original check ID. Both entry
  paths use existing Go stores, reviews and safeguards.
  Four native/controlled-provider journeys now pass at 110×34, 80×24, 60×20 and
  42×14: save a noise rating, review/edit the exact original check, ask for a
  conversational notification/frequency change, approve its exact agreement,
  reject a pause changed by another terminal, confirm a fresh pause, and restore
  after restart. An unrelated check remains byte-for-byte unchanged and manual
  actions add no model calls. The job updates are inserted fixtures; no live
  scheduler/account or physical terminal verification is claimed.
  Relevant native regression: **20 pass, 0 failures, 494 assertions**, nine files,
  241.08 seconds, polled through exit 0. It includes the new four-size noise and
  answer-context journeys plus memory correction, update receipts/feedback,
  conversational follow-ups, app settings controls, Following and exact result
  actions. Log: `/tmp/gator-ux-20261112-memory-noise-native-regression.log`.
  Typecheck and focused Go follow-up/app-control tests pass. The before/after
  notification review has a Go assertion for the original check and scope.
  No implementation or test files changed during this native regression.

- Broader verification after memory inspection `cb095fcc4`: both full Go packages
  pass (`internal/workapp` 36.008 seconds; `cmd/gator` 166.901 seconds).
  Host suite: **127 pass, 1 existing skip, 0 failures**, 128 tests, 23.35 seconds.
  Typecheck passes. Logs: `/tmp/gator-ux-20261112-answer-context-go-regression.log`,
  `answer-context-host-regression.log`, and `answer-context-typecheck.log`.
  The subsequent scoped follow-up review tests also pass after noise changes.

- Remaining memory boundary from implementation inspection: the current context
  panel reconstructs `Decision.ContextItems`, captured at run start. Dynamic
  `knowledge_search`/`knowledge_read` results can supply additional memories or
  notes later. They need retained, owned-run provenance in this inspection flow;
  current memory receipts and a passing initial-recall test do not prove that
  boundary complete. Reuse retained evidence/history rather than a second memory
  store. Row 3 stays in progress, with all fifteen original criteria preserved.

- Answer-specific memory inspection (12 November): contextual result actions
  now navigate to the selected answer's exact retained run. Host syntax admits
  bounded timestamp IDs; Go validates that run against the current conversation
  or the management worker's trusted original ancestry. Foreign/malformed IDs
  and vanished origins cannot read context through this route. Inspection during
  work creates a management view without replacing execution. That view can
  correct or forget saved knowledge but explicitly directs next-message
  omissions back to their original conversation. Context Ctrl controls take
  precedence over the old global help/picker shortcuts. Historical excerpts,
  current saved text, provenance and correction effects remain distinct.
  `answer-context.test.tsx`: **4 pass, 88 assertions**, 20.36 seconds, at 110×34,
  80×24, 60×20 and 42×14. The real Go/history/memory boundary, controlled model
  endpoint and native rendering exercise correction, exact forget review,
  ongoing-task preservation, independent drafts, restart and stale callback
  rejection. Log: `/tmp/gator-ux-20261112-answer-context-native-final.log`.
  These are controlled-provider/native checks, not physical terminal or live
  model verification. Row 3 remains in progress: scoped reviewed noise feedback
  and the final integrated journey are still required.

- Full terminal regression confirmed after `2b1f38392` (12 November): **140 pass,
  3 unchanged skips, 0 failures**, 2,486 assertions, 143 tests / 70 files, 390.15
  seconds. The process was polled to its successful exit; no implementation/test
  edits occurred during that final run. Typecheck passes in
  `/tmp/gator-ux-20261112-menu-restoration-final-typecheck.log`.
  All fifteen criteria remain in scope. A green regression does not prove the
  remaining product behavior or the final integrated acceptance journey.

- Memory-surface investigation for the next implementation: Go's
  `readWorkContext` already retains the supplied excerpt separately from current
  saved text and revision-guards corrections/forgetting. However,
  `NavigateBridge` currently accepts exact navigation commands; simply linking
  an answer to `/context RUN_ID` through navigation would silently fail.
  Answer-specific context needs an original-owner-validated path, including
  managed views during work, before connecting contextual UI controls. Memory
  origin receipts alone do not establish which memories a later answer received.

- Shared frontend Back restoration (`2b1f38392`, 12 November): menu navigation retains presentation snapshots per parent/context,
  without copying execution authority. Back reconstructs explicit parent choices
  when available, restores search/selection/list position by stable choice key,
  and keeps input focus. Reopening a new root/context clears navigation. Retired
  pickers release native references; pending layout restoration is cancelled on
  cleanup. Selection scrolling now follows the native layout frame, including
  narrow-list overflow changes. Following page entries start fresh navigation;
  successful pause/resume retires old agreement controls before its result view.
  Proposal labels and retained-run status remain explicit in result inspection.
  Narrow custom queue hints keep the complete configured binding visible.
  Eight focused native fixture journeys pass at all four sizes, including nested
  preview → file → result-list Back, exact parent-frame equality and selected
  identity after rows are removed/reordered. With NO_COLOR and reduced motion:
  **8 pass, 92 assertions**, 20.36 seconds, in
  `/tmp/gator-ux-20261112-menu-restoration-no-color.log`.
  Related result/source/receipt/read/model/command journeys initially passed
  **19 tests, 357 assertions**; the later Following/navigation/queue check passes
  **16 tests, 209 assertions** in `menu-restoration-focused-final.log`.
  First complete terminal run recorded **124 pass / 3 existing skips / 16 fail**
  (143 tests / 70 files). It exposed old one-Escape-to-close expectations,
  searches appending to now-restored queries, stale successful-check navigation,
  missing proposal/status wording from earlier result work and a clipped long
  custom queue binding. Those are corrected. Tests explicitly exercise Back then
  close, or Ctrl+U to start a different search, without dropping assertions or
  changing deadlines. File read failures are checked in their contextual recovery
  view. The second complete run recorded **139 pass / 3 skips / 1 fail** in
  `menu-restoration-full-tui-verified.log`; the remaining plan editor journey now
  explicitly clears its restored query and passes **3 tests, 27 assertions** in
  `menu-restoration-plan-fixed.log`. Original failure logs are retained.
  The final complete run exits successfully: **140 pass / 3 unchanged skips /
  0 failures**, 2,486 assertions across 143 tests / 70 files in 390.15 seconds.
  Log: `/tmp/gator-ux-20261112-menu-restoration-complete-tui.log`. No behavior or
  test files changed during this final run. Commit typecheck also passes. The
  image-paste/old-voice/managed-browser skips remain skips; controlled checks do
  not become physical verification. This is native-render/controlled
  Go/provider evidence, with separately identified fixture navigation. Physical
  terminal verification and the final integrated journey remain unproven.
  All fifteen criteria remain in scope. Typed editors, file-editor restoration,
  backend pages and durable conversation reading positions remain under 10/13.

- Conversation/delegated-result contextual actions (`ecd1d9094`, 12 November): the configured
  item shortcut and `/actions` include conversation replies and delegated task
  summaries. Their turn ID, original parent/task identities, retained input IDs
  and peer snapshot stay bound to the selected item; changed turns or mismatched
  task metadata require a fresh selection. Inspection/copying uses retained
  content. Complete outputs and reviews open through the existing canonical task
  owner, retaining the parent's independent draft and next-message plan intent.
  Ordinary replies with no legacy transcript messages also expose the configured
  action hint near the composer. No extra model request, execution owner or
  settings store is introduced.
  Native real-Go conversation journey now inspects/copies the delegated result,
  opens its retained input, navigates to the original child and back, and rejects
  foreign-owner/task targets at all four sizes while the parent task keeps
  running. Contextual Ctrl actions also copy the current conversational reply.
  Existing steering, ambiguous routing, isolated task stop, retained results,
  durable request counts and restart assertions remain in the same journey.
  Combined conversation/read-cancellation/composer/command checks: **10 pass,
  335 assertions**, 41.08 seconds, in
  `/tmp/gator-ux-20261112-delegated-actions-regression.log`.
  With `NO_COLOR=1 GATOR_REDUCED_MOTION=1`, conversation/composer checks plus
  four-size dialogue-only render fixtures with Ctrl+K and stale-turn rejection:
  **8 pass, 222 assertions**, 25.27 seconds, in
  `/tmp/gator-ux-20261112-delegated-actions-verified-native.log`. Typecheck passes.
  Earlier runs exposed test assumptions about wrapped words and restoration
  before the native layout frame; the journey now reads wrapped text and waits
  for that frame before keyboard navigation. A new fixture initially omitted
  its required binary argument; typecheck and the failed render caught it. The
  fixture is isolated to its temporary profile with an unused binary. Logs for
  earlier failures remain under `/tmp/gator-ux-20261112-delegated-actions-*`.
  Final isolated-profile fixtures pass **4 tests, 36 assertions**, 5.67 seconds,
  in `delegated-actions-fixtures.log`; commit typecheck passes in
  `/tmp/gator-ux-20261112-delegated-actions-commit-check.log`.
  This is controlled-provider/native-render evidence, with explicit render-only
  fixtures. It is not physical terminal, audio or live-account verification.
  Structured connected-item actions, complete navigation/reading restoration,
  the final combined journey and all other open findings remain in scope.

- Contextual connected-request receipts (`c4af06b90`, 12 November): an answer or warning card
  exposes requests through its exact run ID; `/receipts` reuses those controls.
  Both retain original account/destination/action/correlation/manifest/context,
  reject changed or unavailable records and preserve metadata-only crashed
  dispatch recovery. Global receipt browsing deduplicates retained/recovery rows.
  Supported uncertain Gmail sends offer local saved-check inspection and an
  explicit read-only Sent check; unsupported checks are not offered. Read views
  preserve original item Back, cancellation and contextual read retry. Closing
  an in-flight check leaves its durable outcome inspectable without reopening
  the retired view. A latest-only retained bundle is accepted by the Go receipt
  boundary with unchanged original-owner, manifest and exact-action validation.
  Native real Go + controlled model/mailbox journeys pass at all four sizes:
  exact approval → original warning/result actions → change future account →
  inspect original receipt → cancel a delayed original-account Sent read →
  inspect saved result → deny read permission → explicit repair/retry → restart.
  The service records one send and only the original account's subsequent reads;
  the other configured account is never contacted. Draft/review intent and
  sealed manifest bytes remain unchanged. Existing unknown-send and crashed
  dispatch recovery tests also pass: **6 tests, 125 assertions**, 28.83 seconds,
  in `/tmp/gator-ux-20261112-context-receipts-native-rerun.log`.
  Focused Go controller/command tests pass, including latest-only stale manifest
  and foreign-owner rejection, exact Sent matching, manifest replacement and
  permission revocation. Runtime rebuilt; final typecheck passes. Logs:
  `/tmp/gator-ux-20261112-context-receipts-{go,build,final-go,final-check}.log`.
  The initial native run recorded **2 pass / 4 fail** because the new journey
  searched assistant wording instead of the retained warning card. It now reveals
  the exact run-bound response and uses the real contextual Ctrl shortcut. Initial
  typecheck caught one old helper reference during refactoring; the shared action
  registry replaces it. Original deadlines remain unchanged; failure logs remain.
  These are controlled-provider/native integrations, not a live mailbox or
  physical terminal verification. All fifteen findings stay in scope; structured
  connected-item replies and delegated-result contextual coverage remain under 6.
  Combined receipt/command/approval/read-cancellation regressions: **14 pass,
  306 assertions**, 48.37 seconds, in `context-receipts-regression.log`.
  With `NO_COLOR=1 GATOR_REDUCED_MOTION=1`, all six receipt journeys pass again:
  **6 pass, 125 assertions**, 27.46 seconds, in
  `/tmp/gator-ux-20261112-context-receipts-no-color.log`.

- Retained-read cancellation and recovery (`cb07b35fe`, 12 November; builds on the loading
  view retained in `d0ce6e5e0`): native journeys use real Go-created sealed files
  and source evidence, with controlled host response delays and one injected
  temporary file-read failure. Ctrl+C retires the read view and its UI operation;
  late success/failure cannot reopen it or surface an obsolete error after
  navigation. Esc returns source reads to their source list, and file reads now
  retain their original file-action Back callback. Ctrl+R explicitly retries the
  same bound read. Draft text/review intent and sealed manifests stay intact.
  Cancelling a file read during a real Go-owned, controlled-provider task leaves
  that task running with its original owner/control identity, and it finishes
  when the provider is released. Closing the view does not claim to recall an
  already started host request. Four-size cancellation/recovery plus existing
  result/evidence/reply/generated-file/Office journeys pass together: **11 tests,
  249 assertions**, 85.64 seconds. Typecheck passes. Logs:
  `/tmp/gator-ux-20261112-read-view-{integrated,final-check}.log`.
  The same four-size cancellation/recovery journeys also pass with `NO_COLOR=1`
  and `GATOR_REDUCED_MOTION=1`: **4 tests, 100 assertions**, 15.51 seconds,
  in `/tmp/gator-ux-20261112-read-view-no-color.log`.
  The initial test run failed on expecting `null` where the native lookup returns
  `undefined`; the assertion now follows the observed API. Initial typecheck
  caught the test wrapper's overloaded file signature; forwarding wrappers retain
  the host overload type and production contracts remain unchanged. Earlier
  failures remain in `read-view-{native,typecheck}.log`; the subsequent standalone
  four-size run passed before adding the file-action Back assertion.
  These are native/controlled-provider integrations, not physical terminal or
  live-account checks. All fifteen criteria remain in scope. Contextual connected
  receipt controls and their account/permission journeys are still open under 6;
  full menu/filter/reading restoration remains under 10.

- Contextual terminal result actions (11 November): Ctrl+E opens a visible item;
  an explicitly revealed message stays the target until reading moves. `/actions`
  searches other items, and message/file/update labels open their own controls.
  Go owns the configurable shortcut, preserves older conflicting custom bindings
  and projects the actual hint. Narrow composer hints reserve their own width.
  The answer joins its separately retained deliverables by exact run ID. File,
  source and code controls validate the retained bundle and original context;
  update controls validate identity/version and await their own Go outcome.
  Original account/operation/source metadata remains inspectable. Older code
  proposal records remain inspectable when an exact sealed patch is unavailable.
  Supported update replies retain their exact original update ID in the editable
  draft and durable admission; no generic external reply path is introduced.
  File editing reuses the host's working-version store and preserves the sealed
  bytes/manifest. Read-only evidence returns through its original item menus.
  The short file editor reserves space for editing and save/version controls.
  Native Go + controlled provider journeys pass at **110×34, 80×24, 60×20 and
  42×14: 4 tests, 116 assertions**, including a configured Ctrl+K shortcut,
  source paging, copying, immutable file edits, a rejected other-chat target,
  versioned history-picker restoration, acknowledged done and explicit contextual
  reply. Log: `/tmp/gator-ux-20261111-result-actions-native-final.log`.
  Earlier native runs exposed selection of an adjacent notice, hidden/clipped
  narrow hints and assumptions about asynchronous source loading and visible
  menu rows. The selection/hint layout is fixed; the journey waits for actual
  detail/render/receipt transitions and searches a bounded menu rather than
  assuming every item fits. Original test deadlines remain unchanged. Earlier
  `result-actions-native*.log` failures are retained. Typecheck passes in
  `/tmp/gator-ux-20261111-result-actions-verified-typecheck.log`.
  These are native/controlled-provider checks, not physical terminal, audio or
  live account/model verification. Item 6's contextual connected-account receipts
  and delayed-read cancellation/failure journeys remain, as do the other open
  criteria and the full final acceptance journey.
  Related command/composer/update/evidence/branch/generated-file/Office journeys
  pass together: **16 tests, 329 assertions**, 133.70 seconds, in
  `/tmp/gator-ux-20261111-result-actions-combined.log`. The earlier related run
  recorded **11 pass / 1 fail** because the new footer omitted a remapped command
  shortcut. The footer now retains the actual command binding at wider sizes;
  the existing assertion and deadline remain unchanged. Go configuration tests
  pass, including preservation of an older conflicting binding and explicit
  item-action remapping. Opening visible-item actions indexes native message
  boxes once instead of repeatedly descending the transcript tree; this does
  not claim completion of the long-history benchmarks or reading-anchor work.
  After that lookup change, the native four-size result journeys and composer
  regressions pass again: **6 tests, 168 assertions**, 69.11 seconds, in
  `/tmp/gator-ux-20261111-result-actions-final-native.log`; final typecheck passes
  in `/tmp/gator-ux-20261111-result-actions-final-check.log`.

- Result-action backend boundary (`a5403905f`, 11 November): explicit update read/done/reopen
  and paging controls may carry a bounded request nonce and receive their own
  Go outcome. Legacy passive delivery receipts remain valid. Rejected stale,
  missing, cross-conversation and malformed targets cannot claim success;
  a previous issue cannot turn a later successful action into a false failure.
  Source inspection accepts the current conversation's latest retained bundle
  when it has no transcript bundle, using the same sealed manifest/hash/run and
  original-conversation checks. No live workspace or account is queried.
  Focused Go chat-update/feedback regressions pass; host policy/evidence tests:
  **35 pass**, no skips. Logs:
  `/tmp/gator-ux-20261111-result-receipts-go.log` and
  `/tmp/gator-ux-20261111-result-evidence-host.log`.
  Contextual terminal controls and their integrated journeys are still open;
  this boundary change does not complete item 6.

- Fresh complete terminal regression after `fdd10b37f` (11 November): **116 pass,
  3 unchanged skips, 0 failures**, 1,983 assertions across 119 tests / 66 files
  in 254.75 seconds. No implementation or test files changed during this run.
  The command workflow now checks actual saved appearance and absence of an
  extra task submission. All settings/first-task/plan/cached-owner/recovery
  checks pass together with the unchanged command deadline. Log:
  `/tmp/gator-ux-20261111-terminal-settings-full-tui-verified.log`.
  The existing image-paste/old-voice/managed-browser skips are retained as skips;
  separate controlled integrations do not become physical verification.
  All fifteen findings remain in scope; this regression does not complete the
  remaining product criteria or the final combined acceptance journey.

- Unified conversational/manual terminal settings (10–11 November): terminal
  appearance has one authoritative value, `Settings.Theme`; layout preferences
  live in the same Go configuration document. The native host imports retained
  frontend/Chromium values once, preserving the original database and retaining
  the frontend file as an inactive import source. A newer explicit theme wins an
  import race without losing old navigation/grouping choices. No active second
  settings store or execution path is introduced. Partial settings transactions
  preserve unrelated acknowledged edits and reject a changed target field.
  Manual controls and `gator_app_inspect` / `gator_app_update terminal` use this
  boundary. The legacy `theme` setting and CLI theme command use it too, including
  following terminal colors. Unsupported old work-details expansion preferences
  are retained as data rather than advertised as active terminal controls.
  The UI follows Go's acknowledged projection, refreshes cached owners without
  replaying work, and no longer copies old chat themes into a frontend file.
  Import failures offer retry or reviewed default display recovery that keeps
  the original retained file. Cancelling that review makes no change. An
  unimported foreground cannot claim a conversational preference was saved.
  The existing app-inspection privacy regression caught an exact processing
  hostname entering the model-facing settings response; it is now omitted
  there while the trusted local context view retains its inspectable destination.
  Native Go + controlled provider: manual appearance → conversational actual
  inspection/change → canonical history-picker restoration of the same cached
  owner → offline rejected change → reconnect/save → restart, at all four sizes.
  Additional journeys verify cancelled/confirmed recovery from an unreadable
  legacy file, and appearance/navigation edits during executing Work without
  changing owner/control/scope or losing a newer multiline draft. Related native
  settings/app-controls/setup/model/composer regressions: **17 pass, 414
  assertions**. Host policy, legacy imports, sessions and voice helpers: **51
  pass**. Targeted Go settings/controller/privacy/foreground checks and final
  typecheck pass. Full Go config, workrun and command packages pass; workapp's
  full rerun passes after updating its appearance test to exercise the fifth
  native-colors choice while preserving light-mode and failed-save assertions.
  The first broad Go run recorded that one outdated four-choice assertion; its
  failure is retained in `terminal-settings-full-go.log` and passing rerun in
  `terminal-settings-workapp-rerun.log`. The first full terminal run recorded
  **115 pass / 3 existing skips / 1 fail** (119 tests): the command fixture still
  expected the removed extra `/theme` submission. It now checks persisted values
  and no extra task message; both command tests pass with the original deadline.
  NO_COLOR settings/semantic-color journeys pass **9 tests, 159 assertions**.
  Logs: `/tmp/gator-ux-20261111-terminal-settings-{full-tui,commands,no-color}.log`.
  A fresh full terminal run follows this committed implementation.
  Logs: `/tmp/gator-ux-20261110-terminal-settings-{integrated,host,final-go,
  final-typecheck,runtime}.log`. Initial native settings runs recorded 0/4
  because the restoration test used restricted direct navigation and then
  omitted the history row's version suffix; the actual versioned picker now
  restores the retained owner without weakening navigation checks. Initial
  typecheck caught fixture types before execution. The earlier Go privacy
  failure remains in `terminal-settings-go.log`; its current targeted rerun
  passes. These are native/controlled-provider checks, not physical terminal or
  live-account/model proof. Item 5's full permission explanations and combined
  account/settings journey remain; all fifteen criteria stay in scope.

- Guided first-request model setup (`d0fd58e48` / `20ef9a298` backend, 10 November): the Go
  configuration boundary projects missing prerequisites, a display-safe model
  processing destination and actual Work/Code step limits. It distinguishes
  managed Ollama, a potentially forwarding loopback endpoint, a remote service,
  and an unknown destination. Environment-resolved Azure destinations come from
  the constructed adapter's actual route. URL credentials/paths/queries are not projected.
  Configured means prerequisites are present, not verified endpoint connectivity.
  The same provider adapter constructor checks provider-specific configuration
  without inference or extension execution; a controlled endpoint gets no calls
  during readiness projection. No new settings store or execution path is added.
  Ordinary composer submissions use this state to offer only missing model setup;
  an already configured model is also selectable. Setup never sends the draft.
  The header says Host connected; the composer reports Model setup needed,
  without effort text obscuring it on a narrow terminal. Full setup issues are
  inspectable, including missing environment-key instructions. Recovery after
  a prerequisite changes at admission offers model repair with the original
  draft and request identity retained. Actual execution then has one durable
  task admission; orchestration may legitimately use several inference calls.
  Ctrl+T / /request-context expose processing, workspace/saved-context scope,
  selected account identities and access metadata, inputs, mode and budgets.
  Unavailable metadata does not erase a selected account. Step limits explicitly
  do not promise token or monetary caps. Optional capabilities/account setup
  descriptions come from the existing shared product guide.
  Native Go + controlled HTTP provider: a completely fresh profile, first
  request with private input, invalid provider URL, repair, exact metadata review,
  persisted selection, keyboard context inspection, and explicit useful result
  pass at 110×34, 80×24, 60×20 and 42×14. A separate journey starts with missing
  credentials, selects another model, changes its prerequisites before admission,
  repairs them via contextual recovery, and submits without duplicate admission.
  Model setup's multiline editor reserves space for its title and save/cancel
  instructions; native input bounds and the visible Ctrl+S hint are checked at
  all four sizes. Detail views reserve their scrollbar column and page by the
  visible height so exact context explanations remain readable.
  Final native/client/scope/approval/model/command/composer regressions: **38 pass,
  561 assertions**. NO_COLOR label/scope checks:
  **4 pass, 38 assertions**. Final TUI typecheck and targeted Go composer/model
  status/first-run/admission-readiness regressions pass. Runtime rebuilt from
  this checkout. Logs: `/tmp/gator-ux-20261110-setup-{verified-regression,
  no-color,commit-typecheck,final-go,final-runtime}.log`.
  Earlier new-test runs recorded 0/5 passes: they exposed a narrow setup label
  truncation and a scrollbar column clipping detail text; both are fixed. The
  test harness now emits real legacy PageDown sequences and compares readable
  paged text without decoration, and checks durable admissions rather than
  treating all model calls as distinct tasks. The first regression after editor
  sizing recorded **33 pass / 5 fail** because its new dimensions hook import
  was missing; typecheck caught the same error. The import is fixed and the
  fresh 38-test run above passes. Earlier failure logs remain in
  `/tmp/gator-ux-20261110-setup-{first-task,first-task-rerun,final-regression}.log`.
  No deadline or check was skipped.
  These are controlled-provider/native renderer journeys, not physical terminal
  or live-account/model verification. Connected-account first-request setup and
  recovery after an admitted task's provider failure remain under item 4; this
  is not completion of that item or the full fifteen-item journey.

- Retained next-message review intent (`e6ec73e89`, 10 November): the existing private draft
  store now owns `plan_first` alongside text, uploaded-file references and update
  replies; the transient client set was removed. Empty plan-first drafts survive
  restart and obey the same seven-day retention, disable and clear controls.
  Old draft records remain readable; malformed review intent leaves the file
  intact and reports recovery rather than silently selecting direct work.
  Admission uses the submitted snapshot's intent; its acknowledgement clears
  only that snapshot, preserving a newer draft's independent text/files/reply and
  mode. Commands and missing-model setup retain next-message review intent.
  Stale chat toggles fail. The recovered-draft picker identifies plan-first
  drafts and strips disk-only timestamps before restoring editable state.
  Native Go + controlled provider: choose plan-first on an empty draft, restart,
  resize through all four sizes, type and submit with Enter → exactly one plan
  proposal, no implementation before review. The existing correction/execution
  journey now also asserts the future draft's review intent after restart.
  Focused native/client/composer/queue checks: **25 pass, 314 assertions**;
  related commands/draft sync/editor/product/recovery regressions: **8 pass,
  130 assertions**; host draft storage: **7 pass**; final TUI typecheck passes.
  Logs: `/tmp/gator-ux-20261110-draft-plan-{native,related,host,final-typecheck}.log`.
  The initial client run had one assertion mismatch between an absent optional
  JSON key and an in-memory `undefined`; explicit semantic mode assertions pass
  in the fresh focused run. These are native/controlled-provider checks, not
  physical terminal or live-model verification. This closes the draft gap below,
  not the remaining navigation criteria or the fifteen-item acceptance journey.

- Conversational pending-review corrections: ordinary composer messages now reach
  the original native owner, including the current chat and independently owned
  tasks. Both paths use host admission and its three-task limit; steering/stopping
  remain available at capacity. Conversation continues during the tool-free model
  call. Actual success/failure is retained in the requesting dialogue rather than
  inferred from admission; a completed durable result survives a lost owner ACK.
  The retained correction joins the freshly reviewed execution prompt, while
  unsubmitted future draft files/text remain outside the task's model context.
  Original task IDs, file references, scope and work limits are preserved. Old
  plan approvals fail, and no implementation runs before a fresh keyboard review.
  Plan inspection shows the correction, its status/issue and original input names.
  Provider failure requires an explicit new correction message; cancellation and
  orphan recovery never revive a proposal. Native Go + controlled HTTP provider:
  current/delegated correction, failure/retry, cancellation, all four sizes with
  a separate multiline/file/plan draft, fresh review, actual execution and restart.
  Related terminal regressions: **12 pass, 237 assertions**; host sessions: **18
  pass**. Logs: `/tmp/gator-ux-20261110-plan-edit-terminal-regression.log`,
  `/tmp/gator-ux-20261110-plan-edit-admission-{go,host}.log`; TUI typecheck passes.
  These are controlled-provider/native-renderer journeys, not physical terminal
  or live-account/model verification. This completes pending-review text edits;
  initial-preparation/plan-bound-execution and pending exact-action edits still
  remain under item 1. It does not complete the fifteen-item goal.

- Draft-restoration gap observed while verifying corrections: text and file
  references survive restart, but the next-message plan-first flag is kept only
  in `GatorClient.planFirst`, outside the retained draft format. Preserve that
  review intent across restart and newer-draft/admission races under item 10.

- Pending-plan correction boundary (`e04c74883`, 10 November): the canonical plan store now
  admits a tool-free edit against an exact task/revision/digest and immutable
  requesting dialogue. Admission revokes the old review; success requires a new
  human review, while failure/cancellation retains the original task text,
  options, sources and budgets. The original conversation retains the actual
  result, including after owner restart. Read-only conversation references do
  not acquire the new correction capability. Full Go package checks pass:
  `go test ./internal/worksession ./internal/workapp ./internal/workrun ./cmd/gator`;
  log `/tmp/gator-ux-20261110-plan-edit-go-full.log`. Native owner/host wiring and
  its capacity check are being finished separately; this backend boundary alone
  does not complete item 1 or the full audit.

- Fresh full terminal regression after `3ba57d307` (10 November): default
  `bun run test` passes **94 tests, 3 unchanged skips, 0 failures**, 1,550
  assertions across 62 files in 249.63 seconds. The command deadline remains
  unchanged; the former preview/context and field-subscription failures pass.
  Runtime rebuilt from this checkout; final TUI typecheck passes. Log:
  `/tmp/gator-ux-20261110-terminal-full-verified.log`. No source/test changes
  were made during this fresh run. Existing skipped image-paste, old dictation
  and managed-browser journeys remain skips; focused native voice/browser/file
  evidence elsewhere is not relabeled as physical verification. All fifteen
  findings remain in the ledger: this passing regression is not completion of
  the unresolved acceptance criteria or the final combined journey.

- Context-specific overlay reset (10 November): the first broad terminal run
  after field/shortcut sharing found that an output preview could remain open
  after a chat change. That reset had depended on reading an entire snapshot
  for invalidation. It now observes an explicit memoized conversation context;
  repeated work ticks do not reset dialogs, while an actual context change
  closes the old preview and keeps the original chat's draft. Work details,
  shared data, exact approvals, navigation and rail checks: 7 pass, 110
  assertions; typecheck passes. The broad run recorded 92 pass / 3 existing
  skips / 2 fail (97 tests): the preview regression and the new context assertion
  loaded against the already-running process's cached pre-fix provider. It is
  not a clean final regression result. Both cases pass in the fresh focused run:
  `/tmp/gator-ux-20261110-context-reset-{native,typecheck}.log`. A fresh complete
  run of the committed code follows; no checks were skipped or weakened.

- Shared shortcut projection (10 November): the six always-mounted keyboard-hint
  consumers and palette each formatted the complete registered command map on
  registry changes. The renderer now shares one immutable projection, subscribes
  after initial layer registration, and formats common options once per map.
  The unchanged command-completion test passes in 3,141 ms with its original
  5,000 ms deadline; pre-queue baseline in an isolated checkout passed in
  4,521 ms, while the new queue build previously exceeded 5,000 ms. These are
  individual diagnostic timings, not a calibrated long-history benchmark.
  Combined commands, composer, exact approvals, all four queue sizes, and legacy/
  enhanced Ctrl remaps: 11 pass, 188 assertions in 11.54 seconds. Typecheck passes.
  Logs: `/tmp/gator-ux-20261110-shared-shortcuts-{verified,typecheck}.log`.
  Earlier command failures remain recorded; their relevant rerun now passes.
  A full terminal regression run is still required after this correction.

- Explicit keyboard queue (`6dae42b1c`, 10 November): Tab submits the current owner's draft
  through the established durable queue, with its exact files, update reply and
  plan mode. Enter continues the ordinary conversation/correction router; Ctrl+G
  remains direct guidance. No task picker or automatic chat switch substitutes
  for conversational routing. Idle drafts, slash completion and overlays do not
  queue via Tab. `queue_message` is persisted/inspected/changed through the Go
  shortcut authority; only this contextual action accepts bare Tab. Native fixture
  checks pass at 110×34, 80×24, 60×20, 42×14, plus Ctrl+K and enhanced-protocol
  Ctrl+Shift+K: six passing cases. Ctrl+Shift is not distinguishable in the legacy
  test terminal; the test now declares enhanced protocol explicitly, rather than
  claiming legacy support. Explanations of such terminal limitations remain in
  item 11. Real Go + controlled provider queue/guide/chat-switch/restart journey:
  1 pass, 24 assertions; a queued private file reaches only its intended owner.
  Go config/controller checks and TUI typecheck pass. Logs:
  `/tmp/gator-ux-20261110-tab-queue-{controls-final,concurrent,config,settings-go,verified-typecheck}.log`.
  The broader controls run initially failed the existing default 5-second command
  completion test (also reproduced alone); the shared shortcut projection above
  resolves the relevant rerun. Full regression remains required. No deadline
  has been increased and no check removed.

- Conversational plan cancellation (`acc45f9e4`, 10 November; the changes were
  included in the user's local commit while verification was underway): preparing, review and approved
  undispatched plans are projected by their actual native owner. Ordinary stop
  messages use the existing canonical plan cancellation and revision/digest CAS;
  they cannot steer a reviewed plan, cancel a replacement revision, or stop an
  unrelated owner. Host-only control epochs also bind ordinary guidance to the
  operation seen during routing. The owner confirms a cancelled-plan receipt
  before the conversation says no implementation started; cancellation survives
  restart without reviving work. Full Go workapp/workrun/worksession checks pass;
  host session checks: 16 pass; native conversation/plan journeys: 2 pass and 82
  assertions, including all four composer sizes and restart. These use a real Go
  runtime and controlled HTTP model; no physical account/model validation is
  claimed. Logs: `/tmp/gator-ux-20261110-plan-stop-{final-go,host}.log` and
  `/tmp/gator-ux-20261110-field-subscriptions-native.log`. Conversational plan
  correction, pending exact-action changes, and the remaining item 1 journeys
  are still open; cancellation is not evidence that those are complete.

- Native snapshot subscription correction (`1759f95d1`, 10 November): repeated full backend
  snapshots could leave a native frame rendering while the backend continued
  responding; the ordinary conversation journey stalled and eventually lost its
  test connection. A single renderer-owned data provider now selects immutable
  top-level fields and retains unchanged values. It does not mutate a previously
  displayed approval when a new pending action arrives. Native field/approval/
  draft/composer regression: 5 pass, 108 assertions; real Go + controlled-provider
  conversation and pending-plan journeys: 2 pass, 82 assertions, 17.52 seconds.
  Earlier stalled runs and inspector-induced Bun crash are diagnostic failures,
  not passing evidence; no timeout, runner, or product fallback was substituted.
  Logs: `/tmp/gator-ux-20261110-field-subscriptions-{native,regression}.log`.
  Long-history benchmarking and the final full journey still remain.

- Start: working tree clean; HEAD `7750df414`; no repository/ancestor AGENTS.md found.
- Audit reproductions: root command filter excludes backend entries; composer metadata
  has no shared height budget; idle Ctrl+C calls quit; live approval uses generic dialog copy.
- Final journey to exercise: first task → ongoing commitment → correction during work →
  exact approval → return/catch-up → edit output. Pending implementation.

- Composer/keyboard slice: bounded metadata with Ctrl+T details, compact slash hints,
  idle Ctrl+C preservation, keymap-derived primary help/rail/composer hints.
  `bun run check` passed; composer-budget, commands, cancellation, navigation-polish: 8 pass.
  Tests use native renderer fixtures; cancellation crosses the real Go runtime with a controlled provider.

- Approval slice: show the indexed approval snapshot in the decision surface, keep
  long content scrollable, and disable decisions when identity/connection changes.
  Missing previews cannot approve. Ctrl+R toggles exact data (Ctrl+I aliases Tab
  in traditional terminals). Two approval tests and TUI typecheck pass.
  Resume audit found no live verification processes; resumed existing edits.

- Command discovery slice: root search includes backend names/descriptions and aliases,
  including decoded multiword names. Permissions use labeled choices; appearance,
  connections and checks reuse existing controls; relative paths validate before submission.
  `bun run check` passed; commands/composer-budget/navigation-polish: 8 pass.
  Advanced command forms are explicitly unfinished; no completion claim for item 9.

- Recovery slice: inline Ctrl+R recovery, explicit reconnect without replay, retained
  request/receipt inspection, and acknowledgement retry only for the unchanged
  draft with its original admission identity. Narrow disconnected composer verified.
  `interaction-recovery.test.tsx` passes against real Go + controlled provider;
  receipts tests pass (read-only checks and crashed approved dispatch).
  `client.test.ts` and composer-budget cover identity reuse and four-size layout.

- Voice slice: default Ctrl+V toggles explicit dictation, Ctrl+C cancels without
  leaving the app. Session consent binds to chat + verified provider route and
  can be revoked. Changed routes cannot begin capture using an earlier review.
  Confirmation bodies scroll at narrow sizes; processing destination remains visible.
  Voice controls/composer/navigation: 7 pass; TUI typecheck and config tests pass;
  native host tests: 9 pass. Controlled native capture is separate from physical
  microphone/speaker and real-provider verification; Linux audio integration remains platform-gated.

- Broad regression pass: host suite 110 pass / 1 platform skip. Native TUI suite
  77 pass / 3 skips / 1 failure: exact `Files` category search was outranked by
  a command with both title/category matches. Exact visible labels now win.
  Failed revision test plus commands/panel checks: 7 pass after the fix; typecheck passes.
  Go config/workrun tests pass; runtime rebuilt. Full final suite remains due
  after the remaining product slices; no claim that all 15 are complete.

- Transcript performance slice: the old timeline recreated every wrapper on every
  complete state snapshot. Content reconciliation now preserves unchanged native
  message views (including during fresh JSON snapshots) and their selection/scroll state.
  Transcript/update/activity/working-status checks: 5 pass; additional native identity
  test passes; typecheck passes. Benchmark samples show 5,000-message startup
  14,075→5,761 ms, append 4,051→747 ms. This remains insufficient for a smooth
  long history: retain item 13 as in progress and implement bounded rendering.
  Native syntax-style cleanup warnings surfaced during view teardown; inspect
  resource lifetime under item 15 rather than treating plain-text fallback as resolved.

- Conversation/execution foundation: `workrun.RouteConversation` answers or returns
  guide/stop/new-task/clarification intent with no tools or approval authority.
  Host-provided task IDs and capabilities are validated; no premature action acknowledgement.
  `worksession` dialogue turns retain original input/options independently of execution
  leases/revisions. Pages retain older turns, retries are idempotent, updates use
  compare-and-swap, and terminal dispatches cannot be revived by reading or updating.
  `go test ./internal/worksession ./internal/workrun` passes; dialogue race tests pass.
  This foundation is deliberately not advertised as a working composer flow yet.
  Remaining integration for item 1: durable asynchronous turn admission, configured
  routing model/accounting, task delegation through the existing host session owners,
  accepted guidance acknowledgements, parent/child result routing with original scope,
  restart/pruning retention, and real protocol/UI tests. Do not substitute auto-switching
  to another chat or a queue/guide picker as the completed conversational experience.

- Following commitment slice (builds on `444ed2e87`): native cards combine goal
  outcome/status with associated check health, next action/time, last successful
  check, selected sources and timezone. Standalone checks appear once; goals with
  no check explicitly say so. Service errors remain visible above the scrolling
  cards, and stale service observations are not reported as verified online.
  Readable local-service/sleep/foreground limits are available from the same page.
  Pause/resume binds the exact saved agreement under the jobs-store lock;
  changed agreements and expired resumes are rejected. A saved change followed
  by a failed health refresh explicitly reports that partial outcome.
  `go test ./internal/workapp ./internal/jobs` passes. Host policy: 17 pass.
  TUI typecheck passes; Following/hubs/work-details/pages/followup-chat/navigation:
  10 pass. Four-size native fixtures cover selection, controls and cancellation.
  `following-integration.test.tsx` passes against real Go: reviewed creation,
  inspection, pause/resume persistence, stale digest rejection, reviewed edit,
  management while another owner runs, draft retention and restart without replay.
  The model is controlled for the busy-owner check; this is not live-model quality
  verification or an always-on service test. At this slice, advanced schedules expose a
  disabled edit choice and require typed management (items 2/9); unscheduled goal
  actions open the goals list rather than the exact selected goal (item 2/10).
  NativeSyntaxStyle teardown warnings reproduced in followup-chat; item 15 remains
  open and the warnings are not treated as a successful highlighting fallback.

- Markdown resource lifetime: Solid may defer native node removal until a render
  pass. A component cleanup previously destroyed its syntax style immediately,
  leaving pending highlights with live code blocks borrowing a freed resource.
  Markdown now owns a native subtree for each style and explicitly destroys its
  consumers before releasing that style. `markdown-lifetime.test.tsx` gates the
  native parser across navigation and five theme changes, verifies old consumers
  and styles are destroyed and replacement styles remain usable, and rejects any
  fallback warnings. Typecheck and markdown-lifetime/followup-chat/transcript pass
  (3 tests); the recurring-check journey no longer emits NativeSyntaxStyle warnings.
  Host regression suite: 111 pass, one platform-specific skip. Remaining item 15
  work includes the shared typography, focus, disabled and empty/error treatments
  across surfaces; resource cleanup alone does not complete the visual audit.
  Full native TUI regression: 80 pass, three platform/opt-in skips, two timeouts
  (interrupted-request recovery at 45s and recurring-check chat at 60s). No
  NativeSyntaxStyle warnings occurred in that run. Both timed-out files pass
  together in a focused run (8.35s and 7.86s); the broad-run timeout cause remains
  unresolved, so this is not recorded as a green full-suite result. Do not raise
  deadlines or skip these journeys to claim completion.

- Following terminal verification: the built checkout launcher and real Go
  runtime passed a PTY check with an isolated, seeded goal/check profile at
  110×34, 80×24, 60×20 and 42×14. With `NO_COLOR=1` and reduced motion, keyboard
  review/cancellation left the agreement unchanged at every size; confirmed
  pause/resume persisted exact state, offline resume disclosed the service
  requirement, and quit retained an unsent draft and restored terminal modes.
  No model request or scheduled check was executed. The two broad-run timeout
  journeys also pass three consecutive focused repetitions each (six tests);
  a full run with per-file global isolation is being investigated, without
  changing timeouts or dropping checks. The broad-run timeout cause is still open.

- Per-file-isolation verification at `12176a315`: the complete native TUI suite
  passed with `bun test --isolate --preload ./scripts/preload.ts tests`:
  82 pass, three unchanged platform/opt-in skips, no failures and no syntax-style
  warnings. All 85 tests and their existing deadlines/assertions were retained.
  The default shared-global run's two timeouts remain recorded above; this result
  suggests fixture lifetime/state interference but does not establish its cause.

- Host navigation race: a busy owner's managed view previously returned after
  focus, before its Go navigation frame had been applied. Dependent actions could
  inspect an empty/startup panel and fail even though the requested page appeared
  afterward. The session host now waits for `action_applied` before resolving
  opening or clearing a view. A failed new view is retired, restoring the original
  owner's focus without moving its task, queue or approval. Delayed/failing view
  acknowledgement tests and the real-Go busy-owner/goal navigation journey pass.
  Full host regression: 113 pass, one platform skip. Unisolated pages/recovery/
  followup-chat rerun: three pass, with unchanged test deadlines.
  General back/filter/selection restoration remains unfinished (item 10).

- Exact goal-card navigation and setup: goal rows now carry stable identities in
  the existing panel digest, so reordering identical titles/source labels makes
  stale selections invalid. Cards select the canonical current row by goal ID;
  they do not search by title or start work. Check setup is a typed request that
  reads the selected active goal from Go and opens the existing reviewed form.
  Opening it neither saves a check nor retargets the current chat/workspace.
  Foreign-chat/managed-view setup uses the goal's actual workspace and update
  origin, with save-time goal/source validation retained. Missing/paused goals,
  failed goal reads and unavailable setup acknowledge errors without stuck busy
  state. Expired checks show why resume is unavailable. Go workapp/jobs tests,
  host frame validation and TUI typecheck pass. Native goal/concurrent/Following/
  client integration: 11 pass; duplicate-title navigation and foreign-workspace
  check creation use real Go, and busy navigation preserves the original task and
  draft. Model execution is controlled in the concurrency case. Advanced schedule
  editing and paused-goal management remain open; this is not item 2 completion.

- Default native regression after `5f81ac8de`/`a95289265`: ordinary `bun run test`
  passes with 83 tests passed, three unchanged platform/opt-in skips and no
  syntax-style warnings (86 tests, 58 files, 271.76s). No test runner, assertion,
  deadline or skip was changed. The earlier failing shared-global run remains
  historical evidence; the host navigation acknowledgement race is now fixed,
  and the default run is green at this point. Final regression remains due after
  the other product slices.

- Conversation task ownership: a ready task decision can now bind one independent
  empty child conversation without taking the parent's execution lease or changing
  the active chat. The original turn/options/source are bound through a validated
  child sidecar; only the winning ready→dispatching transition claims a launch.
  Concurrent retries return the same owner without another claim. Retained
  decisions and task owners are immutable, and missing/deleted/forged/linked
  origins fail rather than reviving work. Go worksession/workrun checks pass;
  dialogue tests also pass under the race detector. These APIs do not yet route
  ordinary composer messages or launch background work.
  Startup upload retention now reads dialogue-only and completed turn references
  as well as pending Work requests. Invalid or linked dialogue storage defers
  cleanup. Host uploads tests: seven pass, including actual age-based pruning.

- Item 1 integration still required: capture an explicit source/goal/options/public
  context envelope; persist admission before acknowledgement; route independently
  of Work and revalidate a live RunID before guide/stop; launch only a newly claimed
  child through the existing host owners without changing focus or last-active
  navigation. A child needs its own durable Work admission, execution lease and
  exact approval owner. Do not pass it through the parent's goal-chat Run closure:
  that closure is bound to the canonical goal conversation. Verify task scope and
  input retention through admission, pruning, restart and result projection;
  return statuses/results with validated parent-child links instead of forging
  artifact membership. Unconfirmed dispatches must not automatically replay.
  Keep this item open until real protocol/native journeys exercise that behavior.

- After the task-origin/retention slice `49e27ef06`, the full host suite passes:
  115 pass, one platform skip. Real-Go contextual recovery and Following journeys
  also pass (two tests), with the new startup retention reader in use. Task binding
  itself remains core/race-tested API work; neither those journeys nor the green
  host suite prove ordinary-message routing or background delegation is wired.

- Conversation owner integration in progress: ordinary upload/composer admission
  can now persist and route a separate conversational turn when its native host
  enables the lane. Replies do not acquire the Work lease; guide/stop decisions
  revalidate their live targets. Independent task bootstrap reloads the immutable
  parent envelope and admits into the exact child's own Work journal. Duplicate
  bootstrap and restored turns do not replay. Configured native routing uses the
  captured model and existing usage accounting; delegated work validates source,
  goal, selected accounts, permission and processing configuration, while keeping
  its own execution/approval identity. Public parent context and update origins
  remain references, not replay or approval grants.
  Focused workapp/workrun/native-boundary Go tests pass. Host dispatch, durable
  result restoration, native transcript projection and integrated controlled-model
  journeys are still being implemented/verified; item 1 remains open. The other
  fourteen rows retain their remaining criteria and final-journey requirements.

- Attachment maintenance now holds the existing exclusive profile guard across
  the host's reference scan and actual prune, using a private pipe-owned lease;
  busy profiles defer cleanup. Composer admission holds the matching shared guard
  across validation and durable persistence. The guard's busy/deferred/retained/
  released states pass a real Go lock test. If the helper or its ownership is
  unavailable, cleanup keeps attachments. This reuses the existing retention
  reader and seven-day pruning policy; it does not add another retention store.

- Relative-profile follow-up: the native Unix profile resolves relative to the
  selected workspace, while retained host upload/draft locations preserve their
  earlier launch-relative namespace. Cleanup now locks/scans that actual native
  profile explicitly and keeps existing private inputs in their original location.
  Platform tests cover both relative overrides, absolute paths and Windows;
  the real-Go relative-profile reopen test passes. This also avoids creating a
  spurious launch-directory profile solely to obtain a maintenance lease.

- Native resource follow-up: identical theme names from fresh backend snapshots
  now retain the same palette and syntax resources. A native identity test checks
  ten updates without destroying/remounting its code consumer. Markdown keeps an
  owning parent for Solid's later removal while destroying style consumers first;
  the real-Go dialogue journey no longer emits duplicate-removal warnings. The
  existing delayed-highlighting/theme-change checks still pass. Composer buffer
  synchronization now ignores intermediate programmatic content events instead
  of persisting them as user edits. The native harness renders its initial layout
  before interacting; an unflushed initial layout was not a usable terminal frame.
  Native composer/resource checks: four pass; the existing long-transcript test
  reached its unchanged five-second deadline during simultaneous Go race builds
  and is being rerun independently. No deadline or assertion was weakened.

- Host/native conversation integration: the host consumes only newly claimed
  native dispatches, starts a separately admitted owner without switching focus,
  and returns original-task controls/results through validated durable links.
  Failed/unconfirmed handoffs do not replay. Confirmed summaries restore from the
  original request's revision, rather than borrowing another task's latest answer.
  `/tasks` opens original reviews/outputs and returns to the requesting conversation;
  `/earlier-messages` pages retained dialogue and `/files` recognizes its original
  selected inputs. `/queue-message` explicitly retains the existing queue workflow;
  ordinary Enter submissions remain conversational. Queue/recovery tests now
  explicitly request queue placement and keep their immutable-file/identity checks.
  Host regression: 117 pass, one unchanged platform skip. Native real-Go controlled
  provider journeys: five pass (conversation, concurrent owners, interrupted queue,
  lost admission/reconnect, and syntax lifetime). The conversation journey holds
  the primary provider call, answers a greeting, accepts its budget correction,
  runs another task with a real private input, clarifies two possible targets,
  stops a separately named task without stopping the primary, opens/returns from
  the original child, restores summaries/drafts on restart, and verifies no replay.
  Its final draft is rendered at all four sizes; this is not a repeated full live
  approval journey at each size or live-model/account-quality verification.
  Native warnings are asserted absent through disposal. Go workapp dialogue tests
  also pass under the race detector. README remains unchanged; nothing was pushed.
  Remaining item 1 work includes pending-plan/review steering/cancellation, exact
  delegated approval review/edit/rejection, router/admission failure and capacity
  recovery, selected-account/goal changes, and older-turn navigation/search.
  The full fifteen-item integrated acceptance journey remains incomplete.

- Broad native regression at `485cd007c`: 84 pass, three unchanged skips, one
  failure (the new conversation journey exceeded its original 60-second deadline;
  496.41 seconds for the full run). It passes in focused runs, but that did not
  establish a green default suite. No test runner, deadline, assertion or skip was
  changed. The existing long-transcript test independently passes in 4.09 seconds
  with its original five-second deadline after its earlier concurrent-build timeout.

- Composer layout follow-up: restored/editor/voice draft replacements now wait
  for a computed native frame, request that frame explicitly, coalesce to the
  latest text and cancel pending callbacks at unmount. They do not reenter
  native/Solid layout mutations or treat intermediate buffer replacement events
  as user edits. A native test admits no work and checks an initial pre-layout
  draft plus ten running snapshots/replacements at all four sizes, including the
  actual editor buffer and persisted draft. It passes alongside the real-Go
  conversation journey and composer/keyboard checks (four tests). Full workapp
  checks pass; existing native app-control, check ownership, update-reference,
  task-plan and maintenance Go boundary tests pass. Typecheck passes after adding
  the host's authoritative-profile location to its existing type declaration.
  The default full TUI suite must be rerun after this layout fix; the earlier
  failure remains evidence and is not counted as resolved by focused tests alone.

- Default native regression after `d4b06d8d0`/`b1660ecd1`: ordinary `bun run test`
  passes, with 86 tests passed, three unchanged platform/opt-in skips and no
  failures (89 tests, 60 files, 310.02 seconds). The previously timing-out real-Go
  conversation journey passes inside that shared-global run in 10.20 seconds;
  native pre-layout draft synchronization and relative-profile reopening also
  pass. No runner, deadline, assertion or skip was changed. Relevant Go app
  controls, follow-up ownership, update-reply context, task-plan and maintenance
  boundary checks pass, and the complete workapp package passes after the changes.
  The full fifteen-item acceptance journey is still pending.
  Next conversation checks include pending plans/exact approvals and continuity
  after completed work: returned result cards are implemented, but the router's
  bounded context does not yet include those task summaries, and an idle parent
  currently returns to direct Work admission after its last owner finishes.
  Verify/fix that boundary before claiming a continuous conversational experience.

- Completed-task continuity: a conversation with retained dialogue now keeps its
  conversational admission path after owners finish. The router receives bounded,
  original-task summaries, with active owners prioritized and terminal results
  carrying no guide/stop capability. New work captures those public references in
  its immutable envelope; planning and execution share the same read-only context
  projection, without replay, tool results or inherited approval/control authority.
  Transient result text now also carries its internal admission identity, preventing
  a later unrelated local message from replacing the original task's summary.
  Core tests check bounds, identity, no inherited controls, idle follow-up admission
  and no replay. A controlled native planning integration verifies the reference
  reaches the configured model without executable tools or self-granted approval.
  Full workapp/workrun/worksession checks pass. Expanded real-Go terminal journey
  verifies a result question after all work finishes without starting another Work,
  a new task with the original result as context and no silently inherited files,
  and another result question after restart. That journey passes in a six-test
  focused concurrency/recovery/plan regression, and passes again after the transient
  result-identity fix (53 assertions). TUI typecheck passes. These are controlled
  providers, not live-model/account or physical-phone evidence. The prior default
  89-test green run predates this slice; final broad regression remains required.

- Mobile follow-on research is recorded separately in `MOBILE-ACCESS-RESEARCH.md`
  (`543a74e81`/`e13f60292`). No mobile implementation, deployment, account setup or
  Telegram message was performed; it remains sequenced after all fifteen criteria.
