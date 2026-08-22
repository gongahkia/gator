# Terminal-harness capability audit

This is a source-backed product audit as of 2026-08-22, not a model-quality
benchmark. It compares the committed Gator architecture and tests with public
primary documentation. A documented competitor capability is not evidence that
it is reliable in every environment; it is the feature bar users can reasonably
expect.

## Conclusion

Gator is now a credible local, cross-provider coding-agent core: it creates an
isolated worktree by default, applies a strict OS process sandbox, has a typed
tool loop with approvals and verification, persists resumable runs, supports
trusted hooks/MCP/LSP, and exposes both ACP and a Gator-specific headless
control plane. It is stronger than many extension-first tools at refusing
untrusted project executables and remote credential redirection.

It is not yet a full replacement for Codex CLI, Claude Code, Cursor CLI, Pi,
or OpenCode. The largest remaining local-product gaps are direct terminal UX,
configurable agent types and safe writer-agent orchestration, richer code
intelligence/browser tools, and a broader automation control plane. Hosted
agents, organization governance, and cloud handoff are separate services, not
omissions that a local Go binary can honestly claim to solve.

## Capability map

| Surface | Gator evidence | Comparative assessment |
| --- | --- | --- |
| Process isolation and approvals | `run_command` uses strict Seatbelt on macOS and Bubblewrap on Linux, fails closed elsewhere, filters environment, bounds output, and denies network by default. Worktrees remain a review boundary, not the only security boundary. | Substantive local parity with Codex's documented sandbox/approval model; platform coverage and sandbox regression testing must remain a release gate. [Codex sandboxing](https://learn.chatgpt.com/docs/sandboxing) |
| Isolated changes, resume, and review | New runs use a retained Git worktree; the original checkout is untouched. Session threads can resume, fork from a recorded patch snapshot, inspect a diff, or export/apply a patch. | Competitive for a local review-first flow. Cursor documents similarly isolated worktrees and cleanup; Gator needs setup and dependency-caching ergonomics to match it. [Cursor worktrees](https://cursor.com/docs/configuration/worktrees) |
| MCP and credentials | Explicitly trusted stdio and Streamable HTTP servers; per-tool approval; resource-bound OAuth discovery, PKCE, DCR or pre-registered public clients, and private credential storage. | Stronger default trust posture than automatic project-server connection. It is intentionally narrower than broad plugin ecosystems. [MCP authorization](https://modelcontextprotocol.io/specification/2025-11-25/basic/authorization), [OpenCode MCP](https://opencode.ai/v2/docs/mcp-servers) |
| Rules, profiles, and hooks | Layered `AGENTS.md`, bounded declarative rules/profiles, plus trusted hash-pinned hooks at tool, compaction, verification, and session boundaries. | Solid safety foundation. It does not yet offer the breadth of hook actions or dynamically loadable skill/plugin contracts available in larger harnesses. [Claude Code features](https://code.claude.com/docs/en/features-overview) |
| Subagents | CLI callers can schedule up to four separate-worktree scouts. Native models now have `delegate_readonly`: batches of one to four fresh-context scouts, bounded to eight per run, share the active worktree only through read/search/Git tools, and return 8 KiB untrusted reports. Activity is journaled and visible in the TUI. | This closes the important *model-invocable read-only exploration* gap. Codex, Claude Code, Cursor, and OpenCode support configurable agent types; Gator has one built-in scout and no writer/agent-team protocol. [Codex subagents](https://learn.chatgpt.com/docs/agent-configuration/subagents), [Claude Code subagents](https://code.claude.com/docs/en/sub-agents), [Cursor ACP subagent tasks](https://prod.cursor.com/docs/cli/acp), [OpenCode agents](https://opencode.ai/docs/agents) |
| Code intelligence | Trusted local LSP pull diagnostics with executable hash pinning and per-launch approval. | Useful diagnostics, but no symbols, hover, definition, references, rename, code actions, or formatting. This remains an IDE-integration gap. |
| Terminal and browser work | Execute mode has `terminal_start/read/write/list/stop`: a sandboxed PTY task manager with 15-minute lifetime, bounded scrollback, task cancellation on run exit, and separate digest-redacted approval for each input. Commands and tool output remain agent-mediated; there is no user-attached terminal multiplexer, native web search, browser automation, or computer-use tool. | The PTY closes the basic persistent-process gap for dev servers and REPLs. Direct terminal UX and first-party browser research remain material gaps. MCP can provide optional browser tools but is not a first-party fallback. Pi's extension model shows what is possible, but its packages execute with host authority by default. [Pi extension subagent example](https://github.com/badlogic/pi-mono/blob/main/packages/coding-agent/examples/extensions/subagent/index.ts) |
| Automation/control plane | Versioned local JSONL RPC handles run/steer/cancel/approve; ACP v1 over stdio maps sessions, tool updates, and approvals. | Useful editor integration but not equivalent to Codex App Server or OpenCode's HTTP/OpenAPI server. Gator ACP also deliberately lacks client-injected MCP, prompt images/audio, batch requests, and remote transport. [Codex App Server](https://learn.chatgpt.com/docs/app-server), [Cursor CLI ACP](https://prod.cursor.com/docs/cli/using), [OpenCode server](https://dev.opencode.ai/docs/server/) |
| Delivery, hosted agents, governance | Local diff/patch handoff and MCP-backed integrations only. No first-party PR/issue/CI workflow, remote executor, notifications, user identity, audit service, organization policy, or cloud handoff. | GitHub/GitLab delivery should be MCP-backed before a bespoke API. Managed background agents and governance need server-side product infrastructure. |

## Deliberate non-equivalences

- Gator's trust model rejects changed project hooks, LSP servers, and MCP
  bundles until a developer pins the new hash. This is friction by design, not
  a missing auto-connect feature.
- `delegate_readonly` is not a generic plug-in agent framework. A child cannot
  change files or run processes, and a parent cannot make it a writer through
  prompt text. The limitation is enforced by the available tool definitions.
- A static `--scout` runs from the immutable base commit in a separate
  worktree; a dynamic scout inspects the active worktree so it can review work
  already performed by the primary agent. Both have separate model contexts.
- Gator will not claim hosted-agent or enterprise parity without an authenticated
  remote service, policy distribution, audit storage, and operational support.

## Recommended build order

1. **Writer-agent protocol.** Define named local agent profiles, one child
   worktree per writer, patch/result manifests, mandatory parent review, and a
   conflict-aware handoff. Do not allow two writers into the same checkout.
2. **Richer trusted code intelligence and browser research.** Expand the LSP
   client only after pinning a stable capability contract; offer native HTTP
   fetch/search before optional browser automation, each with explicit network
   policy and output limits.
3. **Control-plane completeness.** Extend ACP notifications for subagent
   progress and add a versioned HTTP/App-Server-like API only when a local
   multi-client use case needs it. Keep remote execution separate.
4. **Delivery integrations.** Use trusted OAuth MCP servers for GitHub/GitLab
   issue, review, and CI interaction, then assess whether a first-party client
   is justified by demonstrated reliability limits.

The implementation decisions and exact test evidence belong in the source and
release notes; this audit intentionally remains a current capability map rather
than a promise that all listed future work exists.
