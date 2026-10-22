# CLI architecture audit

**Historical record.** The standalone CLI and terminal renderer were retired on 22 October 2026. See [the desktop decision](DESKTOP-PRIMARY.md) and [current development instructions](DEVELOPMENT.md). Original observations below are retained.

Recorded 2026-10-18. The question is whether Gator should keep its Go Work
engine but move the user-facing CLI and terminal UI to Rust, Python, or another
language. The product target is an efficient, persistent personal agent in the
terminal, with local state, goals, knowledge, tools, and explicit approval.

## Decision

**Keep the CLI and terminal UI in Go for now.** A port does not have an
evidence-backed user benefit that outweighs the migration and integration
cost. This is a decision about Gator's current code and measurements, not a
general claim that Go is the best TUI language. A separate client can still be
built against a stronger versioned protocol if a measured need appears.

## What the repository actually contains

| Layer | Current boundary | Consequence for a port |
| --- | --- | --- |
| Entry point and commands | `cmd/gator/main.go` dispatches CLI, TUI, jobs, goals, memory, notes, skills, provider setup, and Work. `cmd/gator` has about 14,834 non-test Go lines. | The command package includes application assembly and product operations, not just argument parsing. Replacing it means exposing those operations over an API or duplicating them. |
| Interactive UI | `internal/worktui` has about 4,241 non-test Go lines. `internal/modelcatalog` has about 3,264. Both use Bubble Tea and Lip Gloss. | A new UI language would rewrite mature interaction flows, approval handling, model setup, and tests. |
| Work engine | `internal/workrun` has about 3,499 non-test Go lines. `configuredWorkService` assembles model, connectors, memory, learnings, goals, browser, and specialists in Go. | Work is separable in principle, but the UI currently calls Go closures and stores directly. |
| Machine interface | `gator work-rpc` is a versioned JSON-lines interface for one Work run, streaming events and approvals. | It is a useful future client seam, but it does not expose the full goal, memory, jobs, model setup, or TUI management surface. It currently does not accept `CodePolicy` or `steer_task`. |
| Packaging | Release builds target Linux and macOS, arm64 and amd64. The core CLI is one Go executable; the optional browser controller uses a Node sidecar. | A second-language UI adds another build, runtime or binary, and an IPC lifecycle to support. |

The counts above were generated with `rg --files` and `wc -l` over production
Go files. They are a migration-size estimate, not a complexity score. The
current path is UI callbacks in `cmd/gator/work_interactive.go` →
`configuredWorkService` → `workrun.Service` → local stores, providers, and
approved tools. Approval and cancellation events return through Go channels
to the Bubble Tea model. The Work RPC path is separate and narrower.

## Language comparison against this product

| Choice | Plausible gain | Current cost or missing evidence |
| --- | --- | --- |
| Keep Go and [Bubble Tea](https://github.com/charmbracelet/bubbletea) | Existing UI, Go-native concurrency and direct calls, one core executable, current tests and release targets. Bubble Tea supports full-window applications and event-driven updates. | The TUI and application assembly need cleaner boundaries; Go is not guaranteed to minimize memory or render latency. |
| Rust with [Ratatui](https://ratatui.rs/) | Capable low-level terminal UI and a plausible path to lower client memory if a complete prototype proves it. | Rewrite the TUI and command surface; introduce a second executable and bidirectional IPC, or a difficult FFI boundary; rebuild approval, cancellation, and state-management tests. No integrated memory or usability comparison exists. |
| Python with [Textual](https://textual.textualize.io/guide/app/) | Rich widgets and fast UI iteration; potentially useful for rapid prototypes. | Requires a Python runtime/package or bundling, plus the same IPC and product-surface rewrite. No integrated startup, RAM, distribution, or task-quality evidence shows a win for Gator's low-resource goal. |

Framework capability comes from the linked maintainers' documentation. The
cost and fit judgments are inferences from this repository's current
boundaries. They are not language-wide benchmarks.

## Performance evidence and limits

On a local `darwin/amd64` machine, each of two stripped builds was measured
three times per command. The first row is the original no-cgo audit build; the
second uses the release workflow's pinned Go toolchain and cgo setting.

| Build | Size | Median `--help` peak RSS / first output | Median idle supervisor peak RSS / first output |
| --- | ---: | ---: | ---: |
| Go 1.27, cgo off | 22 MiB | 13.4 MiB / 40 ms | 14.6 MiB / 152 ms |
| Go 1.25.13, cgo on | 20 MiB | 13.0 MiB / 22 ms | 14.0 MiB / 117 ms |

The first invocation after each build took 1,870 ms and 649 ms respectively
to first output; these runs were not a controlled cold-start comparison. The
supervisor used an empty state directory and was stopped after five seconds.
The measurement command and its limits are in [the runtime benchmark](RUNTIME_BENCHMARK.md).
These figures exclude model work, browser workers, and the full process tree.
The builds use different Go versions, so their numbers do not establish a cgo
effect. They do not compare Go with Rust, Python, Muse, dots, or Hermes, and
cannot justify a language migration or a low-RAM marketing claim.

## Findings and next architecture work

1. **Release build defect found and fixed.** The old release workflow built
   macOS on Ubuntu with `CGO_ENABLED=0`, while the macOS desktop runtime was
   selected solely by `darwin` and imported C. A release-style build failed
   because `desktop.DefaultRuntime` was undefined. The fallback now covers
   macOS without cgo and reports why desktop control is unavailable. Release
   builds use native [macOS arm64 and Intel runners](https://docs.github.com/en/actions/reference/runners/github-hosted-runners)
   with cgo enabled so published macOS binaries retain the desktop feature.
   The native Intel and arm64 release jobs still need their first CI run. Local
   Intel builds with and without cgo, and both Linux cross-builds, succeeded
   locally with Go 1.27. The Intel cgo build, Linux arm64 cross-build, and
   connector/desktop package tests also passed locally with the release jobs'
   Go 1.25.13. Native arm64 and the full release CI remain unverified.
2. **The Work RPC documentation overstated its v1 surface.** It previously
   listed a `code` request field that the implementation rejects. The protocol
   document now reflects that boundary. The protocol also lacks
   `steer_task` and most persistent-agent commands. Any new frontend should
   first close these API gaps, then test approval, cancellation, reconnect,
   and state migrations against the Go CLI with the same task corpus.
3. **CLI and app assembly are mixed.** `cmd/gator` owns terminal presentation,
   command parsing, and state/service assembly. The TUI config contains many
   callbacks directly bound to Go stores. Extracting a small application
   facade inside Go would reduce coupling and help either a future TUI
   rewrite or the current UI. It is lower risk than a language migration.
4. **Measure an actual user bottleneck before choosing a new runtime.** Add
   a representative interactive workload to the runtime benchmark: startup,
   scrolling a long conversation, model streaming, approval, goal resume, and
   an idle supervisor. Record process-tree RSS, render responsiveness, task
   success, and installation friction. A Rust or Python prototype should use
   the same Work and state policy through a versioned API and prove a material
   improvement on those measures before replacing the Go UI.

The next productive work is tightening the Go application boundary and
measuring real interactive tasks. No frontend code was ported because the
audit did not find a defensible advantage from doing so now.

## Connector scope checkpoint

The current Google connector supports bounded reads, selected approved writes,
Gmail send reconciliation, goal watches, account pins, and batch evidence. The
[feature map](MUSE_DOTS_FEATURE_MAP.md) still reports no matched live-model
comparison for cross-source usefulness and no real-account validation of Sheet
writes. This is enough infrastructure to test the intended daily briefs,
decks, and email drafts. Further connector families, generalized mutation
recovery, and standing action grants should wait for those task results. Fix
API-contract and data-loss bugs in supported paths, then spend the next
iteration on real account runs and user task outcomes. This is a scope choice,
not a claim that the existing connector work is wasted.
