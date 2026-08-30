# Headless evaluation

`gator eval` is a noninteractive, bounded runner for measuring real Gator
model runs. It never opens the TUI or requests command approval. It is designed
to run as one foreground process in a CI job, container, or scheduler; the
caller owns retries, concurrency, resource limits, and process lifetime.

This runner makes quality evidence possible. It is not quality evidence by
itself. A scripted fixture only tests harness mechanics. The checked-in
`gator-live-smoke` suite is intentionally a smoke case, not a benchmark.

## Run a fixture

An offline script is useful for regression-testing the runner without a
provider. It must not be used as evidence of model task-solving ability.

```sh
gator eval ./internal/eval/testdata/greeting \
  --run-id greeting-script-001 \
  --report /tmp/gator-eval/greeting-script-001.json \
  --require-resolved
```

Use `suite` for a real provider. It is serial by design: a job scheduler can
run independent suite attempts in parallel without hidden in-process workers.

```sh
OPENAI_API_KEY=... gator eval suite ./internal/eval/testdata \
  --live --provider openai --model gpt-5.6 \
  --run-id live-smoke-001 \
  --report-dir /tmp/gator-eval/live-smoke-001 \
  --require-resolved
```

`--provider`, `--model`, and `--base-url` override configuration for live
runs. With no override, Gator uses `GATOR_PROVIDER`, `GATOR_MODEL`, then its
configured defaults. `--require-resolved` writes every completed report first,
then exits nonzero unless the requested run or every suite case resolved.

Each invocation creates a private temporary fixture copy, removes fixture
`.git` metadata, rejects symlinks and special files, initializes a new Git
baseline, and lets Gator create a retained worktree from that baseline. The
source fixture is never the worktree. Reports contain the baseline commit,
fixed task, tool policy, verifier, model identity, result, and retained local
paths. They are published atomically with POSIX mode `0600`.

Reports and worktrees can contain task source and model text. Do not publish
them blindly and do not put credentials in a task, command, setup field, or
fixture file.

## Fixture contract

A case directory contains `eval.json` and a regular-file `repository/` tree.
`script.json` is optional and only used by an offline single-fixture run.

```json
{
  "version": 1,
  "id": "bugfix-001",
  "task": "Fix the failing parser case and add a focused regression test.",
  "max_steps": 24,
  "timeout_seconds": 600,
  "verify": [["go", "test", "./..."], ["go", "vet", "./..."]],
  "sandbox": "strict",
  "network": "deny",
  "repository": "repository",
  "scopes": ["internal/parser"],
  "allowed_commands": [["go", "env", "GOMOD"]],
  "allowed_command_prefixes": [["go", "test"], ["go", "vet"]],
  "setup": [["go", "env", "GOMOD"]]
}
```

All repository and scope paths are fixture-relative; parent and absolute paths
are rejected. `verify` commands are required completion evidence.
`allowed_commands` grants one exact exploratory argv; each prefix is literal,
anchored, and rejects shell and wrapper launchers. All other model-requested
commands are denied in a headless run. `setup` is different: it is a
fixture-author command run before the model. Treat it as trusted code, keep it
fixed argv, and keep it minimal. The suite runner disables writer delegation,
global extensions, repository hooks, MCP servers, and LSP servers.

The report's sandbox and network fields describe the requested fixture policy.
Repository policy can only make execution more restrictive. A model provider
request originates in Gator's control process; `network: deny` applies to agent
tools and sandboxed commands, not to the provider API call itself.

`suite.json` is a versioned list of case directories below its own directory:

```json
{"version": 1, "id": "go-regression-v1", "cases": ["parser-001", "api-002"]}
```

## Container operation

Run Gator inside an externally managed container. Do not give it Docker or
container-runtime socket access. Mount fixtures read-only, mount reports and
state writable, pass a provider credential as the platform's secret mechanism,
and restrict egress outside the container to the provider endpoint.

The image needs Gator, Git, Bubblewrap for Linux strict sandboxing, and every
language runtime and dependency cache needed by the fixtures. A representative
Docker invocation is:

```sh
docker run --rm \
  --read-only --cap-drop=ALL --security-opt=no-new-privileges \
  --pids-limit=512 --memory=8g --cpus=4 \
  --tmpfs /tmp:rw,nosuid,nodev,noexec,size=8g \
  --mount type=bind,src="$PWD/internal/eval/testdata",dst=/fixtures,readonly \
  --mount type=bind,src="$PWD/eval-reports",dst=/reports \
  --mount type=volume,src=gator-eval-state,dst=/state \
  --env OPENAI_API_KEY --env GATOR_STATE_DIR=/state \
  gator-eval:local gator eval suite /fixtures \
    --live --provider openai --model gpt-5.6 \
    --run-id ci-smoke-001 --report-dir /reports --require-resolved
```

The container's network policy must allow the selected provider or the live run
cannot start. Keep tool network denied in the fixture unless a task truly
requires it. Bubblewrap must be usable inside the chosen runtime for
`sandbox: strict`; Gator fails closed if it is not. Setting `sandbox: off` can
be acceptable only when the outer container is the intentional isolation
boundary, and the resulting report must remain marked `off`; it is not
equivalent to Gator strict sandbox evidence.

For a scheduler, invoke that same foreground command as the Job's command.
There is no eval daemon, interactive terminal registry, or detached writer
team to outlive the job. Apply a job-level timeout in addition to every case's
`timeout_seconds`.

## What qualifies as evidence

Do not check off agent quality until versioned live reports exist for a defined
corpus and fixed model configuration. At minimum, record the Gator commit,
fixture revision, provider/model/base URL, policy, step and time budgets, per-
case outcome, cost or token data if the provider exposes it, and failure
artifacts. Include realistic multi-file feature work, bug fixes with regression
tests, repository exploration, failing-verifier behavior, and at least one
task the agent should decline because policy blocks it. Compare the same corpus
and budgets against the selected competing harnesses; publish aggregate counts
and the raw report schema, not credentials or private source.

A resolved report means only that one model completed one fixed fixture under
that configuration. It does not establish broad coding ability, safety, or
competitive parity.
