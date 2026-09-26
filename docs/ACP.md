# Agent Client Protocol integration

`gator agent acp` (or `gator --mode acp`) exposes Gator to a local
[Agent Client Protocol](https://agentclientprotocol.com/) v1 editor over
standard input and output. It is an editor-facing adapter over canonical Work;
it has no private execution engine, journal, worktree lifecycle, or review
state.

```sh
gator agent acp
gator agent acp --verify 'go test ./...'
```

ACP accepts one JSON-RPC 2.0 object per line. It supports `initialize`,
`session/new`, `session/prompt`, `session/cancel`, `session/set_mode`,
`session/list`, `session/load`, `session/resume`, and `session/close`. Model
text and Work tool events stream as ACP `session/update` messages. Work
interactions are translated to `session/request_permission`; every selection is
an exact approval for that Work operation.

`plan` maps to read-only Work inspection. When `--verify` or a bounded project
suggestion is available, `execute` maps to draft Work requiring Code-specialist
evidence. It produces a reviewable Work artifact/candidate and never directly
applies a checkout mutation.

An ACP session ID is a process-local protocol handle, not a Work ID. Prompts in
that session continue the canonical Work conversation/revision returned by the
previous prompt. `session/list`, `session/load`, and `session/resume` therefore
operate only while the ACP process is alive; durable conversation and history
remain available through normal Gator Work state rather than a protocol-specific
journal.

The adapter fixes its repository root at process start. Client-supplied MCP
servers and additional directories are not accepted, and clients cannot weaken
the Work authority, sandbox, provider credentials, or verification policy.
