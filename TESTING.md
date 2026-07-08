# paw — Testing Strategy (`docs/TESTING.md`)

Every stage must be unit-testable without hitting a real model or network. This is what lets an
independent agent implement tasks and verify them locally.

---

## 1. Fake LLM server

`internal/llm/faketest/` provides an `httptest.Server` that speaks both the OpenAI
`/chat/completions` and Ollama `/api/chat` shapes. Tests register canned responses keyed by a
substring of the request (e.g. return a specific `ContextDigest` JSON when the prompt contains a
known marker). This makes `compress`, `plan`, and `edit` deterministic in tests.

The fake also lets a test assert on the OUTBOUND request (e.g. "the brain was sent the digest,
NOT the raw context" — a regression guard for the core thesis) and can return schema-invalid or
hallucinated output on demand to exercise the validation/fallback paths.

---

## 2. Per-stage tests

- `gather`: run against `testdata/repo/` (a small fixture repo). Assert it finds known symbols,
  respects depth bounds, and NEVER calls a model (inject a nil client; a call panics).
  Also cover git-aware units for clean repos, staged changes, unstaged changes, non-git dirs, and
  max-byte truncation.
  Go syntax-aware context tests cover functions, methods, type declarations, parse errors, dedupe,
  and max-byte caps.
- `compress`:
  - Valid drone output → digest passes validation, brain would see only digest.
  - Drone returns a hallucinated `path` not in `RawContext` → item dropped.
  - Drone returns a `quote` not verbatim in the unit → item dropped.
  - Drone returns a line number outside the unit range → item dropped.
  - >50% dropped OR unparseable → heuristic fallback used; assert fallback is deterministic.
  - Assert `validation_drops` reason counters are written for each drop/fallback category.
  - Assert `token_source` recorded correctly (provider vs estimate).
- `plan`: `done=false` with no `next_action` → validation error. Enum enforcement on `kind`.
- `edit` + `patch`: see `docs/PATCH_FORMAT.md` §4 fixture list (create/delete/multi-hunk/
  multi-file/context-mismatch/path-escape).
- `verify`: run fixtures with passing, failing, and timed-out commands; assert command resolution
  for common repo types, stdout/stderr byte counts, timeout metadata, and `FailureDigest`
  truncation.
- `envelope`: round-trip marshal/unmarshal; schema-version mismatch handling.
- `schema`: marshal a fully-populated struct and validate against the embedded JSON Schema
  (keeps structs and schemas in sync).

---

## 3. Pipeline integration test (no network)

`internal/stage/pipeline_test.go`: wire all five stages with the fake LLM, run the loop against
`testdata/repo/` on a task whose fix is a known one-line diff. Assert: loop terminates, patch
applied, verify passes, and total brain-input tokens < total raw-context bytes/4 (a sanity proxy
that compression actually happened).

---

## 4. CLI / golden tests

`cmd/*_test.go`: invoke each subcommand with a fixture `Envelope` on stdin, assert the stdout
`Envelope` matches a golden file (`testdata/golden/`). This locks the pipeable text contract so
`paw gather | paw compress | ...` stays stable across refactors.
`paw stats` golden coverage also checks token source and compression validation audit summaries.

---

## 5. Logged-in CLI smoke tests

`internal/llm/cli_e2e_test.go` is skipped by default. These tests call real installed CLIs and
therefore require the user to be logged in before running. Enable one transport at a time:

```sh
PAW_E2E_CODEX_CLI=1 go test ./internal/llm -run TestCLITransportE2ESmoke/codex
PAW_E2E_GEMINI_CLI=1 go test ./internal/llm -run TestCLITransportE2ESmoke/gemini
PAW_E2E_CLAUDE_CLI=1 go test ./internal/llm -run TestCLITransportE2ESmoke/claude
PAW_E2E_OPENCODE_CLI=1 go test ./internal/llm -run TestCLITransportE2ESmoke/opencode
```

Optional model overrides:

```sh
PAW_E2E_CODEX_MODEL=gpt-5
PAW_E2E_GEMINI_MODEL=gemini-3-pro
PAW_E2E_CLAUDE_MODEL=sonnet
PAW_E2E_OPENCODE_MODEL=opencode/big-pickle
```

The smoke prompt is intentionally non-mutating: `Return exactly {"ok":true} as JSON. Do not
inspect files. Do not edit files.` The expected response is exactly `{"ok":true}`.

---

## 6. Lint / CI gates
- `go vet ./...`
- `golangci-lint run` (config `.golangci.yml`)
- `go test ./... -race`
- `gofmt -l .` must be empty
The Harbor/Docker benchmark is NOT run in unit CI (needs Docker + keys); it has a separate manual
make target and a documented smoke procedure.
