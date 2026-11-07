# Developing Gator

Gator is a CLI/TUI personal agent. The default surface is a conversation and a
message box. Optional controls open through slash commands and Ctrl+P. The
OpenCode-derived Solid/OpenTUI frontend renders the current Go agent state;
no OpenCode agent or server is involved.

Install Go 1.25.13+, Node 22.12+, and Bun 1.4.2, then run `make run`.
`make setup` installs pinned dependencies, `make build` builds both layers, and
`make check` runs the product checks. The `make terminal`, `make terminal-build`
and `make terminal-check` aliases remain available.

```sh
bun ui/launcher/gator.mjs --help
bun ui/launcher/gator.mjs work "Summarize this workspace"
bun ui/launcher/gator.mjs resume <chat-id>
make package PACKAGE_DIR=/tmp/gator-terminal-package
node ui/scripts/install.mjs /tmp/gator-terminal-package /tmp/gator-install
/tmp/gator-install/bin/gator
```

Packaging requires a fresh destination. Installed packages require Bun on PATH;
Go and Node are build requirements. Optional browser and voice features use
explicitly configured native helpers. See [terminal usage](TERMINAL.md).

## Boundaries

- `ui/`: terminal rendering, commands, CLI launcher and packaging.
- `app-host/`: local sessions, drafts, uploads, immutable files, exports,
  preferences, voice and verified retained evidence.
- `internal/workapp/`: canonical conversation state, exact reviews and controls.
- `cmd/gator/ui_bridge.go`: JSON-lines application transport.
- Other Go packages: tools, connectors, memory, scheduling, specialists,
  coding, usage and storage. Frontend changes do not grant execution authority.
- `companion/`: optional authenticated phone extension, independent of Electron.

Public Go commands no longer need `GATOR_INTERNAL_COMMANDS`. Use temporary state
and configuration directories for fixtures. Existing profile paths, IDs,
credentials and formats remain compatible. `GATOR_TERMINAL_USER_DATA` selects a
custom application-data location; `GATOR_DESKTOP_USER_DATA` remains a fallback.
No installed app, user profile or enabled service is automatically removed.

```sh
GATOR_STATE_DIR=/tmp/gator-test-state GATOR_CONFIG_DIR=/tmp/gator-test-config go run ./cmd/gator note list
make check
node ui/scripts/verify-install.mjs /tmp/gator-terminal-package
```

Native sign-in, microphone permissions, service startup and terminal rendering
need native execution. Cross-compilation alone does not verify them. Historical
audits retain their original observations; see the [current port inventory](CLI-TUI-PORT.md).
