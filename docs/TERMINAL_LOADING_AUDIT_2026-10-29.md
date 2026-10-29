# Terminal loading and navigation audit — 2026-10-29

Scope: the native Solid/OpenTUI terminal. Reuse the existing Rattles Dots frame
data and timing through `Spinner`; OpenTUI owns rendering and cleanup. This does
not add a Rust process or change the desktop/companion UI or CLI JSON output.

## Loading inventory

| Surface | Indicator and source of truth |
| --- | --- |
| Active conversation | Existing `WorkingStatus`: live execution status and elapsed time |
| Other chats in the sidebar | Dots only for a currently running chat whose status is Working; stable rows preserve animation across snapshots |
| Startup, reconnect, switching chats | Host connection/session-switch state in the shared progress row |
| Sending a message | Pending acknowledgement, before execution begins |
| Chat catalog | Panel request/loading state; shared progress when loading outside the catalog |
| Settings and other backend requests | Explicit backend `pending_operation`, cleared by the operation's completion path |
| Agreement save | Explicit save-in-progress state |
| Local memory installation/readiness | Installation state or outstanding readiness request count; readiness only displayed in Memory |
| Saved receipt checks | Existing `receipt_busy` state |
| Async menu actions | Current-chat promise lifetime: model selection, memory changes, queue edits, retained drafts, exports, comparisons, clipboard actions and helper checks |
| Action forms | Existing busy prompt spinner until the action acknowledges or fails |
| File import/open/edit/save/versions | Current-chat promise lifetime; draft autosave included |
| Folder picker | Directory resource's loading state; stale directory entries hidden while loading |
| PDF previews | Page rasterization's busy state |
| Update feedback | Feedback's saving state; acknowledgement remains plain text |
| Audio transcription | Transcribing phase; recorded text still requires the existing draft review |
| Shutdown | Closing promise, until the renderer is restored and the resume hint is printed |

New incidental loaders wait 150 ms before appearing to avoid flashing during
quick actions. Existing working/form indicators and feedback saves appear
immediately. Operation tracking clears in `finally`, handles overlapping
operations, and only displays operations owned by the selected chat. Repeated
unrelated snapshots do not restart the visibility delay. `NO_COLOR` and
`GATOR_REDUCED_MOTION=1` retain static `⋯` feedback. Labels remain readable without
color or motion. No progress percentages are invented.

## Ambiguous placements kept static

- Approvals, browser takeover and manual sign-in: waiting for a person does not
  establish that a background operation is progressing.
- Paused, queued, offline, completed or failed work; scheduled checks between
  runs; routine background polling: retained state is not active execution.
- Microphone recording and spoken playback: keep their explicit phase labels
  and stop controls. Only transcription uses a loading indicator.
- External editor/terminal handoff: the external program owns the terminal;
  Gator must not draw over it.
- Cancellation: retain “Stopping…”; it must not look like ordinary execution.

## Navigation changes reviewed alongside loading

- Palette labels and groups use uppercase; original command IDs remain searchable.
- Remove repeated sidebar branding and decorative search bars; use “Search”.
- Separate chat rows vertically, show local date/time and workspace folder, and
  retain full timestamp/path in details.
- Drag the sidebar divider or use Ctrl+Shift+Left/Right. Width is bounded to keep
  the conversation usable, survives hiding/resizing within the process, and is
  not persisted across application restarts.
- `/exit`, `/quit` and Ctrl+Q share application shutdown and existing unsaved/
  active-work review. After terminal restoration, print a quoted runnable resume
  command. `resume ID` opens that retained conversation directly. Reply/attachment
  context does not prevent exit; cancelled reviews preserve the draft.

## Full review coverage

| Category | Evidence | Finding |
| --- | --- | --- |
| Typography | Palette, catalog and progress labels; native renderer assertions | Consistent palette case, searchable aliases, readable time/folder metadata |
| Surfaces | Sidebar divider and shared progress row | Terminal default background and existing palette tokens; no new branded fill |
| Animations | Timed spinner/progress tests, delayed operations and chat switches | Active work only, short-action delay, reduced motion and cleanup |
| Icons | Search label, existing header and connection dot, Dots glyph | Search is explicit text; spinner always accompanies a label |
| Performance | Promise cleanup, stable sidebar rows, timer disposal, typecheck/build | No additional dependency, polling loop or backend work triggered for animation |

| Severity | Location | Before | After | Reason |
| --- | --- | --- | --- | --- |
| Medium | `gator/progress.tsx`, `client.ts`, backend bridge | Several slow operations had no visible progress | Shared current-chat operation feedback with explicit backend signals | Distinguish actual work from an unresponsive interface |
| Medium | `gator/files.tsx`, `path-picker.tsx`, `panel.tsx`, `transcript.tsx` | Plain loading text or no indicator | Shared Dots component at the operation's surface | Consistent motion and lifecycle |
| Medium | `component/session-tabs.tsx` | Other running chats used plain status | Spinner for other actively working chats | Show concurrent work without animating retained/paused states |
| Medium | `component/command-palette.tsx` | Mixed case; uppercasing initially broke hyphenated searches | Uppercase labels with original searchable aliases | Consistency without losing command discovery |
| Medium | Sidebar and catalog components | Repeated branding, tight rows, fixed sidebar | Search text, row spacing and draggable divider | Reduce clutter and let users allocate width |
| High | Launcher, client and application exit | Slash exit could miss frontend shutdown/resume | Shared guarded exit and executable resume hint | Return to the shell and reopen the exact retained chat |

### Alternatives rejected

1. Animate every status: human attention and scheduled waiting would look busy.
2. Start a Rust spinner process: it would compete with OpenTUI for terminal IO.
3. Animate each keystroke's composer draft retention: excessive incidental motion.
4. Flash a spinner for every immediately resolved action: use the short delay.

## Verification and limits

- TypeScript check and production terminal build pass.
- Latest combined renderer run: 6 tests pass (progress, sidebar/navigation,
  working status, and real-Go concurrent conversations), 75 assertions.
- Progress coverage includes overlapping operations, failures, context changes,
  repeated snapshots, quick-operation suppression and human-waiting states.
- Full `go test ./internal/workapp -count=1` passes, including backend progress
  state and local-memory readiness completion tests.
- Catalog, local-model and update tests pass after replacing stale-state waits
  with observable completion conditions. These are focused reruns, not a claim
  that the complete repository test suite was rerun.
- The built launcher passes `ui/scripts/verify-resume.py` in an isolated macOS
  PTY: `/exit`, `/quit`, Ctrl+Q, and reopening via the printed command. No real
  account, provider request or existing user profile is involved.
- Physical terminal/emulator appearance, native Windows drag/exit behavior, real
  audio permissions and sign-in remain unverified. Renderer tests establish the
  component behavior; they do not substitute for those checks.

Approved within the tested native renderer and isolated macOS PTY boundary.
