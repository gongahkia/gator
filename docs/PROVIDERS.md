# Provider support matrix

Gator is a native-first meta-harness: providers own authentication, model selection, sessions, tool loops, compaction, and sandboxing. Gator owns editor UX, context provenance, lifecycle evidence, and permission narrowing. The health check runs each listed version/capability probe with a three-second bound; it only reports ready when that probe and a non-interactive auth probe both pass.

“Fixture-tested” names the exact CLI version exercised by checked-in adapter tests. A range is enforced only where stated. Capability lists are probes, not promises: an operation is unavailable unless the installed CLI advertises it.

| Provider | Executable | Fixture-tested version | Auth probe | Advertised capabilities | Explicit limitations | Verification tier |
| --- | --- | --- | --- | --- | --- | --- |
| Aider | `aider` | 0.77.1 | unavailable: no provider-independent status contract | CLI, message, stream, ask/architect, history | no ACP or structured-output claim; auth cannot be checked | fixture + local authenticated E2E |
| Amp | `amp` | 1.2.3 | unavailable: no non-interactive status contract | execute, JSON stream/input, threads, MCP, plugins | no ACP claim; auth cannot be checked | fixture + local authenticated E2E |
| Cline | `cline` | 1.3.0 | unavailable: no provider-independent status contract | ACP/stdio, JSON, create/resume, plan, MCP, embedded context | ACP profile varies by CLI; auth cannot be checked | fixture + local authenticated E2E |
| Cursor Agent | `cursor-agent` | 1.2.3 | unavailable: `status` has no machine-readable schema | print, stream JSON, history/resume, force | no machine-readable auth readiness | fixture + local authenticated E2E |
| Codex | `codex` | 0.144.4; enforced 0.144.x | `codex login status` | CLI, app-server RPC | only detected app-server surface is advertised | fixture + local authenticated check |
| Claude Code | `claude` | 2.1.119; enforced 2.1.x | `claude auth status` JSON | stream JSON, resume | no capabilities beyond CLI help are advertised | fixture + local authenticated check |
| Droid | `droid` | 1.2.3 | unavailable: no non-interactive status command | exec JSON/JSON-RPC, resume/fork, spec/autonomy/tool controls | list/delete are unavailable unless the CLI advertises them; auth cannot be checked | fixture + local authenticated E2E |
| Gemini CLI | `gemini` | 0.46.0; enforced 0.46.x | unavailable: no non-interactive status contract | ACP, stream JSON, list/resume sessions | auth cannot be checked | fixture + local authenticated E2E |
| Goose | `goose` | 1.36.0 | unavailable: no non-interactive status command | ACP/stdio, load/list/close sessions, MCP HTTP, context/images/extensions | fork/resume unavailable unless advertised; auth cannot be checked | fixture + local authenticated E2E |
| Kimi Code CLI | `kimi` | 1.45.0 | unavailable: no non-interactive status command | ACP/stdio, load/list sessions, MCP HTTP/SSE, context/images, plan | close/fork/resume unavailable unless advertised | fixture + local authenticated E2E |
| Mistral Vibe | `vibe` and `vibe-acp` | 2.1.0 | unavailable: no non-interactive status command | ACP/stdio, load/list/close/fork, context/images, JSON, plan | resume unavailable unless advertised | fixture + local authenticated E2E |
| Copilot CLI | `copilot` | 0.0.411; enforced 0.0.411–0.0.999 | unavailable: no machine-readable status contract | ACP/stdio, resume | native login remains provider-owned; auth cannot be checked | fixture + local authenticated E2E |
| OpenCode | `opencode` | 1.18.0; enforced 1.17.15–1.18.0 | `opencode providers list` | ACP/stdio, load/list/close/fork/resume, MCP HTTP/SSE, context/images | other versions and missing ACP are unsupported | fixture + local authenticated E2E |
| Pi | `pi` | 0.80.7; exact enforced | unavailable: RPC has no credential-status contract | RPC/stdio, state, create/resume, tool filters | list/close sessions and auth status are unavailable; local run is offline | fixture + local offline E2E |

`:GatorHealth` does not read credentials. “Authentication probe passed” means the provider’s own status command reported login; it does not expose or validate credential material. Run `make live-handoff-e2e` locally on a machine authenticated to every provider; it runs native checks before the directed cross-provider handoff matrix.
