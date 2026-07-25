<div align="center">
  <h1>Gator</h1>
  <p>Native-first coding-agent orchestration for Neovim.</p>
  <p>
    <a href="https://github.com/gongahkia/gator/actions/workflows/ci.yml"><img src="https://github.com/gongahkia/gator/actions/workflows/ci.yml/badge.svg" alt="CI"></a>
    <a href="https://github.com/gongahkia/gator/blob/main/LICENSE"><img src="https://img.shields.io/github/license/gongahkia/gator" alt="License"></a>
    <a href="https://github.com/gongahkia/gator/graphs/contributors"><img src="https://img.shields.io/github/contributors/gongahkia/gator" alt="Contributors"></a>
    <a href="https://github.com/gongahkia/gator/stargazers"><img src="https://img.shields.io/github/stars/gongahkia/gator?style=flat" alt="Stars"></a>
  </p>
</div>

---

Gator is a Neovim workspace for steering, reviewing, and coordinating existing coding-agent CLIs. Providers retain ownership of authentication, model selection, sessions, tool loops, compaction, and sandboxing; Gator adds local context provenance, task/run evidence, and editor-native workflows.

Highlights
----------

- **Native-first** — uses provider-owned authentication and sessions; never broadens provider permissions.
- **Local-first** — context, tasks, diagnostics, and persistence stay local by default.
- **Evidence-driven** — validates task, context, policy, and provider capability before a run starts.
- **Editor-native** — keyboard-first task, terminal, context, and review workflows inside Neovim.
- **Extensible** — supports provider adapters and an optional Rust `gator-index` sidecar.

Table of Contents
-----------------

<!-- vim-markdown-toc GFM -->

- [Installation](#installation)
  - [Requirements](#requirements)
  - [lazy.nvim](#lazynvim)
  - [Native package](#native-package)
  - [First run](#first-run)
- [Usage](#usage)
  - [Commands](#commands)
  - [Launch a task](#launch-a-task)
  - [Capture context](#capture-context)
  - [Keyboard controls](#keyboard-controls)
- [Configuration](#configuration)
  - [File-based configuration](#file-based-configuration)
- [Advanced use](#advanced-use)
- [Providers](#providers)
- [Privacy and safety](#privacy-and-safety)
- [Diagnostics and troubleshooting](#diagnostics-and-troubleshooting)
- [Development](#development)
- [Uninstall](#uninstall)
- [Support and security](#support-and-security)
- [License](#license)

<!-- vim-markdown-toc -->

Installation
------------

### Requirements

- Neovim 0.11+
- Git for workspace and review operations

The optional `gator-index` sidecar additionally requires Rust stable and `sqlite3`. Provider CLIs are optional and must be installed and authenticated independently. Windows is not in the current CI matrix.

### lazy.nvim

```lua
{
  "gongahkia/gator",
  config = function()
    require("gator").setup()
  end,
}
```

### Native package

```sh
git clone https://github.com/gongahkia/gator.git \
  "${XDG_DATA_HOME:-$HOME/.local/share}/nvim/site/pack/gator/start/gator"
```

### First run

Restart Neovim, then run:

```vim
:GatorHealth
:Gator
```

`:GatorHealth` uses provider-native non-interactive probes where available. It does not read or store provider credentials.

Usage
-----

### Commands

| Command | Description |
| --- | --- |
| `:Gator` | Open the task, session, context, and review workspace. |
| `:GatorHealth` | Check compatibility, workspace, policy, sidecar, and provider readiness. |
| `:GatorPalette [kind:name]` | Open the palette or execute a registered action, task, or provider entry. |
| `:{range}GatorCaptureSelection task:<id>` | Capture a line or visual selection as provenance-tracked task context. |
| `:{range}GatorCaptureSelection session:<provider>:<id>` | Capture context for an opaque provider-native session. |
| `:GatorExportDiagnostics` | Write a local, redacted diagnostic JSON snapshot. |
| `:GatorBetaReadiness` | Verify beta prerequisites and write a local readiness report. |

Run `require("gator").setup()` before `:GatorCaptureSelection`.

### Launch a task

From a Git workspace, open `:Gator`, choose **Create local task**, enter an objective, then choose **Launch selected task**. Gator writes the task to `.gator/tasks/<id>.md` and presents only locally-ready providers.

Claude Code, Codex, OpenCode, and Pi retain their interactive terminals. Managed providers open a Gator conversation panel: use `i` to prompt, `c` to cancel the current run, and `q` to close the panel. One-shot providers become review-ready when their run exits; **Attach selected session** resumes a documented session or reopens its local history. `.gator/` is ignored by default, so task files remain local to the checkout.

The palette provides the same workflow:

```vim
:GatorPalette action:create-task
:GatorPalette action:import-tasks
:GatorPalette action:launch-task
:GatorPalette action:attach-session
:GatorPalette action:refresh-providers
```

Claude Code, Codex, and OpenCode require a supported executable, provider-native authentication, and a ready session bridge. Pi, Aider, Amp, Cline, Copilot, Cursor, Droid, Gemini, Goose, Kimi, and Vibe require both their documented CLI contract and an explicit `providers.<name>.user_confirmed = true` opt-in. “User-confirmed” means only that you say the CLI is configured; Gator does not verify credentials. ACP providers use Gator approval UI only when they send an ACP permission request; Aider, Amp, Cursor, and Droid retain their provider-default policy. Copilot launch is supported, but its documented ACP profile does not support session reattach.

### Capture context

Select lines in normal or visual mode, then target a task or provider-native session:

```vim
:'<,'>GatorCaptureSelection task:fix-parser
:'<,'>GatorCaptureSelection session:codex:thread-id
```

Gator records provenance for captured context. Project instructions require explicit trust before transfer.

### Keyboard controls

Gator panels are keyboard-first and buffer-local:

- `j` / `k` — move selection
- `<CR>` — confirm or open
- `q` — close or cancel
- `?` — show panel help

Context uses `<Space>` to include or exclude an entry; review uses `a` / `r` to accept or reject a hunk; timelines use `<Space>` to collapse or expand a call. Set `ui.screen_reader = true` (the default) for plain, read-only `gator-text` buffers.

Configuration
-------------

Minimal setup:

```lua
require("gator").setup()
```

The default configuration is:

```lua
require("gator").setup({
  schema_version = 3,
  ui = {
    layout = "adaptive",
    keymaps = {},
    screen_reader = true,
    motion = { enabled = true, interval_ms = 120, reduced = false },
  },
  context = {
    mode = "manual",
    trust = "provenance",
    handoff = { author = "user", max_chars = 4096, review = "required" },
  },
  sessions = { transfer = "manual" },
  providers = {
    pi = { user_confirmed = false },
    aider = { user_confirmed = false }, amp = { user_confirmed = false },
    cline = { user_confirmed = false }, copilot = { user_confirmed = false },
    cursor = { user_confirmed = false }, droid = { user_confirmed = false },
    gemini = { user_confirmed = false }, goose = { user_confirmed = false },
    kimi = { user_confirmed = false }, vibe = { user_confirmed = false },
  },
  workspaces = { mode = "project", max_write_runs = 1 },
  persistence = { sharing = "local" },
  telemetry = { enabled = false, redaction_patterns = {} },
})
```

Override panel actions through `ui.keymaps`; an override applies only to panels that implement that action.

### File-based configuration

To opt into `stdpath("config") .. "/gator.json"`, load it explicitly:

```lua
local settings, provenance = require("gator.config").load()
require("gator").setup(settings)
```

The optional `provenance` result identifies the source for each resolved field. Unversioned, schema-v1, and schema-v2 files migrate to schema v3 in memory; Gator does not rewrite the file.

> [!WARNING]
> Do not put provider credentials, tokens, secrets, passwords, or API keys in Gator configuration.

Advanced use
------------

Automation can inspect local compatibility and immutable coordinator state:

```lua
local gator = require("gator")
local manifest = gator.compatibility_manifest()
local manifest_json = gator.compatibility_manifest_json()
local snapshot = gator.inspect()
```

The compatibility manifest reports Neovim version and local capability status only. `inspect()` returns a versioned snapshot; its `schema_version` is the inspection contract version. Neither API probes, stores, or exposes provider credentials.

Providers
---------

Gator provides adapters for Aider, Amp, Cline, Cursor Agent, Codex, Claude Code, Droid, Gemini CLI, Goose, Kimi Code CLI, Mistral Vibe, Copilot CLI, OpenCode, and Pi.

Installed capabilities are probed, not assumed. Run `:GatorHealth` from the project you plan to use; an operation is available only when the installed CLI advertises the required capability. See the [provider support matrix](docs/PROVIDERS.md) for fixture-tested versions, auth probes, managed transport, and limitations.

Privacy and safety
------------------

Gator is a native-first meta-harness: it can narrow a provider action or require confirmation, but it does not broaden provider permissions. Provider runs are checked against task, context pack, narrowed run policy, and advertised capabilities before launch.

Gator does not persist provider credentials, output, or transcripts, and it has no general persistent activity log. Telemetry is disabled by default. Local diagnostic exports are redacted and not sent automatically.

> [!IMPORTANT]
> Extensions, provider CLIs, and marketplace packages run with your user privileges. Review their source, release provenance, requested permissions, and dependency changes before enabling them. Do not install code that asks you to disable sandboxing, expand permissions, or share credentials.

Diagnostics and troubleshooting
-------------------------------

For a local diagnosis:

1. Run `:GatorHealth` from the affected project.
2. Run `:GatorExportDiagnostics`; for startup/readiness failures, also run `:GatorBetaReadiness`.
3. Review the artifacts before sharing them.
4. Check the provider's installed CLI/version against the [provider support matrix](docs/PROVIDERS.md).

Diagnostic reports remain local until explicitly shared. Do not attach credentials, API keys, private prompts, provider transcripts, unredacted logs, or private repository paths. Full report locations and safe bug-reporting guidance are in [Diagnostics](docs/DIAGNOSTICS.md).

Development
-----------

```sh
make check
make benchmark
make sidecar
```

- `make check` runs Lua tests, Rust indexer tests, format checks, and lint.
- `make benchmark` runs fixture-backed performance budgets.
- `make sidecar` starts the optional Rust indexer.

See [Contributing](CONTRIBUTING.md) for prerequisites and contribution requirements, and [Performance](docs/PERFORMANCE.md) for benchmark limits.

Uninstall
---------

Remove Gator through the plugin manager or delete the native-package directory used during installation. To remove local Gator configuration and state, first inspect these paths in Neovim:

```vim
:echo stdpath('config') .. '/gator.json'
:echo stdpath('state') .. '/gator'
```

Delete only the inspected paths if you also want to remove Gator's local configuration, run/session metadata, review evidence, and workspace links. Provider CLI credentials are managed outside Gator.

Support and security
--------------------

Use GitHub issues for reproducible bugs and feature requests; see [Support](SUPPORT.md). Report vulnerabilities privately through GitHub Security Advisories; see [Security](SECURITY.md).

License
-------

Gator is [MIT licensed](LICENSE). Supported behavior and release changes are recorded in [CHANGELOG.md](CHANGELOG.md).
