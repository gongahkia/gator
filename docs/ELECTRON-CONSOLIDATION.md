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

Implementation and regression verification are in progress. Record exact results
here before completion; this inventory does not claim live-device, signed-release
or physical-platform acceptance.
