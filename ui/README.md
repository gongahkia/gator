# Gator terminal interface

This is the live React terminal frontend. The Go Work TUI remains the source
of truth for commands, model setup, runs, approvals, conversation history, and
deliverables. The frontend renders the chat and command panels from a versioned
JSON-line state projection; it sends requests and keyboard controls back.
`src/protocol.ts` also supplies the display types used by the optional desktop
companion. Native desktop actions run through the same Go application.

The Go executable embeds `cmd/gator/ui/gator-ui.cjs`. To change this frontend:

```sh
cd ui
npm ci
npm run check
npm run test
npm run build
cd ..
go build -o bin/gator ./cmd/gator
./bin/gator
```

Node.js 22 or newer is required for interactive Gator and `gator doctor`. If it
is missing or too old, those commands exit with an installation message.
Headless commands continue to work. The generated bundle is checked against the TypeScript source in
CI and included in native releases. `ui/licenses/` contains notices for the
bundled terminal UI dependencies.

The renderer reads the saved theme from Go. Its result cards and doctor report
use typed status fields and tones; transcript search indexes the content shown
on screen. `docs/DESIGN_SYSTEM.md` records the kit components and the Gator
adapters used for the composer, search, pickers, and workflow forms.
