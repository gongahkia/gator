# Configuration

Norbot reads `config.json`; keep the file outside source control. The supplied `config.example.json` is the complete schema for v1.

## Provider adapters

`kind` can be `openai_responses`, `anthropic_messages`, `gemini_generate_content`, `openai_compatible`, `cli`, or `plugin`.

- API adapters require `model`, `base_url`, and `credential_env`.
- CLI adapters require `image`, `command`, and optionally `credential_env`/`network`. Docker runs inject credentials from the selected local environment variable. Kubernetes CLI runs require `kubernetes_secret` and `kubernetes_secret_key`, referring to an existing Secret in Norbot's namespace.
- `stages` is an allowlist. The TUI only offers compatible adapters for each stage.
- `budget.max_concurrent` and `budget.requests_per_minute` constrain capacity recommendations. API rate-limit response headers are persisted when available; a recommendation must be explicitly accepted through the API before it is recorded as operator-approved.

Build an image for local Pi, OpenCode, Claude Code, or Codex with the CLI on `PATH`, then declare it as a `cli` provider. Credentials are injected only from the selected environment variable into that stage container. Do not put secrets in this file, artifacts, or generated apps.

## Process plugins

Plugins are local absolute executable paths, pinned by SHA-256. Norbot verifies the digest at startup and before every call, then exchanges one JSON-RPC 2.0 request/response through stdin/stdout. A plugin must allow and implement `norbot.initialize`, returning API version `v1` plus provider, tool, and profile capabilities. Provider capabilities can be selected with `kind: "plugin"` and `plugin_id`.

Go shared-library plugins are intentionally not used: process isolation and digest verification avoid toolchain and dependency coupling.

## Generated agent apps

The `agentic` profile is controlled by Norbot, not the generated app. Norbot holds provider credentials and durable turn/action state, validates the manifest tool policy, and records immutable digests and decisions. `artifact_read` and allowlisted HTTPS `http_get` can run for scoped roles; HTTP writes, database mutation, file writes, and shell actions require explicit operator approval. Approval or rejection resumes the exact persisted turn once. Shell runs with no network; approved HTTP writes use the signed managed egress proxy. `database_mutate` is limited to parameterized mutations of each run's `app_<run-id>.agent_state` table.

Public deployments require `security.oidc`: issuer, audience, groups claim, and operator groups. Only signed platform webhooks and `/api/health` bypass OIDC. Configure a reachable managed egress proxy URL plus its shared-secret environment variable before enabling HTTP-write tools. Docker Compose starts the signed CONNECT proxy on `NORBOT_EGRESS_PROXY_ADDR` (or the configured URL port) and publishes port 8181 only to loopback. Kubernetes creates a dedicated namespace-local proxy Deployment, Service, and NetworkPolicy from `runtime.kubernetes.egress_proxy_*`; its Secret is separate from the control-plane environment reference and must contain the configured key.

The proxy accepts only signed HTTPS CONNECT requests on port 443. It resolves approved hostnames itself, rejects literal/private/link-local/multicast addresses, and dials the resolved public address to prevent DNS rebinding. Kubernetes sandbox Jobs receive a deny-ingress/egress policy that allows DNS and the proxy Service only. Set `runtime.kubernetes.network_policy_enforced` only after confirming that the cluster CNI enforces NetworkPolicy; without it, HTTP-write sandbox tools fail closed.

## Artifact storage

`artifacts.enabled: true` enables S3-compatible artifact transfer. `endpoint`, `bucket`, `access_key_env`, and `secret_key_env` are required; `region` defaults to `us-east-1`, and `force_path_style` supports local S3-compatible services such as MinIO. Credentials are read only from the configured environment variables. Norbot uploads channel attachments and approved agent file outputs, records object key/digest/expiry metadata in Postgres, validates digest on remote download, stages outbound files only for delivery, and deletes remote objects before removing expired metadata. Objects are capped at 10 MiB and expire after 30 days.

## Runtime target

`runtime.default_target` is `docker` or `kubernetes`. A run may override it through `POST /api/runs`; its `deployment_target` is durable and existing runs are migrated as Docker.

Kubernetes configuration requires `kubeconfig`, `namespace`, `service_account`, `registry_repository`, and `registry_pull_secret`. Norbot reads the kubeconfig through client-go; it does not require a `kubectl` binary or Docker socket. `norbot kube bootstrap` creates the dedicated namespace, ServiceAccount, Role, RoleBinding, and configured egress-proxy resources. Apply an OCI `kubernetes.io/dockerconfigjson` Secret separately; `norbot kube secret-template` emits a credential-free template. Set `registry_insecure` only for a local HTTP registry; it enables Kaniko’s insecure-registry flags and must not be used for a remote registry.

`norbot kube local` is the supported local bootstrap. It needs Docker, `kind`, and `kubectl`, builds/pushes the local proxy image to `kind-registry`, and writes `config.local-kubernetes.json` plus `.norbot/local-kubernetes.env`. Add `--cilium` on a new cluster to install Cilium and wait for an enforcing CNI before enabling sandbox HTTP tools; this also needs `cilium` in `PATH`. Run Compose with `NORBOT_CONFIG_HOST` and `NORBOT_KUBECONFIG_HOST` as printed by the command.

Each Kubernetes run receives a PVC. Provider CLI, verifier, Kaniko, and smoke Jobs mount it. Generated app images use a run-specific registry tag, workloads default to one replica and ClusterIP, and delete removes retained workload resources plus the PVC. Optional ingress requires all ingress fields and is selected per run. Application NetworkPolicies deny unsolicited ingress and scope same-run traffic; sandbox policies are stricter and route egress exclusively through the proxy.

## Verification and lifecycle

Verifier gates fail closed: locked frontend dependencies, npm test/build/audit, Go test/build/govulncheck, and backend-specific isolated smoke checks must pass before deployment approval. Docker uses Compose; Kubernetes uses temporary Jobs, Kaniko image builds, rollout, and in-cluster smoke before cleanup. Deployment API and TUI controls support status, logs, start, stop, and delete. Worker leases expire to `interrupted`; recovery never auto-replays work.

## Remote operation

Keep `NORBOT_HTTP_ADDR=127.0.0.1:8080` on a host. Connect the local TUI through an SSH tunnel. Docker mode keeps workspaces/deployments on the host; Kubernetes mode keeps the Norbot control plane local while run workloads live in the selected cluster.
