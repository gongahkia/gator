# Gator terminal experience completion checklist

This tracks the 15-item product/interaction goal from the audit of `7750df414`.
Completion requires behavior and verification, including the Go/host boundaries;
an available command alone is not evidence of an enjoyable end-to-end flow.

| # | Acceptance criteria | Status | Evidence / commit |
|---|---|---|---|
| 1 | Conversation remains available during execution; ordinary messages can steer a specific live task or start independent work; ambiguous routing is clarified without losing task identity, files, or drafts. | In progress | Router, independent paged turns, and durable child-task bindings implemented. Concurrent retries claim one dispatch; original decisions/owners cannot be changed; source/input provenance is checked; reads/retries do not launch work; pruning retains dialogue inputs. Core/race tests pass. Composer routing, background launch and result/control return remain unwired. |
| 2 | Following unifies commitments with outcome/state/next action, next and last successful checks, sources/timezone, pause/edit, and truthful foreground/service/offline limits. | In progress | `4b9edf091`: commitment cards and exact check controls. Active goal cards now open their exact canonical picker row and prepare a check for that goal's current workspace, including duplicate titles, other-chat origins and busy owners. Focused Go/host/native integration checks pass. Advanced non-project schedule editing and paused-goal controls remain; final integrated journey pending. |
| 3 | Relevant answers expose supplied memory/provenance/correction; noise feedback offers a reviewed, scoped check/notification change and reports its actual outcome. | Pending | |
| 4 | First task leads through only missing setup; host connectivity and model readiness are distinct; local/cloud processing, capabilities, and work budget are explained. | Pending | |
| 5 | Workspace/account/source scope is inspectable near input; conversational/manual settings agree, including terminal preferences, with confirmed outcomes. | Pending | |
| 6 | Results/files/updates expose keyboard contextual actions to open/edit/reply/inspect evidence/mark done while retaining original identities and conversation context. | Pending | |
| 7 | Input and useful transcript remain visible at 110×34, 80×24, 60×20, 42×14 with combined attachments/reply/plan/slash states and multiline drafts. | Implemented; final journey pending | `204386564`; `composer-budget.test.tsx`: 8 file names + plan/reply + multiline input and slash completion, ≥4 transcript rows at all four sizes. |
| 8 | Approval surface shows exact live action/account/destination/content/consequences; long reviews scroll; changed/stale tokens cannot execute; edits require fresh review. | Implemented; final journey pending | `8ba3a9cb8`; `approval-review.test.tsx`: four sizes, scrolling/exact data, stale and missing previews; `approvals.test.tsx`: real Go token protection and controlled connected send. |
| 9 | Root command search includes local/backend descriptions/names/aliases, grouped browsing remains usable, and commands requiring values have meaningful typed controls. | In progress | `5f54f7f17`; root search and common permission/theme/source/attachment controls verified in `commands.test.tsx`. Remaining advanced command families still expose explicit advanced syntax; typed controls remain required. |
| 10 | Back restores menu/filter/selection, draft and reading position consistently; backend pages and file workflows retain context; rail marks the active page. | In progress | Host-managed navigation now waits for the Go view acknowledgement; a failed new view is retired and original work/focus is restored. Delayed/failing acknowledgement unit tests pass. Menu/filter/selection/back restoration across all flows remains open; active-page rail tests already pass. |
| 11 | Ctrl+C cancels/stops without implicit quit; Ctrl+Q quits through guards; help/hints reflect configured bindings; no function-key shortcuts are introduced. | In progress | Idle stop/draft preservation, explicit quit, remapped help/composer/rail verified; voice/approval/contextual hints remain for later slices. |
| 12 | Disconnected/error states offer contextual reconnect/recovery with intact drafts/request identity; safe admission retries and uncertain external effects stay distinct. | Implemented; final journey pending | `d00952219`; `interaction-recovery.test.tsx`: real Go accepted request + lost acknowledgement retries once, reconnect keeps draft/owner, no model replay; receipts tests preserve unknown external outcomes. |
| 13 | New-content/latest/catch-up affordances preserve stable reading anchors across navigation/restart; long-history rendering is benchmarked and optimized when justified. | In progress | Reproducible native benchmark and reports in `docs/benchmarks/`; unchanged entries retain DOM identity across complete Go snapshots. 5,000-message append improved 4,051→747 ms in single samples. Startup/scroll remain too slow; bounded rendering, persistent anchors, catch-up/latest UI still required. |
| 14 | Voice has direct record/stop/cancel/transcribe/edit flow, visible processing destination and explicit consent; transcript is editable before sending. | Implemented; final journey pending | `09e74ef39`; `voice-controls.test.tsx`: all four sizes, native VoiceSession with controlled capture/transcription, route/chat consent, changed-route rejection, editable draft/no send, recording/transcription cancellation. Physical audio remains unverified on this host. |
| 15 | Shared native typography/spacing/selection/focus/disabled treatments and readable summaries work with terminal colors, NO_COLOR and reduced motion; exact technical details remain inspectable. | In progress | `20e9b7cde`: Markdown cleanup destroys native consumers before their borrowed syntax style, including theme changes. Delayed-highlighting regression and previously warning-producing real-Go followup journey pass without warnings. Shared visual/state audit and full acceptance journey remain. |

## Implementation rules

- Reuse Solid/OpenTUI styling and Go/host safeguards; keep CLI/TUI only, optional rail, no mascot or Electron.
- Preserve data, permissions, immutable evidence, recovery, drafts, and unrelated work. README changes require an essential correction.
- Make atomic local commits with relevant checks; do not push or publish.
- Distinguish native fixture rendering, protocol integration with controlled providers,
  physical terminal interaction, and live model/account verification.
- Track blockers precisely; continue independent items. Do not mark the goal complete with missing criteria.

## Verification journal

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
