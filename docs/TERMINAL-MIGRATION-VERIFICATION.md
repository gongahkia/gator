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
| P08 | Conversation catalog | Pass on Linux; native hosts pending | Search indexed older chats; page results; pin, archive, rename, and organize supported records with current identity/version checks; persist organization across restart. |
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
- [x] real runner narration and tools survive a desktop restart without replaying work (terminal: `tests/activity.test.tsx`)

### `desktop/tests/e2e/app-control.spec.ts`

- [x] chat changes shortcuts, model selection, limits, retention and selected connections (terminal: `tests/app-control.test.tsx`)
- [x] chat opens a data deletion review without deleting content (terminal: `tests/app-control.test.tsx`)
- [ ] dictation preserves the draft, records voice usage and reads only new replies
- [ ] composer model picker preserves drafts, changes actual defaults and honors active-work locks

### `desktop/tests/e2e/appearance.spec.ts`

- [x] light appearance covers chat, settings and palette across restart and cached chats — `ui/tests/appearance.test.tsx`; Electron window chrome is replaced by terminal-rendered surfaces. Native terminal application chrome is owned by the user's terminal.

### `desktop/tests/e2e/attachments.spec.ts`

- [ ] attachments stay with drafts and queued messages and reach the real model only on send
- [x] attachment cancellation, removal and validation failures preserve the unsent message (terminal: `tests/attachments.test.tsx`; picker replaces desktop drag/drop)

### `desktop/tests/e2e/chat-check-health.spec.ts`

- [x] chat health reports offline checks and observed gaps only in their owning chat — `ui/tests/updates.test.tsx` on Linux.

### `desktop/tests/e2e/chat-search.spec.ts`

- [ ] dated chat search highlights literal matches and opens the intended topic by keyboard

### `desktop/tests/e2e/chat-updates.spec.ts`

- [x] failed and interrupted checks remain explicit after reading or marking done — `ui/tests/updates.test.tsx`.
- [x] update feedback waits for host acknowledgement, rejects stale edits and survives restart — `ui/tests/updates.test.tsx`.
- [x] durable follow-up cards return to their chat and retain explicit reply ownership — `ui/tests/updates.test.tsx`.

### `desktop/tests/e2e/concurrent-chats.spec.ts`

- [x] independent chats retain task ownership, drafts and queues while browsing and completing elsewhere — `ui/tests/concurrent.test.tsx`; failure/recovery cases remain separate.

### `desktop/tests/e2e/connected-receipts.spec.ts`

- [x] uncertain sends have owned receipts and read-only Sent checks retained across restart — `ui/tests/receipts.test.tsx`.
- [x] a crashed dispatch reopens only in its own chat and never replays the request — `ui/tests/receipts.test.tsx` on Unix; native Windows crash harness remains pending.

### `desktop/tests/e2e/connections.spec.ts`

- [x] native account setup reviews credentials, verifies access and returns to browser sign-in — `ui/tests/connections.test.tsx` on Linux with a local OAuth server and an owned browser-helper fixture; actual native browser permissions remain pending.

### `desktop/tests/e2e/conversation-catalog.spec.ts`

- [x] indexed history searches old messages and retains reversible organization across restart — `ui/tests/catalog.test.tsx` on Linux.

### `desktop/tests/e2e/desktop.spec.ts`

- [ ] relative profile overrides keep the existing platform interpretation
- [ ] About & updates explains development builds and preserves an unsent message
- [ ] unsent drafts survive reload and reconnect without submission, and retention can be cleared
- [ ] long conversations retain context while searching and signal activity without moving the reader
- [ ] recent chats default to a flat list, offer folder grouping, and isolate drafts
- [ ] motion is interruptible, releases dismissed controls, and honors reduced motion
- [x] queued messages can be edited, reordered, removed, and resumed without losing drafts (terminal: `tests/recovery.test.tsx`)
- [ ] desktop uses real Go state, preserves drafts, edits native forms, and continues saved work
- [ ] the conversational home has no dashboard and usage controls keep the Go review flow
- [ ] review presentation preserves exact previews and only the live approval action
- [ ] first-run model setup uses native fields and keeps credentials out of display state

### `desktop/tests/e2e/files.spec.ts`

- [ ] contextual files reuse retained previews and show the exact origin in chat

### `desktop/tests/e2e/first-task.spec.ts`

- [x] first task keeps a file and draft through model setup and sends only on request — `ui/tests/models.test.tsx` with real Go, immutable intake, restart and a local provider.

### `desktop/tests/e2e/followup-chat.spec.ts`

- [x] chat reviews, saves, edits and pauses an origin-linked responsibility — `ui/tests/followup-chat.test.tsx`, including quiet feedback; typed end-date review in `ui/tests/pages.test.tsx`.

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

- [x] admitted text and attachments survive session loss and resume only by explicit selection — `ui/tests/recovery.test.tsx`.
- [ ] a rejected admission keeps the composer draft and starts no request

### `desktop/tests/e2e/shared-chats.spec.ts`

- [x] new chats share grounded personal memory, keep separate transcripts and reopen the last draft (terminal: `tests/memory-privacy.test.tsx`)
- [ ] native image paste stages a preview, retains drafts per chat, and sends only after review
- [ ] new chat and pasted attachments keep focus and layout across themes and compact windows
- [x] earlier-chat privacy is independent of learning and persists across restart (terminal: `tests/memory-privacy.test.tsx`)
- [x] saved-memory receipts support guarded edits, restart and forgetting inline — `ui/tests/memory.test.tsx`, including original-save undo.
- [ ] memory edits survive concurrent updates and removal without overwriting saved facts

### `desktop/tests/e2e/work-details.spec.ts`

- [ ] compact work details preserve conversation state and distinguish selections, evidence, and output checks

## Native adapter and distribution evidence

Linux native-renderer image/PDF inspection and independent installed launch have
concrete evidence below. Still pending: actual native clipboard/device permissions,
microphone capture and spoken output; native macOS/Windows runtime checks and the
remaining release/platform matrix. A passing fixture does not verify hardware.

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

### Adapter ownership and lifecycle hardening

- Fixed restored draft records carrying disk-only `updated` metadata into the
  strict save contract. Restored drafts can now be edited and saved again.
- Attachment and clipboard image intake merge into the latest draft after async
  import, preserving text typed while intake runs. Per-chat draft loads retain
  their captured identity and cannot overwrite newer in-memory input.
- Matching result listeners ignore old/duplicate nonces, reject chat changes,
  and cancel on shutdown. A refused close (for example, an external step or failed
  draft flush) retains the live host subscription; concurrent closes share cleanup.
- `bun test --cwd ui --preload ./scripts/preload.ts tests/client.test.ts`:
  **5 pass, 0 fail, 22 assertions**. Cases cover retained draft editing, delayed
  loading/import, newer input during admission, rejected admission, result nonce
  and owner, duplicate events, refused close and subscription cleanup.
- Concurrent workspace commit `3ebd27eeb` captured packaging/install/CI work and
  path/theme/approval followups. Preserved that commit and reconciled its contents.
  The preceding full UI run passed **7 tests / 68 assertions**, including real Go
  approval: stale tokens and malicious paste cannot approve, while the explicit
  live decision sends exactly once. New changes continue to require full checks.

### Credential input and form review

- Credentials now stay in a transient backing value; only mask glyphs enter the
  native renderable buffer. Matching foreground/background colors did not provide
  this guarantee. Typing, literal bracketed paste, backspace and clearing preserve
  explicit field confirmation and do not submit the provider form.
- Normalized modified Enter to Go's `ctrl+enter` identity and added visible **F2**
  review fallback for terminals that cannot distinguish Ctrl+Enter from Enter.
- `bun test --cwd ui --preload ./scripts/preload.ts tests/panel.test.tsx`:
  **1 pass, 9 assertions**. Both typed and pasted credentials are absent from
  captured character frames; paste submits nothing; the explicit review sends
  the selected field value with the expected backend key.

### Conversation controls and retained reading position

- Replaced the legacy Ctrl+G external-editor request with the current task-bound
  queue guidance operation. Text clears only after the matching Go acknowledgement;
  attachments/update replies stay queued-message workflows. An explicitly retried
  uncertain admission reuses its prior request identity for Go journal deduplication.
- First-task send opens model setup without losing text/files. Local return from
  a hub does not send `/chat` to Go. Linked-update reply state is visible and can
  be explicitly cancelled. About reads the actual bundled runtime build version.
- Retained evidence now has a read-only scrollable viewer. Transcript search
  reveals the chosen dated message. Reading positions survive activity and hub
  navigation; restoration waits for native layout rather than an early timer.
- `tests/transcript.test.tsx`: **1 pass / 9 assertions** with a 60-message native
  renderer fixture, keyboard scrolling/search, incoming activity and hub roundtrip.
- `tests/concurrent.test.tsx`: **1 pass / 21 assertions** using two real Go runtimes
  and a gated local provider. Covers background execution, separate drafts,
  keyboard chat/catalog switching, queue ownership, live guidance, completion
  attention, durable completed journals and restart without repeated model calls.
- Adapter tests expanded to **7 pass / 35 assertions** for explicit retry identity,
  guidance acknowledgement, update reply cancellation and first-model setup.
  Terminal runtime tests now default to `ui/runtime`, with no desktop-tree path.
- Full frontend validation at this checkpoint: typecheck and production build
  pass; **17 tests, 143 assertions, 0 failures**. These tests establish the named
  journeys, not all P01–P25 acceptance groups.

### Independent distribution and baseline reconciliation

- The full workspace-backed `go test -p 2 ./...` rerun and `go vet ./...` pass.
  No Go source changes have been made. The earlier installer-progress failure
  did not reproduce in three focused repeats or this complete run.
- Production artifact `/home/gongahkia/.cache/gator-migration/artifact-2` passed
  `python3 ui/scripts/verify-package.py ARTIFACT`: actual PTY, fresh cwd outside
  checkout, PATH containing only Bun, connected Go, literal Unicode/bracketed
  paste, command palette, retained draft, clean exit. It contains neither the
  desktop tree, Electron nor OpenCode core/server. This is an earlier checkpoint
  artifact; rebuild and repeat against final code before claiming final parity.
- `node ui/scripts/verify-install.mjs ARTIFACT` passes for artifact-2: independent
  install/version/scripted commands, canonical active-profile guard refusing
  uninstall, owned uninstall after release, and preserved private note data.
- Production launch required OpenTUI's Solid resolver preload before importing
  compiled UI. Added it to the public launcher and proved it through the PTY
  test. A first harness wait deadlocked on a full PTY; draining while awaiting
  exit fixed the harness. These were corrected, not counted as passes initially.
- The post-extraction Electron e2e run reached 52/54 cases before interruption
  and has **no final suite result**. Ten observed failures include outdated
  complementary/row selectors against current dialogs/catalogs, first-run and
  switching/animation/screenshot timing, native clipboard MIME failure under
  Xvfb, and close timeout. They remain open baseline checks. The initial mixed
  run's 42/12 split is not usable as a clean regression baseline.
- Native CI matrix wiring exists, but no remote run has been requested/performed.
  Linux evidence and cross-compilation do not replace deferred native results.

### Unsaved form navigation

- Typed form edits now require an explicit discard decision before hub/chat
  navigation, Escape, or the normal Quit action. Cancellation retains fields;
  backdrop dismissal also resolves the navigation as cancelled. Chat rail/recent
  navigation uses the same guard. Backend form submission remains explicit.
- `tests/panel.test.tsx`: **2 pass / 15 assertions**, including cancelled and
  confirmed keyboard navigation with an unsaved recurring-check form. The prior
  full run remains **17 pass / 143 assertions**; this adds one focused case.

### Existing-profile continuity

- `tests/profiles.test.ts`: **2 pass / 28 assertions** with real Go state. An
  existing-format Electron draft (including private upload references and disk
  metadata), file working version and independent file edit draft reopen through
  the terminal twice without rewriting the draft record. Original source bytes
  and existing saved knowledge remain intact. No conversion into upstream storage
  occurs; the frontend uses the established per-profile paths and formats.
- A second state/config profile sharing the same app user-data directory cannot
  read the first profile's drafts. Disabled retention keeps current-session input
  editable while writing no retained draft; restart stays empty and disabled.
- Relative overrides retain the existing workspace-relative Go interpretation
  on Unix. The Windows branch is present in the same fixture but remains native
  verification pending. This test does not establish backup/credential/service
  executable-path migration or cosmetic Electron-localStorage preference parity.

### Native file keyboard and visual evidence

- `tests/previews.test.tsx`: **1 pass / 11 assertions**. Under Bun, actual
  OpenTUI native images decode PNG pixels and two distinct PDF raster pages;
  Ctrl+Left/Right changes and restores the page bytes. The renderer exercises its
  raster-block fallback, not just labels. Returning preserves draft/attachments
  and creates no messages. Native terminal graphics protocols still need their
  claimed-terminal/native-host checks. Corrupt image data produces a visible
  decode error; the test now generates a valid PNG instead of an invalid old
  one-pixel fixture.
- PDF render completion is generation-bound so an earlier slow page cannot
  overwrite the currently selected page. Shared PDF/native focused tests pass
  **4/4** after extracting reusable fixtures.
- `tests/office-keyboard.test.tsx`: **1 pass / 13 assertions**. Real renderer
  keyboard journeys edit DOCX paragraphs, PPTX existing text, XLSX formulas and
  quoted CSV cells; save immutable versions and export the exact selected bytes.
  Independent content inspection confirms exports; original source files stay
  byte-identical. No model message is submitted.
- Office/cell dialog edits now retain directly through the host instead of
  scheduling autosave on an already-unmounted editor. Existing protected/signed/
  macro restrictions and package-preservation checks remain in the shared suite.

### Loaded list filters and selection

- Added frontend filters for loaded model/knowledge/picker rows, retaining each
  original Go row index. Catalog search still uses Go's indexed query path.
  Empty filtered lists cannot activate a hidden original record. Keyboard arrows
  and paging select visible original identities; native layout reveals selection
  in long panels.
- `tests/panel.test.tsx`: **3 pass / 22 assertions**, including filtering to the
  third original model row and preventing activation after an empty search.
  Typecheck passes. Full acceptance of every knowledge/catalog mutation remains
  separate from these rendering and selection cases.

### Reviewed forms and real control keys

- `tests/pages.test.tsx`: **1 pass / 54 assertions** against Go. Following,
  Saved, Settings and their model/connection/memory/usage/data/recovery pages keep
  the unsent draft. Keyboard recurring-check setup covers multiline outcome,
  directory selection, frequency/mode, a numeric budget, daily limit and UTC end
  date. Review shows exact values and writes no definition until explicit save.
  Save links the original chat, intentionally selects the new goal as Go does,
  and returning restores the original chat's draft and source.
- Corrected zero-valued omitted `selected` to mean the first row. Corrected the
  form review fallback to emit canonical **`ctrl+r`**, rather than an unsupported
  renderer `ctrl+enter` frame. F2 and distinguishable Ctrl+Enter invoke the same
  review, without weakening shared host validation.
- Ctrl+C stop now sends canonical **`esc`**, which the existing host permits.
  `tests/cancellation.test.tsx` proves a real active request stops, the backend
  stays connected and the unsent draft survives. The earlier renderer-only key
  assertion did not establish compatibility with the strict Go host boundary.

### Guarded memory edit acknowledgements

- Action prompts keep their buffer and show acknowledgement progress until the
  original operation succeeds. A rejected/stale save retains the typed correction
  and visible error. Completion from an old prompt cannot clear a new screen.
- `tests/memory.test.tsx` passes against the real Go memory tool/store and a local
  provider: grounded save/provenance, concurrent external correction, stale-write
  refusal with retained editor text, successful correction, restart without model
  replay, reviewed forgetting, and undo of a new original save. The label now
  describes original-save undo rather than incorrectly promising correction undo.
- Full frontend checkpoint: **27 pass / 289 assertions / 0 failures**, with
  typecheck passing. Cases not explicitly mapped in this ledger remain open.

### Interrupted request recovery

- `tests/recovery.test.tsx`: **1 pass / 15 assertions** with an actual blocked
  Go request and two admitted queued requests. Closing and restarting retains all
  three paused requests and the independent unsent draft, without a provider call.
- Deleting the source attachment and aging its private upload does not prune the
  admitted copy. Keyboard review/resume of only that selected request sends the
  correct immutable bytes once; the other recovered requests stay paused.
  Removing the interrupted request and restarting preserves the remaining queue
  without replay. This is distinct from connected-action reconciliation evidence.

### Help, retained prompt text and conversation information

- F1 and `/help` use terminal-specific instructions and current Go commands.
  `/prompt-history` can inspect, append, or edit previously admitted message text
  into the current draft without replaying it. Conversation information separates
  future source/model/connection selections from captured result evidence.
- `tests/application.test.tsx` passes the real-runtime paste/admission/restart
  journey with keyboard help, retained text staging, unchanged provider-call count,
  and conversation-information display. Escape tests wait for the terminal's
  ambiguous ESC prefix to resolve before sending a subsequent function key.

### Spoken-reply cancellation

- Playback now stops on chat switch, preference disable, explicit stop, or close.
  Voice controls expose “Stop spoken reply” and a speaking indicator. Historical
  replies stay silent when returning to a chat.
- `node --test app-host/tests/native.test.mjs`: **5 pass, 0 failures**; the new
  case exercises playback cancellation across all three boundaries using abort
  signals. Frontend typecheck passes after refreshing the local host package.
  Actual speakers, microphone permissions and device behavior remain native cases.

### Indexed catalog organization

- Added F3 catalog actions for rename, pin/unpin and reviewed archive/restore.
  Requests capture the original Go panel/record identity, wait for the actual
  catalog mutation, and retain rejected rename text. Ctrl+R/F5 refresh and existing
  indexed search, scope filter and pagination remain separate operations.
- Catalog rows show dates, source, pin/archive state and literal highlighted
  snippets. Shared result observation now also supports cancellable owner-bound
  completion waits; existing nonce/admission/close contract tests still pass.
- `tests/catalog.test.tsx`: **1 pass / 12 assertions**, exercising 130 retained
  chats, three result pages, old-message indexed search, rename/pin/archive/
  restore, concurrent-version refusal, preserved revisions and restart. No model
  calls occur; the actual SQLite index is verified. Combined with client tests:
  **8 pass / 47 assertions**. Frontend typecheck passes.
- Shared-host full rerun: **67 cases, 66 pass, one native Windows handoff skipped,
  0 failures**. Desktop typecheck passes. The two previously interrupted desktop
  cases were rerun: concurrent memory editing passes; work-details fails an old
  complementary-role selector where the current narrow layout uses a dialog.
  These results do not convert the incomplete full desktop run into a pass.

### Saved file selections and large tables

- Table/workbook rendering uses at most 50 rows and 20 columns per page. Keyboard
  paging retains the per-file position; F7 switches worksheets. F3 can edit cells
  beyond the existing extent. Display truncation never truncates saved content.
- F6 selects cells or existing Office paragraphs for Ask Gator, including read-only
  content. Selections resolve from the chosen immutable version; spreadsheet
  formulas use that version's formula, rather than an old displayed value.
  Text quotes carry their revision and reject a different selected version.
  Unsaved edits must be saved before staging a version, as in the reference.
- Textarea initialization no longer manufactures an unsaved edit. The new save
  requirement exposed this issue in the existing text-file keyboard test.
- `bun run --cwd ui check`: pass. Focused native renderer suite (`files`, `grid`,
  `office-keyboard`): **4 pass / 39 assertions / 0 failures**. The large-table case
  pages a 125-by-45 CSV, selects AS125 and stages its actual value without sending;
  the Office case stages the newly saved formula. Original files remain unchanged.

### Reviewed export replacement

- Export supports an existing destination through a separate review displaying
  its absolute path, size and modification time. Escape cancels. Replacement
  grants are single-use, expire after ten minutes and verify the same regular
  file identity/metadata before writing; a changed destination is left intact.
  New exports retain exclusive creation. Linked files and Gator's private state/
  user-data storage are refused to protect retained inputs and sealed outputs.
- `node --test app-host/tests/export.test.mjs`: **2 pass**, including stale,
  replayed, wrong-path, changed-chat, linked-file and private-storage refusals.
- Focused terminal `export`, `files`, `office-keyboard`: **4 pass / 44 assertions**.
  The keyboard case cancels, refuses a concurrent edit, then exports exact bytes
  after a fresh review while preserving source, draft and provider-call count.
- Full shared-host suite: **69 cases / 68 pass / one native Windows skip / 0
  failures**. Terminal and desktop typechecks pass. Native replacement behavior
  remains part of the deferred macOS/Windows verification.

### Owned external editor and acknowledged handoffs

- `external-editor` prepares the existing Go composer-editor workflow through a
  dedicated host method. Renderer frames still cannot launch arbitrary commands
  or send `external_done`. Terminal handoffs execute only Go-produced argv and
  wait for Go's matching completion acknowledgement before restoring the scene.
- The original chat keeps returned text, attachments and reply ownership even
  when another chat becomes selected. Concurrent typing is retained alongside
  editor text with visible explanation. A nonzero editor exit retains its saved
  text and reports failure. Oversized text stays in an explicit recovery editor
  until shortened/imported or deliberately dismissed. Closing an unlaunched
  editor cleans its Go-owned temporary file; closing a running handoff is refused.
- Native renderer test uses actual terminal setup/suspend/resume and an actual
  Go-created editor process: success, resumed input, exit status 3, full oversized
  buffer recovery, chat switch, simultaneous edits, attachment preservation,
  temporary cleanup and restart without submission. Combined with client cases:
  **8 pass / 54 assertions**. Typecheck passes. The earlier full UI checkpoint
  passed **31 cases / 337 assertions** before the final cleanup/recovery additions.
- Shared host rerun: **69 cases / 68 pass / one Windows skip / 0 failures**;
  desktop typecheck passes. The editor case explicitly skips Windows because
  Go's existing editor command requires POSIX `sh`; macOS/Windows actual runtime
  evidence and real account/install handoffs remain pending.

### Independent artifact and real terminal editor

- Rebuilt Go/runtime and production frontend from `36f747076`; packaged into the
  fresh workspace-backed `artifact-3` directory. The artifact contains production
  dependencies and native assets, with no desktop tree or Electron/OpenCode core.
- `python3 ui/scripts/verify-package.py artifact-3`: pass from an isolated working
  directory with only Bun on PATH. Actual Go connection, Unicode/bracketed paste,
  palette, private retained draft and clean exit; original terminal modes restored.
- The new `--editor` variant passes with Bun and POSIX sh on PATH and a Python
  fixture editor. It verifies real canonical input and echo during the handoff,
  actual keyboard input reaching the child, exact returned text, deleted temporary
  file, resumed raw composer input and final terminal-mode restoration.
- `node ui/scripts/verify-install.mjs artifact-3`: pass for independent install,
  help/version, a scripted note, active-profile uninstall refusal, owned uninstall
  and retained private data. This checkpoint does not establish the remaining
  page/native parity cases or authorize Electron removal.

### Update attention, feedback, replies and check health

- Ported the reference timeline so durable cards interleave with retained chat
  messages and precede explicit replies, while keeping original message indexes.
  Failed/interrupted/limited checks retain their explanation after read/done;
  check-health details include current-setting success, timing, consecutive
  failures, supervisor gaps and the limits of those observations.
- Delivery is recorded only for cards intersecting the rendered viewport after
  scroll restoration, with no dialog or handoff obscuring them. Opening a hub or
  Inbox does not read results. An initial-frame ordering issue was caught by the
  offscreen-card assertion and fixed. Delivery/read/resolution remain independent.
- Feedback has visible acknowledgement progress and blocks leaving while pending.
  Its saved selection comes from Go's result; stale feedback and stale resolution
  changes surface errors. Rating does not modify checks automatically.
- Returning to Chat from a Go panel now closes that panel through canonical Go
  navigation, after the unsaved-form guard. Local hub returns still preserve
  transcript position and drafts without sending a request.
- `bun run --cwd ui check`: pass. Focused `updates`, `transcript`, `client`:
  **10 pass / 96 assertions / 0 failures**. Actual Go/local-provider cases cover
  three result states, owned attention, exact evidence, stale writes, delayed
  renderer acknowledgements, restart, durable reply context, deduplicated replay,
  53-update paging, offscreen delivery and owning-chat health. No external service
  or user's profile is used. Following/quiet-check management cases remain open.

### Connected receipts and crash recovery

- Added explicit recovered-receipt discovery in the transcript. `/receipts`
  inspects the original saved action without retrying it. Read-only Sent checks
  explain that a matching Sent record does not prove recipient delivery.
- `bun run --cwd ui check`: pass. `tests/receipts.test.tsx`: **2 pass / 25
  assertions**, using the real Go Gmail connector against a local HTTP fixture.
  An approved send with no provider receipt stays unknown. Inspecting its saved
  check makes no HTTP request; explicit Check Sent performs exactly two GETs,
  preserves the sealed manifest and survives restart without another send.
- The crash case kills only the single isolated Go child after its approved POST
  and executing receipt are proven. Restart exposes a non-live owned receipt and
  paused request, retains the independent draft, hides it in another chat and
  never repeats the POST. A receipt without a sealed manifest cannot offer a
  Sent check. The Unix process probe explicitly skips native Windows.

### Existing Chromium frontend preferences

- Concurrent user commits `724e1bd93` and `b9c11fb35` captured the preference store,
  terminal wiring, tests and refreshed dependency lock. These commits were kept.
- First use copies bounded LevelDB files into a private temporary directory and
  opens only the copy with pinned `classic-level 3.0.0`. Only the three named Gator
  display preferences are decoded; deleted keys, string encodings and latest
  values are respected. Original database bytes remain untouched and temporary
  copies are removed. A failed/corrupt import writes no completed migration file.
- Sidebar and recent-chat grouping choices apply in the terminal and persist
  through restart. Work-details disclosure choices remain stored; explicit
  terminal information menus replace those disclosures. Theme and shortcuts keep
  their existing Go authority; unrelated origins, keys and credentials are ignored.
- `app-host/tests/preferences.test.mjs`: **3 pass**, including idempotence,
  tombstones, extra-authority refusal, patch serialization and abandoned locks.
  Full host suite: **72 cases / 71 pass / one Windows skip / 0 failures**.
- Terminal preference test: **1 pass / 18 assertions** under Bun, including
  keyboard sidebar/grouping changes, separate chat drafts, restart and unchanged
  Chromium hashes. `node ui/scripts/verify-preference-migration.mjs` also passes
  against a profile created by the actual retained Electron application. Terminal
  and desktop typechecks pass. Native macOS/Windows import remains unverified.

### Production lazy dependencies and service verification

- Audit found stale Bun local-package metadata: the earlier artifact launched,
  but its lazy PDF adapter lacked native canvas/PDF.js. The new service probe
  rejects `artifact-3` with the actual missing-module error. Its earlier PTY and
  install evidence remains limited to the workflows those checks exercised.
- Refreshed host metadata in `ui/bun.lock` (captured in user commit `b9c11fb35`).
  Packaging now resolves every declared host dependency inside the artifact and
  runs production Go-host services, rather than accepting startup alone.
- Fresh `artifact-4`: service probe passes for imported Chromium preferences,
  distinct actual rasterized PDF pages, DOCX/XLSX/PPTX/text save/export and exact
  selected-version attachment; originals remain unchanged and no model is called.
  The standalone PTY, interactive-editor PTY and install/uninstall probes all pass.
- Current Go full suite (`go test -p 2 ./...`) and vet pass with cache-backed
  TMPDIR/GOTMPDIR. Desktop host tests and desktop typecheck pass; shared host has
  **72 cases / 71 pass / one Windows skip**. The broad UI run found three failures
  (Office dialog timing, catalog return and concurrent chat selection); these are
  being corrected and are not counted as parity evidence.

### Regression corrections after preference integration

- Live chat choices now retain stable conversation keys, prefer the retained Go
  catalog title over an active worker's temporary “New chat”, and refresh names/
  activity while the picker is open. Deduplication no longer hides a running chat
  under an obsolete title. Grouping controls follow chat entries in the flat list.
- Office keyboard checks wait for the returned editor of the actual saved
  revision before issuing Save or version actions; an acknowledged draft write
  can precede the replacement dialog. Catalog checks allow the terminal's ESC
  prefix to resolve before F5. Behavioral assertions remain unchanged.
- Focused `office-keyboard`, `preferences`, `catalog`, `concurrent` rerun:
  **4 pass / 66 assertions / 0 failures**; terminal typecheck passes. A fresh full
  frontend run is in progress; the earlier 33/36 result remains recorded above.

### File readiness and account setup regression checks

- The next full frontend run completed with **33 pass / 3 fail / 408 assertions**.
  Failures concerned table handoff, Office editor readiness and update visibility;
  this result does not establish parity. File checks now wait for the actual
  returned editor and attachment, and the manual renderer drives frames while
  awaiting durable visibility receipts. Behavioral assertions remain intact.
- Focused table, Office and update checks: **4 pass**. The new connection journey
  initially failed because it acted before Go loaded the requested panel, then
  expected the old account detail after returning to Connections. It now waits
  for the actual loaded row and acknowledged selection and explicitly reopens
  the account after returning, matching the reference journey.
- Final focused `connections` and `grid`: **2 pass / 21 assertions**. Connection
  setup reviews configuration before saving, masks private tokens, verifies one
  authenticated read, disconnects, and completes a real local OAuth callback
  after switching chats. The pending sign-in and both drafts retain ownership;
  credentials never appear in display state. On Linux an owned `xdg-open` fixture
  verifies the exact authorization URL; it does not prove native browser UI or
  OS permissions. Terminal typecheck passes. Full-suite rerun remains pending.

### Complete frontend regression run and dictation ownership

- Full frontend suite after connection/readiness corrections: **37 pass / zero
  failures / 451 assertions across 23 files** (`bun run --cwd ui test`). This
  supersedes the failing full-run result for those tested journeys; it does not
  close acceptance cases absent from the suite.
- Added the desktop's submission restriction while recording/transcribing: a
  normal chat submission refuses with a visible instruction to finish or cancel
  dictation, keeping the current draft. Voice navigation remains available.
- `bun test --cwd ui --preload ./scripts/preload.ts tests/voice.test.tsx
  tests/client.test.ts`: **8 pass / 55 assertions**; terminal typecheck passes.
  The actual Linux native process boundary receives synthetic PCM, then the real
  Go speech service sends WAV to a local Gemini fixture. Its private usage ledger
  records one voice request and 25 tokens without retaining transcription text.
- Keyboard start requires explicit review. Stop retains the original draft while
  awaiting transcription and appends the returned text for review without a chat
  call. Capture cancellation, a helper permission-denied error, cancellation of
  an in-flight transcription and switching chats preserve draft ownership and
  never submit a chat. Synthetic capture is not microphone hardware/OS permission
  evidence. Native speech language/rate behavior and live audio remain pending.

### Private data backup and restore through terminal controls

- The new keyboard journey exposed a baseline backend defect: file-only backup
  archives omit empty directories, so an unsent chat lost its empty `revisions`
  directory and restore rejected an otherwise valid archive. Restore now rebuilds
  that directory only for chats without a claimed head. Missing referenced
  revisions remain errors; source profiles and sealed evidence are unchanged.
- `go test ./internal/lifecycle`: pass, including new empty-chat round-trip and
  missing-head rejection cases. Rebuilt the private Go runtime before UI checks.
- `tests/data.test.tsx`: **1 pass / 23 assertions**; terminal typecheck passes.
  Keyboard controls export only after review, verify unchanged archive bytes,
  show a separate exact restore plan before creating the destination, and restore
  a new profile with shared usage paused. The original chat/draft stays selected.
  A second Go-host client opens restored knowledge without a synthetic revision
  or submitted task. External project bytes and the backup remain unchanged.
- The same journey cancels a knowledge-deletion review without removing the note
  and saves a reviewed 30-day history-retention policy. Destructive stale-plan,
  source-mapping/relink, and native platform cases remain separate open checks.

### Model selection and first-task setup

- Gator's model/effort picker now projects current Go state while open, with stable
  model identities. Disabled model/effort choices show their reason and cannot
  activate by keyboard or mouse; controls become usable after work completes.
  The upstream-derived selector keeps its original filtering by default and adds
  this behavior as an explicit option documented in `ui/UPSTREAM.md`.
- `tests/models.test.tsx`: **2 pass / 23 assertions**. Alias selection updates the
  actual provider model ID/default, effort changes preserve the unsent draft,
  missing credentials remain unavailable, and active-work restrictions release
  without reopening the picker. A real local request uses the chosen model.
- First-task setup retains the message and immutable file through choosing a
  cloud model and restarting. No provider request occurs before explicit send;
  later mutation of the original file does not alter the admitted attachment.
- Full Go suite after the empty-chat restore correction passes. Frontend typecheck
  passes. Provider creation/credential forms and local-model installation remain
  separate acceptance cases; these two tests do not establish all of P19.

### Provider form submission and credential reviews

- Acceptance work found an inaccessible action: forms advertising Enter to save
  or review could only reopen their selected input in the terminal. F2/Ctrl+R
  now submit the complete form using its advertised Go action and final input
  row, matching Electron's existing protocol. Enter continues to edit a field;
  forms with Go Ctrl+R reviews keep that route. No review is bypassed.
- Provider and panel suite: **4 pass / 33 assertions**. The real Go catalog creates
  custom endpoint metadata only after its separate review, preserves multiline
  model IDs, renames the display alias without changing the model identity, and
  saves a masked API key only into the private auth file. Cancellation retains
  that credential; explicitly confirming removal deletes it. Config/display state
  never contains the fixture key, and the unsent draft remains unchanged.
- Existing recurring-check, data and connection journeys after this shared form
  change: **3 pass / 92 assertions**. Terminal typecheck passes. Installation,
  local-model management and native device sign-in still need separate evidence.

### Local model lifecycle through the terminal

- `tests/local-models.test.tsx`: **1 pass / 13 assertions**, using the actual Go
  local-model manager and an isolated loopback Ollama-protocol fixture. Initial
  download review cancellation makes no pull request. Confirmed download renders
  streamed progress; Escape aborts its HTTP stream. A second explicit download
  updates the installed catalog, supports display rename and selects the original
  provider model ID for one real local HTTP task.
- Removal requires its own review and sends exactly one DELETE, then removes the
  managed provider configuration when its final installed model is removed.
  Setup/rename preserve the draft; setup alone makes no model inference request.
  No installed system Ollama process or real model weights are modified.
- This proves the terminal management journey and backend routing, not an actual
  multi-gigabyte download, privileged installer execution, hardware eligibility on
  other hosts or native macOS/Windows installation. Those remain explicit gaps.

### Assistant knowledge follows the active frontend

- Audit found the Go system guide still described Electron-only controls and
  denied the existence of a standalone CLI. The terminal entry now identifies its
  frontend to its own child environment; Go selects bundled terminal guidance
  with actual commands, file/voice/form controls and current authority boundaries.
  The retained desktop keeps its existing guide. The shared agent identity is
  frontend-neutral; this changes help, not tools, models, storage or permissions.
- `go test ./internal/workrun`: pass, including display-safe state, current model
  identity and terminal-versus-desktop guidance checks. Rebuilt the Go runtime.
  `tests/application.test.tsx`: **1 pass / 16 assertions**, verifying the actual
  provider receives terminal guidance without desktop-only controls/CLI denial,
  alongside the existing paste, draft, restart and navigation assertions.
  Terminal typecheck passes.

### Conversational application controls

- `tests/app-control.test.tsx`: **1 pass / 19 assertions**. Actual Go tool calls
  through a local model fixture change a usable command shortcut, select a model
  for the next request, and retain selected connections and shortcuts at restart.
  Usage-limit and retention changes require the exact live approval. Declining a
  subsequent limit change retains the previous policy.
- A model request to delete knowledge opens the canonical final deletion review;
  cancelling leaves the original note intact. No fixture directly writes the
  resulting application settings. Terminal typecheck passes. This closes the
  first two mapped desktop app-control journeys; native voice and appearance
  acceptance remain separately tracked.

### Dictation also protects live guidance

- The live-task guidance shortcut now refuses to consume the composer while
  microphone capture or transcription is active, matching ordinary send. The
  original text remains in its chat and no queue/guidance frame is sent.
- `tests/client.test.ts` and `tests/voice.test.tsx`: **8 pass / 61 assertions**.
  Both dictation phases reject guidance; idle guidance still receives Go's exact
  acknowledgement before clearing the submitted snapshot.

### Queued-message editing and ordering

- Extended the real Go recovery journey through the keyboard queue menu: edit an
  attached queued request, move it later and earlier while another request stays
  in flight, restart, explicitly resume its edited text with the original private
  attachment, remove the interrupted request, and restart again. The independent
  unsent composer remains unchanged and no request starts from queue management.
- `tests/recovery.test.tsx`: **1 pass / 19 assertions**; terminal typecheck passes.
  The initial fixture incorrectly used Ctrl+A as select-all (it moves to the
  beginning in this terminal) and treated string input names as objects. Corrected
  the fixture to the supported Ctrl+U clear-line operation and protocol shape;
  no queue behavior was relaxed.

### Attachment intake and clipboard process boundary

- `tests/attachments.test.tsx`: **1 pass / 31 assertions**; terminal typecheck
  passes. Keyboard file selection/cancellation/removal retains the unsent text.
  Unsupported files, documents above 8 MiB, images above 4 MiB, a thirteenth
  attachment and combined content above 16 MiB produce visible errors without
  replacing the accepted draft. Exactly twelve files and exactly 16 MiB are
  accepted. No model request occurs during selection or rejected intake.
- The Linux native clipboard helper is exercised as a child process returning a
  real PNG decoded by OpenTUI. The image and text file stay with their original
  chat across switching and restart. Changing both source files later does not
  alter the private copies: explicit files-only send delivers the retained text
  and image data to the real Go provider path and clears only admitted inputs.
- The helper fixture is not an OS clipboard permission test. Native macOS/Windows
  clipboard checks remain deferred. Terminal path selection is the replacement
  for Electron file dialogs/drag-drop; image viewing has separate pixel evidence.

### Native speech preference routing

- Fixed ignored Windows language/rate and ignored macOS language. Playback now
  enumerates installed voices, selects the exact locale or matching primary
  language, and visibly rejects an unavailable language. Reply text travels on
  stdin rather than command arguments or script interpolation. Cancellation
  during enumeration prevents playback. Linux keeps its existing helper route.
- Windows rate maps the stored 0.5–2 preference monotonically around normal 1 to
  the engine-specific -10–10 scale; this is not a claim of identical speech speed
  between platforms. The mapping uses Microsoft's documented
  [Rate](https://learn.microsoft.com/en-us/dotnet/api/system.speech.synthesis.speechsynthesizer.rate)
  and [installed voice selection](https://learn.microsoft.com/en-us/dotnet/api/system.speech.synthesis.speechsynthesizer.selectvoice).
  macOS enumerates the existing speech manager's voices through Apple's
  [voice attributes API](https://developer.apple.com/documentation/appkit/nsspeechsynthesizer/attributes(forvoice:));
  this API is deprecated but is the same speech engine used by `say`. Its native
  availability and selected-voice behavior still require Mac acceptance.
- Native helper suite: **7 pass**; complete shared host: **73 pass / 1 Windows-only
  skip / 0 fail**. Refreshed the copied Bun host dependency; Linux voice and image
  intake journeys: **2 pass / 51 assertions**; terminal typecheck passes. These
  are process-contract and Linux-fixture results, not native audio hardware proof.
- The preceding complete terminal suite passed **45 tests / 604 assertions across
  30 files**. Full Go tests and vet also passed with the restore/help changes.

### Cross-chat personal memory and privacy

- `tests/memory-privacy.test.tsx`: **1 pass / 21 assertions**; terminal typecheck
  passes. The real Go memory tool saves a grounded preference with its original
  chat provenance. A fresh chat has an empty transcript but receives the saved
  personal memory in the actual local provider request. Each chat retains its own
  unsent draft and reopening the original does not merge the second transcript.
- Keyboard privacy controls disable earlier-chat retrieval without disabling
  learning: subsequent requests omit `chat_search`/`chat_read` but retain memory
  tools and saved personal facts. After restart the independent setting persists.
  Re-enabling retrieval and disabling learning restores those history tools while
  keeping the saved preference and its provenance intact. No request is replayed
  on restart or settings navigation.

### Saved guidance choices and lifecycle

- `tests/guidance.test.tsx`: **1 pass / 12 assertions**; terminal typecheck passes.
  Keyboard controls create workspace-scoped procedure guidance with a key and
  complete multiline body, edit its text while type/scope stay read-only, disable
  it, filter active/disabled records, re-enable it and reopen it after restart.
  The exact line breaks, scope, original chat and unsent draft remain intact;
  these management actions submit no model request.
- This covers the saved-guidance portion of the desktop polish journey. Retired
  desktop-only presentation assertions are not claimed as terminal evidence.

### Public activity history and keyboard inspection

- Completed activity now folds above its answer, with duration/failure/stopped
  summaries, chronological narration and grouped tool activity. Ctrl+P `activity`
  opens live or retained public entries and expands/collapses history. Transcript
  search includes narration in expanded history; collapsed history stays out of
  that search. Empty successful work is quiet; incomplete work retains a cue.
- The user concurrently committed the initial rendering changes in `21ec3adb3`;
  this follow-up preserves that commit and adds acceptance evidence plus a guard
  against live activity completing while its inspection menu is open.
- `tests/activity.test.tsx` and `tests/transcript.test.tsx`: **2 pass / 27
  assertions**; terminal typecheck passes. A real Go `read_file` task exposes
  public narration/tool state without the private source body/path. Completion
  folds the trace before its answer; explicit expansion makes narration
  searchable. Restart restores history/draft, does not replay either provider
  request, and retained entries remain inspectable. Existing reading-position
  regression still passes. Full failure/group update fixture coverage remains
  separate from this real-runner case.

### Compact hub navigation

- Fixed a keyboard accessibility gap: hub selection could move outside the
  visible scroll region on short terminals. Following/Saved/Settings now reveal
  the selected destination after layout and resize, and reset selection when
  changing hubs so an index from a longer page cannot hide the new selection.
- Initial `tests/hubs.test.tsx`: **1 pass / 10 assertions**; terminal typecheck
  passed. The additional resize-before-dialog assertion failed after this entry
  was drafted; the correction and final rerun are recorded below.
  At 50×18 and 42×14, lower Settings/Saved destinations scroll into view, Enter
  opens the selected action, and switching to Following selects its first item.
  Sidebar collapse, draft preservation and absence of accidental submissions
  are verified. Real macOS/Windows terminal behavior remains deferred.

### Short-dialog layout correction

- The new 42×14 resize-before-dialog case exposed the pinned selector's zero-row
  viewport when its overflow indicator consumed its only row. OpenTUI reported
  `LayoutLimit`. The prior commit was made before its rerun result was inspected;
  its claimed 11-assertion pass is corrected above.
- Selector geometry now reserves at least one usable row after overflow; short
  dialogs start at the top instead of consuming a quarter of the screen as blank
  space, and width stays positive. The failing assertion remains in the suite.
- `tests/hubs.test.tsx`, `tests/models.test.tsx`, `tests/panel.test.tsx`: **6 pass /
  56 assertions**, including the extended hub case (**11 assertions**); terminal
  typecheck passes. Normal model availability and form masking/navigation guards
  still pass. These changes to imported components are recorded in `UPSTREAM.md`.

### Bounded activity subscriptions

- Recurring-check acceptance exposed an EventEmitter warning from each rendered
  activity history subscribing separately to the client. Activity now consumes
  its parent transcript's reactive expansion state; rendering more histories
  does not add client listeners. This keeps the new presentation bounded without
  hiding warnings or increasing the listener limit.
- Activity, long transcript and recurring-check journeys: **3 pass / 50
  assertions**; terminal typecheck passes. The long transcript adds 60 histories
  and verifies the client subscription count remains unchanged while scrolling,
  search, navigation and expansion still work.

### Conversational recurring-check agreement lifecycle

- `tests/followup-chat.test.tsx`: **1 pass / 22 assertions**, also included in the
  three-file activity regression run above. Actual Go app tools inspect and
  create/edit/pause a recurring check using live agreement digests and approvals.
  The exact preview includes title, end date and offline service state; declining
  creates nothing, and later edits do not change the saved agreement until
  separately approved.
- The resulting check retains its original conversation, read-only mode and exact
  expiry. Pause is reflected by the terminal management page and survives restart
  with its draft, without a new provider request or background-service start.
  Quiet-check feedback, service installation and native scheduling remain
  separately tracked; this is the agreement portion of the desktop journey.

### Installed package and Linux process measurements

- Fresh artifact `artifact-5` was built at `9edc858b6` with recorded Go build
  metadata. Production package verification passes with all dependencies resolved
  inside the artifact, no desktop tree, and Bun-only child PATH: real Go startup,
  legacy preferences, distinct rendered PDF pages, native Office/text editing,
  exports, selected-version attachment and unchanged original files.
- `python3 ui/scripts/verify-package.py <artifact>` and `--editor`: pass. Real PTY
  launch/paste/palette/drafts/quit restores terminal modes; editor handoff restores
  canonical input and echo, imports the returned draft, removes its temporary file
  and resumes raw input. `node ui/scripts/verify-install.mjs <artifact>`: pass for
  install, script commands, active-profile uninstall guard and retained user data.
- Complete terminal suite before the subsequent activity subscription change:
  **49 pass / 666 assertions / 34 files**. Desktop typecheck passes; its own
  host tests pass, followed by shared-host **73 pass / 1 Windows-only skip**.
  New activity/followup checks after that change are recorded above; a final
  rebuilt package and complete terminal rerun remain required.
- Added reproducible Linux-only measurement: `python3 ui/scripts/measure-linux.py
  <artifact> --revisions 0` and the default 150-revision case. Both frontends use
  isolated copies of the same stored conversation and the exact same Go binary.
  The sampler sums the complete product process tree's RSS/PSS, excludes Xvfb and
  the Playwright driver, measures readiness/search and verifies child cleanup.
  The first harness attempts needed ANSI-aware result recognition and atomic
  driver status publication; these were harness defects, not product failures.
- Observed results (one sample each; MiB, not a performance guarantee):

  | Profile | Frontend | Ready | Search | Ready PSS | Sampled peak PSS | Processes | Cleanup |
  | --- | --- | ---: | ---: | ---: | ---: | ---: | --- |
  | Fresh | Terminal | 987 ms | 160 ms | 135.5 | 145.3 | 2 | Pass |
  | Fresh | Electron | 2,046 ms | 80 ms | 443.8 | 445.4 | 7 | Pass |
  | 150 revisions | Terminal | 908 ms | 54 ms | 211.5 | 240.3 | 2 | Pass |
  | 150 revisions | Electron | 1,551 ms | 70 ms | 504.3 | 504.4 | 7 | Pass |

  Search controls differ (terminal prompt/results versus desktop inline search).
  OS cache/load were not controlled, and sampling can miss short-lived peaks.
  [Raw measurements](terminal-measurements/linux-2026-10-26.json) retain byte counts.
  Both clean exits leave no observed product child process; terminal modes are
  restored. Native-platform performance remains unmeasured.

### Preference writer crash and concurrency guard

- Preference updates now use the existing LevelDB native exclusive lock before
  recovering a stale PID lock or reading/writing JSON. Competing recovery cannot
  unlink a newer writer's lock. The compatibility PID file is hard-linked from a
  fully written private owner file, eliminating the ordinary empty-owner crash
  window. JSON remains the same authoritative frontend preference format.
- A live holder is rejected without writing; killing an isolated holder releases
  its OS lock, and a subsequent update recovers the stale PID marker while keeping
  existing settings. Two competing instances preserve every acknowledged patch.
  Invalid/unknown legacy lock owners remain conservative errors rather than being
  deleted speculatively. Native filesystem behavior still needs Mac/Windows checks.
- Preference host suite: **5 pass**. Complete shared host: **75 pass / 1 Windows-only
  skip / 0 fail**. After refreshing Bun's copied host dependency, preference UI:
  **1 pass / 18 assertions**; profile continuity: **2 pass / 28 assertions**;
  terminal typecheck passes. Original Chromium bytes, file drafts/versions, profile
  separation and disabled draft retention are preserved.

### Quiet-check assessment conflicts and recovery

- Extended `bun test --cwd ui --preload ./scripts/preload.ts
  tests/followup-chat.test.tsx`: **1 pass / 33 assertions**; terminal typecheck
  passes. Only a completed, evaluated quiet decision appears for feedback; failed
  and unchanged attempts do not become evaluated decisions.
- Keyboard actions save a missed-change note, mark the decision correct, reject
  an assessment changed concurrently, and retain the complete unsaved note when
  navigation is cancelled. Explicit discard permits refresh; clearing the latest
  assessment advances its sequence without changing the check agreement, paused
  state or alert condition. Restart retains the original chat and draft and
  makes no provider request. Rendering assertions account for wrapped text and
  independently verify the complete authoritative field value.
- The desktop journey's typed expiry input is covered by the existing page-form
  test. Native scheduling and actual background-service installation remain open.

### Appearance across cached sessions

- `bun test --cwd ui --preload ./scripts/preload.ts tests/appearance.test.tsx`:
  **1 pass / 33 assertions**; terminal typecheck passes. The keyboard Appearance
  page persists light, contrast, monochrome and dark choices in Go configuration.
  Native renderer cell colors verify actual chat/settings surfaces and the raised
  command-palette surface at 60×20, rather than checking a theme label alone.
- Reopening an already cached chat refreshes its global appearance while restoring
  its draft. Restart retains the light theme, transcript and unsent message. Theme
  changes and navigation make no additional provider requests. The fixture needed
  corrections for omitted zero selection, the palette's actual title and raised
  background; the final assertions pass. OS terminal chrome remains terminal-owned.

### Installed untrusted-file rendering

- `python3 ui/scripts/verify-package.py artifact-5 --untrusted-file`: **pass** on
  Linux. Actual keyboard controls import and open a text file containing OSC 52
  clipboard, OSC title and screen-clear payloads. The complete final line renders;
  the exact clipboard/title control sequences never appear in raw PTY output.
  Original file bytes and the unsent composer survive, and clean exit restores
  terminal modes. PATH contains only Bun; no model, Electron or OpenCode server
  participates. This verifies the unchanged file editor in the earlier artifact;
  the final package rerun remains required.
- Native CI now runs install/uninstall guards on every runner, plus real PTY,
  editor handoff and untrusted-file probes on Unix. This is configured coverage,
  not evidence of a remote CI or native macOS/Windows pass; no push was performed.
