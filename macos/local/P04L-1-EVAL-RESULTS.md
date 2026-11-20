# P04L-1 — Evaluation infrastructure, results and limits

## Verdict

**PASS for the L1 deterministic evaluation foundation and audit deliverables. NOT TESTED for live local-model quality/performance or local-first product acceptance.** No paid model, real Codex/Claude invocation, model download, unload, removal or default change occurred. Existing local lifecycle defects are documented for a separately gated L2; they were reproduced with disposable settings, fake Ollama handlers and a fake redirect transport.

The four architecture arms each passed **25 development + 25 held-out reference trials: 200/200 total**. These prove execution/grading/report contracts with known scripted answers, not that a model can produce those answers. Architecture C is a cloud-policy **reference**, not cloud inference. B emits delegation proposals and consumes simulated specialist outcomes; it does not execute a specialist.

## Reuse and focused extension

The new `work eval local-first` subcommand reuses `internal/eval` Work v1 datasets, `RunWorkExperiment`, `scriptedWorkService`, real Go Work admission/artifact validation, existing development/held-out splits, aggregate request budgets, wall deadlines and private report persistence. There is no second evaluator, agent framework, database, provider factory or credential lookup.

Changes are limited to a strict versioned profile, a four-field decision grader, tool-call-count grader, reference fixtures and audit tests. The existing same-model `CompareWork` identity policy is unchanged; it is not used to make invalid cross-provider quality comparisons. Existing live Work eval paths remain explicit separate paths. In particular, the legacy `work eval baseline` constructs a real provider directly without a `--live` flag or aggregate request cap; **do not use it as a safe default local-first/paid benchmark command**. No live path was invoked here.

Profile input is bounded to 64 KiB, dataset manifests to 8 MiB; unknown profile fields/future versions, trailing JSON, escaping dataset paths and digest mismatches fail. The captured profile hash and the same bytes parsed for each verified dataset bind metadata to input, rather than re-reading a changed file later. Existing source fixture hashing rejects symlinks/special files and bounds source files/total bytes. These protections constrain reviewed local fixtures; no new executable script or network origin can be supplied through this profile.

The finite decision schema is exactly `route`, `next`, `complete`, `answer`. Duplicate/unknown fields, null/wrong types, invalid enums, trailing documents and >16 KiB decisions fail. Diagnostics retain bounded scalar checks, not an arbitrary response or secret. Correct refusal, “requires a specialist,” and truthful incomplete outcomes pass when expected. Completion of the diagnostic artifact is distinct from `complete: false` for the user's requested task.

### Identity and fair comparisons

Profile: `internal/eval/testdata/local-first-v1/profile.json`, version 1, SHA-256 `49a8fb44e27a0acae29d7d9f64d5c4a619385f7273e6d763bb69f2377f8051ed`.

All four arms share normalized task/source/budget identity `65f5d62b6d8f21f918f893a0072e21f033f38a918e181ee6b651c1baca6fd053`. It includes IDs, split, source fixture content hash, objective, contract, scope, limits and follow-up objectives; architecture-specific scripts and expected grades differ. File digests for each authored manifest are in the profile. The existing canonical experiment's `dataset_sha256` hashes its normalized retained dataset; it need not equal the original manifest's byte hash. Both identities are preserved.

Source baseline at build: `978813b3b5403798c1c8efa6054e1f6a119ad2ca`, plus the hashed L1 working sources in [final scope](evidence/p04l-1/final-scope.json). HEAD advanced externally to `8052f0c2a50809bf04c9eb070a4c069fec785663`, capturing the code and earlier evidence. This session created no commit and did not reset external changes. The binary's harness field is the build's baseline commit, not a claim that L1 was already committed at that point; source hashes and binary SHA identify the actual tested artifact.

`generate.py` is a standard-library authoring tool. The runtime executes the reviewed JSON reference turns, not Python. There are 25 families × two separate source directories/splits × four arms. Held-out cases vary values, identities and some decisions, but are **published template siblings**, not a sealed, unseen model-generalization benchmark. L4 needs independently curated/calibrated held-out tasks before any quality claim.

### Coverage and what is actually exercised

| Families | Deterministic coverage | Limit |
|---|---|---|
| Short chat, summarization, planning, structured output | Exact finite response/artifact/schema checks | Semantic model ability not measured |
| Multi-source synthesis | Two actual frozen text-source reads and named-value comparison | No live retrieval |
| Tool selection, valid arguments, no tool | Real scoped read/write tools; invalid numeric path rejected; inspect-only greeting has **zero** tool calls | Narrow finite tools, not broad agent skill |
| Delegation need, Codex vs Claude, unnecessary delegation | Route/review/refusal checks, different policy arms and held-out specialist choice | No CLI executed; no routing-quality ranking |
| Specialist failure and integration | Simulated failed result / verified-versus-unsupported claim | No live specialist recovery or evidence ingestion acceptance |
| Incomplete work, user corrections, duplicate work | Truthful completion field; actual follow-up revision for correction; retained simulated receipt | No destructive replay attempted |
| Safe cancellation | Cancel decision fixture plus separate pre-cancelled experiment test proving zero model calls | Mid-generation live cancellation NOT TESTED |
| Context compaction and long reference | Public summary/missing-evidence decision, stable message reference | No real tokenizer/context truncation or compaction engine benchmark |
| Prompt injection, unauthorized tool/permission | Untrusted source instruction ignored in reference; actual denied `.env` read; no inferred approval | Live-model resistance NOT TESTED |
| Unavailable runtime, resource exhaustion, offline | Refusal/no-cloud-transfer policy decisions | Runtime/pressure failure is injected fact, not a live OOM experiment |
| Explicit cloud selection (additional 25th family) | Only C/D permit explicit reviewed cloud route | Execution still disabled for all arms |

Negative tests reject malformed decisions, extra secret-like fields, wrong routes, false completion, unnecessary delegation, unauthorized tool scripts, code/network/specialist execution, profile drift, unknown arms, `--live`, provider flags, oversize request/wall/attempt budgets and report overwrite. The aggregate-one-request test must fail and retain a budget outcome. An independent-process settings test, partial-delete test and socket-free redirect test preserve the architecture findings. These audit expectations document existing gaps and must be revised to safe-boundary assertions when L2 corrects them; they are not desired permanent unsafe behavior.

## Final deterministic results

| Arm | Development | Held-out | Scripted turns per split | Sum of trial duration, dev / held-out |
|---|---:|---:|---:|---:|
| A — local-only reference | 25/25 | 25/25 | 79 / 79 | 1320 / 1913 ms |
| B — local + specialist proposal reference | 25/25 | 25/25 | 79 / 79 | 1644 / 1627 ms |
| C — direct-cloud reference | 25/25 | 25/25 | 79 / 79 | 1686 / 1664 ms |
| D — explicit-cloud optional reference | 25/25 | 25/25 | 79 / 79 | 1610 / 1597 ms |

There are 24 finite decision probes and one no-tool greeting per split: **192/192 valid/correct routes, zero reference unnecessary delegations and zero reference incorrect-completion claims**. Across all runs, 632 scripted model turns were consumed. These elapsed times include local fixture Work/storage/grading and are **not inference latency, TTFT, throughput or performance comparisons between architectures**. Every arm has zero real provider calls, paid specialist calls, real downloads and implicit fallback requests by construction of the fixture service/tool allowlist. This does not establish whole-product offline enforcement.

[Exact commands, binary hash and private run directories](evidence/p04l-1/final-evaluation-runs.json), [canonical reports and sidecars](evidence/p04l-1/final-reports/), and the [compressed controlled raw runs](evidence/p04l-1/final-controlled-runs.tar.gz) preserve the evidence (3,536 files, 7,143,581 uncompressed bytes; 1,093,811 compressed bytes). Earlier `reports/` and logs are retained as historical evidence; use `final-reports/` for final metadata with host RAM/unknown measurements.

## Metrics contract

| Metric | Current retained evidence / future measurement |
|---|---|
| Case pass rate | Canonical experiment grades and split counts above |
| Correct / unnecessary delegation | Scalar finite decision checks; reference decisions only |
| Tool/schema validity | Strict decision schema, actual rejected numeric-path test, tool-event counts and existing `WorkMetrics.ToolFailures`; no invented live valid-call rate |
| Complete vs incomplete | Explicit expected decision `complete`; retain canonical diagnostic Work outcome separately |
| Unsupported/fabricated claims | Exact retained decision checks and expected false-completion flags; semantic claim accuracy unknown (`null`) |
| User corrections / recovery | Actual revision fixture plus simulated errors; live correction/recovery quality unknown |
| Unauthorized actions/permissions | No executable/network tools allowed; real denied source access and malformed-script rejection. Live approval compliance unknown |
| Network / fallback | Fixture adapter creates no network provider; four-arm reference restriction. Global production offline policy not established |
| Credential leakage | Distinct fixture keys injected into test environment; all controlled artifacts/output scanned in test; response diagnostics do not echo rejected secret fields |
| First-token / total task / tokens per second | Scripted trial total recorded; TTFT and model throughput `null` |
| Load time / repeated-task behavior | Live runtime absent; NOT TESTED |
| CPU / GPU / RSS / footprint / pressure / swap / KV cache | Baseline host VM/swap captured separately; per-model measurements `null`, not zero |
| Local invocation count | Zero inference calls; scripted model turns separately counted |
| Paid invocations / API usage | Zero real invocations; no token-price assumptions |
| Cost / cost per successful task | Unknown (`null`) for live tasks; no billable calls made, energy/storage not priced |

Metadata identifies scripted model, quantization not applicable, Go runtime version, OS/architecture/CPU count, 16 GiB RAM, planned context 4096, observed context unknown, concurrency 1 and effective request/wall budgets. It does not disguise the planned context as a measured allocation.

## Reproduce without inference

From the repository root:

```sh
go build -trimpath -o /tmp/gator-p04l-1-eval ./cmd/gator
/tmp/gator-p04l-1-eval work eval local-first validate internal/eval/testdata/local-first-v1/profile.json
/tmp/gator-p04l-1-eval work eval local-first run internal/eval/testdata/local-first-v1/profile.json --architecture A --split development --max-model-requests 256 --timeout-seconds 120 --attempts 1 --report-dir /tmp/gator-local-first-A-dev-NEW
```

Repeat explicitly for A/B/C/D and development/held-out with a **new** report directory each time. Development is the default; `all` is an explicitly requested diagnostic, not the default. Each run is capped at 256 scripted requests and 120 seconds, concurrency one; CLI may lower but not raise profile caps. No `--live` is accepted. Do not supply real keys. Regeneration (`python3 internal/eval/testdata/local-first-v1/generate.py`) is an explicit dataset-authoring operation, not needed to run the checked-in corpus.

## Live benchmark plan — not run

Ollama not detected at checked locations and standard loopback endpoint unavailable; exact installed models, quantization/digests and inference readiness UNKNOWN. Consequently all live local quality, model-load, first-token, throughput, RAM/GPU/KV, pressure and repeated-task results are **NOT TESTED**. No 8 GB hardware acceptance or macOS-version model compatibility result is claimed.

After a separately approved installation/download if needed, begin with one already installed reviewed candidate (`qwen2.5-coder:3b`, `qwen3:4b`, then `qwen3:8b` on 16 GB). Require exact digest/quantization/version inventory and local-only runtime policy first. Use an isolated Gator state/config and fixture workspace, no cloud roles/embeddings/CLI/tools, context 4096, output 32 tokens for initial smoke, one request, 60-second total wall bound. Record existing running models before/after; do not unload someone else's model or change global server settings. A smoke can load weights, so pressure admission still applies.

Example **future** local API smoke, only after verifying the named model is already installed and the server cannot forward to cloud: save a fixture request containing `{"model":"qwen3:4b","prompt":"Reply with LOCAL_OK only.","stream":false,"options":{"num_ctx":4096,"num_predict":32}}` to a private file, then:

```sh
curl --noproxy '*' --proto '=http' --max-time 60 --fail-with-body --request POST --header 'Content-Type: application/json' --data-binary @/tmp/gator-approved-local-smoke.json http://127.0.0.1:11434/api/generate --output /tmp/gator-approved-local-smoke-result.json
```

No redirects, remote endpoint, auto-pull, CLI worker, user prompt or default edit. This command was **not executed**. Ensure private output creation in the approved harness; do not let arbitrary API response/error bodies become general logs. Record API load/prompt/eval counts/durations where returned; streaming first-token timing needs a separate bounded streaming sampler. Upstream [API documentation](https://github.com/ollama/ollama/blob/main/docs/api.md) defines those fields. Inspect local residency/context via [ps](https://docs.ollama.com/api/ps); measure process RSS and physical footprint separately, with matched idle/loaded/active/settled baselines, VM/pressure/swap trends and available GPU tools. Do not infer GPU energy from CPU.

Only after adapter context enforcement and offline boundaries are accepted should L4 run actual Work model-quality cases. Budget proposal: 25 development cases, at most 100 primary model requests, 15 minutes, one local inference at a time, 4K context; stop on critical pressure, sustained swap growth or uncertainty. Calibrate this plan against the one-request smoke first. Use fresh explicit held-out authorization after development changes, preserve attempts/failures and report correct refusal as success where appropriate. A 16 GB end-to-end trial must include ordinary desktop + Gator/native headroom, growing histories/context and unload behavior; 8 GB remains a separate hardware/qualified constrained-environment assessment.

Paid A/B/C/D execution requires a **new explicit opt-in** identifying provider/model, exact assignment/data scope, maximum primary/delegate invocations, request count where enforceable, wall time, and a supportable cost estimate or clearly unknown cost. External CLI billing request counts cannot currently be inferred from one invocation. No paid benchmark command is authorized by this tranche, and possession of a key is not consent to spend it.

## Regression verification

Final source tests used environment filtering that removes `GATOR_LIVE_*`, `*_API_KEY`, `OPENAI_ACCESS_TOKEN` and `ANTHROPIC_AUTH_TOKEN` from the test subprocess environment. Tests supply their own inert keys/fake processes where required. No production credential file was opened for this audit.

```sh
go test ./internal/eval ./cmd/gator -run TestLocalFirst -count=1
go test -json -count=1 ./internal/eval ./internal/localmodel ./internal/modelcatalog ./internal/cliagent ./internal/config ./internal/model/... ./internal/agent ./internal/workrun ./cmd/gator
go test -race -json -count=1 ./internal/eval ./internal/localmodel ./internal/config ./internal/cliagent ./cmd/gator -run 'TestLocalFirst|TestSettingsUpdates|TestNativeProvider|TestCloud|TestNative'
go test -count=1 ./internal/auth
go test -race -count=1 -v ./internal/localmodel -run TestLocalFirstAuditLoopbackClientFollowsAnOffHostRedirect
git diff --check
```

- Targeted eval/command tests: PASS.
- Full listed Go regression: **15 packages PASS**, 1163 passing test/subtest events, 15 skips, zero failures; 51.10 seconds wall for the command.
- Focused race run: **5 packages PASS**, 57 passing test/subtest events, 3 skips, zero race failures; 20.24 seconds.
- Eight final deterministic eval processes: exit 0; 200/200 reference trials.
- Additional credential-store regression: `go test -count=1 ./internal/auth` PASS (0.615 seconds package time).
- Controlled-artifact fixture-key scan: zero hits; copied final reports are mode 0600; raw archive mode 0600. See [artifact checks](evidence/p04l-1/artifact-checks.json).
- Final test-only refinement: the redirect reproduction now preserves the actual constructed client’s redirect policy while substituting only a socket-free transport. The dedicated `-race` test passed afterward ([log](evidence/p04l-1/final-redirect-policy.log)); the full/race counts above preceded that test-only refinement. Production/evaluator sources and datasets did not change after those runs.
- Go build and whitespace check: PASS. No changed assertion was weakened.

[Raw final regression commands/exits](evidence/p04l-1/final-regression-commands.json) and [test summary with exact skips](evidence/p04l-1/test-summary.json) preserve package-level timing and reasons. Counts include subtests and must not be represented as 1163 distinct top-level functions. The independent-writer helper skips in the parent but runs in its spawned test process.

Skipped live suites: agent continuity, native CLI conversation, dynamic/internal agents, memory evaluation, TinyFish/project follow-up, desktop UI, project watch, public web and subagent acceptance. Additional skips: Linux-only systemd, opt-in local voice, native LaunchAgent relocation and the parent helper. Race skips are voice, LaunchAgent relocation and parent helper. These were not converted to passes.

Earlier reproduction attempts failed because the new test used unsupported Theme `dark`, then because its helper had not actually revoked the fake CLI. Those harness errors were corrected to valid Theme `mono` and explicit fake-CLI revocation; the final test verifies the child write before releasing the controlled HTTP barrier. Historical failed logs remain, and no production assertion was loosened.

Swift/strict Swift 6 and host TypeScript suites were **not repeated**: all native sources/tests/build/resources and host code are unchanged; no GUI checks are relevant to L1. The previous P04/A2/B2 results remain historical, not new results from this run. Local-model/config/provider/CLI/Work Go regressions were run because those are this increment's relevant boundaries. Source hashes scope-check native/mascot and security paths.

## Exact change inventory

| Files | Purpose |
|---|---|
| `cmd/gator/work_eval.go` | Dispatch finite fixture-only subcommand |
| `cmd/gator/work_eval_local_first.go` | Validate/run existing Work evaluator with matched four-arm identities and bounded effective budgets |
| `cmd/gator/work_eval_local_first_test.go` | Offline default, no credential leakage to controlled artifacts, rejection/overwrite and aggregate-budget tests |
| `cmd/gator/local_first_audit_test.go` | Independent-process lost update/revocation restoration, partial deletion publication, external-runtime non-adoption; parent/child helper |
| `internal/eval/work.go` | Reuse single loaded dataset byte snapshot; add optional finite decision diagnostic and two grader dispatch cases |
| `internal/eval/work_json_grader.go` | Register the two focused grader schemas |
| `internal/eval/local_first.go` | Strict hashed profile/allowlist, finite decision grader and private non-fabricated metadata |
| `internal/eval/local_first_test.go` | Schema/redaction, matched inputs/splits, bounds/authority rejection, private unknown metadata, cancellation tests |
| `internal/localmodel/local_first_audit_test.go` | Socket-free loopback redirect-boundary reproduction |
| `internal/eval/testdata/local-first-v1/` | Generator, versioned profile, four manifests, 50 source folders with 54 input/text files |
| `macos/local/P04L-1-*.md` | These three reports |
| `macos/local/evidence/p04l-1/` | Host/source baseline, exact test/fixture results, controlled raw artifacts, scope/permission/leak checks |

No local manager, provider/auth store, configuration transaction, CLI authority, Go supervisor, native Swift, mascot, app bundle or README production implementation was changed. No production app packaging was needed. P04L-2/3/4 are proposals only. The current product is not yet accepted as a 16 GB or 8 GB local-first coordinator; L1 supplies the reproducible fixtures, evidence and lifecycle requirements needed to make that decision honestly.
