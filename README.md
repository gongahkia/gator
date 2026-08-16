# Gator

Gator is a native, terminal-first coding agent for developers who want an
inspectable route from a task to a tested patch.

It runs an agent inside an isolated Git worktree, streams the exact actions it
takes, and leaves the developer with a diff and verification evidence to
review. Feature work is a primary workflow: Gator is intended to explore an
unfamiliar repository, implement a bounded multi-file change, add or update
tests, and propose the resulting patch.

## Status

The project is being rebuilt from a previous agent meta-harness. The runnable
milestone includes a Gator-owned native loop, direct cloud-provider adapters,
worktree-local tools, command policy, durable local run storage, and a
full-screen terminal application. Real-model usability evidence, context
compaction, and broader replay coverage are still in progress.

## Development

```sh
make check
make build
./bin/gator
./bin/gator help
./bin/gator doctor
OPENAI_API_KEY=... ./bin/gator run --provider openai --verify 'go test ./...' \
  'Add a focused feature with tests'

ANTHROPIC_API_KEY=... ./bin/gator run --provider anthropic \
  --model claude-sonnet-4-6 --verify 'go test ./...' \
  'Add a focused feature with tests'

GEMINI_API_KEY=... ./bin/gator run --provider gemini \
  --model gemini-3.5-flash --verify 'go test ./...' \
  'Add a focused feature with tests'

# Subscription providers keep Gator's tool loop and use Gator's own OAuth
# credential file. The client ID must be registered for Gator's loopback URL;
# Gator does not impersonate Codex, Claude Code, or Pi.
GATOR_CODEX_OAUTH_CLIENT_ID=... ./bin/gator login codex
GATOR_CLAUDE_OAUTH_CLIENT_ID=... ./bin/gator login claude
./bin/gator run --provider codex --verify 'go test ./...' \
  'Add a focused feature with tests'

# Open a project-scoped retained-thread picker, continue the latest thread,
# or target an ID directly. The task is optional: without it, Gator opens the
# resumed conversation in the TUI.
./bin/gator resume
./bin/gator resume --last
./bin/gator resume thread-identifier
./bin/gator resume --all
OPENAI_API_KEY=... ./bin/gator resume --last \
  'Address the failing verification and finish the patch'

# The original raw run-record form remains supported.
OPENAI_API_KEY=... ./bin/gator resume /path/to/run-record \
  'Address the failing verification and finish the patch'

# Save a portable patch or apply it explicitly to a clean compatible checkout.
./bin/gator export /path/to/run-record > gator-review.patch
./bin/gator apply --check /path/to/run-record
./bin/gator apply /path/to/run-record
```

`gator` opens a conversation-first terminal application when run from a Git
checkout and a real terminal. Type a task and press `Enter` to send it; the
same prompt accepts follow-up instructions after the run completes. `Ctrl+R`
also sends in every input mode. The live
conversation includes model text and tool activity, while `PgUp` and `PgDn`
browse earlier entries. Use `/provider`, `/model`, `/login`, and `/verify` to edit the
run configuration; their fields show keyboard-selectable dropdowns where
available. During a run, `Enter` sends a steering instruction
that the agent consumes at its next model or tool boundary; it does not cancel
the run. `Tab` queues the current prompt for the next turn, and a second `Tab`
queues an already-completed slash command. Gator runs queued work in FIFO order
only after the active run succeeds. `/queue`, `/dequeue`, and `/clear-queue`
inspect or manage the bounded, 16-item in-memory queue. Failed or cancelled
runs leave queued instructions paused for the developer to inspect; the queue
is cleared when the TUI exits and is never written to a retained session.
During a run, `Ctrl+C` requests cancellation while retaining the isolated
worktree. The running view shows an event-backed activity phase, current turn,
provider mode, verifier state, and a preview of the next queued work; it does
not estimate token use or fake percentage progress. After a run, the latest
result summarizes verifier status and the visible diff's file/addition/deletion
counts. Failure states include the reason plus the relevant next action.
Use `/review` for the final report and diff preview, then
press `e` for patch handoff commands, `Esc` to return to the conversation, or
`n` for a new thread.

`/vim` toggles Vim-style message editing. Vim Normal mode supports `i`/`a`
to edit, `o` to add a line, `h`/`j`/`k`/`l` to move, `0`/`$` to reach a line
edge, and `x` to delete. `Enter` sends from Normal mode. In Insert mode,
`Enter` adds a line and `Esc` returns to Normal mode. In Normal mode, `:w`
also sends the current message; `:wq` sends it and exits only after the active
and queued work completes successfully.

Before sending, Gator validates the task and local prerequisites: a verifier
list in Execute mode, valid provider settings, required API-key environment
variables, and provider-specific model/base-URL
requirements. Press `F1` for the conversation, running, and review shortcut
reference. `Ctrl+O` opens recent retained threads for the current repository;
press `a` in that picker to switch between the current repository and all
locally retained repositories. The picker shows a summary rather than private
run-record paths and validates the selected worktree when continuation begins.
`/tree` opens an in-terminal navigator for the active retained thread. It
shows every saved turn from root to head with its mode, provider/model, status,
timestamp, prompt summary, and the selected turn's final response. `y` opens
the same navigator from review. Gator retains a linear parent-linked lineage;
unlike Pi's branchable session tree, it does not create, switch, or delete
branches from this view.

Type `/` (or `?` in an empty message) to filter and select local conversation commands;
type `@` in a task to select a repository file or directory from matching path
suggestions. In the command menu, `Tab` completes the selected command and
`Enter` executes it; path suggestions accept either key to insert the path.
Conversation
commands are `/plan`, `/execute`, `/new`, `/status`, `/model`, `/verify`,
`/permissions`, `/worktree`, `/review`, `/threads`, `/recent`, `/tree`, `/clear`,
`/queue`, `/dequeue`, `/clear-queue`, `/help`, and `/quit`. `/plan` gives every
provider an enforced read-only tool surface and does not require a verifier;
switch the same retained thread to `/execute` when you are ready to make edits.
`Ctrl+Space` (reported as `Ctrl+@`
by many terminals) reopens an active `@` path menu. Use `@path/to/file` or `@"path with spaces"`
in a task to mark repository files or directories that the agent should inspect
first. Gator validates every reference against the repository boundary before
starting. Ordinary references remain paths only. Before supported attachments
are sent, Gator shows the exact files, sizes, and selected provider and requires
an explicit confirmation.

Use `@mock.png`, `@screenshot.jpg`, or `@design.webp` to attach images, and
`@report.pdf` to attach a PDF, to a direct-provider task. Gator also accepts
`@` references to UTF-8 text/data documents (`.txt`, Markdown, CSV, JSON,
YAML, TOML, XML, HTML, logs, and common config files) plus `.docx`, `.odt`,
and `.xlsx`. Office files are extracted locally into plain text; PDFs retain
their original bytes so OpenAI, Anthropic, and Gemini can use their documented
document inputs. Text/data attachments work with every direct API adapter;
PDFs require the `openai`, `codex`, `anthropic`, `claude`, or `gemini` provider because generic
Chat Completions endpoints do not share a stable document-input protocol.

Gator permits at most four attachments per task, each up to 4 MiB, with an
8 MiB combined image/document budget. Attachments must be regular files inside
the repository and are read through a descriptor-rooted workspace boundary.
Raw attachment bytes are never written to the private `0600` continuation
session; Gator retains only a name, media type, size, and SHA-256 manifest, so
you must explicitly re-add `@` files in a later turn. The selected provider
receives confirmed bytes under its own data-handling and retention policy.
The byte budget limits local input size but does not cap a PDF's provider-side
page or token cost. Attachment text is framed as untrusted data, but no LLM
prompt can make a model fully immune to prompt injection.
Unsupported binary `@` paths remain ordinary references that the agent can
inspect if its tools can read them. In the running view, file reads, commands,
tool results, and patch line changes are visible as they occur. Press `t` from
review to browse the current run transcript; it shows model-emitted text and
tool activity, not hidden chain-of-thought.

The review screen deliberately does not modify the active checkout. To hand off
reviewed work, `gator export RUN_RECORD_PATH` emits a binary-safe patch to
standard output. `gator apply --check RUN_RECORD_PATH` verifies that the current
checkout is clean and compatible; omitting `--check` applies the patch. Gator
does not stage, commit, or push the result. The existing `run` and `resume`
commands remain available for scripts and CI-like usage.

## Providers

Set `--provider`, or set `GATOR_PROVIDER` before starting the TUI. `GATOR_MODEL`
and `GATOR_BASE_URL` supply the corresponding defaults. `--base-url` overrides
an endpoint for one scripted run.

| Provider | Authentication | Protocol |
| --- | --- | --- |
| `openai` | `OPENAI_API_KEY` | OpenAI Responses API |
| `codex` | Gator-owned ChatGPT/Codex OAuth credential | ChatGPT Codex Responses endpoint |
| `anthropic` | `ANTHROPIC_API_KEY` | Anthropic Messages API |
| `claude` | Gator-owned Claude Pro/Max OAuth credential | Anthropic Messages endpoint |
| `gemini` | `GEMINI_API_KEY` | Gemini GenerateContent API |
| `azure-openai` | `AZURE_OPENAI_API_KEY` plus `--base-url` and deployment model | Azure OpenAI-compatible Chat Completions |
| `mistral`, `xai`, `groq`, `openrouter`, `together`, `fireworks`, `deepseek` | Provider-specific API key | OpenAI-compatible Chat Completions |
| `openai-compatible` | `GATOR_COMPATIBLE_API_KEY` plus `--base-url` | Any compatible Chat Completions endpoint |

`gator login PROVIDER` stores an API-key credential in
`$XDG_STATE_HOME/gator/auth.json` (or `~/.local/state/gator/auth.json`) with
`0600` permissions. Explicit `--api-key` wins over the stored credential,
which wins over the provider environment variable. `/login codex` and
`/login claude` in the TUI display the browser URL and wait for its loopback
callback; `Ctrl+C` cancels the pending login. Those subscription logins require
`GATOR_CODEX_OAUTH_CLIENT_ID` or `GATOR_CLAUDE_OAUTH_CLIENT_ID`, respectively,
and their registered redirect URL (override the documented loopback defaults
with `GATOR_CODEX_OAUTH_REDIRECT_URL` or `GATOR_CLAUDE_OAUTH_REDIRECT_URL`).

The predefined compatible providers use these key variables respectively:
`MISTRAL_API_KEY`, `XAI_API_KEY`, `GROQ_API_KEY`, `OPENROUTER_API_KEY`,
`TOGETHER_API_KEY`, `FIREWORKS_API_KEY`, and `DEEPSEEK_API_KEY`. Set `--model`
for compatible providers without a Gator default. `gator doctor --provider NAME`
reports the selected provider's prerequisite without printing a secret.

The direct adapters preserve Gator's strict tool surface: repository reads and
writes stay inside the isolated worktree and commands are limited to the
explicit `--verify` argv entries. They replay normalized history locally; the
Gemini adapter also retains the provider content needed for thought-signature
tool-call replay in the private session file.

### Provider ownership

Gator never starts Codex, Claude Code, GitHub Copilot, or Cursor Agent. Its
`codex` and `claude` adapters make direct model requests with credentials that
Gator created and stores itself; they do not reuse a vendor CLI session or read
another application's OAuth files, tokens, or API keys. Gator refreshes a
near-expiry credential before starting a run. `copilot` and `cursor` remain
unavailable rather than falling back to their vendor CLIs: Pi has a direct
Copilot protocol, but it needs an application-controlled GitHub OAuth client
and per-model protocol routing; Cursor is not a Pi built-in provider and no
public direct inference contract was verified.

## Design principles

- Gator-owned agent loop, rather than a wrapper around another coding agent;
- isolated worktree by default; direct edits require an explicit later mode;
- a complete event trace, patch, and verifier result for every completed run;
- deterministic tests and replayable model transcripts around provider
  behavior;
- no claim of model or benchmark competitiveness without published evidence.

## Local run data

Runs leave code changes in a sibling `*-gator-runs/` worktree, never in the
active checkout. The printed run-record path defaults to
`$XDG_STATE_HOME/gator/` (or `~/.local/state/gator/`) and contains a
metadata-only event journal, final result, and a private `0600` session file
for `gator resume`. `gator resume --last` selects the newest retained thread
for the current repository; `--all` expands selection to the local state root.
The event log intentionally omits prompts, source text,
tool arguments, tool output, and raw attachment bytes; the worktree is the
reviewable source of truth. The TUI also keeps one private `0600` unfinished draft per repository
and lists resumable conversation threads from the same state root. Threads
retain one worktree across turns and record whether the last turn was Plan or
Execute. Each retained turn points to its immutable parent record, which lets
`/tree` reconstruct the local root-to-head lineage without replaying the agent.
Drafts contain only the current message, verifier text, provider, and
model, and are removed after Gator finishes a new thread. The active-turn queue
is intentionally absent from drafts and sessions, so a restart never performs
queued work automatically. Set `GATOR_STATE_DIR`
to use another local state root.

See [the architecture](docs/ARCHITECTURE.md) for the intended runtime and
acceptance criteria.
