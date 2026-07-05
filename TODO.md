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

================================================================================
+M15-Docs  — README and top-level docs. Code-adjacent only, no marketing.
================================================================================

================================================================================
+M16-Anthropic  — OPTIONAL: anthropic-compatible transport. Do only after M14 green.
================================================================================

================================================================================
+M16.5-ModelAccess  — local-first defaults + subscription CLI brains. Do before M17.
================================================================================

================================================================================
+M16.6-ModelAccessHardening  — harden local/API/CLI model access before benchmarks.
================================================================================


================================================================================
+M16.7-AgentCLIExpansion  — add more subscription/local coding CLI brain transports.
================================================================================

(B) Add Qwen CLI parser tests and command construction tests for JSON, stream JSON, wrapped response, and text fallback @test @llm file:internal/llm
(C) Add Cursor CLI brain transport `cursor-cli` as experimental: headless read-only/ask mode, model passthrough, JSON output, schema-in-prompt, no forced edits @llm file:internal/llm
(C) Add Cursor CLI parser tests and command construction tests; document schema limitation and experimental status @test @docs file:internal/llm file:MODEL_APIS.md
(B) Extend factory/config docs so every CLI brain transport can select provider-specific models via `PAW_BRAIN_MODEL` and, where applicable, provider via `PAW_BRAIN_PROVIDER` @config @docs file:internal/config file:internal/llm file:README.md
(B) Add `paw doctor models` coverage for Aider/Goose/Qwen/Cursor binary/version/model capability checks @cli @llm file:cmd file:internal/llm
(B) Add model listing coverage for Aider/Goose/Qwen/Cursor where supported; unsupported listing must return explicit diagnostic, not silent empty output @cli @llm file:cmd file:internal/llm
(B) Update README/MODEL_APIS with comparison table: local/no-key, subscription CLI, API-key required, model listing support, schema enforcement strength @docs file:README.md file:MODEL_APIS.md

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
