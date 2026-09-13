# Large-outbox delivery profile

The local [outbox benchmark](../../tools/benchmark_outbox.py) creates a fresh
synthetic account for each run, queues task updates, and times only
`SyncEngine.flush_outbox`. A fake gateway acknowledges each update, so these
figures measure local delivery bookkeeping without Google latency or
credentials. Reproduce the post-change samples with:

```sh
uv run python tools/benchmark_outbox.py --counts 250 1000 5000 --runs 20
```

On Fedora 43 with Python 3.12.13 and an Intel Core i7-1355U, the initial
profile of 250 writes attributed 0.320 s of a 0.391 s profiled flush to
`get_mutation`. It scanned and decoded pending rows for each individual ID;
the flush made 31,625 JSON decodes. After changing that lookup to
`SELECT * FROM outbox WHERE account_id=? AND id=?`, the same profile attributed
0.006 s of a 0.070 s flush to `get_mutation`. SQLite reports `SEARCH outbox
USING INTEGER PRIMARY KEY (rowid=?)` for the new query. The shared row decoder
keeps the returned mutation fields unchanged, and no schema migration is needed.

| Queued writes | Before: median or single run | After: median (20 runs) | After: p95 (20 runs) |
| ---: | ---: | ---: | ---: |
| 250 | 0.319 s (3 runs) | 0.198 s | 0.240 s |
| 1,000 | 3.872 s (3 runs) | 0.272 s | 0.945 s |
| 5,000 | 91.716 s (1 run) | 1.067 s | 1.893 s |

The raw [before](baselines/fedora43-outbox-before-2026-09-13.json) and
[after](baselines/fedora43-outbox-after-2026-09-13.json) reports contain every
sample. The single pre-change 5,000-write run has no tail estimate. The
250-write post-change samples varied between 0.076 s and 0.256 s; a separate
five-run set was faster still. These runs show a large scaling improvement,
especially at 1,000 and 5,000 writes, but their ratios are machine-local and
not release budgets.

The benchmark excludes fixture creation and enqueue time. It does not measure
Google API latency, quota handling, or the pull stages of a full sync. Existing
sync tests cover >100 writes, late-page cancellation and resume, conflicts,
retries, and interrupted-delivery recovery; the account-scoped row lookup has
a storage test for fields and ownership.
