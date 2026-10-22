# Conversational desktop implementation

22 October 2026. Implements the first slice of the approved
[surface direction](CONVERSATIONAL-SURFACE-PLAN.md) in the live Electron app.
The HTML prototype remains a fictional interaction study.

## Decision and scope

Use the prototype's simpler organization with Gator's existing visual system:
the neutral dark palette, sage accent, system typography, Lucide icons, native
forms, command palette, restrained motion and precise review controls. The
conversation becomes the front door. Detailed controls remain ordinary screens.

The current decision is **one Gator identity with multiple independent chats**.
The approved follow-up restores New chat and shares durable personal memory
across chats. It retains the existing agent loop, scheduler, model providers,
action reviews, accounting and delivery services.

## Where everything lives

| Entry | Working behavior | Existing capabilities retained |
| --- | --- | --- |
| Talk with Gator / brand | Returns to the current chat; launch reopens the last selected chat with its durable head and source/options. | Conversation IDs, revision ancestry, independent sources and existing history. |
| Saved conversations / folder selection | New chat / Cmd/Ctrl+N creates a separate working context. Reopen an older chat or choose a different source folder for another independent chat. | Per-conversation drafts, prompt history, source snapshots, run options and existing saved topics. |
| Recent chats | Titles first; flat by default, optional folder grouping; filter and expand controls. | Exact list identity validation, current-run attention, disambiguated folder names and full conversation picker. |
| Search / Cmd/Ctrl+K | Existing palette, with added entries for Talk with Gator, Following and Saved. | Advanced commands, keyboard selection, input-prefilling commands and focus return. |
| Following | Read-only saved-goal and actual check summaries; links to Open goals, Manage checks, Updates and Activity & attention. | Goal editor, plans, scheduler scope/timing, inbox read/rating controls, overview and canonical reviews. |
| Saved | Current conversation's files and changes, What Gator remembers, All conversations, Task runs and Unsent drafts. | Knowledge filtering/correction/deletion, retained run evidence, conversation continuation and draft retention. |
| File card in chat / Saved | Opens the exact retained file inside its chat message, with supported preview/edit controls. | Checked/needs-review/proposal states, failed-run origin, text/Markdown previews, shortened-preview notice, source-only HTML and retained locations. |
| Conversation information | Outputs, Sources & memory, Connections and This conversation; closed by default. | Selected sources, retained references, connection provenance, model settings, folder picker, request mode and usage access. |
| Settings / Cmd/Ctrl+, | Existing model, usage, data/backups, recovery and About & updates screens. | Native controls, provider credential handling, exact reviews and reviewed restart checks. |
| Queue above composer | Drag a handle and drop before/after another queued message; keyboard pickup/move/drop is also available. | Edit/remove, paused send, captured run options, live guidance, acknowledgments and FIFO execution after reordering. |

The default home dashboard, rotating starter strip, duplicated conversation/run
tabs and placeholder context-capacity disclosure are retired. Their services
and retained records remain available through the routes above. Detailed screens
keep the existing polished layouts and native fields.

## Durable state and migration

`internal/worksession/active.go` stores the last selected conversation in
`gator/conversations/active.json`. A new unsent chat already has a durable ID;
opening one creates no model request, source snapshot, synthetic revision or
permission grant. Its title becomes the first real request. A failed creation
keeps the previous chat intact. Folder-created chats use the same state reset.

On upgrade, an existing legacy `primary.json` is used as the initial selection
without merging or rewriting any conversation. When no selection or designation
exists, the most recent chat is used. Deleted selections can fall back to a
retained chat; corrupt metadata shows a visible problem. Existing source/options,
revision ancestry and per-chat drafts are restored. Runtime goal binding follows
the restored conversation, including its existing active/paused checks.

Automatic history retention protects an explicitly selected chat, including an
unsent one. Explicit history/all-content deletion still uses the existing scope
review. Restoring never replays a model request, queued message, steering channel
or approval.

Composer drafts now use `conversation:ID` even before the first message. Existing
`workspace:` draft records remain available through Saved drafts → Add to current
message; they are not silently discarded or submitted. No histories are merged.

## Shared memory and earlier-chat retrieval

Personal facts and preferences live in the existing `gator/memory` store, shared
by chats in one local profile. Goal decisions remain scoped to their current
goal. New chat resets the active transcript and task options, with a fresh
working context. It does not delete personal memory, train the provider model,
synchronize devices, or promise lossless recall of every historical detail.

- Foreground `memory_remember` selects useful stable information during the
  normal model/tool loop. There is no separate automatic extraction request.
  The host requires an exact quote from the current user's message and stores
  that quote with the source conversation/run. The model selects what is useful;
  quotation checks establish provenance, not the truth of a claim. Routine
  chatter, one-off tasks, third-party statements, hypotheticals and secrets are
  excluded by the memory policy. File/tool/old-chat text cannot supply the
  current-message evidence.
- Canonical subjects consolidate repeated preferences. A changed quote requires
  the existing record ID and exact updated timestamp; stale inspections cannot
  overwrite a user's newer correction. User edits are marked as corrected and
  no longer presented as the original quote. Advisory locks serialize changes.
- A bounded capsule contains up to four global preferences/basic profile facts;
  other memory is selected through the existing lexical/optional embedding
  retrieval. The combined rendered memory remains bounded to 4,000 bytes with
  exact supplied-context references. Goal memory and excluded records do not
  enter the global capsule.
- `chat_search` and `chat_read` retrieve bounded pages of earlier chat text when
  needed. Search follows retained selected ancestry and reports scan coverage,
  older-turn cursors and unavailable history. Retrieved text is historical
  reference; it carries no old file, connection or action permission. Current
  and other-goal chats are excluded. No bulk transcript mining is added.
- Saved knowledge keeps its existing inspect/correct/forget controls. Explicit
  `memory_forget` removes a saved memory; a hash of its scope/subject suppresses
  automatic relearning until a new explicit remember request. It stores no
  forgotten quote. The original chat still exists until separately deleted.
  This suppression survives backup/restore; transient write locks are excluded.
- **Memory & retrieval → Turn chat learning off/on** controls automatic saves.
  Existing memory can still be recalled, and explicit remember/forget requests
  still work. The same preference is inspectable/changeable through app tools.
  Background jobs and specialists receive no chat-memory mutation tools.

This follows the application-owned storage pattern in Anthropic's
[memory tool documentation](https://platform.claude.com/docs/en/agents-and-tools/tool-use/memory-tool).
[Prompt caching](https://platform.claude.com/docs/en/build-with-claude/prompt-caching)
reuses repeated prompt processing; it does not replace persistent memory or
increase a model's context limit. Gator retains its existing context compaction.
A new chat omits the previous transcript from its working replay; it does not
force a provider to invalidate reusable cached prefixes.

## Image paste in the composer

A paste event with PNG/JPEG/WebP file bytes stages an ordinary attachment for
preview and Send. Accompanying text is inserted at the selection; plain-text
paste follows the browser's normal behavior. No automatic send, clipboard
polling, HTML execution, filesystem path authority or remote rendering is added.
The trusted Electron host decodes valid images to PNG, rejects excessive pixel
counts and applies the existing four-file / 4 MiB each / 8 MiB total bounds
before and after decoding. Staged copies use the existing private upload,
versioning, draft, expiry and message attachment paths.

The event payload follows the [Clipboard API](https://www.w3.org/TR/clipboard-apis/).
Host decoding uses Electron's [nativeImage](https://www.electronjs.org/docs/latest/api/native-image).

Before model setup, pressing Send opens the existing setup screen and keeps
the unsent request. Returning from setup does not submit it automatically.

## Truthful state and the remaining application work

- A saved goal is a saved intention. Only an actual attached check is described
  as configured; its execution state and availability remain separately visible.
  Read failures show unavailable state, with any retained observations identified
  as last reported values. Local operation still depends on this computer.
- Saved shows **files in this conversation**. It does not claim to provide a
  global file index or open arbitrary paths. File checks do not confirm delivery.
- Selected connector IDs remain selections. The information rail does not call
  them authenticated accounts or imply that they authorize external changes.
- Reading an inbox item still does not acknowledge it, rate it or approve an
  action. The exact Go review and live interaction ID remain authoritative.
- Queue drops carry the exact source and target message revisions, plus a
  before/after placement. Go validates both and moves the whole captured request
  in one event-loop action. A consumed/edited target rejects the drop without a
  partial move. The renderer has no separate execution queue.
- **Conversation delivery while work runs remains separate application work.**
  Enter still queues a follow-up; Guide now still requires the live steering
  channel and acknowledgment. Replacing these controls with messaging claims
  would misrepresent the current single foreground loop. A future delivery layer
  needs durable message identities, task routing and accepted corrections.
- Origin-linked cross-topic updates, conversational responsibility setup, a
  cross-topic file index, attachment intake, preferred-topic launch settings and
  richer memory editing remain opportunities. The prototype's example outcomes,
  unread counts, access grants and send confirmations are not production data.

## Interface review

Full review of the changed conversational shell, queue, topic navigation,
Following/Saved drawers, information/preview rail, palette and setup handoff.
Framework: React DOM inside Electron. Styling: existing plain CSS and theme
tokens within the existing Tailwind build; no new styling or motion dependency.
Existing detailed screens were exercised through the retained navigation; their
data export/deletion and OS installation operations were not executed for this UI
change. Native Windows/Linux hardware and new-user usability remain boundaries.

| Category | Evidence inspected | Result |
| --- | --- | --- |
| Typography | Home, both message roles, long Markdown, filenames, drawer captions and native screen headings | Existing type scale, font smoothing, wrapping and tabular numbers retained. |
| Surfaces | Empty/returning chat, library drawers, file cards, queue editing, dialogs and 800–1440px layouts | Existing tokens, structural borders and 40px desktop controls retained; redundant surfaces removed. |
| Animations | Sidebar reversal, information/library switching, modal exit/focus, view transitions, reduced motion | Inspected through the Electron motion journey at 10% playback; no new high-frequency reorder animation. |
| Icons | Navigation, composer options, file states and queue drag handles | Existing Lucide/currentColor/stroke conventions retained; native button focus and labels remain explicit. |
| Performance | Memoized messages, literal transcript search, retained scroll anchors, bounded recent list and queue | No new framework, model loop or global history load; retired dashboard component/styles removed. |

### Changes made

| Severity | Location | Before | After | Why |
| --- | --- | --- | --- | --- |
| MEDIUM | `desktop/src/App.tsx:226`, `internal/worksession/primary.go:20` | Dashboard/new-session front door | Durable main conversation with optional separate topics | Familiar place to return without merging source boundaries. |
| MEDIUM | `desktop/src/App.tsx:246` | Overview cards and rotating starters around first request | Short introduction and one composer | Reduce competing choices before the first exchange. |
| MEDIUM | `desktop/src/RecentConversations.tsx:5` | Mandatory filesystem groups | Flat titles with optional grouping, filter and retained identity guards | Human topics dominate; scope remains inspectable. |
| MEDIUM | `desktop/src/LibraryDrawer.tsx:9` | Six peer functional destinations | Secondary Following/Saved drawers with actual state and explicit routes | Lower navigation load while preserving controls. |
| MEDIUM | `desktop/src/ConversationsPanel.tsx:5` | Repeated Chats/Task runs tabs | Distinct Saved destinations with existing screen styles | Remove duplicate organization without combining records. |
| MEDIUM | `desktop/src/App.tsx:64`, `desktop/src/OutputCards.tsx:4` | Report-heavy output lists before a preview | Contextual cards, exact previews, optional full result details | Outputs can be read beside the exchange; evidence stays available. |
| MEDIUM | `desktop/src/WorkDetails.tsx:37` | Infrastructure-first rail and unmeasured capacity disclosure | Optional conversation information and direct file preview | Progressive disclosure; no placeholder measurement. |
| LOW | `desktop/src/styles.css:196`, `desktop/src/App.tsx:64` | Unbounded-looking assistant turns | Restrained speech surface with existing prose/code/copy controls | Thought boundaries remain readable in a messaging interface. |
| MEDIUM | `desktop/src/App.tsx:198` | Missing model could consume a first request into an error | Setup opens while the request remains unsent | Preserve intent across a prerequisite. |
| LOW | `desktop/src/App.tsx:281`, `desktop/src/Panel.tsx:109` | New destinations absent from palette; native action busy state implicit | Searchable destinations and semantic panel busy state | Preserve keyboard polish and expose actual readiness. |
| MEDIUM | `desktop/src/MessageQueue.tsx:85`, `internal/workapp/bridge_queue.go:103` | Separate up/down buttons | Drag handle, insertion marker and one acknowledged move; keyboard alternative | Direct manipulation with stable identities, honest acceptance and accessible cancellation. |
| LOW | `desktop/src/LibraryDrawer.tsx:13`, `desktop/src/App.tsx:75` | Drawer switches could reuse an old scroll offset or change closing content | Reset the incoming library scroll; retain the outgoing kind through exit | Stable transitions and predictable reading position. |

### Considered but rejected

| Location | Candidate | Rejected because |
| --- | --- | --- |
| Whole desktop | Copy the prototype's complete CSS/theme | User explicitly requested the existing live visual system. |
| Composer | Hide queue/guidance semantics behind a claim of independent conversation delivery | The current loop cannot provide that availability or routing yet. |
| Conversation information | Show a decorative context percentage or authenticated connector count | The application does not report those measurements or account states. |
| Queue drag | Animate a locally reordered copy and send a series of adjacent moves | A changing queue could produce misleading order or partial moves; exact source/target acceptance is clearer. |
| Library navigation | Remove advanced screens after simplifying the sidebar | Existing controls still provide necessary exact reviews and recovery paths. |

### Verification

- `npm run check --prefix desktop`: passed.
- `npm test --prefix desktop`: 27 passed, one Windows-native handoff test skipped
  on macOS. Includes rollback-compatible draft migration and strict drop payloads.
- `go test -p 1 ./internal/worksession ./internal/workapp -count=1`: passed.
  Additional focused tests cover retaining the first main identity during close,
  recovery source changes, stale drop targets and captured request options.
- `go vet ./internal/worksession ./internal/workapp ./cmd/gator`: passed.
- `GOOS=windows GOARCH=amd64 go test -p 1 -exec /usr/bin/true
  ./internal/worksession ./internal/workapp ./cmd/gator`: passed compilation.
  This does not execute Windows binaries or verify native Windows interaction.
- `npm run pack --prefix desktop`: produced the unsigned macOS development app.
  Its bundled UI, host policy/draft store and private runtime matched the built
  assets byte for byte.
- `GATOR_DESKTOP_TEST_EXECUTABLE="$PWD/desktop/release/mac/Gator.app/Contents/MacOS/Gator"
  npm run test:e2e --prefix desktop -- --reporter=line`: **17 passed**. Covers
  persisted main identity across an actual app restart, source/draft separation,
  setup handoff, native pointer queue drops, keyboard move/cancel, edits and
  retained FIFO execution, exact approvals, knowledge/inbox/forms/usage, palette,
  file previews, reading position, responsive layouts and reduced motion.
  Sidebar/drawer/dialog transitions were exercised at 10% playback, including
  reversal and an outgoing drawer retaining its content.

After the shutdown identity fix, the rebuilt package passed the three affected
desktop journeys again: About & updates with an unsent draft, queued-message
editing/dragging/keyboard reordering and execution, and real Go conversation
continuation across restart (**3 passed**). The final package's assets and private
runtime matched the built files. Go store/model tests, vet, TypeScript and host
tests also passed after the final changes.

**Verdict: Approve for the reviewed conversational slice.** Unverified:
native Windows/Linux UI and install/update cycles, destructive data operations,
new-user usability, and the future application contracts listed above. These
checks do not establish those capabilities. No paid inference or external action
was used in these isolated desktop journeys.

## Home ASCII waves — October 22, 2026

**Later decision:** the small concentric ornament described below was replaced
with the user-selected Tide field. See [chat presentation](DESKTOP-CHAT-PRESENTATION.md)
for the current implementation and verification. Click effects remain disabled.

Full polish review scoped to the empty home introduction. React and the existing
plain CSS tokens remain the implementation and styling system. The home keeps
multiple ambient waves; the click-triggered ripple trial was removed at the
user's request.

| Category | Evidence inspected | Result |
| --- | --- | --- |
| Typography | Fixed 93 × 23 character field, existing greeting and composer | Monospaced ASCII uses the existing font token; frames do not shift text or controls. |
| Surfaces | Home with setup callout and a configured-provider presentation, 800/1024/1440px windows | Multiple sage waves surround the alligator; shorter windows compact the art and retain the setup button above the composer. |
| Animations | Native pulse clock at 10% playback, live reduced-motion changes, navigation away and back | Gentle outward waves; reduced motion displays a static frame and cancels the loop. Background clicks do not create additional waves. |
| Icons | Existing home alligator and composer icons | Existing branding, sizes and interactions retained. Decoration is hidden from assistive technology, accepts no focus and does not intercept pointer events. |
| Performance | Component lifecycle, bounded frame generation and visibility handler | Glyph updates capped at 12fps outside React rendering; animation and frame requests are canceled on unmount and paused on document visibility changes. No background click listener or pointer-ripple lifecycle remains. |

### Changes

| Severity | Location | Before | After | Why |
| --- | --- | --- | --- | --- |
| LOW | `desktop/src/AsciiWaves.tsx:25`, `desktop/src/App.tsx:247`, `desktop/src/styles.css:148` | Static home mascot | Soft ASCII ripples with a slow opacity pulse, fixed layout, static reduced-motion frame and short-window sizing | Add the requested personality while preserving the introduction and composer as the primary content. |
| LOW | `desktop/src/AsciiWaves.tsx:16`, `desktop/src/styles.css:150` | Widely spaced rings faded into a single visible wave | Closer wave spacing and a broader soft mask | Keep multiple ambient waves visible together at the user's request. |

### Considered but rejected

| Location | Candidate | Rejected because |
| --- | --- | --- |
| Home artwork | Canvas/WebGL or an animation dependency | A small text field and the browser animation clock provide the requested ASCII effect with the existing stack. |
| Transcript | Keep the wave visible during conversation or use it as an activity indicator | The request concerns the landing page; the existing progress indicator already communicates actual work. |
| Home background | Create additional ripples on click | Removed at the user's request; keep the ambient waves only. |

### Verification

- TypeScript and production UI build passed.
- An isolated Electron browser check passed: changing ASCII frames, unchanged
  greeting/composer bounds, 10% playback, static reduced motion, visibility event
  pause/resume, navigation cleanup, no page errors or horizontal overflow, and
  800 × 600, 1024 × 768 and 1440 × 960 layouts. The setup button remains fully in
  the home scroll viewport at the tested smaller sizes. Screenshots were
  inspected for the wide, short-window and reduced-motion states.
- The configured-provider screenshot used a local presentation snapshot; it
  made no provider request. Visibility was simulated because the attached
  Electron debugger kept a hidden native window logically visible.

**Verdict: Approve for the home decoration.** Unverified: native hidden-window
visibility delivery without debugging, native Windows/Linux rendering, and
screen-reader hardware. This review does not cover other application surfaces.
