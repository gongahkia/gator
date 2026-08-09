# Standalone architecture and migration status

## Product boundary

Gator is a local orchestration layer around provider-owned coding agents. The Go core owns repository discovery, objectives, explicit context, run lifecycle, worktrees, handoffs, review evidence, local persistence, policy, and experiments. Providers own credentials, models, native tools, sessions, compaction, and permissions unless a future structured integration exposes a safe bridge.

```text
                    terminal TUI / headless CLI
                               │
                               ▼
                    internal/core (no terminal I/O)
      ┌────────────────────────┼────────────────────────┐
      ▼                        ▼                        ▼
  workspace/store       policy/context/review      provider registry
      │                        │                        │
  Git/worktrees    events/evidence/episodes    owned subprocesses
```

V1 uses an in-process core/client boundary. It does not start a daemon simply to emulate attachable clients. Its public command surface is intentionally small but the domain is independent of `cmd/gator` and `internal/tui`, so a future socket/API server or `gator.nvim` client can target the same core.

## Go layout

- `cmd/gator`: command parsing and process signal handling.
- `internal/core`: UI-independent domain, persistence, Git, context, provider process, review evidence, policy, and experiment logic.
- `internal/tui`: keyboard-first terminal dashboard client.

The core accepts `context.Context` for commands and provider execution. Gator owns every process it starts through `exec.CommandContext`, waits for it, and uses a bounded kill fallback during cancellation. Commands are argv arrays; Gator never constructs shell command strings.

## Persistence and events

Standalone state is intentionally namespaced under `.gator/standalone/v1` rather than interpreting legacy Lua records. Records are schema-versioned JSON plus append-only JSONL event journals. Runs record the exact effective policy fingerprint. Episodes normalize observed outcome data—task fingerprint, workspace SHA/diff, provider, policy, duration, run state, verification state, and provider-reported usage if ever supplied—without persisting raw source, prompts, terminal output, or native transcripts.

Evidence is separate from a provider assertion. A verification record contains the configured command argv, exit result, timestamps, base/diff hashes, and a digest; it intentionally omits command output. Review commands can only come from the effective policy's named command list.

## Provider boundary

| Provider | Discovery | Standalone execution | Transport claim | Limitation |
| --- | --- | --- | --- | --- |
| Codex | executable/version probe | implemented with `codex exec` | terminal, native output | App Server structured session/resume bridge is not yet ported. |
| Pi | executable/version probe | not yet implemented | terminal capability only | No unsupported prompt/session syntax is guessed. |
| Aider, Amp, Cline, Copilot, Cursor, Gemini, Goose, Kimi, Vibe | executable/version probe | not yet implemented | capability metadata only | ACP/stream/history transports require standalone adapters before execution. |

This distinction is deliberate: Gator does not convert terminal output into a fabricated structured transcript, silently substitute providers, or claim resume/approval/session support it cannot enforce.

## Policy, evidence, and experiments

`Policy` version 1 has bounded routing, context limits, one of two workflow topologies, and an allowlist of verification argv commands. Schema validation rejects unsupported values, empty argv, and unbounded context. Security/privacy controls are not mutable policy parameters.

An experiment runs baseline and candidate policies in separate Git worktrees. Dry runs record plans and are `inconclusive`. An executing comparison is only scored when both runs pass deterministic configured verification; it reports a transparent latency comparison and never promotes a policy automatically. This is an evidence-first continuation path, not an autonomous evolver.

## Legacy Neovim migration

The `lua/`, `plugin/`, and `tests/` trees remain the legacy Neovim implementation. They are useful behavioral specifications and preserve existing users while the standalone product gains structured provider parity. Standalone Go code has no imports, subprocesses, or runtime assumptions involving Neovim.

The optional Rust `crates/gator-index` binary is retained unchanged. It continues to expose its versioned JSON-lines process protocol and independent Rust tests. It is not automatically supervised by standalone V1 because no standalone retrieval operation currently needs it; integrating it later must retain the process/protocol boundary and explicit data/consent controls.

## Known V1 limitations

- The TUI is a focused command dashboard, not yet an attachable multi-client terminal multiplexer.
- Only Codex has a standalone execution adapter; all other discovered providers remain explicitly unsupported for execution.
- Legacy Lua run records are preserved but not imported.
- Handoff transfers metadata/provenance and starts a new run; it does not migrate opaque native provider sessions.
- The experiment comparator is intentionally conservative and does not include policy promotion or an evolver.
