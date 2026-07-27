# Operations

## Backup and recovery

`scripts/backup-postgres.sh` creates an encrypted, checksummed logical PostgreSQL backup. Run it from a scheduler outside the Norbot container with `NORBOT_BACKUP_DIR`, `NORBOT_BACKUP_PASSPHRASE`, and a retention period. Store the directory on encrypted, independent storage; do not keep the passphrase beside the backup.

`scripts/restore-postgres.sh BACKUP.dump.enc` verifies the checksum, requires an explicit `RESTORE`, then replaces database objects. Test it against an isolated Compose project at least quarterly and record the measured recovery time.

`NORBOT_RESTORE_DRILL=1 scripts/restore-drill-postgres.sh BACKUP.dump.enc` performs that isolated restore in an ephemeral PostgreSQL container and asserts that the migration ledger restored. Schedule it from the same independent-backup context; its pass/fail record is the RTO evidence. A production implementation still needs a tested WAL/base-backup pipeline for its selected RPO.

Logical backups are a portable recovery baseline, not point-in-time recovery. For production, use `docker compose -f docker-compose.yml -f docker-compose.production.yml up -d`; it builds the PostgreSQL archive image, enables authenticated encrypted WAL archiving to `NORBOT_WAL_ARCHIVE_HOST_DIR`, and refuses startup without its passphrase. The backup host requires GnuPG. Run `scripts/backup-postgres-pitr.sh` on the chosen base-backup cadence, transfer both encrypted base backups and WAL archive to independent versioned storage, and run `NORBOT_RESTORE_DRILL=1 scripts/restore-drill-pitr.sh BASE.tar.enc` on schedule. A scheduler must retain WAL from before each retained base backup through its declared RPO window. Record base-backup completion, WAL transfer completion, restore-drill result, measured RTO, and oldest recoverable timestamp; alert on any missed objective. PostgreSQL documents the required relationship between a base backup and retained WAL for PITR: <https://www.postgresql.org/docs/current/continuous-archiving.html>.

[`deploy/cron/norbot-pitr.cron.example`](../deploy/cron/norbot-pitr.cron.example) supplies the base-backup schedule and a deliberately explicit restore-drill invocation. The scheduler owner must select and record the backup under test; it must not restore an arbitrary glob.

## Public profile

Set `security.public: true` only behind a TLS proxy. It requires complete OIDC, `security.http.require_https: true`, rate limits, and a metrics-token environment reference. The proxy must forward `X-Forwarded-Proto: https`; Norbot's Compose port remains loopback-only.

`/metrics` is token-protected in public mode; configure the scraper with `X-Norbot-Metrics-Token`. Load [`deploy/alerts/norbot.yaml`](../deploy/alerts/norbot.yaml) into Prometheus or an equivalent alert manager. Durable gauges cover queue/outbox state, approvals, workspace recovery, failed deployments, provider errors/429s, and 24-hour token volume; cache access is a process metric. Token volume is not a currency cost: configure provider-specific pricing/budget alerts in the metrics backend. Treat a dead-letter replay as an audited operator action, not an automatic recovery mechanism.

## Kubernetes

`norbot kube bootstrap` creates the managed `ResourceQuota` and `LimitRange`. Restrict edit/delete of those two objects to a separate cluster-admin role. Before setting `network_policy_enforced: true`, run `norbot kube network-policy-check --timeout=2m` with the same kubeconfig/context/namespace Norbot uses. The check creates a restricted server and client pod, proves direct pod-to-pod HTTP works, applies a deny-egress `NetworkPolicy`, and requires that same request to fail; it deletes the policy and both pods before returning. A failed or timed-out check leaves sandbox HTTPS tools fail-closed. Run it after every CNI or policy-engine upgrade; the check is a live cluster operation and needs pod, pod-exec, and NetworkPolicy create/delete rights in the namespace.

## Supply chain

Import skills at immutable Git revisions. Norbot records the resolved commit and source tree digest, quarantines imports until a named operator activates them, and only exposes reviewed imports to runs. A source repository, README link, and imported `SKILL.md` are untrusted data; they do not grant tools or alter the configured tool policy.

For releases, retain an SPDX SBOM, image digest, provenance attestation, and Cosign verification record. The tag workflow publishes a digest, GitHub attestation, and keyless signature. Dockerfile, Compose, CI service, and live-test images use immutable `@sha256:` references. Deploy immutable `@sha256:` references only, and run `scripts/verify-image-signature.sh IMAGE@sha256:DIGEST` before admitting a control-plane or generated-app image. Pull requests run dependency review; protected branches must require that check. See [GitHub dependency review](https://docs.github.com/en/code-security/concepts/supply-chain-security/dependency-review) and [Cosign verification](https://docs.sigstore.dev/cosign/verifying/verify/).

## Data lifecycle

`retention` defines the maximum age for agent turns, channel messages, run events, and provider usage. Set a field to `0` only when a documented legal/operational retention requirement requires preservation; all other values are purged hourly. Agent turns with pending approvals are retained until the decision expires. Prompts and attachment metadata are redacted for common bearer/API-key/secret patterns before model and turn persistence. Channel-session export/reset and run deletion are the supported operator export/delete paths. Managed attachment storage/access creates immutable audit events without recording contents. Do not put secrets in prompts, channel messages, skill text, logs, or attachments.

## Verification caches

Verification caches are lockfile/toolchain keyed Docker volumes and report hit/miss counters through `/metrics`. `scripts/prune-verification-caches.sh` is dry-run by default; use `--apply` only after reviewing its output. It removes only aged `norbot_verify_node_*` and `norbot_verify_go_*` volumes that Docker reports as unused.
