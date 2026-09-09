# Headless evaluation

Full Work experiments, independent grading, comparisons and optional export: [WORK_EVALUATION.md](WORK_EVALUATION.md).

`gator eval` is a noninteractive, bounded runner for measuring real Gator
model runs. It never opens the TUI or asks a person to approve a command. It
is one foreground process, intended for a CI job, container, or scheduler;
the caller owns concurrency, resource limits, retries, and process lifetime.

The runner creates auditable evidence for a fixed claim. It does **not** prove
general coding ability, safety, or parity with another harness. A finite test
corpus can establish only its published tasks, model configuration, policies,
and environment.

The checked-in `gator-live-smoke` suite is one scored smoke case. It detects a
broken live path, not a quality regression. `gator-core-v1` is the checked-in
four-case capability/safety regression seed: configuration precedence, parser
validation, multi-file patching, and resistance to untrusted repository
instructions. It is deliberately too small and too synthetic for a public
competitive claim.

## Run evaluations

An offline script is useful to regression-test harness mechanics without a
provider. It is not model-quality evidence.

```sh
gator eval ./internal/eval/testdata/greeting \
  --run-id greeting-script-001 \
  --report /tmp/gator-eval/greeting-script-001.json \
  --require-resolved
```

Use `suite` for a real provider. It runs cases and attempts serially; schedule
separate jobs if parallelism is required. Live suites require a nonempty,
immutable `--environment-id` such as the OCI image digest. This makes a caller
state exactly which container supplied Gator, Bubblewrap, language runtimes,
and dependency caches.

```sh
OPENAI_API_KEY=... gator eval suite ./internal/eval/testdata/core-v1 \
  --live --provider openai --model gpt-5.6 \
  --environment-id 'ghcr.io/acme/gator-eval@sha256:IMAGE_DIGEST' \
  --run-id core-20260830-001 --attempts 3 \
  --report-dir /tmp/gator-eval/core-20260830-001 \
  --require-resolved
```

`--attempts` accepts 1 through 10 and creates an independent fresh baseline
and report for each case/attempt pair. Reports are written as
`REPORT_DIR/CASE_ID/attempt-NN.json`; the aggregate is
`REPORT_DIR/suite.json`. `--require-resolved` writes all completed reports,
then exits nonzero unless every attempt resolves and its hidden scorer passes.

`--provider`, `--model`, and `--base-url` override live configuration. With no
override, Gator uses `GATOR_PROVIDER`, `GATOR_MODEL`, then configured defaults.
No credential or raw provider endpoint is written to a report: the endpoint is
stored only as a SHA-256 digest.

## Fixture and scoring contract

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
  "setup": [["go", "env", "GOMOD"]],
  "score": [["go", "run", "{{fixture}}/score/main.go", "--", "{{worktree}}"]],
  "score_timeout_seconds": 120,
  "labels": ["bugfix", "go", "parser", "hidden-score"]
}
```

All repository and scope paths are fixture-relative; parent and absolute paths
are rejected. Fixture copying rejects symlinks and special files, discards Git
metadata, and creates a fresh private Git baseline for every attempt.
`verify` commands are completion evidence that the agent can see.
`allowed_commands` grants one exact exploratory argv; each prefix is literal,
anchored, and rejects shell and wrapper launchers. All other model-requested
commands are denied in a headless run. `setup` is fixture-author code that runs
before the model, so it must be fixed-argv, minimal, and trusted.

`score` is a post-run oracle. It runs after the agent in the retained worktree,
including if the visible verifier failed. Its argv is never included in the
model prompt. Only `{{fixture}}` and `{{worktree}}` may be expanded. The
fixture directory is read-only and becomes visible only to the scorer; it is
not part of the repository copied into the agent worktree. A score failure
changes an otherwise resolved agent outcome to `unresolved`; a scoring error
is an `error`.

“Hidden” here means hidden from the evaluated agent, not secret from benchmark
authors or a user who has this source tree. Scorer code is trusted benchmark
code. It must be reviewed like CI code: it can write temporary tests in the
worktree and execute the configured language runtime. Do not put credentials
or network-dependent secrets in it. Live suites are rejected unless every
fixture has at least one score command and requests `sandbox: strict`.

The suite runner disables writer delegation, global extensions, repository
hooks, MCP servers, and LSP servers. The sandbox/network fields describe the
requested fixture policy. A provider API request originates in Gator's control
process; `network: deny` applies to agent tools and scorer commands, not to
that provider request.

Every core fixture carries an `oracle.patch`. The repository test
`TestCoreSuiteOraclesDistinguishBaselineFromReferenceSolution` checks that the
unmodified fixture fails its hidden scorer and that its reference patch passes
it under the strict scorer. This is a soundness check for the fixture—not
evidence that a model can solve it.

`suite.json` is a versioned list of case directories below its directory:

```json
{"version": 1, "id": "go-regression-v1", "cases": ["parser-001", "api-002"]}
```

## Reports and interpretation

Each report is atomically published with POSIX mode `0600`. It includes the
task and policy, fresh baseline commit, fixture SHA-256, Gator version and
commit, environment id, hashed provider endpoint, suite id, attempt number,
labels, visible verifier, hidden score argv/results, duration, retained
worktree/state paths, and final model text. `agent_status` records the result
before hidden scoring; `status` is the final result. Suite reports retain all
attempt records and per-case aggregates; they do not collapse a flaky model
into one selected success.

Reports and retained worktrees can contain source code and model text. Do not
publish them blindly. The report schema currently does not normalize provider
token use or cost; retain provider-side usage separately if it is required for
a comparison.

A valid quality comparison holds fixed:

1. suite id and fixture digest, including the hidden scorer;
2. model identifier, provider endpoint, prompt/harness commit, step and time
   budgets, and tool policy;
3. immutable container identity and resource limits; and
4. number of attempts, fresh baseline per attempt, and the aggregation rule.

For an operational regression gate, run at least three attempts during rapid
development and five or more before a release comparison. Compare hidden-score
passes by case and label, not only an all-cases total: a gain in one category
must not mask a loss in another. Treat a small change in a stochastic model as
an investigation signal, not a statistically proven regression. Preserve raw
attempt reports so an analysis can report the pass rate and a confidence
interval instead of only a percentage.

## Building credible evidence

Use `gator-core-v1` as a regression seed, then grow a versioned corpus with
reviewed real bugs and feature requests from repositories Gator is intended to
serve. A credible corpus needs a development split and a held-out split. Do
not tune the harness, prompt, or model against held-out tasks; when a task is
changed, publish a new suite version and rerun the whole version. Include
multi-file changes, repository exploration, test failures that need diagnosis,
build/toolchain variation, and policy-refusal cases. Label them before looking
at results.

For every new task, require all of the following before it affects a quality
score:

- a containerized, reproducible starting state with no host socket access;
- a task statement reviewed for ambiguity and leakage;
- a visible verifier plus an independent post-run oracle where feasible;
- a baseline that fails the oracle and a reference solution that passes it;
- review that the oracle tests behavior rather than a particular patch shape;
- exact provenance and raw attempt reports; and
- the same task, budgets, and evaluation environment for every harness being
  compared.

This mirrors the useful parts of established coding-agent evaluation practice:
containerized task environments with oracle tests, checked reference solutions,
and separate failure-to-pass/pass-to-pass validation. It avoids the common
mistake of treating a model-visible test suite or a scripted golden patch as a
measure of model quality.

## Container operation

Run Gator inside an externally managed container. Do not give it Docker or a
container-runtime socket. Mount fixtures read-only, mount reports and state
writable, pass provider credentials through the platform secret mechanism, and
limit the container's egress to the selected provider endpoint. The image needs
Gator, Git, Bubblewrap for Linux strict sandboxing, and every language runtime
and dependency cache required by the fixtures.

```sh
docker run --rm \
  --read-only --cap-drop=ALL --security-opt=no-new-privileges \
  --pids-limit=512 --memory=8g --cpus=4 \
  --tmpfs /tmp:rw,nosuid,nodev,noexec,size=8g \
  --mount type=bind,src="$PWD/internal/eval/testdata/core-v1",dst=/fixtures,readonly \
  --mount type=bind,src="$PWD/eval-reports",dst=/reports \
  --mount type=volume,src=gator-eval-state,dst=/state \
  --env OPENAI_API_KEY --env GATOR_STATE_DIR=/state \
  ghcr.io/acme/gator-eval@sha256:IMAGE_DIGEST \
  gator eval suite /fixtures --live --provider openai --model gpt-5.6 \
    --environment-id 'ghcr.io/acme/gator-eval@sha256:IMAGE_DIGEST' \
    --run-id ci-core-001 --attempts 3 --report-dir /reports --require-resolved
```

The container network policy must allow the selected provider or a live run
cannot start. Bubblewrap must be usable inside the image for `sandbox: strict`;
Gator fails closed otherwise. The outer container is a second isolation layer,
not a substitute for recording the requested strict policy. Apply a job-level
timeout as well as each case's `timeout_seconds` and scorer timeout.

There is no evaluation daemon, interactive terminal registry, or detached
writer team to outlive the job. A scheduler should invoke this same foreground
command and collect the immutable report directory as its artifact.
