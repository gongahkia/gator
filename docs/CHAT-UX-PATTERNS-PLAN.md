# Chat UX patterns: implementation plan

Status: **implementation in progress**. Stages 1–2 (composer sources and retained-source citations) are implemented and have passed their focused tests. Stages 3–5 remain pending. Reviewed the current working tree at HEAD `79763f45c`, including the pending shared-coding and local-voice changes. Those existing changes are preserved. This document proposes an Electron/backend extension, not another product surface or a replacement execution architecture.

References: Shape of AI’s [connectors](https://www.shapeof.ai/patterns/connectors), [action plans](https://www.shapeof.ai/patterns/action-plan), [citations](https://www.shapeof.ai/patterns/citations), [stream of thought](https://www.shapeof.ai/patterns/stream-of-thought), and [branches](https://www.shapeof.ai/patterns/branches). Adapt the interaction ideas to Gator’s existing visual language. Do not import example products’ application code, branding or screenshot assets.

## Current support and gaps

| Pattern | Implemented foundation | Missing relative to the supplied examples |
| --- | --- | --- |
| Connectors | Native Electron Connections settings, account/token setup, access checking and removal; selected connection IDs and retained connection usage in Work details; Google, Slack, Notion, Atlassian, JSON and mapped remote-MCP backend contracts | Composer “Add sources” picker, visible selected-source chips, a clearer supported-service catalog, advanced setup separated from ordinary setup. Backend descriptors are not proof of turnkey sign-in or every service operation. |
| Action plans | Native goal plans with steps/dependencies/revisions; reviewed plan changes; exact-action approvals and durable action receipts | A first-class, editable, task-specific plan card before execution, with a revision-bound “Approve plan” action and execution-linked step states. Existing goal plans describe reported progress; they are not automatically execution state. No Zapier-style general workflow builder is claimed. |
| Citations | Assistant `source/...` references open a verified retained snapshot in the right sidebar; cited lines and actual observed read ranges are distinguished; bounded previews and tamper/cross-conversation checks | Numbered/name citation chips, hover/focus excerpt cards, a unified web/connected-record citation model, verified PDF-page/passages where extraction supports them. Generic Markdown URLs do not provide these guarantees. |
| Visible activity | Public narration interleaves with actual tool events; running/failed/stopped states, elapsed time and retained disclosures; coding diff and file inspection | Safely projected domain chips, exact changed-file links at the relevant activity event, richer research grouping and explicit links between plan, activity and evidence. No hidden reasoning display. |
| Branches/message menu | Retained Work revisions have parents and support back/forward/child selection; separate chats and reuse-message tools exist; local spoken replies exist | Per-message “Branch in new chat,” stable cross-chat ancestry, and “Read aloud” for a chosen older response. Revision navigation within one chat is not yet the requested new-chat fork. |

### Implementation evidence

- Connections: `cmd/gator/work_connections.go:229`, `internal/connector/connector.go:220`, `desktop/src/SettingsNavigation.tsx:3`, `desktop/src/WorkDetails.tsx:138`.
- Goal-plan projection: `cmd/gator/work_ui_results.go:206`; native plan inspection: `internal/tools/local_plans.go:15`; exact current action review: `desktop/src/Result.tsx:37`.
- Source references: `desktop/src/sourceReferences.tsx:15`; exact retained preview enforcement: `desktop/electron/source-preview.mjs:9`; read/citation distinctions: `desktop/src/SourcePreviewSidebar.tsx:26`; typed boundary: `app-host/contracts/sourcePreview.ts`.
- Activity: `internal/workactivity/activity.go:1`, `desktop/src/ActivityTimeline.tsx`, `desktop/src/CodeChanges.tsx`.
- Existing revision graph: `internal/worksession/store.go:49`, navigation: `cmd/gator/work_interactive.go:539`. Current assistant-message actions expose Copy at `desktop/src/App.tsx:95`; `desktop/src/MessageTools.tsx:48` is composer-level draft reuse, not a per-response branch menu.

Line numbers are audit anchors and may move during implementation.

## Delivery order

### 1. Composer sources and clearer connection settings

Add an accessible “Add sources” popover to the existing composer. List actual backend-supported, configured accounts with authentication/access status; allow selection for subsequent requests; show compact selected-account chips and a Manage connections entry. Unsupported services must not appear as functioning integrations merely because they appear in the reference screenshot.

Reuse `connectionControls`, existing settings/authentication, `work_details.connector_ids`, foreground app preference mutation rules and existing frozen request selections. Add a narrow typed source-selection/list projection if required; do not expose credentials or generic raw backend objects. Selection is not sign-in, proof of live access, or authorization to mutate. Queued requests retain their original sources. Account status must distinguish saved credentials, successfully checked access, and stale/failed checks.

Improve the existing Connections settings with per-service setup and separate advanced/custom configuration. Preserve existing host trust and operation permissions for custom HTTP/MCP connections. A “Developer mode” switch must not bypass approval, widen every connector’s authority, or blanket-disable memory just to imitate the example. Ordinary Google sign-in availability depends on the actual host OAuth configuration; expose missing setup honestly. New consumer OAuth/service integrations are separate work.

Acceptance: keyboard navigation, focus return/Escape, checked state, account removal/expiry, failed live checks, source selection during a queued request, cross-chat isolation, no credential leakage and no new execution on opening the menu.

### 2. Citation chips, excerpt previews and a source panel

Extend the current retained-evidence path rather than introducing another source store. Establish a typed, run-bound reference projection with source ID, source kind, display title, locator, captured time when known, and verified anchors. Local file anchors can reuse the current line/read-range data. URL/connected-record anchors must come from actual retained retrieval evidence. PDF page/region anchors require a trustworthy extraction mapping before the UI offers page-level jumps.

Render concise numbered or named chips inline. Hover or keyboard focus opens a bounded excerpt card; click opens the existing right sidebar at the relevant retained passage. Show cited and actually read portions distinctly. Support open-original only through validated links, and label “cached/retained,” “not read,” “anchor unavailable,” missing and changed-source states accurately. A captured source does not prove the response’s claim is correct. Do not turn model-authored references into verified evidence without validation.

Keep code-copy text intact; do not parse citations inside code blocks. No HTML from retrieved content, unrestricted file paths, tracking favicon fetches, automatic network fetching on hover, or additional model request to render a preview. Do not replace a retained source with the current workspace file. Respect source retention/access rules on later reads.

Acceptance: fake/unretrieved citations, invalid ranges, changed bytes, wrong run/chat, deleted sources, unsupported PDF anchors, unsafe URLs/HTML, truncation, hover-to-click behavior, keyboard use and selection/request races. Retain existing source-preview tests and Electron source-link journeys.

### 3. Richer public activity

Enhance the existing `workactivity.Trace` and `ActivityTimeline`; keep their bounds, ordering, restart behavior and non-interactive default. Project an allowlisted display subset of real events: research domains from actual searches/fetches, selected source references, relative changed paths from real coding operations, recorded checks and precise waiting/error states.

The reference’s “stream of thought” becomes a visible **activity/progress stream**: public narration and observable actions, not private model reasoning or a simulated transcript. Keep proposed work, dispatched operations and observed results distinct. A file is not marked edited just because narration says it was; an attempted write or failed command is not shown as verified success. Native coding-agent semantic controls stay separate from raw terminal input.

Do not put tool arguments, credentials, complete terminal output, absolute private paths or provider payloads into the public trace. Link to existing bounded inspectors when detail is appropriate. Preserve scroll position, grouped tools, disclosure state, reduced motion, cancellation and explicit incomplete outcomes. No unrelated polling or new daemon.

Acceptance: real tool-event sequencing, safe display metadata, failed/partial operations, restored traces, scrolling and disclosure, bounded long runs, sensitive-content exclusion, and no replay after reconnect. Prefer deterministic real-runner fixtures; paid live-model evaluation remains a separate explicit command under the existing cumulative budget.

### 4. Per-message actions and explicit new-chat branches

Add a response overflow menu with “Branch in new chat” and “Read aloud,” preserving Copy. Extract/reuse the existing local speech controls so old replies can be spoken explicitly without replaying a whole conversation or starting a model request. Include stop/unavailable states.

Implement a narrow owner-side fork operation backed by `worksession`, with a stable source conversation/revision boundary and an ancestry link. Use a fresh conversation identity; preserve the parent head and running owner. Explain which preceding messages, retained file references, model preferences and memory flags are inherited. Do not send private replay/configuration through the renderer. Keep valid tool-call/result groups intact; initially offer branching at completed, durable turn boundaries, not arbitrary transient activity rows or visual list indexes.

Reuse immutable snapshots/artifact references when valid, with source ownership checks. Review connector selection for the new context; do not copy pending approvals, consumed decision tokens, action IDs, mutation grants, running workers, schedules or uncertain dispatches. No automatic model execution when creating a branch. Shared saved memories continue to follow the existing privacy model; a branch is not a separate user identity. Handle retained-data expiry and deleted ancestors explicitly. Merging branches is deferred.

Acceptance: parent unchanged, exact boundary, later parent content excluded, restart persistence, new draft isolation, shared-source checks, no inherited execution authority, no duplicated workers/action dispatch, no model call on fork, and safe per-message speech.

### 5. Editable plan cards and execution-linked progress

Add a task-level “Plan first” flow for substantial coding/research/multi-step requests. Keep ordinary conversation fast. Show an application-rendered card with intended outcome, finite steps, relevant sources, foreseeable effects, and supported cost/budget information. Use planned/pending markers before execution; checkmarks must not suggest unperformed work has completed. Offer edit/revise and “Approve plan.”

Bind approval to the exact task, conversation, plan ID/revision and reviewed content digest. Editing invalidates the earlier review. Reuse existing durable Work/session admission and approval/action persistence. Use native goal plans for genuinely goal-backed reference data, without treating reported goal status as proof of execution. Task plans need a deliberately scoped run/session representation, not a second scheduler or universal workflow database.

Approving a plan authorizes proceeding within its stated scope; it does not approve arbitrary future shell commands, connected-service writes, spending beyond limits, external terminal access, or artifact application. Preserve exact-preview decisions, native specialist permissions, candidate review/evidence and at-most-once dispatch. The first implementation should stop before out-of-scope effects and ask for a revised plan, rather than silently updating an already-approved plan while executing it. Cancellation, refusal and partial completion remain explicit.

Connect step states to observable execution where possible. Model-reported progress must be labeled as such. Do not invent precise task-to-tool mappings or estimated prices when the backend cannot establish them.

Acceptance: plan revision races, stale/wrong-chat decisions, changing scopes, pending approval and restart, duplicate button submissions, cancellation, post-approval exact-action review, partial execution/unknown outcomes and no bypass of artifact verification. This is the largest backend change; ship after the lighter affordances and their contracts are established.

## Verification and scope boundaries

For each stage, add focused Go/typed-contract checks and real Electron journeys with private disposable profiles. Keep code, sources, speech, connections, existing Work controls, companion routes and recovery regressions functional. Test light/dark appearance, narrow windows, keyboard/screen-reader naming and reduced motion. Use existing React, CSS/Tailwind and Lucide, not another UI framework or a workflow-canvas dependency.

Run relevant package tests, `npm run check --prefix desktop`, `npm test --prefix desktop`, `npm run build --prefix desktop`, and focused Electron journeys; broaden Go/UI regression runs when backend behavior changes. Test failure paths with fakes and real host execution fixtures; identify simulations accurately. No live-account mutation, physical-device acceptance or additional paid evaluation is implied by this plan. The existing $10 cumulative API-evaluation limit remains in force; this audit made no paid model calls.

The original audit was documentation only. Implementation began after the explicit goal instruction. Stage 1 evidence is recorded in [Chat sources](CHAT-SOURCES.md) and Stage 2 in [Chat citations](CHAT-CITATIONS.md); the remaining stages and final whole-repository acceptance are still required. Prior pending coding and local-voice work was reviewed, verified and committed separately as `2e7c59646` and `243802793`. No new paid model evaluation was run.
