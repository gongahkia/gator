# Architecture

## Product contract

Gator's normal run begins with a developer objective and ends with one of four
clear states: a reviewable patch with verification evidence, a plan that needs
developer input, a deliberate refusal due to policy, or an explained failure.
The agent never silently edits the developer's active checkout in its default
mode.

The target is both bug fixes and scoped feature work. A feature run may inspect
the repository, change several files, add or update tests, run named
verification commands, and present the diff for review. Broad autonomous
refactors are not a v1 acceptance claim.

## Runtime

```text
terminal UI -> task/session -> native agent loop -> model adapter
                     |       |
                     |       +-> local event journal / private resume session
                     v
              policy-checked tools
                     |
                     v
             isolated Git worktree
                     |
                     v
               diff + verification
```

The core owns the loop, context selection, tool schemas, tool execution,
policies, event stream, worktree lifecycle, and run outcome. A model adapter
only converts between the provider protocol and the core's typed turn contract.
This keeps provider-specific details out of safety and test-critical code.

The interactive terminal UI is a thin event consumer, not another agent loop.
It collects a task, model, and explicit verifier allowlist; streams lifecycle
events from the executor; and renders the retained worktree and current diff
for review. Resume keeps the original run's model and verification policy so a
continuation cannot silently broaden its command authority.

The current OpenAI adapter uses Responses API streaming for incremental text,
then converts the completed typed response into the core turn contract. The
core does not rely on provider-side conversation persistence: requests use
`store: false` and a private local session file provides resume context.

## Initial tool surface

The first usable runtime will expose only these tools:

1. list and read repository files;
2. search repository text;
3. apply a unified patch inside the run worktree;
4. run an argv command subject to the run policy;
5. inspect Git status and diff.

Tools validate paths against the worktree root. Command execution, network
isolation, and write approval are separate policy boundaries; a worktree alone
is not a security sandbox.

## Acceptance evidence

Before calling a release useful for daily work, Gator must have:

- unit tests for the loop, tool validation, state transitions, and policy
  denials;
- disposable-repository integration tests covering a multi-file feature,
  a failing verifier, and an interrupted/resumed run;
- replay tests for provider event ordering, malformed tool calls, retries, and
  context compaction;
- manual dogfooding on real repositories with recorded usability issues and
  fixes;
- a documented evaluation run with fixed task, model, tool policy, budget,
  time limit, and verifier.

Benchmark results can demonstrate a bounded configuration only. They cannot by
themselves establish general parity with Pi, Codex CLI, or any particular model.

## Local state and privacy

Code changes remain in a sibling worktree. State is written outside the active
checkout at `$XDG_STATE_HOME/gator/` by default (or
`~/.local/state/gator/`). `events.jsonl` stores only event type, time, step,
tool name/call ID, and tool errors. It does not persist source text, prompts,
tool arguments, or tool output. `session.json` is deliberately different: it
contains the conversation required to resume a run, is written atomically with
mode `0600`, and should be treated as private local data.
