# Gator

Gator is a provider-independent terminal work agent. It researches selected
sources, reconciles tables, writes documents, and delegates bounded coding work
inside one persistent conversation. Sources are captured before execution;
deliverables, code candidates, verification, and external-action proposals remain
reviewable before transfer.

The runtime is Go. It reuses the retained Gator Code engine as an isolated
specialist. Local execution, conversation state, evaluation, and recovery do not
require an observability service.

## Build and start

The manifest and CI pin Go 1.25.13. Git and a strict process sandbox are needed
for Code work: Bubblewrap on Linux or Seatbelt on macOS.

```sh
GOTOOLCHAIN=go1.25.13 make build
./bin/gator
```

Configure a provider inside the TUI with `/model`. Press Tab for Local to
download, select, rename, or delete Ollama models, with progress and explicit
confirmation. The TUI can start an installed Ollama runtime and keeps it alive
until Gator exits. Native
credentials stay in the private credential store. See [local models](docs/LOCAL_MODELS.md),
[custom providers](docs/CUSTOM_PROVIDERS.md), and [Work](docs/WORK.md).

```sh
gator inspect --source ./notes 'Identify gaps in these sources'
gator work --source ./notes --artifact brief.md \
  'Write a cited brief; distinguish conflicting and missing evidence'
gator work --source ./accounts --artifact checked.xlsx --artifact memo.md \
  'Reconcile the CSVs by invoice ID and amount using reconcile_tables'
gator work --source ./repository --artifact report.md --verify 'go test ./...' \
  'Implement the requested change with Code, integrate and verify its patches, and explain the result'
```

Use `--connector ID` to select a configured service and `--web-origin
https://example.org` to permit bounded web retrieval. Reads do not grant mutation
or publishing. Scheduled runs support inspect/draft only.

## Continue, review, and transfer

```sh
gator work list
gator work history CONVERSATION_ID
gator work resume CONVERSATION_ID -- 'Revise the brief and keep its earlier constraints'
gator work --conversation CONVERSATION_ID --parent REVISION_ID 'Create another branch'
gator review RUN_ID --preview
gator export RUN_ID --to ./deliverable.tar.gz
gator apply RUN_ID --to ./documents --check
```

A Code candidate is applied explicitly, separately from ordinary file transfer:

```sh
gator apply RUN_ID --code-patch code/SELECTED-CANDIDATE.patch --to ./target-repo --check
gator apply RUN_ID --code-patch code/SELECTED-CANDIDATE.patch --to ./target-repo
```

The selected candidate must have passed aggregate verification. Preflight checks
its retained hashes, the target's cleanliness, and the original bytes of every
changed path. An ordinary artifact apply copies files, including any `.patch`
file; it does not apply code changes.

While Work runs, `/steer TEXT` updates the current task; ordinary text queues a
future turn. `/tasks`, `/cancel-task ID`, `/approve`, `/deny`, and `/cancel` expose
specialist and approval control. Ctrl+C cancels active work and retains its
outcome. Historical task checkpoints are available with `gator work tasks RUN_ID`.

## Verify and evaluate

```sh
GOTOOLCHAIN=go1.25.13 make check
gator eval work validate internal/eval/testdata/work-v1/dataset.json
gator eval work run internal/eval/testdata/work-v1/dataset.json \
  --id work-local --attempts 2 --report-dir /tmp/gator-work-local
gator eval work show /tmp/gator-work-local/experiment.json
```

The 24-case corpus runs through the actual Work service and native Code adapter.
Scripted trials measure execution and grading behavior; they do not establish
live model quality. See [Work evaluation](docs/WORK_EVALUATION.md) for comparisons,
role ablations, budgeted live runs, independent rubric grading, and optional
LangSmith export. [Verification evidence](docs/PRODUCT_DEPTH_VERIFICATION.md)
records the checks actually run for this iteration.

## Product and compatibility guides

- [Architecture decision](docs/PRODUCT_DEPTH_DECISION.md) and [current Work behavior](docs/WORK_DEPTH.md)
- [Specialists and recovery](docs/ORCHESTRATION.md), [jobs](docs/JOBS.md), and [review](docs/REVIEW.md)
- [Work-native headless protocol](docs/WORK_NATIVE_PROTOCOL.md)
- [State migrations](docs/WORK_DEPTH_MIGRATIONS.md) and [Code frontend migration](docs/CODE_TO_GATOR_MIGRATION.md)

RPC, ACP, and app-server coding-engine protocols remain compatibility surfaces.
`work-rpc` is the separately versioned Work-native surface. The standalone Code
frontend remains retired. Automated checks do not establish live provider,
authenticated connector, hosted export, or macOS sandbox results unless the
verification record explicitly says those checks ran.
