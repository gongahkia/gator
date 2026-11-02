# Trusted MCP servers

**Interface status:** Gator is a desktop app. Shell examples below are source-checkout developer operations with an explicit internal opt-in; see [development instructions](DEVELOPMENT.md). They do not install or enable a public CLI.

Gator exposes trusted Model Context Protocol (MCP) tools from a repository
configuration at `.gator/mcp.json`. The configuration is inert until the
developer reviews and explicitly pins it:

```sh
GATOR_INTERNAL_COMMANDS=1 go run ./cmd/gator mcp status
GATOR_INTERNAL_COMMANDS=1 go run ./cmd/gator mcp trust
```

`gator mcp status` shows the hash trust state and, for each configured remote
server, only `authenticated`, `not authenticated`, or `expired`. It never
prints an endpoint's access token, refresh token, or public client metadata.

Trust records use the canonical repository path. Their hash covers the
manifest and a streaming SHA-256 digest of each repository-local stdio
executable (up to 128 MiB per executable). Editing a bundle disables it until
it is trusted again. Every MCP tool call still passes the existing allow-once,
allow-this-exact-tool, or deny policy. A remembered exact-tool permission may
satisfy later calls under that policy; this is not a promise of a new prompt or
payload-specific approval for every project-tool call. Mapped connector mutations
retain their separate exact-action approval and receipt path.

```json
{
  "version": 1,
  "servers": [
    {
      "name": "github",
      "transport": "streamable_http",
      "url": "https://mcp.example.com/mcp"
    },
    {
      "name": "repository-tools",
      "transport": "stdio",
      "command": [".gator/mcp/repository-tools"],
      "network": "deny"
    }
  ]
}
```

Names use lower-case letters, digits, underscores, and hyphens. Stdio commands
are an argv array whose executable is a regular, executable file inside the
repository; Gator never resolves it through `PATH` or a shell. HTTP servers
must use HTTPS unless they are a loopback development server. The client supports the tools subset of MCP `2025-11-25` and `2026-07-28`
over stdio or Streamable HTTP. It first performs bounded, read-only modern
`server/discover` setup, then uses a verified compatible modern response or a
legacy `initialize` handshake and `notifications/initialized`. Unsupported
versions fail before tools become available. Negotiation never repeats a tool
mutation. See [MCP transport hardening](MCP-HARDENING.md) for exact bounds,
compatibility, failure handling and verification.

## Remote OAuth

If a Streamable HTTP server returns `401 Unauthorized`, authenticate the
trusted server explicitly:

```sh
GATOR_INTERNAL_COMMANDS=1 go run ./cmd/gator mcp login github
# If the authorization server requires an existing public client:
GATOR_INTERNAL_COMMANDS=1 go run ./cmd/gator mcp login --client-id YOUR_PUBLIC_CLIENT_ID --redirect-url http://127.0.0.1:3000/callback github
GATOR_INTERNAL_COMMANDS=1 go run ./cmd/gator mcp logout github
```

Gator follows the MCP HTTP authorization flow: it discovers protected-resource
metadata from the `WWW-Authenticate` challenge or the required well-known
paths, discovers OAuth or OpenID Connect authorization-server metadata,
requires advertised PKCE `S256`, and includes the canonical MCP resource in
both authorization-code and token requests. It uses the challenge scope when
present; otherwise it requests the resource metadata's advertised scopes.

For a client unknown to the authorization server, Gator uses OAuth Dynamic
Client Registration as a public client and registers the exact ephemeral
`127.0.0.1` callback URL before it opens the browser. It registers a fresh
client for each such login, because the redirect URI must match exactly. If
dynamic registration is unavailable, register a public client with the service and provide its ID
and exact registered loopback URI using `--client-id` and `--redirect-url`.
Gator cannot use Client ID Metadata Documents because it
does not host a stable public HTTPS metadata document; it reports that
constraint instead of claiming an identity it does not own.

The access and refresh token live only in Gator's private `0600` credential
store. The store key is a hash of the canonical MCP resource; accompanying
metadata must match that exact resource, token endpoint, and public client ID
before Gator attaches or refreshes the token. Tokens are sent only in the
`Authorization: Bearer` header, never in a URL or project file. A project MCP
configuration never contains a client secret, access token, refresh token, or
credential source. Stdio MCP servers do not use this flow; give them required
credentials through the sandbox's explicit environment policy instead.

OAuth remains an external authorization grant. Gator can test protocol flow
locally, but cannot verify a third-party authorization server's account,
scope, consent page, or data handling.

## Discovery, streaming and cancellation

Tool discovery follows opaque cursors with bounds of 32 pages, 256 tools total
across a project configuration and 1 MiB per server catalog. Duplicate tools,
colliding public aliases, repeated/oversized cursors and incomplete discovery
fail explicitly; a partial tool set is not silently enabled. Native tool names
may use ASCII letters, digits, underscores, hyphens and dots (up to 128 bytes).
Gator's model-facing names remain bounded safe aliases; original names are used
on the wire. These rules do not widen connector or server identity syntax.

HTTP streams are parsed incrementally through comments, notifications and
multiline SSE data until the matching response arrives. Capturing a result does
not wait for server EOF. Messages and requests are bounded to 128 KiB; tool
results retain the existing 64 KiB limit. Streams consume at most 1 MiB and 256
nonempty events. A request and its queue wait have a two-minute deadline or the
caller's shorter deadline; setup discovery has a two-second probe deadline.

Both transports have cancellable request gates. Cancelling a stdio operation
attempts a bounded cancellation notification and closes its owned subprocess;
stalled writes and ignored EOF cannot indefinitely block the owner. Closing
stdin starts shutdown, followed by forced termination after 250 ms and a bounded
250 ms reap wait. If exit cannot be observed, cleanup is deferred until exit and
an explicit error is returned. Only read-only discovery may time out and fall
back to legacy setup in the same process; late replies cannot match a new ID.
HTTP cancellation closes its request stream. Neither transport automatically
restarts or replays tool calls after failure.

The newer stateless path sends per-request version/capability metadata and the
required method/tool headers, without legacy session headers. Supported primitive
`x-mcp-header` mappings are validated and encoded from the exact transmitted
arguments; malformed, conflicting or unsupported annotations fail catalog
admission. Derived headers are bounded to 16 KiB. Redirects are refused and
server-authored RPC error text is excluded from ordinary diagnostics.

This is a tools client, not an implementation of every MCP capability. Additional
interaction (`input_required`), server-initiated sampling/elicitation, resources,
prompts, subscriptions, tasks and stream resumption are not implemented. Such
interactions do not gain approval or trigger automatic retry. Existing OAuth
setup is retained; broad vendor compatibility and complete modern OAuth
conformance are not certified by local fixture tests. A timeout or disconnect
after dispatch can leave the remote outcome unknown. Approval and transport
acknowledgement do not prove an external action completed.
