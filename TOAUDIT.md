# Gator command surface — audit draft

This is an editable inventory of Gator's current command surface. It is based
on the active CLI dispatcher in `cmd/gator/main.go`, the usage printed by
`gator help`, and the active Work TUI dispatcher in
`internal/worktui/commands.go`.

Edit this document to describe the command catalogue you want. After review,
the implementation should follow this document rather than attempting to
preserve every existing command by default.

## Scope and terminology

- **CLI** means a public `gator …` command.
- **TUI** means a slash command or key action inside `gator`.
- **Work** is Gator's primary source-to-validated-deliverable workflow.
- **Code specialist** is the bounded internal implementation delegate used by
  `gator work code` and `gator work run`.
- Commands labelled **machine interface** are intentionally designed for
  programs, editors, or a supervised process rather than the interactive TUI.

## CLI: launch and information

| Command | Current purpose |
| --- | --- |
| `gator` | Start the Work TUI. This is the only TUI entry point. |
| `gator --help`, `gator -h` | Print the CLI reference. |
| `gator --version`, `gator -v` | Print build version, commit, and date. |
| `gator update [--check]`, `gator -u` | Check for, or install, a newer release. Development builds can check but cannot self-replace. |
| `gator doctor [--provider PROVIDER]`, `gator -d` | Inspect local prerequisites, configuration, provider state, sandbox availability, and suggested verification. |

## CLI alias and hierarchy rule

The public command surface has one-letter aliases only for top-level families.
The long family name and its alias are exact equivalents: for example,
`gator agent acp` and `gator -a acp` run the same command. Commands that
would collide at the root live beneath the family that owns their workflow.

| Family | Alias | Nested responsibilities |
| --- | --- | --- |
| `agent` | `-a` | profiles, retained children, external delegation, ACP, RPC, and the app server |
| `browser` | `-b` | controlled browser sessions |
| `config` | `-c` | persistent settings and hook trust |
| `doctor` | `-d` | local diagnostics |
| `extension` | `-e` | extension bundles |
| `job` | `-j` | scheduled Work and inbox entries |
| `lsp` | `-l` | LSP trust |
| `mcp` | `-m` | MCP trust and OAuth |
| `provider` | `-p` | providers, credentials, custom endpoints, and connectors |
| `theme` | `-t` | terminal theme selection |
| `update` | `-u` | release checks and update |
| `work` | `-w` | Work runs, retained output, and evaluation |

`-h` and `-v` remain the conventional help and version flags. When a free-form
Work task begins with a reserved subcommand name such as `inspect`, use
`gator work -- TASK` to make it unambiguously a task.

## CLI: provider and model configuration

| Command | Current purpose |
| --- | --- |
| `gator provider PROVIDER [OPTIONS]` | Start the closest supported provider onboarding route. Explicit targets include `openai`, `anthropic`/`claude`, `gemini`, `codex`, `copilot`, `kimi`, `xai`/`grok`, `openrouter`, and `radius`. |
| `gator provider login PROVIDER [AUTH OPTION]` | Store a Gator-owned provider credential. Authentication options include interactive prompting, an API key, an environment variable, or a bearer token where supported. |
| `gator provider logout PROVIDER` | Remove the stored Gator credential for a provider. It does not unset environment variables or vendor-CLI credentials. |
| `gator provider list` | List configured custom Chat Completions providers. |
| `gator provider add ID --base-url URL --model MODEL [--model MODEL…] [--api-key-env NAME]` | Add a custom Chat Completions endpoint. API-key values remain in the named environment variable, not in Gator config. |
| `gator provider discover ID [--apply]` | Preview `/models` discovery, or replace the saved custom-provider catalogue after `--apply`. |
| `gator provider remove ID --yes` | Remove a custom provider configuration. |
| `gator provider connector ACTION …` | Configure connected sources, their authentication, and their bounded read/write permissions. |

Every `gator provider …` form also accepts `gator -p …` as its short form.

Managed local Ollama models are intentionally configured in the TUI with
`/model` → **Local**. `gator local` is retained only to print that migration
guidance; it does not expose a local-model CLI.

## CLI: configuration and project trust

### Persistent configuration

Every `gator config …` form below also accepts `gator -c …` as its short form.

| Command | Current purpose |
| --- | --- |
| `gator config [show]` | Print the saved configuration. |
| `gator config set default-provider PROVIDER` | Set the default provider. |
| `gator config set default-model MODEL` | Set the default model. |
| `gator config set sandbox MODE` (`strict` or `off`) | Set the default Code-specialist process boundary. |
| `gator config set network MODE` (`deny` or `allow`) | Set the default Code-specialist network boundary. |
| `gator config set snapshot-max-files N` | Set the source-snapshot file-count limit. |
| `gator config set snapshot-max-total-mib N` | Set the source-snapshot total-size limit. |
| `gator config set snapshot-max-file-mib N` | Set the individual snapshot-file-size limit. |
| `gator config set snapshot-exclude PATTERN` | Add a snapshot exclusion pattern. |
| `gator config set desktop-notifications STATE` (`on` or `off`) | Enable or disable desktop notifications. |
| `gator config set job-timezone IANA_TIMEZONE` | Set the default scheduled-job timezone. |
| `gator config set job-missed POLICY` (`skip` or `run_once`) | Set the default missed-job policy. |

### Project-defined capabilities and trust

| Command | Current purpose |
| --- | --- |
| `gator agent list`, `gator -a list` | List project profiles and capability-bounded roles. Must run in a Git checkout. |
| `gator agent child ACTION …` | Inspect retained writer-child and child-batch manifests. |
| `gator config hook ACTION` (`status`, `trust`, or `untrust`) | Inspect or trust the hash of `.gator/hooks.json`. |
| `gator lsp ACTION`, `gator -l ACTION` | Inspect or trust the hash of `.gator/lsp.json`. A status check does not start an LSP server. |
| `gator mcp ACTION`, `gator -m ACTION` | Inspect or trust the hash of `.gator/mcp.json`, including resource-bound OAuth login and logout. |
| `gator extension ACTION …`, `gator -e ACTION …` | Install, enable, disable, remove, and trust extension bundles. |

## CLI: Work, analysis, and implementation

| Command | Current purpose |
| --- | --- |
| `gator work [OPTIONS] TASK`, `gator -w [OPTIONS] TASK` | Run the general Work workflow: capture a read-only source snapshot, use an explicit outcome contract, and retain validated artifacts in private output. Common options set the source folder, artifact contract, attachments, connectors, authority mode, provider/model, and limits. |
| `gator work inspect [OPTIONS] TASK` | Start Work through its inspect-only route. |
| `gator work code [CODE POLICY] TASK` | Run Work while requiring a verified internal Code-specialist patch candidate. |
| `gator work run [CODE POLICY] TASK` | Compatibility name for the same Code-specialist Work route. |
| `gator work resume [CONVERSATION_ID [TASK]]` | Open a retained Work conversation in the TUI, or run a further Work revision when given a task. |

### Work conversation management

| Command | Current purpose |
| --- | --- |
| `gator work list` | List retained Work conversations. |
| `gator work show CONVERSATION_ID` | Emit one conversation record as JSON. |
| `gator work history CONVERSATION_ID` | List retained revisions and the current head. |
| `gator work back CONVERSATION_ID` | Move the conversation head to its parent revision. |
| `gator work forward CONVERSATION_ID [REVISION_ID]` | Move the head to a child revision; requires an ID when more than one child exists. |
| `gator work resume [--refresh-source] [--parent REVISION_ID] CONVERSATION_ID TASK` | Continue a conversation or deliberately branch it from a revision. |
| `gator work tasks RUN_ID [TASK_ID]` | Emit retained specialist-task status, or one task, as JSON. |

### Internal Code policy options

`gator work code` and `gator work run` accept the Code policy arguments below. `gator work`
also exposes these where the requested Work outcome needs an internal Code
specialist.

| Option | Current purpose |
| --- | --- |
| `--code-max-steps N` | Bound internal Code-specialist turns. |
| `--verify 'argv …'` | Require a project verification command; repeatable. |
| `--scope PATH` | Select a project-instruction scope; repeatable. |
| `--profile NAME` | Select a declared, policy-narrowing project profile. |
| `--setup 'argv …'` | Declare explicit worktree setup commands; repeatable. |
| `--allow-command 'argv …'` | Pre-approve one exact child command; repeatable. |
| `--allow-command-prefix 'argv …'` | Pre-approve a literal child-command prefix; repeatable. |
| `--sandbox MODE` (`strict` or `off`) | Set the child process boundary. |
| `--network MODE` (`deny` or `allow`) | Set child process network access. |
| `--code-capability …` | Explicitly grant `lsp`, `mcp`, `extension`, `http`, `browser`, or `terminal`. |
| `--browser-session ID` | Grant one already-controlled local browser session to Code. |
| `--image PATH`, `--attach PATH` | Include supported source-relative prompt attachments. |

## CLI: retained outputs, transcripts, and transfer

| Command | Current purpose |
| --- | --- |
| `gator work review WORK_BUNDLE [--preview] [--json]` | Verify a Work bundle and display its manifest and optional safe previews. |
| `gator work review RUN_RECORD_PATH [--listen 127.0.0.1:PORT] [--open]` | Start a one-use, loopback-only browser review for a retained coding run. |
| `gator work export WORK_BUNDLE [--to ARCHIVE] [--replace]` | Write a verified Work bundle to a deterministic `tar.gz` archive. |
| `gator work export RUN_RECORD_PATH` | Write the patch from a retained coding run to standard output. |
| `gator work apply WORK_BUNDLE --to DIRECTORY [--check] [--replace] [--json]` | Preflight or copy a verified Work bundle into an explicit destination directory. |
| `gator work apply WORK_BUNDLE --to DIRECTORY --code-patch PATH [--check]` | Preflight or apply a verified Code candidate held by a Work bundle. |
| `gator work apply [--check] RUN_RECORD_PATH` | Preflight or apply a retained coding patch to the current clean Git checkout. |
| `gator work transcript RUN_RECORD_PATH > transcript.html` | Export a retained coding session as local HTML. |
| `gator work snapshot list|show ID|gc --yes` | Inspect or explicitly collect immutable source snapshots. |
| `gator work worktree ACTION` (`list`, `prune`, or `remove`) | List, prune, or explicitly remove retained Gator worktrees. |

## CLI: connected services, browser sessions, and scheduled Work

### Connectors

| Command | Current purpose |
| --- | --- |
| `gator provider connector list` | List configured connectors and their auth status. |
| `gator provider connector add ID --kind KIND [OPTIONS]` | Add a `json`, `webhook`, `slack`, `google`, `atlassian`, `notion`, or remote `mcp` connector. |
| `gator provider connector status|login|logout|test|remove ID [OPTIONS]` | Inspect, authenticate, test, or remove a connector. |
| `gator provider connector permission ID OPERATION ACCESS POLICY` | Set one connector operation to `read` or `write` with `allow`, `ask`, `deny`, or `draft`. |

### Controlled browser sessions

| Command | Current purpose |
| --- | --- |
| `gator browser install`, `gator -b install` | Install Gator's pinned Playwright/Chromium runtime. |
| `gator browser status` | Show browser runtime and retained session status. |
| `gator browser start [--headed] [--visual-capture]` | Start a Gator-managed browser session. |
| `gator browser attach --cdp URL [--visual-capture]` | Attach to an explicitly supplied local CDP endpoint. |
| `gator browser tabs SESSION_ID` | List candidate tabs. |
| `gator browser select SESSION_ID TAB_ID [TAB_ID…]` | Select the tabs Gator may use. |
| `gator browser origins SESSION_ID ACTION [URL]` | List, add, or remove permitted origins. |
| `gator browser visual SESSION_ID STATE` (`on` or `off`) | Enable or disable visual capture. |
| `gator browser allow-upload SESSION_ID ABSOLUTE_PATH` | Allow one explicit upload path. |
| `gator browser artifacts SESSION_ID` | List retained browser artifacts. |
| `gator browser export SESSION_ID ARTIFACT_ID --out ABSOLUTE_PATH` | Export a retained browser artifact. |
| `gator browser stop SESSION_ID` | Stop a browser session and revoke its authority. |

### Scheduled Work and inbox

| Command | Current purpose |
| --- | --- |
| `gator job add NAME --schedule CRON [OPTIONS] -- TASK`, `gator -j add …` | Create a scheduled Work job. |
| `gator job list` | List scheduled jobs. |
| `gator job ACTION ID` (`show`, `edit`, `enable`, `disable`, `run`, `history`, or `remove`) | Inspect or manage an existing job. |
| `gator job supervisor [--notify=BOOL]` (`true` or `false`) | Run the foreground job supervisor. |
| `gator job ACTION` (`status` or `stop`) | Inspect or stop a running supervisor. |
| `gator job inbox [--unread]` | List scheduled-work inbox entries. |
| `gator job inbox read ENTRY_ID` | Mark one inbox entry as read and show it. |

## CLI: external-agent delegation

Delegation gives an installed external harness an isolated worktree. Gator
continues to own the worktree and verification boundary; the delegated tool
owns its own credentials, tool grants, sandbox, approvals, and session state.

| Command | Current purpose |
| --- | --- |
| `gator agent delegate RUNTIME ACTION [OPTIONS]` | Run an installed vendor or external agent in an isolated worktree. The supported runtimes are Codex, Copilot, Claude, Kimi, OpenCode, and an explicit external command. |

## CLI: evaluation and machine interfaces

### Evaluation

| Command | Current purpose |
| --- | --- |
| `gator work eval DIR [OPTIONS]` | Run one bounded evaluation fixture. It supports offline scripts or an explicitly selected live model and writes a JSON report. |
| `gator work eval suite DIR --attempts N --environment-id OCI_DIGEST --live [OPTIONS]` | Run a real-model evaluation suite against an immutable OCI environment. |
| `gator work eval validate|run|show|compare|judge-rubric|calibrate-rubric|export-langsmith …` | Validate, execute, inspect, compare, judge, calibrate, or export Work experiments. |

### Machine interfaces

| Command | Current purpose |
| --- | --- |
| `gator agent rpc` | Run Gator's local JSONL RPC protocol over standard input/output. |
| `gator agent acp [--verify 'argv …']` | Run a local Agent Client Protocol v1 server over standard input/output for editors. |
| `gator agent serve token ABSOLUTE_PATH` | Create a private bearer-token file for the app server. |
| `gator agent serve --token-file ABSOLUTE_PATH [--listen 127.0.0.1:PORT]` | Run the authenticated loopback HTTP/SSE app server in the foreground. |
| `gator agent serve ACTION --token-file ABSOLUTE_PATH` (`start`, `status`, or `stop`) | Supervise the loopback app-server service. |

The dispatcher also has `gator work-rpc`, plus `--mode rpc` and `--mode acp`.
These are compatibility/internal entry points rather than documented product
commands and should not be expanded without an explicit product decision.

## Active Work-TUI slash commands

The following commands are dispatched by the active Work TUI. They are not
the same thing as historical TUI documentation or retired Code UI commands.

### Session setup and task shaping

| TUI command | Current purpose |
| --- | --- |
| `/help` or `/?` | Show interactive help. |
| `/new` | Start a clean Work conversation. |
| `/model` | Open cloud and local model management. The Cloud section provides provider configuration, sign-in, and stored-credential removal. |
| `/mode MODE` (`auto`, `inspect`, `draft`, or `act`) | Set the Work authority mode. |
| `/effort LEVEL` (`low`, `standard`, or `high`) | Set manager and Code-specialist turn budgets. |
| `/attach PATH` | Attach one source-relative file to the next prompt. |
| `/detach TARGET` (a path or `all`) | Remove pending attachment(s). |
| `/source [PATH]` | Open the source menu to see the current workspace, enter a replacement workspace path, or select a refresh; with `PATH`, select that read-only source workspace directly. |
| `/source-refresh` | Capture changed workspace files in a new immutable snapshot for the next turn. |
| `/artifact ACTION [PATH]` (`list`, `add`, or `remove`) | List or manage expected deliverable paths. |
| `/connector …` | Select a configured connector for this session, or manage connectors using `list`, `add`, `remove`, `clear`, `setup`, `login`, `logout`, `status`, `test`, `permission`, or `delete`. |
| `/web-origin ACTION [URL]` (`list`, `add`, or `remove`) | Manage bounded HTTPS origins for web research. |

### Status, output, and retained Work history

| TUI command | Current purpose |
| --- | --- |
| `/status` | Inspect the active Work orchestration state. |
| `/statusline` | Choose, order, or hide composer-footer items. |
| `/permissions` | Inspect the active Work authority: source snapshot, mode, connectors, and web origins. |
| `/doctor`, `/agents`, `/settings` | Inspect local prerequisites, project roles, or current settings. |
| `/theme THEME` (`gator`, `contrast`, or `mono`) | Persist a terminal theme. |
| `/history` | Show revisions in the current conversation. |
| `/revision-back` | Move to the parent revision. |
| `/revision-forward [REVISION_ID]` | Move to a child revision. |
| `/review` | Preview the latest verified deliverables in this conversation. |
| `/save [--replace] [DIRECTORY]` | Preflight then save verified deliverables after confirmation. |
| `/apply [CANDIDATE] [DIRECTORY]` | Preflight then apply a verified Code candidate after confirmation. |
| `/copy` | Open a chooser for an individual Gator response or the latest verified-deliverables summary, then copy the selected item. |
| `/queue`, `/dequeue`, `/clear-queue` | Inspect or manage queued prompts. |
| `exit` and `/quit` | Exit the TUI. |

The Work TUI exposes no Code-specialist commands. Gator invokes and manages
that internal specialist as part of a Work run.

### TUI commands available while Work is running

| TUI command | Current purpose |
| --- | --- |
| `/steer TEXT` | Send steering to the current operation. |
| `/tasks` | List active specialists and their status. |
| `/cancel-task TASK_ID` | Cancel one active specialist. |
| `/approve`, `/deny` | Respond to the currently displayed exact approval request. |
| `/cancel` | Cancel the active Work run while retaining its outcome. |
| `exit` or `/quit` | Cancel the active run and then exit. |

### Work-TUI keyboard navigation

| Key | Current purpose |
| --- | --- |
| `Ctrl+P` | Open the searchable command palette. |
| `Ctrl+X` | Open retained conversations. |
| `Ctrl+B` | Open the scheduled-work inbox. |
| `Ctrl+J` | Open scheduled jobs. |

## `/model` catalogue controls

`/model` is the active model-management entry point. Its UI is separate from
the Work slash-command dispatcher and has its own controls.

| Area | Controls |
| --- | --- |
| Both Cloud and Local | `↑`/`↓` choose; `Enter` use an installed/configured model; `Tab` switch sections; `r` refresh; `F1` help; `Esc` return. |
| Cloud | `c` configure a provider; `l` sign in; `n` add a custom provider; `g` discover custom-provider models; `d` remove a stored credential; `x` remove a custom provider; `e` rename. |
| Local | `p` review and download a curated Ollama model; `x` review and remove it; `e` rename its display label; `s` start Ollama as a Gator child; `i` show installation help. |

The Local catalogue currently shows General Work models before Coding models.
Every managed-local catalogue model remains text-only in Gator even when an
upstream package separately advertises visual capability.

## Explicitly retired or non-current entries

These names exist only to explain migration or preserve implementation
compatibility. They should not be treated as supported product workflows.

| Entry | Current behavior |
| --- | --- |
| `gator tui`, `gator help`, `gator version`, `gator connect`, `gator login`, and `gator logout` | Removed from the public CLI. Use `gator`, `--help`, `--version`, and the `gator provider …` family instead. |
| Former root commands such as `gator acp`, `gator connector`, `gator code`, `gator eval`, `gator inspect`, `gator serve`, and `gator worktree` | Return migration guidance to their canonical nested family. |
| `gator code --tui` | Removed as a duplicate TUI entry point; start `gator` instead. |
| `gator local` | Returns directions to `/model` → Local. |
| `gator fork` | Returns directions to branch a Work conversation with `gator work resume --parent …`. |
| `gator clone` | Returns directions to continue or branch a retained Work conversation. |
| Historical `/manage`, `/recent`, `/tree`, `/fork`, `/clone`, `/opencode`, `/version`, and `/update` TUI references | Not dispatched by the active Work TUI. Older parity material describes a retired interface and is not a current command contract. |
| `gator work-rpc`, `gator --mode rpc`, `gator --mode acp` | Undocumented compatibility/internal protocol entry points. |
