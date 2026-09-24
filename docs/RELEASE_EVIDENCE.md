# Release evidence

Recorded verification for the Work depth iteration: [PRODUCT_DEPTH_VERIFICATION.md](PRODUCT_DEPTH_VERIFICATION.md).

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
gator work eval run internal/eval/testdata/work-v1/dataset.json --id release-development --attempts 3 --report-dir DIR/reports/release-development
gator work eval run internal/eval/testdata/work-v1/dataset.json --id release-held-out --split held-out --attempts 3 --report-dir DIR/reports/release-held-out
```

Use a new `--id` and `--report-dir` for every experiment. Reusing either
mixes independent trials in a way that makes comparison ambiguous.

Scripted Work runs are deterministic product gates. `--live` accepts explicit
provider, model, request-budget, and timeout values, and writes no API keys or
raw endpoint into the report. Run held-out material only as a deliberate final
comparison. See [Work evaluation](WORK_EVALUATION.md) for split and transaction
fidelity policy.

## Evaluation report

Each Work experiment writes `0600` JSON with a selected `dataset.json`, an
`experiment.json`, and one trial record per case/attempt. Development reports
contain only development cases; held-out material is included only when the
caller explicitly selects that split.

| Field | Meaning |
| --- | --- |
| experiment `id`, `dataset`, `dataset_sha256`, `split` | Corpus and explicit split provenance |
| trial `case_id`, `case_sha256`, `trial`, `family` | Fixture identity and independent attempt |
| `status`, `category`, `error` | Task-grade result and attributable failure category |
| Work lineage | `conversation_id`, `revision_id`, `snapshot_id`, contract/policy/source digests, and bundle path |
| `grades` | Deterministic task and transaction-history grader evidence |
| `transaction` | Independently inspectable Work-history, verification, delivery, recovery, and external-action states |
| `metrics`, `usage`, `duration_ms` | Bounded operational evidence; never credentials or raw provider prompts |

A passing report is evidence for its selected fixtures, provider/model policy,
budget, and harness only. It separately reports task quality and transaction
fidelity; it is not a broad model-quality claim.

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
| Apply to checkout? | no / `gator work apply --check` only / applied |
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

1. Run `gator work inspect` on a real non-Git folder and confirm no source byte or
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
7. Run `gator work code --verify ...` on a real repo and confirm it enters the main
   Gator orchestration path, retains internal Code patch evidence, and leaves
   the source unchanged.
8. Give ordinary Gator one mixed research-and-implementation prompt; confirm
   the manager delegates Code only for the bounded implementation portion.
9. Resume a Work conversation, branch from an earlier revision, and confirm the
   original revision and source snapshot remain unchanged.
10. Queue a second main-TUI prompt, force the active run to fail, and confirm
    the queue pauses rather than executing unattended.

## Remaining non-evidence

Passing this file does not establish: an installed always-on job service,
hosted agents, organization governance, broad native-office fidelity,
unrestricted computer use, support beyond Linux and macOS, or parity with
ChatGPT Work, Claude Cowork, Codex, Claude Code, Cursor, Pi, or OpenCode. See
[Durable jobs](JOBS.md) for the foreground supervisor's exact boundary.
