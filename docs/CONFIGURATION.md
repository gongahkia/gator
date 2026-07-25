# Configuration

Norbot reads `config.json`; keep the file outside source control. The supplied `config.example.json` is the complete schema for v1.

## Provider adapters

`kind` can be `openai_responses`, `anthropic_messages`, `gemini_generate_content`, `openai_compatible`, or `cli`.

- API adapters require `model`, `base_url`, and `credential_env`.
- CLI adapters require `image`, `command`, and optionally `credential_env`/`network`. Norbot runs them in a short-lived stage container with the per-run workspace volume mounted at `/workspace`.
- `stages` is an allowlist. The TUI only offers compatible adapters for each stage.

Build an image for local Pi, OpenCode, Claude Code, or Codex with the CLI on `PATH`, then declare it as a `cli` provider. Credentials are injected only from the selected environment variable into that stage container. Do not put secrets in this file, artifacts, or generated apps.

## Extension API

`internal/extension` exposes versioned ProviderAdapter, Tool, and Profile interfaces. Link custom Go packages into a Norbot build and register them during application setup. Standard deployment configuration remains declarative in `config.json`; behavior that needs code uses these interfaces.

## Remote operation

Keep `NORBOT_HTTP_ADDR=127.0.0.1:8080` on a host. Connect the local TUI through an SSH tunnel. Docker, provider credentials, workspaces, deployment ports, Postgres, and artifacts remain on the host.
