# Contributing

## Prerequisites

- Neovim 0.11+
- Rust stable and `sqlite3` for optional sidecar/database workflows
- `stylua`

## Workflow

1. Keep changes scoped to one user-visible behavior or bug.
2. Add or update headless tests for behavior changes.
3. Run `make check`; run `make benchmark` when performance-sensitive paths change.
4. Describe provider-version/capability impact in the pull request.

Provider changes require checked-in fixtures, [provider-matrix](docs/PROVIDERS.md) updates, and protected live verification when that provider supports it. Do not add provider credentials, tokens, private transcripts, or machine-specific paths to the repository.
