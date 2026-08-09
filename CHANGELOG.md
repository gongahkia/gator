# Changelog

## Unreleased

- Added standalone Go `gator` CLI/TUI with a Neovim-independent core, versioned local state, run/event/evidence/episode records, policy fingerprints, review verification, worktree plans, and bounded policy experiments.
- Retained the Lua Neovim implementation and Rust `gator-index` sidecar as legacy compatibility components while standalone provider adapters are ported incrementally.
- Added capability-gated Copilot ACP reattach with an interactive CLI-resume fallback and explicit readiness states.
- Added explicit managed-session detach/stop lifecycle and `VimLeavePre` child shutdown.
- Added local authenticated Copilot bridge E2E coverage.
- Added first-class adapters for Amp, Aider, Cline, Cursor Agent, Goose, Kimi Code CLI, and Mistral Vibe.
- Added public-release documentation, provider capability matrix, and release hardening.

## Versioning

Gator follows semantic versioning. Adapter capability changes and Neovim support changes are documented in this file.
