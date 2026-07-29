# Gator

Gator is a Neovim plugin for launching local coding-agent CLIs with editor context, then retaining a local run graph for focus, handoff, review, and parallel worktrees.

It does not replace Codex, Pi, ACP agents, or their harnesses. Providers retain authentication, model choice, tools, permissions, compaction, and sessions. Gator records only runs it starts.

## The normal flow

1. Select code, or place the cursor in the relevant buffer.
2. Run `:Gator` and enter a short objective.
3. Pick a ready provider once. Gator remembers it for that Git project.
4. Gator opens a structured chat when that provider exposes one; otherwise it opens the provider's native terminal and a Gator control companion in Neovim.
5. Use `:GatorRuns` to focus, resume/fork, hand off, review, attach new context, or start a ready runbook step.

The launched prompt receives the objective, exact selected/current source, and current Git diff. Gator writes local run metadata and bundles under `.gator/`; it adds that directory to Git's local `info/exclude`, never to tracked `.gitignore`.

## Install

Requirements: Neovim 0.11+, Git, and any provider CLIs you intend to use.

```lua
{
  "gongahkia/gator",
  config = function()
    require("gator").setup()
  end,
}
```

For a temporary development checkout:

```sh
make temp
```

`make temp` is a local development launcher: it enables the explicit readiness confirmations for every supported non-Codex provider for that Neovim session only. A provider still appears only when its executable and required capability probe pass; Codex still requires provider-native login.

Then open a source file and run `:GatorHealth`, followed by `:Gator codex`. `require("gator").setup()` registers every Gator command; no extra `:runtime plugin/gator.lua` step is needed.

## First run

1. In the target Git checkout, open the relevant source file or visually select code.
2. Run `:GatorHealth`. Its first section lists agents ready now; warnings for other providers are collapsed because they are informational unless you plan to use them. Use `:GatorHealth!` for individual provider diagnostics.
3. Run `:Gator codex`, enter a concise objective, and wait for the chat pane. A startup error closes the spinner and marks the run failed instead of leaving it pending.
4. In the chat, `i` sends a follow-up prompt once the agent is ready, `c` cancels the current turn, and `q` detaches without stopping the provider. Markdown is display-formatted; the local transcript retains the provider text for handoff.
5. In Visual mode, press `<leader>gA` to choose a ready chat, ask one question, and send the selected lines and question as one turn. It will not steer a response already in progress; `:'<,'>GatorAsk` remains available without the mapping.
6. Run `:GatorRuns` to find detached runs. Press `<CR>` to focus one or run `:GatorStopSession [run-id]` to terminate Gator's local process.

## Commands

| Command | Use |
| --- | --- |
| `:Gator [provider]` | Capture the visual range/current buffer and launch. |
| `:GatorRuns` | Open the local run graph. |
| `:GatorEvents <run-id>` | Inspect Gator's append-only metadata journal for a run. |
| `:GatorHandoff <run-id> [provider]` | Review a bundle and create a new target-provider session. |
| `:[range]GatorSend [run-id] [selection\|diagnostic\|hunk\|bundle] [bundle-id]` | Send provenance-labelled editor context to an active structured chat. |
| `:[range]GatorAsk [run-id]` | Ask a ready structured chat one question about the selected lines. |
| `:GatorReview [run-id]` | Inspect a worktree diff and run an explicitly configured review command. |
| `:GatorRunbook` | Select and start one ready manual runbook step. |
| `:GatorStopSession [run-id]` | Stop a Gator-managed local process. |
| `:GatorStorage` | Inspect project-local Gator artifact storage and quota candidates. |
| `:GatorPrune` | Preview then remove age- or quota-selected Gator-owned artifacts and eligible clean worktrees. |
| `:GatorForget <run-id>` | Remove a completed run's artifacts and eligible clean worktree. |
| `:GatorHealth[!]` | Check readiness and compatibility; `!` expands individual provider diagnostics. |

`<leader>gA` is installed in Visual mode only when that mapping is unclaimed. It invokes `<Plug>(gator-ask-selection)`, so a config can remap it or disable only the default:

```lua
require("gator").setup({
  ui = { ask_selection = { keymap = false } },
})
vim.keymap.set("x", "<leader>ga", "<Plug>(gator-ask-selection)", { desc = "Gator ask selected text" })
```

There is no legacy dashboard, import, file, or ID workflow. A simple optional launch mapping is:

```lua
vim.keymap.set("n", "<leader>ag", "<cmd>Gator<CR>", { desc = "Gator launch" })
vim.keymap.set("x", "<leader>ag", ":Gator<CR>", { desc = "Gator launch selection" })
```

## Configuration

```lua
require("gator").setup({
  launch = {
    default_provider = "ask", -- "ask" or a provider id
    transport = "auto", -- "auto", "chat", or "terminal"
    stall_after_ms = 120000, -- warn after 120s without a structured provider event; 0 disables
  },
  permissions = {
    codex = { sandbox = "workspace_write" }, -- or "read_only"; sent to Codex App Server
  },
  context = {
    preflight = {
      confirm = false, -- log metadata by default; true asks before every launch/context send
    },
    handoff = {
      profile = "full", -- "full", "compact", or "summary-first"
      source_summary = false, -- explicit opt-in for summary-first review
      max_files = 24, -- changed text files copied into reviewed handoffs
      max_file_chars = 65536, -- total copied-file character limit; 0 disables copies
    },
  },
  budget = {
    max_tokens = 0, -- 0 is unbounded; enforced only for provider-reported usage
    action = "warn", -- "warn" or "stop"
    max_concurrent_runs = 0, -- 0 is unbounded
  },
  retention = {
    max_age_days = 30, -- any positive integer; 0 retains artifacts forever
    max_bytes = 0, -- 0 disables quota selection; quotas are previewed, never auto-pruned at startup
    cleanup_on_start = true, -- automatically delete expired Gator-owned artifacts at startup
    worktrees = "inactive_clean", -- only clean, unlocked, inactive Gator worktrees
  },
  review = {
    commands = {
      unit = { argv = { "make", "test" } }, -- only configured argv commands can run from review
    },
  },
  acp = {
    commands = {
      -- localagent = { argv = { "local-agent", "--acp" } }, -- explicit opt-in only
    },
  },
  extensions = {
    modules = { "my_gator_extension" }, -- explicitly configured, trusted Lua modules only
  },
  runbooks = {
    max_concurrent = 0, -- default per-runbook active-run cap
    max_tokens = 0, -- default per-runbook reported-token cap
  },
  providers = {
    pi = { user_confirmed = true }, -- only after Pi is configured locally
  },
  ui = {
    icons = "ascii", -- "unicode", "nerd_font", "ascii", or "none"
    resources = {
      enabled = true, -- false hides local resource lines in :GatorRuns
      fields = { "wall_time", "context_bytes", "worktree", "usage" }, -- choose any subset
    },
    loading = {
      enabled = true,
      spinner = "whirly.hanoi", -- `:lua =require("gator.ui.loading").presets()`
      interval_ms = 0, -- 0 keeps the upstream cadence; otherwise >= 16 ms
    },
    chat = {
      layout = "split", -- "split", "float", or "fullscreen"
      height = 18, -- split/float height; + and - resize a focused chat
      width = 0, -- float width; 0 uses 75% of the editor width
    },
    ask_selection = {
      keymap = "<leader>gA", -- false disables only Gator's unclaimed default mapping
    },
    renderers = {
      provider_picker = "native", -- or an extension renderer id
      run_graph = "native",
      context_preflight = "native",
      handoff_review = "native",
      dashboard = "native",
    },
    run_graph = {
      columns = { "id", "provider", "role", "state", "context", "resources", "budget", "trust" },
    },
  },
})
```

Gator vendors 169 selectable loading animations from [Rattles](https://github.com/vyfor/rattles) and [Whirly](https://github.com/janlelis/whirly); see [loading dialogs](docs/LOADING.md) and [third-party notices](THIRD_PARTY_NOTICES.md). `ui.motion.enabled = false` or `ui.motion.reduced = true` leaves the dialog visible but static.

Focused chats use `+`/`-` to resize, `f` to toggle fullscreen, `o` to cycle split, float, and fullscreen layouts, and `r` to open the run list. After 120 seconds without a structured provider event, the chat says it is stalled and leaves `c` cancel, `q` detach, and `r` runs available; it never retries or kills the provider automatically. Terminal companions expose focus, stop, detach, runs, journal, and handoff without reading terminal output. `?` explains the selection (`:GatorAsk`) and run flows (resume, review, handoff). These mappings can be overridden through `ui.keymaps`.

Provider selection precedence is explicit command/API provider, project-local remembered provider, global `launch.default_provider`, then the picker. An unavailable configured provider opens the picker; Gator does not silently substitute another agent.

`auto` uses a Gator chat only for documented structured transports: Pi RPC, Codex App Server, and supported ACP/managed providers. Claude Code and OpenCode are not supported by Gator; use their own CLIs outside Gator. Forcing `chat` on an unsupported provider fails explicitly.

## Extensions and integrations

Gator can load explicitly configured trusted Lua modules for lifecycle hooks, custom terminal or ACP adapters, UI slots, run-graph columns, and context processing. It does not scan directories or sandbox modules: an extension has normal Neovim/Lua access. The interface is intentionally unversioned, so plugin upgrades can require extension changes. See [extensions](docs/EXTENSIONS.md).

## Local journal, context, and retention

For every run Gator appends lifecycle, provider/version, trust, approval, usage, context-metadata, handoff, review, recovery, and exit events to `.gator/events/<run-id>.jsonl`. `:GatorRuns` opens it with `l`; `:GatorEvents <run-id>` opens it directly. The journal records Gator-owned metadata, not raw prompts, source, diffs, terminal output, or provider transcripts. It is append-only while Gator writes it, but remains ordinary user-owned local data and is not tamper-proof.

Launches and `:GatorSend` always record their target, artifacts, byte/token estimate, and redaction-match count before delivery. This is passive by default. Set `context.preflight.confirm = true` to inspect that exact metadata and explicitly send or cancel every launch/context transfer.

`:GatorStorage` inventories only Gator's project-local artifacts. `:GatorPrune` is always a preview with explicit confirmation. `retention.max_bytes` adds oldest-first quota candidates to that preview; it never causes startup deletion. Startup cleanup only removes age-expired Gator-owned artifacts and eligible clean inactive worktrees. `:GatorForget` removes one completed run and its Gator-owned artifacts, including its journal. Gator neither imports nor deletes provider-native history created outside Gator.

`:GatorRuns` shows local resources by default: elapsed wall time, exact bytes/sends Gator delivered as context, Gator worktree count/disk use, and provider-reported usage. These values remain separate: elapsed time is local wall time, context bytes are not provider tokens, worktree size is local filesystem usage, and token/cost values stay `unknown` unless a provider reports them. Set `ui.resources.enabled = false` or choose any subset of `wall_time`, `context_bytes`, `worktree`, and `usage` with `ui.resources.fields`.

## Handoffs and parallel runs

Handoff creates a new provider session. It never claims to migrate an opaque provider-native session.

- `full` transfers objective, selected context, current source diff, bounded changed-text-file snapshots, editable review bundle, and a Gator-owned chat transcript when available.
- `compact` transfers a bounded bundle without the full transcript.
- `summary-first` requires explicit opt-in and is unavailable for terminal-originated runs because Gator does not scrape terminal output.

Every handoff is reviewed before launch. The review displays target transport/workspace, context byte size, included/omitted snapshots, redaction/omission reasons, and target diff. You explicitly choose whether snapshots are applied or retained only; binary, oversized, and omitted files remain visible. This is a portable Gator artifact, not a claim of provider-native session migration. Terminal runs are labelled `transcript unavailable`. Usage is `reported` only when a provider emits exact counts; otherwise it is `unknown` with a separate local context estimate. A configured token budget can warn or stop only runs with reported usage.

After Neovim restarts, formerly active runs become `detached`. Gator resumes only when the same provider confirms the persisted native session identity. A native fork is available only for structured providers with a documented fork contract; cross-provider continuation remains a reviewed portable handoff.

The first writer uses the current checkout. A further active writer gets an isolated Git worktree. Active and detached Gator worktrees are Git-locked and have local lease metadata; terminal states unlock them. Startup cleanup removes only expired Gator-owned artifacts and clean, unlocked, inactive leased worktrees. It never force-removes a worktree or deletes a branch. Chat, review, and terminal panes are ephemeral; terminal panes close when their provider exits. Gator does not run a daemon, scheduler, process scanner, or automatic workflow queue.

## Manual runbooks

Runbooks are an opt-in Lua API for deliberate multi-agent work, not a scheduler:

```lua
require("gator").create_runbook({
  id = "fix-and-review",
  title = "Research, implement, review",
  steps = {
    { id = "research", role = "researcher", provider = "pi", objective = "Find the cause", depends_on = {} },
    { id = "write", role = "writer", provider = "codex", objective = "Implement the fix", depends_on = { "research" } },
    { id = "review", role = "reviewer", provider = "pi", objective = "Review the diff", depends_on = { "write" } },
    { id = "integrate", role = "integrator", provider = "codex", objective = "Converge reviewed work", depends_on = { "write", "review" } },
  },
})
```

The run graph renders dependency state and ready steps; press `n` or run `:GatorRunbook` to select one. Gator never starts the next step itself. `researcher` and `reviewer` add a read-only instruction and use an existing workspace; generic CLIs remain provider-owned processes, so this is not a sandbox. `writer` and `integrator` use isolated worktrees; integrator start always requires confirmation. Dependency packets identify every dependency and include bounded Gator-owned transcript/bundle/diff provenance; they do not claim provider-native session migration. See [runbooks](docs/RUNBOOKS.md).

## Provider boundaries

Gator does not read, store, or verify provider credentials. `user_confirmed` means the user asserted local readiness where a CLI has no non-interactive auth-status contract; it is not credential verification.

Every new run stores its actual launch boundary in `:GatorRuns`:

- Codex chat sends `workspace-write` or `read-only` plus `on-request` approval to Codex App Server. Codex enforces that policy; Gator does not. Network and MCP access remain `unknown`.
- Pi chat records filesystem, network, and MCP access as `unknown`; Pi RPC has no Gator approval bridge.
- ACP chat records those controls as `unknown`; Gator can render an approval only when the provider emits one.
- Native terminal runs are provider-owned. Gator does not infer or claim sandboxing, authentication, read-only access, or network restrictions from terminal output.

An intentionally tracked `.gator/policy.json` may set `write_allowed`. Since Gator locally ignores `.gator/`, track this file deliberately with `git add -f .gator/policy.json`. `false` overrides Codex to `read-only` and blocks a provider Gator cannot constrain.

```json
{ "enabled": true, "rules": { "write_allowed": false } }
```

See [provider support](docs/PROVIDERS.md) for the transport matrix and version constraints.

## Development

```sh
make test
make check
```

`make check` runs Lua tests, indexer tests, format checks, and lint.
