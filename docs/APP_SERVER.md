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

# Explicitly supervise one repository-scoped background service.
gator serve start --token-file "$HOME/.local/share/gator/app-server-token"
gator serve status --token-file "$HOME/.local/share/gator/app-server-token"
gator serve stop --token-file "$HOME/.local/share/gator/app-server-token"

# Or keep the server in the foreground with a caller-selected port.
gator serve --token-file "$HOME/.local/share/gator/app-server-token" \
  --listen 127.0.0.1:49152
```

Every control endpoint requires `Authorization: Bearer TOKEN`, where a client
reads `TOKEN` from that file. Do not put it in a command line, configuration
file, browser URL, or repository. `GET /healthz` and `GET /readyz` are the only
unauthenticated endpoints and disclose only readiness, build version, and RPC
protocol version.

`gator serve start` is an explicit local service, not an automatic global
daemon. It inherits the current repository and configuration, starts on a
random literal-loopback port by default, and writes private repository-scoped
metadata and a log under Gator's state directory. `status` and `stop` require
the same token file and first authenticate against the recorded endpoint before
they act on its PID. Gator refuses a service-state directory that is readable
by another local account. The native TUI and ACP do not auto-discover or join
this service yet.

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
event and must reconnect with its last observed event ID. If that ID predates
the retained replay window (or is ahead of the stream), the reconnect receives
`409 Conflict` instead of silently skipping messages; the client must
reconcile its request state before it resumes streaming.

To bound a long-lived local process, Gator retains terminal streams for fifteen
minutes after their last client disconnects, up to 1,024 streams. It then
reclaims them when accepting a later request. A non-terminal run is never
discarded by this retention policy.

`GET /openapi.json` publishes the small HTTP wrapper contract. It does not
invent a second agent API: run/resume/steer/cancel/approve/status/threads stay
defined by [RPC integration](RPC.md).

## Detached terminals

Only `gator serve` and the native TUI create a process-local terminal registry.
If an execute-mode model starts a terminal task and requests the separately
approved `terminal_detach` tool, the app server advertises these additional
methods from `capabilities`:

```text
terminal_list
terminal_read    {"terminal_id":"...", "cursor":0}
terminal_write   {"terminal_id":"...", "input":"..."}
terminal_resize  {"terminal_id":"...", "rows":24, "columns":80}
terminal_stop    {"terminal_id":"..."}
terminal_restart {"terminal_id":"..."}
```

They operate only on that server process's explicitly detached tasks; they
cannot choose a process command, change its fixed worktree or sandbox, attach
to a task from another Gator process, or detach a task themselves.
`terminal_restart` starts a fresh task only from an exited task's exact argv;
the original task and its scrollback remain available. `terminal_write` is
direct developer input from the authenticated bearer-token holder. Its bytes
are neither sent to the model nor appended to the completed run record.

Detached tasks retain their original policy, are capped at eight retained
histories per server process, and expire within two hours while running. A
normal `gator serve` shutdown stops them. The server handles `Ctrl-C` and
normal Unix `SIGTERM` shutdown before it releases its terminal registry;
`gator serve stop` asks the authenticated server's `POST /v1/shutdown`
endpoint to use that same path, rather than trusting or signaling the PID in a
state file. Tasks do not survive a `gator serve` restart, and plain `gator
rpc`, ACP, and child-writer runs do not offer detachment or these controller
methods.

## Security boundary

The listener accepts literal loopback IP addresses only (`127.0.0.1` or
`::1`); it intentionally has no remote, mDNS, CORS, or browser origin mode.
Requests with an `Origin` header are rejected before authentication. Use SSH
port forwarding when a remote client is necessary, and keep the local bearer
token private. The server has a 1 MiB request limit, bounded per-request replay
history, bounded subscriber queues, five-second header reads, and fifteen-second
request reads. SSE has no write timeout by design because it is a long-lived
event stream. Token and service-state reads verify the same private regular
file before and after opening it, rejecting symlink or replacement races.

This is a local bridge, not a hosted executor, remote worktree service, or
organization policy plane. It cannot change Gator's repository root, inject
MCP servers, pass `--setup`, weaken sandbox policy, or forward provider
credentials from a client.
