# Full Work evaluation

`gator eval work` evaluates target `work.v1` through the actual Work service,
manager, supervisor, Code adapter, tools, contracts, and revision store. Existing
`gator eval DIR` and `gator eval suite DIR` remain direct Code compatibility
targets. Their fixtures and scoring behavior are preserved.

## Local experiments

```sh
gator eval work validate internal/eval/testdata/work-v1/dataset.json
gator eval work run internal/eval/testdata/work-v1/dataset.json \
  --id enabled --attempts 2 --report-dir /tmp/gator-enabled
gator eval work run internal/eval/testdata/work-v1/dataset.json \
  --id disabled --attempts 2 --delegation=false --report-dir /tmp/gator-disabled
gator eval work compare /tmp/gator-disabled/experiment.json /tmp/gator-enabled/experiment.json
gator eval work show /tmp/gator-enabled/experiment.json
```

Use a new report directory for each experiment. For example,
`--disable-role source_researcher` permits an individual role ablation. Unknown
or repeated roles are rejected before creating an experiment. Reports expose delegation/routing differences;
comparison requires the same dataset identity, number of trials, and scripted
versus live mode. The dataset identity includes source file contents and modes.
An optional declared `fixture_sha256` pins bytes and makes mismatch a validation
error. Fixture symlinks, paths outside the dataset, unknown graders, and invalid
contracts are rejected.

The checked-in corpus contains 24 tasks, six in each family: evidence research,
tables/documents, code integration, and continuation/control. Each family has
four development and two held-out cases. The corpus covers conflicting sources,
missing evidence, extraction failure, secret exclusion, role authority, duplicate
and missing table values, exact decimals, independent/overlapping patches,
combined verifier failure, sequential conversation state, old-parent branches,
contract fidelity, budget exhaustion, and invalid tasks. Dedicated integration
tests additionally exercise connected/web evidence follow-ups, native Code
follow-ups with frozen project configuration, live approvals/control, scheduling
crash boundaries, and apply-time conflicts.

Scripted runs exercise product execution and grader mechanics. Disabling a tool
needed by a fixed script is an execution ablation; the resulting pass-rate change
is not evidence that delegation improves a live model's reasoning. No provider
quality threshold is selected from scripted measurements.

## Grading and retained reports

`dataset.json`, `experiment.json`, per-trial JSON, readable `report.txt`, private
Work state, and bounded metadata traces are retained locally. Trial records
include contract/policy/source hashes, fixture identity, split, harness, usage,
latency, independent grades, and conversation/revision/bundle identifiers.
Grader definitions and answers remain outside agent-readable source roots.

Deterministic graders check actual artifact text, workbook cell values, status,
contract digests, source integrity, quotation/source references, tool denials,
candidate status, and replay constraints. A passing file-format validator does
not imply correct cell values or supported prose. Grader soundness tests include
fabricated quotations, nonexistent citations, wrong text, missing output, and
fixture mutations. Source integrity is independently checked after every trial.

Reports distinguish grading outcomes from execution outcomes. An expected budget
refusal may pass its test while still appearing as budget exhaustion in execution
counts. Categories include task, fixture, provider, sandbox/setup, grader,
timeout, cancellation, and budget. Reports display denominators, per-case trial
counts, at-least-one success, and success on every trial. Model requests,
unreported usage, retries, interventions, tool failures, artifact checks,
unsupported exact quotations, latency, and token totals are reported. Semantic
entailment and cost are not inferred from those counters.

## Budgeted live execution

```sh
gator eval work run internal/eval/testdata/work-v1/dataset.json \
  --id live-baseline --live --provider PROVIDER --model MODEL \
  --max-model-requests 256 --timeout-seconds 1800 --attempts 2 \
  --report-dir /tmp/gator-live-baseline
```

Provider, model, request budget, and wall budget are mandatory with `--live`.
The request budget is shared across cases, children, retries, and follow-ups.
Token enforcement depends on reported usage; concurrent calls can overshoot the
reported-token boundary. Synthetic model turns never count as live validation.
`.github/workflows/work-live-eval.yml` provides manual dispatch behind a named
environment and explicit opt-in variable. Repository administrators must configure
the environment's required reviewers and credentials; that external protection
is not established by committing YAML.

## Independent synthesis rubric

The versioned `synthesis-rubric.json` defines two criteria with five anchored
scores each. A separate tool-free judge receives retained textual deliverables
and evidence, not the producing agent's private transcript. Nonzero scores must
cite text actually present in the judging input.

```sh
gator eval work judge-rubric /tmp/gator-live-baseline/experiment.json \
  --live --rubric internal/eval/testdata/work-v1/synthesis-rubric.json \
  --provider JUDGE_PROVIDER --model JUDGE_MODEL \
  --max-model-requests 32 --timeout-seconds 300 --output /tmp/judgements.json
gator eval work calibrate-rubric /tmp/judgements.json /tmp/human-scores.json
```

`judge-rubric --live` explicitly sends retained textual artifact/evidence content
to the judge provider. Rubric results are labeled uncalibrated. Human annotations
use rubric-report v1, `source: "human"`, a named `reviewer`, matching experiment,
rubric and input hashes, and selected trial/criterion scores. Calibration reports
pair counts, exact agreement, agreement within one point, and mean absolute
error. Synthetic calibration tests verify arithmetic only. No real human
calibration or live synthesis-quality result is claimed by the local corpus.

## Optional export

`GATOR_OTLP_ENDPOINT` enables bounded metadata-only OTLP HTTP/JSON export at the
end of Work. Parent/child, model attempts, tools, and sealing are represented in
local trace v1. Prompts, arguments, source contents, and private replay are
excluded. The trace retains exporter errors; exporter failure does not replace
local execution state. Traces are bounded to 512 spans; the dropped count is
explicit. The OTLP endpoint can target a compatible collector, including a
Langfuse pipeline, without another custom runtime.

```sh
gator eval work export-langsmith /tmp/gator-enabled/experiment.json
```

This command requires `LANGSMITH_API_KEY`; `LANGSMITH_ENDPOINT` can override the
API base. It creates dataset/example/experiment associations, exports the trace
hierarchy and independent scores, and attaches feedback to the corresponding run.
It uploads metadata rather than task contents or hidden grader answers. Local
HTTP tests cover associations, hierarchy, redaction, and failures. Hosted round
trips require a separately authorized execution with valid credentials.
