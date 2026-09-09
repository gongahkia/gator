# Historical Code TUI capability map

> This document records the retired standalone Code interface for migration
> and regression archaeology. It is not a current product contract. Gator's
> active terminal surface is `internal/worktui`; see
> [Code-to-Gator product migration](CODE_TO_GATOR_MIGRATION.md).

This document maps the public CLI in `cmd/gator/main.go` to the interactive
TUI. It is intentionally a capability audit, not a promise that every
machine-oriented protocol should become an interactive screen.

Status terms:

- **complete**: the developer can complete the same interactive task in the
  TUI.
- **partial**: the main workflow exists, but meaningful CLI options or a
  management action are missing.
- **CLI only**: no equivalent TUI action exists.
- **intentionally CLI only**: an IPC/server or stream-oriented interface;
  exposing it through the TUI would not replace its consumer-facing contract.

## Cloud and local model configuration

`/model` is the model-management entry point. In its Cloud section, choose a
provider and press `c` to open the configuration form. Every built-in direct
provider shown in that catalog has a configuration path:

- every provider can set an exact model ID and an optional endpoint override;
- API-key providers use a masked API-key field;
- Azure OpenAI accepts a resource URL or complete endpoint plus API version;
  Azure OpenAI Responses can instead use a user-supplied bearer token;
- Amazon Bedrock accepts an optional masked bearer token or the standard AWS
  chain, plus optional region and named shared-profile selection;
- Google Vertex accepts an optional masked access token or Application Default
  Credentials (ADC), plus project, location, and an optional absolute ADC
  credential-file path;
- Cloudflare Workers AI and AI Gateway configure account/gateway metadata;
  AI Gateway also configures its request protocol;
- Radius configures its optional gateway URL;
- Claude Code uses a masked Anthropic API key and selects its delegated
  harness. Gator never imports Claude.ai subscription credentials;
- Codex and Copilot configure their model/endpoint here and use `l` for their
  provider-owned OAuth sign-in. Kimi, xAI, OpenRouter, and Radius also expose
  their supported OAuth routes through `l`.

An empty credential input retains the current stored credential. The field is
never placed in a draft, transcript, status message, or `config.json`.
Endpoints and the provider-specific, non-secret settings above are saved in
`config.json`; credentials are saved in the private `auth.json` store, whose
directory and file use private permissions.

The Local section supports catalog/status, prerequisite help, optional Ollama
startup, curated download, selection, display rename, and local-weight removal.
It deliberately displays installation guidance rather than executing an OS
package manager.

## CLI-to-TUI coverage

| CLI capability | TUI coverage | Status | Remaining difference |
| --- | --- | --- | --- |
| `tui` | Default interactive surface | complete | — |
| `help` | `/help`, `?`, and `F1` | partial | TUI documents TUI controls, not the full pipeable CLI reference. |
| `version` | `/version` | complete | Shows running build provenance without network I/O. |
| `update [--check]` | `/update` | partial | The TUI checks only; download and self-replacement remain an explicit post-exit CLI action. |
| `rpc` | None | intentionally CLI only | Stdio protocol for programmatic clients. |
| `serve token/start/status/stop` | None | intentionally CLI only | Loopback app-server lifecycle for external clients. |
| `acp [--verify …]` | None | intentionally CLI only | Stdio editor-agent protocol. |
| `config show` | `/manage` → Config | complete | Bounded redacted inspector never reads `auth.json`; secret-shaped values and sensitive URL query values are hidden. |
| `config set default-provider/default-model` | `/manage` saves the provider/model currently selected in `/model` after confirmation | complete | — |
| `config set sandbox/network` | `/manage` shows and confirms strict/off sandbox and deny/allow network defaults; `/permissions` explains the effective policy | complete | — |
| `agent list` | `/agents` lists profiles and capability-bounded roles | complete | Profiles and role overlays can only narrow policy; roles never expand tools. |
| `child list/show/batches/batch` | `/manage` selects arbitrary retained runs and browses writer-child status, role, patch size, worktree, batch schedules, and path-conflict evidence | complete | — |
| `hook status/trust/untrust` | `/manage` shows configured hash/state and confirms trust changes | complete | — |
| `lsp status/trust/untrust` | `/manage` shows configured hash/state, idle session-cache status, and confirms trust changes; runtime operations still require approval | complete | Opening `/manage` inspects the cache without starting servers. |
| `mcp status/trust/untrust/login/logout` | `/manage` shows configured hash/state, display-safe auth status, and confirms trust, loopback PKCE login, or credential removal | complete | Login stays unavailable until the current `.gator/mcp.json` hash is trusted. |
| `worktree list/prune/remove` | `/manage` lists retained worktrees and confirms metadata pruning or deletion | complete | — |
| `extension list/status` | `/manage` shows installed extension metadata/state; `/extensions` renders trusted static cards | complete | — |
| `extension install/enable/disable/remove/trust/untrust` | `/manage` stages a local directory or HTTPS Git source, reviews the hash, and commits those exact bytes after confirmation | complete | TUI install rejects SSH/`git@`/plain HTTP. CLI may still clone those sources. |
| `provider list` | `/model` lists configured custom-provider models and shows endpoint/credential-source status | complete | — |
| `provider add/discover/remove` | `/model` creates and edits custom Chat Completions providers, previews `/models`, and confirms catalog replacement or removal | complete | CLI remains preferable for scripts. Discovery still requires an explicit apply step. |
| `local list/status` | `/model` Local section | complete | The CLI remains preferable for scripts and textual diagnostics. |
| `local serve` | Recovery prompt can start Ollama as a TUI child | complete | CLI `serve` remains useful when no TUI is running. |
| `local pull/use/remove` | Curated pull/use/remove with confirmation | complete | The per-command `--url` override has no TUI editor. |
| `theme list/set` | `/theme` | complete | — |
| `connect codex/copilot/kimi` | `/model` → Cloud → `l` runs the provider-owned sign-in flow | partial | CLI-only flags such as Codex device auth and Copilot host override are not exposed. |
| `connect xai/openrouter/radius` | `/model` → Cloud → `c` exposes endpoint/advanced settings and `l` starts the supported sign-in flow | complete | xAI's vendor-owned sign-in selects the OpenCode harness afterwards. |
| `connect claude` | `/model` → Cloud → `c` configures the masked Anthropic key and selects the Claude Code harness | complete | `gator connect claude` remains a CLI alternative for scripted onboarding. |
| `login PROVIDER` | `/model` → Cloud → `c` configures every built-in direct provider; `l` starts supported OAuth | complete | CLI remains preferable for scripted onboarding and environment-variable management. |
| `logout PROVIDER` | `/model` → Cloud → `d` confirms removal of the stored Gator credential and reports remaining ambient sources | complete | Environment variables, AWS/ADC, and vendor CLI logins are never unset from the TUI. |
| `delegate codex/copilot/claude/kimi run` | `/model` selects the matching harness; send a task to run it in an isolated worktree | partial | Login options and exact one-shot CLI flags are absent. |
| `delegate opencode login/status/run` | `/opencode` runs status/login in the vendor CLI and selects `PROVIDER/MODEL` for the next task | complete | Gator keeps the isolated-worktree and verification boundary; OpenCode keeps credentials, tools, approvals, and session state. |
| `delegate external run` | None | CLI only | Arbitrary command execution belongs to the explicit CLI boundary. |
| `doctor [--provider]` | `/doctor` reports Git, provider auth kind/status, sandbox availability, web search, dependencies, and local models | complete | Inspect-only: it never starts Ollama, OAuth, or mutates configuration. |
| `run` basic task | Composer, model selection, verifier editor, plan/execute mode, approvals, and review | complete | — |
| `run --provider/--model/--base-url` | Runtime drawer, `/model` persistent endpoint, and `/run` one-run base URL | complete | `/run` endpoint is session-local (draft + this process). `/model` remains the persistent default. |
| `run --image/--attach` | `@` path references, preview, and confirmation | complete | — |
| `run --max-steps` | `/effort` plus `/run` exact turn cap | complete | An exact cap overrides `/effort` for that run. |
| `run --sandbox/--network` | `/manage` persists defaults; `/run` overrides them for this process only | complete | Relaxing to sandbox-off or network-allow requires confirmation. Not written to `config.json`. |
| `run --base/--copy-ignored/--setup` | `/run` edits these for new worktrees; setup and copy-ignored require confirmation | complete | Hidden on resume/fork. Setup is Execute-only. |
| `run --scope/--profile/--scout` | `@` references plus `/run` extra scopes, profile picker, and ≤4 scouts | complete | Profiles can only narrow policy. |
| `run --allow-command/--trust-commands` | Per-command approval plus `/run` exact argv and literal-token prefixes | partial | `--trust-commands` remains CLI-only and is not a TUI toggle. |
| `resume` | `/recent`/`Ctrl+O` list, `p` text target, or `/resume TARGET [instruction…]` | complete | Exact CLI `--max-steps` remains `/run` / `/effort`. |
| `fork` | `/tree`, `/fork`, or `/fork TARGET [instruction…]` | complete | Exact CLI `--max-steps` remains `/run` / `/effort`. |
| `clone` | `/clone` or `/clone TARGET [instruction…]` | complete | Exact CLI `--max-steps` remains `/run` / `/effort`. |
| `transcript RUN_RECORD_PATH` | `/manage` selects retained runs and writes private HTML exports below Gator's state directory; active transcript remains scrollable and copyable | complete | — |
| `review RUN_RECORD_PATH [--open]` | `/review`, `/review TARGET`, and `b` loopback browser review | complete | Listen stays loopback-only. This is not `gator serve`. |
| `export RUN_RECORD_PATH` | `/manage` selects retained runs and writes private patch exports below Gator's state directory | complete | — |
| `apply [--check] RUN_RECORD_PATH` | `/manage` requires a successful clean-checkout compatibility check for the selected run, then a separate apply confirmation | complete | — |
| `eval DIR` | None | CLI only | Offline `--script` or `--live` evaluation; writes a JSON report. Not a TUI workflow. |

## Gaps to implement next

These are ordered by impact on the stated goal that the TUI be a complete
configuration surface. Completed portions are marked below; remaining detail
defines the next bounded iteration rather than implying broad parity.

1. **Credential lifecycle in `/model`** — implemented: `d` removes a stored
   Gator credential after confirmation, names the credential kind, and reports
   remaining ambient sources. Environment variables and vendor CLI stores are
   not unset. Claude Code removal targets the Anthropic store key.
2. **Persistent execution-policy editor** — implemented in `/manage` for the
   currently selected provider/model, strict/off sandbox, and deny/allow
   network. Security-relaxing changes explain their effect and save only after
   confirmation.
3. **Project integration trust center** — implemented in `/manage` for status,
   exact manifest hash, trust, and untrust for hooks, LSP, MCP, and project
   extensions. MCP OAuth login is available from the mcp auth tab after the
   current manifest hash is trusted.
4. **Custom provider manager** — implemented in `/model` for ID, Chat
   Completions URL, model list/default, optional API-key environment variable
   name, two-step `/models` discovery, and confirmed removal. API keys are
   never written to `config.json`.
5. **Extension manager** — `/manage` stages a local directory or HTTPS Git
   source, shows the staged hash, and publishes those exact bytes only after
   confirmation. Enable/disable, removal, and project trust remain confirmed
   actions.
6. **Retained-artifact/worktree manager** — `/manage` now selects arbitrary
   repository run records, scopes writer-child browsing to the selection,
   writes private transcript/patch exports, performs clean-checkout
   compatibility checks, confirms apply, manages worktree prune/remove, and
   displays writer-batch path-conflict evidence.
7. **Advanced run editor** — `/run` edits exact turn cap, one-run base URL,
   one-run sandbox/network (confirm before off/allow), base revision,
   setup argv, copy-ignored opt-in, extra scopes, a narrowing profile,
   bounded scouts, exact allow-command argv, and literal-token prefixes.
   Setup, copy-ignored, sandbox-off, and network-allow require confirmation.
   `--trust-commands` is not a TUI toggle. One-run fields are not written to
   `config.json`.
8. **Retained targeting and browser review** — `/resume`, `/fork`, `/clone`,
   and `/review` accept a thread ID, unique prefix, or run-record path.
   An instruction after the target starts that continuation immediately.
   `/recent` also accepts a typed target (`p`). Review `b` starts the
   loopback browser listener with an editable listen address and optional
   open; it is not `gator serve` and has no RPC.
9. **Diagnostics and vendor harnesses** — implemented: `/doctor` is a get-only
   local report; `/opencode status`, `/opencode login PROVIDER [METHOD]`, and
   `/opencode use PROVIDER/MODEL` expose the installed OpenCode harness without
   importing its credential or session state. `/version` is local build
   provenance and `/update` is check-only. Keep `rpc`, `acp`, and `serve` as
   CLI contracts even if the TUI later offers status/help links for them.

## Deliberate boundaries

The TUI should configure and launch user-facing workflows, but it cannot
replace stdin/stdout protocols (`rpc`, `acp`), a long-running app-server, or a
generic arbitrary external command without changing what those commands are
for. Those remain CLI interfaces. A future TUI can show their status and give
copyable launch guidance without pretending it replaces their automation
contracts.
