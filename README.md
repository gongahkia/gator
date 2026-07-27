# Norbot

## Operator traceback

Expanded runs expose a unified trace across stages, providers, revisions, approvals, policy changes, central-agent turns, tool actions, and sandboxes. See [docs/OBSERVABILITY.md](docs/OBSERVABILITY.md) for console/API usage and the local-only encrypted forensic mode.

Norbot is a local-first, provider-agnostic agentic app builder. Its embedded web console drives explicit Plan → Build → Verify/Fix → Deploy approvals; Go API and workers own durable state, generated artifacts, Docker or Kubernetes workspaces, deployments, and OpenTelemetry visibility.

## Install

```sh
cp .env.example .env
cp config.example.json config.json
docker compose up --build
```

Open `http://127.0.0.1:8080`. The default Compose setup is loopback-only and allows local unauthenticated access. For a shared deployment, configure `security.oidc` with an issuer, audience, groups claim, operator group, SPA `client_id`, and optional `scopes`, then set `NORBOT_ALLOW_UNAUTHENTICATED_LOCAL=false`.

Existing local installs with the former placeholder `security.oidc` block must replace it with `"security": {}` before starting; configure a complete OIDC block instead for shared access.

`norbot init` creates a config interactively. It records a Docker or Kubernetes default; every run can override that default and permanently pins its selected backend.

The example config disables remote artifact storage and managed sandbox HTTP writes by default, so the first Docker Compose boot needs only a provider key. Enable either deliberately after bootstrap.

## Reviews, skills, and health

Builder responses are stored as immutable, digest-checked code/fix proposals. Code approval applies a proposal, deterministic test output becomes a second approval gate, and failed tests require an explicit `fix` action; `workflow.max_fixes` defaults to `2`. `GET /api/runs/{id}/revisions`, `/usage`, and `/skills` expose patch/report, reported-or-estimated token usage, and selected-skill provenance.

`workflow.planning_swarm` is an opt-in planning-only experiment. It runs bounded remote architecture/delivery/security candidates without Norbot tools or writable workspaces, ranks valid candidates with the configured planner provider, and still requires the ordinary human architecture approval. CLI and plugin planners stay single-agent because that isolation boundary cannot be enforced for them. See [configuration](docs/CONFIGURATION.md#planning-swarm).

Each completed app version can create a change run. Norbot snapshots the approved `generated-app` tree under its SHA-256 digest, records that immutable snapshot, copies the parent run’s exact selected skill digests, and restores the snapshot into the child workspace. A normal change starts directly at Build; only `architecture_affecting: true` permits the Planner stage. The child has a new version run ID but inherits the stable `app_id`, so a successful deployment updates the same Docker Compose project or Kubernetes application identity. Failed candidate deployments do not replace the recorded current app version.

Skill imports accept HTTPS Git and OCI sources. Norbot recursively discovers every `SKILL.md` (up to 64 per source) and creates one independently scanned import for each directory. In `auto` mode, a directory with `skill.json` is imported as a native declarative bundle; any other `SKILL.md` is imported as an adapted read-only instruction bundle, with its identity and metadata derived from frontmatter and source path. `native` mode requires `skill.json`; `adapted` mode deliberately ignores manifests and derives metadata for every discovered skill.

Each candidate is independently limited to 128 regular non-executable files, 1 MiB per file, and 10 MiB total. Imports are copied into Norbot-managed storage by SHA-256 digest, stay `scanned` until explicitly activated, and only activated digests can be selected when creating a run. Adapted imports declare no executable tools or capabilities: their source files are available as read-only instructions only. Private source credentials are environment-variable references only.

For catalogue repositories, `follow_readme_links: true` reads the root `README.md`, extracts up to 32 canonical public GitHub repository links, and scans each linked repository independently. Credentials are never sent to linked repositories; invalid, unsafe, or non-skill links are reported as skipped.

`norbot health` prints detailed health and returns nonzero only for a critical down dependency; `norbot health --json`, `GET /api/health/detail`, `/api/health/stream`, and the web console provide the same redacted diagnostics.

## Channels

Norbot is the central HTTPS gateway for Telegram, Slack HTTP Events, Discord signed interactions plus Gateway messages, and official WhatsApp Cloud API. Channel accounts bind to one deployed agentic app, persist only environment-variable secret references, and require explicit pairing before an identity can invoke the app. Sessions retain summaries for 30 days and can be exported or reset through the API.

Create an account through `POST /api/channels/accounts`, then pair an external platform identity through `POST /api/channels/accounts/{id}/pairings`. Configure each provider webhook to `https://<public-host>/api/channels/<account-id>/webhook`; Telegram needs `webhook_secret` and `bot_token`, Slack `signing_secret` and `bot_token`, Discord `public_key` plus `bot_token`, and WhatsApp `verify_token`, `app_secret`, and `access_token`. The account `settings` needs `public_key` for Discord and `phone_number_id` for WhatsApp.

For Docker, set `NORBOT_PUBLIC_HTTPS_DOMAIN` and run `docker compose --profile public up --build`; the included Caddy reverse proxy obtains TLS for a publicly resolvable DNS name. Kubernetes-generated apps retain their existing ingress path; the Norbot gateway itself must be deployed behind a public TLS reverse proxy reachable by the platform webhooks.

## Kubernetes

Kubernetes is additive; Docker Compose remains fully supported. Kubernetes runs use kubeconfig/client-go, a per-run PVC, isolated Jobs, Kaniko builds to an existing OCI registry Secret, temporary live verification, and retained application/PVC resources until deletion.

For a local Kubernetes option, `kind` runs the cluster nodes as Docker containers on this Mac. It is not a replacement for Docker Compose: Docker Compose still runs the Norbot control plane, while `kind` runs the generated apps, verifier, sandbox Jobs, and egress proxy. The bootstrap creates an in-cluster OCI registry, namespace, ServiceAccount/RBAC, registry Secret, signed egress-proxy Deployment/Service, NetworkPolicies, a Compose-ready Kubernetes config, and a local 0600 proxy-secret file.

```sh
brew install kind kubectl
brew install cilium-cli # enables safe Kubernetes HTTP sandbox tools
norbot kube local --cilium
set -a; source .norbot/local-kubernetes.env; set +a
NORBOT_CONFIG_HOST=config.local-kubernetes.json \
NORBOT_KUBECONFIG_HOST="$HOME/.kube/config" docker compose up --build
```

`kind`’s default networking does not by itself prove NetworkPolicy enforcement. `--cilium` creates a new Kind cluster with the default CNI disabled, installs Cilium, then runs a direct pod-to-pod deny-egress probe before enabling HTTPS sandbox tools. For another CNI, use `norbot kube local --verify-network-policy`; the legacy `--confirm-network-policy` alias now runs the same probe. Without a passing probe, local bootstrap leaves those tools fail-closed. Recheck after every CNI upgrade or policy-engine change. Docker deployment and non-network sandbox tools remain available.

For a remote cluster, use `norbot init --target kubernetes`, create the registry Secret, then `norbot kube bootstrap`. The operator needs namespace-creation access for bootstrap, then namespaced access only to Norbot resources. Set `runtime.kubernetes.registry_repository` and `registry_pull_secret`; optional ingress is disabled unless its class, base domain, and controller namespace are all configured.

## Artifact storage and managed egress

Set `artifacts.enabled` to `true` with an HTTPS S3 or S3-compatible endpoint, bucket, and environment-variable credential references to enable durable channel input/output and agent file artifacts. Norbot transfers objects with the AWS Go v2 S3 client, retains a digest and 30-day expiry record in Postgres, verifies downloads before upload, and removes the remote object before its metadata during expiry cleanup. The default remains local files only.

For Docker HTTP-write tools, set `runtime.sandbox.egress_proxy_url` to `http://host.docker.internal:8181` and `egress_proxy_secret_env` to `NORBOT_EGRESS_PROXY_SECRET`; Compose publishes that loopback-only listener. Kubernetes uses a separate namespace-local proxy Deployment and Service. Sandboxes can connect only to DNS and that Service; the proxy checks the signed host allowlist, permits HTTPS/443 only, resolves and dials public IPs only, and is the sole workload with public HTTPS egress.

For remote access, tunnel the embedded web console:

```sh
ssh -L 8080:127.0.0.1:8080 host
```

Then open `http://127.0.0.1:8080` locally.

## Operating model

- One local/self-hosted operator. No cloud control plane, tenancy, or Azure dependency.
- Every stage pauses for explicit approval. Failures and expired worker leases pause for retry, revision, or abandonment; Norbot never auto-replays a recovered job.
- Per-stage providers and deployment target are chosen at run creation and recorded with each event; later config changes do not alter an existing run.
- Provider credentials are environment references only; Norbot never stores raw secrets.
- Norbot owns agent sessions, provider credentials, typed tools, approval audit, sandbox execution, and idempotency. Generated apps receive neither model credentials nor a local tool executor. OpenClaw is not used.
- Verification blocks deployment on locked dependency checks, tests, builds, npm audit, govulncheck, Docker Compose or Kubernetes rollout, or smoke failure.

Configure provider API keys in `.env`; add CLI providers with isolated runner images in `config.json`. See [configuration](docs/CONFIGURATION.md).

## API

- `POST /api/runs` creates and queues a planner run. It accepts optional `deployment_target: "docker"|"kubernetes"` and `public_ingress` fields.
- `GET /api/runs`, `GET /api/runs/{id}`, `GET /api/runs/{id}/events` inspect state and stream replayable events.
- `GET/PUT /api/runs/{id}/architecture` reads or edits the typed planner architecture while approval is pending. `PUT /api/runs/{id}/graph` remains a compatibility projection.
- `GET /api/runs/{id}/planning-swarm` exposes durable planning candidates; `POST /api/runs/{id}/planning-swarm/select` selects a completed candidate for review without bypassing planner approval.
- `POST /api/runs/{id}/change-runs` creates a snapshot-backed linked version. Send `{"change":"...","architecture_affecting":true}` only when the architecture must be replanned; otherwise it starts at Build. `GET /api/apps` lists the current deployed version of each app.
- `POST /api/runs/{id}/approval` approves, revises, explicitly starts a fix, retries, or abandons a run.
- `GET /api/health`, `/api/health/detail`, `/api/health/stream`, `/metrics`, and `/api/capacity` expose operations and quota-aware worker recommendations. `POST /api/capacity/recommendations` persists a recommendation; `POST /api/capacity/recommendations/{id}/accept` records explicit confirmation.
- `POST /api/skills/imports`, `GET /api/skills/imports`, and `POST /api/skills/imports/{id}/activate` operate the scanned native/adapted skill catalog.
- `POST/GET /api/channels/accounts`, pairing/session routes, and `/api/channels/{account}/webhook` operate the central native channel gateway.
- `GET /api/agent/actions` and `POST /api/agent/actions/{id}/decision` expose central, durable tool approvals; a decision resumes its exact persisted turn once.
- `GET/PUT /api/runs/{id}/agent-policy` expose each run's internal-agent and agentic-app capability policy. PUT is monotonic: it can only restrict the run; pending tool actions are revalidated before execution.
- `GET /api/runtime` returns the default target and Kubernetes/ingress availability for web-console onboarding.
- `GET /api/runs/{id}/deployment`, `/logs`; `POST .../start`, `POST .../stop`; and `DELETE .../deployment` control deployed apps.

`/metrics` is a Prometheus scrape endpoint; traces export through the OpenTelemetry Collector to Jaeger.

`docker compose` remains the supported Norbot installation path. Docker runs require Docker; Kubernetes runs require only kubeconfig access from the Norbot control plane.

For the required human inbound proof on dedicated Telegram, Slack, Discord, and WhatsApp identities, send a unique marker and run `norbot live-e2e inbound --account <account-id> --external <identity-id> --marker <marker>`. It succeeds only after Norbot records that inbound message and a delivered outbound reply.
