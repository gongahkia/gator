# Data lifecycle

## Classification

| Class | Examples | Storage and controls |
| --- | --- | --- |
| Secret | provider keys, OIDC credentials, channel tokens, backup passphrases | environment or secret manager only, except explicit local forensic mode described below |
| Restricted | prompts, channel text, agent turns, attachment contents, generated source | PostgreSQL/artifact storage; retention policy; operator/OIDC access only |
| Internal | skill provenance, architectures, revisions, usage, deployment state | PostgreSQL; immutable provenance/audit where applicable |
| Operational | health, aggregate metrics, cache/outbox state | metrics and logs; no secret values |

## Required controls

1. Set explicit retention days in `config.json`; the worker purges eligible records hourly.
2. Keep object storage private, encrypt it using its platform controls, and expire managed attachments after 30 days unless a shorter policy applies.
3. Redact credentials before normal model/turn persistence. Treat imported skill text, README links, attachments, and tool output as untrusted data.
4. Use channel-session export/reset and run deletion for operator-directed export/delete. Backups follow their own documented retention and must be deleted separately.
5. Review `audit_events` for skill activation, approval decisions, outbox replay, and managed-attachment storage/access. Audit metadata must contain identifiers/digests, not attachment contents or secrets.

Storage-at-rest encryption, object-lock retention, and legal deletion requirements are deployment responsibilities; Norbot cannot verify them from application configuration.

## Local forensic exception

When `forensics.raw_capture` is enabled, Norbot retains raw trace payloads encrypted with the local `forensics.master_key_env` key for `forensic_payload_days`. This mode is rejected for public or OIDC deployments and requires a loopback listener. Raw data is not indexed or logged and each reveal/export is audited. See [Operator trace and forensics](OBSERVABILITY.md).
