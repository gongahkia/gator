# RPC integration

`gator rpc` (or `gator --mode rpc`) is Gator's versioned JSONL interface for
IDEs, automation, and custom user interfaces. Send one JSON request per line
to standard input and read responses and lifecycle events from standard
output. Gator never writes human-formatted output in this mode.

The protocol has one way to start work: `run` with `mode: "execute"` and an
explicit `verify` argv list, or `mode: "plan"` for enforced read-only work.
This keeps the same worktree, verifier, and tool policy contract as the TUI.

```json
{"version":1,"id":"plan-1","method":"run","params":{"mode":"plan","task":"Inspect this repository","provider":"openai"}}
{"version":1,"id":"change-1","method":"run","params":{"task":"Add a focused test","provider":"openai","verify":[["go","test","./..."]]}}
```

Every accepted run first returns `{"type":"response","result":{"accepted":true}}`,
then emits zero or more `event` messages, then one terminal `response` or
`error` carrying the same request ID. `steer` and `cancel` target an active
request ID:

```json
{"version":1,"id":"steer-1","method":"steer","params":{"run_id":"change-1","message":"Keep the diff limited to the parser."}}
{"version":1,"id":"cancel-1","method":"cancel","params":{"run_id":"change-1"}}
```

The remaining methods are `capabilities`, `status`, `threads`, and `resume`.
`threads` lists presentation-safe metadata only; a run result supplies the
private `state_path` required for `resume`. Events expose a tool name, not tool
arguments or raw output, so an embedding parent does not accidentally inherit
repository contents or secrets from the agent process.

Set `params.compact` to `true` on `resume` to request a model-generated summary
of older retained messages before the next turn. Gator emits a
`context_compacted` event when it does so; immutable private run records are
not replaced.

Go integrations can import `github.com/gongahkia/gator/rpc` for protocol types
and the concurrent-safe JSONL `Client`.
