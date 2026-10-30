# Subagents and external CLI adapters

Gator can discover, start, inspect, await, steer and cancel bounded specialists.
It retains task status, inputs, summaries, provenance and patch evidence. Three
specialists can run concurrently; the default delegation budget is eight tasks.
Dependencies and explicit continuation from a retained result are supported.
Restarting does not silently replay interrupted tasks.

## Configure an installed CLI

```sh
gator agent cli list
gator agent cli enable codex --model gpt-5.6
gator agent cli enable pi --provider openai --model gpt-5.6
gator agent cli enable claude --api-key-env ANTHROPIC_API_KEY
gator agent cli doctor codex
gator agent cli disable codex
```

Adapters are off until enabled. Install the CLI separately. Export the selected
API key variable in the environment that launches Gator. Configuration stores
only the variable name. Saved CLI logins and OAuth sessions are not imported.
`doctor` checks prerequisites, not provider authentication or protocol compatibility.
Restart an existing Gator session after changing adapter settings.

For a regional OpenAI project, add `--base-url https://us.api.openai.com/v1` to
Codex/Pi setup. Other HTTPS API base URLs are supported. They are explicit
credential destinations, so configure only a provider endpoint you trust.

`--executable PATH` selects a custom installation. `--runtime-root PATH` adds a
read-only runtime dependency; avoid broad grants such as your entire home.
`--timeout 300` sets the per-invocation wall limit (10–1800 seconds).

By default, agents only inspect their private source copy. Add `--allow-edits`
when enabling an adapter to permit patch creation in draft/act runs. Inspect
mode remains read-only. Changes are returned as retained patch artifacts;
Gator's `integrate_code` checks selected patches together before subsequent
assignments can use the verified candidate. Nothing applies to live source
merely because an external CLI reports success.

## Use from Gator

Enabled adapters appear as `cli_codex`, `cli_claude` and `cli_pi` in foreground
chat. For example:

> Ask cli_codex to review this project's error handling and source_researcher to
> check the documentation. Run them independently, then compare their findings.

Noninteractive Work requires a separate opt-in:

```sh
gator work --source ./project --mode inspect --external-agents \
  'Use cli_pi to inspect project.md and report the owner and deadline.'
```

For patch creation, enable `--allow-edits` and use a draft/act Work run:

```sh
gator work --source ./project --external-agents \
  'Ask cli_codex to fix the parser. Integrate its patch and write report.md with verification evidence.'
```

Scheduled jobs do **not** inherit adapters from settings. Their existing model
budgets cannot account for an external CLI's internal provider calls.

Inspect or control running tasks using the existing task commands:

```sh
gator work tasks RUN_ID
gator work tasks RUN_ID TASK_ID
gator work cancel-task RUN_ID TASK_ID
gator work steer-task RUN_ID TASK_ID 'Focus on the parsing failure.'
```

Live steering works for internal LLM specialists. External CLI tasks support
inspection, waiting and cancellation; a follow-up uses a **new invocation with
a selected retained summary**, not native Codex/Claude/Pi session resumption.

## Execution boundary and limits

- Each CLI runs in a private source copy under Gator's strict OS sandbox.
  macOS uses Seatbelt; Linux requires usable bubblewrap. Windows currently
  refuses external CLI execution because Gator has no strict Windows sandbox.
- Credentials, private HOME/cache, process cancellation, source write scopes,
  deadlines and bounded output are handled by Gator. Private CLI copies are
  removed after execution; summaries and reviewed patches are retained. Task cancellation kills
  the owned Unix process group. A hostile executable that deliberately escapes
  its process group is outside this process-management guarantee.
- Codex runs with its internal sandbox set to `danger-full-access` **inside
  Gator's already-enforced strict sandbox**. Nested Seatbelt cannot initialize
  reliably on macOS. Gator never starts it unsandboxed. Its native multi-agent
  feature is disabled, and unattended approval requests are denied.
- Claude uses bare mode with a read/edit tool allowlist; Pi disables project
  trust, extensions, skills, context loading and prompt templates. Neither is
  granted a shell tool. Codex may run commands inside Gator's OS boundary.
- Network access is enabled for model calls. It is not a domain-filtered
  network sandbox. Instructions prohibit unrelated external actions, but that
  instruction is not a technical egress restriction.
- The adapters require successful structured terminal events and independently
  export actual file diffs. Trusted Git metadata stays outside child access.
  Failed, incomplete or malformed responses do not count as successful work.
- There is a 16 MiB output limit, 1 MiB JSON-line limit, 32 KiB adapter summary
  limit and 16 observed assistant-message limit. Assistant messages are **not**
  a hard count of provider requests. Claude also gets its native max-turn limit.
- CLI provider costs and request counts are separate from Gator's model budget.
  Reported CLI token usage is retained per task; `model_requests: 0` in CLI
  evidence means unavailable, not zero billed requests. Wall/output limits
  cannot enforce a strict dollar or token ceiling inside another CLI.

## Validation

Local protocol and sandbox tests cover discovery, permissions, selected
baselines, isolated patch export, protected Git metadata, cancellation,
malformed results, secret-bearing patches and integration into verified
candidates. CLI configuration and Work opt-in are covered separately.

Live synthetic checks on 2026-10-30 with OpenAI `gpt-5.6`:

| Check | Result |
| --- | --- |
| Gator manager starts two internal researchers and combines their findings | Pass |
| Codex reads source, returns correct owner/deadline, leaves original unchanged | Pass |
| Codex produces a reviewable status-change patch | Pass |
| Pi reads source, returns correct owner/deadline, leaves original unchanged | Pass |
| Pi produces a reviewable status-change patch | Pass |

Reports: `scripts/cli-agent-eval/results/`. These are five smoke checks, not a
reliability benchmark or evidence of parity with other assistants. Claude has
protocol coverage but was not run live: it was not installed and no Anthropic
credential was supplied. Native CLI interactive sessions and live CLI steering
are not implemented.

To repeat the opt-in live adapter checks with synthetic state:

```sh
go run ./scripts/cli-agent-eval --kind codex --model gpt-5.6 --base-url https://us.api.openai.com/v1
go run ./scripts/cli-agent-eval --kind pi --model gpt-5.6 --base-url https://us.api.openai.com/v1 --edit
```

These commands use `OPENAI_API_KEY` from the current environment. They do not
change user configuration or personal memory. The internal-manager smoke test
uses `GATOR_LIVE_SUBAGENT_REPORT` (a new report filename), `GATOR_LIVE_MODEL`,
`GATOR_LIVE_ENDPOINT` and `OPENAI_API_KEY`:
`go test ./internal/workrun -run '^TestLiveSubagentManager$' -count=1`.

Protocol references checked against installed CLI help and official docs:

- [Codex noninteractive execution](https://learn.chatgpt.com/docs/non-interactive-mode)
- [Codex CA environment configuration](https://learn.chatgpt.com/docs/config-file/environment-variables)
- [Claude Code headless execution](https://code.claude.com/docs/en/headless)
- [Pi CLI integration](https://pi.dev/docs/latest/cli-integration)
- [Pi JSON stream](https://pi.dev/docs/latest/json)
- [Pi model configuration](https://pi.dev/docs/latest/models-configuration)
