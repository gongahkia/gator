# Gator terminal experience completion checklist

This tracks the 15-item product/interaction goal from the audit of `7750df414`.
Completion requires behavior and verification, including the Go/host boundaries;
an available command alone is not evidence of an enjoyable end-to-end flow.

| # | Acceptance criteria | Status | Evidence / commit |
|---|---|---|---|
| 1 | Conversation remains available during execution; ordinary messages can steer a specific live task or start independent work; ambiguous routing is clarified without losing task identity, files, or drafts. | In progress | `92cd471f4`/`485cd007c` plus result continuity: ordinary messages reply, steer, clarify, delegate and stop specific independent owners without moving focus. Pending-review text corrections now revise the same task asynchronously, preserve its frozen inputs/limits, and require fresh approval; failures/cancellation and restart retain the outcome. Preparing/reviewed plans can be cancelled conversationally; changed control epochs are rejected. Recent original-task summaries reach follow-up conversation/work/planning without acquiring controls. Explicit queues remain available. Corrections during initial preparation or plan-bound execution, changes to pending exact actions/new inputs, delegated exact-approval journeys, broader failure/account/goal journeys and live-model routing quality remain open. |
| 2 | Following unifies commitments with outcome/state/next action, next and last successful checks, sources/timezone, pause/edit, and truthful foreground/service/offline limits. | In progress | `4b9edf091`/`a95289265`: commitment cards and exact check controls. Active goal cards open their exact canonical picker row and prepare a check for that goal's current workspace, including duplicate titles, other-chat origins and busy owners. Focused Go/host/native integration checks pass. Advanced non-project schedule editing and paused-goal controls remain; final integrated journey pending. |
| 3 | Relevant answers expose supplied memory/provenance/correction; noise feedback offers a reviewed, scoped check/notification change and reports its actual outcome. | In progress | `cb095fcc4`/`08c12a19d`/`bda11baf3`: relevant answers expose original context and reviewed noise changes use the authoritative check controller. Inspection now includes sealed knowledge-search previews, full reads and exact carried tool responses, with current saved state, revision-guarded correction/forgetting and native four-size restart coverage. Later corrections do not rewrite historical excerpts. Verified remaining gap: a continuation can receive the old tool response without the newly corrected fact; authoritative refresh/precedence for subsequent requests and the final integrated journey remain required. |
| 4 | First task leads through only missing setup; host connectivity and model readiness are distinct; local/cloud processing, capabilities, and work budget are explained. | In progress | `d0fd58e48` provides Go-owned prerequisite/processing/budget projection. Ordinary first submissions offer missing setup while preserving private files/text and plan intent; Host connectivity and model setup have separate labels. Native fresh-profile → invalid provider form → repair → persisted selection → inspect context/limits → explicit useful task passes at all four sizes. Missing and subsequently changed credentials recover without replaying/admitting the rejected task. Request-specific connected-account/access setup handoffs and recovery after provider rejection during an admitted task remain; final integrated journey and live-account/model verification remain. |
| 5 | Workspace/account/source scope is inspectable near input; conversational/manual settings agree, including terminal preferences, with confirmed outcomes. | In progress | Ctrl+T and /request-context inspect workspace/model processing, selected accounts, inputs, mode and actual Go budgets. Missing account display metadata retains the selected identity. Appearance, navigation and recent-chat grouping now share Go configuration across manual controls, conversational tools, cached owners, CLI theme changes and restart. Legacy frontend values import once; failures do not report saved values. Native four-size settings journeys plus unreadable-import recovery and live-task preservation pass. Full permission explanations, broader account/settings integration and live-model/account verification remain; final combined acceptance journey pending. |
| 6 | Results/files/updates expose keyboard contextual actions to open/edit/reply/inspect evidence/mark done while retaining original identities and conversation context. | In progress | Configurable Ctrl+E and /actions expose original-item result/file/update controls. Answers join deliverables only by exact run identity. Native four-size journeys inspect retained evidence, copy answers, edit/save working file versions without changing sealed originals, reject other-chat targets, mark done with a correlated Go outcome, restore the original owner and explicitly send an update-bound reply while retaining the newer draft/review intent. Delayed source/file reads now close without reopening on late success/failure; exact file-action Back and explicit read retry pass at all four sizes, including an unrelated executing Go task. Original-answer request receipts now share /receipts controls, including exact-account Sent checks, permission repair, cancelled-read completion and restart at all four sizes. Conversation replies and delegated summaries now expose exact-turn/task contextual inspection, copying, retained inputs and original-owner navigation; four-size real-Go journeys preserve the parent draft/review intent without new model work. Structured connected-item actions and the final combined journey remain. |
| 7 | Input and useful transcript remain visible at 110×34, 80×24, 60×20, 42×14 with combined attachments/reply/plan/slash states and multiline drafts. | Implemented; final journey pending | `204386564`; `composer-budget.test.tsx`: 8 file names + plan/reply + multiline input and slash completion, ≥4 transcript rows at all four sizes. |
| 8 | Approval surface shows exact live action/account/destination/content/consequences; long reviews scroll; changed/stale tokens cannot execute; edits require fresh review. | Implemented; final journey pending | `8ba3a9cb8`; `approval-review.test.tsx`: four sizes, scrolling/exact data, stale and missing previews; `approvals.test.tsx`: real Go token protection and controlled connected send. |
| 9 | Root command search includes local/backend descriptions/names/aliases, grouped browsing remains usable, and commands requiring values have meaningful typed controls. | In progress | `5f54f7f17`; root search and common permission/theme/source/attachment controls verified in `commands.test.tsx`. Remaining advanced command families still expose explicit advanced syntax; typed controls remain required. |
| 10 | Back restores menu/filter/selection, draft and reading position consistently; backend pages and file workflows retain context; rail marks the active page. | In progress | Host-managed navigation waits for the Go view acknowledgement; failed views retire and original work/focus is restored. Next-message plan-first intent now persists with text/files/reply context, including empty drafts, restart, commands and newer-draft admission races. Native restart → keyboard submission prepares review without starting work. Shared frontend menus and read-only inspections now restore parent query, selected choice and list position, including refreshed/reordered results at all four sizes. New commands/context owners start fresh navigation; successful check changes retire obsolete controls. Context list/detail/edit/review now retain local filter/reading positions through presentation-only IDs (`bda11baf3`); other typed-editor/file-editor and backend-page restoration across all flows, plus durable reading anchors, remain open; active-page rail tests already pass. |
| 11 | Ctrl+C cancels/stops without implicit quit; Ctrl+Q quits through guards; help/hints reflect configured bindings; no function-key shortcuts are introduced. | In progress | Idle stop/draft preservation, explicit quit, remapped help/composer/rail verified. Tab now explicitly queues a draft after the active task in its current owner; Enter remains conversational and routes clear corrections. Slash completion/forms retain Tab. The queue shortcut uses the same Go settings backend; four sizes, Ctrl remapping and real-Go queued attachments are checked. Saved-context actions use Ctrl+E/Ctrl+X/Ctrl+O, and context refresh is correctly labeled. Other voice/approval/contextual hints and terminal modifier limitations remain. |
| 12 | Disconnected/error states offer contextual reconnect/recovery with intact drafts/request identity; safe admission retries and uncertain external effects stay distinct. | Implemented; final journey pending | `d00952219`; `interaction-recovery.test.tsx`: real Go accepted request + lost acknowledgement retries once, reconnect keeps draft/owner, no model replay; receipts tests preserve unknown external outcomes. |
| 13 | New-content/latest/catch-up affordances preserve stable reading anchors across navigation/restart; long-history rendering is benchmarked and optimized when justified. | In progress | Reproducible native benchmark and reports in `docs/benchmarks/`; unchanged entries retain DOM identity across complete Go snapshots. 5,000-message append improved 4,051→747 ms in single samples. Startup/scroll remain too slow; bounded rendering, persistent anchors, catch-up/latest UI still required. |
| 14 | Voice has direct record/stop/cancel/transcribe/edit flow, visible processing destination and explicit consent; transcript is editable before sending. | Implemented; final journey pending | `09e74ef39`; `voice-controls.test.tsx`: all four sizes, native VoiceSession with controlled capture/transcription, route/chat consent, changed-route rejection, editable draft/no send, recording/transcription cancellation. Physical audio remains unverified on this host. |
| 15 | Shared native typography/spacing/selection/focus/disabled treatments and readable summaries work with terminal colors, NO_COLOR and reduced motion; exact technical details remain inspectable. | In progress | `20e9b7cde`: Markdown cleanup destroys native consumers before their borrowed syntax style, including theme changes. Shared immutable field subscriptions prevent routine snapshots from invalidating unchanged native content; the reproduced conversation render stall clears without changing deadlines or renderer. Exact review objects remain frozen. Detail views reserve their scrollbar column and page by viewport height, retaining readable processing/budget text at all four sizes. Shared visual/state audit and full acceptance journey remain. |

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
