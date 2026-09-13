# Synthetic TUI startup profile

The [startup benchmark](../../tools/benchmark_tui_startup.py) creates isolated
SQLite fixtures with 1,000 tasks/200 events and 10,000 tasks/2,000 events. Each
sample starts a fresh process. One runs Textual headlessly through `run_test`
and waits for its pending messages; another runs `hcb` in a 120×40 pseudo
terminal and stops the clock when the first task-bearing frame is emitted.
Fixture creation is outside the timer. The benchmark sets an unused credential
path and performs no Google requests.

```sh
uv run python tools/benchmark_tui_startup.py --counts 1000 10000 --runs 20
```

These observations were recorded on Fedora 43 with Python 3.12.13 and an
Intel Core i7-1355U. Times are median / p95 over 20 runs, in seconds; RSS
values are medians at the first task frame.

| Fixture | Headless ready before | Headless ready after | First task frame before | First task frame after | Terminal RSS before / after |
| --- | ---: | ---: | ---: | ---: | ---: |
| 1,000 tasks | 0.699 / 1.335 | 0.498 / 0.766 | 0.869 / 1.896 | 0.601 / 1.064 | 59.9 / 59.9 MiB |
| 10,000 tasks | 1.138 / 2.209 | 0.769 / 1.632 | 1.329 / 2.643 | 0.844 / 1.568 | 77.0 / 77.2 MiB |

The raw [before](baselines/fedora43-tui-startup-before-2026-09-13.json) and
[after](baselines/fedora43-tui-startup-after-2026-09-13.json) reports include
all samples, fresh-process import and construction times, headless mount time,
and memory samples. Import time also fell between runs (0.354 s to 0.258 s
median at 1,000 tasks), which the row change cannot explain. Machine load
varied enough that the overall startup differences cannot be attributed solely
to the code change. These are local observations, not release budgets.

A 10,000-task `cProfile` run before the change attributed 0.798 s cumulative
to `refresh_workspace`, including 0.347 s in workspace loading and 0.400 s in
surface rendering. Its 10,000 task-row calls took 0.344 s cumulative under
profiling. Textual stylesheet application also took 0.909 s cumulative; these
overlapping profiled totals include profiler overhead and are not wall-clock
components to add together. In an unprofiled row-only check, building 10,000
ordinary task rows took 0.120 s median before the change and 0.064 s after.
The measured work was copying and scanning Rich text for URLs even when the
newly built row had no URL. The task-row builder now calls `linkify_urls` only
when its visible text contains an `http://` or `https://` candidate. Linked
titles and Notes previews retain clickable links in the TUI test.

The pseudo terminal records when HCB emits a task frame, not when a physical
terminal emulator paints pixels. Headless timing includes `pilot.pause()` and
does not draw terminal output. No macOS or real-account startup measurement has
been made.
