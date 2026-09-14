# Desktop bridge profile

The local desktop bridge is measured with a new deterministic SQLite fixture;
the benchmark does not open Qt, contact Google, or use account credentials.

```sh
uv run python tools/benchmark_desktop_bridge.py --runs 5
```

The 14 September 2026 Fedora 44 run used Linux `7.2.5-200.fc44.x86_64`, 10,000
tasks, 2,000 events, a 200-task page, and five repeat requests. The raw report
is retained in [the baseline JSON](baselines/fedora44-desktop-bridge-search-2026-09-14.json).

| Path | Median | Maximum | Response bytes |
| --- | ---: | ---: | ---: |
| Workspace summary | 2.15 ms | 7.41 ms | 898 |
| 200-task page | 9.26 ms | 11.09 ms | 85,085 |
| Indexed search (`release-marker`, limit 50) | 3.31 ms | 3.46 ms | 5,252 |
| Date-bounded event range | 13.10 ms | 15.55 ms | 119,701 |
| Full task workspace | 306.34 ms | 348.14 ms | 4,251,073 |

The search measurement covers the loopback bridge route through the Python
core's indexed local search and JSON envelope. It excludes the Qt HTTP client,
event loop, model projection, rendering, and network latency. The full
workspace number is retained as a contrast case and is not the desktop startup
path.
