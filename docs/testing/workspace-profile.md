# Large-workspace loading profile

On the [10,000-task, 2,000-event fixture](performance-baseline.md), a workspace
snapshot took 138 ms median and 212 ms p95 in the initial 20-run report. A
five-call `cProfile` run showed that Python model hydration dominated; SQLite
`execute` calls accounted for 0.029 s of 1.028 s cumulative workspace time.

| Five profiled workspace loads | Before | After empty-array fast path |
| --- | ---: | ---: |
| `list_tasks` cumulative | 0.583 s | 0.554 s |
| `list_events` cumulative | 0.442 s | 0.325 s |
| Event hydration (`_event`) cumulative | 0.399 s | 0.276 s |
| SQLite `execute` cumulative | 0.029 s | 0.031 s |

All 2,000 fixture events stored `[]` in each of their recurrence, reminder,
attendee, and attachment columns. Event hydration now returns an empty tuple for
that exact stored value and still decodes nonempty JSON. This removes four JSON
decodes per fixture event without changing the event model or database format.
Existing event round-trip tests cover nonempty values in those fields.

A second [20-run large-fixture report](baselines/fedora43-large-after-empty-json-2026-09-13.json)
measured workspace loading at 111 ms median and 131 ms p95, compared with
[138 ms and 212 ms initially](baselines/fedora43-large-2026-09-13.json). The
separate runs also changed unrelated CLI and outbox timings, so these are
machine-local observations rather than a controlled percentage claim or a
release budget.

To reproduce the profile, create a new synthetic database with
`uv run python tools/benchmark_python.py --database /tmp/hcb-profile.db`, then
run the following against that database:

```sh
uv run python - <<'PY'
import cProfile
import pstats
from pathlib import Path
from hcb.application import ApplicationService
from hcb.storage import Storage

with Storage(Path('/tmp/hcb-profile.db')) as storage:
    app = ApplicationService(storage)
    profiler = cProfile.Profile()
    profiler.enable()
    for _ in range(5):
        app.workspace('benchmark')
    profiler.disable()
    pstats.Stats(profiler).sort_stats('cumtime').print_stats(20)
PY
```
