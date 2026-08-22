# Trusted MCP servers

Gator exposes trusted Model Context Protocol (MCP) tools from a repository
configuration at `.gator/mcp.json`. The configuration is inert until the
developer reviews and explicitly pins it:

```sh
gator mcp status
gator mcp trust
```

`gator mcp status` shows the hash trust state and, for each configured remote
server, only `authenticated`, `not authenticated`, or `expired`. It never
prints an endpoint's access token, refresh token, or public client metadata.

Trust records use the canonical repository path. Their hash covers the
manifest and a streaming SHA-256 digest of each repository-local stdio
executable (up to 128 MiB per executable). Editing a bundle disables it until
it is trusted again. Every MCP tool call still asks
for allow-once, allow-this-exact-tool, or deny approval.

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
must use HTTPS unless they are a loopback development server. The current
client uses MCP 2025-11-25 over stdio or Streamable HTTP.

## Remote OAuth

If a Streamable HTTP server returns `401 Unauthorized`, authenticate the
trusted server explicitly:

```sh
gator mcp login github
# If the authorization server requires an existing public client:
gator mcp login --client-id YOUR_PUBLIC_CLIENT_ID --redirect-url http://127.0.0.1:3000/callback github
gator mcp logout github
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
