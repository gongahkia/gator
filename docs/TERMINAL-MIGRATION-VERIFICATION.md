# Terminal migration verification

Started 26 October 2026. **Parity gate: OPEN. Electron removal is not authorized
by evidence yet.** This ledger records observed results, not inferred parity.

## Baseline and isolation

- Implementation starts at `912a7e7ac17f2c14ebb1aeb516b738eccf4e96c1`.
- Pre-existing `desktop/licenses/DEPENDENCIES.txt` replaces a Darwin canvas notice
  with the local Linux notice. Preserve this edit outside migration commits.
- Existing changes to the migration plan and development pointer belong to this
  migration and are captured with this ledger.
- Use temporary `GATOR_STATE_DIR`, `GATOR_CONFIG_DIR`, `GATOR_DESKTOP_USER_DATA`,
  workspace, and draft roots. Never use the live profile for fixtures.
- Host: Linux x64, Node 24.21.0, Go 1.26.10, Bun 1.4.2 initially installed.
- Native Windows and macOS evidence must come from those hosts; cross-builds
  and Linux tests do not establish native runtime parity.

## Commands and results

| Command | Result |
| --- | --- |
| `npm test --prefix desktop` | 69 cases: 68 pass, 1 native Windows case skipped on Linux; 0 failures. Baseline only. |
| `go test ./...` | Initial run exhausted 2.5 GiB `/tmp`. Rerun with workspace-backed TMPDIR/GOTMPDIR and `-p 2`: all packages except cmd/gator pass; existing installer-progress test failed (see below). |
| `npm run check --prefix desktop` | Pass before and after service extraction. |
| `npm run build --prefix desktop` | Pass before extraction; extracted frontend build also passes. |

## Required acceptance groups

Each group requires success, applicable cancellation, error, stale identity,
navigation, and restart evidence. A host unit test by itself does not establish
keyboard accessibility. Native/manual cases remain pending until exercised.

| ID | Capability | Status | Acceptance scope |
| --- | --- | --- | --- |
| P01 | Shell and onboarding | Pending | Welcome, first-task guidance, setup without lost drafts, slash discovery, help, multiline input, history, search, scrolling, theme, and status display work from the keyboard. |
| P02 | Conversations and workspaces | Pending | New/reopened chats, last selection, selected revision ancestry, workspace selection, source refresh, and per-chat options retain their current semantics. Launching in a directory does not silently widen a restored chat's source access. |
| P03 | Independent running chats | Pending | Switch between chats while their tasks continue; retain correct activity, unread, approval, queue, and error ownership, with current admission/process limits. |
| P04 | Model and effort controls | Pending | Select actual configured models and effort, preserve drafts, respect active-work restrictions, and capture the selected options with queued/admitted messages. |
| P05 | Transcript and activity | Pending | Stream replies and public work activity; search current/older retained text, inspect artifacts and results, copy content, and keep the reader's position when new activity arrives. |
| P06 | Admission and recovery | Pending | Preserve text/files until confirmed admission; edit/reorder/remove/resume queues; distinguish rejection, unconfirmed send, interruption, and completion. Restart leaves restored work paused and never automatically repeats an external action. |
| P07 | Approvals and action receipts | Pending | Show the exact review and bind decisions to the correct live run/token. Inspect connected requests, uncertain sends, reconciliation, and retained receipts without confusing review, read, retry, and approval. |
| P08 | Conversation catalog | Pending | Search indexed older chats; page results; pin, archive, rename, and organize supported records with current identity/version checks; persist organization across restart. |
| P09 | Personal memory and saved knowledge | Pending | Browse, filter, inspect, correct, and forget supported memory/notes/guidance; retain provenance, guarded edits, cross-chat recall, learning preferences, and earlier-chat privacy. |
| P10 | Context and connections | Pending | Inspect sources, supplied memory, outputs, and selected connections; change future selections without changing captured queues or treating historical provenance as current authority. |
| P11 | Goals and recurring checks | Pending | Create/manage goals and plans, configure/review checks, inspect timing/scope/health, and distinguish a saved goal from a running recurring check. |
| P12 | Updates and attention | Pending | Following, Inbox, quiet-check health, origin-linked chat updates, pagination, unread/read/done receipts, replies, and usefulness feedback retain their independent meanings. |
| P13 | Composer and file drafts | Pending | Keep per-chat drafts and attachments through navigation/restart; browse/recover/remove drafts; respect disabled retention; recover independent file edit drafts. |
| P14 | Attachments | Pending | Select/import/remove supported files, including files-only messages and clipboard images; preserve private immutable copies and current model intake. Current limits: 12 inputs, documents 8 MiB each, images 4 MiB each, 16 MiB combined. |
| P15 | Text and tabular editing | Pending | Open full text/Markdown/code/JSON and CSV/TSV; edit supported content/cells, validate where applicable, preserve supported formatting/quoting, and explicitly save/discard. |
| P16 | Native Office content editing | Pending | Inspect/edit the supported DOCX paragraph/table/run content, XLSX sheets/cells/formulas, and PPTX existing text. Preserve package parts and existing protected/signed/macro restrictions and rendering limits. |
| P17 | Image and PDF previews | Pending | Inspect actual images and PDF pages through supported terminal/native adapters, navigate pages, and return to the same chat/draft. Do not replace visual inspection with filenames or extracted text alone. |
| P18 | File versions and agent handoff | Pending | Save immutable working versions, inspect/compare/restore versions, export exact selected bytes, and stage Ask Gator with the saved version and supported selection. Preserve originals and sealed outputs; reject stale saves. |
| P19 | Provider and local model setup | Pending | Configure credentials/endpoints/custom providers; inspect/download/select/rename/remove local models and view installation progress. Preserve masking, readiness checks, and explicit reviews. |
| P20 | Account and tool integrations | Pending | Complete supported browser/device/API-key sign-in, verification, reconnect/disconnect, and reviewed external editor/install/tool handoffs. Preserve current browser, computer-control, connector, and specialist capabilities. |
| P21 | Voice | Pending | Explicitly start/stop/cancel dictation, review text in the original draft, account for provider usage, and optionally speak only new replies. Preserve permission, cancellation, conversation-binding, and transient-audio rules. |
| P22 | Usage and private data | Pending | Inspect/change limits and pause/resume, retention previews, deletion reviews, backup/export/verify/restore/relink, and recovery. Preserve cross-process guards and supported backup contents. |
| P23 | Preferences and app control | Pending | Inspect/change applicable settings through both terminal pages and conversation; keep theme, usable shortcut customization, privacy, voice, and reviewed app mutations consistent. |
| P24 | Profile and service continuity | Pending | Reuse conversations, knowledge, credentials, file versions, usage, drafts, and profile overrides; preserve reviewed background service start/stop/login behavior after executable paths change. |
| P25 | Distribution and lifecycle | Pending | Develop/install/launch/update/uninstall the CLI with documented dependencies; handle reconnect, quit, crashes, and updates without losing drafts or replaying requests. Retain platform capability disclosures. |

## Desktop journeys to carry forward

These existing tests define individual acceptance cases. Port their behavioral
assertions to terminal/host/native tests before retiring any Electron suite.

### `desktop/tests/e2e/activity.spec.ts`

- [ ] public activity interleaves, folds above the answer, and preserves the reader
- [ ] real runner narration and tools survive a desktop restart without replaying work

### `desktop/tests/e2e/app-control.spec.ts`

- [ ] chat changes shortcuts, model selection, limits, retention and selected connections
- [ ] chat opens a data deletion review without deleting content
- [ ] dictation preserves the draft, records voice usage and reads only new replies
- [ ] composer model picker preserves drafts, changes actual defaults and honors active-work locks

### `desktop/tests/e2e/appearance.spec.ts`

- [ ] light appearance covers chat, settings, palette and native chrome across restart and cached chats

### `desktop/tests/e2e/attachments.spec.ts`

- [ ] attachments stay with drafts and queued messages and reach the real model only on send
- [ ] attachment cancellation, removal and validation failures preserve the unsent message

### `desktop/tests/e2e/chat-check-health.spec.ts`

- [ ] chat health reports offline checks and observed gaps only in their owning chat

### `desktop/tests/e2e/chat-search.spec.ts`

- [ ] dated chat search highlights literal matches and opens the intended topic by keyboard

### `desktop/tests/e2e/chat-updates.spec.ts`

- [ ] failed and interrupted checks remain explicit after reading or marking done
- [ ] update feedback waits for host acknowledgement, rejects stale edits and survives restart
- [ ] durable follow-up cards return to their chat and retain explicit reply ownership

### `desktop/tests/e2e/concurrent-chats.spec.ts`

- [ ] independent chats retain task ownership, drafts and queues while browsing and completing elsewhere

### `desktop/tests/e2e/connected-receipts.spec.ts`

- [ ] uncertain sends have owned receipts and read-only Sent checks retained across restart
- [ ] a crashed dispatch reopens only in its own chat and never replays the request

### `desktop/tests/e2e/connections.spec.ts`

- [ ] native account setup reviews credentials, verifies access and returns to browser sign-in

### `desktop/tests/e2e/conversation-catalog.spec.ts`

- [ ] indexed history searches old messages and retains reversible organization across restart

### `desktop/tests/e2e/desktop.spec.ts`

- [ ] relative profile overrides keep the existing platform interpretation
- [ ] About & updates explains development builds and preserves an unsent message
- [ ] unsent drafts survive reload and reconnect without submission, and retention can be cleared
- [ ] long conversations retain context while searching and signal activity without moving the reader
- [ ] recent chats default to a flat list, offer folder grouping, and isolate drafts
- [ ] motion is interruptible, releases dismissed controls, and honors reduced motion
- [ ] queued messages can be edited, reordered, removed, and resumed without losing drafts
- [ ] desktop uses real Go state, preserves drafts, edits native forms, and continues saved work
- [ ] the conversational home has no dashboard and usage controls keep the Go review flow
- [ ] review presentation preserves exact previews and only the live approval action
- [ ] first-run model setup uses native fields and keeps credentials out of display state

### `desktop/tests/e2e/files.spec.ts`

- [ ] contextual files reuse retained previews and show the exact origin in chat

### `desktop/tests/e2e/first-task.spec.ts`

- [ ] first task keeps a file and draft through model setup and sends only on request

### `desktop/tests/e2e/followup-chat.spec.ts`

- [ ] chat reviews, saves, edits and pauses an origin-linked responsibility

### `desktop/tests/e2e/inline-files.spec.ts`

- [ ] inline text editing saves full versions, recovers drafts and sends the edited bytes
- [ ] Word Excel PowerPoint and CSV edit inline and export native files with unchanged originals
- [ ] PDF pages and images render inline without enabling edits or network requests
- [ ] generated outputs edit in their message and send the working version without changing sealed output
- [ ] saved file versions compare inline and export the exact chosen bytes without a model call
- [ ] larger attachment selections survive reload, reject overflow and reach the local model intact

### `desktop/tests/e2e/polish.spec.ts`

- [ ] Inbox opens without acknowledging results, and explicit read and rating actions persist
- [ ] knowledge and model filters preserve the selected record and original model identity
- [ ] native follow-up fields review exact values and path selection never switches workspace
- [ ] saved guidance persists native choices and retired terminal settings are absent
- [ ] the desktop starts without a standalone Node or Gator CLI on PATH
- [ ] text fields use quiet focus across search, forms and inline editors

### `desktop/tests/e2e/request-recovery.spec.ts`

- [ ] admitted text and attachments survive session loss and resume only by explicit selection
- [ ] a rejected admission keeps the composer draft and starts no request

### `desktop/tests/e2e/shared-chats.spec.ts`

- [ ] new chats share grounded personal memory, keep separate transcripts and reopen the last draft
- [ ] native image paste stages a preview, retains drafts per chat, and sends only after review
- [ ] new chat and pasted attachments keep focus and layout across themes and compact windows
- [ ] earlier-chat privacy is independent of learning and persists across restart
- [ ] saved-memory receipts support guarded edits, restart and forgetting inline
- [ ] memory edits survive concurrent updates and removal without overwriting saved facts

### `desktop/tests/e2e/work-details.spec.ts`

- [ ] compact work details preserve conversation state and distinguish selections, evidence, and output checks

## Native adapter and distribution evidence

Pending: image/PDF visual inspection, clipboard image intake, microphone
permission/capture/cancellation and spoken output on claimed platforms;
independent installed launch; release packaging and platform matrix.

## Work log

- Baseline inventory: existing Go stdio bridge owns workflow state; the desktop
  `Sessions` host owns up to four open runtimes and three active chats. Shared
  services are currently Electron-independent modules next to the Electron main
  process. Extract them without changing protocol or file formats.
- Downloaded the pinned OpenCode snapshot into `/tmp/gator-migration/` for source
  audit. Its actual frontend is Solid/OpenTUI 0.6.8; client/data, configuration,
  plugins, and execution routes require explicit adaptation/exclusion.

### Shared service extraction

- Moved Electron-independent application services, contracts, behavioral tests,
  and Office fixtures to `app-host/`. Electron consumes a local package dependency;
  its renderer keeps forwarding type modules to avoid changing its UI behavior.
- `npm test --prefix desktop`: 7 desktop-only tests pass; 62 shared host cases,
  61 pass and the native Windows handoff skipped. No failures after fixture move.
- Desktop typecheck and Vite build pass. Desktop e2e suite is running under Xvfb;
  initial public activity layout case failed its positive scroll-offset assertion,
  while actual runner/restart, voice, app-control and model cases pass so far.
- Baseline Go failure: `TestStreamOllamaInstallerReportsOutputInProgressOrder`
  observed only the initial installer line. No Go changes have been made.

### OpenCode-derived shell and first runtime journey

- Added `ui/UPSTREAM.md`, pristine upstream source hashes/license, adapted
  Solid/OpenTUI dialogs, keymap, palette, session frame/rail, composer, and Gator
  client/data contexts. Upstream execution/server/config/plugin dependencies are
  excluded. Bun 1.4.2 + OpenTUI 0.6.8 + patched Solid 1.9.15 work on this host.
- Added Electron-independent `Application` composition and frontend contracts.
  Host validation, admission, session ownership and native file services remain
  the same implementations used by the desktop reference.
- `bun run --cwd ui check`: pass. `bun run --cwd ui build`: pass.
- `bun run --cwd ui test`: 4 pass, 0 fail, 41 assertions, including actual Go
  state and a local scripted provider through the native renderer's keyboard
  harness. Unicode/multiline paste does not send; Enter admits; one user message
  is retained; the palette and narrow layout work; restart restores the draft
  and leaves provider request count unchanged. No upstream agent/server runs.
- Real PTY smoke: startup reaches connected New chat; Ctrl+Q exits zero and
  restores raw/alternate-screen modes while stopping the owned Go process.
- `make terminal` is an experimental development entry point. The installed
  artifact, approval slice, complete pages/files/native adapters and parity gate
  remain pending. This is not a feature-complete replacement yet.
- Go installer-progress baseline failure passes three isolated repetitions.
- Initial Xvfb desktop integration run: 42 pass, 12 fail. It overlapped service
  extraction, so it cannot distinguish baseline failures from extraction effects.
  One failure resolves an old moved module in cached test code; others include
  switching/timing/layout/native clipboard conditions. Rerun after extraction
  before using those results as parity evidence. Desktop remains available.

### Native services and rendered file pages

- Added shared PDF rasterization with the existing PDF.js/native canvas family,
  bounded page dimensions and output. Actual two-page PDF fixture renders distinct
  PNG pages; nonexistent pages are rejected. This proves rendered pixels, not a
  filename or extracted-text substitute. Terminal image display/native-host checks
  remain separate acceptance cases.
- Added bounded native helpers for links, clipboard text/images, PCM microphone
  recording through ffmpeg, and OS speech. These are explicit operations with
  literal argv and visible helper/permission errors. Windows/macOS device and OS
  permission behavior is not yet verified here. No real microphone was activated.
- Added conversation-bound voice orchestration using the existing Go speech
  transport/provider route and accounting. Injected capture tests cover explicit
  start/stop, cancellation on chat change, no historical-reply playback, cleanup,
  and captured-route transcription. They do not establish hardware parity.
- `npm test --prefix app-host`: 66 cases, 65 pass, one native Windows handoff
  skipped, zero failures. Native helper tests exercise real process argv, output
  limits and cancellation. Existing Office/file/session/approval validation tests
  remain passing.
- Concurrent workspace commit `3c2bb343b` appeared during implementation. It
  includes the pre-existing notice edit and early hub/client changes. Preserved
  its history and continued from it without resetting or amending it.

### Terminal workspace actions and file workbench

- Added keyboard-accessible Following, Saved and Settings hubs, typed panel
  controls, model/effort, queue management, draft retention/recovery, guarded
  memory corrections, update feedback, receipt inspection, and recent/open chats.
  Async actions wait for the matching request nonce and reject chat/disconnect
  changes; opening a hub does not submit agent work or acknowledge updates.
- Added full-buffer text editing, Office paragraph/table/slide and worksheet cell
  selection, immutable version history/comparison/restoration/export/Ask Gator,
  actual terminal image/PDF display, clipboard intake, explicit voice controls,
  and terminal suspend/resume for Go-produced reviewed external steps.
- Preserved exact approval preview JSON in the transcript. Configured desktop
  shortcuts are translated to terminal Ctrl equivalents with Ctrl+P, Ctrl+J,
  slash and function-key fallbacks. Native key/OS permission claims remain pending.
- `bun run --cwd ui check`: pass. `bun run --cwd ui test`: 6 pass, zero failures,
  59 assertions. Real-runtime renderer tests now open text and all three Office
  formats from the keyboard, save a text version, preserve original bytes, and
  stage the exact selected saved version without sending it. Office native exports
  are produced through the same shared content services; existing package-part
  round-trip assertions remain in the host suite.
- File tests exposed the workbook address delimiter and formula-prefix boundary;
  corrected the frontend to preserve `sheet!A1` identities and leading `=` when
  editing existing formulas. Keyboard tests also caught terminal comma binding
  normalization and asynchronous picker focus handling.
- User confirmed macOS/Windows hosts exist and native runtime verification will
  happen later. Continue implementation and Linux evidence; keep those cases and
  Electron removal pending until the parity gate has actual native results.
