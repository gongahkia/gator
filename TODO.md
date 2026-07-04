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


================================================================================
+M8-Compress  — DRONE model + deterministic validation. THE CORE. Depends M2,M3,M7.
================================================================================


================================================================================
+M9-Plan  — BRAIN planning. Depends M2,M3,M8.
================================================================================


================================================================================
+M10-Patch  — deterministic unified-diff apply. NO model. Depends M1.
================================================================================


================================================================================
+M11-Edit  — BRAIN edit → unified diff → deterministic apply. Depends M9,M10.
================================================================================


================================================================================
+M12-Verify  — deterministic build/test/lint runner + failure compaction. NO model.
================================================================================


================================================================================
+M13-CLI  — subcommands wiring stages; pipeable text contract. Depends M6-M12.
================================================================================

================================================================================
+M14-Harbor  — Terminal-Bench 2.0 integration via Harbor Python adapter. Depends M13.
================================================================================

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
