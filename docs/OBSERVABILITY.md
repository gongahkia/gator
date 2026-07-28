# Operator trace and forensics

Every run records an append-only `trace_events` timeline. Each row can link the run, workflow stage and attempt, provider, planner/code revision, approval operation, central-agent turn, tool action, sandbox execution, policy version, actor, and OpenTelemetry trace/span context.

Norbot does not capture or expose private model reasoning. For `openai_responses` calls to GPT-5 models, it requests the provider-supported reasoning summary and records that summary with the provider-completed event and trace. It is an operator-facing rationale, not raw chain-of-thought; other providers continue to expose their returned output and normal trace metadata only.

## Console

Open **Runs**, expand one run, then use **Unified trace**. It is the primary traceback view: search the redacted timeline, inspect entity references and trace IDs, and expand the summary for agent turns, sandbox executions, and policy history. The JSON export uses the exact trace filter and records the export in `audit_events`.

The API equivalents are:

- `GET /api/events/stream` for replayable global run lifecycle events. Connect with `Last-Event-ID` or `?after=<event-id>` to resume; a new connection starts at the current cursor.
- `GET /api/runs/{id}/trace?cursor=&q=&stage=&type=&severity=&provider_id=&tool=&actor=&entity_id=&from=&to=`
- `GET /api/runs/{id}/trace/export?format=json|ndjson&include=redacted|raw`
- `GET /api/runs/{id}/trace/{event_id}/raw`
- `GET /api/runs/{id}/agent-turns`, `GET /api/agent/turns/{id}`, `GET /api/runs/{id}/sandboxes`
- `GET /api/runs/{id}/agent-policy-history`

Pagination is opaque. `from` and `to` use RFC3339. Standard trace responses contain only redacted payload projections; trace/span IDs join them to the local OpenTelemetry trace backend.

## Builder-response failures

Norbot writes `stage-output/builder-<attempt>.json` before validating a builder response. If validation fails, it updates that artifact with `builder_validation` and records a `builder_response_rejected` trace event containing the rejection reason, invalid path, file count, response size, and required path prefix. Structured logs include the same metadata but never the response body.

## Local forensic mode

`forensics.raw_capture` stores raw payloads in encrypted `forensic_payloads` records. It is intentionally rejected unless all of the following are true:

1. `security.public` is false.
2. OIDC is disabled.
3. `NORBOT_HTTP_ADDR` is loopback (`127.0.0.1`, `::1`, or `localhost`).
4. `forensics.master_key_env` names an environment variable containing a base64-encoded 32-byte key.

Example:

```json
{
  "forensics": {"raw_capture": true, "master_key_env": "NORBOT_FORENSICS_KEY"},
  "retention": {"trace_events_days": 365, "forensic_payload_days": 30}
}
```

Raw reveal/export is audited. It is excluded from search indexes, structured logs, OpenTelemetry attributes, URLs, local browser storage, and notifications. Existing values that were redacted before this feature cannot be reconstructed.

## Correlation contract

Workers persist W3C `traceparent` with jobs, restore it before execution, then write `trace_id` and `span_id` onto run events, revisions, approvals, provider usage, agent turns/actions, and sandbox executions. Logger calls in those paths must use the execution context so `run_id`, stage/attempt, provider, turn/action, and trace/span IDs stay aligned.

Trace retention defaults to 365 days; encrypted forensic payloads default to 30 days. Deleting a run deletes its trace and associated forensic payloads.
