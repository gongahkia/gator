# Configuration

Norbot reads `config.json`; keep the file outside source control. The supplied `config.example.json` is the complete schema for v1.

## Provider adapters

`kind` can be `openai_responses`, `azure_openai_responses`, `anthropic_messages`, `gemini_generate_content`, `vertex_ai_generate_content`, `cohere_v2_chat`, `ollama_chat`, `aws_bedrock_converse`, `openai_compatible`, `cli`, or `plugin`. See [provider configurations](PROVIDERS.md) for required fields, authentication, documented presets, and live-validation procedure.

- Most API adapters require `model`, `base_url`, and `credential_env`; Ollama has no credential field, Bedrock requires `model` and `region` with AWS credentials, and Vertex requires `model`, `base_url`, `project`, and `region` with Application Default Credentials.
- CLI adapters require `image`, `command`, and optionally `credential_env`/`network`. Docker runs inject credentials from the selected local environment variable. Kubernetes CLI runs require `kubernetes_secret` and `kubernetes_secret_key`, referring to an existing Secret in Norbot's namespace.
- `stages` is an allowlist. The web console only offers compatible adapters for each stage.
- `budget.max_concurrent` and `budget.requests_per_minute` constrain capacity recommendations. API rate-limit response headers are persisted when available; a recommendation must be explicitly accepted through the API before it is recorded as operator-approved.

Build an image for local Pi, OpenCode, Claude Code, or Codex with the CLI on `PATH`, then declare it as a `cli` provider. Credentials are injected only from the selected environment variable into that stage container. Do not put secrets in this file, artifacts, or generated apps.

## Planning swarm

`workflow.planning_swarm` is disabled by default. When enabled, only the Planner stage fans out to up to three remote API candidates (`architecture`, `delivery`, and `security`) using the run's already-selected planner provider and model. Norbot then uses that same provider to rank valid candidates and submits the selected architecture to the existing human approval gate.

CLI and process-plugin providers intentionally use the ordinary single-planner path: Norbot cannot impose the same no-tool, no-network, read-only boundary on those adapters. Candidates receive no Norbot tool permissions, extensions, or writable workspace. A swarm needs at least two schema-valid candidates; otherwise it records a degraded execution and falls back to the normal planner. `max_parallel` is `1..3` (default `3`), and `timeout_seconds` is `30..600` (default `180`).

Each attempt persists prompt/config/output digests, candidate roles, architectures, ranking reasons, provider/model, failures, and automatic or operator candidate selection. The console exposes candidate selection while planner approval is pending; selection replaces the proposed architecture but never approves it.

## Process plugins

Plugins are local absolute executable paths, pinned by SHA-256. Norbot verifies the digest at startup and before every call, then exchanges one JSON-RPC 2.0 request/response through stdin/stdout. A plugin must allow and implement `norbot.initialize`, returning API version `v1` plus provider, tool, and profile capabilities. Provider capabilities can be selected with `kind: "plugin"` and `plugin_id`.

Go shared-library plugins are intentionally not used: process isolation and digest verification avoid toolchain and dependency coupling.

## Generated agent apps

The `agentic` profile is controlled by Norbot, not the generated app. Norbot holds provider credentials and durable turn/action state, validates the manifest tool policy, and records immutable digests and decisions. `artifact_read` and allowlisted HTTPS `http_get` can run for scoped roles; HTTP writes, database mutation, file writes, and shell actions require explicit operator approval. Approval or rejection resumes the exact persisted turn once. Shell runs with no network; approved HTTP writes use the signed managed egress proxy. `database_mutate` is limited to parameterized mutations of each run's `app_<run-id>.agent_state` table.

At creation, each run stores a digest-checked capability policy for Planner, Builder, Verifier, Deployer, and (for agentic apps) every declared tool. The expanded run view and `GET /api/runs/{id}/agent-policy` show it. `PUT /api/runs/{id}/agent-policy` accepts only a stricter version: stage/model/network/local-runtime switches can be turned off; tools can be disabled; approval can be required; roles, HTTPS hosts, shell commands, and safe path prefixes can be narrowed; and `max_calls` can be lowered. It cannot restore any capability, remove an approval requirement, add an allowlist item, or increase a limit. Pending actions are validated against the latest policy immediately before execution. `http_get` is an HTTPS fetch tool, not a general web-search tool.

`tool_policy` is the creation-time ceiling. `allowed_path_prefixes` and `max_calls` are optional but should be explicit. Empty path prefixes for `artifact_read`/`file_write` default to the safe run paths `agent-input/`, `agent-output/`, `generated-app/`, and `stage-output/`; empty host/command lists permit no network destination/command. CLI agents mount the workspace read-only outside Builder; empty CLI `network` defaults to `none`. Kubernetes refuses a requested CLI network denial unless `network_policy_enforced` has passed the live check.

Public deployments require `security.oidc`: issuer, audience, groups claim, and operator groups. Only signed platform webhooks and `/api/health` bypass OIDC. Configure a reachable managed egress proxy URL plus its shared-secret environment variable before enabling HTTP-write tools. Docker Compose starts the signed CONNECT proxy on `NORBOT_EGRESS_PROXY_ADDR` (or the configured URL port) and publishes port 8181 only to loopback. Kubernetes creates a dedicated namespace-local proxy Deployment, Service, and NetworkPolicy from `runtime.kubernetes.egress_proxy_*`; its Secret is separate from the control-plane environment reference and must contain the configured key.

The proxy accepts only signed HTTPS CONNECT requests on port 443. It resolves approved hostnames itself, rejects literal/private/link-local/multicast addresses, and dials the resolved public address to prevent DNS rebinding. Kubernetes sandbox Jobs receive a deny-ingress/egress policy that allows DNS and the proxy Service only. Set `runtime.kubernetes.network_policy_enforced` only after `norbot kube network-policy-check` passes against that cluster; without it, HTTP-write sandbox tools fail closed. Repeat the check after every CNI or policy-engine change.

## Artifact storage

`artifacts.enabled: true` enables S3-compatible artifact transfer. `endpoint`, `bucket`, `access_key_env`, and `secret_key_env` are required; `region` defaults to `us-east-1`, and `force_path_style` supports local S3-compatible services such as MinIO. Credentials are read only from the configured environment variables. Norbot uploads channel attachments and approved agent file outputs, records object key/digest/expiry metadata in Postgres, validates digest on remote download, stages outbound files only for delivery, and deletes remote objects before removing expired metadata. Objects are capped at 10 MiB and expire after 30 days.

## Runtime target

`runtime.default_target` is `docker` or `kubernetes`. A run may override it through `POST /api/runs`; its `deployment_target` is durable and existing runs are migrated as Docker.

`runtime.docker` is server-owned execution transport, never generated-app configuration. Its default is:

```json
{"mode":"rootless_remote_tls","host_env":"DOCKER_HOST","tls_verify_env":"DOCKER_TLS_VERIFY","cert_path_env":"DOCKER_CERT_PATH"}
```

The referenced environment values must be `tcp://...`, `1`, and an absolute certificate path respectively; Norbot then checks Docker `SecurityOptions` for rootless mode before it creates a workspace or deployment. Mount client certificates read-only with `docker-compose.remote-tls.yml`; the base Compose file deliberately does not mount `/var/run/docker.sock`.

`unsafe_local_socket` is a local-development compatibility mode only: `"docker":{"mode":"unsafe_local_socket"}`. It requires `NORBOT_ALLOW_UNSAFE_LOCAL_DOCKER_SOCKET=true`, `docker-compose.unsafe-local.yml`, and local `security.public: false`; Norbot rejects the configuration for public deployments and displays a persistent console warning. It is not a security boundary for generated code.

Kubernetes configuration requires `kubeconfig`, `namespace`, `service_account`, `registry_repository`, and `registry_pull_secret`. Norbot reads the kubeconfig through client-go; it does not require a `kubectl` binary or Docker socket. `norbot kube bootstrap` creates the dedicated namespace, ServiceAccount, Role, RoleBinding, and configured egress-proxy resources. Apply an OCI `kubernetes.io/dockerconfigjson` Secret separately; `norbot kube secret-template` emits a credential-free template. Set `registry_insecure` only for a local HTTP registry; it enables Kaniko’s insecure-registry flags and must not be used for a remote registry.

`norbot kube local` is the supported local bootstrap. It needs Docker, `kind`, and `kubectl`, builds/pushes the local proxy image to `kind-registry`, and writes `config.local-kubernetes.json` plus `.norbot/local-kubernetes.env`. Add `--cilium` on a new cluster to install Cilium and wait for an enforcing CNI before enabling sandbox HTTP tools; this also needs `cilium` in `PATH`. Run Compose with `NORBOT_CONFIG_HOST` and `NORBOT_KUBECONFIG_HOST` as printed by the command.

Each Kubernetes run receives a PVC. Provider CLI, verifier, Kaniko, and smoke Jobs mount it. Generated app images use a run-specific registry tag, workloads default to one replica and ClusterIP, and delete removes retained workload resources plus the PVC. Optional ingress requires all ingress fields and is selected per run. Application NetworkPolicies deny unsolicited ingress and scope same-run traffic; sandbox policies are stricter and route egress exclusively through the proxy.

## Verification and lifecycle

Verifier gates fail closed: locked frontend dependencies, npm test/build/audit, Go test/build/govulncheck, and backend-specific isolated smoke checks must pass before deployment approval. Docker and Kubernetes generate their Dockerfiles from Norbot-owned templates after every builder stage; model-authored Compose, Dockerfile, `.dockerignore`, and `.norbot/` deployment descriptors are rejected or archived outside `generated-app` before execution. Docker uses fixed `docker build`, internal labeled networks, loopback-only frontend publishing, and hardened `docker run` flags; Kubernetes uses temporary Jobs, Kaniko image builds, rollout, and in-cluster smoke before cleanup. Deployment API and web console controls support status, logs, start, stop, and delete. Worker leases expire to `interrupted`; recovery never auto-replays work.

## Remote operation

Keep `NORBOT_HTTP_ADDR=127.0.0.1:8080` on a host and tunnel the web console over SSH when needed. Docker mode keeps workspaces/deployments on the host; Kubernetes mode keeps the Norbot control plane local while run workloads live in the selected cluster.
