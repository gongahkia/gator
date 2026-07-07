# Contributing

Keep changes scoped and testable. Deterministic stages must not import `internal/llm`.

## Add a stage

1. Implement `internal/stage.Stage`: `Name() string` and `Run(context.Context, *envelope.Envelope) (*envelope.Envelope, error)`.
2. Put stage logic in `internal/<stage>/`.
3. Add a `cmd/<stage>.go` subcommand that reads an envelope from stdin and writes an envelope to stdout.
4. Add unit tests for the stage package.
5. Add or update a golden CLI test in `cmd/*_test.go`.
6. Wire the stage into `paw run` only after its standalone subcommand is covered.

## Add an LLM transport

1. Implement `llm.Client` in `internal/llm/<transport>.go`.
2. Keep provider request/response structs private to that file.
3. Normalize responses into `llm.ChatResponse` with usage populated from provider fields when present.
4. Support schema-constrained output if the provider exposes it; otherwise use the nearest deterministic fallback shape.
5. Register the transport in `internal/llm/factory.go`.
6. Add tests using `internal/llm/faketest` or an `httptest.Server`; no real network calls in unit tests.

## Checks

```sh
make fmt
make test
make lint
go vet ./...
```

`make test` runs `go test ./... -race`. Benchmark checks are separate: see [BENCHMARKS.md](BENCHMARKS.md), [docs/RESULTS.md](docs/RESULTS.md), and `make bench-smoke`.

## Install pre-commit hooks

```sh
brew install pre-commit
pre-commit install
pre-commit run --all-files
```

The hooks run `gofmt -w`, `go vet ./...`, and `golangci-lint run`. The `golangci-lint` hook installs `v2.12.2`, matching CI.
