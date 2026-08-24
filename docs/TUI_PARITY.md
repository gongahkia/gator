# TUI capability map

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
| `version` | None | CLI only | No build/version panel. |
| `update [--check]` | None | CLI only | Update and self-replacement remain outside the running TUI. |
| `rpc` | None | intentionally CLI only | Stdio protocol for programmatic clients. |
| `serve token/start/status/stop` | None | intentionally CLI only | Loopback app-server lifecycle for external clients. |
| `acp [--verify …]` | None | intentionally CLI only | Stdio editor-agent protocol. |
| `config show` | `/status` and `/permissions` show only active session state | partial | No full, redacted settings inspector. |
| `config set default-provider/default-model` | `/model` Cloud configuration saves a direct provider/model default | partial | Selecting an already-configured model with `u` remains session-only; custom and harness defaults do not have a persistent editor. |
| `config set sandbox/network` | `/permissions` is read-only | CLI only | No persistent sandbox or network policy editor. |
| `agent list` | None | CLI only | Project-defined role inventory is not displayed. |
| `child list/show/batches/batch` | None | CLI only | Retained writer-child manifests and batches are not browsable. |
| `hook status/trust/untrust` | None | CLI only | Project hook trust cannot be inspected or changed. |
| `lsp status/trust/untrust` | Runtime LSP approval can occur during a run | partial | Bundle status, hash review, and persistent trust revocation are absent. |
| `mcp status/trust/untrust/login/logout` | MCP use can occur during a trusted run | partial | Status, trust decision, OAuth setup, and credential removal are absent. |
| `worktree list/prune/remove` | Current run can be reviewed | partial | No global retained-worktree list, stale-metadata prune, or confirmed deletion. |
| `extension list/status` | `/extensions` renders cards from already trusted extensions | partial | It is not an extension inventory or status view. |
| `extension install/enable/disable/remove/trust/untrust` | None | CLI only | No extension management or project trust controls. |
| `provider list` | `/model` lists configured custom-provider models for selection | partial | Endpoint and credential-source details are not editable. |
| `provider add/discover/remove` | None | CLI only | No custom-provider editor, discovery action, or confirmed removal. |
| `local list/status` | `/model` Local section | complete | The CLI remains preferable for scripts and textual diagnostics. |
| `local serve` | Recovery prompt can start Ollama as a TUI child | complete | CLI `serve` remains useful when no TUI is running. |
| `local pull/use/remove` | Curated pull/use/remove with confirmation | complete | The per-command `--url` override has no TUI editor. |
| `theme list/set` | `/theme` | complete | — |
| `connect codex/copilot/kimi` | `/model` → Cloud → `l` runs the provider-owned sign-in flow | partial | CLI-only flags such as Codex device auth and Copilot host override are not exposed. |
| `connect xai/openrouter/radius` | `/model` → Cloud → `l` can start Gator OAuth or the corresponding supported flow | partial | Provider-specific advanced options remain CLI-only. |
| `connect claude` | `/model` → Cloud → `c` configures the masked Anthropic key and selects the Claude Code harness | complete | `gator connect claude` remains a CLI alternative for scripted onboarding. |
| `login PROVIDER` | `/model` → Cloud → `c` configures every built-in direct provider; `l` starts supported OAuth | complete | CLI remains preferable for scripted onboarding and environment-variable management. |
| `logout PROVIDER` | None | CLI only | Stored cloud credentials cannot yet be removed from `/model`. |
| `delegate codex/copilot/claude/kimi run` | `/model` selects the matching harness; send a task to run it in an isolated worktree | partial | Login options and exact one-shot CLI flags are absent. |
| `delegate opencode login/status/run` | None | CLI only | No OpenCode harness management surface. |
| `delegate external run` | None | CLI only | Arbitrary command execution belongs to the explicit CLI boundary. |
| `doctor [--provider]` | Local-model status includes host and dependency advice | partial | No full provider, sandbox, Git, web-search, and dependency diagnostic report. |
| `run` basic task | Composer, model selection, verifier editor, plan/execute mode, approvals, and review | complete | — |
| `run --provider/--model/--base-url` | Runtime drawer and `/model` configuration | partial | Endpoint override is persistent rather than an explicit one-run field. |
| `run --image/--attach` | `@` path references, preview, and confirmation | complete | — |
| `run --max-steps` | `/effort` controls bounded turn budgets | partial | No exact integer turn-cap input. |
| `run --sandbox/--network` | `/permissions` reports the policy | CLI only | No persisted policy editor. |
| `run --base/--copy-ignored/--setup` | None | CLI only | These alter worktree construction and require explicit advanced controls. |
| `run --scope/--profile/--scout` | `@` references constrain context paths | partial | No explicit scope list, profile picker, or scout assignment editor. |
| `run --allow-command/--trust-commands` | Per-command approval in TUI | partial | No pre-approved argv list or unsafe whole-run auto-approve switch. |
| `resume` | `/recent`, `Ctrl+O`, all-repository toggle, then compose a continuation | partial | No path/ID text target, exact max-step input, or direct noninteractive continuation. |
| `fork` | `/tree` and `/fork` choose a retained turn | partial | No direct ID/path, exact max steps, or noninteractive instruction flags. |
| `clone` | `/clone` clones the active retained branch | partial | No arbitrary-turn picker, direct ID/path, exact max steps, or noninteractive instruction flags. |
| `transcript RUN_RECORD_PATH` | Scrollable in-TUI transcript and `/copyall` | partial | No HTML export or arbitrary run-record picker. |
| `review RUN_RECORD_PATH [--open]` | `/review` provides structured retained-worktree review and focused diffs | partial | No browser-review server, listen-address input, or arbitrary run-record picker. |
| `export RUN_RECORD_PATH` | Review shows the exact export command | partial | No direct patch file/clipboard export action. |
| `apply [--check] RUN_RECORD_PATH` | Review shows the exact apply/check commands | partial | No explicit clean-checkout compatibility check or apply confirmation flow. |

## Gaps to implement next

These are ordered by impact on the stated goal that the TUI be a complete
configuration surface. They are not implemented by this change.

1. **Credential lifecycle in `/model`** — add a safe `remove credential`
   confirmation and provider credential metadata. Entry and provider-specific
   configuration are complete; removal remains CLI-only.
2. **Persistent execution-policy editor** — add a Settings panel for default
   provider/model, strict/off sandbox, and allow/deny network. The control
   must explain that relaxing either policy is security-relevant and save only
   after confirmation.
3. **Project integration trust center** — status, manifest hash, trust, and
   untrust for hooks, LSP, and MCP; MCP OAuth must retain the current
   trusted-manifest precondition.
4. **Custom provider manager** — text fields for ID, OpenAI-compatible chat
   endpoint, model list/default, and credential-source policy; catalog
   discovery and deletion need an explicit confirmation. Do not put a custom
   provider API key in `config.json`.
5. **Extension manager** — inventory, install path input, enable/disable,
   remove confirmation, and project trust review. Installation must preserve
   the current bounded source validation and never execute extension UI code.
6. **Retained-artifact/worktree manager** — browser for run records, child
   manifests, worktrees, transcript export, patch export, clean-check, and
   confirmed apply/remove. Apply/delete need exact target display and a final
   confirmation.
7. **Advanced run editor** — exact turn cap, base revision, setup commands,
   copy-ignored opt-in, scopes, agent profile, scout assignments, and explicit
   pre-approved argv. Each option needs a safety explanation; `trust-commands`
   should not be made a casual toggle.
8. **Diagnostics and vendor harnesses** — full Doctor report and OpenCode
   harness login/status/run controls. Keep `rpc`, `acp`, and `serve` as CLI
   contracts even if the TUI later offers status/help links for them.

## Deliberate boundaries

The TUI should configure and launch user-facing workflows, but it cannot
replace stdin/stdout protocols (`rpc`, `acp`), a long-running app-server, or a
generic arbitrary external command without changing what those commands are
for. Those remain CLI interfaces. A future TUI can show their status and give
copyable launch guidance without pretending it replaces their automation
contracts.
