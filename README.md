# Gator

Gator is a Neovim workspace for steering, reviewing, and coordinating existing coding-agent CLIs without owning their credentials. It preserves provider-native sessions and policies while adding local context provenance, task/run evidence, and editor-native workflows.

## Requirements

- Neovim 0.11+
- Git
- An installed, provider-authenticated coding-agent CLI for the adapter you use
- Optional: Rust stable and `sqlite3` for `gator-index`

## Install and setup

Install with any runtimepath-compatible plugin manager, then configure Gator:

```lua
require("gator").setup({
  context = { mode = "manual", trust = "provenance" },
  telemetry = { enabled = false },
})
```

Native package loading registers `:Gator`, `:GatorHealth`, `:GatorCaptureSelection`, and `:GatorPalette`. Lazy loading may call `require("gator").setup()` directly.

Run `:GatorHealth` before first use. It verifies Neovim, Git, local policy, optional indexing, and provider availability without reading provider credentials.

## Safety model

Gator is a native-first meta-harness. Providers retain authentication, model selection, tool loops, compaction, and sandboxing. Gator may narrow a provider action or require confirmation; it never broadens provider permissions. Project instructions are discovered as provenance-tracked context and require explicit trust before transfer.

## Provider support

See [the provider capability matrix](docs/PROVIDERS.md). A listed adapter means Gator has a capability-aware integration; individual operations remain unavailable unless the installed CLI proves support.

## Development

Run `make check` for Lua, Rust, formatting, and lint checks. `make benchmark` runs fixture-backed performance cases. Protected authenticated verification is manual-only, default-branch-only, and runs on the `gator-live-agents` self-hosted runner; credentials are never exported by the workflow.

## Release and support

Gator is MIT licensed. See [CHANGELOG.md](CHANGELOG.md), [CONTRIBUTING.md](CONTRIBUTING.md), [SECURITY.md](SECURITY.md), and [SUPPORT.md](SUPPORT.md).

### Extension and marketplace trust

Gator extensions, provider CLIs, and marketplace packages execute with your user privileges. Install only extensions from maintainers you can verify; review their source, release provenance, requested permissions, and dependency changes before enabling them. Marketplace publication is not a security review or a trust guarantee. Do not install a package that asks you to disable provider sandboxing, expand permissions, or share credentials.
