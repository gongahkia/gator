# Retained-source citation chips

Assistant file references and explicit evidence references render as compact numbered chips. Hovering or focusing a chip reads a bounded retained excerpt; clicking it, or **View in source**, opens the existing source sidebar. Ordinary unretained web links remain ordinary links. A source reference is not proof that a generated claim follows from it.

## Supported evidence

- Local `source/...` references continue to resolve within the answer’s exact run. The sidebar distinguishes cited lines from verified tool read ranges, including partial first/final columns. The live workspace is never used as a fallback.
- `[Title](gator-evidence:ACTUAL_ID)` resolves a sealed manifest evidence ID, supporting retained web captures and connected-service responses. The agent prompt requires actual tool/catalogue IDs and prohibits invented ones.
- A normal web URL becomes a citation only when it uniquely matches a retained evidence record. Multiple captures of one URL or connector operation require an exact evidence ID; no latest/active fallback is allowed.
- Source metadata includes the backend-generated locator/kind, bounded label and capture timestamp when available. Normalized web previews recorded in the private replay can be shown as the preview actually received by Gator. They are labeled separately from exact raw-byte read ranges.
- Binary attachments without a retained text extraction and PDFs without trustworthy page mappings explicitly report unavailable/unsupported capture. This feature does not infer page numbers from model text or implement PDF-page mapping. Connected response excerpts are retained data, not proof that every field was inspected.

User prompts and code blocks keep their text; a user’s file path is not replaced with an authoritative assistant citation. Source output is rendered as React text, including HTML-looking content. There are no remote images, favicon requests, automatic network fetches, model calls, or memory writes involved in displaying a citation.

## Existing authority and storage

`desktop/electron/source-preview.mjs` uses the existing sealed Work manifest/evidence snapshots. `gator:source-catalog` exposes only bounded metadata for the current conversation/run; `gator:source-preview` validates references, snapshot paths, manifest/content digests and the selected owner again before returning text. Both use the existing trusted-window IPC boundary. No second evidence store or source authentication scheme was added.

Catalogues are limited to 256 references and approximately 60 KiB. Metadata loading is deferred until the answer nears the visible viewport. Previews read at most 512 KiB of retained source and return at most 400 lines / 64 KiB. Hover cards show at most 1,200 characters, preferring the cited/read passage or a clearly labeled observed normalized preview. Original URLs are limited to validated HTTP(S) links without URL credentials, opened only by explicit user action.

The renderer scopes loaded metadata, hover results and sidebar results to the exact conversation/run/reference/selection. Closing and reopening a hover preview revalidates the retained source. A late response cannot overwrite another selection, and the previous source is not rendered under a new source identity during the effect transition. Missing sources, modified bytes, unsupported formats and stale conversation/owner state produce explicit errors.

## Verification

- `node --test desktop/tests/source-preview.test.mjs`: 13 passed. Covers exact local ranges, tamper/link/wrong-chat rejection, bounded UTF-8 data, web/connected evidence IDs, ambiguous captures, unknown references, unsafe URLs, partial byte-page columns, normalized preview versus raw reads, response revocation, oversized catalogues and unsupported/missing binary/PDF text.
- `npm run check --prefix desktop`, `npm run build --prefix desktop`, and final `build:ui`: PASS.
- `npm test --prefix desktop`: desktop 29 passed / 1 opt-in skipped; shared host 75 passed / 1 skipped. A subsequently added normalized-preview host test passed in the final 13-case focused host run; those counts are not a claim that the earlier whole Node run included it.
- `go test ./internal/workrun -run 'Test.*Prompt|Test.*Evidence|Test.*WebPreview|Test.*Claim'`: PASS.
- Real Electron `source-links.spec.ts`: all 6 passed. Includes original retained local-file preview after the current file changes, hover/focus excerpt, exact cited passage, safe connected JSON rendering through real Go/fixture HTTP retrieval, explicit unknown evidence, delayed A/B host-response races, compact/reduced-motion display, and existing link-edit/effort regressions.
- The delayed-response test explicitly uses an adversarial IPC response fixture. The connected/local cases use the real Go backend with scripted model responses and private local services. Web boundary cases use retained filesystem fixtures. No commercial model evaluation, live external account, physical device or PDF page-level native acceptance is claimed.
- Formatting/diff checks passed. Whole-product acceptance remains due after the activity, message-branch and action-plan stages.

Interface review: existing React/CSS/Lucide conventions were retained. Typography uses existing sans/mono tokens and tabular citation numbers; surfaces use the shared raised/shadow tokens and bounded scrolling; no new animation was added; icons/labels preserve keyboard names; metadata and excerpt reads are bounded and do not trigger network activity. A large editor/HTML renderer and 40 px inline citation boxes were considered and rejected because they add runtime/authority or disrupt paragraph reading. The larger excerpt action remains available for inspection. Screenshot review covered the compact dark view; light-mode and whole-product regression coverage remains part of final acceptance.
