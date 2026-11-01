# Electron product consolidation

1 November 2026. Source inventory: `eb105ca13`, clean worktree. The owner
requested this inventory before removing the terminal product. Electron is the
sole primary application; the terminal migration is cancelled. The authenticated
phone companion remains an optional extension of the same application.

## Backend-connected capability inventory

The former frontend lives at `ui/` in the inventory revision. Its `gator/actions`,
`gator/native`, `gator/files`, `gator/panel`, `gator/client` and `gator/hub` modules,
plus its integration tests, were compared with the Electron renderer, IPC and
shared host. The table maps capabilities, not equivalent keyboard shortcuts.
“Existing” means connected code was found; verification results below distinguish
that from an executed test. Generic Go pages remain reachable through command
search and in-app slash commands, not a standalone CLI.

| Capability in former terminal | Backend/host authority | Electron destination and disposition |
| --- | --- | --- |
| Chat, stream, stop, independent running chats | `Sessions`, `Backend`, `internal/workapp` | Existing `App`, `RecentConversations`; concurrent-chat journeys |
| Queued messages, ordering, editing, resume, live guidance | Go request journals and queue frames | Existing `MessageQueue`, `App` Guide now; no replay on restart |
| Exact live approvals and specialist controls | Go approval tokens, `/tasks`, `/steer-task`, `/cancel-task` | Existing `Result`, command search and typed Go panels; native protocols remain semantic |
| Reconnect and uncertain admission | `Backend`, `Sessions`, request journal | Existing reconnect screen and stable admission IDs |
| Drafts, attachments, per-chat recovery | `DraftStore`, `UploadStore` | Existing `useDrafts`, `DraftManager`, `Attachments` |
| Reuse a previous message without sending | Retained Go transcript; frontend draft staging | **Ported:** Message tools → Reuse a message, append or explicitly review replacement; keep attachments/reply identity |
| Command arguments independent of message draft | Go command handling, frontend argument prompt | **Ported:** command search opens a separate argument dialog rather than overwriting the composer |
| External composer editor | `Backend.prepareComposerEditor`, Go-owned editor handoff | **Ported:** narrow host-owned editor IPC and Message tools action; no arbitrary renderer command execution |
| Public activity and transcript search | Go activity projection | Existing `ActivityTimeline`, `useConversationSearch` |
| Current selections vs retained evidence | Go `work_details`, verified manifests | Existing `WorkDetails`, latest-only context and output projections |
| Following goals, checks, Inbox, attention | Go goals/jobs/inbox/overview adapters | Existing `LibraryDrawer`, `/goals`, `/followup`, `/inbox`, `/overview` |
| Create/edit/pause reviewed recurring checks | `followup`, `prepareFollowup`, `SaveReviewed` | Existing conversation approval and typed Following forms |
| Quiet-check feedback, timing, health, recovery | Go health and feedback versions | Existing check detail/quiet-review panels and `ChatCheckHealth` |
| Read/done/reply/usefulness and update paging | Go update identities and receipts | Existing `ChatUpdates`; independent meanings retained |
| Goal plans, dependencies, occurrences, proposals | Go goal/suggestion services | Existing `/goals`, `/agenda`, `/suggestions` typed panels and review actions |
| Saved knowledge, notes, correction, forgetting | Go knowledge, provenance and expected revisions | Existing `/knowledge` panels, `ChatMemory` |
| Personal memory, learning, earlier-chat privacy | Go memory/learning/retrieval settings | Existing `/memory`, `/learnings`, guarded typed forms |
| Indexed conversations, pin/archive/rename/page | Go conversation catalog | Existing `ConversationCatalog` with original record IDs |
| Run history, source snapshot/revision navigation | Go history/worksession/source adapters | Existing `/history`, `/revisions`, `/source`, `/source-refresh` |
| Text/code/Markdown/JSON and CSV/TSV editing | `FileStore`, immutable versions | Existing `FileSurface`, `TextFileEditor`, `FileGrid` |
| Word paragraphs/tables, Excel values/formulas, slide text | Shared Office extraction/editing | Existing Office controls in `FileSurface`; simplified layout, no claim of native recalculation |
| Image and PDF visual preview | Shared file validation, renderer decoding | Existing image previews and `PDFPreview`; no terminal image protocol needed |
| Version comparison/restoration, edit-draft recovery | Shared file/comparison services | Existing `FileVersionComparison`, version menu; originals/sealed outputs unchanged |
| Ask Gator about exact saved version/selection | `FileStore` attachment link and provenance | Existing file Ask Gator and selection controls; user still sends |
| Export/review existing destination safely | `ExportStore`, selected saved bytes | **Ported:** shared replacement tokens/fingerprints in Electron; reject changed/symlink/private destinations |
| Model, effort, API/provider credentials | Go composer IDs and model/provider forms | Existing `ModelPicker`, Settings → Models, typed secret fields |
| Empty-chat welcome and first-task setup | Actual managed-chat state and frontend onboarding | **Corrected:** an empty managed chat has an identity even when `home` is false; setup now appears while keeping the independent draft and attachments |
| Local model download/cancel/rename/remove | Go local-model manager | Existing local-model panels; review and stream state retained |
| Connections, token/OAuth sign-in, verify/disconnect | Go connection flows, trusted external handoff | Existing Connections forms and system browser/Terminal handoff |
| Browser setup and connected coding specialists | Existing Go tool/agent policy | Existing command search `/browser`, `/agents`, `/tasks` and reviews; not removed with terminal frontend |
| Voice capture/transcription and playback preferences | Shared voice policy/accounting | Existing `VoiceControls`: Electron microphone and system voice replace terminal ffmpeg/playback |
| Clipboard text/image intake and copy | Electron clipboard/input, shared image validation | Existing native paste, `Attachments`, `CopyButton`; no OSC52 needed |
| Theme, shortcuts, sidebar, folder grouping | Go preferences and Electron local display state | Existing Appearance, shortcuts, `RecentConversations`, sidebar |
| Usage, pause/resume, limits | Shared usage service and Go reviews | Existing `UsagePanel` and `/usage` |
| Retention/deletion, backup/verify/restore/relink | Go lifecycle/private storage | Existing `/data` typed pages, exact reviews; no profile migration/deletion |
| Supervisor/start-at-login/service repair | Existing Go recovery workflow | Existing `/recovery`; future execution still requires reviewed start |
| Version/releases/updates | Bundled metadata, signed update guard | Existing About & updates; desktop packaging remains authoritative |
| Local terminal enrollment/grants; phone Work/tmux | Go external-target and Work services, companion auth | Retain host-local maintenance commands and companion routes; not terminal-frontend features |

Terminal-only rendering, ANSI safety, pane layout, terminal key fallbacks, TUI
installer/launcher and OpenCode frontend dependencies are not additional backend
capabilities. They are removed rather than reproduced in Electron. Existing
system Terminal handoffs are narrowly prepared by Go; these are not the Gator
terminal product. Internal maintenance commands, background workers and native
coding-agent executables remain necessary application infrastructure.

## Removal boundary

Remove the tracked `ui/` product, terminal packaging/CI/Make targets, and the
terminal-specific model help guide. Preserve `internal/workapp`, shared
`app-host` services/contracts/tests, all Go agent/tools/data, external tmux,
companion, and Onibi attribution. Do not uninstall Bun or delete installed
applications, user profiles, drafts, credentials, services or historical releases.
Historical terminal migration evidence remains labelled as superseded. The
desktop development/build/package/release commands become the only current app
instructions. No new provider calls are required for this consolidation.

The owner separately requested live model-driven Electron tests during this work.
Those opt-in evaluations and their shared spending ledger are documented below;
ordinary consolidation/regression tests remain independent of paid providers.

## Additional removal and compatibility details

The generic `Application` wrapper was used only by the removed TUI. Its runtime
composition duplicated Electron's host. It and the terminal-only Chromium
preference importer are removed after checking for remaining consumers. The
importer's `classic-level` dependency and its exclusive dependency subtree are
removed from both lockfiles. No retained package version is deliberately upgraded.
Electron continues to use its own existing Chromium preferences; no original
preferences, imported JSON files, application installations or profiles are deleted.
`Backend`, `Sessions`, drafts, uploads, files, exports, policy, voice and the
Go-owned handoff services remain shared and authoritative. The new native-service
workflow preserves the old terminal workflow's backend coverage without Bun,
terminal packaging or a second application host.

The removed launcher forwarded `work`, `work-rpc`, `config`, `provider`, `agent`,
`job`, `goal`, `memory`, `note`, `data`, `usage`, `doctor` and other legacy service
handlers to the private Go runtime. Those handlers remain available to explicit
maintenance/evaluation callers and the app. This change removes their public
Bun launcher, not their business logic, and does not turn them into an arbitrary
renderer command-execution API. Ordinary interactive equivalents are mapped
above. Host-local device grants/enrollment remain local administrative operations;
a remote client or model gains no new authority to perform them.

If a terminal installation used a custom `GATOR_TERMINAL_USER_DATA` location,
select that same location explicitly with `GATOR_DESKTOP_USER_DATA` to reuse its
retained draft/file data. Default application data already used the same location.
The terminal-only display-preference JSON is no longer read; Electron's own
settings remain. No automatic copy or migration is attempted. If branch protection
requires old `terminal-*` check names, the repository owner must replace those
required checks with the retained desktop/native-service checks; no remote rules
were changed here.

Editor dispatch is bound to the current idle Go owner and a host-prepared job.
The normal result is saved through the existing draft store before IPC
acknowledgement when retention is enabled; newer draft text, attachment and reply
identities are kept. Returned oversized text requires explicit shortening in the
open recovery view; that recovery buffer is not promised across app reloads.
Go's internal input buffer no longer silently replaces the renderer's independent
saved draft at reload. Command argument dialogs are scoped to their original chat
and rechecked at the host before command dispatch. All review/approval checks
remain in Go. No new model call, execution worker or terminal input API is added.

## Verification

Verified locally on Intel macOS with Node 24 and the existing Go installation.
The module's Go declaration, provider dependencies and Onibi notices are unchanged.
The native-service workflow is prepared for its existing platform matrix; it has
not been executed on remote CI in this session.

| Check | Observed result |
| --- | --- |
| `go test ./...` | PASS: 72 tested packages, 11 without test files; live API tests skipped by default |
| `go vet ./...` | PASS |
| Focused Go application-guide, editor, bridge and follow-up tests | PASS |
| `go test -race ./internal/workrun ./internal/workapp -run 'ApplicationContext\|Conversation.*Knowledge\|RetiredFrontend\|ComposerEditor\|BridgeEditor\|Followup' -count=1` | PASS |
| `npm run check --prefix desktop` | PASS |
| `GATOR_INDEPENDENCE_BINARY="$PWD/desktop/runtime/gator-runtime" npm test --prefix desktop` | PASS: 19 desktop tests plus 73 shared-host tests; one existing opt-in skip in each suite on macOS |
| Companion auth/server/recovery/inspection/mutation/external and Chromium clients, sequential Node runner | PASS: 25, one opt-in native-tmux skip |
| Real-Go independence fixture with Onibi, tmux and Tailscale absent from `PATH` | PASS; ordinary Work succeeds and creates no external catalog |
| New three Electron consolidation journeys | PASS: message reuse/arguments, Go-owned editor/error/empty/oversize recovery, exact export replacement |
| Local `npm run pack --prefix desktop` and ASAR audit | PASS for unsigned macOS x64 package; no terminal renderer, OpenTUI/Solid or terminal-only Level dependency |
| npm lockfile dry-run installation and workflow YAML parsing | PASS; no retained dependency version intentionally changed |
| Paid live Electron source-change journey | PASS twice; details and failed preliminary attempts retained in the evaluation guide |

The companion command was:

```sh
node --test --test-concurrency=1 \
  companion/auth.test.mjs companion/server.test.mjs \
  companion/inspection.test.mjs companion/mutation.test.mjs \
  companion/recovery.test.mjs companion/external.test.mjs \
  companion/inspection-client.test.mjs companion/mutation-client.test.mjs \
  companion/external-client.test.mjs companion/terminal-client.test.mjs
```

Earlier parallel host/companion runs hit fixture deadlines while independently
compiling Go. Sequential reruns passed without increasing those timeouts.
Additional ordinary Electron regression outcomes are recorded after the final
run below. The native editor fixture uses a disposable launcher that imitates
the existing system Terminal handoff; it does not type into a developer terminal.
Memory fixtures now separately answer native semantic-comparison requests instead
of accidentally returning unrelated user-chat tool calls. Production memory and
evidence policy have not changed.

Foreground regression runs retained failures involving unexpected typed/deleted
characters, a disappearing voice popup, a missing approval and a detached
follow-up card during resize. The voice and attachment scenarios passed when
rerun with optional `GATOR_E2E_BACKGROUND=1`. The test-only `launchDesktop` helper
then sets macOS/Windows fixture windows unfocusable and blurs them while retaining
the actual CDP-controlled renderer. It also captures the child handle before
shutdown to preserve accurate cleanup status. No production window policy or
authentication changes were made. Linux keeps its existing display behavior;
paid live demos explicitly bypass background mode. Final full-run results follow
below rather than treating these preliminary foreground failures as a clean run.

The initial new source/link fixture omitted its provider's required default model,
which caused native startup alerts. That fixture is corrected and now validates
its isolated configuration through the existing Go loader before launching Electron.
The four focused source/link/effort journeys subsequently passed without alerts.

## Live UI evaluation

Run `make build`, then `npm run test:live --prefix desktop` to watch real OpenAI
answers populate the Electron conversation. The ignored root `.env` supplies the
key to the test-only Go cost guard; the renderer gets a temporary loopback token.
The journey changes a project's deadline and blocker between two actual requests,
checks updated facts and no external action, and retains screenshots and answers.
It is a real source/tool/model/UI integration slice, not a comprehensive consumer
quality score. The proxy buffers responses for usage settlement, so provider token
streaming timing is not assessed. Default UI and CI tests do not invoke it.

The original $10 evaluation ceiling remains. At the last recorded run, the shared
ledger conservatively accounts for $6.266585 including search reservations, with
zero unresolved model reservations. No ledger reset was performed. See
[exact attempts, bounds and artifacts](../scripts/project-followup-eval/README.md#live-electron-journey).

## Scoped interface review and limits

New controls reuse React, plain CSS, existing tokens and Lucide icons. Typography
uses existing body/metadata styles, bounded plain-text previews and editable
review text. Surfaces retain the existing modal/popover hierarchy and 40–44px
actions. Icons keep the application's inherited color and stroke conventions.
No animation library was added; the requested effort transitions reuse CSS and
the existing reduced-motion/interruptible-motion checks. Performance stays bounded by
50 displayed message matches and the existing input/output size limits. This is
a scoped review of the added controls, not a claim that every visual state in
the application was manually reviewed at slowed animation speed.

The owner subsequently requested source-reference inspection, effort transitions
and pasted-link editing. These are implemented in `SourcePreviewSidebar`,
`sourceReferences`, `ComposerInput`, `LinkPopover` and the existing `ModelPicker`.
The source IPC reads only an already selected run's digest-bound manifest and
retained textual evidence. It checks current conversation ownership before and
after I/O. Exact read ranges come from matching `read_file` tool results in that
run's private revision; the private replay itself never crosses IPC. Missing
historical/compacted ranges are explicitly unknown, not inferred from citations.
Reads clipped mid-line retain their actual final character offset. The preview
is bounded to 400 lines/64 KiB from files no larger than 512 KiB. Binary or absent
evidence fails explicitly, with no fallback to live workspace contents.

User messages now share the safe Markdown renderer used for assistant replies;
structured reviews and approvals retain their application-generated forms.
The native textarea remains the draft/IME/selection/undo authority. A matching
text layer provides clickable links, and unfocused Markdown links display their
label. Editing uses canonical Markdown, with no HTML-backed persistence or new
editor dependency. Conversation link edits stage a new draft copy; they do not
rewrite an executed request. Exact draft context keys prevent cross-chat edits.
Only explicit Open link invokes the existing host URL allowlist and native
browser. Neither remote images nor favicons are fetched automatically.

The effort fill and handle use interruptible 200ms CSS transitions; persistent
labels crossfade without remounting the control. Dragging stays immediate and
reduced motion disables interpolation. A Chromium check slows active transitions
to 10% playback, reverses direction, then checks reduced motion. Source and link
screenshots were inspected; light-theme link contrast has automated coverage.

The dependency notice generator now also retains direct production host-package
notices when npm marks a root-hoisted copy as a development dependency through
the local-file link. `fflate` and `@xmldom/xmldom` remain attributed; no dependency
version or project license was changed.

The inventory did not justify replacing semantic specialist controls with
terminal keys, adding a second host/database, or importing terminal rendering
and animation dependencies. Those alternatives would duplicate authority without
adding a missing consumer capability. Host-local administration intentionally
remains distinct from the renderer's finite, reviewed operations.

Physical phone, signed installer, Windows native runtime, Linux/WSL native runtime
and a real external-editor application were not accepted in this session. The
external editor button remains unavailable on Windows pending a verified native
editor handoff. No new native tmux test was run against any user session. Retained
historical platform records remain evidence for their original scope only.
