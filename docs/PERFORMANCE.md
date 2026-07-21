# Performance budgets

`make benchmark` measures startup, context assembly, a fixture-backed long JSONL stream, coalesced timeline updates under a sustained stream, worktree creation planning, native diff rendering, and indexer/retrieval work. It writes the complete duration and memory sample report to `.gator-test/benchmark-report.json`; CI uploads one report for every OS/Neovim matrix entry.

Each workload has three samples. A limit includes a 100% documented platform-variance allowance, and CI fails only when all three samples exceed the effective duration or memory limit. This avoids failing on a single noisy sample while preserving an artifact for actionable regressions.

| Workload | Raw duration limit | Raw memory-delta limit | Effective duration limit | Effective memory-delta limit |
| --- | ---: | ---: | ---: | ---: |
| startup | 200 ms | 16 MiB | 400 ms | 32 MiB |
| context assembly | 200 ms | 16 MiB | 400 ms | 32 MiB |
| long stream | 800 ms | 32 MiB | 1,600 ms | 64 MiB |
| sustained UI loop | 250 ms | 16 MiB | 500 ms | 32 MiB |
| worktree planning | 250 ms | 16 MiB | 500 ms | 32 MiB |
| diff rendering | 800 ms | 32 MiB | 1,600 ms | 64 MiB |
| indexer/retrieval | 500 ms | 32 MiB | 1,000 ms | 64 MiB |

The checked-in source of these limits is `lua/gator/performance/budgets.lua`; update this document and the budgets together when a representative workload changes.
