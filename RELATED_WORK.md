# paw — Related Work & Positioning (`docs/RELATED_WORK.md`)

Read this before implementing so you understand what `paw` does and does NOT claim. The core
mechanism — *a small model prunes/compresses retrieved context before a large model consumes it,
task-aware, at line granularity* — is a **proven, published technique, not a `paw` invention.**
`paw`'s contribution is productizing it well with two engineering properties the prior work lacks.
All facts below verified against sources as of 2026-07; re-check before any public comparison.

---

## 1. Prior art (what already exists)

### SWE-Pruner — the closest prior art (SJTU + Douyin, arXiv:2601.16746, Jan 2026)
- **What it is:** middleware between a coding agent and its environment. Intercepts raw file-read
  output (grep/cat), and a trained **0.6B "neural skimmer"** selects relevant lines given a
  natural-language **goal hint** the agent emits, returning pruned context to the agent.
- **Results:** 23–54% token reduction on SWE-bench Verified while holding or improving success
  rate; up to 14.84× on single-turn LongCodeQA. Reduces agent rounds up to ~26%. Evaluated on
  **Claude Sonnet 4.5 and GLM-4.6** (our headline family).
- **Key motivating datum we reuse:** in their trace analysis, **read-type operations consumed
  ~76% of total agent tokens** — this is exactly the surface `paw`'s compress stage targets.
- **How paw differs:** SWE-Pruner requires a *custom-trained* 0.6B model (61K synthetic training
  examples); it is a Python research artifact; it trusts the skimmer's output. `paw` uses a
  **stock, untrained** Ollama model with **schema-constrained output + deterministic verbatim
  validation**, ships as a **single Go binary**, and exposes composable subcommands.

### Focus / Active Context Compression (arXiv:2601.07190)
- Model-*controlled* compression: the agent itself decides when to compact via a tool. 22.7%
  token savings at equal accuracy on SWE-bench instances; needs "aggressive prompting" to work.
- **How paw differs:** paw's compression is a deterministic pipeline stage the drone performs and
  Go validates, not a capability the brain must remember to invoke; and paw's savings target the
  brain's *input*, independent of the brain's own behavior.

### TokenPilot (arXiv:2606.17016)
- Cache-efficient dual-granularity context management (ingestion-aware compaction + lifecycle
  eviction); 56–87% cost reduction, emphasizes prompt-cache prefix stability.
- **How paw differs:** paw is a full harness/product, not a context-management framework; cache
  alignment is out of scope for v1 (a noted future idea, see §4).

### LangChain Deep Agents SDK
- Batteries-included agent harness with built-in compression, offloading, subagent isolation.
- **How paw differs:** monolithic Python framework; paw is a composable single binary with the
  drone/brain split and validation as the identity.

### LLMLingua / LLMLingua-2 (Microsoft)
- Foundational: small-model perplexity/token-classification prompt compression; up to 20×
  compression with ~1.5% reasoning loss; cross-model transfer demonstrated.
- **Caveat we heed:** token-level pruning **breaks code/JSON/paths** because individual tokens
  carry structural meaning. This is WHY paw's drone only *selects and quotes verbatim spans* and
  never rewrites tokens — structure cannot be corrupted by construction.

### The Token Company (`bear-2`, YC-backed)
- General-purpose NL compression API, sub-50ms, "one line wraps your OpenAI/Anthropic client."
- **Explicit scope boundary (their words):** *not designed for code or highly structured
  languages (JSON schemas, SQL, config files).* → No product overlap with paw; paw's entire
  domain is the code/tool-output they disclaim.

---

## 2. The honest positioning of paw

paw does **not** claim to invent small-model context compression. paw claims to be:

> the first installable, single-binary coding agent that brings this research-proven pruning
> technique to any open-weight model — with **no training**, **offline capability**, a
> **verified (not merely trusted) compression step**, and **composable Unix stages**.

This is a *productization + engineering-properties* claim, which is (a) true, (b) survives contact
with the literature, and (c) is exactly the kind of "take a proven result and ship it well" work
applied AI labs hire for. Do not overstate it in README or commit messages.

---

## 3. Differentiation table (keep this current)

| Property | SWE-Pruner | Focus | TokenPilot | LLMLingua-2 | Token Company | **paw** |
|---|---|---|---|---|---|---|
| Compresses code/tool-output | yes | yes | yes | poorly (breaks structure) | no (NL only) | yes |
| Needs a trained/custom model | yes (0.6B) | no | no | yes | yes (proprietary) | **no (stock Ollama)** |
| Works offline / air-gapped | no | no | no | partial | no | **yes** |
| Verifies compressor output is real bytes | no | no | no | no | no | **yes** |
| Shipped installable single-binary tool | no | no | no | no | API only | **yes** |
| Composable/pipeable stages | no | no | no | no | no | **yes** |
| Token reduction reported | 23–54% | ~23% | 56–87% | up to 20× | 40–60% | *TBD — measure* |

The last row is empty on purpose: paw must MEASURE its own reduction (see `docs/BENCHMARKS.md`) and
compare honestly. Do not copy others' numbers into paw's README.

---

## 4. Explicitly deferred ideas (post-v1, informed by prior art)
- Prompt-cache prefix stabilization (TokenPilot-style) to compound savings.
- A trained/distilled drone (SWE-Pruner-style) IF the stock-model ablation shows the stock drone
  underperforms — this is a fallback, not the plan.
- Model-controlled on-demand compaction (Focus-style) as an optional brain tool.

---

## 5. Sources (re-verify before public comparison)
- SWE-Pruner: arXiv:2601.16746 ; https://github.com/Ayanami1314/swe-pruner
- Focus / Active Context Compression: arXiv:2601.07190
- TokenPilot: arXiv:2606.17016
- LLMLingua: arXiv:2310.05736 ; LLMLingua-2 (ACL 2024)
- The Token Company: https://thetokencompany.com/
- LangChain Deep Agents: https://docs.langchain.com/oss/python/deepagents/overview
