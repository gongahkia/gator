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

`make run` still launches Electron during migration. `make terminal` launches
the actual OpenCode-derived terminal against Gator's Go runtime. Use Ctrl+P for
the searchable launcher; `/following`, `/saved`, `/settings`, `/files`, `/queue`,
`/chats`, `/drafts`, `/updates`, `/memories`, `/receipts`, and `/voice` expose Gator
workflows. Ctrl+J adds a line where Shift+Enter is unavailable. F2 opens file
versions/export/Ask Gator, F3 edits Office/table content, Ctrl+S saves an immutable
working version, and F4 discards edits. Returning to chat retains the draft.

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
  Linux. They speak only newly received replies when enabled.
- Existing macOS-only computer control and Windows-unavailable strict sandbox/
  interactive agent terminal tools retain their current limits.

Native runtime and permission checks on macOS/Windows will be completed later as
agreed; those checks remain pending and do not count as a passing removal gate.
