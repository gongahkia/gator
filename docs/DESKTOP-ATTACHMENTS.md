# Desktop message attachments

Decision: 22 October 2026. Add file selection to the continuing chat rather
than requiring users to move files into a project folder or enter `/attach`.
Keep the existing React/CSS presentation, command palette and agent services.

## User flow

- The paperclip opens the system file picker. Files can also be dropped onto
  the composer. Selection prepares a private local copy; it does not send a
  message or contact an inference provider.
- Removable filename/size chips belong to the current draft. Local draft
  retention also keeps attachment selections. Switching conversations must
  preserve their independent selections. Files-only messages are supported.
- Send includes the selected inputs once. Queued messages capture their own
  selections; later edits/reordering retain those files. Attachments cannot be
  injected through the text-only Guide now path.
- Sent messages retain filename labels, including after reopening. Actual
  outputs keep their separate Files surface. Uploading an input does not
  imply that Gator has produced, verified or delivered an output.
- Picker cancellation leaves the draft unchanged. Invalid selections and
  loader errors preserve the unsent message and file chips for correction.

Supported types follow `internal/attachment`: PNG/JPEG/WebP, PDF, DOCX, ODT,
XLSX, PPTX and the existing text/Markdown/tabular/configuration formats. The
limits remain four combined uploaded/workspace inputs, 4 MiB per file and
8 MiB of provider-visible input in total. Office extraction shares those
limits; format checks use the canonical loader. Existing model
capabilities still determine visual/PDF handling. GIF, ZIP, audio and video
are outside this implementation. Clipboard image paste is a separate input
path and is not included here.

## Delivery boundary

The Electron host copies selected files under the active state directory's
`gator/uploads/<random identity>/<filename>`, with private directory/file
permissions. Project folders and originals are untouched. Temporary copies
older than seven days are pruned on startup. Disabling draft retention removes
saved draft records; staged copies retain their temporary expiry. Sent input
evidence follows the existing agent retention policy.

The renderer receives opaque relative identities and display metadata.
`upload_submit` carries a request identity, draft/conversation binding, text
and selected file identities. The host and Go validate them independently.
Go loads files beneath the host-selected upload root with the existing
descriptor-rooted workspace reader, validates content and combined limits,
and acknowledges acceptance or rejection. The desktop keeps the draft until
that acknowledgement. Existing execution, tool approval, provider selection
and artifact services handle the accepted request.

Filename labels are optional revision display sidecars. They do not change
the strict revision/replay schema. Missing or damaged presentation data must
not prevent reading a saved answer. No attachment bytes enter ordinary UI
state, activity traces, command text or draft JSON.

Implementation references: Electron's [native file dialog](https://www.electronjs.org/docs/latest/api/dialog)
and [File-to-path bridge](https://www.electronjs.org/docs/latest/api/web-utils).

## Interface review

Full review using `make-interfaces-feel-better`, scoped to attachment selection,
drafts, queued/sent filename labels, errors and Sources & memory. React 19,
native CSS, existing Lucide icons and IconSwap; no additional UI framework.

| Category | Evidence inspected | Result |
| --- | --- | --- |
| Typography | Filename chips, sizes, queue/sent labels and error copy | Existing type tokens; filenames truncate with a full title; sizes use tabular figures; long error filenames wrap. |
| Surfaces | Composer, picker, remove controls, optional inspector, wide/narrow layouts | Existing surface tokens and 40 px controls; no empty attachment strip. |
| Animations | Existing icon swap, spinner and composer shadow feedback | Existing motion conventions; textual Preparing/Drop feedback accompanies visual cues. |
| Icons | Paperclip, file, remove and loading controls | Same currentColor Lucide set and shared icon swap. |
| Performance | Import buffers, bounded file lists, UI state and draft persistence | Four inputs; bounded descriptor reads; payload bytes stay out of React and ordinary bridge state. |

| Severity | Location | Before | After | Why |
| --- | --- | --- | --- | --- |
| MEDIUM | `desktop/src/Attachments.tsx:6`, `App.tsx:278`, `styles.css:162` | No desktop attachment entry despite core file support | Paperclip, drop target, removable filename/size chips and files-only messages | Make the supported capability discoverable and reviewable before sending. |
| MEDIUM | `desktop/src/useDrafts.ts:45`, `DraftManager.tsx:18`, `App.tsx:212` | Drafts and restart guards handled text alone | Persist per-conversation file selections, show them in saved drafts, and protect unsaved selections during updates | Attachments belong to the intended message and must survive navigation/reopening. |
| MEDIUM | `desktop/src/MessageQueue.tsx:154`, `Attachments.tsx:43`, `WorkDetails.tsx:96` | No uploaded input labels in the queue/chat; inspector only knew workspace paths | Captured queue/sent filenames and next-message selections in Sources & memory | Distinguish selected inputs from retained outputs and avoid contradictory empty states. |
| MEDIUM | `desktop/src/App.tsx:50`, `App.tsx:223`, `Attachments.tsx:14`, `desktop/electron/preload.cjs:3`, `internal/attachment/uploads.go:48` | Existing send behavior cleared text on transport delivery; initial errors exposed IPC/staging details | Wait for canonical upload acceptance; show readable filename-based errors beside the composer and keep its text/files; focus returns after picker/removal | Correcting a bad file should preserve work and keep the rest of the app usable. |

Considered and rejected:

- Copying uploads into the selected project would modify that project and
  blur snapshot/source boundaries. Use private staging instead.
- Sending file payloads through ordinary UI state or command text would
  expand the display boundary and expose content unnecessarily. Use bounded
  opaque identities and the existing loader.
- Adding attachments to Guide now would imply binary support in its text-only
  steering channel. Queue file-bearing follow-ups through the normal path.

## Verification

- `npm run check --prefix desktop` passed.
- `npm run build --prefix desktop` built the Go runtime and UI; subsequent
  `npm run build:ui --prefix desktop` builds include the final presentation.
  Existing CSS-highlight/Lucide warnings remain.
- `npm test --prefix desktop`: 30 host checks passed, one Windows-only handoff
  check skipped. Covers private immutable copies, duplicate filenames with
  independent identities, rollback of partial imports, byte/count/type/path
  limits, attachment-only draft recovery and conversation-bound request shapes.
- `go test ./internal/attachment ./internal/workapp ./internal/worksession ./cmd/gator -run 'Test(DesktopUploads|BridgeUploads|Uploads|ActivitySidecar)' -count=1`
  passed, as did the earlier attachment/composer/UI-bridge checks. These cover
  rooted loading, invalid image bytes, combined workspace/upload limits,
  queue capture, stale conversation rejection and optional filename sidecars.
- `go test ./internal/workrun ./cmd/gator -run 'Test(ExecutorPassesPromptAttachments|ExecutorExtractsPDFAttachment|WorkCommandSendsExplicitAttachments|DesktopUploads)' -count=1`
  passed, exercising the existing manager-input and PDF handling paths.
- From `desktop`, `./node_modules/.bin/electron-builder --dir --publish never`
  rebuilt the macOS application.
- Six initial packaged Electron checks passed: attachments with a real local
  provider fixture, cancellation/removal/content errors, ordinary draft
  recovery, recent-conversation isolation, queued edits/reordering and the
  full Go-state/restart smoke check. These used
  `npm run test:e2e --prefix desktop -- attachments.spec.ts desktop.spec.ts --grep 'attachments stay|attachment cancellation|unsent drafts survive|recent topics default|queued messages can|desktop uses real Go state' --reporter=line --output=node_modules/.cache/attachment-results`
  with `GATOR_DESKTOP_TEST_EXECUTABLE` set to the rebuilt app.

The expanded packaged checks added actual dropped disk-backed files, files-only
messages, separate conversation drafts, filename restoration and pending-file
visibility in Sources & memory. The error/drop and existing inspector checks
passed on their first run. A restart assertion initially matched streamed
narration before the run completed, causing shutdown to correctly require a
stop decision. Updated the test to await the canonical assistant response and
absence of live activity; the complete success/restart check passed in 8.4 s.

An additional isolated native-app review exercised empty, hover, focus,
preparing, selected, disabled-at-four-files and keyboard-removal states. Used
the Chromium Animation domain at playbackRate 0.1 to inspect the loading icon
transition, then restored normal playback and exercised reduced motion. The
attachment control measures 40 × 40 px; removal restores composer focus.
Long filenames and three remaining chips fit at 800 × 700 without horizontal
overflow or pushing the composer beyond the window. Current pending, queued,
loading and narrow screenshots were inspected. All inference checks used a
local fixture and isolated profiles.

The final build's two expanded attachment checks passed in 16.1 s, including
readable errors with no IPC method names or staging identifiers. Command:
`npm run test:e2e --prefix desktop -- attachments.spec.ts --reporter=line --output=node_modules/.cache/attachment-final-clean-errors`
with the same packaged executable. The final narrow error screenshot was
inspected after the copy cleanup.

**Verdict: Approve for the exercised macOS desktop attachment scope.** Native
Windows/Linux execution and assistive-technology checks were not performed in
this pass. Clipboard image paste remains outside the implemented capability.

## Rollback

The host/runtime delivery boundary is commit `bc1a65693`. The following
composer commit depends on that boundary. Revert the composer surface before
reverting its transport; staged copies are private data and expire separately
from a code rollback. Strict revision JSON remains compatible throughout.
