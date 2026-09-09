You are working in `/home/gongahkia/Desktop/coding/projects/gator`. Build the next substantial product iteration of Gator. Read and understand the repository before changing it, then implement, verify, and document the work. This is an implementation assignment, not a request for another proposal or a collection of scaffolds.

## Product objective

Gator should be a provider-independent, terminal-native work agent: a persistent conversation that can research, analyze data, produce documents, perform bounded coding work through specialists, and prepare explicit external actions. It should offer the depth of modern coding/work assistants while remaining useful with multiple cloud providers and compatible local models.

Preserve Gator's own identity: explicit sources, frozen evidence, isolated output, developer-owned outcome contracts, reviewable deliverables, and user-controlled transfer or action. Coding is a first-class internal specialty within one Work experience. Do not restore a separate Code frontend or turn the product into an unrestricted shell agent.

The release must make these workflows concrete:

1. A cited research brief from local documents plus explicitly selected connected/web sources, followed by a revision that remembers earlier constraints and retains evidence.
2. Reconciliation of CSV/XLSX data into a checked workbook and explanatory memo, with calculations and discrepancies traceable to source data.
3. Repository investigation, parallel or sequential Code assignments where useful, verified patch integration, a readable change report, and a follow-up that builds on the prior staged changes before explicit application.
4. Scheduled inspect/draft variants using the same effective contract and producing recoverable attempts and inbox results.

## Read first and establish the baseline

Follow the applicable `AGENTS.md`. Inspect `go.mod`, Makefile, relevant scripts, CI/release configuration, and Git status before editing. This is a Go application with a pinned Playwright sidecar; do not introduce a frontend framework because of unrelated instruction boilerplate.

Read `docs/research/product-depth-report.md`, `docs/WORK.md`, `docs/CODE_TO_GATOR_MIGRATION.md`, `docs/ORCHESTRATION.md`, `docs/EVALUATION.md`, `docs/WRITERS.md`, and the implementation paths they identify. Inspect the retained Gator Code engine and the available pre-Work Git history. If another original Gator Code repository is available, compare it explicitly; otherwise state that limitation and proceed using the retained code and history.

The research baseline is commit `70adf39b57e7bd5469d792e3674ca7f331f5512e`. Verify the current tree: another instance may have changed it. Preserve all unrelated work and treat current code as authoritative. The report is evidence and a recommended design, not permission to assume every finding remains unchanged.

Read `docs/research/product-depth-verification.txt`. At the baseline, all 50 Go packages passed tests; vet, build, formatting, seven focused race-test packages, and the opt-in managed Chromium integration passed. The host used Go 1.26.7 while the manifest/CI specify 1.25.13. macOS Seatbelt and live cloud/connector/evaluation-service checks were not verified.

`docs/research/product-depth-reproductions.patch` contains five diagnostic tests that passed by reproducing existing defects. Apply it only to a disposable baseline checkout if needed. In the implementation, write regressions asserting the corrected behavior; do not preserve tests that assert the defects.

Write a compact architecture decision and dependency-ordered implementation plan grounded in current code. Then execute it. Continue through the deliverables below; do not stop after planning, package creation, or the first milestone. Use coherent increments and appropriate checks. If external credentials are unavailable, finish deterministic implementation and integration tests and identify the remaining live checks precisely.

## Architectural direction

Keep Go as the primary runtime and reuse `internal/agent`, `internal/model`, `internal/run`, `internal/orchestrator`, `internal/eval`, and the existing isolation/evidence packages. Add a shared Work application service so CLI, TUI, jobs, and the new Work headless surface exercise the same request-resolution and execution behavior.

Reconsider LangChain, LangGraph, Google ADK for Go, LangSmith, and Langfuse against actual requirements. Distinguish an agent library, a durable execution runtime, and an observability/evaluation service. The default recommendation is to deepen native orchestration and add optional OpenTelemetry/LangSmith integration. Do not add a second Python/TypeScript runtime merely to obtain a supervisor abstraction already present in Gator. If an external runtime is materially better, demonstrate that with a small representative comparison and an explicit compatibility/migration decision before adopting it.

Keep the local runtime, local records, and local evaluation reports usable without SaaS. Reuse the current file-backed stores where appropriate; add transactions or another store only where concurrency and recovery requirements justify them. Avoid a broad package rewrite or a parallel configuration/policy system.

## 1. Correct Work state and port the missing Code foundations

Implement branch-correct conversational continuity. `internal/workrun/executor.go` currently invokes the runner without `InitialMessages`, while `internal/worksession/store.go` retains objective/final text instead of replayable conversation state. Reuse relevant journal and context-management behavior from Code. Retain private normalized messages, selected attachment/evidence references, effective configuration, and versioned compaction state. Keep raw private transcripts out of portable bundles.

Resolve a continuation's parent before its default snapshot. The chosen parent must supply coherent context, source, and prior artifacts unless a source refresh or explicit override is requested. Test explicit old-parent branches as well as Back/Forward. Do not treat the conversation head as the requested parent's state.

Separate immutable source-capture identity from deduplicated content identity. Two directories containing identical bytes must not overwrite each other's origin, timestamp, or exclusion records. Preserve blob/tree deduplication and version old state safely. Do not invent missing historical origin metadata.

Repair the trusted project-configuration path into Code. `.gator` is excluded by the source snapshot, but Code loads profiles/rules/integration configuration from the copied repository. Capture selected approved configuration separately, with original canonical trust identity and content digest. Preserve explicit per-run grants and existing trust checks. Test profiles, scoped instructions, hooks, MCP, LSP, and extension resolution through the actual Work-to-Code assembly where applicable. Never indiscriminately copy credentials or grant trust to a scratch path.

Preserve streaming and visual-input capability declarations through `nativeWorkBackend` in `cmd/gator/command_work.go`. Add assembled-backend tests for streaming, nonstreaming, text-only models, cancellation, and unsupported attachments. Make provider changes handle opaque provider replay data explicitly; do not blindly send incompatible state to a different adapter.

Acceptance: meaningful regressions cover all five reproduced gaps, old state remains readable under documented semantics, and Code policy/configuration works from a frozen Work capture without reading mutable live configuration during execution.

## 2. Give Work one typed execution service and live control

Replace `work_interactive.go` and `command_job.go`'s buffered self-invocation as their application boundary. Prefer direct service calls; retain a subprocess only if it has a justified, versioned bidirectional protocol. Reuse existing runner/event/control mechanisms instead of copying the old TUI wholesale.

Support context cancellation, incremental events, mid-run user steering, and approval requests/responses. Distinguish steering the current task from queuing a future turn. In the TUI, expose current work, active specialists, progress, errors, and cancellation; a running operation must remain controllable. Retain useful current conversation/queue/navigation behavior.

Route the complete typed outcome contract and effective policy into scheduled execution. Do not round-trip contracts through a lossy subset of CLI flags. Define and implement refresh/frozen-snapshot behavior, bounded process output where still applicable, cancellation, retry classification, and interrupted-attempt reconciliation. Validate the schedule-claim/attempt-intent crash boundary.

Add a versioned Work-native headless surface with structured events, stable terminal results, and explicit conversation/revision identifiers. Keep existing RPC/ACP/app-server compatibility semantics intact; do not silently redefine coding-engine requests as Work requests. Use adapters where appropriate and document which surfaces are Work-native.

Acceptance: CLI, TUI, jobs, and Work headless execution reach the same service; exact effective contract/policy digests survive each path; control and approval tests use a running operation; cancellation leaves inspectable state and no uncontrolled child work.

## 3. Build a durable, bounded specialist supervisor

Evolve `internal/orchestrator`; do not create a competing scheduler. Preserve the convenient bounded `delegate_agents` operation while introducing independently identifiable child tasks with start, inspect, await/result, cancel, and explicit continuation/retry behavior. Choose tool names that fit the existing conventions.

A task needs a globally unambiguous identity within durable state, parent run/task linkage, role/configuration version, input and source references, selected context, dependencies, effective authority, budget, attempt identity, timestamps, terminal result references, and typed error category. Persist lifecycle transitions as they occur, including starts and each completed child before the rest of its batch finishes.

Forward per-child events and serialize ownership of shared run state. Review existing event slices, manifest evidence, connector callbacks, and renderer maps before introducing concurrent writers. Implement bounded scheduling, dependency validation, cycle rejection, subtree cancellation, wait deadlines, and backpressure. No recursive Code delegation; the manager owns task coordination.

Add versioned role definitions and per-role provider/model configuration through existing configuration mechanisms. Support fresh task context by default, deliberate selected-history/evidence input, and explicit continuation of a retained child. A role must have an executable tool/authority boundary and structured result contract, not just a different system prompt. Model-produced task arguments may narrow authority but cannot grant commands, network, integrations, credentials, mutation, or publishing access.

Enforce aggregate budgets across manager, children, and retries: concurrent tasks, total invocations, model requests/steps, wall time, and tokens where supported. Record actual versus estimated usage and unknown cost. Avoid unlimited fallback/retry loops. Configured routing/fallback must be visible in evidence and preserve policy.

Define recovery boundaries honestly. Reuse completed deterministic work and retained results after interruption. Reconcile unknown external outcomes instead of automatically repeating them. Persist pending user interaction without treating elapsed time as approval. Do not promise exactly-once command or external execution.

Acceptance: deterministic tests and race checks cover simultaneous completion, one failed child among successful peers, cancellation while queued/running/waiting, budget contention, crash recovery, invalid dependencies, unauthorized capability requests, and role/context isolation. The TUI and headless client can inspect the same child lifecycle.

## 4. Make Code delegation iterative and composable

Reuse relevant machinery in `internal/run/writer.go`, `writer_batch_scope.go`, `internal/patch`, and worktree verification. Keep Code as a bounded backend, not another user-facing manager.

Add explicit staged baseline selection. A child can start from the frozen source plus a chosen accepted patch set, including a prior conversation revision's staged changes. Record exact baseline/content identity. Give children private writable roots and a real changed-path envelope; existing instruction scopes alone are not write enforcement.

Implement patch selection, comparison, conflict detection, staged integration, and verification of the combined candidate. Nonoverlapping patches still require aggregate verification. A failed verifier or conflicting patch must remain visible and must not be represented as a completed integrated change. Freeze candidate versions while independent reviewers inspect them.

Add a Work-bundle code review/application flow. `artifact.Apply` currently copies artifacts; copying a `.patch` file does not apply its changes. Show changed paths, baseline, selected patches, verification, and target conflicts. Provide explicit code-patch application with target preflight and preserve ordinary document export/apply behavior. Do not mutate live source during agent execution or silently apply every retained patch.

Acceptance: demonstrate parallel independent changes, overlapping edits, a sequential follow-up based on earlier staged code, combined verification failure, and an apply-time changed-target conflict. Retain patch and verification evidence for successful and failed attempts.

## 5. Add research and analysis capabilities that complete workflows

Extend a small role registry according to distinct capability needs. Prioritize connected/web research, deterministic table analysis, and independent claim verification. Add document-design roles only where they improve a measured workflow; do not add numerous prompt-only personas.

Create bounded on-demand document extraction and table-inspection/transformation tools using existing attachment and artifact code. Preserve source locators such as path, section/page where available, sheet/cell/row, or connector resource. Report unsupported extraction or missing evidence explicitly. Do not claim exact page/cell provenance that the parser cannot produce.

Build a selected-source evidence catalog that includes frozen local files, attachments, connector read snapshots, and explicitly allowed web retrievals. Expose only the appropriate subset to each specialist. Correct the mismatch between a source-only specialist prompt and a tool surface that can also read output/previous roots.

Reuse HTTP/browser policy and provenance mechanisms where suitable. Web research needs bounded retrieval, approved origins/network policy, URL and retrieval time, content digest, and stable evidence references. Reading a connected source does not grant mutation or publishing. Credentials remain outside model context and artifacts.

For data workflows, implement deterministic operations and independent checks of results; do not ask the model to calculate a spreadsheet entirely in prose. For research, retain claim-to-source references and distinguish unsupported or conflicting claims. A structurally valid PDF/XLSX is not sufficient semantic evidence.

Acceptance: the research brief, reconciled workbook/memo, and mixed code/document workflow run through the actual service with reproducible fixtures. Each supports a follow-up revision and review of its evidence. Do not require a live external account for deterministic tests.

## 6. Deliver a real local evaluation platform and optional LangSmith integration

Extend `internal/eval` while preserving existing Code fixtures/reports and hidden-scoring isolation. It currently exercises `gatorrun.Executor` with delegation disabled. Introduce versioned targets for direct Code and full Work; the full Work target must execute the actual manager, supervisor, policies, tools, contracts, and conversation path.

Implement dataset manifests/validation, trials, independent graders, retained traces/evidence, report viewing, baseline comparison, and export. Use stable CLI operations consistent with the repository. Support local JSON and readable reports without a cloud service. Keep scripted deterministic execution tests separate from live model-quality experiments.

Seed a curated initial corpus around 24 meaningful tasks across research/evidence, tables/documents, coding/integration, and continuation/control/automation. Use development and held-out splits. Include the reproduced failures and realistic boundary cases; avoid filling the corpus with trivial variations. Retain full contracts, fixture digests, capabilities, configuration, budgets, grader versions, and interaction scripts.

Use deterministic graders for boundaries, schema/contract fidelity, actual file/table values, source-reference validity, code tests, integration, and unauthorized actions. Add versioned rubric grading for synthesis quality where useful, with explicit evidence and human calibration. Keep grader data outside agent-readable fixtures. Evaluate grader soundness; do not rely on the producing agent to grade itself.

Separate task failure, fixture invalidity, provider outage, sandbox/setup failure, grader failure, timeout, and budget exhaustion. Display all categories and denominators. Compare outcome quality, artifact/semantic checks, unsupported claims, interventions, latency, retries, model calls, tokens, and known cost. Report repeated-trial variability; do not confuse at-least-one success with consistent success.

Include matched delegation-disabled/enabled baselines and later per-role ablations. Do not claim more agents improve quality without measured results. Establish baseline measurements before selecting quality thresholds; no fabricated provider comparisons or success percentages.

Add normalized usage and optional OpenTelemetry spans across parent run, child task, model attempt, tool, verification, and sealing. Keep Gator's durable schema independent of vendor attributes. Bound export queues and retention; exporter failures must be observable without corrupting local run state. Default telemetry should exclude credentials, raw prompts, source contents, and private transcripts. Make content capture explicit.

Implement an optional LangSmith adapter with trace hierarchy, experiment/dataset/example association, and score/feedback linkage. Tracing alone does not satisfy this deliverable. Test mapping, errors, redaction, and associations locally. Verify the hosted round trip only when valid credentials and authorization are available; clearly mark it unverified otherwise. Keep the OTLP boundary suitable for alternatives such as Langfuse without requiring a second custom integration now.

Acceptance: local end-to-end Work experiments and comparisons work; held-out scoring is isolated; failures are attributable; usage rolls up across the tree; optional export is tested; CI runs deterministic regressions. Add an explicit, budgeted live-eval command/protected workflow for configured providers, and report which live checks actually ran.

## Engineering, compatibility, and verification

Make the smallest coherent changes that meet these outcomes. Extract shared application responsibilities from `cmd/gator`; keep UI rendering, provider protocols, credentials, domain policy, artifact formats, and execution state in appropriate layers. Avoid unrelated renaming, formatting, dependency modernization, or speculative abstractions.

Version changed stores, manifests, role definitions, protocol envelopes, and eval schemas. Add migration/compatibility tests and readable failure messages. Do not delete historical Code state, weaken sandboxing, alter tests to conceal regressions, or broaden authority to make demos pass. Preserve exact-payload external-action approval and the scheduled inspect/draft restriction.

Restore a current root README and update Work/orchestration/evaluation/job/review documentation. Distinguish current, partially implemented, historical compatibility, and intentionally retired capabilities. Document reproducible CLI walkthroughs, configuration precedence, role/provider behavior, state recovery, migrations, and benchmark invocation. Do not claim old Code-only behavior as current Work functionality.

Run narrow discriminating tests first, then the applicable project checks. Use `make check` when appropriate for implementation, inspect formatting changes, and run targeted race tests for all new concurrent lifecycle/state paths. Run the pinned Chromium integration if browser behavior changes. Test Go 1.25.13 and supported OS paths through available environments/CI; identify anything unavailable. Never treat mocked provider responses as live model validation.

Before finishing, review the actual diff against the acceptance criteria and exercise the product workflows. Remove scaffolding that has no connected behavior. Leave changes reviewable without committing, publishing, deploying, or contacting external recipients unless separately authorized.

The final handoff must state what works end to end, important architectural decisions, legacy capabilities ported, tests and evals actually run with outcomes, compatibility/migration effects, and precise remaining limitations. Do not mark the release complete with placeholder tools, a trace-only “eval platform,” or unverified live-quality claims.

## Primary research references

Recheck current official documentation before depending on mutable APIs. The companion report contains source-specific findings and limitations.

- [OpenAI Work/Codex subagents](https://learn.chatgpt.com/docs/agent-configuration/subagents)
- [OpenCode agents](https://opencode.ai/docs/agents/)
- [Claude Code subagents](https://code.claude.com/docs/en/sub-agents) and [agent teams](https://code.claude.com/docs/en/agent-teams)
- [LangChain subagent architecture](https://docs.langchain.com/oss/javascript/langchain/multi-agent/subagents)
- [LangGraph checkpointers](https://docs.langchain.com/oss/python/langgraph/checkpointers)
- [Google ADK for Go](https://github.com/google/adk-go)
- [LangSmith OpenTelemetry tracing](https://docs.langchain.com/langsmith/trace-with-opentelemetry) and [evaluation association](https://docs.langchain.com/langsmith/evaluate-with-opentelemetry)
- [Langfuse OpenTelemetry](https://langfuse.com/integrations/native/opentelemetry)
- [OpenTelemetry GenAI conventions](https://github.com/open-telemetry/semantic-conventions-genai)
- [Anthropic agent-evaluation guidance](https://www.anthropic.com/engineering/demystifying-evals-for-ai-agents)
- [Harbor agent interfaces](https://www.harborframework.com/docs/agents) and [Promptfoo script providers](https://www.promptfoo.dev/docs/providers/custom-script/)
