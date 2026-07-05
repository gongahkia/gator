# paw

`paw` is a Go CLI coding-agent harness that productizes a known context-pruning pattern: a small "drone" model compresses raw repository/tool output into a validated digest before a larger "brain" model plans and edits. The drone does not make code decisions. Go validation checks schema shape, paths, line ranges, and verbatim spans before the brain sees the digest.

This project does not claim to invent small-model context compression. See [RELATED_WORK.md](RELATED_WORK.md) and [DESIGN.md](DESIGN.md) for the positioning.

## Install

From source:

```sh
go install github.com/gongahkia/paw@latest
```

From this checkout:

```sh
make build
```

For a release binary, put the `paw` executable on `PATH`.

Default mode uses local Ollama for both models:

```sh
ollama serve
ollama pull gpt-oss:20b
ollama pull qwen3:8b
```

## Quickstart

```sh
paw run --instruction "fix the failing test"
```

Pipeline shape:

```sh
paw run --explain
```

Output:

```txt
gather | compress | plan | edit | verify
```

## Configuration

Default config path: `$XDG_CONFIG_HOME/paw/config.toml` or `~/.config/paw/config.toml`.

| env var | config field | default |
| --- | --- | --- |
| `PAW_BRAIN_TRANSPORT` | `brain.transport` | `ollama` |
| `PAW_BRAIN_BASE_URL` | `brain.base_url` | `http://localhost:11434` |
| `PAW_BRAIN_API_KEY` | `brain.api_key` | empty |
| `PAW_BRAIN_MODEL` | `brain.model` | `gpt-oss:20b` |
| `PAW_DRONE_TRANSPORT` | `drone.transport` | `ollama` |
| `PAW_DRONE_BASE_URL` | `drone.base_url` | `http://localhost:11434` |
| `PAW_DRONE_API_KEY` | `drone.api_key` | empty |
| `PAW_DRONE_MODEL` | `drone.model` | `qwen3:8b` |
| `PAW_MAX_TURNS` | `max_turns` | `40` |
| `PAW_MAX_BRAIN_TOKENS` | `max_brain_tokens` | `200000` |
| `PAW_CALL_TIMEOUT` | `call_timeout` | empty |
| `PAW_GATHER_MAX_DEPTH` | `gather.max_depth` | stage default |
| `PAW_GATHER_MAX_FILE_BYTES` | `gather.max_file_bytes` | stage default |

Anthropic-compatible brain transport example:

```toml
[brain]
transport = "anthropic"
base_url = "https://api.deepseek.com/anthropic"
api_key = "..."
model = "deepseek-v4-pro"
```

OpenAI-compatible API override for benchmark runs:

```sh
export PAW_BRAIN_TRANSPORT=openai
export PAW_BRAIN_BASE_URL=https://api.z.ai/api/paas/v4
export PAW_BRAIN_API_KEY=...
export PAW_BRAIN_MODEL=glm-4.6
```

## Prior Art

Closest related systems include SWE-Pruner, Focus, TokenPilot, LLMLingua, and The Token Company. `paw` differs by packaging the pattern as a single Go binary with pipeable stages, stock-model drone support, offline-capable defaults, and deterministic validation of quoted spans. paw's measured benchmark results belong in [docs/RESULTS.md](docs/RESULTS.md), not in copied prior-art numbers.

## Docs

- [DESIGN.md](DESIGN.md)
- [RELATED_WORK.md](RELATED_WORK.md)
- [SCHEMAS.md](SCHEMAS.md)
- [MODEL_APIS.md](MODEL_APIS.md)
- [PATCH_FORMAT.md](PATCH_FORMAT.md)
- [BENCHMARKS.md](BENCHMARKS.md)
- [TESTING.md](TESTING.md)
- [docs/RESULTS.md](docs/RESULTS.md)
