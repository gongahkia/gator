# Product capability audit

This is a source-backed product audit as of 2026-09-09, not a model-quality
benchmark. It compares the committed Gator architecture and tests with public
primary documentation. A documented competitor capability is not evidence that
it is reliable in every environment; it is the feature bar users can reasonably
expect.

## Conclusion

Gator now has a distinct product thesis beyond a generic coding harness: local
folders and explicitly connected data become validated, reviewable artifact
bundles. The source is read-only, output is isolated, completion is checked by
a developer-owned contract, and external actions are independently proposed
and approved. The existing isolated coding workflow remains available under
`gator code`.

This is a credible Work CLI foundation, not yet a full replacement for ChatGPT
Work or Claude Cowork. It lacks a conversation-oriented Work TUI, format-native
DOCX/XLSX/PDF generation, rich service-specific connectors, built-in persistent
jobs, and cloud continuation. The coding workflow likewise does not claim full
parity with Codex CLI, Claude Code, Cursor CLI, Pi, or OpenCode. Hosted agents,
organization governance, and cross-device handoff remain separate services.

## Capability map

| Surface | Gator evidence | Comparative assessment |
| --- | --- | --- |
| General Work isolation and outcomes | `gator work` accepts an ordinary folder as read-only `source/...`, writes only inside private `output/...`, and seals an embedded outcome contract with file media types, sizes, hashes, and deterministic JSON/CSV/text validations. `gator inspect` removes artifact and action authority entirely. | This matches the source-to-deliverable shape documented for ChatGPT Work while making local boundaries and completion evidence unusually explicit. It is currently CLI-only and supports fewer document formats and no cloud workspace. [OpenAI Work guide](https://learn.chatgpt.com/docs/get-started-with-work), [OpenAI Work use cases](https://learn.chatgpt.com/use-cases) |
| Artifact review and transfer | A verified Work bundle can be reviewed by run ID with safe previews, exported as deterministic `tar.gz`, or applied to an explicit existing directory after conflict preflight. The source folder is never an implicit destination. | Strong local review semantics and shell composability. Claude Cowork emphasizes finished files and connected workflows; Gator is narrower but makes artifact integrity and transfer boundaries directly inspectable. [Claude Cowork](https://claude.com/product/cowork) |
| Connected data and external actions | User-owned connector descriptors expose only per-run selected operations. JSON reads carry URL, time, byte count, and SHA-256 provenance. Webhook actions pin the exact endpoint and JSON digest; draft never sends, act requires a fresh one-shot approval, and ambiguous remote outcomes are recorded as `unknown` rather than safe-to-retry. | The capability/action split is a differentiator, but the catalog is intentionally tiny. Rich email, calendar, Drive, CRM, and project-management workflows still require service-specific schema, authentication, and semantics rather than generic webhook branding. |
| Persistent jobs | No built-in scheduler is claimed. `docs/JOBS.md` specifies a separate private supervisor, immutable execution history, missed-run policy, overlap control, credential preflight, and a prohibition on unattended approval. | Honest gap versus products with scheduled tasks. Existing launchd/systemd/CI can supervise headless `gator work`; a background goroutine or detached terminal would not meet the durability bar. [OpenAI Work use cases](https://learn.chatgpt.com/use-cases) |
| Process isolation and approvals | `run_command` uses strict Seatbelt on macOS and Bubblewrap on Linux, fails closed elsewhere, filters environment, bounds output, and denies network by default. Worktrees remain a review boundary, not the only security boundary. | Substantive local parity with Codex's documented sandbox/approval model; platform coverage and sandbox regression testing must remain a release gate. [Codex sandboxing](https://learn.chatgpt.com/docs/sandboxing) |
| Isolated changes, resume, and review | New runs use a retained Git worktree; the original checkout is untouched. A developer can select an immutable base, copy only explicitly listed ignored files, and run exact sandboxed `--setup` argv before scouts or the model. Session threads can resume, fork from a recorded patch snapshot, inspect a diff, or export/apply a patch. | Competitive for a local review-first flow. Cursor documents similarly isolated worktrees and cleanup; Gator still has no shared dependency-cache or project-authored setup-script mechanism. [Cursor worktrees](https://cursor.com/docs/configuration/worktrees) |
| MCP and credentials | Explicitly trusted stdio and Streamable HTTP servers; per-tool approval; resource-bound OAuth discovery, PKCE, DCR or pre-registered public clients, and private credential storage. | Stronger default trust posture than automatic project-server connection. It is intentionally narrower than broad plugin ecosystems. [MCP authorization](https://modelcontextprotocol.io/specification/2025-11-25/basic/authorization), [OpenCode MCP](https://opencode.ai/v2/docs/mcp-servers) |
| Local coding models | `gator local` exposes a checked-in catalog of four Ollama-hosted coding models, shows source and package size, requires confirmation before a model pull or removal, confines runtime management to literal loopback HTTP, and persists the selected installed model as a no-key OpenAI-compatible provider. The normal Gator loop, tools, worktrees, sandbox, sessions, TUI, JSONL RPC, and ACP remain unchanged. | This closes the manual endpoint-configuration gap for a small, auditable local catalog. It deliberately does not execute arbitrary Hugging Face repositories, silently install a system runtime, proxy a remote endpoint, or claim local-model quality without evaluation evidence. [Ollama API](https://docs.ollama.com/api/openai-compatibility), [Ollama model management](https://docs.ollama.com/api/introduction) |
| Rules, profiles, hooks, and extensions | Layered `AGENTS.md`, bounded declarative rules/profiles, capability-bounded project role prompts, trusted hash-pinned hooks, and explicit global or hash-pinned project extension bundles. Extension sidecars use the active strict sandbox and require per-tool approval. | The project can specialize a child prompt, but cannot configure arbitrary tools or permissions. This is safer and narrower than the hook/skill/plugin contracts in larger harnesses; unlike Pi packages, extensions do not receive implicit host authority. [Claude Code features](https://code.claude.com/docs/en/features-overview), [Pi extensions](https://github.com/badlogic/pi-mono/blob/main/packages/coding-agent/docs/extensions.md) |
| Subagents | CLI callers can schedule up to four separate-worktree scouts. Native models have `delegate_readonly`: batches of one to four fresh-context scouts, bounded to eight per run, share the active worktree only through read/search/Git tools, and return 8 KiB untrusted reports. Execute mode offers one serial writer or exactly two parallel writers, with two writers total per run. Each writer has a separate worktree from the same parent snapshot; parallel calls must declare non-overlapping paths, then Gator records scope violations and overlap. Ref-free patch commits feed `git merge-tree --write-tree`, so real textual conflicts are reported without touching refs or worktrees. Parent-owned manifests retain effective narrowed policy, ownership/deadline/heartbeat, baseline, verifier/patch digests, child state, and comparison evidence. Writer roles may further restrict inherited policy. Patch application remains explicit and Gator never auto-merges. | This is a bounded, conflict-aware local writer scheduler without permission escalation. Codex, Claude Code, Cursor, and OpenCode still offer user/global definitions, detached execution, broader orchestration, and richer semantic conflict handling. Gator has durable foreground lifecycle evidence but no detached/background writer registry or automatic conflict resolution. [Codex subagents](https://learn.chatgpt.com/docs/agent-configuration/subagents), [Claude Code subagents](https://code.claude.com/docs/en/sub-agents), [Cursor subagents](https://prod.cursor.com/docs/subagents), [OpenCode agents](https://opencode.ai/docs/agents) |
| Code intelligence | Trusted local LSP pull diagnostics, hover, completion, immediate code actions, formatting, rename, definitions, references, document symbols, and workspace-symbol queries with executable hash pinning, operation-specific approval, a strict sandbox, wire/output limits, and one lazy server per configuration. Supported servers receive bounded disk-snapshot `didOpen`/`didChange`/`didClose` synchronization. ACP may add explicit external read-only roots for read/navigation/indexing; edit suggestions remain confined to the isolated worktree. The native TUI and loopback app server retain up to eight idle managers only for a compatible retained worktree, trusted bundle hash, and root set; the next incompatible run retires the cache and process exit stops it. | This closes basic navigation, lookup-completion, reviewed quick-fix/format/rename discovery, document synchronization, and session-local resume indexing. A restart-surviving Gator-owned index remains intentionally unavailable. [LSP code action](https://microsoft.github.io/language-server-protocol/specifications/lsp/3.17/specification/#textDocument_codeAction) |
| Terminal and browser work | Execute mode has `terminal_start/read/write/list/stop`: a sandboxed multi-PTY task manager with bounded scrollback and explicit developer input. An explicit `gator browser` session now provides pinned local Playwright/Chromium, JavaScript rendering, selected-tab control, screenshots, user-only authentication, bounded artifacts, uploads, downloads, and per-mutation approval. Managed sessions use a temporary profile; attached sessions require literal-loopback CDP and developer-selected tabs. Exact origin policy covers managed navigation, frames, subresources, fetch/XHR, and WebSockets; service workers are blocked for managed contexts. | This closes controlled local browser automation and visual verification, not full browser or cloud-agent parity. It still has no hosted VM, remote continuation, PR workflow, unrestricted computer use, cross-profile cookie export, or remote browser. Attached profiles remain less isolated because existing service workers cannot be disabled without changing the user's browser state. [Playwright BrowserContext](https://playwright.dev/docs/api/class-browsercontext), [Playwright service workers](https://playwright.dev/docs/service-workers) |
| Automation/control plane | Versioned local JSONL RPC handles run/steer/cancel/approve; `gator serve` exposes that protocol on an authenticated literal-loopback HTTP/SSE bridge with bounded replay and an OpenAPI wrapper document. `gator serve start/status/stop` explicitly supervises one repository-scoped background bridge, with private state/log files and token-authenticated endpoint checks before PID control. Its capabilities additionally control only explicitly detached terminal tasks in that one server process; ACP v1 over stdio maps sessions, model tools, approvals, and subagent/terminal/coordinator lifecycle updates. | This is viable for a local editor, desktop client, or CI-side controller that needs to maintain an approved dev process after the agent run. It is still not Codex's remote app-server or OpenCode's automatically discovered user-wide server: no remote listener, CORS, client-provided MCP, batch requests, media prompts, hosted transport, auto-discovery by TUI/ACP, or restart-surviving task daemon. [Codex App Server](https://learn.chatgpt.com/docs/app-server), [Cursor CLI ACP](https://prod.cursor.com/docs/cli/using), [OpenCode server](https://dev.opencode.ai/docs/server/) |
| Delivery, hosted agents, governance | Local diff/patch handoff and MCP-backed integrations only. No first-party PR/issue/CI workflow, remote executor, notifications, user identity, audit service, organization policy, or cloud handoff. | GitHub/GitLab delivery should be MCP-backed before a bespoke API. Managed background agents and governance need server-side product infrastructure. |

## Deliberate non-equivalences

- An artifact bundle is not an office suite. Markdown, text, JSON, CSV, and
  safe previews are real current formats; DOCX, XLSX, slides, and rendered PDFs
  remain future format-specific writers and validators.
- A generic authenticated JSON source or webhook is not branded as a complete
  SaaS integration. Service-specific connectors must add bounded resource
  selection, typed operations, OAuth scopes, idempotency, and useful errors.
- `--actions approve` is interactive by design. Headless and scheduled work may
  draft an exact action proposal but cannot inherit or synthesize approval.
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

1. **Work conversation lifecycle.** Add a Work-native retained session and TUI
   around the existing bundle contract without merging it into Git run state.
2. **Format-native deliverables.** Add one excellent DOCX or XLSX writer with
   structural validation and preview before expanding format breadth.
3. **One complete connected workflow.** Pick a real source-to-artifact use case
   and add service-specific resource selection, OAuth scopes, and typed errors;
   retain generic JSON/webhook as infrastructure.
4. **Foreground product evidence.** Dogfood non-Git folder, structured-data,
   connector, draft-action, approval, review, export, and apply flows before
   claiming a daily driver.
5. **Durable jobs, later.** Implement `job run` and immutable execution records
   first, then ship a supervisor only after every gate in `docs/JOBS.md` passes.

The implementation decisions and exact test evidence belong in the source and
release notes; this audit intentionally remains a current capability map rather
than a promise that all listed future work exists.
