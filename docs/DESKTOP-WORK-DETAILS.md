# Compact work details

Decision: adopt a compact, optional inspector beside the desktop conversation.
Recent conversations stay in the left navigation; the transcript and exact
approval requests stay in the center. This is a presentation change and
read-only projection of existing Go-owned state, not a new agent runtime.

## Meaning of the information

- **Context used** means the occupancy of one particular model request's
  context window. It needs the effective model capacity, the actual request
  contents, measurement method, and snapshot time. Daily token totals and
  summed run usage do not measure occupancy. This iteration says **Not
  measured**, with no invented percentage or capacity. Selected attachments
  and recorded sources are labeled separately; neither proves everything
  currently loaded into the model. Existing saved-context inspection remains
  available through Go's context view.
- **Connection selection** means a connector ID selected for a new request.
  Queued requests retain their own selections. It does not establish sign-in,
  account identity, current OAuth scope, consent to a read, or approval of a
  write. **Retained connection sources** come only from a verified retained manifest's
  connector provenance. These records can be inherited from earlier turns and
  do not establish fresh use in the latest request. An absent record does not
  rule out actions or other use outside this provenance. No new
  connection toggles, login probes, or permission grants are introduced.
- **Outputs** are files and change candidates projected from verified retained
  bundles, identified by bundle plus file/candidate identity. Include earlier
  runs in the selected conversation lineage, not just its latest bundle.
  **Checks passed** reports the existing deterministic validation flag; it
  does not promise correctness, external delivery, or application of a patch.
  Failed runs may retain inspectable files and must be labeled accordingly.
  Delivery is not inferred from assistant prose, filename, or run completion.
- **Previews** are bounded representations produced by the existing artifact
  preview service after verification. HTML and SVG are displayed as source,
  never executed. Binary formats can have metadata-only previews. Historical
  preview bytes describe the verified retained snapshot, not a continuously
  monitored live file. No arbitrary filesystem reads are exposed to React.
- **Workspace** describes the selected local folder, retained snapshot, model,
  goal association and requested mode. Automatic mode is not presented as a
  read-only grant. No remote execution/control switch is invented.

## Presentation

Use independently collapsible Outputs, Context, Connections, Workspace and
relevant Follow-up checks sections. Outputs start open; diagnostic sections
start closed. Preserve disclosure choices when hiding and showing the rail.
Do not automatically open it on provider updates or replay an entrance while
streaming. Preserve the composer and transcript while previewing an output.
On narrow windows use a dismissible drawer with keyboard focus management.
The inspector never approves, sends, saves, applies, or replays an action.

Recent filtering is local to the bounded recent list, clearly labeled, and
does not claim to search full conversation contents. Searching reveals matches
inside collapsed or shortened groups without changing the user's group
preferences. Keep the existing Go-validated restore path and one full
conversation browser; pin/rename/archive remain separate product decisions.

## Verification boundary

Exercise empty and populated sections, unavailable telemetry, multiple runs,
failed validation/run, repeated filenames, binary/source/truncated previews,
recorded versus selected connectors, conversation changes, draft and reading
position preservation, streaming updates, drawer keyboard behavior, reduced
motion, and narrow layouts using isolated profiles. No paid inference, real
account probes, or user profile changes are needed.

Implementation commits and verification results are recorded in
`DESKTOP-CODEX-AUDIT.md` after the work is complete.
