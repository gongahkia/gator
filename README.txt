I've learned a lot by looking at other people's code so
maybe someone can learn something from looking at mine.

= = = = = = = =

Gator is a desktop personal agent for ongoing work.

Open the Gator app to start a conversation, continue an ongoing goal, review
follow-up results, and manage saved knowledge. Gator can research selected
sources, analyze data, create documents, and prepare actions through selected
tools. External changes still require explicit approval.

The Electron frontend is the primary and currently supported user interface.
The standalone CLI and React terminal renderer are retired. Existing saved
conversations, goals, knowledge, credentials, and settings remain in the same
local Gator profile. The app bundles the Go runtime; users do not need a CLI
or a separate Node.js installation.

For local development, install Go 1.25.13 and Node.js 22.12 or newer, then run:

  npm ci --prefix desktop
  make desktop

On Windows, use `npm start --prefix desktop` instead of `make desktop`. The
Windows desktop port is a preview; capability limits and native verification
gates are documented in docs/WINDOWS.md.

Choose a workspace from the app's folder control. A first launch starts in a
private empty workspace rather than implicitly reading your home directory.
Use Settings for models, usage limits, backups, and recovery.

Build with `make build`; create a local application bundle with `make package`.
`make dist` creates platform installers under desktop/release/. macOS builds
are unsigned development builds until release signing is configured.
See desktop/README.md and docs/DESKTOP.md for setup and current boundaries.
See docs/DESKTOP-PRIMARY.md for the CLI retirement decision.

The Go process under desktop/runtime/ is a private app component, not an
installable command-line product. Reviewed system sign-in or installation
steps may still open the operating system's Terminal and return to the app.

= = = = = = = =

🐊
