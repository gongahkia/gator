# Competitive gap implementation

Objective: implement the code changes in the [22 October gap audit](COMPETITIVE_GAP_AUDIT_2026-10-22.md), with atomic commits and engineering, product and desktop verification. This ledger retains the entire scope across implementation sessions. An implementation entry is not a claim of successful live use.

## Requirements and evidence ledger

| Gap | Work and completion evidence required | Status |
| --- | --- | --- |
| G01 | Repeatable live/product evaluation, failure attribution, matched baseline and unseen cases; multi-day follow-up evidence | Pending |
| G02 | Independent task ownership; browse and converse during other work; original-task steering, approvals and results; bounded concurrency | Pending |
| G03 | Durable origin-linked updates in chat; delivery/read/resolution separation; reply routing; no duplicate delivery or execution | Pending |
| G04 | Reviewed conversational responsibilities with sources, timing, expiry and attention; actual save, pause and guarded edits | Pending |
| G05 | Task-led onboarding, account setup/health/reconnect without Terminal, understandable model/access choices | Pending |
| G06 | Memory receipts/corrections and clear learning/history/privacy controls; cross-chat/restart/conflicting-history and multilingual evaluation | Pending |
| G07 | Ground-truth alert/miss evaluation, conversational notification conditions, failed-check visibility and quiet behavior | Pending |
| G08 | Complete selected connected jobs, reviewed delivery, account checks and receipts; uncertain send reconciliation | Pending |
| G09 | Durable admission/queue, explicit interrupted states, recovery without replaying external effects; crash-transition tests | In progress: admission and queue |
| G10 | Representative Office/file fidelity, precise revisions/version comparison, larger bounded intake; native opening checks | Pending |
| G11 | Integrated browser readiness/state/private sign-in/takeover, scoped browsing authority and representative workflow tests | Pending |
| G12 | Paginated/searchable chats, rename/archive/pin, scoped cross-chat files; retrieval over hundreds of records | Pending |
| G13 | Release/update lifecycle, supervisor drain/restart and native platform gates; signed distribution requires external accounts | Pending |
| G14 | Authenticated remote execution option with explicit ownership/availability; client-off completion and local-resource boundaries | Pending |
| G15 | Authenticated remote conversation/update/decision surface, revocation, offline delivery and deduplication | Pending |
| G16 | Useful conversational voice/steering, interruption and transcript correction; inspectable personal style | Pending |
| G17 | Save/test/version/revert reusable procedures from successful tasks; skills remain separate from permissions | Pending |
| G18 | Latency/resources/cost metrics, long-history and low-resource checks, accessibility and multilingual product verification | Pending |

## Execution order

1. Persist message admission and independent task ownership; route results and recovery to their chat.
2. Add conversational responsibilities and account activation, then prove a complete follow-up workflow.
3. Improve memory, retrieval, files, browser and reusable workflows using those journeys.
4. Complete update/platform engineering and authenticated remote execution/client surfaces.
5. Verify voice, performance, accessibility and all acceptance scenarios; retain explicit external evidence gaps.

Use the current React/CSS/Lucide conventions, neutral text-field focus, keyboard command palette, compact chat and inline files. Do not revive the retired user CLI. Credentials, production signings, real-account operations and independent user judgments cannot be manufactured by code changes; record their status accurately while completing all independent work.

## Completed changes and verification

- Admission storage: private, bounded per-chat message journals; atomic generation checks for edits/reorders/admission; independent execution leases released by process exit. `go test -race ./internal/worksession` passed, including concurrent writers, stale changes, corruption, retained options and cross-process ownership. Desktop integration/recovery behavior is still in progress.
