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
vim.keymap.set({ "n", "v" }, "<leader>ag", "<cmd>Gator<CR>", { desc = "Gator launch" })
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
    },
  },
  providers = {
    pi = { user_confirmed = true }, -- only after Pi is configured locally
  },
  ui = { icons = "ascii" }, -- "unicode", "nerd_font", "ascii", or "none"
})
```

Provider selection precedence is explicit command/API provider, project-local remembered provider, global `launch.default_provider`, then the picker. An unavailable configured provider opens the picker; Gator does not silently substitute another agent.

`auto` uses a Gator chat only for documented structured transports: Pi RPC, Codex App Server, and supported ACP/managed providers. Claude and unsupported/terminal-only providers retain their native Neovim terminal. Forcing `chat` on an unsupported provider fails explicitly.

## Handoffs and parallel runs

Handoff creates a new provider session. It never claims to migrate an opaque provider-native session.

- `full` transfers objective, selected context, diff, editable review bundle, and a Gator-owned chat transcript when available.
- `compact` transfers a bounded bundle without the full transcript.
- `summary-first` requires explicit opt-in and is unavailable for terminal-originated runs because Gator does not scrape terminal output.

Every handoff is reviewed before launch. Terminal runs are labelled `transcript unavailable`. Usage is labelled `reported` only when a provider emits it, `estimated` for Gator's local context estimate, or `unknown`.

The first writer uses the current checkout. A further active writer gets an isolated Git worktree. Gator does not run a daemon, scheduler, process scanner, or automatic workflow queue.

## Provider boundaries

Gator does not read, store, or verify provider credentials. `user_confirmed` means the user asserted local readiness where a CLI has no non-interactive auth-status contract; it is not credential verification.

See [provider support](docs/PROVIDERS.md) for the transport matrix and version constraints.

## Development

```sh
make test
make check
```

`make check` runs Lua tests, indexer tests, format checks, and lint.
