# Agent Client Protocol integration

`gator acp` (or `gator --mode acp`) runs Gator as a local [Agent Client
Protocol](https://agentclientprotocol.com/) v1 agent over standard input and
output. It is the editor-facing interface. `gator rpc` remains Gator's own
JSONL API for CI and bespoke automation; the two protocols are deliberately
separate.

Start the process at the root of the checkout that the editor opens:

```sh
gator acp
gator acp --verify 'go test ./...'
gator acp --verify 'npm test'
```

The transport is one JSON-RPC 2.0 object per line. It supports the normal ACP
v1 lifecycle: `initialize`, `session/new`, `session/prompt`, `session/cancel`,
`session/set_mode`, `session/list`, `session/load`, `session/resume`, and
`session/close`. Model text streams as `session/update` agent-message chunks;
tool calls and their terminal status stream as structured ACP tool updates.
Gator-owned subagent, terminal-exit, hook, compaction, and steering lifecycle
events also stream as completed `other` tool calls, so an ACP client can show
progress without treating coordinator messages as model text.
Gator sends `session/request_permission` for each non-verification command and
waits for a standard permission response with `allow_once`, `allow_always`, or
`reject_once`.

The server supports one active prompt per ACP session. A cancel notification is
processed while a prompt is running. Closing a session cancels its active work
and waits for the executor to release the retained worktree before completing
the close request. Retained Gator thread IDs are ACP session IDs, so
`session/list` and `session/load`/`session/resume` restore local conversations
without exposing private run-record paths.

## Execution policy

Gator never accepts an execution policy from an ACP prompt. If `--verify` is
given, the session starts in Execute mode and completion requires that exact
argv list, plus final `git_status` and `git_diff` evidence. Without an explicit
flag, Gator uses its bounded project suggestions (`go test ./...`, `npm test`,
`pytest`, or `cargo test`) when applicable; otherwise the session starts in
read-only Plan mode. ACP clients can switch an unused session between Plan and
Execute only when the process has a verifier policy. A retained Plan thread
cannot be converted to Execute in place because Gator preserves a thread's
original command authority.

The same strict process sandbox, isolated worktree, hook policy, project MCP
and LSP trust, and provider credential handling used by the TUI apply to ACP
runs. An ACP client cannot change the checkout, add filesystem roots, or inject MCP
servers: `cwd` must be the repository Gator was started for,
`additionalDirectories` must be empty, and `mcpServers` must be empty. Trusted
project MCP configuration continues to come only from `.gator/mcp.json` after
`gator mcp trust`. Trusted local LSP diagnostics, read-only navigation,
informational completion, formatting, rename, and workspace-confined code-action suggestions
continue to come only from `.gator/lsp.json` after `gator lsp trust`.
ACP prompts cannot provide a worktree setup command: that capability is limited
to the local developer's explicit `gator run --setup` invocation, before an
agent session exists.

ACP authentication methods are intentionally not advertised. Configure a
provider through Gator's existing `login`, `connect`, environment, or local
credential workflow before starting the agent. Gator does not ask an editor to
forward provider credentials.

## Current protocol boundary

This is a local stdio profile, not a hosted ACP endpoint. It does not provide
the draft streamable-HTTP/WebSocket transport, client-provided MCP servers,
ACP-client direct user terminal attachment or delegated vendor-terminal control,
image/audio prompt blocks, embedded resource content, or JSON-RPC batch
envelopes. Execute-mode ACP sessions can still expose Gator's approved native
terminal-task tools as ordinary ACP tool calls. Batch envelopes receive a clear
`-32600` error rather than a partial response. These are intentional unsupported
surfaces, not fallbacks to Gator's proprietary RPC protocol.
