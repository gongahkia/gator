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
limits; malformed files are rejected by the canonical loader. Existing model
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

## Review and verification

Full scoped interface review and packaged verification are in progress.
The Go input, application and optional sidecar checks passed. Host tests
passed (30 passed, one Windows-only handoff check skipped). UI typecheck and
the runtime/UI build passed. macOS packaging completed. Native Windows/Linux
execution and assistive-technology checks have not been repeated.
