# ADR 0001: standalone Go runtime with an in-process core

## Status

Accepted.

## Context

Gator began as a Neovim plugin. Its Lua workflow is a useful behavioral specification, but the runtime, state, UI, and process lifecycle are coupled to Neovim APIs. The product now needs a terminal-native entrypoint usable in a repository without starting Neovim, while preserving provider-owned credentials, permissions, sessions, and agent loops.

The repository also contains a tested Rust `gator-index` executable. It currently has a versioned JSON-lines protocol but is not on the normal Lua workflow path.

## Decision

Introduce a Go module whose `cmd/gator` binary is the standalone product entrypoint. The initial shape is deliberately in-process:

```text
cmd/gator
  ├── CLI commands
  ├── terminal TUI
  └── internal/core
        ├── workspace and Git operations
        ├── provider capability discovery and process supervision
        ├── context manifests and prompt assembly
        ├── versioned local persistence and append-only events
        ├── review evidence, policy, episodes, and experiments
        └── UI-independent run state
```

`internal/core` has no Neovim dependency and does not write to a terminal. CLI and TUI are clients of explicit Go APIs, not owners of application state. This leaves a future local socket/API client boundary possible without requiring a daemon for V1.

Standalone artifacts live under `.gator/standalone/v1/`. This avoids silently reinterpreting the existing Lua plugin's `.gator/` records. A future importer can be explicit, versioned, and tested.

The initial TUI uses the Go standard library rather than an external framework. The decision keeps the first distributable binary dependency-free while providing a keyboard-first, testable dashboard. It is intentionally isolated behind `internal/tui`, so a richer framework can replace it without changing core semantics.

The Rust indexer is retained unchanged as an independent executable/protocol boundary. It is neither rewritten solely for language consistency nor automatically invoked until a standalone retrieval feature needs it.

## Provider boundary

Providers remain external subprocesses. A provider adapter declares its capabilities and argv construction. Gator never records terminal output as a structured transcript. Gator persists process/lifecycle metadata, context metadata, evidence, and provider-reported facts only when explicitly available.

## Consequences

- Existing Lua/Neovim code is legacy compatibility code, not a dependency of the standalone core.
- The standalone binary can run `health`, `providers`, `runs`, `events`, policy, review, and experiment operations headlessly.
- Actual provider execution is explicit and cancellation is propagated through `context.Context` to owned subprocesses.
- Worktree experiments run only in isolated worktrees and never mutate the user's active checkout.
- Provider-native structured protocols can be added incrementally without making terminal providers appear equivalent.

## Alternatives considered

### Keep Neovim as the runtime

Rejected: it cannot satisfy the standalone CLI/TUI requirement and keeps orchestration state coupled to editor state.

### Start with a local daemon/server

Rejected for V1: attachable multi-client behavior is valuable but does not justify daemon lifecycle, local authentication, and recovery complexity before the core/run model is proven. The core/client separation keeps this available later.

### Rewrite the Rust indexer in Go

Rejected: the existing executable has independent tests and a clean protocol. There is no demonstrated maintenance or performance reason to port it.
