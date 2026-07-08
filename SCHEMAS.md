# paw — Data Schemas (`docs/SCHEMAS.md`)

Authoritative field-level definitions for the `Envelope` and its sub-structs. Go structs live in
`internal/envelope/`. JSON Schemas (used both for the drone's structured-output `format` and for
deterministic validation) live in `internal/schema/` and are embedded with `go:embed`.

Rule: **Go struct tags and JSON Schema `required`/`properties` MUST stay in sync.** A unit test
(`internal/schema/schema_test.go`) marshals a fully-populated struct and validates it against the
embedded schema to enforce this.

---

## 1. `RawContext` (output of `gather`)

```go
type RawUnit struct {
    ID       string `json:"id"`        // stable within an envelope, e.g. "u001"
    Kind     string `json:"kind"`      // "file_slice" | "search_hits" | "dir_listing" | "verify_failure"
    Path     string `json:"path"`      // real path relative to Cwd; "" for non-file kinds
    StartLine int   `json:"start_line"`// 1-based; 0 if N/A
    EndLine   int   `json:"end_line"`  // inclusive; 0 if N/A
    Text     string `json:"text"`      // the actual bytes (may be large)
}

type RawContext struct {
    Units      []RawUnit `json:"units"`
    TotalBytes int       `json:"total_bytes"`
}
```

Provenance rule for validation: the set of `(Path, StartLine..EndLine)` and the literal `Text`
of each `RawUnit` are the ground truth that `compress` output is checked against.

---

## 2. `ContextDigest` (output of `compress`, the drone) — VALIDATED

JSON Schema (`internal/schema/context_digest.schema.json`), also passed as Ollama `format`:

```json
{
  "type": "object",
  "additionalProperties": false,
  "required": ["items", "summary"],
  "properties": {
    "summary": { "type": "string", "maxLength": 1200 },
    "items": {
      "type": "array",
      "maxItems": 40,
      "items": {
        "type": "object",
        "additionalProperties": false,
        "required": ["unit_id", "path", "relevance", "spans"],
        "properties": {
          "unit_id":   { "type": "string" },
          "path":      { "type": "string" },
          "relevance": { "type": "integer", "minimum": 0, "maximum": 100 },
          "spans": {
            "type": "array",
            "maxItems": 10,
            "items": {
              "type": "object",
              "additionalProperties": false,
              "required": ["start_line", "end_line", "quote"],
              "properties": {
                "start_line": { "type": "integer", "minimum": 0 },
                "end_line":   { "type": "integer", "minimum": 0 },
                "quote":      { "type": "string", "maxLength": 2000 }
              }
            }
          }
        }
      }
    }
  }
}
```

```go
type DigestSpan struct {
    StartLine int    `json:"start_line"`
    EndLine   int    `json:"end_line"`
    Quote     string `json:"quote"`
}
type DigestItem struct {
    UnitID    string       `json:"unit_id"`
    Path      string       `json:"path"`
    Relevance int          `json:"relevance"`
    Spans     []DigestSpan `json:"spans"`
}
type ContextDigest struct {
    Summary string       `json:"summary"`
    Items   []DigestItem `json:"items"`
}
```

**Deterministic validation performed in `internal/compress/validate.go` (NO model):**
1. Parses & schema-validates against the embedded schema.
2. For each `DigestItem`: `UnitID` MUST match a `RawUnit.ID` present in the input `RawContext`.
3. `path` MUST equal that unit's `Path`.
4. For each `DigestSpan`: `Quote` MUST satisfy `strings.Contains(unit.Text, quote)` (verbatim).
5. `start_line`/`end_line` MUST fall within `[unit.StartLine, unit.EndLine]` when the unit has
   line info.
6. Any item failing 2–5 is DROPPED (logged to trace). If >50% of items are dropped OR the digest
   fails to parse, `compress` returns the **heuristic fallback digest** instead (see below).

**Heuristic fallback (deterministic, no model):** rank `RawUnit`s by BM25-style term overlap with
the `Instruction`, take the top-N by a token budget, emit them as digest items with `relevance`
set from the score and `spans` set to the whole unit. Guarantees the pipeline never stalls on a
bad drone and gives a clean control condition for the ablation ("compress off = fallback only").

---

## 3. `Plan` (output of `plan`, the brain)

```json
{
  "type": "object",
  "additionalProperties": false,
  "required": ["done", "reasoning"],
  "properties": {
    "done":      { "type": "boolean" },
    "reasoning": { "type": "string", "maxLength": 2000 },
    "next_action": {
      "type": "object",
      "additionalProperties": false,
      "required": ["kind", "description"],
      "properties": {
        "kind":        { "type": "string", "enum": ["edit_file"] },
        "description": { "type": "string", "maxLength": 2000 },
        "target_path": { "type": "string" }
      }
    }
  }
}
```

```go
type NextAction struct {
    Kind        string `json:"kind"` // edit_file
    Description string `json:"description"`
    TargetPath  string `json:"target_path,omitempty"`
}
type Plan struct {
    Done       bool        `json:"done"`
    Reasoning  string      `json:"reasoning"`
    NextAction *NextAction `json:"next_action,omitempty"`
}
```

Validation: if `Done` is false, `NextAction` MUST be present. In v1, the only supported action is
`edit_file`, and it requires `target_path`. Additional action kinds are intentionally rejected
until the pipeline has deterministic handlers for them.

---

## 4. `Patch` (output of `edit`, the brain)

The brain returns a unified diff as text (see `docs/PATCH_FORMAT.md` for the exact grammar). The
struct wraps it plus metadata:

```go
type Patch struct {
    UnifiedDiff string   `json:"unified_diff"` // standard unified diff, possibly multi-file
    Files       []string `json:"files"`        // paths the diff claims to touch (for a fast sanity check)
    Note        string   `json:"note,omitempty"`
}
```

Validation before apply: `Files` MUST be non-empty; each must be within `Cwd` (reject absolute or
`../` escapes); the diff must parse. Apply is deterministic in `internal/patch/`.

---

## 5. `VerifyResult` (output of `verify`, deterministic)

```go
type VerifyResult struct {
    Passed        bool   `json:"passed"`
    ExitCode      int    `json:"exit_code"`
    Command       string `json:"command"`         // what was run
    FailureDigest string `json:"failure_digest"`  // deterministically truncated/structured output
    RawTailBytes  int    `json:"raw_tail_bytes"`  // how much raw output existed before truncation
}
```

`FailureDigest` construction (deterministic, no model): keep the last N lines, plus any lines
matching common failure markers (`FAIL`, `Error`, `assert`, `panic`, `Traceback`, `expected`,
`got`), deduplicated, capped at a byte budget (default 4 KB). This is fed back into `gather` as a
`verify_failure` `RawUnit`.

---

## 6. `Budget` (accounting, threaded through every envelope)

```go
type Budget struct {
    MaxTurns                 int `json:"max_turns"`
    Turn                     int `json:"turn"`
    BrainInputTokens         int `json:"brain_input_tokens"`   // headline metric
    BrainOutputTokens        int `json:"brain_output_tokens"`
    BrainCacheCreationTokens int `json:"brain_cache_creation_tokens,omitempty"`
    BrainCacheReadTokens     int `json:"brain_cache_read_tokens,omitempty"`
    BrainTokenSource         string `json:"brain_token_source,omitempty"` // provider|estimate|mixed
    DroneTokens              int `json:"drone_tokens"`
    DroneTokenSource         string `json:"drone_token_source,omitempty"` // provider|estimate|mixed
    MaxBrainTokens           int `json:"max_brain_tokens"`      // hard cap; loop stops if exceeded
}
```

Token counts come from provider `usage` fields when present; otherwise from a local tokenizer
estimate (see `docs/MODEL_APIS.md` §5). Source fields are role-level provenance: `provider`,
`estimate`, `mixed`, or omitted when no tokens were added.

---

## 7. Trace log (observability, for ablation & debugging)

Every run writes an NDJSON trace to `--trace-file` (default `./.paw/trace-<taskid>.ndjson`): one
line per stage invocation with `{stage, turn, input_bytes, output_bytes, tokens, token_source,
dropped_items, validation_drops, used_fallback, duration_ms, envelope}`. `token_source` is
`provider`, `estimate`, `mixed`, or `none` for that stage's token delta. `validation_drops` is a
compact reason counter for `compress`, keyed by `unknown_unit`, `path_mismatch`, `quote_missing`,
`line_out_of_range`, `schema_error`, and `too_many_dropped`; it never stores raw quotes.
`envelope` is the post-stage snapshot used by `paw resume`. `paw bench` aggregates the metric
fields into the results tables and writes token source metadata into generated result configs. No
model is involved in tracing.
