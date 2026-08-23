# Local app server

`gator serve` exposes Gator's existing versioned RPC protocol over an
authenticated loopback HTTP/SSE transport. It is intended for a local editor,
desktop client, or CI-side controller that needs event streaming without
managing a child process's standard input and output.

Create a private capability token once. The target must be an absolute path;
Gator creates the file only if it does not exist, with mode `0600`, and never
prints its contents.

```sh
gator serve token "$HOME/.local/share/gator/app-server-token"
gator serve --token-file "$HOME/.local/share/gator/app-server-token" \
  --listen 127.0.0.1:49152
```

Every control endpoint requires `Authorization: Bearer TOKEN`, where a client
reads `TOKEN` from that file. Do not put it in a command line, configuration
file, browser URL, or repository. `GET /healthz` and `GET /readyz` are the only
unauthenticated endpoints and disclose only readiness, build version, and RPC
protocol version.

## Contract

`POST /v1/rpc` accepts exactly one JSON object matching the public
[`rpc.Request`](../rpc/protocol.go) v1 type. It returns `202 Accepted` with an
event URL, then the existing RPC server processes the request without changing
its run, verifier, worktree, approval, or credential policy.

```json
{
  "version": 1,
  "id": "plan-1",
  "method": "run",
  "params": {"mode": "plan", "task": "Inspect the repository", "provider": "openai"}
}
```

The returned `events` path is `GET /v1/events/{id}`. It is an SSE stream of the
same RPC `response`, `event`, and `error` messages that `gator rpc` writes as
JSONL. Each SSE item has a monotonic ID. Reconnect with `Last-Event-ID` to
replay later retained messages. Gator keeps at most 256 messages per request
ID and 64 queued messages per client; a slow client receives an `overflow`
event and must reconnect with its last observed event ID.

To bound a long-lived local process, Gator retains terminal streams for fifteen
minutes after their last client disconnects, up to 1,024 streams. It then
reclaims them when accepting a later request. A non-terminal run is never
discarded by this retention policy.

`GET /openapi.json` publishes the small HTTP wrapper contract. It does not
invent a second agent API: run/resume/steer/cancel/approve/status/threads stay
defined by [RPC integration](RPC.md).

## Security boundary

The listener accepts literal loopback IP addresses only (`127.0.0.1` or
`::1`); it intentionally has no remote, mDNS, CORS, or browser origin mode.
Requests with an `Origin` header are rejected before authentication. Use SSH
port forwarding when a remote client is necessary, and keep the local bearer
token private. The server has a 1 MiB request limit, bounded per-request replay
history, bounded subscriber queues, five-second header reads, and fifteen-second
request reads. SSE has no write timeout by design because it is a long-lived
event stream.

This is a local bridge, not a hosted executor, remote worktree service, or
organization policy plane. It cannot change Gator's repository root, inject
MCP servers, pass `--setup`, weaken sandbox policy, or forward provider
credentials from a client.
