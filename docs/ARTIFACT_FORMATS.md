# Rich Work artifacts

Gator renders binary office artifacts from provider-independent semantic
specifications. Models never write raw ZIP or PDF bytes.

## Documents

`work_write_document` accepts a version-1 document with a title, theme, and
blocks. Supported blocks are headings, paragraphs, callouts, bulleted and
numbered lists, tables, and page breaks. The same specification renders to
DOCX or PDF. Built-in themes are `professional`, `minimal`, and `report`.

An optional DOCX template may contain `{{gator:title}}` and `{{gator:body}}`
placeholders or content controls tagged `gator:title` and `gator:body`.
Templates with macros, binary parts, or external relationships are rejected.
PDF uses Gator's deterministic layout and embedded fonts rather than claiming
pixel-identical Word conversion.

## Workbooks

`work_write_workbook` supports multiple sheets, typed values, formulas, number
formats, column widths, frozen rows, filters, styled tables, and bar, column,
line, and pie charts. An existing XLSX can be used as a template. Macro-enabled
or externally linked workbooks are rejected before rendering. New workbooks
stream sheets larger than 10,000 rows; inputs remain bounded at 100,000 rows,
256 columns, and 64 sheets.

DOCX, XLSX, and PDF requirements receive structural validators automatically
from their artifact path. Preview surfaces report document text/page counts or
workbook sheet counts without printing binary content.

Each semantic render also contributes trusted manifest evidence: output path,
format, renderer version, semantic-spec digest, optional frozen-template
digest, and the exact generated artifact digest. Sealing rejects stale evidence
if the file was replaced after rendering.
