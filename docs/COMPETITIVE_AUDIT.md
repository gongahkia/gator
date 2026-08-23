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
or OpenCode. The largest remaining local-product gaps are a full terminal UX,
configurable agent types and parallel writer-team orchestration, richer browser
tools, and persistent code intelligence. Hosted
agents, organization governance, and cloud handoff are separate services, not
omissions that a local Go binary can honestly claim to solve.

## Capability map

| Surface | Gator evidence | Comparative assessment |
| --- | --- | --- |
| Process isolation and approvals | `run_command` uses strict Seatbelt on macOS and Bubblewrap on Linux, fails closed elsewhere, filters environment, bounds output, and denies network by default. Worktrees remain a review boundary, not the only security boundary. | Substantive local parity with Codex's documented sandbox/approval model; platform coverage and sandbox regression testing must remain a release gate. [Codex sandboxing](https://learn.chatgpt.com/docs/sandboxing) |
| Isolated changes, resume, and review | New runs use a retained Git worktree; the original checkout is untouched. A developer can select an immutable base, copy only explicitly listed ignored files, and run exact sandboxed `--setup` argv before scouts or the model. Session threads can resume, fork from a recorded patch snapshot, inspect a diff, or export/apply a patch. | Competitive for a local review-first flow. Cursor documents similarly isolated worktrees and cleanup; Gator still has no shared dependency-cache or project-authored setup-script mechanism. [Cursor worktrees](https://cursor.com/docs/configuration/worktrees) |
| MCP and credentials | Explicitly trusted stdio and Streamable HTTP servers; per-tool approval; resource-bound OAuth discovery, PKCE, DCR or pre-registered public clients, and private credential storage. | Stronger default trust posture than automatic project-server connection. It is intentionally narrower than broad plugin ecosystems. [MCP authorization](https://modelcontextprotocol.io/specification/2025-11-25/basic/authorization), [OpenCode MCP](https://opencode.ai/v2/docs/mcp-servers) |
| Rules, profiles, hooks, and extensions | Layered `AGENTS.md`, bounded declarative rules/profiles, capability-bounded project role prompts, trusted hash-pinned hooks, and explicit global or hash-pinned project extension bundles. Extension sidecars use the active strict sandbox and require per-tool approval. | The project can specialize a child prompt, but cannot configure arbitrary tools or permissions. This is safer and narrower than the hook/skill/plugin contracts in larger harnesses; unlike Pi packages, extensions do not receive implicit host authority. [Claude Code features](https://code.claude.com/docs/en/features-overview), [Pi extensions](https://github.com/badlogic/pi-mono/blob/main/packages/coding-agent/docs/extensions.md) |
| Subagents | CLI callers can schedule up to four separate-worktree scouts. Native models have `delegate_readonly`: batches of one to four fresh-context scouts, bounded to eight per run, share the active worktree only through read/search/Git tools, and return 8 KiB untrusted reports. Execute mode offers one serial writer or exactly two parallel writers, with two writers total per run. Each writer has a separate worktree from the same parent snapshot; parallel calls must declare non-overlapping paths, then Gator records actual paths and reports scope violations or overlap. Atomic parent-owned child and batch manifests retain baseline, state, patch digest, worktree, run record, grouping, and final conflict evidence. Roles change prompt only; patch application remains explicit and Gator never auto-merges. | This is a bounded, conflict-aware local writer scheduler without permission escalation. Codex, Claude Code, Cursor, and OpenCode still offer per-role model/tool policies, user/global definitions, background execution, broader orchestration, and richer conflict handling. Gator has no detached/background writer lifecycle or automatic conflict resolution. [Codex subagents](https://learn.chatgpt.com/docs/agent-configuration/subagents), [Claude Code subagents](https://code.claude.com/docs/en/sub-agents), [Cursor subagents](https://prod.cursor.com/docs/subagents), [OpenCode agents](https://opencode.ai/docs/agents) |
| Code intelligence | Trusted local LSP pull diagnostics, hover, completion, immediate code actions, formatting, rename, definitions, references, document symbols, and workspace-symbol queries with executable hash pinning, operation-specific approval, a strict sandbox, wire/output limits, workspace-only returned locations, and one lazy server per configuration. The native TUI and loopback app server retain up to eight idle managers only for a compatible resume of the exact retained worktree and trusted bundle hash; the next run that observes a change retires the cache and session exit stops it. Completion is informational; edit-producing operations expose only bounded workspace edits and never execute server commands or apply changes automatically. | This closes basic navigation, lookup-completion, reviewed quick-fix/format/rename discovery, and session-local resume indexing. Durable indexing and editor document synchronization remain IDE-integration gaps. [LSP code action](https://microsoft.github.io/language-server-protocol/specifications/lsp/3.17/specification/#textDocument_codeAction) |
| Terminal and browser work | Execute mode has `terminal_start/read/write/list/stop`: a sandboxed PTY task manager with a 15-minute ordinary lifetime, bounded scrollback, task cancellation on run exit, and separate digest-redacted approval for model input. In the native TUI and authenticated `gator serve`, a second approval permits `terminal_detach` for one existing task in a process-local registry: its original sandbox is fixed, it expires within two hours, is capped at eight tasks, and the normal shutdown path stops it. The native TUI attaches through `Ctrl+T`; an app-server bearer-token holder can list, read, write, resize, or stop only an explicitly detached task, without forwarding its input to the model or completed run record. The TUI attachment sizes its PTY to the viewport, renders bounded cursor/erase-aware output with private alternate-screen transitions, and offers line input, interrupt, stop, or opt-in raw keyboard input. With an explicit network grant, it also offers approval-gated public HTTPS text fetches and Brave API search. There is no full VT emulator, restart-surviving terminal service, browser automation, or computer-use tool. | This closes basic persistent-process, developer-attachment, session-background terminal, and source-backed web-research gaps for dev servers, progress displays, key-driven REPLs, and public research. A full multiplexer/terminal emulator and browser automation remain material gaps. A restart-surviving background service needs a supervised daemon with recovery and authorization, not orphaned PTYs. MCP can provide optional browser tools but is not a first-party fallback. [OpenCode CLI](https://opencode.ai/v2/docs/cli), [Brave Web Search API](https://api-dashboard.search.brave.com/api-reference/web/search/get), [Pi extension subagent example](https://github.com/badlogic/pi-mono/blob/main/packages/coding-agent/examples/extensions/subagent/index.ts) |
| Automation/control plane | Versioned local JSONL RPC handles run/steer/cancel/approve; `gator serve` exposes that protocol on an authenticated literal-loopback HTTP/SSE bridge with bounded replay and an OpenAPI wrapper document. `gator serve start/status/stop` explicitly supervises one repository-scoped background bridge, with private state/log files and token-authenticated endpoint checks before PID control. Its capabilities additionally control only explicitly detached terminal tasks in that one server process; ACP v1 over stdio maps sessions, model tools, approvals, and subagent/terminal/coordinator lifecycle updates. | This is viable for a local editor, desktop client, or CI-side controller that needs to maintain an approved dev process after the agent run. It is still not Codex's remote app-server or OpenCode's automatically discovered user-wide server: no remote listener, CORS, client-provided MCP, batch requests, media prompts, hosted transport, auto-discovery by TUI/ACP, or restart-surviving task daemon. [Codex App Server](https://learn.chatgpt.com/docs/app-server), [Cursor CLI ACP](https://prod.cursor.com/docs/cli/using), [OpenCode server](https://dev.opencode.ai/docs/server/) |
| Delivery, hosted agents, governance | Local diff/patch handoff and MCP-backed integrations only. No first-party PR/issue/CI workflow, remote executor, notifications, user identity, audit service, organization policy, or cloud handoff. | GitHub/GitLab delivery should be MCP-backed before a bespoke API. Managed background agents and governance need server-side product infrastructure. |

## Deliberate non-equivalences

- Gator's trust model rejects changed project hooks, LSP servers, and MCP
  bundles until a developer pins the new hash. This is friction by design, not
  a missing auto-connect feature.
- `delegate_readonly` is not a generic plug-in agent framework. A child cannot
  change files or run processes, and a parent cannot make it a writer through
  prompt text. The limitation is enforced by the available tool definitions.
- Writer delegation is bounded and explicit. A parent may run one writer or a
  pair with declared non-overlapping paths, but every child starts from the
  same parent snapshot and may change only its own worktree. Gator reports
  changed-path evidence and returns deltas rather than applying or merging
  them. A parent model still has to judge patch compatibility, so this is not a
  conflict-free merge guarantee.
- A static `--scout` runs from the immutable base commit in a separate
  worktree; a dynamic scout inspects the active worktree so it can review work
  already performed by the primary agent. Both have separate model contexts.
- Gator will not claim hosted-agent or enterprise parity without an authenticated
  remote service, policy distribution, audit storage, and operational support.

## Recommended build order

1. **Terminal UX, fuller code intelligence, and browser research.** Evolve the
   line-oriented terminal attachment into a multiplexer only with a stable
   isolation and transcript contract; extend the bounded session LSP cache only
   with separately reviewed lifecycle and document-synchronization design; add
   browser automation only if its network policy, approval, output, and session
   lifecycle can be bounded as tightly as native web research.
2. **Control-plane hardening.** Keep the loopback HTTP/SSE bridge small and
   prove its reconnect, overload, and credential-file behavior in release
   testing. Keep remote execution separate rather than exposing an unauthenticated
   local agent listener.
3. **Delivery integrations.** Use trusted OAuth MCP servers for GitHub/GitLab
   issue, review, and CI interaction, then assess whether a first-party client
   is justified by demonstrated reliability limits.

The implementation decisions and exact test evidence belong in the source and
release notes; this audit intentionally remains a current capability map rather
than a promise that all listed future work exists.
