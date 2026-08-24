# RPC integration

`gator rpc` (or `gator --mode rpc`) is Gator's versioned JSONL interface for
IDEs, automation, and custom user interfaces. Send one JSON request per line
to standard input and read responses and lifecycle events from standard
output. Gator never writes human-formatted output in this mode.

The protocol has one way to start work: `run` with `mode: "execute"` and an
explicit `verify` argv list, or `mode: "plan"` for enforced read-only work.
This keeps the same worktree, verifier, and tool policy contract as the TUI.
`run` also accepts `scopes`, `profile`, `base_ref`, `copy_ignored_files`, and `scouts`.
Scopes select layered project instructions; a base ref is resolved to an
immutable commit; copying ignored setup files remains opt-in; and scouts are
bounded read-only subagent assignments. This is Gator's own versioned JSONL
protocol, not an ACP implementation.

```json
{"version":1,"id":"plan-1","method":"run","params":{"mode":"plan","task":"Inspect this repository","provider":"openai"}}
{"version":1,"id":"change-1","method":"run","params":{"task":"Add a focused test","provider":"openai","verify":[["go","test","./..."]]}}
```

`images` and `attachments` optionally supply explicit repository-relative
paths for a `run` or `resume` request. `images` accepts PNG, JPEG, and WebP;
`attachments` accepts PDFs and Gator's supported text/data or Office document
formats. Gator reads these paths through the fixed repository boundary, applies
the same four-file, 4 MiB-per-file, and 8 MiB-combined limits as the terminal
UI, and never accepts raw attachment bytes over RPC. This keeps a controller
from widening Gator's filesystem scope or bypassing attachment validation:

```json
{"version":1,"id":"visual-1","method":"run","params":{"mode":"plan","task":"Inspect the attached regression.","provider":"openai","images":["failure.png"],"attachments":["browser.log"]}}
```

Every accepted run first returns `{"type":"response","result":{"accepted":true}}`,
then emits zero or more `event` messages, then one terminal `response` or
`error` carrying the same request ID. `steer`, `cancel`, and `approve` target an
active request ID:

```json
{"version":1,"id":"steer-1","method":"steer","params":{"run_id":"change-1","message":"Keep the diff limited to the parser."}}
{"version":1,"id":"cancel-1","method":"cancel","params":{"run_id":"change-1"}}
{"version":1,"id":"approve-1","method":"approve","params":{"run_id":"change-1","decision":"allow_once"}}
```

`decision` is `allow_once`, `allow_always`, or `deny`. Required `--verify` argv
runs without this prompt. Any other `run_command` blocks until `approve` or
`cancel`. Clients that ignore `command_approval_requested` will hang; Gator does
not auto-allow. `always` is remembered on the retained thread, not globally.

The remaining methods are `capabilities`, `status`, `threads`, and `resume`.
`threads` lists presentation-safe metadata only; a run result supplies the
private `state_path` required for `resume`. Events expose a tool name, not tool
arguments or raw output, except `command_approval_requested`, which includes the
`argv` a parent must see to approve. Command stdout is still omitted. Method
availability is discovered through `capabilities`; the plain JSONL process has
no process-long terminal registry.

Set `params.compact` to `true` on `resume` to request a model-generated summary
of older retained messages before the next turn. Gator emits a
`context_compacted` event when it does so; immutable private run records are
not replaced.

Go integrations can import `github.com/gongahkia/gator/rpc` for protocol types
and the concurrent-safe JSONL `Client`.

## HTTP/SSE transport

For a local controller that cannot manage stdio, [the local app server](APP_SERVER.md)
accepts this exact request type at `POST /v1/rpc` and emits the same protocol
messages as SSE at `GET /v1/events/{id}`. It is an authenticated loopback
bridge, not another execution path: client input cannot add setup commands,
filesystem roots, MCP servers, credentials, or a weaker policy.

`gator serve` additionally exposes control of an already detached terminal
task through `terminal_list`, `terminal_read`, `terminal_write`,
`terminal_resize`, and `terminal_stop`. It advertises those methods only for
that server process. The complete lifecycle and the direct-developer-input
boundary are documented in [the app-server guide](APP_SERVER.md#detached-terminals).

For an editor that needs a stable local endpoint, `gator serve start` launches
one explicit repository-scoped background bridge, while `status` and `stop`
require the same private token file. It is not automatically started or shared
with the native TUI or ACP client.
