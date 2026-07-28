# Norbot

Norbot is a local, single-operator agentic app builder. It creates frontend, full-stack, and agentic applications through explicit Plan, Build, Verify/Fix, and Deploy approvals.

## Local setup

Requirements: Docker Desktop, Docker socket access, and an API key for one configured provider.

```sh
cp .env.example .env # add OPENAI_API_KEY
docker compose up --build
open http://127.0.0.1:8080
```

The console and webhook listener bind to loopback only. Docker Desktop socket mode is intentionally local-only: the Norbot container can control Docker on this Mac. Generated deployment descriptors remain server-owned and generated app containers are constrained, but this is not a shared or production deployment mode.

```sh
docker compose exec norbot norbot local doctor
docker compose exec norbot norbot local reset --yes
```

`local reset --yes` removes only Norbot-labelled Docker resources and resets the Norbot database. It does not remove unrelated Docker resources.

## Channels and agent tools

Each channel account has one required `owner_external_id`. Only that identity can invoke the agent or receive operator-queued output; rejected messages are retained in channel logs. Pairing is not used.

Telegram webhook traffic is served only on `127.0.0.1:8081`. Tunnel that port over HTTPS and configure the account’s webhook as:

```text
https://YOUR-TUNNEL/api/channels/ACCOUNT_ID/webhook
```

The console on port 8080 must not be tunneled. Telegram E2E requires a bot token, owner chat ID, and HTTPS tunnel supplied through environment-variable references.

Agent tools are offline by design: `artifact_read`, approval-required `file_write`, and approval-required allowlisted `shell`. Shell actions run in a read-only, capability-dropped sandbox with `--network none`; HTTP and database-mutation tools are unavailable.

## Local proof suite

[`proofs/local-v1.json`](proofs/local-v1.json) defines nine real-provider runs: three frontend, three full-stack, and three agentic. Run the first six from the console, approving each stage after review. For each agentic case, attach the Telegram account sequentially, send the specified owner message, approve the pending action, and record the run ID and result in the proof report.

The suite is a pass only when each run reaches deployed/completed status and its declared browser/API/tool evidence is present. Provider or Telegram setup failures are reported as failures, never converted to a pass.
