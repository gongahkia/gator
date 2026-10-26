# Gator terminal migration plan

26 October 2026. Status: implementation in progress; Electron remains the reference. See the
[verification ledger](TERMINAL-MIGRATION-VERIFICATION.md) for evidence and open cases.

Implemented checkpoints: neutral shared host and contracts; pinned OpenCode
frontend slice and Gator adapter; keyboard hubs and file workbench; native-service
adapters; independent package/install tooling. Adapter contract tests now cover
draft restoration, delayed imports/results, admission ownership and shutdown.
These checkpoints are implementation progress, not a passed parity gate. The
remaining behavioral journeys and deferred macOS/Windows runtime checks must pass
before the terminal becomes primary or Electron is removed.
Large tables now page bounded row/column windows, file selections resolve
against the exact saved version chosen for Ask Gator, and exports support guarded
replacement review. The editor handoff now restores owned drafts after an
acknowledged Go completion; account/install and native-platform journeys remain.
Durable update cards now retain independent visibility/read/done/feedback receipts,
origin-linked replies and paged health details with real Go integration evidence.
Connected-action receipt checks and an actual runtime-crash recovery fixture now
verify that reopening never retries the external send or rewrites sealed evidence.
An idempotent preference import now preserves applicable Chromium display choices
without opening or changing the original database; actual Electron-to-Bun evidence
is recorded in the ledger. Native-platform import checks remain deferred.
A real Go connection journey now covers reviewed token setup, access verification,
disconnect and local OAuth callback completion across chat navigation. Browser
launch is fixture-verified on Linux; native browser/permission checks remain open.
Dictation now blocks ordinary draft submission and live guidance until
capture/transcription ends.
Linux keyboard/process tests verify review, cancellation, ownership and Go usage
accounting with synthetic audio; native audio hardware acceptance remains open.
Keyboard backup/verify/restore and retention reviews now have real Go evidence;
restore also preserves valid unsent chats whose empty directories are omitted
from backup archives. Remaining private-data edge cases stay open in the ledger.
Model and effort controls now retain visible availability reasons and refresh
while open; first-task setup preserves immutable attachments through restart.
Provider forms now expose their canonical save/review action independently of
field editing, with keyboard evidence for private credentials and metadata.
Local-model management now has keyboard evidence for reviewed download, streamed
progress/cancellation, rename, selection, inference routing and reviewed removal
against an isolated runtime fixture; native installation remains unverified.
Go now supplies terminal-specific in-app help to terminal-hosted runs while
preserving the desktop reference guide and the same execution authority.
Conversational app-control evidence now covers persisted shortcuts, future model
and connection selections, approved limits/retention and cancelled data deletion.
Queue keyboard evidence now covers edits, ordering, removal and explicit recovery
with immutable attachments while retaining the independent composer draft.
Attachment intake now has keyboard evidence for all current byte/count limits,
visible rejection, clipboard PNG capture, restart and explicit files-only send.
Native playback now carries language and rate on all three platforms; helper
contract tests pass, while actual macOS/Windows voice checks remain deferred.
Cross-chat memory/privacy evidence now verifies real provider context, independent
learning/history controls, separated transcripts and retained drafts at restart.
Saved guidance now has keyboard evidence for typed scope/type choices, full text
edits, status filtering, enable/disable and persistence.
Completed public activity now folds above its answer, supports keyboard inspection
and expanded-history search, with real-runner restart/non-replay evidence.
Short terminal hubs now reveal keyboard selection after navigation and resize;
compact 50×18 and 42×14 tests retain drafts and reach lower destinations.
A native zero-height selector failure at 42×14 is fixed with bounded list
geometry and compact dialog positioning; the failing regression now passes.
Activity histories share the transcript subscription, with a 60-history regression
preventing per-message listener growth.
Conversational recurring checks now have real-tool evidence for decline, create,
edit and pause reviews, origin/expiry preservation and restart without execution.
The refreshed standalone package passes production services, PTY/editor and owned
install/uninstall probes. Linux fresh/150-revision startup, search, complete-tree
RSS/PSS and cleanup measurements are recorded against the retained Electron app.
Preference writes now serialize stale-lock recovery through an OS-backed lock;
crash/concurrent-writer checks preserve acknowledged updates and profile imports.
Quiet-check feedback now has keyboard evidence for missed/correct assessments,
stale-edit rejection, unsaved-form recovery and clearing without changing checks.
Appearance now has rendered-color evidence across settings, cached chats, compact
dialogs and restart, including all four supported themes without extra requests.
Installed PTY checks now open untrusted file escape payloads without emitting
their clipboard/title commands; native CI includes PTY/editor and install probes.
Conversation information now includes latest-result-only files and proposed
changes, retained preview/check disclosures and origin navigation. Following
surfaces unavailable check health instead of implying there are no checks.
A real Go-generated artifact now has end-to-end editing/restart/Ask Gator evidence;
its sealed output and manifest remain byte-identical after the edited version sends.
The fresh standalone artifact passes production services, installed lifecycle and
PTY/editor/untrusted-file checks; PTY assertions now interpret incremental redraws.
Saved-knowledge corrections now select their actual form field after opening a
later list item; real-note keyboard tests cover conflicts, restart and forgetting.
Revision/source evidence now covers explicit snapshot refresh, backward/forward
ancestry, retained drafts and reopening from a different launch directory.

Replace the Electron application with Gator's adaptation of **OpenCode's actual
terminal frontend**, carrying forward Gator's current capabilities. Use an
OpenCode-derived shell, components, and interaction system, with Gator's identity,
pages, and backend. Keep Electron available during the port as a working
reference. Remove its frontend, host, dependencies, and packaging only after the
terminal implementation passes the feature parity gate below.

This is a frontend and application-host migration on top of the current Go
application. Preserve the current agent engine, tools, data, approval model, and
recent improvements. Adapt OpenCode's frontend through a Gator-specific application
adapter. The earlier Gator terminal remains a reference for useful behavior and
regression cases; it is no longer the frontend implementation to restore.

## Starting points

| Reference | Purpose |
| --- | --- |
| `912a7e7ac17f2c14ebb1aeb516b738eccf4e96c1` | Current committed baseline when this plan was written. Reconcile later changes before implementation. |
| `bb232e59b` | Last commit before terminal retirement; inspect terminal behavior, launcher integration, and useful regression cases. The old React renderer is historical reference material. |
| `f936f5d68` | Completed terminal UI audit on 18 October, using the independent Claude Code inspired React kit. |
| `07e27c752` | Terminal retirement on 22 October; identifies deleted files, entry-point changes, and build/release changes. |
| `0d9bab473` | Renamed the retained Go application model from `internal/worktui` to `internal/workapp`. |

The [terminal UI audit](TERMINAL_UI_AUDIT.md) describes the earlier experience.
The [desktop decision](DESKTOP-PRIMARY.md) records the preserved application and
data boundaries. The [current surface map](DESKTOP-CONVERSATIONAL-IMPLEMENTATION.md)
and the live code/tests establish the capabilities to port. Historical documents
contain older limits and behavior; use the current implementation when they differ.

## OpenCode source baseline and integration boundary

The upstream reference is [anomalyco/opencode](https://github.com/anomalyco/opencode),
inspected at commit
[`f669c46c9604ea3d70b8a7ed1f0c10f3a5ac1f61`](https://github.com/anomalyco/opencode/tree/f669c46c9604ea3d70b8a7ed1f0c10f3a5ac1f61)
on 26 October 2026. Its TUI package declares version `2.1.4`. Use this immutable
snapshot as the initial source baseline, and record any deliberately selected
replacement revision before implementation. Keep V1 examples separate from this
V2 source tree.

| Upstream source at that revision | Implication for Gator |
| --- | --- |
| [`packages/tui/package.json`](https://github.com/anomalyco/opencode/blob/f669c46c9604ea3d70b8a7ed1f0c10f3a5ac1f61/packages/tui/package.json) | The frontend uses Solid and OpenTUI. `@opencode/tui` is a private workspace package with dependencies on other upstream packages, including client/core packages; it is not an assumed standalone npm widget. |
| [`packages/tui/src/app.tsx`](https://github.com/anomalyco/opencode/blob/f669c46c9604ea3d70b8a7ed1f0c10f3a5ac1f61/packages/tui/src/app.tsx) | The startup input expects an OpenCode server endpoint and constructs an OpenCode client. Adapt this composition root to start Gator's host and consume its adapter. |
| [`context/client.tsx`](https://github.com/anomalyco/opencode/blob/f669c46c9604ea3d70b8a7ed1f0c10f3a5ac1f61/packages/tui/src/context/client.tsx) and [`context/data.tsx`](https://github.com/anomalyco/opencode/blob/f669c46c9604ea3d70b8a7ed1f0c10f3a5ac1f61/packages/tui/src/context/data.tsx) | Client/event/data contexts depend on OpenCode API shapes. Replace or adapt these boundaries and their callers explicitly; changing a backend URL is insufficient. |
| [`package.json`](https://github.com/anomalyco/opencode/blob/f669c46c9604ea3d70b8a7ed1f0c10f3a5ac1f61/package.json) and [`packages/cli/package.json`](https://github.com/anomalyco/opencode/blob/f669c46c9604ea3d70b8a7ed1f0c10f3a5ac1f61/packages/cli/package.json) | The inspected workspace pins Bun `1.4.4` and has Bun and Node build paths. Establish Gator's supported build/runtime combination and native dependencies through a working prototype. |
| [`LICENSE`](https://github.com/anomalyco/opencode/blob/f669c46c9604ea3d70b8a7ed1f0c10f3a5ac1f61/LICENSE) | Retain the upstream MIT copyright/license and applicable dependency notices with reused code and distributions. |

Maintain a source fork or vendored frontend slice with an `UPSTREAM.md` record of
the repository, commit, copied packages/files, notices, local patches, and update
procedure. Reuse the shell, composer, transcript, tabs/sidebar, dialogs, palette,
theme/keybinding infrastructure, and other suitable frontend components. Add
Gator-specific views using that same UI system. Using OpenTUI alone to build an
unrelated imitation would not satisfy the requested frontend adoption.

The adapter owns the translation between frontend views and Gator operations.
Gator continues to own conversation IDs, durable messages, work state, queue
admission, approvals, models, tools, and storage. Upstream client caches may become
view projections; they must not introduce a second authoritative session store.
Replace upstream agent/server startup and direct execution paths. Gator is not a
model provider plugged into a second OpenCode agent loop.

Prefer adapting the client/data contexts to a typed Gator frontend interface over
emulating the entire OpenCode server API. Preserve the current private Go stdio
bridge. If a necessary upstream dependency needs a different transport, document
and test the smallest local boundary before adopting it; an HTTP server is not
an implicit requirement of this plan.

## Architecture and product decisions

- Launch the interactive terminal application with `gator`. Retain appropriate
  script commands, help/version output, and internal service entry points. Restoring
  the terminal does not imply reviving unrelated retired products or protocols.
- Adopt OpenCode's Solid/OpenTUI terminal frontend with a conversation-first
  layout, collapsible navigation, visible session activity, and dedicated views.
  Keep Gator's identity, palette, bottom composer, slash discovery, searchable
  pickers, and focused review forms. Keep ordinary chat visually restrained.
- Keep Go authoritative for work, permissions, model configuration, approvals,
  admission, accounting, and retained records. Keep one agent loop.
- Extract reusable Node-compatible host services from `desktop/electron/` into a
  neutral package. Both frontends use these services during migration. Test their
  compatibility with the selected terminal runtime; retain an explicit Node host
  process only if needed. The final application must not import or start Electron.
- Move display/action types out of `desktop/src/` into a shared location. Separate
  terminal input/rendering, the OpenCode-to-Gator adapter, host operations, and Go
  bridge operations explicitly. Keep the frontend's data requirements separate
  from Go wire types so upstream changes stay localized to the adapter.
- Keep `internal/workapp` named for its application role. Avoid a second large
  package rename or a rewrite of working host services into another language.
- Plan for Go plus the OpenCode frontend toolchain. Pin Solid, OpenTUI, the
  selected Bun/Node versions, transforms, native assets, and runtime packaging.
  Choose the distribution after proving the required platform matrix; the old
  React/esbuild/Node bundle is not the assumed build path.
- Keep upstream code changes focused and distinguish imported files from Gator
  extensions. Record the dependency closure, maintain notices, and run adapter
  and UI regression checks before taking an upstream update.
- Keep the existing profile identity and storage formats. Introduce neutral names
  for frontend settings only with backward-compatible reads and migration tests.

The intended dependency flow is:

```text
OpenCode-derived Solid/OpenTUI frontend
          |
Gator frontend adapter: client, events, data, routes, commands
          |
Shared application host: sessions, drafts, files, platform adapters
          |
Go JSON-lines bridge and application model
          |
Current agent engine, tools, stores, scheduler, and providers
```

During the port, Electron also calls the shared host. After removal, neither
development nor installed terminal builds may resolve services from `desktop/`.
Choose the smallest practical package layout in step 2; `ui/` can hold the adapted
frontend, its provenance record, and Gator extensions. Import the necessary
frontend dependency closure without copying the entire upstream monorepo.

## Proposed terminal navigation

These are target interactions to implement, not a list of commands already
available in the current application. Reconcile aliases with the existing Go
command registry in step 1. Adapt OpenCode's navigation/components to these
destinations, with a collapsible rail and session tabs or a switcher as space allows.

| Destination | Terminal access | Contents |
| --- | --- | --- |
| Chat | Default view, `/chat`, `/new` | Conversation, activity, composer, model/effort, queue, and pending reviews. |
| Chats | Chat switcher, `/conversations` | Recent/running chats and the searchable retained catalog. |
| Following | `/following` hub | Goals, plans, checks, updates, health, and attention; routes to the existing detailed controls. |
| Saved | `/saved` hub | Knowledge, conversations, task runs, unsent drafts, and the current conversation's files. |
| Settings | `/settings` hub | Models, connections, memory/privacy, appearance/keys, usage, data/backups, recovery, and About/updates. |
| Files and context | Contextual file actions, `/files`, `/context` | Open/edit/version/export/Ask Gator, sources, and selected connection information. |

Slash discovery and the searchable launcher expose these destinations and their
actions. Returning from a page restores the chat's draft and reading position.
Lists, details, forms, and explicit confirmations share consistent keyboard
navigation; opening a page does not submit a request or grant access.
Collapse secondary navigation on narrow terminals while preserving command access
to every destination. Surface unread updates and pending reviews without filling
ordinary conversation with a permanent dashboard.

Preserve Gator's distinction between sending, queuing, and guiding active work
when adapting the OpenCode composer. Route shell/tool execution and permissions
through Gator's existing controls. Map undo/redo only to operations Gator can
actually reverse; external actions cannot be presented as undone by transcript
navigation. Reconcile upstream commands, plugin/config discovery, sharing,
telemetry, and update routes so imported UI does not activate unrelated OpenCode
services or read/write a separate OpenCode profile.

## What feature parity means

Parity means that a user can complete the same currently supported task, with
the same data, scope, review, recovery, and persistence guarantees. The terminal
may use different controls and layout. A page appearing in a command menu is not
proof that its actions work.

Every workflow needs a successful path plus its applicable cancellation, failure,
stale-state, navigation, and restart checks. A retained engine function without
an accessible terminal workflow is still a gap. Hidden Electron windows and
Electron-powered helpers do not count as a completed port.

Visual file previews, Office editing, clipboard images, and voice need explicit
terminal or native adapters. An external viewer may be useful where terminal
graphics are unavailable, but opening a file alone does not establish parity
for editing, selecting content, versioning, or asking Gator about it. Record helper
requirements and any changed interaction in the verification ledger. A reduced
capability remains pending unless the user explicitly changes the requirement.

Preserve existing platform support and its documented limitations. The Windows
preview's unavailable sandbox/terminal tools must not become silently enabled.
Cross-compilation establishes build compatibility; native behavior needs native
evidence at the level claimed for that platform.

## Required capability inventory

All rows start **pending**. Expand them into individual acceptance cases in
`docs/TERMINAL-MIGRATION-VERIFICATION.md` during step 0. For each case record the
baseline source/test, terminal entry, platform, verification command/result,
and remaining gap. Add newly discovered workflows to the inventory.

| ID | Capability | Required terminal outcome |
| --- | --- | --- |
| P01 | Shell and onboarding | Welcome, first-task guidance, setup without lost drafts, slash discovery, help, multiline input, history, search, scrolling, theme, and status display work from the keyboard. |
| P02 | Conversations and workspaces | New/reopened chats, last selection, selected revision ancestry, workspace selection, source refresh, and per-chat options retain their current semantics. Launching in a directory does not silently widen a restored chat's source access. |
| P03 | Independent running chats | Switch between chats while their tasks continue; retain correct activity, unread, approval, queue, and error ownership, with current admission/process limits. |
| P04 | Model and effort controls | Select actual configured models and effort, preserve drafts, respect active-work restrictions, and capture the selected options with queued/admitted messages. |
| P05 | Transcript and activity | Stream replies and public work activity; search current/older retained text, inspect artifacts and results, copy content, and keep the reader's position when new activity arrives. |
| P06 | Admission and recovery | Preserve text/files until confirmed admission; edit/reorder/remove/resume queues; distinguish rejection, unconfirmed send, interruption, and completion. Restart leaves restored work paused and never automatically repeats an external action. |
| P07 | Approvals and action receipts | Show the exact review and bind decisions to the correct live run/token. Inspect connected requests, uncertain sends, reconciliation, and retained receipts without confusing review, read, retry, and approval. |
| P08 | Conversation catalog | Search indexed older chats; page results; pin, archive, rename, and organize supported records with current identity/version checks; persist organization across restart. |
| P09 | Personal memory and saved knowledge | Browse, filter, inspect, correct, and forget supported memory/notes/guidance; retain provenance, guarded edits, cross-chat recall, learning preferences, and earlier-chat privacy. |
| P10 | Context and connections | Inspect sources, supplied memory, outputs, and selected connections; change future selections without changing captured queues or treating historical provenance as current authority. |
| P11 | Goals and recurring checks | Create/manage goals and plans, configure/review checks, inspect timing/scope/health, and distinguish a saved goal from a running recurring check. |
| P12 | Updates and attention | Following, Inbox, quiet-check health, origin-linked chat updates, pagination, unread/read/done receipts, replies, and usefulness feedback retain their independent meanings. |
| P13 | Composer and file drafts | Keep per-chat drafts and attachments through navigation/restart; browse/recover/remove drafts; respect disabled retention; recover independent file edit drafts. |
| P14 | Attachments | Select/import/remove supported files, including files-only messages and clipboard images; preserve private immutable copies and current model intake. Current limits: 12 inputs, documents 8 MiB each, images 4 MiB each, 16 MiB combined. |
| P15 | Text and tabular editing | Open full text/Markdown/code/JSON and CSV/TSV; edit supported content/cells, validate where applicable, preserve supported formatting/quoting, and explicitly save/discard. |
| P16 | Native Office content editing | Inspect/edit the supported DOCX paragraph/table/run content, XLSX sheets/cells/formulas, and PPTX existing text. Preserve package parts and existing protected/signed/macro restrictions and rendering limits. |
| P17 | Image and PDF previews | Inspect actual images and PDF pages through supported terminal/native adapters, navigate pages, and return to the same chat/draft. Do not replace visual inspection with filenames or extracted text alone. |
| P18 | File versions and agent handoff | Save immutable working versions, inspect/compare/restore versions, export exact selected bytes, and stage Ask Gator with the saved version and supported selection. Preserve originals and sealed outputs; reject stale saves. |
| P19 | Provider and local model setup | Configure credentials/endpoints/custom providers; inspect/download/select/rename/remove local models and view installation progress. Preserve masking, readiness checks, and explicit reviews. |
| P20 | Account and tool integrations | Complete supported browser/device/API-key sign-in, verification, reconnect/disconnect, and reviewed external editor/install/tool handoffs. Preserve current browser, computer-control, connector, and specialist capabilities. |
| P21 | Voice | Explicitly start/stop/cancel dictation, review text in the original draft, account for provider usage, and optionally speak only new replies. Preserve permission, cancellation, conversation-binding, and transient-audio rules. |
| P22 | Usage and private data | Inspect/change limits and pause/resume, retention previews, deletion reviews, backup/export/verify/restore/relink, and recovery. Preserve cross-process guards and supported backup contents. |
| P23 | Preferences and app control | Inspect/change applicable settings through both terminal pages and conversation; keep theme, usable shortcut customization, privacy, voice, and reviewed app mutations consistent. |
| P24 | Profile and service continuity | Reuse conversations, knowledge, credentials, file versions, usage, drafts, and profile overrides; preserve reviewed background service start/stop/login behavior after executable paths change. |
| P25 | Distribution and lifecycle | Develop/install/launch/update/uninstall the CLI with documented dependencies; handle reconnect, quit, crashes, and updates without losing drafts or replaying requests. Retain platform capability disclosures. |

## Ordered implementation checklist

### Step 0 Capture the current baseline

- [x] Record implementation HEAD, preserve pre-existing changes, and use isolated
  state/config/draft roots for migration fixtures. Do not test destructive reviews
  or profile conversion against the user's live profile.
- [x] Create the verification ledger and expand P01–P25 from current screens,
  preload/host operations, Go bridge frames, and existing test cases. Record
  current unsupported platform behavior separately from migration regressions.
- [ ] Run the applicable existing Go, desktop host, and desktop integration checks;
  record failures and environment limits before changing their implementation.
- [ ] Capture representative old-terminal layouts and current desktop journeys,
  including first run, long chat, live approval, file edit, concurrent chats, and
  restart recovery. Keep reusable fixture data independent of DOM selectors.
- [x] Record the exact OpenCode source revision, package versions, dependency
  closure, licenses, and relevant upstream tests. Inventory imports that reach
  OpenCode client/core/server/config/plugin services and classify each as retained
  frontend support, an adapter target, or code excluded from the Gator build.

Done when every known capability has a baseline and a proposed acceptance case.

### Step 1 Define the terminal experience and resolve difficult adapters early

- [ ] Specify Chat, Following, Saved, Settings, and conversation information as
  keyboard-accessible OpenCode-derived screens/panels. Use a collapsible navigation
  rail, visible chat activity, and a restrained composer. Keep slash commands and
  a searchable launcher as entry points, including in narrow terminals.
- [ ] Define new-chat/chat-switch, model/effort, queue, file, and review interactions.
  Choose nonconflicting keybindings supported by real terminals, with fallbacks
  for combinations terminals cannot distinguish and retained readable help.
- [ ] Prototype the pinned OpenCode shell with a Gator client/data/event adapter.
  Prove startup, current conversation state, a streamed reply, and an exact
  approval can use Gator without an OpenCode agent/server. Validate resize, long
  lists, Unicode input, multiline/bracketed paste, and exact slash input.
- [ ] Prove the Solid/OpenTUI build and native assets on the initial target host;
  select Bun or Node runtime packaging against the full required platform matrix.
  Verify extracted host compatibility rather than assuming all Node APIs behave
  identically under Bun. Record the decision and dependency lockfile strategy.
- [ ] Prove an Electron-free path for image/PDF viewing, native Office content
  editing, clipboard image intake, microphone capture, and spoken output. Record
  platform helpers, permissions, installation, failure behavior, and gaps early.

Done when the hard capabilities have concrete implementation routes. Keep any
unresolved parity requirement open rather than silently narrowing the product.

### Step 2 Extract shared services and implement the frontend adapter

- [x] Move the current display/action/file contracts to a neutral location and
  update desktop imports. Account for protocol evolution since the old renderer:
  acknowledgments, approval tokens, upload admission, session requests, receipts,
  memory actions, catalog actions, and current panel field types.
- [ ] Define a typed Gator frontend interface and adapt OpenCode startup,
  client/data/event contexts, subscriptions, loading, reconnect, and callers.
  Translate stable IDs, message/activity projections, model choices, permissions,
  and paging explicitly; avoid fabricated OpenCode state and unchecked type casts.
- [ ] Preserve ordering and confirmation semantics across the adapter: only Go
  admission clears drafts, only current tokens resolve approvals, and reconnect
  does not duplicate submissions or resurrect completed work. Add contract tests
  for stale state, duplicate/delayed events, subscription cleanup, and chat changes.
- [x] Extract backend transport, session management, drafts, uploads, file storage,
  Office transformations, comparisons, and retained-request cleanup. Move their
  tests and dependency/license ownership with them.
- [ ] Separate host operations from Electron IPC/dialogs. Keep canonical Go
  validation plus current host checks for identity, paths, scope, stale revisions,
  limits, and process ownership. Replace Electron-specific checks at the terminal
  adapter boundary rather than discarding validation with IPC.
- [ ] Define platform adapter interfaces for paths, clipboard, external viewers,
  links, audio, notifications, terminal handoffs, and update/lifecycle integration.
  Keep the desktop application working against the extracted services.

Done when shared services run and test without importing Electron, the Gator
adapter satisfies the frontend's required contracts, and the existing desktop
workflows still pass against the extracted services.

### Step 3 Integrate OpenCode frontend source as the Gator terminal shell

- [x] Bring in the pinned OpenCode frontend source and required reusable support
  packages as a maintained fork or vendored slice. Preserve licenses and useful
  tests; document inherited, adapted, and replaced modules in `UPSTREAM.md`.
- [ ] Wire the shell, composer, transcript, tabs/sidebar, dialogs, command palette,
  themes, and keybindings to Gator's adapter. Supply Gator branding, routes,
  settings/profile paths, help, and lifecycle handling. Remove upstream server
  startup and direct execution paths from the production dependency graph.
- [ ] Add the public console launcher and a development terminal target while
  keeping the existing desktop launch target available during migration. Handle
  missing runtime/native dependencies clearly; make help/version/script output
  pipeable and keep build-tool requirements separate from installed runtime needs.
- [ ] Connect first run, model setup, a streamed task, cancellation, exact approval,
  and saved conversation reopen as a complete vertical slice.
- [ ] Restore proper raw-mode cleanup, terminal handoff/resume, interrupt/exit
  behavior, resize, Unicode/grapheme editing, literal untrusted output, and safe
  handling of terminal escape sequences. Paste must never imply send or approval.

Done when a packaged OpenCode-derived terminal build completes that slice against
Gator with an isolated profile, no Electron process, and no OpenCode agent/server.

### Step 4 Port durable chats and concurrent execution

- [ ] Integrate the shared session host, per-chat ownership and limits, chat
  switching/new chat, current activity, foreground controls, and unread state.
  Bind OpenCode's session navigation to Gator IDs and lifecycle; do not create a
  competing upstream session database.
- [ ] Port composer model/effort controls, prompt/draft history, attachments,
  text guidance, queue editing/reordering, and per-request captured options.
- [ ] Port admission acknowledgments, paused restored queues, interrupted and
  unconfirmed requests, explicit retry, receipts, and uncertain-send inspection.
- [ ] Verify disconnects, crashes, normal quit, and restart across multiple chats.
  Flush drafts/files and stop owned processes deliberately; never route a pending
  approval or completion into the newly selected chat.

Done when P02–P07 and the chat portions of P13 pass, including failure/restart cases.

### Step 5 Port all pages and conversation controls

- [ ] Implement the modern Following/Saved hubs and all their detail/actions:
  goals/plans, checks, Inbox, attention, task runs, knowledge, conversations, drafts,
  and contextual files. Preserve filters, pagination, guarded organization, and
  safe navigation while another chat is working. Build these Gator-specific views
  within the adapted Solid/OpenTUI shell; OpenCode does not supply their backend.
- [ ] Implement Settings for models, connections, appearance, memory/privacy,
  usage, private data/backups, recovery, and About/updates. Support current typed
  fields and exact review screens, not just generic text dumps.
- [ ] Port personal memory receipts, guarded correction/forgetting, learning and
  earlier-chat privacy controls, chat-linked check health, updates, and feedback.
- [ ] Port conversation information and app-control actions, preserving source,
  permission, connection, evidence, and current-versus-captured option distinctions.

Done when all page actions in P08–P12, P19, P22, and P23 work from the keyboard;
opening a page must not implicitly acknowledge, approve, or run anything.

### Step 6 Port complete file workflows

- [ ] Integrate file selection/path completion, private uploads, clipboard intake,
  files-only drafts, retained input labels, current limits, and admission handling.
- [ ] Build terminal file views/editors for supported text, JSON, Markdown, tables,
  and Office content using the existing content services. Preserve full buffers,
  validation, read-only restrictions, cell/formula semantics, and format limits.
- [ ] Implement image/PDF preview adapters, version history, bounded comparisons,
  stale-save handling, draft recovery, Save/Discard, restoration, and exact export.
- [ ] Integrate Ask Gator with the exact saved file/version and supported text/cell
  selection. Verify exported native files independently and confirm original
  uploads and sealed work manifests remain unchanged.

Done when P14–P18 pass for the current supported formats. File versioning, Office
editing, and visual previews are required work, not post-removal enhancements.

### Step 7 Complete native integrations and terminal usability

- [ ] Finish platform adapters for provider sign-in, account verification,
  reconnect/disconnect, external editors, reviewed installation, links, copy/paste,
  and existing notification paths. Resume the same session after handoffs.
- [ ] Implement dictation/playback and voice settings using the existing provider
  and accounting services. Cover OS denial, cancellation, conversation switching,
  limits, recording cleanup, and no historical-reply playback.
- [ ] Finish themes, status-line preferences, keybinding configuration, help, and
  app-control integration using the adapted frontend infrastructure. Reconcile
  Gator and upstream bindings where terminal keys differ; preserve user preferences
  and Gator's send/queue/guide behavior.
- [ ] Exercise narrow/resized terminals, long transcripts/files, focus return,
  visible busy/error states, reduced animation, and usable text-only fallbacks.

Done when P01, P20, P21, and the remaining P23 controls pass on the claimed platforms.

### Step 8 Preserve profiles and migrate frontend data

- [ ] Keep existing state/config locations, profile selection, path resolution,
  and stored formats. Test both absolute and relative overrides, including their
  existing Windows/Unix differences.
- [ ] Locate and preserve Electron user-data drafts and applicable preferences,
  which are separate from the Go profile. Test idempotent import, disabled draft
  retention, interrupted import, duplicate/conflicting drafts, attachment
  references, and multiple profiles without merging their private content.
- [ ] Preserve file edit drafts, versions, aliases, admitted request journals,
  conversations/indexes, credentials, memory, and current backup/restore behavior.
  Keep compatibility for old settings/environment names as needed. Point imported
  UI preferences/caches at Gator-owned paths; do not migrate user content into
  OpenCode's storage or implicitly import its credentials, plugins, or config.
- [ ] Migrate executable paths for background checks/start-at-login through the
  current reviewed ownership mechanisms. Preserve definitions and paused state;
  migration must not install services, start checks, or submit work implicitly.

Done when existing-profile fixtures open correctly through the CLI and can still
be opened by the retained desktop reference during the migration.

### Step 9 Build and distribute a complete CLI

- [ ] Package the Go executable, OpenCode-derived frontend, shared host, chosen
  Bun/Node runtime or documented prerequisites, OpenTUI native assets, and notices.
  Resolve workspace/catalog dependencies into the Gator build and lockfile. Verify
  launch outside both repositories with development dependencies absent.
- [ ] Use the Windows console subsystem for the public CLI; retain appropriate
  hidden/background-worker behavior. Preserve architecture checks, native private
  storage, process locking, and documented OS capability boundaries.
- [ ] Add CLI install/uninstall and explicit update/version workflows appropriate
  to the distribution. Preserve reviewed lifecycle guards, draft flushes, profile
  ownership, and non-replay guarantees. Match the baseline's actual supported
  update behavior rather than assuming signing accounts are configured.
- [ ] Add CLI builds and tests to CI for the current platform/architecture matrix.
  Check OpenTUI/native asset loading as well as Go compilation. Keep desktop checks
  during the port. Update source-checkout instructions without prematurely
  presenting the terminal as feature-complete.

Done when the terminal artifact runs independently and the replacement release
configuration is reviewable. Publishing, tagging, or changing release accounts
is separate from implementing and testing the local migration.

### Step 10 Verify parity and make the CLI primary

- [ ] Complete every acceptance case in the ledger. Map every desktop integration
  journey to terminal/host/native evidence; port the assertions about behavior
  before retiring their Electron UI tests.
- [ ] Run Go tests/vet, shared-host tests, terminal typechecks/tests/build, and real
  pseudo-terminal integration tests. Exercise actual Go state with local scripted
  providers, isolated profiles, restart/crash scenarios, and clean process exits.
- [ ] Run upstream-derived UI regressions and Gator adapter contract tests. Verify
  the final package neither starts OpenCode's agent/server nor routes Gator work,
  credentials, storage, sharing, or updates through upstream defaults.
- [ ] Compare representative workflows with the desktop baseline. Check process
  cleanup, startup, long-chat responsiveness, and complete process-tree memory;
  keep performance claims tied to measured workloads.
- [ ] Test the installed artifact from a temporary copy with the `desktop/` tree
  absent and Electron unavailable. Native/manual requirements stay pending until
  verified; skipped tests and cross-builds do not establish runtime parity.
- [ ] Write the parity report and switch `make run`, supported public entry points,
  primary documentation, and release wiring to the terminal application.

The removal gate passes only when all required parity cases pass, the independent
CLI build/install path works, existing profiles are preserved, and no runtime or
build dependency on Electron remains in the terminal product. The frontend must
use the recorded OpenCode source/components through the verified Gator adapter;
an old-renderer reskin or an OpenCode backend substitution does not meet the target.
Any proposed scope reduction needs an explicit user decision; it cannot be
recorded as a passing test.

### Step 11 Remove Electron and verify the final repository

- [ ] Move any remaining reusable code, tests, fixtures, assets, notices, and
  protocol definitions out of `desktop/` before removing that directory.
- [ ] Remove Electron/React DOM frontend code, preload/IPC/window code, desktop
  packaging and updater code, unused dependencies, desktop-only scripts/assets,
  generated outputs, and obsolete CI/release jobs. Retain shared Office/archive
  dependencies and notices where the terminal still uses them. Retain OpenCode,
  Solid, and OpenTUI provenance and applicable notices in the terminal distribution.
- [ ] Audit references by meaning. Keep `internal/desktop` computer-control tools,
  the separate browser/Playwright runtime, backend services, and useful historical
  documentation. A matching word or package name alone is not a deletion rule.
- [ ] Refresh installation/help/development/architecture/platform documentation,
  preserve historically accurate audit records, and remove stale active claims
  that only the Electron application is supported.
- [ ] Repeat clean-install, build, full required tests, profile/restart migration,
  package-content, and no-Electron checks after deletion. Update the ledger with
  final commit/build evidence and unresolved platform limitations.

Done when the repository builds and runs the complete terminal product without
the Electron codebase and every required parity case still passes. A later goal
covering this full plan authorizes removal after the gate; it does not require a
second permission request solely to perform the planned cleanup.

## Verification sources to carry forward

- Host unit tests currently under `desktop/tests/`: backend/session ownership,
  drafts, uploads, policy, files, Office round trips, comparisons, voice, terminal
  handoffs, profile paths, and update guards.
- Integration journeys currently under `desktop/tests/e2e/`: first task,
  conversations/catalog/search, concurrent chats, recovery, attachments,
  inline files/versions, memory/privacy, updates/check health, receipts,
  connections, app controls/voice, data/recovery, and current review semantics.
- Current Go tests in `internal/workapp`, `internal/worksession`, `internal/workrun`,
  and associated stores/services, plus bridge/assembly tests in `cmd/gator`.
- Relevant tests from the pinned OpenCode frontend and support packages, with a
  record of adapted fixtures and Gator-specific adapter tests. Upstream tests
  passing against OpenCode's own services do not establish Gator integration.
- [File editing boundaries](INLINE-FILES-RESEARCH.md),
  [app controls and voice](APP-CONTROL.md),
  [recovery and private data](RECOVERY_USAGE_DATA.md),
  [release lifecycle](DESKTOP-RELEASES.md), and
  [Windows support](WINDOWS.md), reconciled with live source and tests.

Keep the plan's checkboxes and the verification ledger current after each coherent
implementation step. Record commands and outcomes rather than marking a feature
complete from code inspection alone. Leave a clear next action when work spans
multiple goal turns.

## Suggested goal instruction

```text
Execute docs/TERMINAL-MIGRATION-PLAN.md. Replace Gator's Electron application
with an adaptation of OpenCode's actual terminal frontend, using its pinned
Solid/OpenTUI source and components through a Gator-specific frontend adapter.
Keep Gator's current Go backend authoritative and preserve every required
capability and existing user data. Build a conversation-first terminal workspace
with Gator's Chats, Following, Saved, Settings, and file workflows. Maintain
upstream provenance, licenses, and a reproducible runtime/build configuration.
Keep Electron available as the reference until the documented feature parity
gate passes, then remove its frontend, host, dependencies, packaging, and obsolete
wiring. Maintain the plan and verification ledger as you work. Do not count
stubs, inaccessible features, skipped checks, an old-renderer reskin, an OpenCode
backend substitution, or undocumented scope reductions as completion. Finish by
verifying the installed CLI works without Electron or an OpenCode agent/server
and reporting the evidence and any remaining platform limitations.
```
