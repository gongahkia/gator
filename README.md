# Norbot

Norbot is a local-first, provider-agnostic agentic app builder. A Bubble Tea TUI drives explicit Planner → Builder → Verifier → Deployer approvals; Go API and workers own durable state, isolated Docker workspaces, generated artifacts, local deployments, and OpenTelemetry visibility.

## Install

```sh
cp .env.example .env
cp config.example.json config.json
docker compose up --build
docker compose exec norbot norbot tui --api http://127.0.0.1:8080
```

For remote use, bind the API to loopback and run the TUI through SSH:

```sh
ssh -L 8080:127.0.0.1:8080 host
norbot tui --api http://127.0.0.1:8080
```

## Operating model

- One local/self-hosted operator. No cloud control plane, tenancy, or Azure dependency.
- Every stage pauses for explicit approval. Failures and expired worker leases pause for retry, revision, or abandonment; Norbot never auto-replays a recovered job.
- Per-stage providers are chosen at run creation and recorded with each event.
- Provider credentials are environment references only; Norbot never stores raw secrets.
- Agentic generated apps contain an app-local SQLite typed-tool executor, audit log, and parameter-bound approval API. OpenClaw is not used.
- Verification blocks deployment on locked dependency checks, tests, builds, npm audit, govulncheck, Compose health, or smoke failure.

Configure provider API keys in `.env`; add CLI providers with isolated runner images in `config.json`. See [configuration](docs/CONFIGURATION.md).

## API

- `POST /api/runs` creates and queues a planner run.
- `GET /api/runs`, `GET /api/runs/{id}`, `GET /api/runs/{id}/events` inspect state and stream replayable events.
- `PUT /api/runs/{id}/graph` edits a planner graph only while it awaits planner approval.
- `POST /api/runs/{id}/approval` approves, revises, retries, or abandons a run.
- `GET /api/health`, `/metrics`, and `/api/capacity` expose operations and quota-aware worker recommendations. `POST /api/capacity/recommendations` persists a recommendation; `POST /api/capacity/recommendations/{id}/accept` records explicit confirmation.
- `GET /api/runs/{id}/deployment`, `/logs`; `POST .../start`, `POST .../stop`; and `DELETE .../deployment` control deployed apps.

`/metrics` is a Prometheus scrape endpoint; traces export through the OpenTelemetry Collector to Jaeger.

`docker compose` is the supported installation path. Docker must be available to the Norbot service for workspace and generated-app deployment lifecycle operations.
