# Release evidence

This is the acceptance record for calling a Gator build useful for daily
general work or coding work. It is not a benchmark submission and not a claim
of model quality.

## Automated evidence

`go test ./...` covers:

- non-Git source isolation, named read-only/write-only roots, symlink escape
  rejection, and descriptor-rooted regular-file reads;
- typed text, JSON, and CSV writers with per-file and aggregate limits;
- deterministic outcome contracts, artifact inspection, hashes, manifests,
  media-aware previews, portable export, and conflict-aware apply;
- selected connected JSON sources with URL-bound credentials and sealed
  provenance;
- webhook proposals bound to an exact target, escaped JSON preview, and
  payload digest, including draft-only behavior, fresh approval, denial,
  definite failure, and uncertain remote outcomes;
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
gator eval suite DIR --live --provider PROVIDER --model MODEL --environment-id OCI_IMAGE_DIGEST --attempts 3 --report-dir DIR/reports/UNIQUE_ID --require-resolved
```

Use a new `--run-id` for every attempt. Reusing an id against a previous
report file invites cached conclusions; the harness always executes, but
humans comparing reports will mix attempts if ids collide.

The offline form is a deterministic harness regression only. The suite form
requires `--live`, strict scored fixtures, and an immutable `--environment-id`
because it is reserved for model-quality evidence. `--live` accepts explicit
provider, model, and base-URL overrides and writes no API keys or raw endpoint
into the report. See [headless evaluation](EVALUATION.md) for fixture policy,
container operation, repeated trials, and the evidence threshold.

## Evaluation report

Each report is `0600` JSON with:

| Field | Meaning |
| --- | --- |
| `id` | Fixture identity from `eval.json` |
| `run_id` | This attempt. Must be unique per prediction |
| `status` | Final `resolved`, `unresolved`, or `error`, including the post-run score outcome |
| `agent_status` | Agent/verifier outcome before hidden post-run scoring |
| `task` | Exact task string |
| `provider` / `model` | Adapter used |
| `max_steps` / `steps` | Budget and actual turns |
| `timeout_seconds` | Wall-clock cap |
| `verify` | Exact argv list that had to pass |
| `sandbox` / `network` | Requested fixture execution policy |
| `base_commit` | Fresh baseline commit created from the copied fixture |
| fixture/harness/environment provenance | `fixture_sha256`, `harness_version`, `harness_commit`, `environment_id`, plus a hashed provider endpoint |
| `scopes` / command policy / `setup` | Fixed evaluation authority; do not put secrets in these fields |
| `score` / `score_status` / `score_results` | Trusted hidden oracle argv and bounded post-run diagnostic evidence |
| `suite_id` / `attempt` / `labels` | Corpus identity, independent trial, and predeclared category |
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

For Work sessions also record:

| Field | Fill in |
| --- | --- |
| Source folder (name only) | |
| Outcome contract | artifact paths, validators, external-action disposition |
| Bundle ID | |
| Review verification | pass/fail |
| Export/apply | not attempted / check only / applied |
| Connected sources | IDs only, or “none” |
| External actions | none / pending / denied / executed / failed / unknown |

Copy the table into `docs/dogfood/` as `YYYY-MM-DD-<short-sha>.md` if you
keep local notes. Do not commit transcripts, session files, or credentials.

### Minimum dogfood set before calling a release daily-driver

1. Run `gator inspect` on a real non-Git folder and confirm no source byte or
   external state changes.
2. Produce Markdown plus JSON or CSV from a real folder; verify the bundle and
   preview it by ID.
3. Export that bundle, run apply preflight against an existing directory, then
   apply only after reviewing conflicts; confirm the source remains unchanged.
4. Use an authenticated connected JSON source and confirm the manifest records
   resource, retrieval time, byte count, and digest without a credential.
5. Create a webhook proposal with `--actions draft`; confirm no request is sent
   and the review shows escaped exact JSON plus its digest.
6. In an isolated test endpoint, deny one act-mode proposal and approve another;
   confirm only the approved payload is sent and each decision is requested.
7. New code Execute run with `--verify` on a real repo; review worktree; do not
   apply until `--check` is clean.
8. Plan then Execute on the same retained coding thread.
9. Resume `--last`, then fork an earlier turn and confirm the source thread is
   unchanged.
10. Cancel a running turn and confirm retained evidence is reviewable.

## Remaining non-evidence

Passing this file does not establish: built-in persistent jobs, hosted agents,
organization governance, general DOCX/XLSX/PDF generation, unrestricted
computer use, support beyond Linux and macOS, or parity with ChatGPT Work,
Claude Cowork, Codex, Claude Code, Cursor, Pi, or OpenCode. See
[Durable jobs](JOBS.md) for why scheduling remains a separate release gate.
