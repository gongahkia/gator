# Paginated pull-sync profile

The [pull benchmark](../../tools/benchmark_pull.py) uses an in-process fake
Google gateway and a fresh SQLite database for every sample. It needs no Google
account, credential file, network access, or existing HCB data. It returns five
task lists and two calendars, with up to 100 tasks per page and 250 events per
page. It times the initial pull and a second pull with no remote changes:

```sh
uv run python tools/benchmark_pull.py --tasks 1000 --events 200 --runs 5
uv run python tools/benchmark_pull.py --tasks 10000 --events 2000 --runs 5
```

Google Tasks limits `tasks.list` to [100 tasks per page](https://developers.google.com/workspace/tasks/reference/rest/v1/tasks/list).
The fake returns Calendar's `nextSyncToken` on the final page, matching the
[incremental sync contract](https://developers.google.com/workspace/calendar/api/guides/sync).
The benchmark checks canonical row counts and the final Calendar tokens. Its
page-application timer surrounds SQLite transactions and includes Python model
conversion and search-index triggers; it is not a SQLite-only measurement.

These five-run samples were recorded on 13 September 2026 on Fedora 43,
Python 3.12.13, and an Intel Core i7-1355U. Times are milliseconds,
median / p95; with five runs, p95 is the slowest observed sample. These are
machine-local observations, not release budgets.

| Fixture and stage | Before | After |
| --- | ---: | ---: |
| 1,000 tasks: initial task-list + task pull | 38.7 / 40.6 | 40.4 / 44.5 |
| 1,000 tasks: unchanged incremental task pull | 8.57 / 8.64 | 0.44 / 0.51 |
| 10,000 tasks: initial task-list + task pull | 435.7 / 451.3 | 421.5 / 430.4 |
| 10,000 tasks: unchanged incremental task pull | 99.76 / 112.39 | 0.42 / 0.45 |
| 10,000 tasks + 2,000 events: initial page application | 612.2 / 657.7 | 592.9 / 603.9 |

The raw [medium before](baselines/fedora43-pull-medium-before-2026-09-13.json),
[medium after](baselines/fedora43-pull-medium-after-2026-09-13.json),
[large before](baselines/fedora43-pull-large-before-2026-09-13.json), and
[large after](baselines/fedora43-pull-large-after-2026-09-13.json) reports retain
every sample, page count, database size, and peak worker RSS. At 10,000 tasks,
the initial pull made 100 task-page calls and eight event-page calls; the
unchanged incremental pull made one task-page call per list and one event-page
call per calendar. The after-run median peak worker RSS was about 37 MiB.

An additional 10,000-task, 2,000-event `cProfile` run attributed 0.587 seconds
of its 1.045-second total to 24,448 SQLite `execute` calls. The task and event
upsert functions accounted for 0.364 and 0.183 seconds of cumulative time;
those figures overlap the SQLite total. Before the change, five unchanged
task-list upserts on the incremental pass fired the list-update search trigger,
which rebuilds child task search documents. The targeted change compares the
received list with the stored remote fields while ignoring only the newly
generated local timestamp. It skips the write when the list is unchanged and
still applies changed title, ETag, deletion, and other fields. The large
unchanged incremental task pull fell from about 100 ms to less than 0.5 ms
median in these samples. Initial-pull differences are within the variation of
these runs and are not attributed to the change.

The fake returns already-decoded Python objects and no network latency. It does
not measure OAuth, JSON decoding, Google quota behavior, retry delays, live
Calendar token expiry, or a windowed app. Task-page and Calendar-page restart
tests separately verify that committed pages survive a database reopen, and
the final Calendar cursor is stored only after the last page. Live Google
acceptance remains open and must use a disposable account for any writes.
