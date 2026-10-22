# Custom providers and local endpoints

**Interface status:** Gator is a desktop app. Shell examples below are source-checkout developer operations with an explicit internal opt-in; see [development instructions](DEVELOPMENT.md). They do not install or enable a public CLI.

For Gator's reviewed Ollama Work and coding-model catalog, use the **Local**
section of [`/model`](LOCAL_MODELS.md). It downloads a selected package with
explicit confirmation and binds it to every native Gator interface without
manually entering an endpoint. This document covers the separate path for a
local server or compatible endpoint that you operate yourself.

Gator's custom-provider path is for an OpenAI-compatible Chat Completions
endpoint that you operate or have independently documented. It keeps Gator's
native agent loop, worktree flow, tools, verification, sessions, and RPC
protocol; only the inference endpoint changes.

Create the provider once:

```sh
# A manually operated Ollama, LM Studio, or vLLM endpoint.
GATOR_INTERNAL_COMMANDS=1 go run ./cmd/gator provider add local-llm \
  --base-url http://127.0.0.1:11434/v1/chat/completions \
  --model qwen3-coder

# A compatible endpoint that requires a key held in the process environment.
GATOR_INTERNAL_COMMANDS=1 go run ./cmd/gator provider add team-gateway \
  --base-url https://models.example.com/v1/chat/completions \
  --api-key-env TEAM_GATEWAY_API_KEY \
  --model coding-large --model coding-small
```

The first `--model` is the default; subsequent model flags add selectable
models. Use the configured ID everywhere a built-in provider would appear:

```sh
GATOR_INTERNAL_COMMANDS=1 go run ./cmd/gator work --provider local-llm 'Summarize the current project and open questions'
GATOR_INTERNAL_COMMANDS=1 go run ./cmd/gator config set default-provider local-llm
GATOR_INTERNAL_COMMANDS=1 go run ./cmd/gator provider list
GATOR_INTERNAL_COMMANDS=1 go run ./cmd/gator provider discover local-llm
GATOR_INTERNAL_COMMANDS=1 go run ./cmd/gator provider discover local-llm --apply
GATOR_INTERNAL_COMMANDS=1 go run ./cmd/gator provider remove local-llm --yes
```

The terminal picker and JSONL RPC validate the same catalog. In `/model`, `n`
creates a custom provider, `c` on a custom row edits it, `g` previews the
sibling `/models` catalog as untrusted server data, and `x` removes endpoint
metadata after confirmation. A custom provider
does not receive PDF bytes because the generic Chat Completions protocol has no
stable document-input format; Gator converts selected PDFs to bounded,
page-marked text first. Image-only pages are explicitly marked as needing OCR.
Gator persists endpoint metadata and model IDs in `config.json`, never API keys.
A keyless endpoint sends no `Authorization`
header; a keyed endpoint reads exactly the named environment variable at run
start. Removing a provider does not unset that environment variable.

`discover` requests the standard sibling `/models` endpoint derived from a
base URL ending in `/chat/completions`. Off-origin redirects are rejected. It
prints discovered IDs first; `--apply` or the TUI confirmation is the explicit
step that replaces the persisted model catalog. The
existing default is retained when still present, otherwise Gator selects the
first discovered ID. Removing a provider also clears defaults and display
aliases that pointed at it.
