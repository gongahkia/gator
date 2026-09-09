# Gator orchestration

## Decision

Gator uses a user-facing manager with bounded specialists exposed as tools.
This is the common architecture across the official guidance for [OpenAI agent
orchestration](https://openai.github.io/openai-agents-python/multi_agent/),
[OpenCode agents](https://opencode.ai/docs/agents/), [Claude Code
subagents](https://code.claude.com/docs/en/sub-agents), and [LangChain
subagents](https://docs.langchain.com/oss/javascript/langchain/multi-agent/subagents).
There is no single cross-language industry-standard library behind those
products; the stable part is the manager-as-tools pattern.

The implementation lives in `internal/orchestrator` and uses Gator's existing
Go `agent.Model`, `agent.Tool`, event, permission, snapshot, and manifest
contracts. Gator does not add LangGraph or Google ADK as a second runtime.
LangGraph's own reference now recommends direct tool-calling supervisors over
the separate `langgraph-supervisor` helper, and [Google ADK for
Go](https://github.com/google/adk-go) would duplicate the agent loop Gator
already owns. A second runtime would split provider behavior, approvals,
logging, and evidence without adding a capability Gator currently lacks.

## Runtime shape

```text
user objective
      |
      v
Gator manager ────────────────────> final answer + sealed artifacts
      |
      +── delegate_agents (bounded budget, up to 3 parallel tasks)
              |
              +── source_researcher  (read-only frozen source)
              +── artifact_reviewer  (read-only source and staged output)
              +── code               (isolated Gator Code worktree)
```

The manager owns the conversation, decides whether delegation is useful,
receives only bounded results, verifies material claims, and remains responsible
for the final answer. Every language-model specialist starts with fresh context.
Specialists cannot recursively delegate or inherit connector or publish access.

## Current specialists

| Specialist | Trigger | Authority | Returned result |
| --- | --- | --- | --- |
| `source_researcher` | A focused source question benefits from separate context | Read/list/search the immutable `source/...` snapshot | Concise findings with exact paths and uncertainty |
| `artifact_reviewer` | Staged output needs independent contract review | Read frozen source, prior output, and staged output; call `artifact_status` | Prioritized defects and validation evidence |
| `code` | The Work objective contains a bounded implementation task | Run Gator Code against a private Git repository copied from the exact source snapshot | Summary, changed paths, and a retained `.patch` artifact |

The Code specialist never edits the user's source and is not directly
user-facing. Strict sandboxing, denied network, no recursive writer/scout
delegation, and `git diff --check` are invariant defaults. The user may add
project verifiers, scopes, a profile, setup commands, exact command grants, or
literal command-prefix grants. LSP, MCP, extensions, HTTP research, browser,
and terminal access remain absent unless explicitly granted for that Work run;
project trust and stored configuration are still required underneath the
grant. The manager receives this envelope as read-only host policy and cannot
widen it through `delegate_agents`.

## Bounds and evidence

- One manager receives at most eight specialist invocations per run.
- One call can schedule one to three independent tasks concurrently.
- Tasks are limited to 8 KiB and summaries to 16 KiB.
- Each invocation has a stable ID, task digest, output digest, status, step
  count, and timestamps.
- A returned patch is bound to its path and SHA-256 digest in `manifest.json`.
- Full private specialist transcripts are not copied into portable artifacts.
- A specialist failure is contained and reported to the manager; it does not
  itself decide whether the parent outcome contract passed.

These controls follow the delegation advice in OpenAI's [latest-model
guide](https://developers.openai.com/api/docs/guides/latest-model): give
subagents narrow responsibilities, explicit tools, and clear completion
conditions instead of treating delegation as unconstrained autonomy.

## Candidate specialists

Add a specialist only when it has a distinct context or authority boundary and
can return a bounded, verifiable result. Useful next candidates are:

1. `connected_researcher`: search only explicitly selected service connectors,
   retaining source provenance without mutation authority.
2. `spreadsheet_analyst`: profile tables, propose formulas, and return a compact
   analysis specification for the trusted XLSX writer.
3. `document_designer`: turn approved content into a semantic document spec,
   leaving DOCX/PDF rendering to trusted code.
4. `claim_verifier`: independently map material claims to local or connected
   evidence and flag unsupported statements before sealing.
5. `automation_designer`: draft a job definition and missed-run/retry policy;
   the user still installs or enables the job explicitly.
6. `publication_previewer`: prepare the exact target, audience, and payload for
   a connected action without gaining permission to send it.

Browser research should be added only after Gator can retain URL, retrieval
time, content digest, and citation evidence in the same manifest. A generic
"do anything" specialist would recreate the cluttered harness this product is
moving away from and is intentionally excluded.

## Reassessment criteria

Reconsider an external orchestration runtime only if Gator needs durable
distributed graphs, cross-process checkpoints, or interoperability with a
runtime-specific agent ecosystem that cannot be expressed through the current
tool contract. A framework change must preserve provider neutrality, exact
authority narrowing, local evidence, and resumable state before it is adopted.
