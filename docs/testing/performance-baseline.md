# Shared-core performance baseline

The synthetic benchmark needs no Google account or credentials. It creates a
fresh SQLite fixture, then measures the current Python application and outbox
paths in a separate process. Run it from the repository root:

```sh
uv run python tools/benchmark_python.py --tasks 1000 --events 200 --runs 20 --outbox-writes 150 > medium.json
uv run python tools/benchmark_python.py --tasks 10000 --events 2000 --runs 20 --outbox-writes 250 > large.json
```

Pass `--database /path/to/new-fixture.db` to retain a synthetic database for
profiling; the path must not already exist.

Each report retains the earlier one-shot database-open, agenda, workspace, and
calendar-layout timings. It adds 20 observations each for a fresh Python CLI
`--help` invocation, local workspace search, full workspace materialization,
and delivery of queued task updates through `SyncEngine.flush_outbox`. The
outbox timer excludes enqueueing and uses an in-memory gateway that acknowledges
writes without Google requests. `p95` is the 19th sorted observation of 20;
the raw samples are in the JSON files.

These initial runs were recorded on 13 September 2026 with Fedora 43, Linux
7.1.12, Python 3.12.13, and an Intel Core i7-1355U (12 logical CPUs). Times are
milliseconds; each cell is median / p95. They are observations on this machine,
not release targets.

| Fixture | CLI help startup | Local search | Workspace load | Local outbox flush | Peak worker RSS |
| --- | ---: | ---: | ---: | ---: | ---: |
| [1,000 tasks, 200 events, 150 writes](baselines/fedora43-medium-2026-09-13.json) | 461 / 563 | 1.1 / 1.9 | 15 / 22 | 129 / 221 | 43.7 MiB |
| [10,000 tasks, 2,000 events, 250 writes](baselines/fedora43-large-2026-09-13.json) | 468 / 781 | 1.6 / 2.0 | 138 / 212 | 297 / 478 | 52.3 MiB |

Peak RSS is the measurement process's high-water mark after the one-shot and
repeated work. Fixture creation and the separate CLI startup processes are not
included in that memory figure. The startup timing measures CLI help imports,
not account loading or TUI first paint. Search and workspace timings reuse an
open database, so they are warm-path timings; the one-shot database-open timing
also benefits from the operating system's file cache. The benchmark does not
measure Google network latency, OAuth, real account data, or a desktop UI. Keep
those separate from this local baseline when setting user-facing budgets.

The [workspace loading profile](workspace-profile.md) records a measured
hydration bottleneck and the later comparison after a targeted change.
The [outbox delivery profile](outbox-profile.md) records queue-size scaling,
the indexed lookup change, and the before/after samples.
The [TUI startup profile](tui-startup-profile.md) measures fresh-process
headless readiness and the first task-bearing terminal frame.
The [paginated pull profile](pull-profile.md) measures synthetic initial and
unchanged incremental Tasks and Calendar pulls without Google credentials.
The [read-only live pull profile](live-pull-profile.md) records request latency
and page-size comparisons without retaining account contents in the repository.

## Fedora follow-up after conflict and sync hardening

On 13 September 2026, the same machine ran the 20-sample Python benchmark
again after the Qt history build finished. Values below are median / p95 ms;
the [medium](baselines/fedora43-phase2-medium-2026-09-13.json) and
[large](baselines/fedora43-phase2-large-2026-09-13.json) reports retain every
sample. They are not performance budgets.

| Fixture | CLI help | Search | Workspace | Outbox flush | Peak worker RSS |
| --- | ---: | ---: | ---: | ---: | ---: |
| 1,000 tasks / 200 events / 150 writes | 432 / 636 | 1.1 / 1.5 | 11.9 / 21.8 | 32.8 / 60.6 | 43.9 MiB |
| 10,000 tasks / 2,000 events / 250 writes | 409 / 420 | 1.7 / 2.0 | 122 / 169 | 59.3 / 70.4 | 52.9 MiB |

The outbox flush median fell from 129 to 33 ms at 150 writes and from 297 to
59 ms at 250 writes. CLI help, search, and workspace measurements stayed in
the same broad range; warm-cache and CPU-frequency
variation limit direct comparisons between runs.

Separate [synthetic pull](baselines/fedora43-phase2-pull-2026-09-13.json) and
[TUI startup](baselines/fedora43-phase2-tui-2026-09-13.json) reports used five
repeat runs. At 10,000 tasks and 2,000 events, the paginated fake-gateway
initial pull spent a median 589 ms in Tasks and 250 ms in Calendar reads and
application; the repeated empty incremental pull spent about 1 ms in each.
At that size, median first task-bearing terminal frame was 1,021 ms, with 77.3
MiB RSS at that frame. These runs exclude Google network latency and do not
predict macOS or desktop-app performance.

The final pull rerun includes local-parent normalization and schema 11's
partial parent-reference index. Before that index, the same empty incremental
Tasks pull took about 13 ms because it scanned every task in a list; the
five-run median returned to 1.1 ms with the index. The other phase-two reports
do not exercise this query and were not repeated after the index change.
