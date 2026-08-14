# Gator

Gator is a native, terminal-first coding agent for developers who want an
inspectable route from a task to a tested patch.

It runs an agent inside an isolated Git worktree, streams the exact actions it
takes, and leaves the developer with a diff and verification evidence to
review. Feature work is a primary workflow: Gator is intended to explore an
unfamiliar repository, implement a bounded multi-file change, add or update
tests, and propose the resulting patch.

## Status

The project is being rebuilt from a previous agent meta-harness. The first
runnable milestone includes the native loop, OpenAI Responses adapter,
worktree-local tools, command policy, durable local run storage, and a
full-screen terminal application. Real-model usability evidence, context
compaction, broader replay coverage, and an explicit patch-application flow are
still in progress.

## Development

```sh
make check
make build
./bin/gator
./bin/gator help
./bin/gator doctor
OPENAI_API_KEY=... ./bin/gator run --verify 'go test ./...' \
  'Add a focused feature with tests'

# Continue an interrupted run using the printed run-record path.
OPENAI_API_KEY=... ./bin/gator resume /path/to/run-record \
  'Address the failing verification and finish the patch'
```

`gator` opens the interactive application when run from a Git checkout and a
real terminal. Describe the task, keep or edit the suggested verification
commands, and press `Ctrl+R` to start. `Tab` moves between task, verifier, and
model fields. During a run, `Ctrl+C` requests cancellation while retaining the
isolated worktree. The review screen shows the final report, worktree, run
record, and a diff preview; press `c` to continue a retained run, `n` for a new
task, or `d` to refresh the diff.

Type `/` (or `?` in an empty task) to filter and select local composer commands:
`/status`, `/model`, `/verify`, `/permissions`, `/worktree`, `/review`,
`/clear`, `/help`, and `/quit`. Use `@path/to/file` or `@"path with spaces"`
in a task to mark repository files or directories that the agent should inspect
first. Gator validates every reference against the repository boundary before
starting; it does not copy the referenced source into the task or journal.

The review screen deliberately does not modify the active checkout. Inspect the
printed worktree before bringing a patch into your branch. The existing `run`
and `resume` commands remain available for scripts and CI-like usage.

## Design principles

- native agent loop, rather than a wrapper around another coding agent;
- isolated worktree by default; direct edits require an explicit later mode;
- a complete event trace, patch, and verifier result for every completed run;
- deterministic tests and replayable model transcripts around all harness
  behavior;
- no claim of model or benchmark competitiveness without published evidence.

## Local run data

Runs leave code changes in a sibling `*-gator-runs/` worktree, never in the
active checkout. The printed run-record path defaults to
`$XDG_STATE_HOME/gator/` (or `~/.local/state/gator/`) and contains a
metadata-only event journal, final result, and a private `0600` session file
for `gator resume`. The event log intentionally omits prompts, source text,
tool arguments, and tool output; the worktree is the reviewable source of
truth. Set `GATOR_STATE_DIR` to use another local state root.

See [the architecture](docs/ARCHITECTURE.md) for the intended runtime and
acceptance criteria.
