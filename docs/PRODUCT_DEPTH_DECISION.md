# Work depth implementation decision

Baseline: `70adf39b57e7bd5469d792e3674ca7f331f5512e`. The initial working tree
contained only untracked `docs/research/` evidence, which is preserved. The host
uses Go 1.26.7; the manifest and CI pin 1.25.13. Pre-Work history and the retained
Code engine are available; no separate original Code checkout was found.

Keep the provider-neutral Go runner, file-backed private state, existing
orchestrator, and isolated Code backend. Extract application execution from CLI
parsing, with typed contracts and explicit controls. Portable deliverables must
remain distinct from private replay and executable project configuration.

The dependency order is:

1. Version capture and conversation state; resolve parents before sources; retain
   normalized replay and captured configuration; preserve model capabilities.
2. Introduce typed Work execution and control; route CLI, TUI, jobs and a separately
   versioned Work protocol through it; close scheduling recovery gaps.
3. Extend the existing orchestrator with durable child lifecycle, authority
   boundaries, bounded scheduling, usage, and explicit recovery.
4. Add selected staged Code baselines, candidate integration/verification and
   explicit target application with conflict preflight.
5. Add bounded document/table operations and selected-source evidence with
   independent checks; exercise complete workflows and their continuations.
6. Extend local evaluation with Work trials, independent scoring, comparisons,
   optional redacted telemetry/experiment export, CI and product documentation.

Framework review (official documentation checked September 9, 2026):
[LangChain subagents](https://docs.langchain.com/oss/javascript/langchain/multi-agent/subagents)
provide a library-level manager/tool pattern already present here.
[LangGraph persistence](https://docs.langchain.com/oss/python/langgraph/persistence)
provides execution checkpoints, a different responsibility from model adapters.
[ADK Go](https://github.com/google/adk-go) is a Go agent toolkit, but adopting its
loop would require porting Gator's policy and evidence boundaries. No measured
advantage has been established for replacing this runtime.
[LangSmith evaluation](https://docs.langchain.com/langsmith/evaluate-with-opentelemetry)
and [Langfuse OTEL](https://langfuse.com/integrations/native/opentelemetry) are
optional observability/evaluation destinations, not local execution stores.
Local reports and recovery must work without them. Hosted results and live model
quality require separate verification; deterministic fixtures cannot establish
either.

Recovery preserves completed evidence and marks interrupted operations for
inspection. It cannot reconstruct overwritten version-1 origin metadata or
promise exactly-once external execution. Approval is an explicit response and
never inferred from time elapsed.
