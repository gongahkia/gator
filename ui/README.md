# Gator terminal interface

This is the live React terminal frontend. The Go Work TUI remains the source
of truth for commands, model setup, runs, approvals, conversation history, and
deliverables. The frontend renders the chat and command panels from a versioned
JSON-line state projection; it sends requests and keyboard controls back.

The Go executable embeds `cmd/gator/ui/gator-ui.cjs`. To change this frontend:

```sh
cd ui
npm ci
npm run check
npm run build
cd ..
go build -o bin/gator ./cmd/gator
./bin/gator
```

Node.js 22 or newer is required for the React display. When it is absent,
Gator starts the native Go TUI. Set `GATOR_NATIVE_TUI=1` to use that interface
on purpose. The generated bundle is checked against the TypeScript source in
CI and included in native releases. `ui/licenses/` contains notices for the
bundled terminal UI dependencies.
