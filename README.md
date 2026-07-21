# Gator

Gator is a Neovim workspace for steering, reviewing, and coordinating existing coding-agent CLIs. It preserves provider-native authentication, model selection, sessions, tool loops, compaction, and sandboxing while adding local context provenance, task/run evidence, and editor-native workflows.

Gator is MIT licensed for both the Lua plugin and optional Rust `gator-index` sidecar; see [LICENSE](LICENSE).

## Support and compatibility

Supported Neovim baseline: **0.11+**. The public CI matrix checks Neovim 0.11 and stable on current macOS and Ubuntu runners. The optional sidecar requires Rust stable; `sqlite3` is required for sidecar/database workflows. Git is required for workspace and review operations.

Provider CLIs are optional and provider-owned. Install and authenticate each CLI separately, then use `:GatorHealth` to see its local readiness. The exact supported provider surfaces, fixture versions, and explicit limitations are in [docs/PROVIDERS.md](docs/PROVIDERS.md). Windows is not in the current CI matrix.

## Install

With `lazy.nvim`:

```lua
{
  "gongahkia/gator",
  config = function()
    require("gator").setup()
  end,
}
```

For a Unix native-package install:

```sh
git clone https://github.com/gongahkia/gator.git \
  "${XDG_DATA_HOME:-$HOME/.local/share}/nvim/site/pack/gator/start/gator"
```

Then restart Neovim and run:

```vim
:GatorHealth
:Gator
```

`GatorHealth` uses provider-native non-interactive probes where available. It does not read or store provider credentials.

## Setup and configuration

Minimal setup:

```lua
require("gator").setup()
```

The supported configuration surface is:

```lua
require("gator").setup({
  schema_version = 2,
  ui = {
    layout = "adaptive", -- "adaptive" or "modal"
    keymaps = {},
    screen_reader = true,
    motion = { enabled = true, interval_ms = 120, reduced = false },
  },
  context = { mode = "manual", trust = "provenance" },
  sessions = { transfer = "manual" },
  workspaces = { mode = "project", max_write_runs = 1 },
  persistence = { sharing = "local" },
  telemetry = { enabled = false, redaction_patterns = {} },
})
```

To opt into `stdpath("config") .. "/gator.json"`, load it explicitly. The optional second result identifies the source for each resolved field:

```lua
local settings, provenance = require("gator.config").load()
require("gator").setup(settings)
```

Do not put provider credentials, tokens, secrets, passwords, or API keys in Gator configuration.

## Commands and workflow

| Command | Supported behavior |
| --- | --- |
| `:Gator` | Opens the task/session/context/review workspace. |
| `:GatorHealth` | Runs local compatibility, workspace, policy, sidecar, and provider readiness checks. |
| `:{range}GatorCaptureSelection task:<id>` | Captures a visual/line selection as provenance-tracked context for a task. |
| `:{range}GatorCaptureSelection session:<provider>:<id>` | Captures context for an opaque provider-native session. |
| `:GatorPalette <action>` | Runs a registered Gator palette action. |

Run `require("gator").setup()` before `:GatorCaptureSelection`. Open `:Gator` to inspect explicit empty, loading, failure, recovery, and unavailable-provider states. Core actions open task, linked-session, context-inspection, and review panels when their local evidence is available.

## Keyboard and screen reader UX

Every Gator panel is keyboard-first and buffer-local: `j`/`k` moves the current selection, `<CR>` confirms or opens it, `q` closes or cancels, and `?` shows panel help. Context uses `<Space>` to include or exclude an entry; review uses `a`/`r` to accept or reject a hunk; timelines use `<Space>` to collapse or expand a call. Focus stays in the review controls after opening a diff, and closing the primary workspace restores the prior user window.

Override supported action names through `ui.keymaps`; only panels that implement an action receive its override. Set `ui.screen_reader = true` (the default) for plain, read-only `gator-text` buffers that include selection, status, policy, and decision text.

```lua
require("gator").setup({
  ui = {
    keymaps = { next = "]", previous = "[", confirm = "<C-m>", cancel = "<Esc>" },
    screen_reader = true,
  },
})
```

## Safety and provider model

Gator is a native-first meta-harness. It may narrow a provider action or require confirmation; it never broadens provider permissions. Project instructions are provenance-tracked context and require explicit trust before transfer.

Provider runs are validated against their task, context pack, narrowed run policy, and advertised capability before launch. The asynchronous supervisor bounds runtime and in-memory output, reports typed exit/timeout/cancellation states, escalates cancelled processes, and terminates managed runs during editor shutdown; it never persists provider output or credentials.

## Troubleshooting

1. Run `:GatorHealth` from the project you intend to use.
2. Resolve the reported requirement: Neovim version, Git workspace, configuration/policy issue, optional sidecar dependency, or provider CLI readiness.
3. For an unavailable provider, verify its installed CLI/version against [docs/PROVIDERS.md](docs/PROVIDERS.md) and complete authentication in that provider’s own CLI.
4. Reopen `:Gator`; its workspace exposes recovery and unavailable-provider state rather than assuming a provider is usable.
5. For a reproducible plugin failure, run `make check` from the repository checkout.

## Optional sidecar and development

`gator-index` is optional. Install Rust stable and `sqlite3` before using sidecar/database workflows. `make sidecar` starts the sidecar from `crates/gator-index`; `make check` runs Lua tests, Rust tests, formatting, and lint checks; `make benchmark` runs fixture-backed performance budgets.

Contributions require `make check` before a pull request. Provider changes also require fixtures and a provider-matrix update; see [CONTRIBUTING.md](CONTRIBUTING.md).

## Uninstall

Remove the plugin through your plugin manager, or remove the native-package directory used during installation. To remove local Gator configuration and state, first inspect these paths in Neovim:

```vim
:echo stdpath('config') .. '/gator.json'
:echo stdpath('state') .. '/gator'
```

Delete only the paths you inspected if you also want to remove Gator’s local configuration, run/session metadata, review evidence, and workspace links. This does not remove provider CLI credentials because Gator does not own them.

## Release, support, and security

Gator follows semantic versioning; supported behavior changes are recorded in [CHANGELOG.md](CHANGELOG.md). Report bugs and feature requests through GitHub issues as described in [SUPPORT.md](SUPPORT.md). Report vulnerabilities privately through GitHub Security Advisories as described in [SECURITY.md](SECURITY.md).

## Supported vs. planned

Only behavior implemented in this repository, `:GatorHealth`, and [docs/PROVIDERS.md](docs/PROVIDERS.md) is supported. Provider capabilities not advertised by the installed CLI, authenticated checks marked unavailable, and unlisted provider integrations are not promised behavior.

Gator is MIT licensed. See [CHANGELOG.md](CHANGELOG.md), [CONTRIBUTING.md](CONTRIBUTING.md), [SECURITY.md](SECURITY.md), and [SUPPORT.md](SUPPORT.md).

### Extension and marketplace trust

Gator extensions, provider CLIs, and marketplace packages execute with your user privileges. Install only extensions from maintainers you can verify; review their source, release provenance, requested permissions, and dependency changes before enabling them. Marketplace publication is not a security review or a trust guarantee. Do not install a package that asks you to disable provider sandboxing, expand permissions, or share credentials.
