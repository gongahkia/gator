# Codex Desktop presentation audit

21 October 2026. Baseline: `c5b74d931` (the checkpoint of the previously
completed conversation, motion, and queue work). Changes after that checkpoint
are limited to desktop presentation, frontend tests, and documentation.

## Reference and scope

Full review of Gator's desktop shell, home, conversation/composer/queue,
Conversations and task runs, overview, goals, inbox, follow-ups, saved knowledge,
model setup, usage, data, recovery, command search, forms, and confirmations.
React DOM, Tailwind v4, semantic CSS, and Lucide remain the existing stack.

The comparison uses official OpenAI documentation and its published Codex
screenshots, rather than claiming an installed Codex build was measured:

- [Codex app screenshot and changelog](https://learn.chatgpt.com/docs/changelog):
  restrained navigation, centered new-chat composer, separate settings, and
  conversation/review organization. Screenshots are historical examples, not a
  guarantee that every current build has identical pixels.
- [Projects and chats](https://learn.chatgpt.com/docs/projects): project and
  conversation navigation.
- [Desktop settings](https://learn.chatgpt.com/docs/app/settings): categorized
  settings and a documented 14px UI/code default.
- [Follow-up interaction](https://learn.chatgpt.com/docs/prompting): visible
  queued messages and distinct guidance/next-run behavior.
- [Code review](https://learn.chatgpt.com/docs/code-review): dedicated code
  review presentation.

Reference images were inspected in a temporary directory; OpenAI branding,
assets, private app code, and account controls are not copied into Gator.
Gator retains its alligator identity and personal-agent destinations.

## Difference map

| ID | Surface | Codex reference / Gator difference | Presentation adaptation | Status |
| --- | --- | --- | --- | --- |
| P1 | Typography and surfaces | Codex uses readable system typography; many Gator labels and controls are 10–12px | Shared text/radius tokens, larger text, restrained borders and consistent focus/hover | Implemented: typography commit |
| P2 | New conversation | Codex places the prompt beside the starting question; Gator pins it far below a dashboard | Center the starting composer; put the existing live overview and requested scrolling dashboard below it | Implemented: home commit |
| P3 | Navigation and settings | Codex has one Settings entry and a category rail; Gator spreads four settings links down the workspace sidebar | One Settings entry, categorized views for existing models, usage, data, and recovery; preserve all Go navigation/actions; Cmd/Ctrl+, opens it | Implemented: settings commit |
| P4 | Conversation | Gator repeats avatars, names, and a left gutter on every message | Right-aligned user bubbles and an unboxed assistant reading column; preserve meaningful result roles and accessible speaker labels | Implemented: conversation commit |
| P5 | Response/code actions | Codex provides usable copy controls; Gator has plain code blocks and no response copy | Language-labelled code headers and explicit response/code copy feedback, without adding IPC or model actions | Implemented: copy controls commit |
| P6 | Lists and forms | Gator stretches terminal-style rows across the whole window and duplicates row actions in its footer | Bounded content width, native page toolbars, single row actions, direct keyboard list navigation/page shortcuts with typing guards, and better empty states | Implemented: native panels commit |
| P7 | Diagnostic text and shortcuts | Metadata, key legends, and command-like instructions compete with primary page content | Disclose supporting diagnostics/shortcuts; keep forms, review details, warnings, and all underlying text accessible | Implemented: disclosure commit |
| P8 | Approvals and details | Most Gator results have identical document-card treatment | Clear semantic status icons, deliberate review hierarchy, and exact action details visible before approval | Implemented: result/review commit |
| P9 | Page header | Gator repeats the conversation title on unrelated settings/list pages | Show the active page and workspace/settings context; keep conversation tools on conversation surfaces | Implemented: page header commit |
| P10 | Detail tabs | Gator tab roles lacked linked panels and arrow navigation | Roving focus, Arrow/Home/End keys, and labelled tab/panel relationships | Implemented: detail tabs commit |
| K1 | Queue | Gator already has visible editable/reorderable queued cards | Retain Go acknowledgement, captured options, pause/resume, and draft protection; do not change scheduling | Retained |
| K2 | Motion | Gator already has interruptible transitions and reduced-motion support | Keep existing motion rules; no entrance replay during typing or provider updates | Retained |
| K3 | Usage | Gator accounts for local provider admission; Codex screenshots can show account/subscription quotas | Keep real local daily counters, uncapped states, UTC reset, and the separate review/confirm flow | Intentional difference |
| D1 | Projects, pins, recent chats in sidebar | Gator's current snapshot does not project a stable global thread/project tree or pin actions | Do not invent cached thread lists or decorative pin controls; requires a separate data contract/product decision | Deferred: backend data |
| D2 | Git review, file tree and terminal | Existing deliverable summary is not Codex's diff/worktree/terminal model | Keep Deliverables and Checks; no fake diff panes, commit controls, or arbitrary file/process actions | Deferred: backend capability |
| D3 | Attachments, voice, remote/cloud, plugins | Gator does not expose equivalent native desktop capabilities for all these controls | Keep real commands and terminal handoffs; do not add unsupported composer buttons | Deferred: capability |
| D4 | Theme editor / native translucency | Reference settings include custom colors/fonts and OS translucency | This pass adopts the type scale and neutral surfaces; custom/light/system themes need a presentation preference consistent with the shared Go theme, plus accessibility and native-material review | Deferred: separate UI scope |
| D5 | Profile, sharing, cloud/account quotas | Gator is a local profile; the user previously removed the profile tile | Retain the local identity and factual counters | Intentional difference |

## Review coverage

| Category | Evidence inspected | Result |
| --- | --- | --- |
| Typography | Source CSS, every destination, official app/settings screenshots | P1, P4, P5 |
| Surfaces | Home/composer, list/form/review pages, usage, native dialogs and settings | P1–P3, P6–P8 |
| Animations | Existing presence/icon/view hooks and packaged motion integration coverage | K2 retained; 10% playback, interruption, focus release, and reduced motion passed after layout changes |
| Icons | Existing Lucide set, result icons, duplicated actions and sidebar labels | P3, P5, P8 |
| Performance | React ownership, snapshot IDs, scroll containers and queue acknowledgement | Preserve Go ownership and bounded scrolling; no new renderer scheduler or animation dependency |

### Findings by principle

| Severity | Location | Before | After | Why |
| --- | --- | --- | --- | --- |
| MEDIUM | `desktop/src/styles.css` typography tokens | Tiny secondary text and inconsistent sizes | P1: readable shared type/spacing tokens | Typography should support scanning without zoom |
| MEDIUM | `desktop/src/App.tsx` home layout | A large gap separates the home question and composer | P2: composer below the starting question, live content below | Group the primary action with its context |
| MEDIUM | `desktop/src/App.tsx` sidebar | Four settings destinations and promotional sidebar text | P3: one Settings entry and category rail | Preserve navigation hierarchy and reduce competition |
| MEDIUM | `desktop/src/App.tsx` messages | Avatar/name gutters repeat through the conversation | P4: user bubbles and assistant reading column | Increase usable reading width and distinguish speakers through layout |
| MEDIUM | `desktop/src/Result.tsx` Markdown / `CopyButton.tsx` | No native response/code copy affordance | P5: explicit copy controls with success/error feedback | Make output usable without manual text selection |
| MEDIUM | `desktop/src/Panel.tsx` actions and rows | Duplicate footer actions, flat full-width rows and generic emptiness | P6: desktop toolbars, bounded forms/lists and keyboard rows | Match desktop interaction conventions while keeping exact panel actions |
| MEDIUM | `desktop/src/Panel.tsx` supporting blocks | Diagnostic prose/key legends dominate several pages | P7: supporting disclosures, preserved warnings and reviews | Use progressive disclosure without hiding decision-critical content |
| MEDIUM | `desktop/src/Result.tsx` result cards | One document-card hierarchy for most results | P8: semantic status/review treatment | Distinguish action reviews, warnings and ordinary output |
| MEDIUM | `desktop/src/App.tsx` main header | Unrelated conversation title and disabled workspace tools on settings | P9: active page title and contextual tools | Page identity should match the visible content |
| MEDIUM | `desktop/src/App.tsx` work detail tabs | Tab roles without their full keyboard contract | P10: linked panels and standard focus/navigation behavior | Desktop tab controls must work with the keyboard |

## Considered but rejected

| Location | Candidate | Rejected because |
| --- | --- | --- |
| Sidebar | Copy Codex's project/pin tree using remembered frontend labels | No stable authoritative tree/action contract exists in the current snapshot |
| Composer | Add microphone, attachment, cloud and plugin icons for visual parity | Controls must correspond to capabilities Gator actually exposes |
| Review pane | Reuse deliverable status as Git diff/commit status | These represent different records and permissions |
| Settings | Copy subscription periods, profile menu, and account balances | Gator's local profile and shared provider counters have different meanings |
| Whole app | Introduce another component or animation framework | The existing React/CSS system covers this presentation work |

## Verification and rollback

Each P-series change is committed independently. Git history and the final
verification record below provide the rollback points. Review/confirm flows,
exact panel IDs, secret fields, queue/steering semantics, and the existing
backend protocol remain in force.

### Atomic commit ledger

The previous work is preserved separately in checkpoint `c5b74d931`. Every
commit in the following range changes only desktop presentation/tests or docs;
the Go runtime, shared protocol, terminal UI, and Electron host are unchanged.

| Commit | Change |
| --- | --- |
| `283a19477` | Initial difference map and presentation-only scope |
| `f9bda0035` | P1: typography and surface tokens |
| `ba1a7b72d` | P2: centered home composer and live dashboard placement |
| `bfebdbee9` | P3: Settings entry and category navigation |
| `e99a7d277` | P4: conversation bubbles and assistant reading column |
| `b25c00276` | P5: response/code copy controls and feedback |
| `72c14560c` | P6: native panel toolbars, rows, forms, and empty states |
| `2fffd7909` | P7: diagnostic and keyboard disclosures |
| `2388d5ddd` | P8: result status and visible exact approval previews |
| `ebc789c50` | P9: active page context in the header |
| `5622cf0ae` | P10: linked detail tabs and keyboard navigation |
| `3c18225b9` | P6 follow-up: displayed page shortcuts with typing guards |
| `fe069f483` | Integration selectors and settled screen captures |

Use `git revert <commit>` for a selected change. Later changes share the
foundational tokens and panel presentation, so full rollback should run in
reverse chronological order. The bounded presentation range is
`c5b74d931..fe069f483`; `git revert --no-commit c5b74d931..fe069f483` stages its
combined reversal for review and a rollback commit. It excludes the previous
work checkpoint and this final verification documentation.

### Verification record

Verified locally on macOS on 21 October 2026:

- `npm run check --prefix desktop`: passed.
- `npm test --prefix desktop`: all six host tests passed.
- `npm run pack --prefix desktop`: built the UI/runtime and unsigned macOS x64
  bundle at `desktop/release/mac/Gator.app`.
- `GATOR_DESKTOP_TEST_EXECUTABLE="$PWD/desktop/release/mac/Gator.app/Contents/MacOS/Gator" npm run test:e2e --prefix desktop`:
  all six integration paths passed together, **50.8 seconds**. They cover:
  - motion at 10% playback, reversals, inert dismissed controls, focus return,
    reduced motion, no entrance replay on refresh/typing, and narrow drawers;
  - queue edits/order/removal, acknowledged guidance, paused continuation, and
    drafts retained when a queued message starts;
  - real Go state, Saved knowledge empty/populated, drafts, native follow-up
    reviews, saved conversations, task-run filters/keyboard navigation, exact
    copied Markdown/code strings, and detail-tab keyboard behavior;
  - dashboard pause/manual/reduced-motion behavior, real usage counters, and
    separate Go limit/pause review and confirmation;
  - exact approval preview visibility and only the live request ID being sent;
  - first-run native credential fields and secret exclusion from display state.
- `git diff --exit-code c5b74d931 -- cmd internal ui desktop/electron`: passed,
  confirming the backend/protocol/host boundary is unchanged in this pass.
- `git diff --check`: passed.

The initial development run found one old Refresh-button test selector; the
toolbar's accessible name now omits its decorative shortcut. Updating that
selector resolved it. The final packaged run passed without scoped reruns.

Before/after projections and settled screens were inspected across all ten
desktop destinations, plus home, chat, form/review, and narrow layouts.
Integration captures include 1440×960, 1024×768, and 800×600 windows, queue
editing, usage, approval review, and the sidebar's slowed midpoint. Captures
are under `desktop/test-results/` and are intentionally untracked. The audit's
temporary profiles and local fake providers do not use real saved work, paid
inference, sign-in, or enabled background services.

### Verification boundaries

- The isolated approval test injects a display snapshot and records the exact
  renderer action; it does not execute a real model-requested file write.
  Go's approval validation is unchanged.
- Copy tests replace the modern clipboard API and exercise failure feedback;
  they do not overwrite the system clipboard. Native clipboard permission and
  the legacy sandbox fallback still need a manual check on each supported OS.
- Windows/Linux runtime, signed/notarized distribution, OS-native materials,
  and actual installed Codex pixel parity were not verified here. Published
  reference screenshots are the comparison boundary.
- Performance review covered ownership, scrolling, motion replay, and source
  dependencies; this is not a memory/startup benchmark or a full accessibility
  certification.

Verdict: **Shippable for local macOS use within the existing unsigned desktop
rollout.** P1–P10 are implemented and verified. D1–D5 remain explicit product,
capability, or separate presentation decisions; they are not hidden controls
or simulated functionality.
