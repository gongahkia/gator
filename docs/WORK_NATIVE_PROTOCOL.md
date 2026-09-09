# Work-native protocol v1

`gator work-rpc` accepts newline-delimited JSON on stdin and emits versioned JSON
on stdout. This is a Work application surface. Existing RPC, ACP and app-server
requests retain their coding-engine semantics.

The first frame is `{"version":1,"type":"start","request":{...}}`. Request fields:

| Field | Meaning |
| --- | --- |
| `source`, `objective` | Selected directory and bounded objective |
| `provider`, `model` | Optional configured adapter selection |
| `conversation_id`, `parent_revision_id` | Continuation and optional branch |
| `snapshot_id`, `refresh_source` | Explicit capture or live source refresh |
| `mode`, `contract` | Full Work mode and contract v1 |
| `code` | Complete `workrun.CodePolicy` object; existing Go field names are preserved |
| `max_steps`, `limits` | Manager steps and aggregate `model_requests`, `tokens`, `wall_seconds` |
| `connectors`, `web_origins` | Explicit source grants |
| `images`, `attachments` | Validated normalized attachments; `data` is base64 JSON bytes |

For example, an inspection starts with:

```json
{"version":1,"type":"start","request":{"source":"./notes","objective":"Summarize gaps","mode":"inspect","contract":{"version":1,"external_actions":"forbid","artifacts":[]},"limits":{"model_requests":16,"wall_seconds":120}}}
```

While it runs, clients can send:

```json
{"version":1,"type":"steer","text":"Keep uncertainty explicit"}
{"version":1,"type":"inspect_task","task_id":"subagent-001"}
{"version":1,"type":"cancel_task","task_id":"subagent-001"}
{"version":1,"type":"approval","interaction_id":1,"approved":false}
{"version":1,"type":"cancel"}
```

Output frames are `{"version":1,"type":"TYPE","data":...}`. Types are `event`,
`approval`, `task`, `control_error`, `protocol_error`, and terminal `result`.
Approvals include an interaction ID, kind, and exact preview; the client must
show that preview before responding. IDs cannot be replayed after resolution.
The result contains conversation, revision, snapshot and manifest identifiers,
status, final text, and any execution error. Setup errors produce a failed result
with empty execution identifiers. Unknown fields, unsupported versions, and
multiple JSON values per line are rejected. Input frames are bounded to 4 MiB.

Clients should continuously consume output; event delivery applies bounded
backpressure. EOF ends control input, not the running task. Use a cancel frame
for deliberate cancellation. Pending interactions without a response end only
when cancelled or timed out. A failed output writer cancels and joins Work.
