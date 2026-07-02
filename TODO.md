# paw — TODO

Format: **todo.txt**. One task per line. Grammar used here:
`x` prefix = done. `(A)`/`(B)`/`(C)` = priority. `+project` tags a milestone.
`@context` tags the area of the codebase. `key:value` = metadata (e.g. `ref:docs/DESIGN.md#3`,
`file:internal/...`). Tasks are ordered; do them top-to-bottom within a milestone.

An independent coding agent should be able to implement this repo from THIS file plus the
referenced docs (`docs/RELATED_WORK.md`, `docs/DESIGN.md`, `docs/SCHEMAS.md`, `docs/MODEL_APIS.md`,
`docs/PATCH_FORMAT.md`, `docs/BENCHMARKS.md`, `docs/TESTING.md`) with no further design decisions.
Read `docs/RELATED_WORK.md` first: it defines the honest positioning (paw productizes a proven
technique; it does not claim to invent it). Every task is a concrete file edit or CRUD action on
the repo. No task depends on marketing, publishing, or promotion.

Module path placeholder: `github.com/OWNER/paw` — replace `OWNER` (and rename `paw` if desired)
before starting; this is the only global find-replace required.

================================================================================
+M0-Scaffold  — repo skeleton, builds, CI. No agent logic yet.
================================================================================


================================================================================
+M1-Envelope  — the wire contract and schemas. Everything else depends on this.
================================================================================


================================================================================
+M2-Schema  — embedded JSON Schemas + validators (drone output + struct sync).
================================================================================


================================================================================
+M3-LLM  — provider-agnostic client (openai + ollama first; anthropic optional/later).
================================================================================


================================================================================
+M4-Config  — config file + env var loading (no secrets in code).
================================================================================


================================================================================
+M5-Budget  — token/turn accounting threaded through the loop.
================================================================================


================================================================================
+M6-Stage  — Stage interface + Pipeline runner (in-process + streaming). Depends M1.
================================================================================


================================================================================
+M7-Gather  — deterministic context collection. NO model. Depends M1.
================================================================================

(A) Implement optional ctags internal/gather/ctags.go: if universal-ctags present, produce a symbol-map RawUnit; degrade silently if absent @gather file:internal/gather/ctags.go
(A) Fold prior VerifyResult.FailureDigest into a verify_failure RawUnit when present on the incoming envelope @gather file:internal/gather/gather.go ref:docs/SCHEMAS.md#5
(A) Add testdata/repo/ fixture: a tiny Go (or generic) repo with a known bug and a known symbol to search for @gather file:testdata/repo/ ref:docs/TESTING.md#2
(A) Write internal/gather/gather_test.go: finds known symbol; respects depth/byte bounds; injecting a non-nil model client is unnecessary — assert gather has no llm.Client field at all (compile-time guarantee it cannot call a model) @gather file:internal/gather/gather_test.go ref:docs/TESTING.md#2

================================================================================
+M8-Compress  — DRONE model + deterministic validation. THE CORE. Depends M2,M3,M7.
================================================================================

(A) Create internal/compress/compress.go implementing Stage: chunk RawContext, build drone prompt, call drone client with schema.Raw("context_digest") as JSONSchema, receive candidate ContextDigest @compress file:internal/compress/compress.go ref:docs/DESIGN.md#4.2
(A) Create internal/compress/prompt.go: system+user prompt instructing the drone to ONLY score relevance, extract minimal verbatim spans, and NEVER invent paths/symbols/lines; paste the JSON schema text into the prompt to ground output @compress file:internal/compress/prompt.go ref:docs/MODEL_APIS.md#4
(A) Create internal/compress/validate.go: the deterministic checks in docs/SCHEMAS.md §2 — schema-valid, unit_id exists in RawContext, path matches unit, quote is verbatim substring (strings.Contains), line numbers within unit range; drop failing items, log to trace @compress file:internal/compress/validate.go ref:docs/SCHEMAS.md#2
(A) Create internal/compress/fallback.go: deterministic BM25-style ranking of RawUnits vs Instruction (implement simple TF/IDF over whitespace+identifier tokens; no external dep, or use a small pure-Go bm25 lib pinned in go.mod), emit top-N-by-token-budget as a ContextDigest; used when >50% items dropped or unparseable @compress file:internal/compress/fallback.go ref:docs/SCHEMAS.md#2
(A) Wire threshold logic in compress.go: if parse fails OR dropped/total > 0.5 → use fallback; record used_fallback in trace; always emit a valid ContextDigest @compress file:internal/compress/compress.go
(A) Add --disable-compress behavior hook: when set, compress.go skips the drone entirely and returns fallback(); this is the ablation control @compress file:internal/compress/compress.go ref:docs/BENCHMARKS.md#4
(A) Write internal/compress/compress_test.go with faketest drone: valid→passes; hallucinated path→dropped; non-verbatim quote→dropped; bad line range→dropped; >50% dropped→fallback; unparseable→fallback; assert the digest handed onward never contains bytes absent from RawContext @compress file:internal/compress/compress_test.go ref:docs/TESTING.md#2
(A) Write internal/compress/fallback_test.go: deterministic ranking stable across runs; respects token budget @compress file:internal/compress/fallback_test.go

================================================================================
+M9-Plan  — BRAIN planning. Depends M2,M3,M8.
================================================================================

(A) Create internal/plan/plan.go implementing Stage: build prompt from Instruction + validated Digest + prior VerifyResult; call brain with schema.Raw("plan"); validate with schema.ValidatePlan; enforce next_action-present-when-not-done and kind-specific required fields @plan file:internal/plan/plan.go ref:docs/DESIGN.md#4.3, docs/SCHEMAS.md#3
(A) Create internal/plan/prompt.go: instruct one concrete step at a time (avoid long-horizon incoherence); include only the digest, NEVER the RawContext @plan file:internal/plan/prompt.go ref:docs/DESIGN.md#4.3
(A) Update budget on plan call (AddBrain with usage) @plan file:internal/plan/plan.go ref:docs/SCHEMAS.md#6
(A) Write internal/plan/plan_test.go with faketest brain: done=true short-circuits; done=false w/o next_action → error; assert outbound brain request contains digest summary and NOT raw unit text (core-thesis regression guard) @plan file:internal/plan/plan_test.go ref:docs/TESTING.md#1

================================================================================
+M10-Patch  — deterministic unified-diff apply. NO model. Depends M1.
================================================================================

(A) Add go get github.com/bluekeyes/go-gitdiff/gitdiff @patch file:go.mod ref:docs/PATCH_FORMAT.md#2
(A) Create internal/patch/apply.go: parse unified diff (gitdiff.Parse), validate each target path is inside Cwd (reject abs and .. escapes), apply per file (gitdiff.Apply), atomic write via temp+rename, in-memory backup + rollback-all on any failure; return structured apply error naming file+first failing hunk @patch file:internal/patch/apply.go ref:docs/PATCH_FORMAT.md#2
(A) Create internal/patch/extract.go: strip surrounding prose/code-fences from a model response, extracting from first "--- " to last hunk line, before parsing @patch file:internal/patch/extract.go ref:docs/PATCH_FORMAT.md#1
(A) Add testdata/patches/ fixtures: create-new, delete-file, single-hunk, multi-hunk, multi-file, context-mismatch (must fail), path-escape (must fail) @patch file:testdata/patches/ ref:docs/PATCH_FORMAT.md#4
(A) Write internal/patch/apply_test.go covering all fixtures incl. rollback-on-partial-failure and path-escape rejection @patch file:internal/patch/apply_test.go ref:docs/PATCH_FORMAT.md#4, docs/TESTING.md#2

================================================================================
+M11-Edit  — BRAIN edit → unified diff → deterministic apply. Depends M9,M10.
================================================================================

(A) Create internal/edit/edit.go implementing Stage: build prompt from current step + digest; call brain (plain text or json with a unified_diff field); extract diff (internal/patch/extract); apply via internal/patch/apply; on apply error, re-prompt brain ONCE with failing file slice + error, then bounce to plan @edit file:internal/edit/edit.go ref:docs/DESIGN.md#4.4, docs/PATCH_FORMAT.md#3
(A) Create internal/edit/prompt.go: instruct the model to emit ONLY a unified diff obeying docs/PATCH_FORMAT.md §1 rules (relative a//b/ paths, no fences, no prose) @edit file:internal/edit/prompt.go ref:docs/PATCH_FORMAT.md#1
(A) Update budget on edit calls @edit file:internal/edit/edit.go
(A) Write internal/edit/edit_test.go with faketest brain returning a known-good diff for testdata/repo/ bug → asserts file changed on disk; and a bad diff → asserts single retry then structured failure @edit file:internal/edit/edit_test.go ref:docs/TESTING.md#2

================================================================================
+M12-Verify  — deterministic build/test/lint runner + failure compaction. NO model.
================================================================================

(A) Create internal/verify/verify.go implementing Stage: run the task verification command (from config/env PAW_VERIFY_CMD, or a sensible default like the repo's test runner), capture exit code + combined output @verify file:internal/verify/verify.go ref:docs/DESIGN.md#4.5
(A) Create internal/verify/digest.go: deterministic FailureDigest — keep last N lines + lines matching FAIL/Error/assert/panic/Traceback/expected/got, dedup, cap 4KB; set RawTailBytes @verify file:internal/verify/digest.go ref:docs/SCHEMAS.md#5
(A) Write internal/verify/verify_test.go: passing cmd → Passed true; failing cmd → Passed false + digest keeps markers + respects byte cap @verify file:internal/verify/verify_test.go ref:docs/TESTING.md#2

================================================================================
+M13-CLI  — subcommands wiring stages; pipeable text contract. Depends M6-M12.
================================================================================

(A) Create cmd/run.go: `paw run` — flags --instruction/--instruction-file, --max-turns, --raw-context, --disable-compress, --drone-model, --trace-file, --noninteractive (also via PAW_NONINTERACTIVE); builds config+clients, constructs Pipeline, calls RunLoop against Cwd; exit 0 on done/verify-pass @cli file:cmd/run.go ref:docs/DESIGN.md#5, docs/BENCHMARKS.md#2
(A) Create cmd/stage_common.go: helper to read an Envelope from stdin and write to stdout for the standalone stage subcommands @cli file:cmd/stage_common.go ref:docs/DESIGN.md#3
(A) Create cmd/gather.go: `paw gather --instruction ...` seeds an Envelope and runs the gather stage, emitting the Envelope on stdout @cli file:cmd/gather.go
(A) Create cmd/compress.go: `paw compress` reads Envelope stdin, runs compress stage, writes stdout (honors --disable-compress/--drone-model) @cli file:cmd/compress.go
(A) Create cmd/plan.go: `paw plan` reads Envelope stdin, runs plan stage, writes stdout @cli file:cmd/plan.go
(A) Create cmd/edit.go: `paw edit` reads Envelope stdin, runs edit stage (applies patch to Cwd), writes stdout @cli file:cmd/edit.go
(A) Create cmd/verify.go: `paw verify` reads Envelope stdin, runs verify stage, writes stdout @cli file:cmd/verify.go
(A) Add `paw run --explain`: print the composed pipeline as `gather | compress | plan | edit | verify` and exit (the shareable one-liner demo) @cli file:cmd/run.go ref:docs/DESIGN.md#3
(A) Write cmd golden tests: pipe a fixture Envelope through each subcommand with faketest server, compare stdout to testdata/golden/* @cli file:cmd/run_test.go ref:docs/TESTING.md#4
(A) Write internal/stage full integration test (real stages, fake LLM) per docs/TESTING.md §3: loop terminates, patch applied, verify passes, brain-input tokens < rawBytes/4 @stage file:internal/stage/integration_test.go ref:docs/TESTING.md#3

================================================================================
+M14-Harbor  — Terminal-Bench 2.0 integration via Harbor Python adapter. Depends M13.
================================================================================

(A) Create adapters/harbor/pyproject.toml: package paw-harbor, module paw_harbor, dep on harbor; python 3.12 @bench file:adapters/harbor/pyproject.toml ref:docs/BENCHMARKS.md#3
(A) Create adapters/harbor/src/paw_harbor/__init__.py exporting PawAgent @bench file:adapters/harbor/src/paw_harbor/__init__.py
(A) Create adapters/harbor/src/paw_harbor/agent.py: PawAgent(BaseInstalledAgent) with name/version/install/run exactly per docs/BENCHMARKS.md §2 — install() apt-installs ripgrep/git/ctags/ca-certificates and uploads bin/paw-linux-amd64 to /usr/local/bin/paw; run() writes instruction to file and execs `paw run` in /workspace with PAW_NONINTERACTIVE; MUST NOT create top-level tests/ dir @bench file:adapters/harbor/src/paw_harbor/agent.py ref:docs/BENCHMARKS.md#2
(A) Ensure `make build-linux` output path (bin/paw-linux-amd64) matches adapter upload path; add adapters/harbor/README.md with exact env vars + `harbor run --agent-import-path paw_harbor:PawAgent` commands @bench file:adapters/harbor/README.md ref:docs/BENCHMARKS.md#1
(A) Create cmd/bench.go: `paw bench` shells out to `harbor run` with configured flags; supports --config full|no-compress|raw mapping to run.go flags; parses Harbor jobs-dir results + paw NDJSON traces; prints the comparison table in docs/BENCHMARKS.md §4 @bench file:cmd/bench.go ref:docs/BENCHMARKS.md#4
(A) Add Makefile targets: bench-smoke (5-task subset, configs raw+full), bench-oracle (harbor run --agent oracle sanity), bench-full (89-task, all three configs) @bench file:Makefile ref:docs/BENCHMARKS.md#6
(A) Create docs/RESULTS.md template with the empty comparison table + reproduction command block + a dedicated "vs SWE-Pruner (prior art)" section per docs/BENCHMARKS.md §4a (measured paw full-vs-raw reduction beside SWE-Pruner's reported 23-54%, clearly labeling borrowed vs reproduced numbers); bench.go appends/updates rows here @bench file:docs/RESULTS.md ref:docs/BENCHMARKS.md#4,#4a
(A) Manual milestone check (document in adapters/harbor/README.md, not automated): (1) oracle passes locally, (2) PawAgent completes >=1 task with reward.txt==1, (3) bench-smoke raw vs full shows brain-token reduction at held pass-rate @bench ref:docs/BENCHMARKS.md#6

================================================================================
+M15-Docs  — README and top-level docs. Code-adjacent only, no marketing.
================================================================================

(A) Create README.md: what paw is using the HONEST positioning from docs/DESIGN.md §2 (productize a proven technique; NOT "we invented small-model compression"); install (`go install`/release binary + Ollama prereq for drone), quickstart (`paw run --instruction "..."`), the pipeline one-liner (`paw run --explain`), config/env table; include a short "Prior art & how paw differs" section linking docs/RELATED_WORK.md (name SWE-Pruner/Focus/TokenPilot/LLMLingua/The Token Company); link docs/*.md; NO promotional copy, NO invented-novelty claims, NO benchmark boasting beyond linking docs/RESULTS.md @docs file:README.md ref:docs/DESIGN.md#2, docs/RELATED_WORK.md
(A) In README.md, do NOT copy prior-art token-reduction numbers as paw's own; paw's reduction is whatever docs/RESULTS.md measures @docs file:README.md ref:docs/RELATED_WORK.md#3
(A) Add CONTRIBUTING.md: how to add a Stage (implement interface + subcommand + golden test), how to add an llm transport, test/lint commands @docs file:CONTRIBUTING.md ref:docs/TESTING.md
(A) Add LICENSE (Apache-2.0, matching the ecosystem norm for benchmarks/harnesses) @docs file:LICENSE
(A) Add docs/ARCHITECTURE.md as a short pointer file linking RELATED_WORK/DESIGN/SCHEMAS/MODEL_APIS/PATCH_FORMAT/BENCHMARKS/TESTING so newcomers have one entry point @docs file:docs/ARCHITECTURE.md

================================================================================
+M16-Anthropic  — OPTIONAL: anthropic-compatible transport. Do only after M14 green.
================================================================================

(B) Implement internal/llm/anthropic.go: POST {base_url}/v1/messages; x-api-key + anthropic-version headers; system/messages body; concat content[] text; usage.input_tokens/output_tokens; schema via single tool with input_schema + read tool_use.input @llm file:internal/llm/anthropic.go ref:docs/MODEL_APIS.md#3
(B) Extend factory.go + config to allow transport=anthropic for brain (e.g. DeepSeek anthropic base https://api.deepseek.com/anthropic) @llm file:internal/llm/factory.go ref:docs/MODEL_APIS.md#3
(B) Write internal/llm/anthropic_test.go against faketest server (add /v1/messages route to the fake) @llm file:internal/llm/anthropic_test.go ref:docs/TESTING.md#1

================================================================================
+M17-SWEbench  — OPTIONAL secondary benchmark. Do only after M14 full run recorded.
================================================================================

(C) Add SWE-bench Verified support: reuse Harbor SWE-bench adapter if available, else a paw-invoking agent script; document run commands in docs/BENCHMARKS.md §5 and add results rows to docs/RESULTS.md @bench ref:docs/BENCHMARKS.md#5
(C) Add Makefile target bench-swebench @bench file:Makefile ref:docs/BENCHMARKS.md#5

================================================================================
NOTES FOR THE IMPLEMENTING AGENT
================================================================================
- Do milestones in order. Within M7-M12 the stages are independent and can be built in parallel,
  but all depend on M1/M2/M3.
- Never let gather or verify import internal/llm (compile-time guarantee they call no model).
- Never send RawContext to the brain in plan/edit — only the validated ContextDigest. Tests in
  M9/M13 assert this; do not weaken them.
- Positioning honesty: paw does NOT claim to invent small-model context compression (SWE-Pruner,
  Focus, TokenPilot, LLMLingua did it first). paw's claims are: no-training + stock model +
  offline + verified spans + installable single binary + composable. Keep README/commits within
  those claims. Never present prior-art numbers as paw's own; measure paw's own (docs/RESULTS.md).
- Every new stage needs: the Stage impl, a cmd/ subcommand, unit tests, and a golden test.
- Re-verify provider endpoints and Harbor/dataset versions (docs/MODEL_APIS.md, docs/BENCHMARKS.md)
  before any real benchmark run; they change.
