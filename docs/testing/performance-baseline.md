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
