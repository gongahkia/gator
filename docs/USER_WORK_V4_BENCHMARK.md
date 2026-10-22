# User Work benchmark, fifty-task corpus

**Interface status:** Gator is a desktop app. Shell examples below are source-checkout developer operations with an explicit internal opt-in; see [development instructions](DEVELOPMENT.md). They do not install or enable a public CLI.

Recorded 2026-10-16. [Version 4](../internal/eval/testdata/user-work-v4/dataset.json) preserves the 30 version 3 cases and adds 20 jobs across documents, design, spreadsheets, presentations, research, and file organization. The new cases have 14 development and 6 held-out tasks. The full corpus has 37 development and 13 held-out tasks. The four inherited version 2 held-out cases were inspected in earlier live work; they are no longer unseen.

The added range follows [Anthropic's Cowork examples](https://support.claude.com/en/articles/13345190-get-started-with-claude-cowork), which include organizing files, synthesizing research, producing reports and documents, creating formula-driven spreadsheets and charts, and making presentations. Anthropic's [product guide](https://claude.com/resources/guides/claude-cowork-product-guide/product-overview) also describes cross-file knowledge work, while its [Cowork use-case library](https://academy.claude.com/cowork/use-cases) includes branded content and design specs. These sources informed task selection; they are not a claim that Gator matches Cowork's breadth or quality.

| New task | Inputs | Deliverable | Main thing measured |
| --- | --- | --- | --- |
| `qbr-pdf-to-memo` | PDF account review, account note | DOCX executive memo | PDF extraction, adoption arithmetic, open-ticket qualification |
| `incident-postmortem` | Timeline CSV, investigation note | DOCX postmortem | Structured timeline, owners, unconfirmed cause |
| `client-proposal-pdf` | Discovery notes, price sheet | Client PDF | Scoped proposal, approved price, optional exclusion |
| `grant-progress-report` | Grant terms, program data, notes | DOCX report | Distinct denominators and observed outcomes |
| `nda-deviation-memo` | Synthetic draft and playbook | DOCX deviation review | Clause comparison, redlines, counsel handoff |
| `workshop-facilitator-guide` | Agenda, access needs | DOCX guide | Timed run sheet and usable offline/access options |
| `brand-campaign-kit` | Brand guide, approved facts | HTML, email, ad copy | Consistent voice and early-access qualifier across assets |
| `accessible-event-poster` | Event details, palette | SVG and alt text | Concrete visual asset with legible access details |
| `dashboard-prototype` | Metrics snapshot, design tokens | Self-contained HTML prototype | Responsive layout, text status, keyboard-usable filter |
| `design-spec-from-feedback` | Interviews, product constraints | DOCX spec and SVG flow | State design, accessibility, feasible scope |
| `scenario-model-workbook` | Event registrations, show rate, staffing limits | XLSX and recommendation | Input-linked formulas, capacity, and event cost cap |
| `funnel-chart-workbook` | Four weeks of funnel data | XLSX chart and findings | Formula rates, chart, cautious interpretation |
| `receipt-expense-sheet` | Three PDF receipts and policy | XLSX and memo | PDF-to-sheet extraction and partial meal exclusion |
| `sprint-tracker-workbook` | Tasks CSV, capacity note | XLSX and handoff | Dependencies, status, formula-driven summary |
| `research-to-board-deck` | Saved announcement, adoption table | Four-slide PPTX | Visible facts, rumor qualification, decision request |
| `deck-update` | Existing PPTX and update note | Edited PPTX | Preserve existing deck while replacing stale figures |
| `downloads-batch-rename` | Three inconsistently named notes | Organized copies and CSV map | Date-first naming, collision handling, content preservation |
| `account-research-brief` | CRM snapshot, saved site, call note | Account brief | Cross-source call preparation with unknown budget kept unknown |
| `weekly-wrap` | Five daily operations notes | Weekly summary | Deduplicate incident mentions and carry open actions forward |
| `interview-synthesis` | Four qualitative interviews | Findings and JSON themes | Shared needs, conflicting requests, sample-size caution |

Four small PDFs and one deck source were generated with [the fixture generator](../scripts/generate_user_work_v4_fixtures.go). All other fixtures are local text/CSV. The source folders are intentionally synthetic and modest in size, making these calibrated probes rather than a representative production workload. Saved public pages test source interpretation; live search, browser actions, connected apps, and scheduled execution remain separate gaps. The file-organization case writes copies to an isolated output bundle, so it does not test changing a real desktop folder.

## What the checks mean

All 50 cases pass scripted execution through Gator's Work service. Scripted turns contain reference tool actions, so this proves fixture loading, artifact creation, contract validation, and deterministic grader wiring. It **does not** measure autonomous model success. There is no live version 4 pass rate yet; the earlier version 2 score of 11/21 belongs only to its original live run and mixed provider/task failures.

Version 4 adds graders for extracted DOCX/PDF text, actual spreadsheet formulas, and chart presence. Existing graders inspect visible PPTX slide text, JSON fields, workbook cells, and text/CSV outputs. The [review rubric](../internal/eval/testdata/user-work-v4/review-rubric.json) covers factual support, completion, audience fit, and manual rework. The PDF extractor may collapse spaces at line boundaries; document graders therefore use short factual spans, and reviewers must inspect the whole artifact for meaning.

For the four design cases, a reviewer should render the HTML/SVG and check visual hierarchy, small-screen legibility, color contrast, keyboard access, and brand consistency. A valid SVG/HTML file or a matching CSS token is not evidence of good design. Likewise, DOCX/PDF layout, spreadsheet usability, and slide presentation quality need human inspection. The scripted examples are reference answers, not evidence that Gator chose those outputs unaided.

The corpus exposed two product defects during calibration: new workbook creation passed a typed-nil template reader and panicked; editing a PPTX mutated an existing slide's XML while decoding text. Both paths now have regression tests. Finding those defects is useful engineering evidence, but neither fix establishes a live task pass rate.

## Run commands

```sh
go run ./scripts/generate_user_work_v4_fixtures.go
GATOR_INTERNAL_COMMANDS=1 go run ./cmd/gator work eval validate internal/eval/testdata/user-work-v4/dataset.json
GATOR_INTERNAL_COMMANDS=1 go run ./cmd/gator work eval run internal/eval/testdata/user-work-v4/dataset.json \
  --id user-work-v4-scripted --split all --report-dir /tmp/gator-user-work-v4-scripted
GATOR_INTERNAL_COMMANDS=1 go run ./cmd/gator work eval run internal/eval/testdata/user-work-v4/dataset.json \
  --id user-work-v4-live-development --split development --live \
  --provider PROVIDER --model MODEL --max-model-requests 500 \
  --timeout-seconds 3600 --report-dir /tmp/gator-user-work-v4-live-development
```

Use a new report directory for each run. A live run requires provider credentials and enough request quota; the prior Gemini free-tier quota was exhausted. Keep provider failures separate from task failures. Review development failures before running the six newly added held-out cases, and keep the old inspected cases distinct when reporting an unseen score. Repeat runs and compare against a same-model baseline before claiming usefulness.
