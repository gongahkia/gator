# Trusted local LSP

Gator can expose bounded read-only inspection from a repository-local Language
Server Protocol (LSP) server to an Execute-mode native agent. For one existing
workspace source file it supports pull diagnostics, hover, go-to definition,
find references, and document symbols. Definitions and references return only
regular files inside the active worktree. It does not expose completion, rename,
code actions, edits, formatting, or a shared workspace-symbol index.

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

When the model asks for an LSP tool, Gator first requests the usual allow-once,
allow-always, or deny approval using an argv-shaped record such as
`lsp SERVER definition PATH`. The supported operation names are `diagnostics`,
`hover`, `definition`, `references`, and `document_symbols`; approval is scoped
to the exact server, operation, and path. Only after approval does Gator start
the server. The server runs in Gator's strict sandbox by default with its
configured network mode. It is started anew for one lookup and shut down
afterward.

Gator implements the LSP 3.17 requests `textDocument/diagnostic`,
`textDocument/hover`, `textDocument/definition`, `textDocument/references`,
and `textDocument/documentSymbol`. It advertises and checks the corresponding
server capability at `initialize`; a server can expose whichever subset it
supports. Position-taking tools accept a one-based line and zero-based UTF-16
character offset. LSP uses JSON-RPC 2.0 framed with `Content-Length`; Gator
caps frames at 256 KiB, returns at most 128 diagnostics, locations, or symbols
and 64 KiB of tool output, and presents result lines as one-based terminal
values. A server that lacks a requested capability is not started again as a
fallback and returns a clear unsupported-capability error.

The trust hash and sandbox make the capability explicit, but an LSP executable
is still code selected by the developer. Do not trust a project LSP bundle you
would not run locally. A strict sandbox contains it to Gator's worktree and
private scratch space; it does not make an arbitrary language server harmless.
