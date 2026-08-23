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
configurable agent types and parallel writer-team orchestration, richer code
intelligence/browser tools, and a broader automation control plane. Hosted
agents, organization governance, and cloud handoff are separate services, not
omissions that a local Go binary can honestly claim to solve.

## Capability map

| Surface | Gator evidence | Comparative assessment |
| --- | --- | --- |
| Process isolation and approvals | `run_command` uses strict Seatbelt on macOS and Bubblewrap on Linux, fails closed elsewhere, filters environment, bounds output, and denies network by default. Worktrees remain a review boundary, not the only security boundary. | Substantive local parity with Codex's documented sandbox/approval model; platform coverage and sandbox regression testing must remain a release gate. [Codex sandboxing](https://learn.chatgpt.com/docs/sandboxing) |
| Isolated changes, resume, and review | New runs use a retained Git worktree; the original checkout is untouched. Session threads can resume, fork from a recorded patch snapshot, inspect a diff, or export/apply a patch. | Competitive for a local review-first flow. Cursor documents similarly isolated worktrees and cleanup; Gator needs setup and dependency-caching ergonomics to match it. [Cursor worktrees](https://cursor.com/docs/configuration/worktrees) |
| MCP and credentials | Explicitly trusted stdio and Streamable HTTP servers; per-tool approval; resource-bound OAuth discovery, PKCE, DCR or pre-registered public clients, and private credential storage. | Stronger default trust posture than automatic project-server connection. It is intentionally narrower than broad plugin ecosystems. [MCP authorization](https://modelcontextprotocol.io/specification/2025-11-25/basic/authorization), [OpenCode MCP](https://opencode.ai/v2/docs/mcp-servers) |
| Rules, profiles, hooks, and extensions | Layered `AGENTS.md`, bounded declarative rules/profiles, capability-bounded project role prompts, trusted hash-pinned hooks, and explicit global or hash-pinned project extension bundles. Extension sidecars use the active strict sandbox and require per-tool approval. | The project can specialize a child prompt, but cannot configure arbitrary tools or permissions. This is safer and narrower than the hook/skill/plugin contracts in larger harnesses; unlike Pi packages, extensions do not receive implicit host authority. [Claude Code features](https://code.claude.com/docs/en/features-overview), [Pi extensions](https://github.com/badlogic/pi-mono/blob/main/packages/coding-agent/docs/extensions.md) |
| Subagents | CLI callers can schedule up to four separate-worktree scouts. Native models have `delegate_readonly`: batches of one to four fresh-context scouts, bounded to eight per run, share the active worktree only through read/search/Git tools, and return 8 KiB untrusted reports. Execute mode offers one serial writer or exactly two parallel writers, with two writers total per run. Each writer has a separate worktree from the same parent snapshot; parallel calls must declare non-overlapping paths, then Gator records actual paths and reports scope violations or overlap. Atomic parent-owned child and batch manifests retain baseline, state, patch digest, worktree, run record, grouping, and final conflict evidence. Roles change prompt only; patch application remains explicit and Gator never auto-merges. | This is a bounded, conflict-aware local writer scheduler without permission escalation. Codex, Claude Code, Cursor, and OpenCode still offer per-role model/tool policies, user/global definitions, background execution, broader orchestration, and richer conflict handling. Gator has no detached/background writer lifecycle or automatic conflict resolution. [Codex subagents](https://learn.chatgpt.com/docs/agent-configuration/subagents), [Claude Code subagents](https://code.claude.com/docs/en/sub-agents), [Cursor subagents](https://prod.cursor.com/docs/subagents), [OpenCode agents](https://opencode.ai/docs/agents) |
| Code intelligence | Trusted local LSP pull diagnostics, hover, completion, immediate code actions, definitions, references, document symbols, and workspace-symbol queries with executable hash pinning, operation-specific approval, a strict sandbox, wire/output limits, workspace-only returned locations, and one lazy server per configuration for the current run. Completion is informational; code actions expose only bounded edits to existing workspace files and never execute server commands or apply changes automatically. | This closes basic navigation, lookup-completion, and reviewed quick-fix discovery without a cross-run language-server cache. Rename, formatting, persistent indexing, and editor synchronization remain IDE-integration gaps. [LSP code action](https://microsoft.github.io/language-server-protocol/specifications/lsp/3.17/specification/#textDocument_codeAction) |
| Terminal and browser work | Execute mode has `terminal_start/read/write/list/stop`: a sandboxed PTY task manager with 15-minute lifetime, bounded scrollback, task cancellation on run exit, and separate digest-redacted approval for model input. During a native TUI run, `Ctrl+T` attaches a developer to an existing task, sizes its PTY to the viewport, renders bounded cursor/erase-aware output with private alternate-screen transitions, and offers line input, interrupt, stop, or opt-in raw keyboard input. Raw keys are translated to conventional terminal bytes and grouped into one digest record when raw mode ends or the task exits. Direct input remains inside the task sandbox. When the developer enables network access, native `http_fetch` adds an approved, HTTPS-only, proxy-free public-text fetch with pinned DNS addresses, no redirects, and a 256 KiB response limit. If `BRAVE_SEARCH_API_KEY` is configured, `web_search` adds an approved fixed-endpoint Brave query that returns no more than ten untrusted title/URL/snippet records; its token never enters events or model context. There is no full VT emulator, background terminal, browser automation, or computer-use tool. | This closes the basic persistent-process, developer-attachment, and native search gaps for dev servers, progress displays, and key-driven REPLs with common alternate-buffer transitions. A full multiplexer/terminal emulator and first-party browser research remain material gaps. MCP can provide optional browser tools but is not a first-party fallback. Pi's extension model shows what is possible, but its packages execute with host authority by default. [Brave Web Search API](https://api-dashboard.search.brave.com/api-reference/web/search/get), [Pi extension subagent example](https://github.com/badlogic/pi-mono/blob/main/packages/coding-agent/examples/extensions/subagent/index.ts) |
| Automation/control plane | Versioned local JSONL RPC handles run/steer/cancel/approve; ACP v1 over stdio maps sessions, model tools, approvals, and subagent/terminal/coordinator lifecycle updates. | Useful editor integration but not equivalent to Codex App Server or OpenCode's HTTP/OpenAPI server. Gator ACP also deliberately lacks client-injected MCP, prompt images/audio, batch requests, and remote transport. [Codex App Server](https://learn.chatgpt.com/docs/app-server), [Cursor CLI ACP](https://prod.cursor.com/docs/cli/using), [OpenCode server](https://dev.opencode.ai/docs/server/) |
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

1. **Terminal UX, richer code intelligence, and browser research.** Evolve the
   line-oriented terminal attachment into a multiplexer only with a stable
   isolation and transcript contract; add LSP rename/formatting only after a
   separately reviewed edit capability design; offer native search
   before optional browser automation, each with explicit network policy and
   output limits.
2. **Control-plane completeness.** Add a versioned HTTP/App-Server-like API
   only when a local multi-client use case needs it. Keep remote execution
   separate.
3. **Delivery integrations.** Use trusted OAuth MCP servers for GitHub/GitLab
   issue, review, and CI interaction, then assess whether a first-party client
   is justified by demonstrated reliability limits.

The implementation decisions and exact test evidence belong in the source and
release notes; this audit intentionally remains a current capability map rather
than a promise that all listed future work exists.
