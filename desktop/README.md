# Gator desktop

Release signing, notarization, update behavior and the remaining account/native
setup are documented in [desktop releases](../docs/DESKTOP-RELEASES.md).
Development builds keep using manual updates; neither signing account is set up
yet. Current source-checkout commands are in [development](../docs/DEVELOPMENT.md).

Gator's primary and currently supported user interface is this Electron app.
React DOM and Tailwind render the frontend; the private Go runtime owns
conversations, tools, model configuration, approvals, goals, usage, and data.
The standalone CLI and React terminal renderer are retired.

## Run

From the repository root, with Go 1.25.13 and Node.js 22.12+ installed:

```sh
make run
```

`make run` (or `make desktop`) installs the pinned desktop dependencies when
needed, builds the private runtime and frontend, then opens Gator.
On Windows or without Make, run `npm ci --prefix desktop` followed by
`npm start --prefix desktop`. Installed apps
bundle Electron's Node runtime; users do not need Node or a Gator CLI on PATH.

The app uses the existing local Gator profile and opens your continuing main
conversation. Existing topics remain separate and are available in the sidebar
and Saved. Following groups goals, configured checks and updates.

Before a main conversation exists, Gator starts in a private empty workspace.
Conversation information → This conversation provides the folder picker. Choosing
a different folder starts a separate topic. To choose a starting folder during
development:

```sh
GATOR_DESKTOP_WORKSPACE=/absolute/path/to/notes make desktop
```

`GATOR_STATE_DIR` and `GATOR_CONFIG_DIR` select the local profile.
`GATOR_DESKTOP_USER_DATA` selects Electron's separate preferences/drafts directory.
`GATOR_DESKTOP_BINARY` can select a separately built private runtime during
development. These overrides are useful for isolated testing.

## Ask Gator about the app

You can ask questions such as “Where do I change the model?”, “Which model are
you using?”, “What can you access?”, or “How do I manage what you remember?”.
Each request includes a bundled guide to Gator's actual desktop controls and a
limited snapshot of the current model, effective work mode, selected connections,
and supplied saved-context count. The snapshot excludes credentials and arbitrary
app logs. Gator can guide you to settings; changing the model still uses
Settings → Models or `/model`.

## Check and package

```sh
npm run check --prefix desktop
npm test --prefix desktop
make build
make package
```

`make build` builds the frontend and private `desktop/runtime/gator-runtime`
(`gator-runtime.exe` on Windows).
`make package` creates an unpacked native app under `desktop/release/`.
On macOS, it is `desktop/release/mac/Gator.app`. To exercise that package:

```sh
GATOR_DESKTOP_TEST_EXECUTABLE="$PWD/desktop/release/mac/Gator.app/Contents/MacOS/Gator" \
  npm run test:e2e --prefix desktop
```

`make dist` builds desktop installers. The release workflow produces macOS
DMG/ZIP, Linux AppImage/DEB, and Windows NSIS/ZIP artifacts with matching runtime architectures.
Windows is a preview port; see [capabilities and verification](../docs/WINDOWS.md).
macOS builds are unsigned development builds until signing and notarization
are configured. No standalone CLI archives are built or installed.

The executable under `runtime/` is an internal app component. Bare invocation
and former user CLI commands direct users to the desktop app. App-owned
background services and reviewed system authentication/installation handoffs
retain their existing paths. Such a handoff may open Terminal temporarily;
it does not launch an interactive Gator terminal interface.

For developer-only fixtures and maintenance, `GATOR_INTERNAL_COMMANDS=1`
explicitly enables the existing runtime command handlers. This is not a
supported user interface or a security boundary. Never substitute it for
an action review. See [the retirement decision](../docs/DESKTOP-PRIMARY.md).

The live surface mapping and remaining application work are documented in
[the conversational implementation](../docs/DESKTOP-CONVERSATIONAL-IMPLEMENTATION.md).
