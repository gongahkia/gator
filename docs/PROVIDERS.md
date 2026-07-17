# Provider support matrix

Gator is a native-first meta-harness: the provider owns authentication, model choice, sessions, tool loop, compaction, and sandboxing. Gator owns editor UX, context provenance, lifecycle evidence, and permission narrowing.

| Provider | CLI | Verification | Notes |
| --- | --- | --- | --- |
| Aider | `aider` | protected authenticated E2E | native session integration where advertised |
| Amp | `amp` | protected authenticated E2E | native thread integration where advertised |
| Cline | `cline` | protected authenticated E2E | ACP/stream capability depends on CLI version |
| Cursor Agent | `cursor` | protected authenticated E2E | native session capability depends on CLI version |
| Codex | `codex` | protected live check | provider-owned app-server/session behavior |
| Claude Code | `claude` | protected live check | provider-owned authentication and sessions |
| Droid | `droid` | protected authenticated E2E | list/delete are unavailable when not advertised |
| Gemini CLI | `gemini` | protected authenticated E2E | CLI capability varies by installed version |
| Goose | `goose` | protected authenticated E2E | native session capability depends on CLI version |
| Kimi Code CLI | `kimi` | protected probe only | no authenticated E2E workflow yet |
| Mistral Vibe | `vibe` | protected probe only | no authenticated E2E workflow yet |
| Copilot CLI | `copilot` | protected authenticated E2E | native CLI login remains provider-owned |
| OpenCode | `opencode` | protected authenticated E2E | ACP capability depends on CLI version |
| Pi | `pi` | protected authenticated E2E | RPC capability depends on CLI version |

`:GatorHealth` reports local executable, Git workspace, policy, and telemetry state. It does not claim provider readiness until a bounded provider probe succeeds.
