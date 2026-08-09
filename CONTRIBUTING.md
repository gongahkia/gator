# Contributing

## Prerequisites

- Go 1.25+
- Git
- Neovim 0.11+ for the retained legacy plugin suite
- Rust stable for the retained optional `gator-index` sidecar
- `stylua`

## Workflow

1. Keep changes scoped to one user-visible behavior or bug.
2. Add or update headless tests for behavior changes.
3. Run `make check`; run `make benchmark` when retained Lua performance-sensitive paths change.
4. Standalone changes must keep `internal/core` free of Neovim/UI dependencies, use argv subprocess execution, and add Go tests for domain behavior.
5. Describe provider-version/capability impact in the pull request.

Provider changes require checked-in fixtures, [provider-matrix](docs/PROVIDERS.md) updates, and protected live verification when that provider supports it. Do not add provider credentials, tokens, private transcripts, or machine-specific paths to the repository. The standalone provider registry must report unsupported execution capability explicitly rather than guessing a CLI protocol.
