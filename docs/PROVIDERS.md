# Provider support

Gator selects only adapters that pass their local executable/version/capability checks. Provider authentication remains provider-owned. `user_confirmed` is an explicit user readiness assertion for CLIs without a machine-readable auth-status command; it is not credential verification.

| Provider | Auto transport | Resume/handoff truth |
| --- | --- | --- |
| Pi | Gator chat through `pi --mode rpc` | Gator obtains Pi's RPC session ID and records reported session usage; cross-provider handoff starts a new session. |
| Codex | Gator chat through App Server when the tested protocol contract is available | Gator records a new App Server thread; terminal fallback uses Codex's provider-owned resume flow. |
| Claude Code | Native Neovim terminal | Gator does not treat headless streaming as a durable interactive chat contract. |
| OpenCode | Native Neovim terminal | Gator records the provider session created by its bridge. |
| Aider, Amp, Cline, Copilot, Cursor, Gemini, Goose, Kimi, Vibe | Managed/ACP chat only when each installed adapter advertises the required contract | Capability and resume behavior remain adapter-specific; Gator presents provider approval requests interactively where ACP emits them. |

Gator never scrapes a terminal to fabricate chat history. A terminal-originated run can still hand off its objective, captured source, current diff, bounded changed-text-file snapshots, and an editable note, but is explicitly marked `transcript unavailable`. Cross-provider handoff creates a new provider session and materializes Gator-owned files under `.gator/handoffs/<bundle-id>/files/`; it does not claim to migrate opaque provider state.

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

## Version policy

Fixture-tested version ranges are enforced by each adapter probe. Gator does not treat a newer CLI version as compatible solely because its executable exists. Update an adapter range only with matching protocol fixtures and a local verification run.
