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
task -> session -> native agent loop -> model adapter
                     |       |
                     |       +-> durable event journal / transcript replay
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

