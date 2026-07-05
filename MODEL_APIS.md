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
- **Request body (minimum):**
  ```json
  {
    "model": "glm-4.6",
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
- **Benchmark headline model id:** `glm-4.6` (config-overridable). Also validated: `deepseek-v4-pro`
  on DeepSeek base_url.

### Env vars (OpenAI transport)
```
PAW_BRAIN_BASE_URL   e.g. https://api.z.ai/api/paas/v4
PAW_BRAIN_API_KEY    provider key
PAW_BRAIN_MODEL      e.g. glm-4.6
```

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
- GLM is the top open-weight family on Terminal-Bench (our primary benchmark), and strongest on
  the agentic/front-end work daily-driver users care about.
- Cheap "coding plan" subscription lowers the barrier for people trying `paw`.
- "The harness that lifts GLM above much pricier proprietary models" is a sharper narrative than
  DeepSeek (already cheap+good, needs less help).
- DeepSeek remains a first-class, tested provider; both are OpenAI-compatible so client code is
  identical and switching is one config line.
