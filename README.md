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
worktree-local tools, command policy, and a streaming line-oriented terminal
experience. Durable run storage, replay, context compaction, a full-screen TUI,
and real-model usability evidence are still in progress.

## Development

```sh
make check
go run ./cmd/gator help
go run ./cmd/gator doctor
OPENAI_API_KEY=... go run ./cmd/gator run --verify 'go test ./...' \
  'Add a focused feature with tests'
```

## Design principles

- native agent loop, rather than a wrapper around another coding agent;
- isolated worktree by default; direct edits require an explicit later mode;
- a complete event trace, patch, and verifier result for every completed run;
- deterministic tests and replayable model transcripts around all harness
  behavior;
- no claim of model or benchmark competitiveness without published evidence.

See [the architecture](docs/ARCHITECTURE.md) for the intended runtime and
acceptance criteria.
