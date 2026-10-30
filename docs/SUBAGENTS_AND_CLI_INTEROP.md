# Subagents and external CLI adapters

Gator can discover, start, inspect, await, steer, answer and cancel bounded specialists.
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

Live steering works for internal LLM specialists and Codex/Pi. Codex/Pi/Claude
follow-ups now resume the same native conversation and private checkout. The
manager selects the completed task with `start_agent`'s `continue` field; it
cannot supply an arbitrary native thread ID. Continuation keeps the original
source capture and permission envelope, and cannot select a new baseline patch.
Claude supports structured questions and native interruption, but rejects
active-turn steering. Send a follow-up after interrupting or completing its turn.

Codex/Claude clarification requests appear in task details as `native.questions` and
`native.question_id`. The manager can use `answer_agent`, or the user can answer
from another terminal (replace QUESTION_ID with that exact current token):

```sh
gator work answer-task RUN_ID TASK_ID QUESTION_ID '{"format":["Markdown"]}'
# Use - instead of the JSON argument to read answers from stdin.
```

Answers are bound to the current question and queued once. Approval requests
are never converted into clarification questions or automatically approved.
Codex gets a host-defined `gator_ask_user` tool that works in ordinary coding
turns; its built-in `request_user_input` path is also handled. For the host tool,
the answer key is `answer`. Neither path adds filesystem or execution authority.
Claude answers map host question keys back to the original question text; the
original questions are included in the protocol response without granting
additional tool permissions. Multi-select answers can be sent as comma-joined
labels in the answer text. Claude's user-message delivery has no native RPC
acknowledgement; the journal distinguishes delivery from acknowledgement and
waits for a terminal result before recording turn completion.
Pi currently asks ordinary clarification questions in its response text; it
does not expose Codex's structured pending-question protocol.

## Execution boundary and limits

- Each CLI runs in a private source copy under Gator's strict OS sandbox.
  macOS uses Seatbelt; Linux requires usable bubblewrap. Windows currently
  refuses external CLI execution because Gator has no strict Windows sandbox.
- Credentials, private HOME/cache, process cancellation, source write scopes,
  deadlines and bounded output are handled by Gator. One-shot private copies are
  removed after execution. Native sessions retain their private checkout and
  CLI history under `gator/cli-sessions` in the profile state directory, in
  addition to summaries and reviewed patches. Task cancellation kills
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
- One-shot adapters have a 16 MiB output limit and 16 observed assistant-message
  limit. Native sessions have a 64 MiB process-output limit and a lifetime
  admission budget of 256 commands. Both limit JSON lines to 1 MiB and summaries
  to 32 KiB. Claude also gets its native max-turn limit. These are **not** hard
  counts of provider requests.
- CLI provider costs and request counts are separate from Gator's model budget.
  One-shot reported token usage is retained per task; native-session token usage
  is not yet collected. `model_requests: 0` in CLI
  evidence means unavailable, not zero billed requests. Wall/output limits
  cannot enforce a strict dollar or token ceiling inside another CLI.
- Native session metadata and trusted Git files are outside the worker's
  writable roots. A process-held lease prevents concurrent owners. Commands
  are journaled before delivery; uncertain commands and orphaned owners block
  resumption until reconciliation. Automatic crash recovery is not yet wired.
- Workers still belong to the foreground Work operation. Closing that owner
  stops its children; this is not yet an always-on background task service.

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

Reports: `scripts/cli-agent-eval/results/`. These are five original smoke checks, not a
reliability benchmark or evidence of parity with other assistants. Claude has
protocol coverage but was not run live: it was not installed and no Anthropic
credential was supplied. Claude native sessions, resumption, questions,
permission denial and interruption have deterministic protocol/process coverage;
they still require verification against a real authenticated Claude CLI.

The separate `session-manager-*` reports exercise a **live Gator manager and
live Codex/Pi worker** through the normal Work service. Each run makes two
assignments with a native process restart between them. Both providers retained
the same native conversation and recalled the codeword, owner and deadline
without those facts being copied into the follow-up. Original files remained
unchanged. Each final report passes six checks; this is two scenarios, not
twelve independent reliability trials. The initial Codex report is retained:
its date-format-only grader failure was fixed and the scenario passed on rerun.
The `cross-chat-*` reports split the assignments across two Work conversation
turns. Initial runs exposed first-turn ownership being assigned before the new
conversation ID was available. Both providers pass after that fix, with seven
checks per scenario. Failed reports are retained alongside successful reruns.
The `native-controls-*` reports add live Codex structured questions, live
Codex/Pi steering, native interruption and resumption after interruption. Codex
passes five checks in one scenario; Pi passes four in one scenario. Pi's initial
failure is retained: its aborted assistant message was incorrectly classified
as failure because the final settled event carries no abort flag. That mapping
was corrected and the live scenario passed on rerun. These control tests invoke
the native session runner; they do not prove the terminal user journey.
The terminal question UI and cross-chat background
ownership remain part of the active continuity plan.

To repeat the opt-in live adapter checks with synthetic state:

```sh
go run ./scripts/cli-agent-eval --kind codex --model gpt-5.6 --base-url https://us.api.openai.com/v1
go run ./scripts/cli-agent-eval --kind pi --model gpt-5.6 --base-url https://us.api.openai.com/v1 --edit
go run ./scripts/cli-agent-eval --session-manager --kind codex --model gpt-5.6 --base-url https://us.api.openai.com/v1
go run ./scripts/cli-agent-eval --session-manager --cross-chat --kind pi --model gpt-5.6 --base-url https://us.api.openai.com/v1
go run ./scripts/cli-agent-eval --controls --kind codex --model gpt-5.6 --base-url https://us.api.openai.com/v1
```

These commands use `OPENAI_API_KEY` from the current environment. They do not
change user configuration or personal memory. The internal-manager smoke test
uses `GATOR_LIVE_SUBAGENT_REPORT` (a new report filename), `GATOR_LIVE_MODEL`,
`GATOR_LIVE_ENDPOINT` and `OPENAI_API_KEY`:
`go test ./internal/workrun -run '^TestLiveSubagentManager$' -count=1`.

Protocol references checked against installed CLI help and official docs:

- [Codex noninteractive execution](https://learn.chatgpt.com/docs/non-interactive-mode)
- [Codex app-server](https://learn.chatgpt.com/docs/app-server)
- [Codex CA environment configuration](https://learn.chatgpt.com/docs/config-file/environment-variables)
- [Claude Code headless execution](https://code.claude.com/docs/en/headless)
- [Claude streaming sessions](https://code.claude.com/docs/en/agent-sdk/streaming-vs-single-mode)
- [Claude clarification protocol](https://code.claude.com/docs/en/agent-sdk/user-input)
- [Anthropic SDK control implementation](https://github.com/anthropics/claude-agent-sdk-python/blob/main/src/claude_agent_sdk/_internal/query.py)
- [Pi CLI integration](https://pi.dev/docs/latest/cli-integration)
- [Pi JSON stream](https://pi.dev/docs/latest/json)
- [Pi RPC sessions](https://pi.dev/docs/latest/rpc)
- [Pi model configuration](https://pi.dev/docs/latest/models-configuration)
