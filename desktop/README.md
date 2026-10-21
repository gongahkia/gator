# Gator desktop

An Electron companion to the terminal, adapted from the sibling Referene
(OpenHands Agent Canvas) frontend. React DOM and Tailwind render the desktop;
Gator's existing Go application owns conversations, tools, model configuration,
approvals, goals, usage, and data controls.

## Run

From the repository root, with Go and Node.js 22.12+ installed:

```sh
npm ci --prefix desktop
make desktop
```

This builds the Go runtime and frontend, then opens the desktop app. The
terminal still starts with `make build` and `./bin/gator`.

Both use your normal Gator profile. A desktop launch starts in a private empty
workspace; choose a folder from the top-right folder control, or resume a saved
goal or conversation. You can choose a starting folder explicitly:

```sh
GATOR_DESKTOP_WORKSPACE=/absolute/path/to/notes make desktop
```

`GATOR_STATE_DIR` and `GATOR_CONFIG_DIR` work exactly as in the CLI. Use them
to try the desktop with a separate profile. `GATOR_DESKTOP_USER_DATA` selects
Electron's separate window/preferences directory for isolated development.
`GATOR_DESKTOP_BINARY` can select a separately built Gator binary for development.

## Check and package

```sh
npm run check --prefix desktop
npm test --prefix desktop
npm run build --prefix desktop
npm run test:e2e --prefix desktop
npm run pack --prefix desktop
```

`pack` creates an unpacked native application under `desktop/release/`.
On macOS, it is `desktop/release/mac/Gator.app`. To exercise that application
with the same isolated integration tests:

```sh
GATOR_DESKTOP_TEST_EXECUTABLE="$PWD/desktop/release/mac/Gator.app/Contents/MacOS/Gator" \
  npm run test:e2e --prefix desktop
```

`npm run dist --prefix desktop` builds platform installers. Build on the target
OS and architecture; the Go runtime is compiled for the build host. macOS
artifacts are unsigned development builds; signing and notarization are a
separate release step. Electron bundles Node; an installed application does
not require a separate Node install.

See [desktop architecture and rollout](../docs/DESKTOP.md) for controls,
boundaries, verification, and the decision to keep both interfaces.
