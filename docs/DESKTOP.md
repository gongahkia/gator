# Desktop companion

## Decision

Add desktop alongside the terminal first. Compare their usefulness in real
work before deciding whether to retire the terminal. A desktop release does
not change the CLI or the current React terminal renderer.

The sibling `Referene` directory contains OpenHands Agent Canvas 1.29.0, a
React DOM / Tailwind web app with an Electron desktop shell. Gator adapts its
neutral palette, sidebar, spacious conversation area, command menu, and
optional right detail panel. Product navigation follows Gator's ongoing
goals, inbox, reviewed follow-ups, saved knowledge, and deliverables.

## Ownership

```text
React terminal (Claude Code Kit) ─┐
                                ├─ Go UI bridge ─ Go Work application
React desktop ─ Electron host ───┘                ├─ conversations and goals
                                                 ├─ tools and approvals
                                                 ├─ checks and inbox
                                                 └─ shared usage and data
```

The desktop host launches the same `gator ui-backend` transport used by the
terminal. It consumes JSON-line display snapshots and sends typed actions.
`ui/src/protocol.ts` is the shared frontend contract. The desktop has no
agent loop, provider credentials, second copy of goal state, or HTTP listener.
Normal state/config overrides still select the profile. The two interfaces
can use the same saved profile, but each window/process owns its own live
foreground run; opening the desktop does not attach to a terminal's live run.
Shared usage and storage locks remain owned by Go.

Desktop view navigation and form edits are small adapters on the existing Go
workflow. A panel carries a content identity; a stale click cannot activate a
newly reordered row or confirm a different preview. All native field values
are checked before any are applied. Saving follows the existing Go validation
and review path. Secrets are replacement inputs and stay absent from display
snapshots and the renderer's persistent storage. Approval buttons carry the
current live interaction ID; retained cards have no approval authority.

## Product surfaces

| Surface | Behavior |
| --- | --- |
| Home | Four live overview summaries and an automatically scrolling strip of workspace actions |
| Navigation | Goals, conversations, inbox, follow-ups, knowledge, usage, backups, recovery, models |
| Conversations | One sidebar entry with Chats and Task runs views; saved chats reopen the transcript, while runs retain outcomes, verification, delivery, and review controls |
| Conversation | Markdown agent text, literal user/output text, typed result cards, streaming progress, stop/steering, and editable queued-message cards |
| Reviews | Exact structured previews with request-bound approve/deny buttons; separate confirmation for workflow writes |
| Native forms | Text/password/multiline fields; existing Go review and save actions |
| Work details | Deliverables and recurring checks; no Context tab or profile footer |
| Usage & limits | Daily request/token meters, shared limit controls, activity breakdown, and expandable accounting details |
| Commands | Cmd/Ctrl+K, searchable commands, arrow navigation, input-prefilling commands |
| Transcript search | Cmd/Ctrl+F, searches prose plus typed result/deliverable data |
| Drafts | Kept in memory while opening views; unsaved forms prompt before leaving |
| Terminal handoff | Reviewed connector sign-in and other interactive CLI steps open the system terminal and return their status |

Enter sends and Shift+Enter inserts a line. Cmd/Ctrl+N starts a conversation
in the selected workspace. During work, Enter queues a follow-up and
Cmd/Ctrl+Enter (or Guide now) sends guidance to the current task. Shift+Enter
keeps inserting a line. Escape closes a view/menu/search or stops active
work; Escape inside a queue editor cancels that edit. Modal focus stays inside
the dialog and returns to its trigger.

The home dashboard advances every six seconds and pauses on hover, keyboard
focus, or manual interaction. Previous/next and pause/resume controls remain
available. Reduced-motion preferences disable automatic movement while keeping
manual navigation. Profile tiles, promotional headings, refresh timestamps,
composer footnotes, and the idle send shortcut have been removed.

Conversations and run history are distinct records: one conversation can
contain several task runs, and some runs may have no retained conversation.
The desktop groups both under Conversations rather than presenting separate
sidebar destinations. Task runs remains a global list, including failed and
stale runs; opening a run retains its existing link back to its conversation.
The `/history` command and all retained records remain available.

### Queued messages

Queued follow-ups appear as a compact list above the composer, following the
[documented ChatGPT desktop pattern](https://learn.chatgpt.com/docs/prompting).
Each message can be edited in place, moved earlier/later, or removed. The
composer draft stays separate from queue edits. Stop and failure retain the
queue with a Paused label; Send now starts a selected paused message. Remaining
messages continue in order after a successful run, as before. Guide now uses
Gator's existing steering channel and becomes available when that task's live
operation is ready. It does not interrupt the task.

Go still owns the existing bounded queue and captured run options. The bridge
projects message identities/text and accepts narrow actions against the exact
message revision or active operation. Edits and guidance clear only after Go
acknowledges acceptance. A message that starts during editing leaves its edits
available as a new draft; an existing composer draft is preserved. Navigating
away from unfinished queue edits uses the existing discard confirmation.
Queue lists and editors have bounded height, labelled 40px controls, keyboard
access, and the app's existing restrained button feedback. No separate queue,
agent loop, or scheduling policy was added to React.

### Motion

Motion uses the existing CSS system and browser animation APIs. Sidebar and
drawer layout transitions retarget when interrupted; labels fade while the
rail resizes. Toggle icons cross-fade in place. Command and discard dialogs
release keyboard focus and become inert as soon as they close, then unmount
when the browser finishes their exit. Reopening cancels the pending removal.

| Interaction | Timing |
| --- | --- |
| Sidebar and work-details layout | 260 ms |
| Sidebar labels, hover and press feedback | 100–120 ms |
| View and detail-tab changes | 180 ms |
| Dialog fade / entrance movement | 140 / 180 ms |
| Toggle icon cross-fade | 300 ms, without bounce |
| Usage details expansion | 220 ms where supported; native disclosure otherwise |

Navigation transitions keep the existing DOM and local form values. Their key
uses the view identity rather than snapshot IDs, so typing, provider deltas,
counter refreshes, and saved-knowledge refreshes do not replay the entrance.
Initial page load is static. Reduced-motion preferences remove transitions,
movement, and automatic dashboard scrolling. Motion does not delay Go actions.

Usage is structured display data from Go's shared accounting store. Uncapped
limits stay explicitly uncapped; the desktop does not invent billing estimates
or provider subscription quotas. Limit edits and pause/resume actions retain
the existing separate review and confirmation, including stale-policy checks.

The desktop starts with a private empty folder rather than capturing the
user's home directory. Choose a workspace with the native folder picker, set
`GATOR_DESKTOP_WORKSPACE`, or resume existing work. Nothing in the sibling
reference repository is required at build or runtime.

## Host boundary

The window loads bundled local assets through `gator://app`. The renderer is
sandboxed, has context isolation enabled, and has no Node or arbitrary IPC
access. The preload exports narrow methods; main checks the sender frame and
validates actions. New windows and page navigation are blocked. Browser links
open explicitly in the system browser and only accept HTTP(S). Markdown raw
HTML is skipped; remote images are not fetched merely by viewing a message.
The content security policy denies renderer network requests. These choices
follow [Electron's security guidance](https://www.electronjs.org/docs/latest/tutorial/security).

Only Go-produced external actions can launch a terminal handoff. The renderer
cannot supply executables, arguments, environment, or shell code. Arguments
are shell-quoted, scripts are private, and terminal return status resumes the
same Go workflow. macOS uses Terminal; Linux needs `x-terminal-emulator`.
An interactive handoff must finish or be cancelled in that terminal before
the desktop closes. Closing other active foreground work asks to stop it,
then shuts down its Go child. Scheduled checks retain their existing service
lifecycle; the desktop does not silently enable an OS service.

## Rollout

Keep both interfaces while checking:

- Whether a user can resume a goal, understand changes, review an action, and
  find the result without knowing slash commands.
- Whether model setup, source recovery, shared limits, and restore reviews
  remain understandable in the desktop.
- Whether startup, idle memory, packaging size, and everyday responsiveness
  justify the extra Electron runtime.
- Whether real terminal usage still provides a useful workflow.

Remove a frontend only after there is evidence to do so. The Go runtime and
headless CLI remain reusable regardless of that later product decision.

## Development verification

The desktop's Electron integration test uses a real built Go backend and an
isolated profile with a local fake inference endpoint. It exercises home,
overview navigation, draft preservation, detail tabs, native follow-up edits
and review, conversation continuation, and multiple window sizes. It also
checks empty and populated Saved knowledge, dashboard motion controls and
reduced motion, real usage counters, and reviewed usage changes. It does not
call paid providers or enable background services. Go tests cover stale view
actions, atomic field validation, secret redaction, reviewed mutations, live
approval IDs, and active-run navigation guards. Host tests cover action and
resource policy, process communication, and child shutdown.

Native packaging is available through `desktop/package.json`. Signed and
notarized release distribution, automatic updates, and Linux desktop runtime
verification remain release work; they are not prerequisites for trying the
desktop alongside the terminal locally.

Referene attribution is in `desktop/licenses/OPENHANDS.txt`.

The app icon is the same standalone 🐊 symbol used in the interface, on a
transparent background. `desktop/assets/icon.icns` supplies the macOS bundle
icon; `icon.png` supplies the running Dock/window icon and Linux packaging.
Both assets are checked in. To regenerate them on macOS, run
`swift desktop/scripts/generate-icons.swift`; ordinary builds do not need Swift.

### Local verification, 21 October 2026

- `go test ./...` and `go vet ./...` passed; subsequent model-status and bridge
  changes passed the affected command/package tests and vet checks.
- `go test -race ./internal/worktui ./internal/modelcatalog -count=1` passed.
- Terminal TypeScript checks, all five UI tests, and bundle build passed. The
  regenerated terminal bundle includes the empty-block rendering fix.
- Desktop TypeScript checks, all six host tests, and frontend/runtime build
  passed.
- All five Electron integration paths passed against the current packaged
  `Gator.app`, using its bundled assets and Go executable. Four passed in the
  full run; the slow-motion path passed separately after its initial 60-second
  timeout. That path now allows 120 seconds for 10% playback/native startup
  and opens the command menu through keyboard interaction. Queue coverage
  includes delayed inference, captured order, paused continuation, and edits
  retained after a message starts; all profiles/providers are isolated.
- Queue changes passed `go test ./internal/worktui`, affected command bridge
  tests, and focused race tests for queue actions, FIFO execution, failure, and
  cancellation. Both desktop and terminal TypeScript checks passed.
- Screens were checked at 1440×960, 1024×768, and 800×600, including narrow
  detail-panel access, command keyboard navigation, dialog focus trapping and
  restoration, and the grid with its titlebar hidden.
- `npm run pack --prefix desktop` produced an unsigned macOS x64 application
  at `desktop/release/mac/Gator.app` (about 362 MB). Test screenshots are under
  `desktop/test-results/` and are intentionally untracked.

Interactive OAuth/terminal handoffs have script quoting and return-status
tests; live sign-in with an external account was not exercised. Linux runtime,
native OS fullscreen transitions, signed installers, and notarization were
not verified in this local pass.

### Interface review: requested desktop cleanup

Full review of the requested home, right panel, Saved knowledge, Usage &
limits, conversation navigation, and desktop motion changes. React DOM,
Tailwind v4, and the existing semantic CSS remain the styling system. Other workflow pages retain their existing layouts.

| Category | Evidence inspected | Result |
| --- | --- | --- |
| Typography | Home, usage totals, reset date, narrow windows | Tabular numbers and existing font/wrapping conventions retained |
| Surfaces | Home cards, usage meters/settings, native forms, details pane, conversation navigation | Requested clutter removed; controls retain at least 40px hit areas |
| Animations | Sidebar/drawer resizing, navigation, dialogs, tabs, icon swaps, disclosures, and dashboard controls | Checked at 10% playback, including reversals, exits, focus, and reduced motion; native scroll timing belongs to Chromium |
| Icons | Dashboard controls, usage actions, existing navigation, and toggle cross-fades | Existing Lucide/currentColor/stroke conventions retained; icon changes keep both states in place |
| Performance | Scroll container, six-second interval, hidden document guard | No animation dependency or continuous React render loop |

| Severity | Location | Before | After | Why |
| --- | --- | --- | --- | --- |
| HIGH | `desktop/src/Panel.tsx:48`, `ui/src/main.tsx:281`, `ui/src/protocol.ts:43` | Empty source references caused an array dereference on null | Nullable wire type and empty-array rendering fallback in both frontends | Empty knowledge must remain usable |
| MEDIUM | `desktop/src/App.tsx:185`, `desktop/src/styles.css:97` | Promotional headings, refresh copy, profile tile, composer footnote and idle send hint | Requested elements and obsolete styles removed | Reduce visual competition with the task |
| MEDIUM | `desktop/src/App.tsx:79` | Context occupied the first right-panel tab and repeated workspace metadata | Deliverables opens first; Checks remains available | Keep the right panel focused on outputs and follow-ups |
| MEDIUM | `desktop/src/StarterDashboard.tsx:13` | Static starter grid | Scrollable action dashboard with pause, manual navigation and reduced-motion support | User-requested motion remains controllable |
| MEDIUM | `desktop/src/UsagePanel.tsx:20`, `internal/uisnapshot/usage.go:5`, `cmd/gator/work_controls.go:82` | Terminal action rows and a long prose dump | Typed quota cards, grouped settings, activity counts and collapsed details | Improve scanning without changing accounting or review ownership |
| MEDIUM | `desktop/src/ConversationsPanel.tsx:5`, `desktop/src/App.tsx:176`, `desktop/src/StarterDashboard.tsx:9` | Conversations and Work history looked like competing archives | One Conversations sidebar entry, with Chats and Task runs views; recent-work shortcut opens this section | Preserve chat continuation and per-run evidence while removing a duplicate entry point |
| MEDIUM | `desktop/src/styles.css:71`, `desktop/src/App.tsx:188` | Sidebar width, labels, and header controls snapped between states; opening details could widen a collapsed rail at intermediate widths | Interruptible grid/label/header transitions, stable navigation labels, and consistent collapsed width | Preserve spatial context and accessible controls during resizing |
| MEDIUM | `desktop/src/motion.tsx:20`, `desktop/src/useModalFocus.ts:3` | Dialogs and details disappeared immediately | Browser-completed exits, immediate inert/hidden state, and immediate focus release; rapid reopening cancels removal | Preserve visible context without keeping dismissed actions or focus traps active |
| LOW | `desktop/src/motion.tsx:42`, `desktop/src/App.tsx:120` | View and detail-tab changes were abrupt | Short opacity/position entrance keyed only by navigation; no initial-load or per-update replay | Add continuity without animating typing or streaming content |
| LOW | `desktop/src/motion.tsx:56`, `desktop/src/styles.css:32` | Toggle icons and press scale changed abruptly | Existing Lucide icons cross-fade in place; explicit short color, shadow and scale transitions | Keep feedback consistent and interruptible |
| LOW | `desktop/src/styles.css:344` | Usage details and its chevron opened instantly | Short native content-size and rotation transitions with reduced-motion and browser fallbacks | Connect the disclosure control with its content |

| Location | Candidate | Rejected because |
| --- | --- | --- |
| Dashboard | Seamless duplicated cards and a continuous animation loop | Duplicate focus targets and constant motion add cost; native scrolling keeps normal keyboard and pointer behavior |
| Usage | Copy Codex's subscription periods and credit balances | Gator owns daily UTC admission limits; provider billing/subscription data is not available here |
| Desktop styling | Add a new animation/component framework | Existing CSS and browser scrolling cover the requested behavior |

Verification uses the commands listed above, plus the real Go accounting
projection test and Electron assertions that pending usage edits have not
changed persisted policy before confirmation. The Saved knowledge regression
failed against the previous build and passed after the fix. Screenshots cover
empty, populated, uncapped, capped, paused, and narrow states. The conversation
test verifies one chat containing two runs, selected navigation in the
collapsed sidebar, retained run details, and reopening the chat with its draft.
The motion test exercises 10% playback, reversal before completion, dismissing
and reopening during exits, immediate focus release, stable entrance counts
while typing/refreshing, reduced motion, and collapsed-rail detail panels at
1024 and 800 pixels (including the titlebar-free layout).
Verdict: Approve
for the requested scope. Platform and external sign-in limitations above
remain unverified.

### Interface review: queued follow-ups

Full review of the composer and queued-message states in React DOM and the
existing Tailwind v4/semantic CSS. The reference establishes the interaction
pattern; this is an adaptation to Gator's queue and steering semantics.

| Category | Evidence inspected | Result |
| --- | --- | --- |
| Typography | Literal multiline/emoji previews, counts, editor, paused label | Existing type scale; wrapping and tabular counts retained |
| Surfaces | Queue list, editor, composer; wide and narrow windows | Bounded scrolling, explicit focus rings, 40px controls |
| Animations | Existing button press/color and send-icon feedback | Reuses the shared reduced-motion behavior; no per-message staged animation |
| Icons | Queue, order, edit, remove, send, stop | Lucide outline icons with currentColor and the existing stroke weight |
| Performance | Go projection, React keys, action acknowledgement, list bounds | No renderer scheduler, frame loop, or animation dependency |

| Severity | Location | Before | After | Why |
| --- | --- | --- | --- | --- |
| MEDIUM | `desktop/src/App.tsx:223`, `desktop/src/MessageQueue.tsx:41` | Count beside a native timing select | Queued-message previews above the composer, editable order/text and removal | Make waiting messages visible and directly manageable |
| MEDIUM | `desktop/src/App.tsx:167`, `desktop/src/App.tsx:225` | Select next request or guidance before sending | Enter queues; Guide now or Cmd/Ctrl+Enter guides the live task; a full queue keeps the draft and disables further queuing | Expose the two actions at their point of use |
| HIGH | `internal/worktui/bridge_queue.go:44`, `desktop/src/MessageQueue.tsx:10` | No message identity, queue-edit action, or acceptance acknowledgement | Revision-bound, bounded host/Go actions, preserved captured options, and acknowledged saves/guidance | Prevent stale clicks or completion races from changing later work or losing edits |
| MEDIUM | `desktop/src/MessageQueue.tsx:62`, `desktop/src/App.tsx:132` | A stopped queue had no direct desktop continuation or edit protection | Paused label and Send now; unsent edits survive queue advancement and prompt before navigating away | Preserve user input and make paused work recoverable |
| LOW | `desktop/src/styles.css:165` | No queue surface | Compact neutral list, multiline clamping, bounded scrolling and existing control feedback | Keep the transcript and composer usable at smaller sizes |

| Location | Candidate | Rejected because |
| --- | --- | --- |
| Queue row | Send now silently interrupts the active task | Gator's existing distinction is queued follow-up versus guidance; stopping must remain explicit |
| Queue ordering | Add drag-and-drop and an animation library | Move-up/down controls work with keyboard and pointer, using the existing stack |
| Queue editor | Replace the composer's current draft when editing | A separate inline editor preserves both drafts |

Verification: Go queue and adapter tests, desktop host-policy tests, both
TypeScript checks, and Electron integration tests with a delayed local test
provider. Queue coverage includes edit/cancel, reorder/remove, guidance, FIFO
execution, pause/resume, stale-edit retention, and wide/narrow layouts. The
existing motion integration test checks shared feedback at 10% playback and
with reduced motion. Exact commands and packaged results are recorded in the
local verification section above. No paid provider calls or background
services are used. Linux runtime and live external accounts remain unverified.
Verdict: Approve for this scope, subject to those platform boundaries.
