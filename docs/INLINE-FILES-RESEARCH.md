# Files inside Gator chat

Research and implementation decision, 22 October 2026. **The user approved A
and B: inline file workflows plus native Word, Excel and PowerPoint content
editing.** Implemented in the existing Electron/React app. This supersedes the
initial narrower scope below; the preimplementation audit is retained as a
record of why canonical file services were needed.

## What the user means

Keep the conversation as the working surface. A file supplied by the user or
created by Gator should be something they can open, read, change themselves,
and refine with Gator without leaving that conversation. The user explicitly
requested this inside chat. An inline file card should expand in its message;
the right-hand inspector is not the proposed destination for this interaction.

The existing visual language, command palette and concise message layout stay.
Only messages with actual files get file cards; ordinary replies gain no extra
file/context sections. No new global artifact tab or file-manager screen is
needed for the proposed workflow.

## Official references inspected

| Reference | Confirmed behavior | Lesson for Gator |
| --- | --- | --- |
| [ChatGPT Work: Work with files](https://learn.chatgpt.com/docs/artifacts-viewer?surface=app) | Generated documents, spreadsheets, presentations and PDFs can be reviewed with the conversation. Desktop previews sit alongside chat; Work on the web supports reviewing generated files in chat, download and targeted feedback. Supported annotations supply a selected area as context for a revision. | The result is reviewable work. A request to change a passage or cell should carry its exact file/selection, rather than making the user re-describe it. |
| [Claude: What are artifacts and how do I use them?](https://support.claude.com/en/articles/17153992-what-are-artifacts-and-how-do-i-use-them) | Artifacts normally open beside the conversation. Template artifacts can be edited directly or through Claude; a highlighted passage in a doc/Markdown document can be sent to Edit with Claude. | Direct manipulation and conversational revision are complementary controls on the same file. |
| [Claude Docs](https://support.claude.com/en/articles/16923645-get-started-with-claude-docs) | Users can type directly, ask for edits in the conversation, or request edits through comments. Direct edits save automatically. Export includes Word, PDF and Markdown. | A document should feel like a document. Preserve changes and make the current editable version clear. |

The inline placement below is Gator's proposed adaptation to the user's
instruction, not a claim that every reference uses inline placement or offers
identical editing for arbitrary uploaded Office files. Native Office editing,
rich-text documents, preview rendering and agent-driven file revision are
different capabilities and should be presented accurately.

## Before implementation: Gator capability

| Capability | Current state | Evidence |
| --- | --- | --- |
| Attach inputs to chat | Available: selected files become private copies and stay with drafts/queued messages | `desktop/src/Attachments.tsx`, `desktop/electron/uploads.mjs` |
| Input file preview/edit | Not available in the attachment UI; chips and sent filenames are not document editors | `PendingAttachments` and `InputFiles` in `desktop/src/Attachments.tsx` |
| Generated file cards | Available in the transcript | `desktop/src/OutputCards.tsx` |
| Generated file preview | Opens the right-hand WorkDetails preview; Markdown renders, other available preview content is preformatted text | `desktop/src/App.tsx` (`openOutput`), `desktop/src/WorkDetails.tsx` |
| Preview coverage | Bounded retained representations: Markdown/text, formatted JSON, CSV sample, extracted DOCX text/PPTX outline, and metadata for PDF/XLSX/binary types as available | `internal/artifact/preview.go`; desktop projection can further restrict coverage |
| Direct file editing in the app | No inline editor or canonical file-save action in the current preview workflow | `desktop/src/WorkDetails.tsx`, `desktop/src/protocol.ts`, `desktop/electron/main.mjs` |
| Ask the agent to change files | Existing task tools can work on permitted files; there is no selection-to-revision interaction attached to these preview cards | Current work runtime versus the preview UI |

The existing preview strings cannot become an editor's source buffer. They may
be truncated, sanitized, formatted or extracted from another file format. A
real editor needs full original content, durable file identity, revision-aware
saving and accurate format support. Historical output bundles must remain
verifiable; editing them in place would invalidate their recorded contents.

## Approved interaction

1. **Open a file in its message.** Click its compact card or attachment name.
   Expand a document surface directly below the filename, retaining the rest
   of the chat and the main composer. Collapse returns to the same card.
2. **Read or edit.** A small header shows filename, version and Preview/Edit.
   Document editing uses document typography and a modest formatting toolbar;
   code/JSON use a source view; CSV uses a table. Show Edit only when that exact
   format is supported. Large previews can scroll within the expanded surface.
3. **Make a focused request.** Select a passage or table range and choose
   Ask Gator. Stage a request with the file, version and selection attached;
   keep the normal chat send/queue behavior. The user can also ask for a whole
   file revision. A follow-up should target the version the user is reading.
4. **Save and revisit.** Save/Discard should be explicit in the first release,
   with unsaved work retained while moving through the conversation. Save
   creates a new working version, used by subsequent requests. A quiet version
   menu permits comparison/restoration; label user edits honestly. Keep original
   uploads and historical outputs intact. Export/save-back to an original local
   file is a separate, explicit destination choice.

Only the current file view needs controls; don't repeat a large toolbar below
every response. The expanded surface should not automatically take keyboard
focus or follow the latest agent update while someone is editing. Keep the
existing Gator palette, radii, typography, focus rings and reduced motion.

### Suggested format scope

| Format | Proposed first pass | Separate larger capability |
| --- | --- | --- |
| Text and Markdown | Complete preview and direct document-style editing | Advanced layout features |
| Code and JSON | Full content, source editing and syntax-appropriate checks | IDE/project editing |
| CSV | Table preview and direct cell edits | Spreadsheet formulas, charts and Excel round trips |
| Images and PDF | Real visual previews, download, whole-file revision requests where supported | Image editing tools, PDF text/layout editing and visual annotations |
| Word, Excel and PowerPoint | Explicitly labeled extracted/read-only preview where supported; agent revision requests can be a later increment | Faithful Office rendering and direct native-format editing need an explicit scope decision |

This table records the original scope proposal. The user subsequently chose
both options, including native Office content editing; the implemented scope
is specified below.

## Engineering decisions

- Reuse conversation file identities and the current output cards, but add
  canonical content/revision operations. A renderer-only textarea would not
  fulfill editing, persistence or agent awareness.
- Preserve original inputs and sealed outputs. Store edits as derivative
  versions; use the current revision for agent follow-ups and export.
- Detect an externally changed source or a competing save, retain the local
  edits, and offer reconciliation rather than overwriting newer work.
- Stream/render previews by format without loading every file into every
  chat snapshot. Bind requested content and save operations to the selected
  conversation and exact file.
- Keep file edit drafts independent of the ordinary message draft. Review
  navigation, reload, cancellation, failed saves and queue capture explicitly.
- Introduce the content service, inline preview, editing/version persistence,
  selection-based revisions and richer formats in rollbackable commits.

## Implemented scope

| Format | Available inside the file card | Limits |
| --- | --- | --- |
| Text, Markdown, code, JSON | Full content, Preview/Edit, source editing; Markdown formatting controls and rendered preview; JSON syntax validation | UTF-8 text; HTML/SVG stays inert source |
| CSV/TSV | Editable cells, value bar, add rows/columns, keyboard navigation, paging | 10,000 rows, 256 columns, 100,000 cells; quoting and newline style preserved |
| Images | PNG/JPEG/WebP visual preview, export and whole-file requests | No pixel editing |
| PDF | Bundled PDF.js page rendering and pagination | Preview only; no PDF layout editing or annotations |
| Word `.docx` | Paragraph and table-cell text, simple run formatting, embedded images | No advanced page layout, tracked-change tools or object manipulation; protected documents are read only |
| Excel `.xlsx` | Sheet tabs, values/formulas, value/formula bar, rows/columns, paging, selected-cell requests | Protected sheets are read only; no calculation engine; after edits cached formula results are cleared and Excel is asked to recalculate on opening the exported workbook |
| PowerPoint `.pptx` | Slide pagination, simplified positioned shapes/images, text in existing shapes | No shape positioning/theme design tools; masters, media and layout parts remain in the native package |

The editor reads at most 16 MiB of original or generated content. Office ZIPs
also have entry, uncompressed-size and content limits. Signed/macro-enabled
packages cannot be changed. Existing chat attachment limits remain four files,
4 MiB per file and 8 MiB total. An edited file outside those limits can still be
saved and exported; the UI reports that it could not be added to the next
message. Empty text can be saved/exported but cannot be sent as an attachment.

Selection requests currently carry selected text or one selected cell, not a
multi-cell range or visual annotation. A prior version can be opened and
restored as a new version; there is no side-by-side semantic diff viewer.

## Storage and authority

- The host authorizes each file reference against actual current-conversation
  input IDs, current draft attachments, or verified output manifests. Renderer
  paths alone cannot select arbitrary local files.
- Full content is opened on demand through typed preload IPC. Retained display
  excerpts never become save buffers. An older unavailable output may still
  show its retained excerpt, explicitly labeled, without Edit controls.
- Private copies, immutable versions and edit drafts live in
  `GATOR_STATE_DIR/gator/files`. Edits do not mutate original uploads or sealed
  Work bundles. Output identities use profile-relative bundle locations so
  versions survive a backup restored into another folder. Uploaded derivatives
  keep aliases to their version history.
- File drafts persist independently of message drafts. Optimistic revision
  checks and an exclusive save lock reject stale writes. Discard can reload the
  latest version without deleting another editor's newer draft. The UI keeps
  unsaved edits visible and blocks Send/removal until Save or Discard.
- Save includes the resulting bytes in the next message when limits permit.
  Ask Gator stages exact saved bytes plus filename, version and selection.
  Neither automatically submits a message. Agent tools still use the existing
  work contracts and approval paths.
- History/all-private-content deletion includes the retained copies, versions,
  aliases and file edit drafts. Backups include those files, excluding transient
  locks/temp files and credentials. Automatic age retention explicitly keeps
  file versions/drafts until the user deletes history or all content. Electron
  composer drafts and short-lived upload staging retain their existing separate
  lifecycle; exporting a private-content backup is not a backup of those
  Electron preferences.
- No model or remote document-rendering request occurs merely to open or edit
  a file. Cloud model follow-ups still send the selected attachment/context to
  the configured model. HTML stays source; PDF workers run from bundled assets.

## Verification

Detailed checks and the scoped interface review are recorded below after the
packaged-app pass. Independent native-format verification used full documents
created by python-docx 1.2.0, openpyxl 3.1.5 and python-pptx 1.0.2 in a temporary
venv, outside this repository. Each library reopened its edited file and read
back the changed content. Word run formatting/table content, Excel number
format/chart, and PowerPoint untouched placeholder/master parts were preserved.
This is format round-trip verification, not a claim of pixel-identical rendering
or a manual test in Microsoft Office.

## Interface review

**Mode:** full, scoped to the new file workflow. React DOM, existing native CSS
variables, Lucide icons and preload IPC. This is not another audit of unrelated
Gator screens. The app keeps its established dark theme under either OS color
preference; white Office/PDF paper is deliberate document content.

| Category | Evidence inspected | Result |
| --- | --- | --- |
| Typography | File titles, Markdown/source editors, Word/slide text, formula bar, version dates | Existing font stack and tokens; source/formulas use monospace, numeric metadata uses tabular figures |
| Surfaces | Pending, sent-input and generated cards; editor toolbars, paper/table/PDF surfaces, history, Ask Gator, empty/error states | Nested radii and structural borders; bounded scroll keeps composer controls reachable |
| Animations | File chevron open/close, interrupted reversal, inherited button feedback, reduced motion | Restrained 140 ms chevron; no animated document reflow or per-keystroke entrance |
| Icons | File, preview/edit, export, history, save, ask, pagination, Markdown and row/column controls | Existing Lucide/currentColor conventions; controls have accessible names and selected/disabled states |
| Performance | Main IPC projection, lazy PDF bundle/worker, bounded native parsers, grid paging, edit drafts | File bytes load on demand, not in every backend snapshot; large tables page instead of creating a full-sheet DOM |

### Changes and resolved findings

| Severity | Location | Before | After | Why |
| --- | --- | --- | --- | --- |
| HIGH | `desktop/src/Attachments.tsx:41`, `desktop/src/OutputCards.tsx:5` | Filename chips or extracted previews could not support direct editing | Compact cards open full authorized content on demand | The editable buffer must represent the actual file |
| MEDIUM | `desktop/src/WorkDetails.tsx:109`, `desktop/src/App.tsx:131` | A file already in chat opened another primary preview surface | File index entries open/focus their exact card in the transcript | Keeps work in the conversation and preserves provenance |
| HIGH | `desktop/src/FileSurface.tsx:45`, `desktop/electron/files.mjs` | No canonical Save, conflict handling, recoverable edit draft or version history | Explicit Save/Discard, immutable versions, optimistic saves, crash-lock recovery and history restore | Prevents silent loss and stale overwrites |
| HIGH | `desktop/src/App.tsx:145`, `desktop/src/Attachments.tsx:41` | Sending/removing a file could use old bytes during direct editing | Dirty edits block Send/removal; Save replaces the attachment in place and uses current bytes | Prevents a mismatch between the visible editor and what Gator receives |
| MEDIUM | `desktop/src/styles.css:896` | Several attachments plus an expanded editor pushed Send outside the window | Bound pending-file height, full-width expansion and independent document scrolling; hide the greeting while an attached file is open | Maintains a usable composer in normal and narrow windows |
| MEDIUM | `desktop/src/FileGrid.tsx:6`, `desktop/src/TextFileEditor.tsx:4` | Source-only Markdown and isolated spreadsheet inputs | Modest Markdown tools, value/formula bar, rows/columns and keyboard navigation | Supports direct manipulation without introducing an IDE or full Office ribbon |
| MEDIUM | `desktop/src/FileSurface.tsx:110`, `desktop/src/styles.css:858` | Embedded images sat inside editable Word text; paper used the browser's blue focus treatment | Images remain outside editable paragraphs; paper has a legible green focus ring | Text editing does not imply image deletion and keyboard focus stays visible |
| MEDIUM | `desktop/src/PDFPreview.tsx:6`, `desktop/electron/file-content.mjs` | Document metadata/extracted text stood in for visual previews | Bundled PDF pages and actual image previews, with no Edit control for unsupported editing | Accurate capabilities and reviewable output |
| LOW | `desktop/src/styles.css:821` | New surfaces could introduce unrelated spacing, borders, controls or motion | Reuse Gator tokens, 40 px desktop controls, contained outlines and reduced-motion rules | Keeps the feature consistent with the existing product |
| MEDIUM | `desktop/src/FileSurface.tsx:67`, `desktop/src/App.tsx:357` | Missing/stale files and failed saves had no file-specific recovery path | Retained excerpts are labeled, errors stay inline, edits remain, navigation explains draft retention | Failure states preserve the user's work and explain what is actionable |

### Considered but rejected

| Candidate | Reason |
| --- | --- |
| Move the editor into a new global artifact tab or primary right-hand panel | Conflicts with the user's explicit inline-chat direction; the existing file index remains a navigation aid |
| Render arbitrary HTML/SVG as executable previews | Source editing is useful without introducing an executable document surface |
| Upload Office files to a rendering/conversion service | Unnecessary for the approved content-editing scope and would introduce a new data destination |
| Build full Word layout, Excel calculation/chart design and slide-shape tools in this pass | Those are larger editing products; preserve native package parts and clearly describe the supported content controls |
| Animate document height or every editor change | Disrupts reading position, typing and large-table performance; only the disclosure indicator needs motion |

### Verification and verdict

All interactive checks used isolated profiles and local fixture providers.
Real user files, credentials and InvestmentGini repositories were not changed.

- `go test` passed for `internal/worksession`, `internal/workapp`,
  `internal/workrun`, `internal/attachment`, `cmd/gator` and `internal/lifecycle`
  across the scoped runs. The new lifecycle test checks backup inclusion and
  explicit history deletion of versions/drafts; runtime tests cover restored
  input references and verified artifact digests. After updating the bundled
  app guide, `go test ./internal/workrun -run
  'TestApplicationContext|TestConversationReceivesCurrentAppKnowledgeAfterModelChange'
  -count=1` passed again.
- `npm run check --prefix desktop` passed.
- `npm test --prefix desktop`: **43 passed, one Windows-native test skipped on
  macOS**. File-service checks cover originals, edits, reload recovery, stale
  saves, restored versions, alias history, backup relocation, active/abandoned
  locks, unauthorized references, symlinks, bad digests and native Office parts.
- `npm run pack --prefix desktop` built the macOS runtime/UI/app. An initial
  archive made while source files were changing failed the packaged launch
  check; rebuilding from a fixed snapshot resolved it. The packaged main,
  file-service and Office modules were compared byte-for-byte with source.
- With `GATOR_DESKTOP_TEST_EXECUTABLE` pointing at the packaged app,
  `npm run test:e2e --prefix desktop -- inline-files.spec.ts files.spec.ts
  work-details.spec.ts attachments.spec.ts --reporter=line
  --output=node_modules/.cache/inline-files-packaged-results` passed **all eight
  tests**. These cover genuine Go-generated output manifests and edited bytes
  reaching the next model request, in addition to display-only navigation
  fixtures and attachment/queue regressions.
- After the final layout and attachment-identity fixes, the rebuilt packaged
  app passed all four tests in `inline-files.spec.ts` again
  (`--output=node_modules/.cache/inline-files-final-packaged`). That final suite
  also checks that removing an earlier neighboring
  attachment preserves the open editor, its unsaved bytes and the Send guard.
  Native exports are inspected for the actual edited Word/slide text and Excel
  values/formulas; original uploads and sealed outputs remain byte-identical.
  PDF pages draw real pixels, paginate, expose no Edit control and make no
  external rendering requests. File edits alone make no model requests.
- Independent full-package Office round trips passed with python-docx,
  openpyxl and python-pptx, as described above.
- The production dependency audit reported zero advisories. This is a
  dependency check, not a general security assurance.
- Visual inspection covered normal/narrow windows, hover, keyboard focus,
  empty text, failed JSON save, native Office paper, PDF pagination, history,
  selected-cell requests and attachment replacement. A temporary Playwright/CDP
  review slowed the disclosure motion to **10%** and interrupted/reversed it.
  Reduced motion, the fixed dark theme under the OS light preference, and the
  existing contrast/mono variants were checked. At 800 × 650, Send stays
  inside the viewport and the document does not cause horizontal page overflow.

**Not verified:** native Windows/Linux GUI behavior for these editors, manual
screen-reader use, Microsoft Office visual fidelity, complex Office layouts and
all multilingual/embedded-font PDF variants. Fast opening/loading states were
inspected in code and during use, but were not held open for a dedicated visual
snapshot. Those checks do not imply unsupported editing features are available.

**Verdict: Approve for the reviewed macOS scope.** No actionable interface-polish
findings remain in that scope; the unverified platform and native-application
checks above remain explicit release verification limits.
