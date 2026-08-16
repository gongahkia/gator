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
milestone includes the native loop, cloud-provider adapters, delegated vendor
CLI support, worktree-local tools, command policy, durable local run storage,
and a full-screen terminal application. Real-model usability evidence, context
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

# Reuses an existing vendor CLI login. This has a separate tool-permission
# boundary, so the script command requires an explicit acknowledgement.
./bin/gator run --provider codex --allow-external-cli --verify 'go test ./...' \
  'Add a focused feature with tests'

# Continue an interrupted run using the printed run-record path.
OPENAI_API_KEY=... ./bin/gator resume /path/to/run-record \
  'Address the failing verification and finish the patch'

# Save a portable patch or apply it explicitly to a clean compatible checkout.
./bin/gator export /path/to/run-record > gator-review.patch
./bin/gator apply --check /path/to/run-record
./bin/gator apply /path/to/run-record
```

`gator` opens the interactive application when run from a Git checkout and a
real terminal. Describe the task, keep or edit the suggested verification
commands, choose a provider and model, and press `Ctrl+R` to start. Provider
and model fields show keyboard-selectable dropdowns; type to filter, use the
arrow keys to choose, then press `Enter` or `Tab`. `Tab` moves between task,
verifier, provider, and model fields. During a run, `Ctrl+C`
requests cancellation while retaining the isolated worktree. The review screen
shows the final report, worktree, run record, and a diff preview; press `c` to
continue a retained thread, `e` for patch handoff commands, `n` for a new task,
or `d` to refresh the diff.

The composer shows local run prerequisites before `Ctrl+R`: a
task and verifier list, valid provider settings, required API-key environment
variables or a vendor CLI on `PATH`, and provider-specific model/base-URL
requirements. Press `F1` for the full composer, running, and review shortcut
reference. `Ctrl+O` opens recent retained threads for the current repository;
the picker shows a summary rather than private run-record paths and validates
the selected worktree when continuation begins.

Type `/` (or `?` in an empty task) to filter and select local composer commands;
type `@` in a task to select a repository file or directory from matching path
suggestions. Both menus accept arrow keys and `Enter` or `Tab`. Composer
commands are `/plan`, `/execute`, `/new`, `/status`, `/model`, `/verify`,
`/permissions`, `/worktree`, `/review`, `/threads`, `/recent`, `/clear`,
`/help`, and `/quit`. `/plan` gives native providers an enforced read-only
tool surface and does not require a verifier; switch the same retained thread
to `/execute` when you are ready to make edits. Delegated CLI providers cannot
enter enforced Plan mode. `Ctrl+Space` (reported as `Ctrl+@`
by many terminals) reopens an active `@` path menu. Use `@path/to/file` or `@"path with spaces"`
in a task to mark repository files or directories that the agent should inspect
first. Gator validates every reference against the repository boundary before
starting. Ordinary references remain paths only. Before supported attachments
are sent, Gator shows the exact files, sizes, and selected provider and requires
an explicit confirmation.

Use `@mock.png`, `@screenshot.jpg`, or `@design.webp` to attach images, and
`@report.pdf` to attach a PDF, to a native-provider task. Gator also accepts
`@` references to UTF-8 text/data documents (`.txt`, Markdown, CSV, JSON,
YAML, TOML, XML, HTML, logs, and common config files) plus `.docx`, `.odt`,
and `.xlsx`. Office files are extracted locally into plain text; PDFs retain
their original bytes so OpenAI, Anthropic, and Gemini can use their documented
document inputs. Text/data attachments work with every native API adapter;
PDFs require the `openai`, `anthropic`, or `gemini` provider because generic
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
prompt can make a model fully immune to prompt injection. A delegated CLI is
rejected for attachments because Gator cannot verify its file-input contract.
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
| `anthropic` | `ANTHROPIC_API_KEY` | Anthropic Messages API |
| `gemini` | `GEMINI_API_KEY` | Gemini GenerateContent API |
| `azure-openai` | `AZURE_OPENAI_API_KEY` plus `--base-url` and deployment model | Azure OpenAI-compatible Chat Completions |
| `mistral`, `xai`, `groq`, `openrouter`, `together`, `fireworks`, `deepseek` | Provider-specific API key | OpenAI-compatible Chat Completions |
| `openai-compatible` | `GATOR_COMPATIBLE_API_KEY` plus `--base-url` | Any compatible Chat Completions endpoint |

The predefined compatible providers use these key variables respectively:
`MISTRAL_API_KEY`, `XAI_API_KEY`, `GROQ_API_KEY`, `OPENROUTER_API_KEY`,
`TOGETHER_API_KEY`, `FIREWORKS_API_KEY`, and `DEEPSEEK_API_KEY`. Set `--model`
for compatible providers without a Gator default. `gator doctor --provider NAME`
reports the selected provider's prerequisite without printing a secret.

The native adapters preserve Gator's strict tool surface: repository reads and
writes stay inside the isolated worktree and commands are limited to the
explicit `--verify` argv entries. They replay normalized history locally; the
Gemini adapter also retains the provider content needed for thought-signature
tool-call replay in the private session file.

### Existing CLI subscriptions

`codex`, `claude`, `copilot`, and `cursor` use the installed Codex CLI, Claude
Code, GitHub Copilot CLI, or Cursor Agent CLI. Sign in with the vendor's own
supported command first, then choose the matching Gator provider. Gator neither
reads nor copies those tools' OAuth files, tokens, or API keys.

These providers run their own agent/tool loop inside Gator's isolated worktree.
That is intentionally a separate permission boundary from Gator's native tool
allowlist: Codex is started with its workspace-write sandbox; Claude Code uses
its CLI auto permission mode; Copilot and Cursor apply their own CLI policies.
Gator therefore requires `--allow-external-cli` in scripted mode and always
runs the required verifier argv entries itself after the delegated CLI exits.
Review the retained worktree before applying any patch. A resumed delegated run
starts a fresh vendor CLI task in the same retained worktree; it does not import
or emulate a vendor conversation token.

## Design principles

- native agent loop, rather than a wrapper around another coding agent;
- isolated worktree by default; direct edits require an explicit later mode;
- a complete event trace, patch, and verifier result for every completed run;
- deterministic tests and replayable model transcripts around all harness
  behavior;
- no claim of model or benchmark competitiveness without published evidence.

## Local run data

Runs leave code changes in a sibling `*-gator-runs/` worktree, never in the
active checkout. The printed run-record path defaults to
`$XDG_STATE_HOME/gator/` (or `~/.local/state/gator/`) and contains a
metadata-only event journal, final result, and a private `0600` session file
for `gator resume`. The event log intentionally omits prompts, source text,
tool arguments, tool output, and raw attachment bytes; the worktree is the
reviewable source of truth. The TUI also keeps one private `0600` unfinished draft per repository
and lists resumable conversation threads from the same state root. Threads
retain one worktree across turns and record whether the last turn was Plan or
Execute. Drafts contain only the composer task, verifier text, provider, and
model, and are removed after Gator finishes a new thread. Set `GATOR_STATE_DIR`
to use another local state root.

See [the architecture](docs/ARCHITECTURE.md) for the intended runtime and
acceptance criteria.
