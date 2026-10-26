# Gator terminal development and installation

The terminal migration is in progress. Electron remains the reference until the
[parity gate](TERMINAL-MIGRATION-PLAN.md) passes. Check the
[verification ledger](TERMINAL-MIGRATION-VERIFICATION.md) for actual evidence.

Use Go 1.25.13+, Node 22.12+ for build scripts/shared-host tests, and Bun 1.4.2.
The selected frontend uses Solid 1.9.15 and OpenTUI 0.6.8 with native platform
assets; a Node-only terminal launch is not claimed.

```sh
make terminal-build
make terminal
make terminal-check
```

The Go build uses a temporary directory under `ui/runtime/` and removes it after
success or failure, avoiding small RAM-backed `/tmp` filesystems. An explicit
`GOTMPDIR` overrides that default and remains owned by you.

`make run` still launches Electron during migration. `make terminal` launches
the actual OpenCode-derived terminal against Gator's Go runtime. Use Ctrl+P for
grouped menus and searchable terminal actions. Slash commands appear in the
composer: type `/`, use ↑/↓ to choose, Tab to complete, Enter to select, or Esc
to dismiss. Commands needing arguments stay editable. The menu sections retain
the same capabilities without a flat slash-command list. `/following`, `/saved`, `/settings`, `/files`, `/queue`,
`/chats`, `/drafts`, `/updates`, `/memories`, `/receipts`, and `/voice` expose Gator
workflows. F1 or `/help` shows terminal help; `/prompt-history` stages retained
message text without sending it. Ctrl+J adds a line where Shift+Enter is unavailable. F2 opens file
versions/export/Ask Gator, F3 edits Office/table content, Ctrl+S saves an immutable
working version, and F4 discards edits. F6 selects saved cell/paragraph content
for Ask Gator. Tables page with Ctrl+Up/Down (rows) and Ctrl+Left/Right (columns);
F7 switches worksheets. Returning to chat retains the draft. Exporting to an
existing file shows a separate replacement review; changed destinations require
a fresh review. Export outside Gator's private storage.
Settings → Models and providers (also `/models`) separates model selection,
effort, and provider/local-model setup. Escape in a model or effort list returns
to that parent menu. Panels show one consolidated keyboard hint bar.
In forms, F2 and Ctrl+R perform the displayed save/review action; Enter edits
the selected field. Ctrl+Enter also works in terminals that distinguish it.
Provider configuration and model rename forms use their existing Go save action;
a custom provider still requires its separate confirmation review. Unsaved form edits require an explicit discard decision.
In Conversations, Ctrl+F searches indexed messages, Ctrl+L changes scope,
Ctrl+Left/Right pages results, F3 manages the selected chat, and Ctrl+R/F5 refreshes.
Choose `external-editor` in Ctrl+P to edit the idle draft with `$VISUAL`, `$EDITOR`
or the existing `nvim` fallback. Gator suspends terminal rendering and restores
text and attachments after exit. The current Go editor command requires POSIX
`sh`; native Windows editor support remains unverified. `continue-external`
resumes other Go-reviewed sign-in/install handoffs in the current terminal.

Build and install a local artifact into a new prefix:

```sh
node ui/scripts/package.mjs /tmp/gator-terminal-artifact
node ui/scripts/install.mjs /tmp/gator-terminal-artifact /tmp/gator-install
/tmp/gator-install/bin/gator --help
/tmp/gator-install/bin/gator
node ui/scripts/uninstall.mjs /tmp/gator-install
```

The installed product needs Bun on PATH, but no checkout, Go compiler, Node,
Electron, development dependencies, or OpenCode server. Optional browser tools
retain their existing external Node/Playwright requirements. Windows uses the
console launcher `gator.cmd`; native Windows/macOS runtime evidence is pending.
The installer refuses to replace an existing path. To update, finish work, save
drafts, stop background checks through Recovery, install a reviewed artifact at
a new prefix, then use Recovery's existing reviewed service ownership flow to
move/re-enable the executable. Do not simply delete an active service executable.
Signing, publishing and release-account configuration are separate work.

Profiles keep Gator's existing configuration/state directories and file formats.
The terminal reads the same per-profile draft store under Electron's established
Gator user-data directory. `GATOR_DESKTOP_USER_DATA`, `GATOR_DESKTOP_WORKSPACE`,
state/config overrides and existing desktop-named Go preferences remain
compatible. `GATOR_TERMINAL_USER_DATA` and `GATOR_TERMINAL_WORKSPACE` explicitly
select alternate roots. No private data is imported into OpenCode storage.
Uninstall leaves all profiles intact. Tests must use isolated roots.
On first terminal use, sidebar and recent-chat grouping preferences are imported
from a private copy of Gator's Chromium Local Storage into
`frontend-preferences.json`. The original database remains unchanged. Subsequent
terminal choices take precedence; the Go profile remains authoritative for theme,
shortcuts and other application settings. Work-details disclosure choices are
retained, while terminal conversation information uses explicit menus.

Native adapters:

- Images render through OpenTUI (native terminal graphics or its raster fallback).
  PDFs rasterize actual pages through PDF.js/native canvas and support page
  navigation. Office editing preserves the existing content/package boundaries.
- Clipboard: macOS AppKit/pbcopy/pbpaste; Windows PowerShell/Windows Forms;
  Linux `wl-clipboard` on Wayland or `xclip` on X11. Missing helpers report errors;
  copying text also supports OSC 52 when the terminal allows it.
- Dictation requires `ffmpeg` and explicit OS microphone permission. The helper
  uses PulseAudio on Linux, AVFoundation on macOS and DirectShow on Windows.
  `GATOR_MICROPHONE_DEVICE` selects a device (on Windows, its DirectShow audio
  device name). Audio is transient and transcription is explicitly reviewed in
  its original draft. Configure the provider/key through Models and Voice.
- Spoken replies use `say` on macOS, System.Speech on Windows, or `espeak-ng` on
  Linux. They speak only newly received replies when enabled. macOS and Windows
  choose an installed voice matching the configured language (exact locale first,
  then the same primary language); missing voices produce a visible error. Speech
  rate is platform-relative: Windows maps 0.5–2 to its engine-specific -10–10
  scale, while macOS/Linux use words per minute. Native audio checks are pending.
- Existing macOS-only computer control and Windows-unavailable strict sandbox/
  interactive agent terminal tools retain their current limits.

Native runtime and permission checks on macOS/Windows will be completed later as
agreed; those checks remain pending and do not count as a passing removal gate.

Public work activity stays chronological while running and folds above the answer
when finished. Use `activity` in Ctrl+P to inspect live/retained entries or expand
a history summary. Transcript search includes narration in expanded history.
Use `work-details` in Ctrl+P to inspect current selections and retained evidence.
`outputs` lists retained previews and proposed changes, including the latest result
when it has no transcript card. File checks and proposal inspection do not apply
changes or confirm external delivery. `/files` opens complete retained files and
their working versions; unavailable originals report an error.

The Unix installed-terminal verification uses Python and a pinned terminal
emulator only for testing:

```sh
python3 -m venv .pty-venv
.pty-venv/bin/pip install -r ui/scripts/requirements-pty.txt
.pty-venv/bin/python ui/scripts/verify-package.py /tmp/gator-terminal-artifact --editor --untrusted-file
```
