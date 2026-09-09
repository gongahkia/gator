# Trusted local LSP

Gator can expose bounded read-only inspection from a repository-local Language
Server Protocol (LSP) server to an Execute-mode native agent. For one existing
workspace source file it supports pull diagnostics, hover, completion, code
actions, document formatting, symbol rename, go-to definition, find references,
and document symbols. It also supports a repository-wide workspace-symbol query.
Definitions, references, and workspace symbols return only regular files inside
the active worktree. Completion returns bounded informational suggestions. Code
actions, formatting, and rename return only bounded text edits to existing
regular files in that worktree; Gator never executes a server-provided command,
applies an LSP edit automatically, or follows resource operations such as
create, rename, or delete. The native agent can turn a returned edit into an
ordinary reviewable `apply_patch` call. Gator does not expose a persistent
cross-process server or on-disk index. The authenticated legacy `gator serve`
machine-integration process can retain a trusted server across compatible
resumed runs of the same retained worktree. Main-TUI Work delegations are
one-shot and do not retain a child LSP process between revisions.

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
`hover`, `completion`, `code_actions`, `format`, `rename`, `definition`,
`references`, `document_symbols`, and `workspace_symbols`; approval is scoped to the exact server, operation, and
path or symbol query. Only after approval does Gator start the server. The
server runs in Gator's strict sandbox by default with its configured network
mode. The first approved lookup lazily starts it; later approved lookups reuse
that same server within the current run. The authenticated `gator serve`
compatibility surface additionally keeps at most eight idle trusted managers
in memory, keyed by canonical retained-worktree path and exact trusted bundle
hash. A changed manifest, executable, trust record, or root set retires the old
manager. Gator shuts every cached manager down when the app-server process
exits and discards unhealthy clients after transport failure. This cache is
process-local; it writes no index and resurrects no process after restart.

When a server advertises `textDocumentSync`, Gator synchronizes each
file-targeted lookup from its current on-disk snapshot: it sends `didOpen` once,
sends a full-content `didChange` only when the bounded (128 KiB) content hash
changes, and sends `didClose` on discard or shutdown. Incremental-capable
servers receive a valid full replacement because Gator observes files, not
editor keystrokes. Servers without document synchronization remain supported.
Gator never shares an LSP process between distinct worktrees or distinct
additional-root sets.

Gator implements the LSP 3.17 requests `textDocument/diagnostic`,
`textDocument/hover`, `textDocument/completion`, `textDocument/codeAction`,
`textDocument/formatting`, `textDocument/rename`, `textDocument/definition`,
`textDocument/references`,
`textDocument/documentSymbol`, and `workspace/symbol`.
It advertises and checks the corresponding server capability at `initialize`; a
server can expose whichever subset it supports. Position-taking tools accept a
one-based line and zero-based UTF-16 character offset. The workspace-symbol
query is limited to 512 printable bytes. Completion accepts both the LSP list
and array result forms, returns no more than 128 labels/details/documentation/
insert-text suggestions, and marks a response truncated when the 64 KiB tool
boundary would otherwise be exceeded. Code-action requests use a selected
range, return at most 64 actions and 128 total edits, limit every replacement
text to 16 KiB, and omit any whole action whose edit leaves the worktree, needs
a resource operation, or does not fit those limits. Formatting and rename use
the same text-edit limit and return no partial proposal: an edit that is too
large, leaves the worktree, requires a resource operation, or exceeds 128 edits
is omitted with `truncated: true`. Rename accepts a one-based position and a
new name of at most 256 printable bytes; formatting uses fixed four-space LSP
formatting options and does not write the file. A `command_omitted` marker
records that the server paired an otherwise usable action with a command Gator
will not run. LSP uses JSON-RPC 2.0 framed with
`Content-Length`; Gator caps frames at 256 KiB, returns at most 128 diagnostics,
locations, symbols, or completion suggestions, and presents result lines as
one-based terminal values. A server that lacks a requested capability is not
started again as a fallback and returns a clear unsupported-capability error.

The trust hash and sandbox make the capability explicit, but an LSP executable
is still code selected by the developer. Do not trust a project LSP bundle you
would not run locally. A strict sandbox contains it to Gator's worktree and
private scratch space; it does not make an arbitrary language server harmless.
