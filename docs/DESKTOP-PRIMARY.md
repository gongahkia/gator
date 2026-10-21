# Desktop as Gator's primary interface

Decision: 22 October 2026. Supersedes the earlier dual-interface rollout.

## Decision

The Electron application is the primary and currently supported way for users
to interact with Gator. Retire the standalone Gator CLI and React terminal
frontend for now. Keep the Go application and saved data behind the desktop.
This is one reversible repository change, committed atomically.

## Implementation

- Delete the React terminal renderer, its package/build dependencies, and the
  embedded terminal bundle. Move the display protocol into `desktop/src/`.
- Remove terminal-only status-line controls and command discovery. Legacy
  settings remain readable in old profiles and backups; no data migration or
  deletion is needed.
- Bundle the Go application as `runtime/gator-runtime`. Bare execution and
  former public commands explain that the desktop app is the user entry point.
  Rejected commands do not initialize or maintain a saved profile.
- Retain the private UI transport, app-owned background supervisor, and existing
  canonical command handlers used for app workflows, tests and maintenance.
  `cmd/gator` remains the internal implementation location, not a CLI product.
- Use `make desktop` to launch, `make build` to build, `make package` for a
  local app bundle, and `make dist` for installers. `make runtime` is an
  internal development build and does not install anything on PATH.
- Replace CLI release archives with desktop DMG/ZIP and AppImage/DEB artifacts.
  Update CI to check the frontend/host and packaged macOS workflows. The old
  installer URL returns migration guidance and never downloads a CLI.
- Retire the CLI self-update entry point. Desktop update/signing distribution
  remains an application-release concern; no automatic updater is added here.

## Compatibility and boundaries

Existing conversations, goals, knowledge, credentials, approvals, usage limits,
backups, and state/config overrides retain their formats and locations. The
agent loop and authority model are unchanged. Background service definitions
continue to invoke the supervisor role. Enabling/reinstalling a service from
Recovery binds it to the new bundled runtime path.

Some reviewed system sign-in, installation or live-action review steps still
use an app-owned system Terminal handoff and return their status to Gator.
These do not launch the removed Gator terminal UI. Converting every such step
into a native dialog is separate work; disabling them would remove existing
capabilities. Handoff arguments and approval semantics remain owned by Go.

The explicit `GATOR_INTERNAL_COMMANDS=1` opt-in retains a developer fixture and
maintenance harness around existing services. It is not a supported user CLI,
security boundary, or permission grant. The desktop transport enables it for
its own subprocess workflows; renderer requests still pass the existing narrow
IPC checks and canonical validations/reviews.

This change does not uninstall third-party/system binaries or delete published
historical release assets. Repository-generated legacy executables are removed
from the development workspace. No Gator command was found on the user's PATH
when this change began.

## Verification

| Check | Result |
| --- | --- |
| `go test ./...` | Pass across the repository |
| `go vet ./...` | Pass |
| `npm run check --prefix desktop` | Pass |
| `npm test --prefix desktop` | 10/10 host tests pass |
| `npm run pack --prefix desktop` | Local macOS application package builds |
| Packaged `npm run test:e2e` | 15/15 pass, including startup with an empty tool PATH and absence of terminal-only settings |
| Runtime executable smoke checks | Bare/former CLI commands reject with desktop guidance; help/version work; no profile directory is created |
| Packaged runtime | SHA-256 matches the tested source build; resource directory contains only `gator-runtime` |
| Legacy installer | Prints retirement/migration guidance with no external tools on PATH; exits without installing |
| Linux ARM64 | `GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build` passes for the private runtime |
| Build/release wiring | Make targets inspected with `make -n`; all three changed workflow YAML files parse |
| Changed interface screenshots | Empty command search for retired settings and first-run desktop without Node/CLI inspected |
| `git diff --check` | Pass |

Tests use temporary profiles and local fake providers, with the app launched
without the developer command opt-in. Fixture creation alone opts into internal
services. No real provider requests, user data migration, service installation,
or external action approval was performed.

Remote release jobs, Linux Electron installers, signing, notarization and
publication were not executed by this repository edit. The local package is
an unsigned development build; no new release has been published here.
