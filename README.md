# Gator

Gator is a private Neovim workspace for running, steering, reviewing, and coordinating existing coding-agent CLIs without owning their credentials.

The first public-release target supports Aider, Amp, Cline, Codex, Claude Code, Gemini CLI, Copilot CLI, OpenCode, and Pi through capability-aware adapters. Gator keeps agent-native sessions intact while providing context packs, worktree workflows, review, and opt-in shared Git artifacts.

## Development

Run `make test` for Lua tests and `make indexer-test` for the optional Rust indexer.

## Loading

Gator supports Neovim's native packages and runtimepath-based lazy loaders. Native packages source `plugin/gator.lua` and register `:Gator` and `:GatorHealth`; lazy loaders may call `require("gator").setup()` directly without registering commands or launching agents.

## Protected live verification

`live-agent-e2e.yml` is manual-only, default-branch-only, and requires the `protected-live-agents` environment plus a `gator-live-agents` self-hosted runner. The runner supplies each provider's native login; the workflow exports no credential secrets.

## Status

Foundation only. The GitHub issue tracker is the implementation backlog.
