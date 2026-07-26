# Gator

Gator is a Neovim plugin for launching local coding-agent CLIs with editor context, then retaining a local run graph for focus, handoff, review, and parallel worktrees.

It does not replace Codex, Pi, Claude, ACP agents, or their harnesses. Providers retain authentication, model choice, tools, permissions, compaction, and sessions. Gator records only runs it starts.

## The normal flow

1. Select code, or place the cursor in the relevant buffer.
2. Run `:Gator` and enter a short objective.
3. Pick a ready provider once. Gator remembers it for that Git project.
4. Gator opens a structured chat when that provider exposes one; otherwise it opens the provider's native terminal in Neovim.
5. Use `:GatorRuns` to focus, stop, resume, hand off, or start a parallel writer.

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
nvim --cmd 'set rtp+=/absolute/path/to/gator' \
  --cmd 'lua require("gator").setup()' .
```

Then run `:GatorHealth` from the target Git project, followed by `:Gator`.

## Commands

| Command | Use |
| --- | --- |
| `:Gator [provider]` | Capture the visual range/current buffer and launch. |
| `:GatorRuns` | Open the local run graph. |
| `:GatorHandoff <run-id> [provider]` | Review a bundle and create a new target-provider session. |
| `:GatorStopSession [run-id]` | Stop a Gator-managed local process. |
| `:GatorHealth` | Check provider readiness and compatibility. |

There is no legacy dashboard, import, file, or ID workflow. A simple optional mapping is:

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
  },
  context = {
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
  },
  providers = {
    pi = { user_confirmed = true }, -- only after Pi is configured locally
  },
  ui = {
    icons = "ascii", -- "unicode", "nerd_font", "ascii", or "none"
    loading = {
      enabled = true,
      spinner = "whirly.hanoi", -- `:lua =require("gator.ui.loading").presets()`
      interval_ms = 0, -- 0 keeps the upstream cadence; otherwise >= 16 ms
    },
  },
})
```

Gator vendors 169 selectable loading animations from [Rattles](https://github.com/vyfor/rattles) and [Whirly](https://github.com/janlelis/whirly); see [loading dialogs](docs/LOADING.md) and [third-party notices](THIRD_PARTY_NOTICES.md). `ui.motion.enabled = false` or `ui.motion.reduced = true` leaves the dialog visible but static.

Provider selection precedence is explicit command/API provider, project-local remembered provider, global `launch.default_provider`, then the picker. An unavailable configured provider opens the picker; Gator does not silently substitute another agent.

`auto` uses a Gator chat only for documented structured transports: Pi RPC, Codex App Server, and supported ACP/managed providers. Claude and unsupported/terminal-only providers retain their native Neovim terminal. Forcing `chat` on an unsupported provider fails explicitly.

## Handoffs and parallel runs

Handoff creates a new provider session. It never claims to migrate an opaque provider-native session.

- `full` transfers objective, selected context, current source diff, bounded changed-text-file snapshots, editable review bundle, and a Gator-owned chat transcript when available.
- `compact` transfers a bounded bundle without the full transcript.
- `summary-first` requires explicit opt-in and is unavailable for terminal-originated runs because Gator does not scrape terminal output.

Every handoff is reviewed before launch. Included text-file snapshots are applied in the isolated target workspace and retained under `.gator/handoffs/<bundle-id>/files/`; binary, oversized, and omitted files are shown explicitly in the review. This is a portable Gator artifact, not a claim of provider-native session migration. Terminal runs are labelled `transcript unavailable`. Usage is `reported` only when a provider emits exact counts; otherwise it is `unknown` with a separate local context estimate. A configured token budget can warn or stop only runs with reported usage.

The first writer uses the current checkout. A further active writer gets an isolated Git worktree. Chat, review, and terminal panes are ephemeral; terminal panes close when their provider exits. Gator does not run a daemon, scheduler, process scanner, or automatic workflow queue.

## Provider boundaries

Gator does not read, store, or verify provider credentials. `user_confirmed` means the user asserted local readiness where a CLI has no non-interactive auth-status contract; it is not credential verification.

See [provider support](docs/PROVIDERS.md) for the transport matrix and version constraints.

## Development

```sh
make test
make check
```

`make check` runs Lua tests, indexer tests, format checks, and lint.
