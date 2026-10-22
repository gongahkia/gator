# Files inside Gator chat

Research and proposed direction, 22 October 2026. **File-workflow implementation
is awaiting the user's verification.** Restoring the existing ASCII atmosphere
in chat is separately authorized and does not authorize this feature.

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

## Current Gator capability

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

## Proposed interaction for verification

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

This is a proposed initial scope, not an approved exclusion. If direct Word,
Excel or PowerPoint editing is central to the desired experience, include it
in the design before implementing the simpler formats.

## Engineering consequences after verification

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

## Verification needed from the user

Confirm the inline interaction and choose the first format scope: the suggested
text/Markdown/code/JSON/CSV editing plus image/PDF previews, or a larger first
pass including native Office editing. A visual prototype can precede either.
No file preview/editor implementation is authorized until that confirmation.
