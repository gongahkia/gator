# paw benchmark results

Status: no benchmark run recorded yet.

## Terminal-Bench 2.0

Rows below are placeholders until issues #2 and #3 record smoke/full runs. Every populated row
must include the reproducibility metadata described in [Reproduction](#reproduction).

<!-- paw-results:start -->
| run_id | commit | config | dataset | brain_model | drone_model | hardware | date | tasks | wall_time | tokens_brain_in | tokens_brain_out | tokens_drone | pass_rate | trace_bundle |
| --- | --- | --- | --- | --- | --- | --- | --- | ---: | ---: | ---: | ---: | ---: | ---: | --- |
| tb2-raw-tbd | TBD | [raw](results-configs/example.toml) | terminal-bench@2.0.x | TBD | n/a | TBD | TBD | 0 | n/a | n/a | n/a | n/a | n/a | TBD |
| tb2-no-compress-tbd | TBD | [no-compress](results-configs/example.toml) | terminal-bench@2.0.x | TBD | n/a | TBD | TBD | 0 | n/a | n/a | n/a | n/a | n/a | TBD |
| tb2-full-tbd | TBD | [full](results-configs/example.toml) | terminal-bench@2.0.x | TBD | TBD | TBD | TBD | 0 | n/a | n/a | n/a | n/a | n/a | TBD |
<!-- paw-results:end -->

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
- `pass_rate`: include a 95% CI when the sample size is large enough.
- `trace_bundle`: path or URL to a `.tar.gz` containing the run traces.

```sh
make build-linux
make bench-oracle
make bench-smoke
make bench-full
```

For any recorded row, commit the exact config under `docs/results-configs/`, preserve trace files
in the bundle named by `trace_bundle`, and keep raw Harbor output outside git unless it is small
enough to review.

## vs SWE-Pruner (prior art)

This section separates measured paw results from prior-art numbers. Do not read the SWE-Pruner row as a paw result.

| system | benchmark | model | compressor | token reduction |
| --- | --- | --- | --- | --- |
| SWE-Pruner | SWE-bench Verified | GLM-4.6 / Claude Sonnet 4.5 | trained 0.6B neural skimmer | 23-54%, reported by SWE-Pruner authors |
| paw full vs raw | Terminal-Bench 2.0 | GLM-4.6 | stock drone model + deterministic span validation | TBD, measured from the raw/full rows above |

paw should only claim its own measured `full` vs `raw` reduction after both rows are populated by the same benchmark run setup.

## SWE-bench Verified

Status: no SWE-bench run recorded yet.

| run_id | commit | config | dataset | brain_model | drone_model | hardware | date | tasks | wall_time | tokens_brain_in | tokens_brain_out | tokens_drone | pass_rate | trace_bundle |
| --- | --- | --- | --- | --- | --- | --- | --- | ---: | ---: | ---: | ---: | ---: | ---: | --- |
| swebench-raw-tbd | TBD | [raw](results-configs/example.toml) | TBD | TBD | n/a | TBD | TBD | 0 | n/a | n/a | n/a | n/a | n/a | TBD |
| swebench-full-tbd | TBD | [full](results-configs/example.toml) | TBD | TBD | TBD | TBD | TBD | 0 | n/a | n/a | n/a | n/a | n/a | TBD |
