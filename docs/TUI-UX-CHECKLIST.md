# Gator terminal experience completion checklist

This tracks the 15-item product/interaction goal from the audit of `7750df414`.
Completion requires behavior and verification, including the Go/host boundaries;
an available command alone is not evidence of an enjoyable end-to-end flow.

| # | Acceptance criteria | Status | Evidence / commit |
|---|---|---|---|
| 1 | Conversation remains available during execution; ordinary messages can steer a specific live task or start independent work; ambiguous routing is clarified without losing task identity, files, or drafts. | In progress | Tool-free conversation router and independent paged turn store implemented; identity/capability validation, active-operation independence, idempotent admission and versioned dispatch tests pass. Not yet wired to ordinary composer submissions or background task delegation. |
| 2 | Following unifies commitments with outcome/state/next action, next and last successful checks, sources/timezone, pause/edit, and truthful foreground/service/offline limits. | Pending | |
| 3 | Relevant answers expose supplied memory/provenance/correction; noise feedback offers a reviewed, scoped check/notification change and reports its actual outcome. | Pending | |
| 4 | First task leads through only missing setup; host connectivity and model readiness are distinct; local/cloud processing, capabilities, and work budget are explained. | Pending | |
| 5 | Workspace/account/source scope is inspectable near input; conversational/manual settings agree, including terminal preferences, with confirmed outcomes. | Pending | |
| 6 | Results/files/updates expose keyboard contextual actions to open/edit/reply/inspect evidence/mark done while retaining original identities and conversation context. | Pending | |
| 7 | Input and useful transcript remain visible at 110×34, 80×24, 60×20, 42×14 with combined attachments/reply/plan/slash states and multiline drafts. | Implemented; final journey pending | `204386564`; `composer-budget.test.tsx`: 8 file names + plan/reply + multiline input and slash completion, ≥4 transcript rows at all four sizes. |
| 8 | Approval surface shows exact live action/account/destination/content/consequences; long reviews scroll; changed/stale tokens cannot execute; edits require fresh review. | Implemented; final journey pending | `8ba3a9cb8`; `approval-review.test.tsx`: four sizes, scrolling/exact data, stale and missing previews; `approvals.test.tsx`: real Go token protection and controlled connected send. |
| 9 | Root command search includes local/backend descriptions/names/aliases, grouped browsing remains usable, and commands requiring values have meaningful typed controls. | In progress | `5f54f7f17`; root search and common permission/theme/source/attachment controls verified in `commands.test.tsx`. Remaining advanced command families still expose explicit advanced syntax; typed controls remain required. |
| 10 | Back restores menu/filter/selection, draft and reading position consistently; backend pages and file workflows retain context; rail marks the active page. | Pending | |
| 11 | Ctrl+C cancels/stops without implicit quit; Ctrl+Q quits through guards; help/hints reflect configured bindings; no function-key shortcuts are introduced. | In progress | Idle stop/draft preservation, explicit quit, remapped help/composer/rail verified; voice/approval/contextual hints remain for later slices. |
| 12 | Disconnected/error states offer contextual reconnect/recovery with intact drafts/request identity; safe admission retries and uncertain external effects stay distinct. | Implemented; final journey pending | `d00952219`; `interaction-recovery.test.tsx`: real Go accepted request + lost acknowledgement retries once, reconnect keeps draft/owner, no model replay; receipts tests preserve unknown external outcomes. |
| 13 | New-content/latest/catch-up affordances preserve stable reading anchors across navigation/restart; long-history rendering is benchmarked and optimized when justified. | In progress | Reproducible native benchmark and reports in `docs/benchmarks/`; unchanged entries retain DOM identity across complete Go snapshots. 5,000-message append improved 4,051→747 ms in single samples. Startup/scroll remain too slow; bounded rendering, persistent anchors, catch-up/latest UI still required. |
| 14 | Voice has direct record/stop/cancel/transcribe/edit flow, visible processing destination and explicit consent; transcript is editable before sending. | Implemented; final journey pending | `09e74ef39`; `voice-controls.test.tsx`: all four sizes, native VoiceSession with controlled capture/transcription, route/chat consent, changed-route rejection, editable draft/no send, recording/transcription cancellation. Physical audio remains unverified on this host. |
| 15 | Shared native typography/spacing/selection/focus/disabled treatments and readable summaries work with terminal colors, NO_COLOR and reduced motion; exact technical details remain inspectable. | Pending | |

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
