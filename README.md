# Gator

Gator is a terminal-native, local-first work agent. Point it at a folder, give
it an outcome, and it returns validated files in isolated output with a sealed,
reviewable evidence manifest. Source material is read-only and model prose is
never treated as proof that work finished.

Gator is the only user-facing agent. Coding is a first-class capability, but
Gator Code is now an internal specialist rather than a second application. The
manager decides when a bounded implementation task is useful, passes a
user-owned capability envelope, and receives a summary, changed paths, and a
patch artifact. It cannot silently widen the child policy or apply the patch to
live source.

The product contract and migration decisions are documented in:

- [Work architecture](docs/WORK.md)
- [Gator orchestration](docs/ORCHESTRATION.md)
- [Code-to-Gator migration map](docs/CODE_TO_GATOR_MIGRATION.md)
- [Terminal design system](docs/DESIGN_SYSTEM.md)

## Status

The repository currently includes immutable local-source snapshots, branching
Work conversations, bounded fresh-context specialists, DOCX/XLSX/PDF writers,
Slack/Google Workspace/Atlassian/Notion connectors, typed remote MCP mappings,
local jobs and inboxes, an internal isolated coding executor, controlled local
browser sessions, native provider adapters, and headless RPC/ACP/evaluation
surfaces.

No GitHub release has been published yet. The
[release-evidence checklist](docs/RELEASE_EVIDENCE.md) remains the human gate
for a daily-driver claim.

## Build

Gator supports macOS and Linux on `amd64` and `arm64`.

```sh
make check
make build
./bin/gator
```

Plain `gator` opens the conversation-first terminal UI. Automation can use the
headless `work`, `inspect`, `review`, `export`, and `apply` commands.

## First run

Submit the work you want to do. If no provider is configured, Gator retains the
prompt and asks for OpenAI, Anthropic, or Gemini setup. It uses the provider's
normal environment variable when present; otherwise it opens a hidden API-key
prompt. A successful setup saves the provider and stable default model before
the original task resumes.

Providers can also be connected explicitly:

```sh
./bin/gator connect openai
./bin/gator connect anthropic
./bin/gator connect gemini
```

Credentials live in Gator's private auth store, never `config.json`. Direct
model requests resolve credentials in this order: an explicit run key, Gator's
credential store, then the provider environment variable.

## Main terminal UI

The initial screen contains only the `🐊 Gator` wordmark and centered composer.
Typing docks the composer below the conversation. `Ctrl+P` opens a searchable
universal palette containing the slash-command surface. Navigation is separate:
`Ctrl+X L` opens retained conversations, `Ctrl+X I` opens the inbox, and
`Ctrl+X J` opens scheduled jobs.

Useful local commands are:

```text
/model                         connect or switch the default model
/effort low|standard|high      set manager and internal Code budgets
/attach PATH                   attach a source-relative file to the next prompt
/detach PATH|all               clear pending attachments
/code status                   inspect the child capability envelope
/code verify COMMAND           add required project verification
/code scope PATH               add a project-instruction scope
/code profile NAME             select a project profile
/code setup COMMAND            add a user-owned setup command
/code allow COMMAND            pre-approve one exact child argv
/code allow-prefix PREFIX      pre-approve a literal argv prefix
/code grant CAPABILITY         grant lsp/mcp/extension/http/browser/terminal
/code revoke CAPABILITY        revoke one child capability
/code sandbox strict|off       set the child process sandbox
/code network deny|allow       set child process network access
/code max-steps N              set the child turn budget
/code browser SESSION          select a controlled browser session
/code reset                    restore strict, offline child defaults
/status · /permissions         inspect the active orchestration envelope
/doctor · /agents · /settings inspect local configuration
/history · /back · /forward   navigate retained revisions
/queue · /dequeue · /clear-queue
/review · /copy · /theme · /new · /quit
```

Prompts entered while a run is active join a bounded 16-item in-memory FIFO
queue. Each queued prompt captures its own options. The queue proceeds only
after a successful revision and pauses on failure; it is never persisted for
automatic execution after restart.

`/attach` is explicit per-send consent. PNG, JPEG, and WebP are visual inputs;
PDF, DOCX, ODT, XLSX, and supported UTF-8 text/data formats use the bounded
attachment loader. Office text is extracted locally. Gator allows at most four
files, 4 MiB per file, and 8 MiB total. Raw attachment bytes are not retained
in Work conversation metadata.

## Headless Work

```sh
OPENAI_API_KEY=... ./bin/gator work \
  --source ./research \
  --artifact report.md \
  'Create a concise source-grounded brief'

./bin/gator inspect --source ./research \
  'Identify the most material inconsistencies'

./bin/gator work --source ./data \
  --artifact analysis.xlsx \
  'Clean the inputs and produce a workbook'
```

Work modes are monotonic:

- `inspect`: source and selected connector reads only;
- `draft`: inspect plus writes to isolated output and proposed actions; and
- `act`: draft plus individually approved connected mutations or publishing.

The default artifact is `report.md`. Explicit paths select deterministic JSON,
CSV, DOCX, XLSX, PDF, text, and containment validation. A failed run still
retains its workspace and manifest for inspection.

## Coding through Gator

Use ordinary Gator prompts for mixed work. `gator code` and `gator run` remain
compatibility spellings for scripts, but both route through the Gator manager
and require successful internal Code patch evidence. Neither opens the retired
Code TUI.

```sh
OPENAI_API_KEY=... ./bin/gator code \
  --verify 'go test ./...' \
  --scope internal/parser \
  --profile implementer \
  'Fix the parser and add focused tests'

# `run` is the same orchestration route.
./bin/gator run --verify 'npm test' \
  --allow-command-prefix 'npm test' \
  'Repair the failing frontend test'
```

The internal Code specialist always receives an isolated Git repository made
from Gator's exact frozen source snapshot. `git diff --check` is always added to
the verifier list. When no verifier is supplied, Gator detects common Go, npm,
Python, and Rust project checks.

Safe defaults are strict sandboxing, denied process network, no integration
capabilities, at most 32 child turns, and no recursive writer/scout delegation.
The following controls are available to the user, not the manager model:

```text
--code-max-steps N
--verify 'argv ...'                 repeatable
--scope PATH                        repeatable
--profile NAME
--setup 'argv ...'                  repeatable
--allow-command 'argv ...'          repeatable exact grant
--allow-command-prefix 'argv ...'   repeatable literal prefix
--sandbox strict|off
--network deny|allow
--code-capability lsp|mcp|extension|http|browser|terminal
--browser-session ID
--image PATH
--attach PATH
```

An integration grant is only one key: its existing project trust,
authentication, and operation-specific approval rules still apply. A browser
session additionally requires `--network allow`, `--code-capability browser`,
and developer-selected tabs. Headless JSON and the main TUI's subprocess path
cannot display a child approval prompt, so exploratory commands and integration
operations need exact pre-grants there.

The former `--base`, `--copy-ignored`, static `--scout`, and
`--trust-commands` Code-frontend controls are retired. Snapshot provenance now
owns source selection, Gator owns specialist parallelism, and unbounded command
trust is not carried into child execution.

Standalone Code `resume`, `fork`, and `clone` are also retired. Continue and
branch through Gator conversations:

```sh
./bin/gator resume
./bin/gator resume WORK_CONVERSATION_ID 'Revise the result'
./bin/gator work history WORK_CONVERSATION_ID
./bin/gator work back WORK_CONVERSATION_ID
./bin/gator work forward WORK_CONVERSATION_ID REVISION_ID
./bin/gator work resume --parent REVISION_ID WORK_CONVERSATION_ID \
  'Try the alternative approach'
```

Historical standalone Code records remain readable by the existing review,
export, apply, and transcript commands; new requests are never written to that
conversation format.

## Review and transfer

Every Work bundle includes source identity, snapshot digest, contract digest,
artifact hashes, validation results, connected-source provenance, external
actions, and specialist evidence. Code patches are bound to the invoking
specialist and SHA-256 digest.

```sh
./bin/gator review WORK_BUNDLE --preview
./bin/gator export WORK_BUNDLE --to gator-work.tar.gz
./bin/gator apply WORK_BUNDLE --to ./destination --check
./bin/gator apply WORK_BUNDLE --to ./destination
```

Work never modifies the selected source directory. Transfer is explicit and
performs manifest verification and target preflight first.

## Providers and model configuration

Set persistent defaults with:

```sh
./bin/gator config set default-provider anthropic
./bin/gator config set default-model claude-sonnet-5
./bin/gator config show
```

Gator includes direct adapters for OpenAI Responses, Anthropic Messages,
Gemini, Azure OpenAI Responses, Amazon Bedrock, Google Vertex, Cloudflare AI
Gateway, Mistral, MiniMax, Kimi Coding, xAI, Radius, OpenRouter, OpenCode
gateways, and configured OpenAI-compatible endpoints. Provider-specific
non-secret options and endpoint overrides stay in `config.json`; secrets stay
in the auth store or explicitly named environment variables.

Vendor coding harnesses are a separate, explicit boundary:

```sh
./bin/gator delegate codex run --verify 'go test ./...' 'Implement the fix'
./bin/gator delegate copilot run --verify 'go test ./...' 'Implement the fix'
./bin/gator delegate claude run --verify 'go test ./...' 'Implement the fix'
./bin/gator delegate kimi run --verify 'go test ./...' 'Implement the fix'
./bin/gator delegate opencode run --verify 'go test ./...' 'Implement the fix'
```

The installed harness owns its tools, approvals, session state, and
credentials. Gator never imports another CLI's credential or silently swaps its
native manager for a vendor harness.

## Trusted project capabilities

Gator loads root and scoped `AGENTS.md`, `.gator/rules.json`, and named profiles
from `.gator/agents.json`. Declarative profiles and roles may only narrow
authority. Executable integrations are hash-pinned separately:

```sh
gator hook status|trust|untrust
gator lsp status|trust|untrust
gator mcp status|trust|untrust|login|logout
gator extension list|status|install
gator browser install|start|attach|tabs|select|origins|visual|stop
```

Strict command execution uses macOS Seatbelt or Linux Bubblewrap, a filtered
environment, private scratch, worktree-only writes, and denied network by
default. Strict mode fails closed when its platform prerequisite is missing.
Git isolation remains a review boundary, not a replacement for process
sandboxing.

See [writer roles](docs/WRITERS.md), [LSP](docs/LSP.md), [MCP](docs/MCP.md),
[extensions](docs/EXTENSIONS.md), [browser sessions](docs/BROWSER.md), and the
[architecture](docs/ARCHITECTURE.md).

## Connectors, jobs, and protocols

Connected reads and mutations use explicit connector descriptors, permissions,
resource-bound authentication, retained provenance, and exact action approval.
See the connector contract in [Work architecture](docs/WORK.md).

Recurring jobs run through a manually started foreground supervisor, cannot
approve external actions, and write immutable attempts to the local inbox. See
[jobs](docs/JOBS.md).

Local integrations can use authenticated loopback HTTP/SSE, JSONL RPC, or ACP
without parsing TUI output:

```sh
gator rpc
gator acp
gator serve token /absolute/private/token-file
gator serve start --token-file /absolute/private/token-file
```

See [RPC](docs/RPC.md), [ACP](docs/ACP.md), and the
[app-server contract](docs/APP_SERVER.md).

## Evaluation and development

Offline fixtures exercise harness mechanics without claiming model quality.
Real-model suites require explicit provider credentials and environment
identity:

```sh
./bin/gator eval ./internal/eval/testdata/greeting \
  --run-id greeting-script-001 \
  --report /tmp/gator-eval/greeting-script-001.json \
  --require-resolved

OPENAI_API_KEY=... ./bin/gator eval suite ./internal/eval/testdata/core-v1 \
  --live --provider openai --model gpt-5.6 \
  --environment-id 'gator-eval@sha256:IMAGE_DIGEST' \
  --attempts 3 --report-dir /tmp/gator-eval/core-001 --require-resolved
```

Use `make check` before handoff. See [evaluation](docs/EVALUATION.md),
[architecture](docs/ARCHITECTURE.md), and
[release evidence](docs/RELEASE_EVIDENCE.md).
