# Custom providers and local endpoints

For Gator's reviewed Ollama coding-model catalog, use
[`gator local`](LOCAL_MODELS.md). It downloads a selected package with explicit
confirmation and binds it to every native Gator interface without manually
entering an endpoint. This document covers the separate path for a local server
or compatible endpoint that you operate yourself.

Gator's custom-provider path is for an OpenAI-compatible Chat Completions
endpoint that you operate or have independently documented. It keeps Gator's
native agent loop, worktree flow, tools, verification, sessions, and RPC
protocol; only the inference endpoint changes.

Create the provider once:

```sh
# A manually operated Ollama, LM Studio, or vLLM endpoint.
gator provider add local-llm \
  --base-url http://127.0.0.1:11434/v1/chat/completions \
  --model qwen3-coder

# A compatible endpoint that requires a key held in the process environment.
gator provider add team-gateway \
  --base-url https://models.example.com/v1/chat/completions \
  --api-key-env TEAM_GATEWAY_API_KEY \
  --model coding-large --model coding-small
```

The first `--model` is the default; subsequent model flags add selectable
models. Use the configured ID everywhere a built-in provider would appear:

```sh
gator run --provider local-llm --verify 'go test ./...' 'Add focused tests'
gator config set default-provider local-llm
gator provider list
gator provider discover local-llm
gator provider discover local-llm --apply
gator provider remove local-llm --yes
```

The terminal picker and JSONL RPC validate the same catalog. A custom provider
does not accept PDFs because the generic Chat Completions protocol has no
stable document-input format. Gator persists endpoint metadata and model IDs in
`config.json`, never API keys. A keyless endpoint sends no `Authorization`
header; a keyed endpoint reads exactly the named environment variable at run
start.

`discover` requests the standard sibling `/models` endpoint derived from a
base URL ending in `/chat/completions`. It prints discovered IDs first;
`--apply` is the explicit step that replaces the persisted model catalog. The
existing default is retained when still present, otherwise Gator selects the
first discovered ID.
