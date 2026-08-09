# Legacy Neovim compatibility implementation

The Lua implementation under `lua/` and `plugin/` is the pre-standalone Gator client/runtime. It remains available during the standalone migration for existing users and as a detailed behavioral specification for provider capability handling, context redaction, worktrees, handoffs, review, and health checks.

New standalone development belongs in the Go runtime (`cmd/gator`, `internal/core`, and `internal/tui`) and must not add a Neovim dependency to that core. A future `gator.nvim` should become a thin client/integration over the standalone domain or local API, rather than being re-expanded into the primary runtime.

The retained Lua suite is still part of `make check`. See [the standalone migration guide](STANDALONE.md) for current standalone support and limitations.
