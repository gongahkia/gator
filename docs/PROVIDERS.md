# Provider support

Gator selects only adapters that pass their local executable/version/capability checks. Provider authentication remains provider-owned. `user_confirmed` is an explicit user readiness assertion for CLIs without a machine-readable auth-status command; it is not credential verification.

| Provider | Auto transport | Resume/handoff truth |
| --- | --- | --- |
| Pi | Gator chat through `pi --mode rpc` | Gator records Pi's session ID/path and exact session usage. Same-provider resume/fork requires Pi confirmation; cross-provider handoff starts a new session. |
| Codex | Gator chat through App Server when the tested protocol contract is available | Gator records the App Server thread. Same-provider `thread/resume`/`thread/fork` requires the returned thread identity to match; terminal fallback uses Codex's provider-owned resume flow. |
| Aider, Amp, Cline, Copilot, Cursor, Gemini, Goose, Kimi, Vibe | Managed/ACP chat only when each installed adapter advertises the required contract | Capability and resume behavior remain adapter-specific; Gator presents provider approval requests interactively where ACP emits them. |

Gator never scrapes a terminal to fabricate chat history. A terminal-originated run opens a separate Gator companion split with focus, stop, detach, run-list, journal, and handoff controls; prompts and output remain in the provider terminal. A terminal run can still hand off its objective, captured source, current diff, bounded changed-text-file snapshots, and an editable note, but is explicitly marked `transcript unavailable`. Cross-provider handoff creates a new provider session and materializes Gator-owned files under `.gator/handoffs/<bundle-id>/files/`; it does not claim to migrate opaque provider state.

For structured and managed chats, Gator marks a turn stalled after the configured `launch.stall_after_ms` (120 seconds by default) without an adapter event, records only that metadata, and exposes cancel, detach, and journal controls. It does not retry, inject a prompt, terminate, or infer the provider's underlying state.

Gator marks previously active local records `detached` on startup. It does not reconnect by PID or recreate an empty chat as if it were a provider session. Resume is offered only when the stored session declares support and the selected provider confirms the same session identity.

## Explicit ACP commands

An ACP-compatible local agent can be added only through explicit configuration:

```lua
require("gator").setup({
  acp = {
    commands = {
      localagent = { argv = { "local-agent", "--acp" } },
    },
  },
})
```

Gator starts exactly that argv. It negotiates agent capabilities at initialization and uses `session/load`, `session/resume`, or `session/list` only when the agent advertises the corresponding capability. A configured executable is not a credential check and does not mean its session history or usage counters exist.

## Readiness

Run `:GatorHealth` in the Git project you want to use. It reports an adapter as:

- `detected`: executable and provider-native readiness/auth probe passed.
- `user_confirmed`: executable/contract passed and the user enabled the adapter opt-in; credentials are not verified.
- `indeterminate`: an executable, version, capability, or auth requirement is unavailable.

Pi requires:

```lua
require("gator").setup({
  providers = { pi = { user_confirmed = true } },
})
```

Set that only after configuring Pi's own local provider credentials.

## Unsupported providers

Claude Code and OpenCode are not supported by Gator. Use their own CLIs outside Gator.

## Version policy

Fixture-tested version ranges are informational, not launch gates. Gator accepts any parseable provider version when its required local protocol/capability probe passes; versions outside the fixture-tested range remain visible in `:GatorHealth!` as unverified. Gator never updates a provider or requires an upgrade: if the required protocol probe fails, use the provider's own CLI and report the incompatibility in [GitHub Issues](https://github.com/gongahkia/gator/issues).
