# Code-to-Gator product migration

## Product decision

Gator is the only user-facing agent. The original Code application is no
longer a second terminal product: its isolated Git executor is an internal
specialist that Gator may call with a bounded task. `gator code` and `gator run`
remain compatibility spellings, but both enter the Gator manager and require a
retained Code patch before the manager can complete.

The authority direction is one-way:

```text
user controls + objective
          |
          v
    Gator manager
          |
          +-- immutable capability envelope
          v
 internal Code specialist --> summary + changed paths + patch digest
          |
          x  no direct UI, no source mutation, no recursive delegation
```

## Complete legacy-surface map

| Original Code capability or convenience | Product decision | Gator location |
| --- | --- | --- |
| Provider, model, credentials, endpoint overrides, provider options, custom providers | Promote unchanged | Shared configuration and model factory used by the Gator manager and Code backend |
| Centered composer and dock-after-submit layout | Promote | Main `internal/worktui` home and conversation views |
| Cloud/provider onboarding and credential lifecycle | Promote and simplify | Main `/model`, `/connect`, `/login`, `/logout`, first-run setup, and their root CLI equivalents |
| Effort presets and exact turn budgets | Promote | Main `/effort`; separate manager and Code budgets |
| Prompt image/PDF/Office/text attachments | Promote | Main `/attach`, `--image`, and `--attach`; bytes go to the manager, not silently to a child |
| Prompt queue | Promote | Main 16-item, in-memory FIFO queue; each item captures its own options |
| Copy latest answer | Promote | Main `/copy` |
| Themes | Promote | Main `/theme` and persisted root theme setting |
| Help, status, permissions, doctor, agent/profile and settings inspection | Promote | Main `/help`, `/status`, `/permissions`, `/doctor`, `/agents`, and `/settings` |
| Code project verifiers | Promote into delegation policy | Auto-detected for common project roots; repeatable `--verify` or `/code verify` |
| Scoped `AGENTS.md`, rules, and named profiles | Promote into delegation policy | `--scope`, `--profile`, `/code scope`, and `/code profile` |
| Worktree setup commands | Promote with explicit user ownership | `--setup` or `/code setup`; never accepted from the manager model |
| Exact and prefix command approvals | Promote with explicit user ownership | `--allow-command`, `--allow-command-prefix`, `/code allow`, and `/code allow-prefix` |
| Interactive approval prompts | Keep only where the invoking root CLI owns stdin | Headless JSON/TUI child runs require pre-grants; no model-created approval |
| Sandbox and network selection | Promote as explicit Code envelope | Strict/deny by default; `--sandbox`, `--network`, or matching `/code` commands |
| LSP, MCP, extensions, HTTP/web research, browser, terminal | Promote as two-key grants | Existing trust/configuration plus a per-run `--code-capability` or `/code grant`; browser also requires a selected session and network |
| Git isolation, repository instructions, journaling, final diff/status, patch export | Keep in backend | `internal/run`, invoked only through `work_code_agent.go` for product use |
| Initial static scouts and dynamic reader/writer children | Reframe | Gator owns source research, artifact review, and Code delegation; Code cannot recursively delegate |
| Code thread resume/fork/clone | Replace | Gator Work conversations and immutable revision branches |
| Code recent-run picker and thread tree | Replace | `Ctrl+X` conversation picker plus `/history`, `/back`, and `/forward`; `Ctrl+P` is reserved for actions |
| Code diff/review/transcript UI | Replace at product boundary | Sealed Work bundle, subagent patch evidence, root `gator review`, `export`, and historical read compatibility |
| Base-ref selection | Retire for managed delegation | Code receives frozen source plus an explicitly selected accepted candidate |
| Copy ignored files into a worktree | Retire for managed delegation | Snapshot inclusion/exclusion is explicit, bounded, and recorded before delegation |
| Sandbox-off/network confirmation modal | Replace | The user must type the explicit root flag or `/code` setting; the manager cannot choose it |
| Attached interactive terminal UX | Retire from Code frontend | Terminal can be an explicitly granted internal tool, but has no child-facing UI |
| Vim composer mode, mouse-heavy diff controls, Code-only drawers and control center | Retire | They do not improve the single, conversation-first Gator surface enough to justify a second UI state machine |
| Vendor-harness chooser inside Code | Keep as root boundary, not child convenience | Explicit `gator delegate`; never selected silently by Gator or Code |
| Extension prompt commands and declarative Code UI cards | Retire from the product surface | Bundle metadata remains readable for compatibility, but only explicitly granted backend tools are active |
| JSONL RPC, ACP, and app-server transports | Keep as machine-facing backend compatibility | They remain non-TUI integration surfaces around the retained engine; a future incompatible Work-native protocol must use Work conversations and capability envelopes |

## Delegation invariants

- The selected source is snapshotted before the manager or Code specialist
  starts. Code works against a private Git repository made from that snapshot.
- The manager may choose a narrower task but cannot change provider authority,
  commands, sandbox, network, integrations, or step limits.
- `git diff --check` is always required and cannot be removed. Project-specific
  verifiers are additive.
- Code writer/scout delegation is always disabled to prevent recursive agent
  trees. Parallelism belongs to Gator's manager.
- Integration names are not grants. A capability must be explicitly enabled,
  and its existing project trust/authentication checks still apply.
- A compatibility `gator code`/`gator run` request cannot complete without a
  successful Code invocation and a retained patch artifact.
- The child never applies its patch to live source. Review and transfer stay at
  the Gator product boundary.

## Current port and remaining UX

Work now uses normalized private conversation replay and Code context compaction,
forwards model streaming/visual declarations, captures approved project
configuration with original trust identity, and offers live steering,
cancellation, and exact approval responses. Code candidates support selected
baselines, integration, independent frozen review, and explicit target apply.
See [state migrations](WORK_DEPTH_MIGRATIONS.md) and [current behavior](WORK_DEPTH.md).

The standalone Code frontend remains retired. Historical Code journals, direct
Code evaluation fixtures, RPC, ACP, app-server, and legacy review remain readable
compatibility surfaces. A full historical diff browser and terminal attachment
are not reintroduced into the Work TUI. Root management commands continue to
manage providers, browser sessions, integration installation, and project trust.
