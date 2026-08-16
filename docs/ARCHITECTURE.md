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
terminal UI -> task/session -> selected backend -> model adapter or vendor CLI
                     |       |                         |
                     |       +-> local event journal   +-> isolated Git worktree
                     v                                      |
              policy-checked native tools                     v
                     |                               Gator-owned verification
                     v                                      |
             isolated Git worktree <-------------------------+
                     |
                     v
               diff + verification
```

For native cloud providers, the core owns the loop, context selection, tool
schemas, tool execution, policies, event stream, worktree lifecycle, and run
outcome. An adapter only converts between the provider protocol and the core's
typed turn contract. This keeps provider-specific details out of safety and
test-critical code. OpenAI uses Responses; Anthropic uses Messages; Gemini uses
stateless GenerateContent; and the compatible adapter supports Chat
Completions-compatible providers.

Delegated CLI providers are deliberately different. Codex, Claude Code,
GitHub Copilot CLI, and Cursor Agent run their own supported agent loop using
their existing local login. Gator does not parse, copy, or exchange their OAuth
state. It supplies the isolated worktree and task, records bounded lifecycle
events, then performs the required verifier argv calls and Git inspection
itself. The vendor CLI's tool policy remains its own security boundary, so a
noninteractive delegated run requires explicit caller acknowledgement.

The interactive terminal UI is a thin event consumer, not another agent loop.
It collects a task, model, execution mode, and explicit verifier allowlist;
streams lifecycle events from the executor; and renders the retained worktree
and current diff for review. Native Plan mode removes patching and command
tools entirely. Resume keeps one conversation thread on the same worktree and
preserves its original model and verification policy so a continuation cannot
silently broaden its command authority.

Supported `@` references are a separate, bounded developer input channel.
The TUI loads only repository-local PNG, JPEG, and WebP images; PDFs; selected
UTF-8 text/data formats; and DOCX, ODT, and XLSX documents. Images and PDFs
retain their original bytes for native provider inputs. Office documents are
locally converted to bounded plain text before transmission. Gator sends PDFs
only through adapters with a documented native document protocol (OpenAI
Responses, Anthropic Messages, and Gemini GenerateContent); text attachments
are framed as untrusted reference material for every native adapter. Generic
Chat Completions endpoints therefore fail clearly for PDF inputs rather than
silently dropping them. At most four attachments may total 8 MiB (4 MiB per
file), and their bytes exist only in the private session needed for stateless
replay. The live transcript displays model-emitted text, tool calls, summaries
of reads and commands, and patch line previews. It does not attempt to expose
hidden model reasoning.

The OpenAI and Anthropic adapters stream incremental text before converting the
completed response into the core turn contract. The core does not rely on
provider-side conversation persistence: OpenAI requests use `store: false` and
the private local session file provides resume context. Gemini's stateless
replay stores only opaque provider content necessary to carry thought
signatures through a function-call round trip; that content stays in the same
private `0600` session file as the rest of the conversation.

## Initial tool surface

Execute mode exposes only these tools:

1. list and read repository files;
2. search repository text;
3. apply a unified patch inside the run worktree;
4. run an argv command subject to the run policy;
5. inspect Git status and diff.

Plan mode exposes only read/search and Git inspection tools. Delegated CLI
providers are refused in Plan mode because Gator cannot enforce their vendor
tool permissions.

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

Each new run also writes a private `0600` thread record indexed by repository.
It advances to the latest session after every completed turn, allowing the TUI
to retain a single worktree across a multi-turn conversation while preserving
the immutable per-turn run records.
