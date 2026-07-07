# paw — Model & Provider APIs (`docs/MODEL_APIS.md`)

Everything the `internal/llm` package needs. All shapes below were verified against live provider
docs as of 2026-07. Pin these; re-verify if a provider changes.

---

## 1. Two transports, one interface

```go
type ChatMessage struct {
    Role    string `json:"role"`    // "system" | "user" | "assistant"
    Content string `json:"content"`
}

type ChatRequest struct {
    Model       string
    Messages    []ChatMessage
    Temperature float64
    MaxTokens   int
    // JSONSchema, when non-nil, requests schema-constrained output.
    // openai transport → response_format; ollama → format; anthropic → tool-based coercion.
    JSONSchema  json.RawMessage
}

type Usage struct {
    InputTokens  int
    OutputTokens int
}

type ChatResponse struct {
    Content string
    Usage   Usage // zero values if provider omitted usage; caller falls back to estimate
}

type Client interface {
    Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error)
}
```

Two implementations: `openaiClient` and `anthropicClient`. Selection is by config
`transport: openai|anthropic|ollama`. Ollama is a thin variant of the openai transport pointed at
the Ollama native `/api/chat` (it supports the richer `format` schema mode).

---

## 2. OpenAI-compatible transport (benchmark/API override: GLM via Z.AI; also DeepSeek, vLLM, OpenRouter)

- **Endpoint:** `{base_url}/chat/completions` (POST).
- **Z.AI base_url:** `https://api.z.ai/api/paas/v4`  → full URL `.../v4/chat/completions`.
- **DeepSeek base_url:** `https://api.deepseek.com` → `.../chat/completions`.
- **Auth header:** `Authorization: Bearer {api_key}`.
  Local loopback endpoints (`localhost`, `127.0.0.1`, `[::1]`) may omit `api_key`.
- **Request body (minimum):**
  ```json
  {
    "model": "glm-5.2",
    "messages": [{"role":"system","content":"..."},{"role":"user","content":"..."}],
    "temperature": 0.0,
    "max_tokens": 4096,
    "stream": false
  }
  ```
- **Structured output:** add
  ```json
  "response_format": { "type": "json_schema", "json_schema": { "name": "digest", "schema": { ... }, "strict": true } }
  ```
  If a given provider/model rejects `json_schema`, fall back to `{"type":"json_object"}` plus the
  schema pasted into the prompt (record which mode was used in the trace).
- **Response:** `choices[0].message.content` (string). `usage.prompt_tokens` /
  `usage.completion_tokens` populate `Usage`.
- **2026 API brain recommendation:** `glm-5.2` on Z.AI's OpenAI-compatible API for long-horizon
  coding work. The current benchmark CLI default remains `glm-4.6` until benchmark baselines are
  rerun. Also validated: `deepseek-v4-pro` on DeepSeek base_url.

### Env vars (OpenAI transport)
```
PAW_BRAIN_BASE_URL   e.g. https://api.z.ai/api/paas/v4
PAW_BRAIN_API_KEY    provider key
PAW_BRAIN_MODEL      e.g. glm-5.2
```

### Local OpenAI-compatible profiles

These use the same `openai` transport and the same `/chat/completions` client path. Run
`paw doctor models` after starting the server; model IDs are server-specific.

- **LM Studio:** `base_url = "http://localhost:1234/v1"`. LM Studio documents `/v1/models` and
  `/v1/chat/completions`; use the model identifier shown by LM Studio.
  Source: https://lmstudio.ai/docs/developer/openai-compat
- **vLLM:** `base_url = "http://localhost:8000/v1"` when served on port 8000. vLLM documents an
  OpenAI-compatible server for Chat/Completions APIs; use the served model ID.
  Source: https://docs.vllm.ai/en/latest/serving/online_serving/openai_compatible_server/
- **llama.cpp server:** `base_url = "http://localhost:8080/v1"` by default. The server docs list
  default port 8080, OpenAI-compatible chat completions, and schema-constrained JSON support.
  Source: https://github.com/ggml-org/llama.cpp/blob/master/tools/server/README.md

### 2026 model notes

- **GLM-5.2** is Z.AI's current API recommendation for coding brains. Z.AI documents model id
  `glm-5.2`, up to 1M context, and migration from earlier GLM-4.x/5.x models. GLM-5 remains
  relevant for self-hosted/open-weight comparisons; its model card reports 77.8 on SWE-bench
  Verified.
- **Qwen3-Coder-Next** is an 80B-A3B coding model with 256K context in the Qwen3-Coder family.
  It is a strong drone candidate when served by Ollama cloud, a LAN OpenAI-compatible endpoint, or
  a verified local quantized runtime. Ollama lists the local image around 52GB, so do not assume it
  fits every 16GB laptop.
- **Qwen3-Coder-30B-A3B-Instruct** is a smaller coding MoE option with 30.5B total / 3.3B active
  parameters and OpenAI-compatible serving examples through vLLM/SGLang.

Sources: https://docs.z.ai/guides/overview/quick-start,
https://docs.z.ai/guides/overview/migrate-to-glm-new, https://github.com/zai-org/GLM-5,
https://github.com/QwenLM/Qwen3-Coder, https://huggingface.co/Qwen/Qwen3-Coder-30B-A3B-Instruct,
https://ollama.com/library/qwen3-coder-next

### macOS local sizing guidance

These are conservative starting points for Apple Silicon unified memory, not guarantees.
Ollama documents that context length and parallelism change required RAM, and its `gpt-oss`
library page says `gpt-oss:20b` can run with as little as 16GB memory. Always verify with
`paw doctor models` on the target machine.

- **8-12GB memory:** keep the drone local, use a subscription/API brain or a 3-4B local brain.
- **16GB memory:** keep `qwen3:8b` as the conservative local drone. Prefer Qwen3-Coder-Next as a
  coding drone only through cloud/LAN/local runtimes that have already passed a real smoke.
- **24-36GB memory:** use the default pair for reliability; test Qwen3-Coder-30B-A3B or
  Qwen3-Coder-Next through an OpenAI-compatible local server with reduced context.
- **48GB+ memory:** test Qwen3-Coder-Next as drone or local brain, and use GLM-5-family API or
  self-hosted brains for long-horizon tasks after the doctor and a real task smoke pass.

Sources: https://ollama.com/library/gpt-oss, https://docs.ollama.com/faq,
https://docs.ollama.com/context-length, https://ollama.com/library/qwen3-coder-next

---

## 3. Anthropic-compatible transport (optional; DeepSeek also exposes this)

- **Endpoint:** `{base_url}/v1/messages` (POST). DeepSeek base_url for this mode:
  `https://api.deepseek.com/anthropic`.
- **Headers:** `x-api-key: {key}`, `anthropic-version: 2023-06-01`, `Content-Type: application/json`.
- **Body:** `{ "model": "...", "max_tokens": N, "system": "...", "messages": [{"role":"user","content":"..."}] }`.
- **Response:** `content[]` array; concatenate text blocks. `usage.input_tokens` /
  `usage.output_tokens`.
- **Structured output:** Anthropic-style has no `response_format`; coerce via a single tool with
  an `input_schema` equal to the JSON Schema and instruct the model to call it, then read
  `tool_use.input`. If the compatible endpoint lacks tool support, fall back to prompt-embedded
  schema + `json_object`-style parsing.

This transport is OPTIONAL for v1 — implement openai + ollama first; anthropic can be a later task.

---

## 4. Ollama transport (default, local)

- **Endpoint:** `http://localhost:11434/api/chat` (POST). Base configurable via `PAW_BRAIN_BASE_URL`
  and `PAW_DRONE_BASE_URL`.
- **Body:**
  ```json
  {
    "model": "qwen3:8b",
    "messages": [{"role":"system","content":"..."},{"role":"user","content":"..."}],
    "stream": false,
    "format": { <full JSON Schema object here> },
    "options": { "temperature": 0 }
  }
  ```
  The `format` field accepts a **full JSON Schema object** (not just `"json"`); Ollama constrains
  generation to schema-shaped JSON. This is the mechanism that makes the drone's output parseable.
  Still validate deterministically afterward — schema-shaped ≠ semantically valid (paths/quotes
  can still be wrong, which is exactly what `internal/compress/validate.go` catches).
- **Response:** `message.content` (a JSON string matching the schema). Ollama returns token counts
  in `prompt_eval_count` (input) and `eval_count` (output) → populate `Usage`.
- **Best practice:** also paste the JSON Schema as text into the prompt to ground the model
  (per Ollama docs). Use `temperature: 0` for determinism.

### Env vars (drone)
```
PAW_DRONE_TRANSPORT  "ollama" (default) | "openai"
PAW_DRONE_BASE_URL   default http://localhost:11434
PAW_DRONE_MODEL      e.g. qwen3:8b  (any 3–8B instruct/coder model; must support format/json)
```

### Env vars (local brain)
```
PAW_BRAIN_TRANSPORT  "ollama" (default) | "openai" | "anthropic"
PAW_BRAIN_BASE_URL   default http://localhost:11434
PAW_BRAIN_MODEL      default gpt-oss:20b
```

## 5. CLI brain transports (subscription reuse, prompt-only)

CLI transports implement `Client` by invoking an installed coding CLI. They are intended for
brain use, not drone compression. They reuse the CLI's own login/session and do not read or copy
credential files.

Logged-in smoke tests are env-gated and skipped by default:
`PAW_E2E_CODEX_CLI=1`, `PAW_E2E_GEMINI_CLI=1`, `PAW_E2E_CLAUDE_CLI=1`,
`PAW_E2E_OPENCODE_CLI=1`. Optional model override env vars use the same names with `_MODEL`.
The smoke prompt asks only for `{"ok":true}` and explicitly says not to inspect or edit files.

All CLI brain transports accept `PAW_BRAIN_MODEL`. Goose also accepts `PAW_BRAIN_PROVIDER`
because its CLI has separate `--provider` and `--model` flags. OpenCode encodes provider in the
model string as `provider/model`.

### Model access matrix

| transport | local/no-key | subscription CLI | API key required | model listing | schema enforcement |
| --- | --- | --- | --- | --- | --- |
| `ollama` | yes | no | no | `/api/tags` | native `format` JSON Schema |
| `openai` loopback | yes | no | no | `/models` | `response_format.json_schema` |
| `openai` remote | no | no | yes | `/models` | `response_format.json_schema`, fallback to JSON object |
| `anthropic` | no | no | yes | `/v1/models` | tool `input_schema`, fallback prompt |
| `codex-cli` | no | yes | CLI-managed | unsupported | native `--output-schema` |
| `gemini-cli` | no | yes | CLI-managed | unsupported | prompt-only |
| `claude-cli` | no | yes | CLI-managed | unsupported | native `--json-schema` |
| `opencode-cli` | provider-dependent | provider-dependent | provider-dependent | `opencode models` | prompt-only |
| `aider-cli` | provider-dependent | provider-dependent | provider-dependent | `aider --list-models <query>` | prompt-only |
| `goose-cli` | provider-dependent | provider-dependent | provider-dependent | unsupported; use `goose configure` | prompt-only |
| `qwen-cli` | provider-dependent | yes | CLI-managed | unsupported; interactive `/model` only | prompt-only |
| `cursor-cli` | no | yes | CLI-managed | `cursor-agent models` | prompt-only, experimental |

### Codex CLI

- **Transport:** `codex-cli`.
- **Command:** `codex exec --ephemeral --sandbox read-only --cd {cwd} [-m model] [--output-schema schema.json] -`.
- **Auth:** whatever `codex login` has configured: ChatGPT subscription auth or API key auth.
- **Structured output:** uses `--output-schema` with a temp JSON Schema file.
- **Safety boundary:** runs read-only and prompts Codex to answer from the supplied digest instead
  of editing files. `paw` still applies patches deterministically.

### Gemini CLI

- **Transport:** `gemini-cli`.
- **Command:** `gemini --prompt "Answer only from stdin..." --approval-mode plan --output-format json --skip-trust [-m model]`.
- **Auth:** whatever the Gemini CLI has configured, including Google login or env-based API auth.
- **Structured output:** schema is pasted into the prompt; output parser accepts raw JSON objects
  or common JSON wrapper fields such as `response`/`text`/`content`.
- **Safety boundary:** plan approval mode; `paw` remains responsible for patch application.

### Claude CLI

- **Transport:** `claude-cli`.
- **Command:** `claude -p --permission-mode plan --output-format json [--model model] [--json-schema schema]`.
- **Auth:** existing Claude Code auth. Do not pass `--bare`; that mode skips OAuth/keychain auth.
- **Structured output:** uses `--json-schema` when `ChatRequest.JSONSchema` is set.
- **Safety boundary:** plan permission mode; `paw` remains responsible for patch application.

### OpenCode CLI

- **Transport:** `opencode-cli`.
- **Command:** `opencode run --format json --dir {cwd} [--model provider/model] {prompt}`.
- **Auth:** OpenCode provider auth. `opencode/big-pickle` and other Zen/free models are optional
  user choices, not benchmark defaults.
- **Structured output:** schema is pasted into the prompt; parser accepts JSON events, JSON arrays,
  and raw text.
- **Safety boundary:** no `--dangerously-skip-permissions`; `paw` remains responsible for patch
  application.

### Aider CLI

- **Transport:** `aider-cli`.
- **Command:** `aider --message {prompt} --dry-run --no-git --no-auto-commits [--model model]`.
- **Structured output:** schema is pasted into the prompt.
- **Safety boundary:** dry-run, no git, no auto-commits, no auto-lint/test/shell suggestions.

### Goose CLI

- **Transport:** `goose-cli`.
- **Command:** `goose run --no-session --quiet --output-format json --no-profile --max-turns 1 --text {prompt} [--provider provider] [--model model]`.
- **Structured output:** schema is pasted into the prompt.
- **Safety boundary:** no session and no profile extensions by default.

### Qwen Code CLI

- **Transport:** `qwen-cli`.
- **Command:** `qwen --prompt {prompt} --approval-mode plan --output-format json [--model model]`.
- **Structured output:** schema is pasted into the prompt.
- **Safety boundary:** approval mode `plan` analyzes only and does not modify files or execute
  commands per Qwen Code docs.

### Cursor CLI (experimental)

- **Transport:** `cursor-cli`.
- **Command:** `cursor-agent --print --output-format json --mode ask [--model model] {prompt}`.
- **Auth:** existing Cursor CLI auth/subscription.
- **Structured output:** schema is pasted into the prompt. Cursor's documented CLI parameters
  include JSON output but no schema flag, so this transport is weaker than Codex/Claude.
- **Safety boundary:** ask mode and no force/yolo flags; `paw` remains responsible for patch
  application. Keep this experimental until logged-in smoke tests prove stable headless behavior.
  Source: https://cursor.com/docs/cli/reference/parameters.md

If `PAW_DRONE_TRANSPORT=openai`, the drone uses a cheap OpenAI-compatible API model instead of
local Ollama (for users with no local GPU). Same `response_format` schema path as §2.

---

## 6. Token counting

Prefer provider-reported `usage`. When absent (some compatible endpoints omit it), estimate with
a local BPE tokenizer: use `github.com/tiktoken-go/tokenizer` (`cl100k_base` encoding) as a
provider-agnostic approximation. Record `token_source: "provider" | "estimate"` in the trace so
reported numbers are never silently fabricated.

---

## 7. Retries, timeouts, determinism

- All requests use `context.Context` with a per-call deadline (config `PAW_CALL_TIMEOUT`, default
  120s for brain, 60s for drone).
- Retry policy: max 2 retries, exponential backoff (250ms, 1s), only on 429/5xx/network. Never
  retry a 4xx schema/validation error.
- `temperature: 0` everywhere by default for reproducible benchmark runs.
- All base URLs, model ids, and keys come from config/env — nothing hardcoded except documented
  defaults.

---

## 8. Why GLM (Z.AI) remains the benchmark headline (not DeepSeek)

Decision recorded for future maintainers:
- GLM-5.2 is Z.AI's current strongest coding model, while GLM-5 remains a published open-weight
  reference with 77.8 on SWE-bench Verified.
- GLM is the open-weight family to track on Terminal-Bench-style, long-horizon agentic work.
- Cheap "coding plan" subscription lowers the barrier for people trying `paw`.
- "The harness that lifts GLM above much pricier proprietary models" is a sharper narrative than
  DeepSeek (already cheap+good, needs less help).
- DeepSeek remains a first-class, tested provider; both are OpenAI-compatible so client code is
  identical and switching is one config line.
