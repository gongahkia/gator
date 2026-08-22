# Local LSP diagnostics

Gator can expose diagnostics from a repository-local Language Server Protocol
(LSP) server to an Execute-mode native agent. This is a narrow code-intelligence
surface: Gator requests diagnostics for one existing workspace file and returns
bounded structured locations, severity, source, code, and message. It does not
yet expose completion, navigation, symbols, code actions, edits, or a language
server index.

Create `.gator/lsp.json` in the repository:

```json
{
  "version": 1,
  "servers": [
    {
      "name": "gopls",
      "command": ["tools/gopls", "serve"],
      "language": "go",
      "network": "deny"
    }
  ]
}
```

Each server has a lower-case name, a descriptive language ID, and a
repository-relative executable command. The executable must be a regular,
executable file in the checkout. `network` defaults to `deny`; `allow` is an
explicit opt-in for language servers that need it. Gator never resolves an LSP
executable from `PATH` and accepts no shell command strings, absolute paths,
symlinks escaping the worktree, or remote LSP endpoints.

Review the file and executable, then activate it from the checkout:

```sh
gator lsp status
gator lsp trust
```

Gator stores a canonical-repository trust record containing a SHA-256 hash of
the manifest and a streaming SHA-256 digest of every configured executable (up
to 128 MiB each). Editing either disables the
bundle until it is explicitly trusted again. Remove the trust record with
`gator lsp untrust`.

## Runtime boundary

When the model asks for a server's diagnostic tool, Gator first requests the
usual allow-once, allow-always, or deny approval using an argv-shaped record:
`lsp SERVER diagnostics PATH`. Only after approval does it start the server.
The server runs in Gator's strict sandbox by default with its configured
network mode. It is started anew for the one lookup and shut down afterward.

Gator implements the LSP 3.17 pull-diagnostic request
`textDocument/diagnostic`; a configured server must advertise
`diagnosticProvider` during `initialize`. LSP uses JSON-RPC 2.0 framed with
`Content-Length`; Gator caps frames at 256 KiB, returns at most 128 diagnostics
and 64 KiB of tool output, and changes LSP's zero-based line numbers to
one-based terminal output. Servers which offer only push diagnostics are not
supported by this first surface.

The trust hash and sandbox make the capability explicit, but an LSP executable
is still code selected by the developer. Do not trust a project LSP bundle you
would not run locally. A strict sandbox contains it to Gator's worktree and
private scratch space; it does not make an arbitrary language server harmless.
