# Contributing

## Prerequisites

- Neovim 0.11+
- Rust stable and `sqlite3` for the optional indexer tests
- `stylua`

Run `make check` before opening a pull request. Provider changes require fixtures, capability-matrix updates, and protected live verification when the provider supports it.

Do not add provider credentials, tokens, or private transcripts to the repository.
