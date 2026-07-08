# paw — Design & Architecture

> **Working name:** `paw` (Pipelined Agent Workbench). Placeholder — rename via a single
> find-and-replace across the repo (`paw` → `<newname>`) plus the module path in `go.mod`.
> This document is the authoritative architecture reference.

---

## 1. One-paragraph thesis

`paw` is a coding-agent harness delivered as a single static Go binary. Its defining bet is
that **a large frontier/API model should never read raw repository noise**. A small, cheap
model (local via Ollama, or a cheap API model) sits in front of the expensive model and does
nothing but **compress and structure context** — turning 40 KB of `grep`/`ls`/test output into
a few hundred tokens of relevance-ranked, schema-valid digest. Every output of the small model
is **validated by deterministic Go code** against a JSON Schema before the big model is allowed
to see it, so a weak model cannot silently corrupt the task. The big ("brain") model makes
**100% of the code decisions**; the small ("drone") model never edits code and never makes a
judgment that isn't checked by code.

The harness is built as a set of **composable Unix-style stages**, each with a strict
text-in/text-out contract, each independently runnable as a subcommand (`paw compress`,
`paw verify`, …) and pipeable in a shell. The default agent is just the wired-up pipeline:
`gather | compress | plan | edit | verify`, looped.

The **same binary** that a user installs is the one that produces benchmark numbers. Benchmarks
(Terminal-Bench 2.0 first, SWE-bench Verified second) run through the official Harbor harness via
a thin Python adapter that installs and invokes the Go binary inside the sandbox. Per-stage
ablation is a first-class feature so the token-savings and pass-rate claims are reproducible and
attributable — directly answering the "disclose the harness" critique in the literature. Because
the core pruning idea is already proven in research (SWE-Pruner, Focus, TokenPilot — see
`docs/RELATED_WORK.md`), `paw`'s job is to be the first well-engineered, installable, no-training,
verified-compression *product* built on that idea, and to benchmark honestly against it.

---

## 2. Positioning: productize a proven technique (honest prior-art framing)

**The core mechanism is NOT novel, and we do not claim it is.** "A small model prunes/compresses
tool output before the large model sees it, at line granularity, task-aware" is a validated
research result as of 2026. See `docs/RELATED_WORK.md` for the full comparison. In brief:
- **SWE-Pruner** (SJTU + Douyin, Jan 2026) is the closest prior art: middleware that intercepts
  file-read output and prunes it with a trained 0.6B "skimmer" before it reaches the agent, on
  SWE-bench Verified with GLM-4.6 — the same model family we headline. It reports 23–54% token
  reduction while holding or improving success rate.
- **Focus** (model-controlled compression) and **TokenPilot** (cache-aware context management)
  show similar 22–87% savings.
- **LLMLingua / LLMLingua-2** (Microsoft) established years earlier that small-model prompt
  compression retains reasoning at high ratios and transfers across models.
- **The Token Company** (`bear-2`) ships a general-purpose NL compression API but explicitly
  states it is *not* for code or structured formats.

That the mechanism works is GOOD NEWS: it de-risks the central hypothesis. `paw`'s contribution
is therefore **not the mechanism but the artifact and two specific engineering properties** the
prior work does not have:

1. **A shipped, installable tool, not a research scaffold.** SWE-Pruner/Focus/TokenPilot are
   Python research artifacts (and SWE-Pruner needs a *custom-trained* 0.6B model on 61K synthetic
   examples). None is a single-binary, `go install`-able, daily-driver coding agent. `paw` is.
2. **No training, stock models, offline-capable.** `paw`'s drone is ANY off-the-shelf Ollama
   model (3–8B) constrained by JSON-schema structured output + deterministic validation — zero
   fine-tuning, swappable, runs air-gapped. "Proven-technique-grade pruning with a stock 8B model
   and no training" is a claim the trained-skimmer approaches cannot make.
3. **A verified compression contract.** Every prior system TRUSTS its compressor's output. `paw`
   is the only one that deterministically verifies each emitted span is *real, verbatim bytes*
   from the gathered context before the brain is allowed to see it (see §4.2). This safety
   property is what makes using an untrained weak drone sound, and it is the smallest defensible
   research delta if you later want to write it up.
4. **Composable Unix stages.** None of the prior tools are pipeable filters; `paw` stages are.

**Honest one-line positioning:** *the first installable, single-binary coding agent that brings a
research-proven context-pruning technique to any open-weight model — no training, offline-capable,
with a verified (not merely trusted) compression step.*

**Explicitly NOT (anti-scope — do not build in v1):**
- No auto-evolving / self-mutating harness (compute trap; research artifact, not a tool).
- No custom-trained compressor model in v1 (that is SWE-Pruner's path; ours is stock + validated).
- No embedded inference in the Go binary (Go has no viable local-inference story; we shell out
  to Ollama/llama.cpp over HTTP).
- No multi-daemon / microservice architecture (adoption killer). Stages are in-process by
  default; the "process" identity is the *subcommand* surface, not separate long-lived daemons.
- The drone model NEVER edits code, applies patches, or makes an unchecked decision.
- No plugin system, no sub-agents-spawning-sub-agents in v1.
- MCP support is limited to a context sidecar (`paw mcp serve`) for `gather`/`compress`/`digest`;
  it is not a second agent runner and does not expose planning, editing, or verification tools.

---

## 3. The stage model (the "Unix" core)

The agent is a pipeline of stages. Each stage is a Go type implementing a common interface and
is **also** exposed as a CLI subcommand that reads stdin and writes stdout. Stages communicate
via a small, versioned, newline-delimited-JSON (NDJSON) envelope on the wire, and via in-memory
structs when composed in-process (the default). The envelope is the "text as universal interface"
contract.

```
gather → compress → plan → edit → verify
  │         │          │      │       │
  │         │          │      │       └─ deterministic: run tests/build/lint, produce pass/fail + failure digest
  │         │          │      └───────── BRAIN: produce a patch (unified diff) for one step
  │         │          └──────────────── BRAIN: choose next action / produce a step plan
  │         └─────────────────────────── DRONE: compress gathered context to a schema-valid digest (VALIDATED)
  └───────────────────────────────────── deterministic: run cheap tools (rg, ls, sed -n, ctags, test runner)
```

### Stage contract (Go)

```go
// Stage is one composable unit. Implementations MUST be pure w.r.t. their inputs
// except for explicitly declared side effects (edit and verify touch the filesystem).
type Stage interface {
    Name() string // "gather", "compress", "plan", "edit", "verify"
    // Run consumes one Envelope and returns the next Envelope (or an error).
    // ctx carries deadline/cancellation. r/w are used ONLY in standalone CLI mode
    // to stream the envelope; in-process composition passes structs directly.
    Run(ctx context.Context, in *Envelope) (*Envelope, error)
}
```

### Envelope (the wire contract)

`Envelope` is the single serialized unit passed between stages. It is versioned so the CLI
subcommands remain composable across releases.

```go
type Envelope struct {
    SchemaVersion string          `json:"schema_version"` // e.g. "paw.env/1"
    TaskID        string          `json:"task_id"`
    Instruction   string          `json:"instruction"`     // original task text (immutable)
    Cwd           string          `json:"cwd"`
    Stage         string          `json:"stage"`           // last stage that wrote this envelope
    Turn          int             `json:"turn"`
    Digest        *ContextDigest  `json:"digest,omitempty"`   // produced by compress
    Plan          *Plan           `json:"plan,omitempty"`     // produced by plan
    Patch         *Patch          `json:"patch,omitempty"`    // produced by edit
    Verify        *VerifyResult   `json:"verify,omitempty"`   // produced by verify
    Budget        Budget          `json:"budget"`             // token/turn budget accounting
    Raw           *RawContext     `json:"raw,omitempty"`      // produced by gather (may be large)
    Done          bool            `json:"done"`
}
```

Full field-level schemas for `ContextDigest`, `Plan`, `Patch`, `VerifyResult`, `RawContext`, and
`Budget` are specified in `docs/SCHEMAS.md`.

### Composition modes

- **In-process (default):** `paw run` constructs the stages and passes `*Envelope` structs
  directly. No serialization between stages, no IPC, low latency. State lives in memory in one
  process.
- **Standalone / pipeable:** each stage subcommand reads a JSON `Envelope` on stdin and writes a
  JSON `Envelope` on stdout, so `paw gather ... | paw compress | paw plan | ...` works in a shell
  and interleaves with arbitrary Unix tools (`jq`, `tee`, user scripts). This is the identity
  feature and the ablation mechanism, not the hot path.

---

## 4. The stages in detail

### 4.1 `gather` (deterministic, no model)
Collects raw context using cheap, fast, non-LLM tools only:
- `ripgrep` (`rg`) for symbol/string search (preferred; fall back to `grep -R`).
- `ls -la`, directory walking (bounded depth), `sed -n 'START,ENDp'` for file slices.
- Universal `ctags` if available for symbol maps (optional; degrade gracefully).
- The verify stage's previous failure output (fed back on loop iterations).
Output: `Envelope.Raw` (a `RawContext` — a list of labeled text blobs with provenance). This can
be large; it is the thing `compress` shrinks. `gather` NEVER calls any model.

### 4.2 `compress` (DRONE model + deterministic validation) — the core contribution
Takes `Envelope.Raw` and produces `Envelope.Digest`, a small, relevance-ranked, **schema-valid**
`ContextDigest`. Mechanism:
1. Chunk `Raw` into labeled units (file slice, search hit group, test failure).
2. Call the drone model via Ollama `/api/chat` with `format` set to the JSON Schema of
   `ContextDigest` (Ollama enforces schema-shaped output — see `docs/MODEL_APIS.md`).
   The prompt asks the drone ONLY to (a) score each unit's relevance to the instruction 0–100,
   (b) extract the minimal quoted spans that matter, (c) NEVER invent file paths, symbols, or
   line numbers.
3. **Deterministic validation (Go, no model):**
   - JSON must parse and satisfy the schema (types, required fields, enums).
   - Every `path` in the digest MUST exist in `Raw` provenance (reject hallucinated paths).
   - Every quoted span MUST be a verbatim substring of the corresponding `Raw` unit
     (reject fabricated content). This is a literal `strings.Contains` check.
   - Every cited line number MUST be within the unit's real line range.
   - If any check fails → the offending unit is dropped and the failure is logged; if too many
     fail → fall back to a deterministic heuristic digest (BM25/relevance ranking, no model) so
     the pipeline never blocks on a bad drone.
4. Emit the validated `ContextDigest`. This digest — not `Raw` — is what the brain sees.

The validation step is what makes it safe to use a 3–8B model here: the drone can only ever
*select and quote real bytes*, never author new content the brain will trust.

### 4.3 `plan` (BRAIN model)
Given `Instruction` + validated `Digest` + prior `VerifyResult`, the brain produces a `Plan`:
either `next_action` (one concrete step) or `done`. Uses the provider client
(`docs/MODEL_APIS.md`). Kept short: one step at a time to avoid long-horizon incoherence (the
documented failure mode of even strong models past step 3–4 in some harnesses).

### 4.4 `edit` (BRAIN model)
Given the current step and digest, the brain produces a `Patch` as a **unified diff** against
named files. `paw` applies the patch deterministically (see `docs/PATCH_FORMAT.md`) — the model
does not run shell commands to edit; it emits a diff and Go applies it. Rejected/oversized/
non-applying patches are bounced back to the brain with the apply error (bounded retries).

### 4.5 `verify` (deterministic, no model)
Runs the task's verification: build, tests, lint, or a task-provided check command. Produces a
`VerifyResult` (pass/fail + a **compressed** failure digest — we truncate and structure test
output deterministically here; we do NOT send raw multi-thousand-line test logs to the brain).
On failure, the loop continues: `verify` output feeds the next `gather`/`compress`/`plan`.

---

## 5. Control loop

```
turn = 0
env = gather(initial)            # deterministic
loop:
    env = compress(env)          # drone + validation
    env = plan(env)              # brain
    if env.Plan.Done: break
    env = edit(env)              # brain → unified diff → deterministic apply
    env = verify(env)            # deterministic
    if env.Verify.Passed: break
    if budget exhausted or turn >= maxTurns: break
    env = gather(env)            # deterministic, now includes verify failure
    turn++
```

Budget accounting (`Budget`) tracks brain-input tokens, brain-output tokens, drone tokens, and
turn count. The headline metric is **brain-input tokens per resolved task** — the number the
compression layer is designed to cut.

---

## 6. Models & providers

- **Brain (default headline): GLM (Z.AI), OpenAI-compatible** at `https://api.z.ai/api/paas/v4`.
  Rationale in `docs/MODEL_APIS.md`. Provider-agnostic client supports any OpenAI-compatible or
  Anthropic-compatible base URL, so DeepSeek, Gemini (via OpenAI-compat gateways), local vLLM,
  etc. all work by config.
- **Drone (default): local via Ollama** `http://localhost:11434`, small model (Qwen3-class 3–8B
  or Llama-3.x 3–8B). Can also be a cheap API model if no local GPU.
- All model access goes through one internal `llm` package with two transports: `openai` and
  `anthropic` (both are just HTTP + JSON). See `docs/MODEL_APIS.md` for exact request/response
  shapes, env vars, and pinned behavior.
- Model routing is an integration surface, not a core product layer: users can point brain/drone
  roles at different compatible endpoints, but v1 does not optimize, arbitrate, or broker model
  selection.

---

## 7. Benchmarking (credibility spine)

- Primary: **Terminal-Bench 2.0** via **Harbor** (`harbor run --dataset terminal-bench@2.0`).
  Integration is a Python adapter subclassing `BaseInstalledAgent` that (a) `install()`s the
  `paw` binary into the sandbox container and (b) `run()`s it against the instruction, letting
  Harbor read `reward.txt`. Full contract in `docs/BENCHMARKS.md`.
- Secondary: **SWE-bench Verified**, only after a benchmark-capable model produces a credible
  Terminal-Bench smoke pass rate under matching raw/full configs.
- `paw bench` is a thin local wrapper that shells out to Harbor with the right flags and collects
  results + token accounting into `docs/RESULTS.md`-style tables.
- **Ablation:** `paw run --disable-compress` (drone off → brain reads raw) and
  `paw run --drone-model X` let us produce the per-stage delta table that makes the token claim
  attributable.

Near-term roadmap:
- Context gateway depth: git-aware and syntax-aware gather units that preserve provenance.
- Validation auditability: per-span drop reasons and token-source reporting in traces/stats.
- Tool execution correctness: repo-aware, bounded verify command resolution.
- Local stage benchmarks: model-free tests for gather/compress/plan/edit/verify behavior before
  expensive Harbor runs.

---

## 8. Repository layout (target)

```
paw/
├── go.mod                       # module github.com/<you>/paw  (Go 1.23+)
├── main.go                      # thin: calls cmd.Execute()
├── cmd/                         # cobra commands: root, run, gather, compress, plan, edit, verify, bench, version
├── internal/
│   ├── envelope/                # Envelope + sub-structs + (de)serialization + schema version
│   ├── schema/                  # embedded JSON Schemas (go:embed) + validators
│   ├── stage/                   # Stage interface + Pipeline runner (in-process + streaming)
│   ├── gather/                  # deterministic context collection (rg/ls/sed/ctags wrappers)
│   ├── compress/                # drone call + deterministic validation + heuristic fallback
│   ├── plan/                    # brain planning
│   ├── edit/                    # brain edit → unified diff
│   ├── patch/                   # deterministic unified-diff apply
│   ├── verify/                  # deterministic build/test/lint runner + failure compaction
│   ├── llm/                     # provider-agnostic client (openai + anthropic transports)
│   ├── budget/                  # token/turn accounting
│   └── config/                  # config file + env var loading
├── adapters/harbor/             # Python BaseInstalledAgent adapter for Terminal-Bench via Harbor
├── testdata/                    # fixtures for stage unit tests
├── docs/                        # this file + the referenced specs
└── README.md
```

---

## 9. Referenced specs (read before implementing)
- `docs/RELATED_WORK.md` — prior art (SWE-Pruner, Focus, TokenPilot, LLMLingua, The Token
  Company), what paw shares and how it differs; read this first to understand positioning.
- `docs/SCHEMAS.md` — exact JSON Schemas / Go structs for every Envelope sub-type.
- `docs/MODEL_APIS.md` — provider endpoints, request/response shapes, env vars, Ollama schema mode.
- `docs/PATCH_FORMAT.md` — unified-diff format `edit` must emit and `patch` must apply.
- `docs/BENCHMARKS.md` — Harbor adapter contract, `reward.txt`, run commands, pinned versions.
- `docs/TESTING.md` — how each stage is unit-tested with fixtures and a fake LLM server.
