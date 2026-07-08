# paw — Benchmarks (`docs/BENCHMARKS.md`)

How `paw` is evaluated. Local, model-free stage benchmarks come first. Publishable external
benchmarks use **Terminal-Bench 2.0** via **Harbor** after a credible smoke pass with a
benchmark-capable model. **SWE-bench Verified** is secondary and stays deferred until the
Terminal-Bench raw/full comparison is credible. All facts verified against Harbor/Terminal-Bench
docs as of 2026-07; re-verify versions before a leaderboard submission.

---

## 1. The harness is Python; `paw` is a binary it drives

Terminal-Bench 2.0 runs on **Harbor** (`pip install harbor` / `uv tool install harbor`). Harbor
launches each task in a **Docker sandbox**, hands the agent an instruction, lets it act, then runs
the task's `tests/test.sh`, which writes `reward.txt` (`1` = pass, `0` = fail) and `ctrf.json`.

Custom agents integrate by subclassing **`BaseInstalledAgent`** (for CLI tools installed into the
container). We provide that subclass in `adapters/harbor/`. Reference implementation to mirror:
Harbor's own `claude_code.py` installed agent.

Run form (local Docker):
```
export PAW_BRAIN_BASE_URL=https://api.z.ai/api/paas/v4
export PAW_BRAIN_API_KEY=...        # brain key
export PAW_BRAIN_MODEL=glm-4.6
harbor run \
  --dataset terminal-bench@2.0 \
  --agent-import-path paw_harbor:PawAgent \
  --model openai/glm-4.6 \
  --n-concurrent 4
```

For the leaderboard, results are produced with `--k 5` (5 rollouts) and a `--jobs-dir`; the
official leaderboard audits config and trajectories, so we keep the run config in-repo.

---

## 2. The `BaseInstalledAgent` subclass contract

File: `adapters/harbor/src/paw_harbor/agent.py`. Methods to implement (signatures per Harbor
`BaseInstalledAgent` / `BaseAgent`):

```python
from harbor.agents.installed.base import BaseInstalledAgent, with_prompt_template
from harbor.environments.base import BaseEnvironment
from harbor.models.agent.context import AgentContext

class PawAgent(BaseInstalledAgent):
    @staticmethod
    def name() -> str:
        return "paw"

    def version(self) -> str | None:
        return "0.1.0"

    async def install(self, environment: BaseEnvironment) -> None:
        # 1. Install runtime deps the paw binary needs inside the sandbox:
        #    ripgrep, git, build-essential, universal-ctags (best-effort), ca-certificates.
        await self.exec_as_root(environment,
            command="apt-get update && apt-get install -y ripgrep git ca-certificates universal-ctags")
        # 2. Copy the prebuilt static linux/amd64 paw binary into the container.
        #    The binary is built by `make build-linux` and shipped in the adapter package
        #    (or fetched from a release URL); place at /usr/local/bin/paw and chmod +x.
        await environment.upload_file(Path("bin/paw-linux-amd64"), "/usr/local/bin/paw")
        await self.exec_as_root(environment, command="chmod +x /usr/local/bin/paw")

    @with_prompt_template
    async def run(self, instruction: str, environment: BaseEnvironment,
                  context: AgentContext) -> None:
        # Pass brain/drone config through env vars already present in the container's env,
        # or inject them here. The drone runs API-mode in-sandbox (no local GPU in CI):
        #   PAW_DRONE_TRANSPORT=openai, cheap model, same or separate key.
        # Invoke paw non-interactively. paw MUST:
        #   - read the instruction (passed as arg or stdin),
        #   - operate on /workspace (the task cwd Harbor sets),
        #   - exit 0 when it believes it is done.
        cmd = (
            "cd /workspace && "
            "PAW_NONINTERACTIVE=1 paw run "
            "--instruction-file /tmp/paw_instruction.txt "
            "--max-turns 40 --trace-file /workspace/.paw/trace.ndjson"
        )
        # Write instruction to a file to avoid shell-escaping issues:
        await environment.write_file(Path("/tmp/paw_instruction.txt"), instruction)
        await self.exec_as_agent(environment, command=cmd)
        # Populate context with metrics for observability (token counts from the trace):
        # (read /workspace/.paw/trace.ndjson, sum brain_input_tokens, set on context if the
        #  AgentContext model exposes fields / metadata for it.)
```

Notes:
- Harbor calls `setup()`→`install()` then `run()`. Do NOT put task-solving logic in Python; the
  Python file only installs and launches the Go binary. All intelligence is in `paw`.
- Harbor's verifier runs `tests/test.sh` after `run()` returns and produces `reward.txt`. `paw`
  must therefore leave the workspace in the final state (apply patches to real files), which it
  does via the deterministic `patch` stage.
- Known Harbor gotcha (documented in the pi adapter): if the agent creates a `/tests` dir during
  the run, `docker cp` can misplace the verifier's test files. `paw` MUST NOT create a top-level
  `tests/` dir in the workspace during a run; write all scratch under `.paw/`.

---

## 3. Adapter package layout

```
adapters/harbor/
├── pyproject.toml            # package name paw-harbor, entry module paw_harbor
├── README.md                 # exact run commands + env vars
├── bin/paw-linux-amd64       # built by `make build-linux` (git-ignored; CI or make produces it)
└── src/paw_harbor/
    ├── __init__.py           # exports PawAgent
    └── agent.py              # the BaseInstalledAgent subclass above
```

Install for local runs: `uv tool install harbor` then `uv pip install -e adapters/harbor`.

---

## 4. `paw bench` (external harness wrapper)

`cmd/bench.go` shells out to `harbor run` with the flags above, then parses the Harbor `jobs-dir`
output (`results/**/result.json`, `reward.txt`) plus `paw`'s own NDJSON traces to emit a table:

| run_id | commit | config | dataset | brain_model | drone_model | hardware | date | tasks | wall_time | tokens_brain_in | tokens_brain_out | tokens_drone | pass_rate | trace_bundle |

Configs `bench` can sweep (the ablation that makes claims attributable):
- `full` — compress ON (drone), brain = GLM.
- `no-compress` — `--disable-compress` (heuristic fallback only; no drone) → isolates the
  drone's contribution to both pass-rate and brain-token reduction.
- `raw` — `--raw-context` (brain reads full `RawContext`, no compression at all) → the upper
  bound on brain tokens, the control the token-savings % is computed against.

Results land in `docs/RESULTS.md`. Every recorded row must include reproducibility metadata:
full commit SHA, checked-in config path under `docs/results-configs/<run-id>.toml`, dataset id and
revision, exact brain/drone model ids, hardware, ISO date, total wall time, total brain/drone token
counts, brain/drone token source (`provider`, `estimate`, or `mixed`) in the generated result
config, pass rate with 95% CI when sample size is sufficient, and trace bundle path or URL.

`paw bench` refuses to write `docs/RESULTS.md` rows when a requested `--n-tasks`/`--n-attempts`
run is incomplete or Harbor marks the root job unfinished. Partial or interrupted runs belong in
issue comments or diagnostic notes, not in the measured results table.

Local stage benchmarks are separate from Harbor results. They should test context-gateway depth,
compression validation, trace/stat accounting, and bounded tool execution without Docker or model
keys. They can gate development; Harbor rows are for publishable external evidence only.

### 4a. Direct comparison vs SWE-Pruner (prior art)
Because SWE-Pruner is the closest prior art and publishes on SWE-bench Verified with GLM-4.6,
`docs/RESULTS.md` includes a dedicated comparison section. The honest comparison holds the
**model (GLM), benchmark, and reporting method** constant and contrasts:
- **paw `full`** (stock-Ollama drone + validation) vs **paw `raw`** (no compression) → paw's own
  reduction %, measured, not borrowed.
- paw's reduction % placed beside SWE-Pruner's published 23–54% range as *context*, with an
  explicit note that paw uses **no trained model** whereas SWE-Pruner trains a 0.6B skimmer — so
  the comparison is "stock+validated vs trained," not apples-to-apples on raw ratio alone.
- If feasible, run SWE-Pruner's public repo (github.com/Ayanami1314/swe-pruner) on the same task
  subset for a true side-by-side; if not feasible (their harness differs), cite their numbers and
  clearly label them as reported-by-authors, not reproduced. NEVER present borrowed numbers as
  paw's own.
Do NOT claim paw beats SWE-Pruner on token ratio unless measured head-to-head; paw's honest edge
is no-training + offline + verified + installable, not necessarily a higher ratio.

---

## 5. Pinned versions (re-verify before submission)
- Harbor: latest published (`uv tool install harbor`); dataset `terminal-bench@2.0`.
- Sandbox: local Docker for dev; Daytona/Modal `--env` for scaled runs (optional, needs keys).
- SWE-bench Verified: Harbor dataset `swe-bench/swe-bench-verified@latest`, verified on
  2026-07-07 with Harbor `0.17.1` via `harbor run --print-config --dataset
  swe-bench/swe-bench-verified@latest`, resolving to name `swe-bench/swe-bench-verified` and ref
  `latest`. Harbor Hub listed this dataset with 500 tasks on the same date. Do not publish
  SWE-bench rows until Terminal-Bench raw/full smoke runs show a nonzero credible pass signal with
  the intended benchmark model family.

### 5a. SWE-bench Verified

`paw bench` is dataset-agnostic once Harbor exposes the dataset as normal tasks. This repo pins the
current Harbor registry id to `swe-bench/swe-bench-verified@latest`; re-run the print-config check
before publishing any SWE-bench result. Use this only after Terminal-Bench raw/full smoke runs are
credible with a benchmark-capable model; it is not a near-term local development gate.

```sh
make build-linux
make bench-swebench
```

The target runs `raw` and `full` configs through the same `PawAgent` adapter and writes rows into
`docs/RESULTS.md`. Record the exact dataset id, Harbor version, and model env used with any result.

### 5b. Local Terminal-Bench harness validation

Verified on 2026-07-07:
- Harbor `0.17.1`; Docker client `29.6.1`, server `29.5.3`; Ollama `0.31.1`.
- `make build-linux` produced `bin/paw-linux-amd64`.
- Oracle job `.paw/bench-jobs/paw-oracle` completed on `terminal-bench@2.0` with reward `1.0`
  and `0` exceptions.
- PawAgent job `.paw/bench-jobs/paw-tb-harness-smoke-openai-qwen` completed 1 task with `0`
  exceptions, reward `0.0`, trial `gpt2-codegolf__RHEScQF`.
- PawAgent env shape: `PAW_BENCH_CONFIG=no-compress`, brain transport `openai` to local Ollama
  OpenAI-compatible endpoint `http://host.docker.internal:11434/v1`, model `qwen3:0.6b`, dummy
  local API key, trace artifact `/workspace/.paw/trace.ndjson`.
- Local trace artifact downloaded to
  `.paw/bench-jobs/paw-tb-harness-smoke-openai-qwen/gpt2-codegolf__RHEScQF/artifacts/workspace/.paw/trace.ndjson`.

---

## 6. Success criteria for publishable Harbor results
1. `harbor run --agent oracle` passes locally (proves Docker + Harbor work).
2. Local, model-free stage benchmarks pass for gather/compress/plan/edit/verify contracts.
3. `PawAgent` installs the binary and completes at least one task end-to-end with
   `reward.txt == 1` using the intended benchmark model family.
4. `paw bench --config raw` and `--config full` both complete the same 5-task smoke subset and
   produce a comparison table with a nonzero credible pass signal.
5. Only then schedule full Terminal-Bench or SWE-bench runs and record completed rows in
   `docs/RESULTS.md`.
