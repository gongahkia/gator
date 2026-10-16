# Terminal runtime benchmark

Gator's low RAM target needs measurements under the same machine, model,
provider, input, and idle conditions as any comparison agent. A small tool at
`scripts/measure_cli` records one direct process invocation as JSON. It reports
wall time, first stdout/stderr byte, and peak resident memory from the operating
system's per-process usage record. It discards command output and never records
API keys or task contents. Run each command several times and compare medians;
do not include `go run` or a first-time model download in the measured command.

```sh
go build -o /tmp/gator-benchmark ./cmd/gator
go run ./scripts/measure_cli --label gator-help -- /tmp/gator-benchmark --help

# Use an empty state directory for an idle supervisor measurement. The tool
# sends SIGTERM after five seconds; the supervisor must not run real jobs here.
GATOR_STATE_DIR=/tmp/gator-benchmark-state \
  go run ./scripts/measure_cli --label gator-supervisor-idle --idle 5s -- \
  /tmp/gator-benchmark job supervisor --notify=false
```

The tool supports macOS and Linux. Peak RSS is for the directly measured
process. An agent that spawns workers or a browser needs a process-tree or
whole-system measurement in addition to this number. First output is a startup
indicator, not model first-token latency. A realistic comparison also needs a
shared provider/model and task, recorded model requests, task success, cost,
and the full process tree's peak memory. No claim that Gator uses less RAM than
Muse, dots, or another terminal agent follows from a Gator-only sample.

## Initial local sample

On 2026-10-16, a built Gator binary was measured three times per command on
`darwin/amd64` with the tool above. The median `gator --help` run reached
13.1 MiB peak RSS and emitted its first byte in 24 ms. Its first run took
854 ms to first output; the later two took 24 and 23 ms. A supervisor with an
empty state directory, stopped after five seconds, reached 14.3 MiB median
peak RSS and emitted startup output in 112 ms median. These are startup and
idle figures for Gator alone. No model task or competitor ran in this sample.
