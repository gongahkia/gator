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
the user-facing Gator manager as a backend-only specialist. `gator code` is a
compatibility spelling for that same orchestration path, not a second product.

This is a credible local Work application, not yet a full replacement for
ChatGPT Work or Claude Cowork. It now includes a conversation-oriented TUI,
format-native DOCX/XLSX/PDF generation, a bounded set of service-specific
connectors, and a manually started durable job supervisor. It still lacks cloud
continuation, an installed always-on scheduler, organization governance, and
cross-device handoff. Its internal coding specialist likewise does not claim
full parity with the user-facing surfaces of Codex CLI, Claude Code, Cursor CLI,
Pi, or OpenCode.

## Capability map

| Surface | Gator evidence | Comparative assessment |
| --- | --- | --- |
| General Work isolation and outcomes | `gator work` accepts an ordinary folder as read-only `source/...`, writes only inside private `output/...`, and seals an embedded outcome contract with file media types, sizes, hashes, and deterministic structured and format-native validations. `gator inspect` removes artifact and action authority entirely. | This matches the source-to-deliverable shape documented for ChatGPT Work while making local boundaries and completion evidence unusually explicit. Gator remains local-only and covers a narrower collaboration surface than a cloud workspace. [OpenAI Work guide](https://learn.chatgpt.com/docs/get-started-with-work), [OpenAI Work use cases](https://learn.chatgpt.com/use-cases) |
| Artifact review and transfer | A verified Work bundle can be reviewed by run ID with safe previews, exported as deterministic `tar.gz`, or applied to an explicit existing directory after conflict preflight. The source folder is never an implicit destination. | Strong local review semantics and shell composability. Claude Cowork emphasizes finished files and connected workflows; Gator is narrower but makes artifact integrity and transfer boundaries directly inspectable. [Claude Cowork](https://claude.com/product/cowork) |
| Connected data and external actions | User-owned connectors expose only per-run selected operations. First-party Slack, Google Workspace, Atlassian, and Notion adapters and typed remote-MCP mappings share provenance and action policy. Draft never sends; act requires a fresh one-shot approval, and ambiguous remote outcomes are recorded as `unknown` rather than safe-to-retry. | The capability/action split is a differentiator, but the catalog remains deliberately bounded. Email, calendar, CRM, and broader service workflows still require service-specific schema, authentication, and semantics rather than generic webhook branding. |
| Persistent jobs | `gator job supervisor` owns private job definitions, timezone-aware scheduling, missed-run policy, single-instance locking, bounded retries, immutable attempts, and an inbox. Unattended jobs cannot approve external actions. | This is a durable foreground scheduler rather than an installed daemon or cloud task service. The user must supervise its process lifecycle; restart reconciliation remains deliberately limited. [OpenAI Work use cases](https://learn.chatgpt.com/use-cases) |
| Process isolation and approvals | `run_command` uses strict Seatbelt on macOS and Bubblewrap on Linux, fails closed elsewhere, filters environment, bounds output, and denies network by default. Worktrees remain a review boundary, not the only security boundary. | Substantive local parity with Codex's documented sandbox/approval model; platform coverage and sandbox regression testing must remain a release gate. [Codex sandboxing](https://learn.chatgpt.com/docs/sandboxing) |
| Isolated changes, resume, and review | Work conversations retain immutable source snapshots and revision branches. When Gator delegates implementation, the Code backend receives that exact snapshot in a private Git repository and returns a patch artifact without touching live source. Setup commands and project verifiers come from the user-owned child envelope. | Competitive for a local review-first flow, while intentionally replacing Code-specific base selection and thread UX with one Work history. Cursor documents similarly isolated worktrees and cleanup; Gator still has no shared dependency-cache mechanism. [Cursor worktrees](https://cursor.com/docs/configuration/worktrees) |
| MCP and credentials | Explicitly trusted stdio and Streamable HTTP servers; per-tool approval; resource-bound OAuth discovery, PKCE, DCR or pre-registered public clients, and private credential storage. | Stronger default trust posture than automatic project-server connection. It is intentionally narrower than broad plugin ecosystems. [MCP authorization](https://modelcontextprotocol.io/specification/2025-11-25/basic/authorization), [OpenCode MCP](https://opencode.ai/v2/docs/mcp-servers) |
| Local coding models | `gator local` exposes a checked-in catalog of four Ollama-hosted coding models, shows source and package size, requires confirmation before a model pull or removal, confines runtime management to literal loopback HTTP, and persists the selected installed model as a no-key OpenAI-compatible provider. The main manager and its internal Code backend use the shared provider factory. | This closes the manual endpoint-configuration gap for a small, auditable local catalog. It deliberately does not execute arbitrary Hugging Face repositories, silently install a system runtime, proxy a remote endpoint, or claim local-model quality without evaluation evidence. [Ollama API](https://docs.ollama.com/api/openai-compatibility), [Ollama model management](https://docs.ollama.com/api/introduction) |
| Rules, profiles, hooks, and extensions | Layered `AGENTS.md`, bounded declarative rules/profiles, trusted hash-pinned hooks, and explicit global or hash-pinned project extension bundles remain shared backend infrastructure. The user may pass scopes and a profile to Code; integrations require both existing trust and an explicit per-run grant. | A project can specialize the internal child without letting the manager invent permissions. This is safer and narrower than the hook/skill/plugin contracts in larger harnesses; unlike Pi packages, extensions do not receive implicit host authority. [Claude Code features](https://code.claude.com/docs/en/features-overview), [Pi extensions](https://github.com/badlogic/pi-mono/blob/main/packages/coding-agent/docs/extensions.md) |
| Subagents | The user-facing manager can schedule up to three fresh-context source researchers, artifact reviewers, or Code tasks per call, bounded to eight invocations per run. Read-only specialists see only frozen source/staged output. Code runs in a separate isolated repository, cannot recursively delegate, and returns summary, changed paths, and digest-bound patch evidence. | This is a bounded manager-as-tools design with explicit authority narrowing, not a general-purpose agent framework or child-facing product. Hosted competitors still offer detached execution, broader orchestration, and richer collaboration. [Codex subagents](https://learn.chatgpt.com/docs/agent-configuration/subagents), [Claude Code subagents](https://code.claude.com/docs/en/sub-agents), [Cursor subagents](https://prod.cursor.com/docs/subagents), [OpenCode agents](https://opencode.ai/docs/agents) |
| Code intelligence | Trusted local LSP pull diagnostics, hover, completion, code actions, formatting, rename, definitions, references, document symbols, and workspace-symbol queries remain available to internal Code only when the user grants `lsp`. Executable hash pinning, operation-specific approval, sandboxing, and wire/output limits still apply. | This preserves useful backend intelligence without exposing a second Code control center. A restart-surviving Gator-owned index and rich main-TUI code navigation remain intentionally unavailable. [LSP code action](https://microsoft.github.io/language-server-protocol/specifications/lsp/3.17/specification/#textDocument_codeAction) |
| Terminal and browser work | The retained backend has sandboxed multi-PTY tools and controlled Playwright/Chromium sessions with selected-tab and origin policy. They are unavailable to Code unless the user explicitly grants `terminal` or `browser`; browser also requires network and a selected session. There is no child terminal UI. | This supports controlled browser automation and visual verification, not full browser or cloud-agent parity. It still has no hosted VM, remote continuation, unrestricted computer use, cross-profile cookie export, or remote browser. Attached profiles remain less isolated because existing service workers cannot be disabled without changing the user's browser state. [Playwright BrowserContext](https://playwright.dev/docs/api/class-browsercontext), [Playwright service workers](https://playwright.dev/docs/service-workers) |
| Automation/control plane | Versioned local JSONL RPC handles run/steer/cancel/approve; `gator serve` exposes that protocol on an authenticated literal-loopback HTTP/SSE bridge with bounded replay and an OpenAPI wrapper document. `gator serve start/status/stop` explicitly supervises one repository-scoped background bridge, with private state/log files and token-authenticated endpoint checks before PID control. Its capabilities additionally control only explicitly detached terminal tasks in that one server process; ACP v1 over stdio maps sessions, model tools, approvals, and subagent/terminal/coordinator lifecycle updates. | This is viable for a local editor, desktop client, or CI-side controller that needs to maintain an approved dev process after the agent run. It is still not Codex's remote app-server or OpenCode's automatically discovered user-wide server: no remote listener, CORS, client-provided MCP, batch requests, media prompts, hosted transport, auto-discovery by TUI/ACP, or restart-surviving task daemon. [Codex App Server](https://learn.chatgpt.com/docs/app-server), [Cursor CLI ACP](https://prod.cursor.com/docs/cli/using), [OpenCode server](https://dev.opencode.ai/docs/server/) |
| Delivery, hosted agents, governance | Local diff/patch handoff and MCP-backed integrations only. No first-party PR/issue/CI workflow, remote executor, notifications, user identity, audit service, organization policy, or cloud handoff. | GitHub/GitLab delivery should be MCP-backed before a bespoke API. Managed background agents and governance need server-side product infrastructure. |

## Deliberate non-equivalences

- An artifact bundle is not an office suite. Gator can render and validate
  semantic DOCX, XLSX, and PDF deliverables, but it does not reproduce every
  native editing, collaboration, macro, or presentation feature.
- A generic authenticated JSON source or webhook is not branded as a complete
  SaaS integration. Service-specific connectors must add bounded resource
  selection, typed operations, OAuth scopes, idempotency, and useful errors.
- `--actions approve` is interactive by design. Headless and scheduled work may
  draft an exact action proposal but cannot inherit or synthesize approval.
- Gator's trust model rejects changed project hooks, LSP servers, and MCP
  bundles until a developer pins the new hash. This is friction by design, not
  a missing auto-connect feature.
- Gator's specialist tools are not a generic plug-in agent framework. Read-only
  children cannot change files or run processes, and Code cannot widen its
  immutable envelope or recursively delegate through prompt text.
- Code delegation returns a reviewable patch rather than applying or merging
  it. The manager and outcome contract can verify evidence, but the user still
  owns transfer into a destination checkout.
- Gator will not claim hosted-agent or enterprise parity without an authenticated
  remote service, policy distribution, audit storage, and operational support.

## Recommended build order

1. **Live main-TUI control.** Add event streaming, cancellation, and steering to
   the Work conversation without exposing a child Code interface.
2. **Integrated review and transfer.** Add a Work-bundle review/apply view that
   understands artifact validation, connected actions, and Code patch evidence.
3. **Consolidated settings.** Turn model, connector, permissions, and trust
   inspection into one main-Gator settings surface.
4. **Workflow depth.** Dogfood and harden a small number of complete
   source-to-artifact connector workflows before expanding the service catalog.
5. **Supervisor operations.** Add robust interrupted-attempt reconciliation and
   optional OS-service installation only after lifecycle behavior is proven.

The implementation decisions and exact test evidence belong in the source and
release notes; this audit intentionally remains a current capability map rather
than a promise that all listed future work exists.
