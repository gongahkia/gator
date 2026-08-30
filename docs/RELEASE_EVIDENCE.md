# Release evidence

This is the acceptance record for calling a Gator build useful for daily
work. It is not a SWE-bench submission and not a claim of model quality.

## Automated evidence

`go test ./...` covers:

- the bounded tool loop, policy denials, and state transitions
- disposable-repository feature, failing-verifier, and resume/interrupt runs
- provider history replay (function-call correlation and opaque provider data)
- malformed tool calls (unknown name, invalid JSON, missing call id) that
  return a tool error and never execute
- transient HTTP 429/503 retries that do not append failed assistant turns
- context compaction that summarizes only older history, rejects tool-calling
  summaries, and fail-closes without returning partial history

Evaluation runner checks:

```sh
go test ./internal/eval
gator eval DIR --run-id UNIQUE_ID --script DIR/script.json --report DIR/reports/UNIQUE_ID.json --require-resolved
gator eval suite DIR --live --provider PROVIDER --model MODEL --report-dir DIR/reports/UNIQUE_ID --require-resolved
```

Use a new `--run-id` for every attempt. Reusing an id against a previous
report file invites cached conclusions; the harness always executes, but
humans comparing reports will mix attempts if ids collide.

The offline form is a deterministic harness regression only. The suite form
requires `--live` because it is reserved for model-quality evidence. `--live`
accepts explicit provider, model, and base-URL overrides and writes no API keys
into the report. See [headless evaluation](EVALUATION.md) for fixture policy,
container operation, and the evidence threshold.

## Evaluation report

Each report is `0600` JSON with:

| Field | Meaning |
| --- | --- |
| `id` | Fixture identity from `eval.json` |
| `run_id` | This attempt. Must be unique per prediction |
| `status` | `resolved` (verifier passed), `unresolved` (ran, failed), or `error` (timeout or harness failure) |
| `task` | Exact task string |
| `provider` / `model` | Adapter used |
| `max_steps` / `steps` | Budget and actual turns |
| `timeout_seconds` | Wall-clock cap |
| `verify` | Exact argv list that had to pass |
| `sandbox` / `network` | Requested fixture execution policy |
| `base_commit` | Fresh baseline commit created from the copied fixture |
| `scopes` / command policy / `setup` | Fixed evaluation authority; do not put secrets in these fields |
| `duration_ns` | Wall time |
| `error` | Failure text, never credentials |
| `state_path` / `worktree_path` | Local artifacts for review |

A resolved report is evidence for that fixture, model, policy, budget, and
time limit only. It does not make a competitive or agent-quality claim without
a defined corpus and live results.

## Dogfood checklist

Record one row per session. Do not silently skip a failed item.

| Field | Fill in |
| --- | --- |
| Date | |
| Gator commit | `git rev-parse HEAD` |
| Repository (name only, not secrets) | |
| Provider / model | |
| Mode (plan/execute) | |
| Sandbox / network | |
| Task (one sentence) | |
| Outcome | patch ready / needs input / policy refusal / failure |
| Verifier | exact argv and pass/fail |
| Apply to checkout? | no / `gator apply --check` only / applied |
| Usability issues | |
| Fixes landed or follow-ups | issue ids or “none” |

Copy the table into `docs/dogfood/` as `YYYY-MM-DD-<short-sha>.md` if you
keep local notes. Do not commit transcripts, session files, or credentials.

### Minimum dogfood set before calling a release daily-driver

1. New Execute run with `--verify` on a real repo; review worktree; do not
   apply until `--check` is clean.
2. Plan then Execute on the same retained thread.
3. Resume `--last` with a continuation instruction.
4. Fork or clone an earlier turn and confirm the source thread is unchanged.
5. Deny an exploratory command, then allow-once, then confirm the worktree
   still matches intent.
6. `/doctor` inspect-only; `/manage` trust status without mutating unless
   intended.
7. Browser review on loopback (`gator review PATH` or TUI `b`); confirm the
   checkout is untouched.
8. Cancel a running turn and confirm the worktree is retained.

## Remaining non-evidence

Passing this file does not establish: hosted agents, org governance, detached
writer teams, full JavaScript/screenshot browser automation, unrestricted
computer use, Windows strict sandbox, or parity with Codex CLI,
Claude Code, Cursor CLI, Pi, or OpenCode.
