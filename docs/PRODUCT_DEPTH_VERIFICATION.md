# Product depth verification

Recorded September 10, 2026 (Asia/Singapore). This record describes deterministic
implementation evidence, not live model quality or a hosted-service release gate.
The [architecture decision](PRODUCT_DEPTH_DECISION.md) gives the six-stage
dependency order; [migration semantics](WORK_DEPTH_MIGRATIONS.md) describe old
state and recovery limits.

## Repository and environment

The research baseline is `70adf39b57e7bd5469d792e3674ca7f331f5512e`. Initial
discovery found only untracked research evidence. The current tree and retained
pre-Work Code history were inspected before implementation. No separate original
Code checkout was available for comparison. The diagnostic reproduction patch
was not applied to the working tree; regressions assert corrected behavior.

Concurrent repository commits, including `d3079c8`, `2d6ea99`, `2fe0dc6`, and
`f2a53e2`, and `cb5cb3a`, arrived during implementation and include some in-progress Work
changes as well as UI/history changes. Those commits and unrelated work were
preserved. This implementation session did not commit, publish, deploy, or send
external messages. Disposable Git repositories used by tests/application checks
have fixture commits only.

Host: Linux amd64, Go 1.26.7 with `nodwarf5`. The manifest, CI, release workflow,
and explicit verification commands use Go 1.25.13. Git, Bubblewrap, and the
repository's pinned managed Chromium sidecar were available.

## Connected implementation evidence

| Stage | Implemented behavior | Discriminating verification |
| --- | --- | --- |
| 1: state and Code foundations | Separate capture/tree IDs; parent-first replay/source/artifacts; private versioned replay and compaction; adapter-state handling; frozen approved project configuration; streaming/visual interfaces | `internal/snapshot/product_depth_test.go`, `internal/workrun/product_depth_test.go`, `cmd/gator/product_depth_test.go`, `cmd/gator/work_configuration_test.go` |
| 2: service and control | CLI/TUI/jobs/Work RPC share typed service; streaming controls and exact-payload approvals; full job contracts and frozen sources; durable intent/retry references | Running service, TUI, and headless tests; direct/headless/scheduled policy-digest comparison; job intent/watermark crash test and explicit frozen-source edit test |
| 3: specialist lifecycle | Existing orchestrator owns durable child state, bounded concurrency/invocations, dependencies, cancellation, selected continuation, roles/routes, aggregate usage/budgets | Supervisor peer completion/failure/recovery/cancellation tests; shared-budget contention/refund/parent tests; Work lifecycle race checks |
| 4: Code composition | Private accepted baselines, real scope envelope, independent/overlapping patch integration, combined verification, frozen candidate review, explicit application | `internal/patch/candidate_test.go`, actual native Code follow-up test, strict process scope test, corpus code cases, CLI application walkthrough below |
| 5: research/data | Frozen selected local/attachment/web/connector evidence; source-only reader; claim quote checks; bounded CSV/XLSX extraction and exact reconciliation | Research brief plus evidence-preserving follow-up through Service; deterministic workbook cells/independent totals, discrepancies and memo checks; mixed Code/document and table follow-up corpus cases |
| 6: evaluation/telemetry | Work datasets/trials/graders/comparison, per-role ablation, independent rubric judge/calibration format, local bounded traces, optional OTLP and LangSmith experiment export | 24-case full-service corpus; fabricated-citation/wrong-output/fixture-integrity grader tests; separate tool-free rubric execution/budget/tamper test; local HTTP hierarchy/association/redaction/export-error tests |

The assembled configuration test changes live `.gator` configuration after capture,
then exercises captured profiles/scoped rules, an approved hook, MCP initialization
and tool resolution, LSP tool availability, and extension prompt resolution using
original trust identity. Actual LSP protocol behavior remains covered by the
existing LSP package tests; this assembly test does not start a language server.

The research service fixture combines local, explicitly selected web, and connected
reads with conflicting values. It checks an unselected-origin refusal, retained
citations/quote support, and a follow-up after live input mutation without a new
web fetch. Tampering with retained evidence fails bundle verification. This uses
local HTTP fixtures, not an external research account.

The native Code follow-up fixture accepts the first staged patch, starts the next
revision from that candidate, and verifies the cumulative change while live project
configuration has changed. The original source remains unchanged. Code is still
an internal specialist; legacy Code RPC/ACP/app-server surfaces remain compatible.

## Commands and results

Completed checks:

```sh
GOTOOLCHAIN=go1.25.13 make check
GOTOOLCHAIN=go1.25.13 go test -race ./internal/agent ./internal/orchestrator \
  ./internal/workrun ./internal/worksession ./internal/jobs ./internal/worktui \
  ./internal/telemetry ./cmd/gator
GOTOOLCHAIN=go1.25.13 go test -race ./internal/run ./internal/patch \
  ./internal/sandbox ./internal/mcp ./internal/rpc ./internal/acp ./internal/appserver
GOTOOLCHAIN=go1.25.13 GATOR_BROWSER_INTEGRATION=1 \
  go test -count=1 -run TestSidecarManagedBrowserIntegration ./internal/browser
```

`make check` passed tests for all 52 packages, vet, formatting and native build.
The two race groups above passed. The opt-in pinned Chromium integration passed
in 116.253 seconds. A host-toolchain full suite also passed earlier in the session.
The final focused rerun also passed:
`GOTOOLCHAIN=go1.25.13 go test -race ./internal/eval ./internal/workrun ./internal/jobs ./cmd/gator`.
The separate rubric execution regression passed after it was added. Documentation
links and workflow YAML were checked locally. `gofmt -l cmd internal` and
`git diff --check` produced no findings.

Go 1.25.13 with `CGO_ENABLED=0` also built `./cmd/gator` for `darwin/arm64`,
`darwin/amd64`, and `linux/arm64`; native Linux amd64 is covered by `make check`.
These are compilation checks, not runtime validation on those machines. CI and
release YAML parse locally; Linux CI/release installs Bubblewrap for Code corpus
execution. GitHub-hosted workflows were not dispatched in this session.

The actual CLI reviewed a retained `single-code-patch` bundle, including its
baseline, selected patch, changed path and passing verifier. Against a disposable
clean Git target, `apply --code-patch ... --check` left bytes unchanged; explicit
application changed `a.txt` from `base` to `alpha`. After committing that changed
target, another preflight rejected it with `target conflict: a.txt changed from
the reviewed baseline`. The original fixture still contained `base`.

## Measured scripted experiments

The final corpus identity, including fixture bytes, is:

```text
04837f0506a728d8fbd21b7fa23d053fe408bd4bf4c61e4d11ffa826c7b55cb3
```

There are 24 cases: 16 development and eight held out, with two trials each.
The measurements below supersede earlier in-progress corpus measurements.

| Experiment | Passed trials | Development / held out | Cases passing at least once / every trial | Model calls | Sum of trial latency |
| --- | --- | --- | --- | --- | --- |
| Delegation enabled | 48/48 | 32/32, 16/16 | 24/24, 24/24 | 266 | 2,412 ms |
| Delegation disabled | 32/48 | 24/32, 8/16 | 16/24, 16/24 | 156 | 295 ms |
| `source_researcher` disabled | 44/48 | 32/32, 12/16 | 22/24, 22/24 | 262 | 7,940 ms |

All three have 46 completed executions and two expected budget outcomes. Each
records 136 artifact checks, including four expected failed checks in boundary
cases, and two detected unsupported exact quotations. Enabled execution records
16 tool failures; disabled execution records 36; the role ablation records 16.
These counters include deliberate denials/errors. Retries and user interventions
are zero in this scripted corpus; separate running-operation tests cover control.

All scripted calls have unreported token usage and unknown cost. Zero reported
tokens does not mean zero consumption. Trials were run while other checks used
the machine, so these timings are execution observations, not a performance
comparison. Fixed scripts need specific tools; ablation failures measure those
execution dependencies. They do not establish that delegation improves live model
reasoning or that quotation matching establishes semantic entailment.

The disabled experiment's eight failing cases are `combined-verifier-failure`,
`invalid-dependency`, `mixed-code-document`, `overlapping-patches`,
`parallel-independent`, `reader-boundary`, `single-code-patch`, and
`unauthorized-specialist`. The role ablation fails `invalid-dependency` and
`reader-boundary`. Both commands correctly exit nonzero for failing trials.

Reports were retained under `/tmp/gator-depth-enabled`,
`/tmp/gator-depth-disabled`, and `/tmp/gator-depth-source-researcher-disabled`.
These local paths are not durable release artifacts. Each directory contains
`experiment.json`, `report.txt`, trial records and private execution evidence.
Both `eval work compare` operations and `eval work validate` passed. Reproduce
with the commands in [Work evaluation](WORK_EVALUATION.md), using fresh output
directories. The direct binary build used for these measurements records harness
`none`; the dataset digest and this repository-state record identify the run.

## Failed and unavailable verification

An initial broad test run hit a localhost EOF in
`TestOAuthLoginDiscoversRegistersAndBindsCredentialToResource`. Three isolated
reruns passed, followed by passing full suites. This initial failure is retained
here; the passing reruns do not explain its original cause.

Two intermediate suite/race runs overlapped edits to the corpus and grader
implementation, producing `unknown grader "unsupported_quote"` from a binary
compiled before its fixture was updated. The updated normal corpus passes. A
mistyped `--disable-role source_reader` initially produced invalid-role trials;
that report is excluded above. The CLI/service now rejects unknown/repeated
ablation roles before creating an experiment, covered by a focused regression.

The following external acceptance checks remain unverified:

- Live provider quality, provider comparisons, real connected accounts, and live
  synthesis judging were not run. No provider/model and live experiment budget
  were selected; OpenAI, Anthropic, Gemini and LangSmith API-key environment
  variables were absent. Stored credential contents were not inspected.
- Hosted LangSmith dataset/example/experiment/run/feedback round trip and an
  external OTLP/Langfuse collector were not exercised. Local HTTP mapping/error/
  redaction tests passed. They cannot verify a hosted account configuration.
- Real human rubric calibration requires independently supplied annotations.
  The supplied synthetic tests verify evidence validation and agreement arithmetic
  only; rubric results remain explicitly uncalibrated.
- macOS Seatbelt and ARM runtime checks require those hosts. Cross-compilation
  passed; no macOS runtime was available locally.
- The `work-live-eval` GitHub environment's reviewers and secrets are external
  repository settings. The workflow requires the explicit opt-in variable and
  bounded inputs, but those protection settings were not verified or changed.

Recovery retains results and marks unknown outcomes for inspection; it does not
provide instruction-level restart or exactly-once effects. Historical overwritten
version-1 source metadata cannot be reconstructed. PDF text extraction remains
explicitly unsupported by the native extractor; selected visual inputs use model
capability checks. No live-quality threshold or release-wide quality claim is
derived from these deterministic checks.
