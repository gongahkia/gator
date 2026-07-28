# Configuration

Norbot reads `config.json` and environment-variable references only; credentials are never stored in run data or generated applications.

Required local settings:

- one or more providers with stages `planner`, `builder`, and `verifier`;
- `runtime.default_target: "docker"`;
- `runtime.docker.mode: "unsafe_local_socket"`;
- an offline sandbox configuration with no egress proxy;
- `NORBOT_ALLOW_UNSAFE_LOCAL_DOCKER_SOCKET=true`.

Allowed agent tools are `artifact_read`, `file_write`, and `shell`. `file_write` and `shell` must require approval. HTTP tools, database mutation, remote artifacts, OIDC, public mode, Kubernetes, planning swarm, and forensic capture are rejected at startup.

Docker socket mode is a local, one-user boundary only. The Compose file publishes the console and webhook listener exclusively on `127.0.0.1`; do not expose the console through a tunnel.
