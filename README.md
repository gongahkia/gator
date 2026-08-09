# Gator

Gator is a local-first, terminal-native meta-harness for coding agents. It launches and surrounds existing agent CLIs with repository context, workspaces, policy, event/evidence records, review, and reproducible experiments. Gator is not another model or agent loop: providers continue to own their own credentials, models, native tools, sessions, permissions, and compaction.

The standalone `gator` binary is the primary interface. It works from a Git repository without launching Neovim.

```text
Gator TUI / CLI
       │
       ▼
UI-independent Go core ── provider subprocesses (Codex, Pi, ACP-capable CLIs, ...)
       │
       ├── .gator/standalone/v1/ runs, append-only events, evidence, episodes
       ├── Git repositories and isolated worktrees
       └── optional Rust gator-index executable via its versioned JSONL protocol
```

The existing Lua plugin remains in this repository as legacy Neovim compatibility code and as a behavioral reference. It is not a runtime dependency of the standalone core. See [the migration guide](docs/STANDALONE.md) and [ADR 0001](docs/adr/0001-standalone-go-runtime.md).

## Install and build

Requirements:

- Go 1.25 or newer;
- Git;
- an installed provider CLI for any provider you execute;
- Rust stable only when building/testing the retained optional `gator-index` sidecar.

```sh
make standalone-check
make gator-build
./bin/gator help
```

## Normal standalone flow

Open the primary keyboard-first terminal UI from a Git checkout:

```sh
gator
```

The TUI shows repository/branch status, effective policy, recent runs, provider readiness, event history, and verification status. Its `run` flow collects an objective, provider, explicit files/ranges, diff choice, worktree choice, and an explicit execution confirmation. When it executes a terminal provider, Gator temporarily gives that provider the terminal and does not capture its output as a fake structured transcript.

The same core is available headlessly:

```sh
# Inspect local capability discovery; this never reads credentials.
gator providers
gator health

# Record a reviewable metadata-only plan. Providers are not started without --execute.
gator run --provider codex --file README.md "Summarise the project architecture"

# Run the first standalone execution adapter (Codex CLI's `exec` subcommand).
gator run --provider codex --worktree --execute "Add a focused regression test"

gator runs
gator events run-<id>
gator review run-<id>
gator review --verify run-<id>
gator handoff run-<id> codex
gator worktrees
```

`--execute` is always explicit. Current standalone execution support is intentionally narrower than provider discovery: Gator can discover all legacy providers, but it reports an unavailable execution adapter rather than pretending their semantics are equivalent. See [provider status](docs/STANDALONE.md#provider-boundary).

## Context, state, review, and safety

The standalone core treats an objective as fundamental; editor selections are only one possible future context source. Current context sources are explicit workspace files/ranges and the current Git diff. Gator redacts known secret-like strings before delivery and persists only a context manifest (paths, byte counts, hashes, line ranges, and redaction counts), not source text.

Standalone state is versioned and kept separately from legacy Lua artifacts:

```text
.gator/standalone/v1/
  policy.json
  runs/
  events/
  evidence/
  episodes/
  experiments/
```

Event journals are JSONL and append-only while Gator writes them. Verification accepts only named argv commands defined in the effective policy; it does not infer or run arbitrary shell commands. Evidence records command identity, argv, result, time, and Git hashes, but not command output. Gator worktrees are created under a sibling `gator-worktrees/` directory and experiments never write to the active checkout.

Provider credentials, native permission systems, sandbox settings, extensions, and telemetry consent remain outside the mutable orchestration policy. Telemetry is disabled by default.

## Policy and experiments

Initialize a project-local standalone policy:

```sh
gator policy init
gator policy show
gator policy validate
```

The versioned, schema-validated policy can control bounded routing, context limits, a limited workflow topology, and named verification commands. Its fingerprint is recorded with every run. It cannot change credentials, permission escalation, provider sandboxes, telemetry consent, or destructive Git privileges.

Compare two policies in isolated worktrees:

```sh
gator experiment compare \
  --provider codex \
  --baseline policies/baseline.json \
  --candidate policies/candidate.json \
  "Fix the failing parser test"
```

Without `--execute`, this produces two isolated, reproducible plans and an explicitly `inconclusive` comparison. With `--execute`, both provider runs must pass their deterministic configured verification commands before Gator can make a latency-based comparison. Gator never automatically promotes a candidate policy.

## Development

```sh
make check             # Lua + Rust + Go formatting, tests, vet, and binary build
make standalone-check  # Go formatting, tests, vet, and standalone binary build
make gator-build       # ./bin/gator
make test              # retained Lua/Neovim suite
make indexer-test      # retained Rust sidecar suite
make benchmark         # retained Lua benchmark suite
```

The Go tests run without provider credentials. Live provider verification remains opt-in. Consult [CONTRIBUTING.md](CONTRIBUTING.md), [standalone architecture](docs/STANDALONE.md), [provider details](docs/PROVIDERS.md), and [legacy Neovim documentation](docs/LEGACY_NEOVIM.md).
