# Developing the desktop application

The Electron app is Gator's supported user interface. Install Go 1.25.13 and
Node.js 22.12 or newer, then run from the repository root:

```sh
make run
```

The planned return to a terminal application is tracked in the
[terminal migration plan](TERMINAL-MIGRATION-PLAN.md). Electron remains the current
interface until the plan's feature parity gate passes.

`make run` installs the pinned desktop dependencies when missing or when the
package manifests change, builds the runtime and frontend, then opens Gator.
`make desktop` is an alias for the same command. `make build` builds the
private runtime and renderer; `make package` creates an unpacked local app under
`desktop/release/`; `make dist` creates installers there. These app targets
handle desktop dependencies automatically. Run `make help` to list the commands.

On Windows or without Make, use:

```sh
npm ci --prefix desktop
npm start --prefix desktop
```

No Gator executable is installed on PATH. Users of packaged releases need
neither Go nor Node.

## Application boundaries

- `desktop/src/`: React components and the display/action protocol.
- `desktop/electron/`: native host, narrow IPC, private runtime lifecycle,
  drafts and reviewed system handoffs.
- `internal/workapp/`: conversation application state, controls, application
  callbacks and display-safe projections. Its headless Bubble Tea message loop
  remains an implementation detail; there is no terminal renderer.
- `cmd/gator/ui_bridge.go`: JSON-line transport adapter for that model.
- Other `cmd/gator` handlers: canonical service adapters still used by the
  app, background workers, developer fixtures and evaluation. They are private
  implementation code, not a second user product.

Application projections must not imply authorization or success from model
prose. Keep canonical review, source, lineage and delivery checks in Go. Profile
formats and IDs are independent of Go package names.

## Developer and evaluation harness

Older examples used a public `gator` command. That command is retired. From a
source checkout, a developer can explicitly invoke an internal service:

```sh
GATOR_INTERNAL_COMMANDS=1 go run ./cmd/gator work eval validate internal/eval/testdata/work-v1/dataset.json
```

Shell examples in current service/evaluation documents use this explicit
developer opt-in. Short names such as `memory add` describe the service
operation; they are not end-user installation instructions. These examples use
POSIX shell syntax. In PowerShell, set `$env:GATOR_INTERNAL_COMMANDS = '1'` for
the development session, then run `go run ./cmd/gator ...`; remove the opt-in
afterward with `Remove-Item Env:GATOR_INTERNAL_COMMANDS`.

Use temporary state/config directories for fixtures, especially commands that
write, delete, restore, install services or perform an external action:

```sh
GATOR_STATE_DIR=/tmp/gator-dev-state GATOR_CONFIG_DIR=/tmp/gator-dev-config GATOR_INTERNAL_COMMANDS=1 go run ./cmd/gator note list
```

The opt-in is a routing convention, not a permission bypass or security boundary.
It never authorizes sending, spending, uploading or deleting anything. Live
evaluations require the explicit provider, model and budget options described
in [Work evaluation](WORK_EVALUATION.md).

## Verification

```sh
go test ./...
go vet ./...
npm run check --prefix desktop
npm test --prefix desktop
npm run pack --prefix desktop
npm run test:e2e --prefix desktop
```

Cross-compilation establishes build compatibility. Native installers, sign-in,
OS service startup and updates require actual platform execution. See
[Windows verification](WINDOWS.md) and [desktop scope](DESKTOP.md).

Historical audit/attempt records retain their original commands and observations
where marked. Do not read an old terminal benchmark as verification of a current
desktop release.
