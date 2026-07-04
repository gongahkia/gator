# paw benchmark results

Status: no benchmark run recorded yet.

## Terminal-Bench 2.0

`paw bench` updates the table between the markers below.

<!-- paw-results:start -->
| config | tasks | pass@1 | brain_in_tok/task (median) | drone_tok/task | wall_s/task |
| --- | ---: | ---: | ---: | ---: | ---: |
| raw | 0 | n/a | n/a | n/a | n/a |
| no-compress | 0 | n/a | n/a | n/a | n/a |
| full | 0 | n/a | n/a | n/a | n/a |
<!-- paw-results:end -->

## Reproduction

```sh
make build-linux
make bench-oracle
make bench-smoke
make bench-full
```

## vs SWE-Pruner (prior art)

This section separates measured paw results from prior-art numbers. Do not read the SWE-Pruner row as a paw result.

| system | benchmark | model | compressor | token reduction |
| --- | --- | --- | --- | --- |
| SWE-Pruner | SWE-bench Verified | GLM-4.6 / Claude Sonnet 4.5 | trained 0.6B neural skimmer | 23-54%, reported by SWE-Pruner authors |
| paw full vs raw | Terminal-Bench 2.0 | GLM-4.6 | stock drone model + deterministic span validation | TBD, measured from the raw/full rows above |

paw should only claim its own measured `full` vs `raw` reduction after both rows are populated by the same benchmark run setup.
