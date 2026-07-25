# Norbot

Norbot is a local-first, provider-agnostic agentic app builder. A Bubble Tea TUI drives explicit Planner → Builder → Verifier → Deployer approvals; Go API and workers own durable state, generated artifacts, Docker or Kubernetes workspaces, deployments, and OpenTelemetry visibility.

## Install

```sh
cp .env.example .env
cp config.example.json config.json
docker compose up --build
docker compose exec norbot norbot tui --api http://127.0.0.1:8080
```

`norbot init` creates a config interactively. It records a Docker or Kubernetes default; every run can override that default and permanently pins its selected backend.

## Kubernetes

Kubernetes is additive; Docker Compose remains fully supported. Kubernetes runs use kubeconfig/client-go, a per-run PVC, isolated Jobs, Kaniko builds to an existing OCI registry Secret, temporary live verification, and retained application/PVC resources until deletion.

```sh
norbot init --target kubernetes
norbot kube secret-template > registry-pull.yaml # add registry credentials before applying
norbot kube bootstrap
```

The operator must provide a kubeconfig with namespace creation access for bootstrap, then namespaced access only to Norbot resources. Set `runtime.kubernetes.registry_repository` and `registry_pull_secret`; optional ingress is disabled unless its class, base domain, and controller namespace are all configured. The generated workload defaults to one replica, ClusterIP service, restricted security context, and DNS/TCP-443 egress policy.

For remote use, bind the API to loopback and run the TUI through SSH:

```sh
ssh -L 8080:127.0.0.1:8080 host
norbot tui --api http://127.0.0.1:8080
```

## Operating model

- One local/self-hosted operator. No cloud control plane, tenancy, or Azure dependency.
- Every stage pauses for explicit approval. Failures and expired worker leases pause for retry, revision, or abandonment; Norbot never auto-replays a recovered job.
- Per-stage providers and deployment target are chosen at run creation and recorded with each event; later config changes do not alter an existing run.
- Provider credentials are environment references only; Norbot never stores raw secrets.
- Agentic generated apps contain an app-local SQLite typed-tool executor, audit log, and parameter-bound approval API. OpenClaw is not used.
- Verification blocks deployment on locked dependency checks, tests, builds, npm audit, govulncheck, Docker Compose or Kubernetes rollout, or smoke failure.

Configure provider API keys in `.env`; add CLI providers with isolated runner images in `config.json`. See [configuration](docs/CONFIGURATION.md).

## API

- `POST /api/runs` creates and queues a planner run. It accepts optional `deployment_target: "docker"|"kubernetes"` and `public_ingress` fields.
- `GET /api/runs`, `GET /api/runs/{id}`, `GET /api/runs/{id}/events` inspect state and stream replayable events.
- `PUT /api/runs/{id}/graph` edits a planner graph only while it awaits planner approval.
- `POST /api/runs/{id}/approval` approves, revises, retries, or abandons a run.
- `GET /api/health`, `/metrics`, and `/api/capacity` expose operations and quota-aware worker recommendations. `POST /api/capacity/recommendations` persists a recommendation; `POST /api/capacity/recommendations/{id}/accept` records explicit confirmation.
- `GET /api/runtime` returns the default target and Kubernetes/ingress availability for onboarding and TUI target selection.
- `GET /api/runs/{id}/deployment`, `/logs`; `POST .../start`, `POST .../stop`; and `DELETE .../deployment` control deployed apps.

`/metrics` is a Prometheus scrape endpoint; traces export through the OpenTelemetry Collector to Jaeger.

`docker compose` remains the supported Norbot installation path. Docker runs require Docker; Kubernetes runs require only kubeconfig access from the Norbot control plane.
