# Operations

## Backup and recovery

`scripts/backup-postgres.sh` creates an encrypted, checksummed logical PostgreSQL backup. Run it from a scheduler outside the Norbot container with `NORBOT_BACKUP_DIR`, `NORBOT_BACKUP_PASSPHRASE`, and a retention period. Store the directory on encrypted, independent storage; do not keep the passphrase beside the backup.

`scripts/restore-postgres.sh BACKUP.dump.enc` verifies the checksum, requires an explicit `RESTORE`, then replaces database objects. Test it against an isolated Compose project at least quarterly and record the measured recovery time.

Logical backups are a portable recovery baseline, not point-in-time recovery. A non-disposable production deployment must also archive PostgreSQL WAL and take base backups to independent object storage. Set and test an explicit RPO/RTO before public use. PostgreSQL documents the required relationship between a base backup and retained WAL for PITR: <https://www.postgresql.org/docs/current/continuous-archiving.html>.

## Public profile

Set `security.public: true` only behind a TLS proxy. It requires complete OIDC, `security.http.require_https: true`, rate limits, and a metrics-token environment reference. The proxy must forward `X-Forwarded-Proto: https`; Norbot's Compose port remains loopback-only.

`/metrics` is token-protected in public mode; configure the scraper with `X-Norbot-Metrics-Token`. Load [`deploy/alerts/norbot.yaml`](../deploy/alerts/norbot.yaml) into Prometheus or an equivalent alert manager. These durable gauges cover queued/running/dead outbox events, pending approvals, workspace failures, and queue depth. Treat a dead-letter replay as an audited operator action, not an automatic recovery mechanism.

## Kubernetes

`norbot kube bootstrap` creates the managed `ResourceQuota` and `LimitRange`. Restrict edit/delete of those two objects to a separate cluster-admin role. `network_policy_enforced: true` is an operator attestation; run a deny/allow probe against the actual CNI before enabling managed sandbox egress.

## Supply chain

Import skills at immutable Git revisions. Norbot records the resolved commit and source tree digest, quarantines imports until a named operator activates them, and only exposes reviewed imports to runs. A source repository, README link, and imported `SKILL.md` are untrusted data; they do not grant tools or alter the configured tool policy.

For releases, retain an SPDX SBOM, image digest, provenance attestation, and Cosign verification record. Pull requests run dependency review; protected branches must require that check. See [GitHub dependency review](https://docs.github.com/en/code-security/concepts/supply-chain-security/dependency-review) and [Cosign verification](https://docs.sigstore.dev/cosign/verifying/verify/).

## Data lifecycle

`retention` defines the maximum age for agent turns, channel messages, run events, and provider usage. Set a field to `0` only when a documented legal/operational retention requirement requires preservation; all other values are purged hourly. Agent turns with pending approvals are retained until the decision expires. Prompts and attachment metadata are redacted for common bearer/API-key/secret patterns before model and turn persistence. Channel-session export/reset and run deletion are the supported operator export/delete paths. Do not put secrets in prompts, channel messages, skill text, logs, or attachments.
