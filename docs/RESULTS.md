# paw benchmark results

Status: Terminal-Bench 2.0 smoke run recorded for issue #2.

## Terminal-Bench 2.0

Rows below include the issue #2 smoke run. Partial or interrupted jobs are not benchmark results.
Every populated row must include the reproducibility metadata described in [Reproduction](#reproduction).

<!-- paw-results:start -->
| run_id | commit | config | dataset | brain_model | drone_model | hardware | date | tasks | wall_time | tokens_brain_in | tokens_brain_out | tokens_drone | pass_rate | trace_bundle |
| --- | --- | --- | --- | --- | --- | --- | --- | ---: | ---: | ---: | ---: | ---: | ---: | --- |
| paw-smoke-raw | d91f8d0fe23a3f80527ad670bcd1208b266a0f87 | [raw](results-configs/paw-smoke-raw.toml) | terminal-bench@2.0 | openai/qwen2.5-coder:1.5b | n/a | macOS arm64 Docker Desktop local Ollama | 2026-07-07 | 5 | 68.0 | 756 | 242 | 0 | 0.00 | .paw/bench-jobs/paw-smoke-raw |
| paw-smoke-full | d91f8d0fe23a3f80527ad670bcd1208b266a0f87 | [full](results-configs/paw-smoke-full.toml) | terminal-bench@2.0 | openai/qwen2.5-coder:1.5b | openai/qwen2.5-coder:1.5b | macOS arm64 Docker Desktop local Ollama | 2026-07-07 | 5 | 93.3 | 562 | 132 | 0 | 0.00 | .paw/bench-jobs/paw-smoke-full |
<!-- paw-results:end -->

Smoke notes, 2026-07-07: Harbor 0.17.1, Ollama 0.31.1, `terminal-bench@2.0`,
`n_tasks=5`, `n_concurrent=1`, `PAW_CALL_TIMEOUT=120s`, brain/drone model
`qwen2.5-coder:1.5b`, macOS arm64 Docker Desktop local Ollama. Raw and full used the same subset:
`break-filter-js-from-html`, `gpt2-codegolf`, `llm-inference-batching-scheduler`,
`reshard-c4-data`, `write-compressor`. Raw completed with 2 agent exceptions; full
completed with 3 agent exceptions. Token and wall-time columns are medians per task
from trace artifacts.

Invalid diagnostic attempts, not benchmark results:

| run_id | reason |
| --- | --- |
| paw-full-no-compress | Incomplete Terminal-Bench job: 86 task results from an attempted full run. |
| paw-full-raw | Incomplete Terminal-Bench job: 88 task results from an attempted full run. |

## Reproduction

Required columns:

- `run_id`: stable row identifier, also used in result config and trace bundle paths.
- `commit`: full 40-character git SHA.
- `config`: checked-in config file under `docs/results-configs/<run-id>.toml`.
- `dataset`: dataset id and revision, for example `terminal-bench@2.0.3`.
- `brain_model` / `drone_model`: exact provider model identifiers; use `n/a` when a config has no
  drone.
- `hardware`: machine or runner description, for example `Mac Studio M4 Max 128GB`.
- `date`: ISO 8601 date or timestamp.
- `wall_time`: total wall time for the run.
- `tokens_brain_in` / `tokens_brain_out` / `tokens_drone`: total tokens across completed tasks.
- Token source (`provider`, `estimate`, or `mixed`) is recorded in the generated result config.
- `pass_rate`: include a 95% CI when the sample size is large enough.
- `trace_bundle`: path or URL to a `.tar.gz` containing the run traces.
  Publish compact trace bundles only; full traces can contain raw private source text.

```sh
make build-linux
make bench-oracle
make bench-smoke
# only after a credible raw/full smoke pass with the intended benchmark model:
# make bench-full
```

For any recorded row, commit the exact config under `docs/results-configs/`, preserve trace files
in the bundle named by `trace_bundle`, and keep raw Harbor output outside git unless it is small
enough to review.

## vs SWE-Pruner (prior art)

This section separates measured paw results from prior-art numbers. Do not read the SWE-Pruner row as a paw result.

| system | benchmark | model | compressor | token reduction |
| --- | --- | --- | --- | --- |
| SWE-Pruner | SWE-bench Verified | GLM-4.6 / Claude Sonnet 4.5 | trained 0.6B neural skimmer | 23-54%, reported by SWE-Pruner authors |
| paw full vs raw | Terminal-Bench 2.0 | GLM-4.6 | stock drone model + deterministic span validation | TBD, measured only from completed matched raw/full rows |

paw should only claim its own measured `full` vs `raw` reduction after both rows are populated by the same benchmark run setup.

## SWE-bench Verified

Status: no SWE-bench run recorded yet.

| run_id | commit | config | dataset | brain_model | drone_model | hardware | date | tasks | wall_time | tokens_brain_in | tokens_brain_out | tokens_drone | pass_rate | trace_bundle |
| --- | --- | --- | --- | --- | --- | --- | --- | ---: | ---: | ---: | ---: | ---: | ---: | --- |
| swebench-raw-tbd | TBD | [raw](results-configs/example.toml) | TBD | TBD | n/a | TBD | TBD | 0 | n/a | n/a | n/a | n/a | n/a | TBD |
| swebench-full-tbd | TBD | [full](results-configs/example.toml) | TBD | TBD | TBD | TBD | TBD | 0 | n/a | n/a | n/a | n/a | n/a | TBD |
