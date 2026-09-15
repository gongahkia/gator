# Desktop bridge profile

The local desktop bridge is measured with a new deterministic SQLite fixture;
the benchmark does not open Qt, contact Google, or use account credentials.

```sh
uv run python tools/benchmark_desktop_bridge.py --runs 5
```

The 15 September 2026 Fedora 44 run used Linux `7.2.5-200.fc44.x86_64`, 10,000
tasks, 2,000 events, a 200-task page, and five repeat requests. It includes the
durable local sibling-order index used by paged desktop clients. The raw report
is retained in [the baseline JSON](baselines/fedora44-desktop-bridge-local-order-2026-09-15.json).

| Path | Median | Maximum | Response bytes |
| --- | ---: | ---: | ---: |
| Workspace summary | 2.00 ms | 5.83 ms | 914 |
| 200-task page | 8.01 ms | 9.05 ms | 85,085 |
| First-sibling reorder | 16.40 ms | 21.27 ms | 457 |
| Indexed search (`release-marker`, limit 50) | 2.71 ms | 2.85 ms | 5,251 |
| Date-bounded event range | 10.00 ms | 12.07 ms | 119,717 |
| Full task workspace | 251.25 ms | 252.27 ms | 4,251,084 |

The search measurement covers the loopback bridge route through the Python
core's indexed local search and JSON envelope. It excludes the Qt HTTP client,
event loop, model projection, rendering, and network latency. The full
workspace number is retained as a contrast case and is not the desktop startup
path. The reorder measurement sends five first-sibling move requests for
different tasks through the bridge, including the Python transaction, outbox,
idempotency receipt, and sparse local-rank update.
