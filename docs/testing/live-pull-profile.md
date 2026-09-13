# Read-only live Google pull profile

On 13 September 2026, the [live pull benchmark](../../tools/benchmark_live_pull.py)
used the connected personal account for one initial and one near-immediate
incremental pull per run. It called only the Tasks and Calendar list endpoints.
Credentials were read from the owner-only local file and keyring; the normal HCB
database was not opened. Each run wrote the returned rows to a private temporary
SQLite database and deleted it on exit. No Google write or Drive account-data
call was made. The exact aggregate counts and per-request timing arrays are in
owner-only reports under `~/.local/state/hcb/live-pull-*.json`, outside this
repository.

The account had approximately 900 tasks and 2,100 events, spread across multiple
lists and calendars. The row counts were the same in all five runs. Seconds below
are wall time for the initial Tasks and Calendar pull stages, excluding OAuth
credential loading and client construction. The original and task-only settings
were measured once each; the final settings were measured three times.

| Page settings | Initial task requests / seconds | Initial event requests / seconds | Total initial seconds |
| --- | ---: | ---: | ---: |
| Tasks default 20, events default 250 | 53 / 19.27 | 11 / 6.41 | 25.68 |
| Tasks 100, events default 250 | 19 / 7.37 | 11 / 6.53 | 13.90 |
| Tasks 100, events 1,000 | 19 / 7.34 median | 5 / 4.82 median | 12.08 median |

The three final total samples were 11.83, 12.08, and 12.16 seconds. These
figures include SQLite application; recorded transaction time was below one
second per initial run. The task-page change removed 34 requests and the
event-page change removed six. Google documents a [20-task default and
100-task maximum](https://developers.google.com/workspace/tasks/reference/rest/v1/tasks/list)
and a [250-event default and 2,500-event maximum](https://developers.google.com/workspace/calendar/api/v3/reference/events/list).
The chosen event page size is 1,000, below that maximum. Larger responses can
have longer individual latency; this is a single-network observation, not a
release budget or a guaranteed speedup.

The final near-immediate incremental pulls took 4.56, 4.73, and 4.79 seconds.
They still needed 16 serial read requests: one task-list request, one for each
task list, one calendar-list request, and one for each selected calendar. One
run returned two recently updated tasks in the overlap window, so the three
incremental results are not a controlled empty-delta comparison. Page size
cannot remove those one-per-list requests. Whether bounded parallel fetching
is worthwhile remains open until a user-facing sync target is set.

During review, the incremental Calendar paginator was corrected to send the
same sync token on every page. This follows
Google's [incremental sync pagination contract](https://developers.google.com/workspace/calendar/api/guides/sync).
The repository tests use a fake gateway for paginated responses; this live
account did not produce a multi-page incremental Calendar delta, so that path
is not live-accepted here.

This profile does not cover OAuth startup, Google retries or quota errors,
app rendering, macOS, or write-path acceptance. It used a real non-disposable
account only for read requests. The separate
[full live acceptance procedure](live-google-tui-smoke.md) remains open.
