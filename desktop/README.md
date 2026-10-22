# Gator desktop

Release signing, notarization, update behavior and the remaining account/native
setup are documented in [desktop releases](../docs/DESKTOP-RELEASES.md).
Development builds keep using manual updates; neither signing account is set up
yet. Current source-checkout commands are in [development](../docs/DEVELOPMENT.md).

Gator's primary and currently supported user interface is this Electron app.
React DOM and Tailwind render the frontend; the private Go runtime owns
conversations, tools, model configuration, approvals, goals, usage, and data.
The standalone CLI and React terminal renderer are retired.

Product direction and the remaining capability gaps are recorded in the
[22 October competitive audit](../docs/COMPETITIVE_GAP_AUDIT_2026-10-22.md).

## Run

From the repository root, with Go 1.25.13 and Node.js 22.12+ installed:

```sh
make run
```

`make run` (or `make desktop`) installs the pinned desktop dependencies when
needed, builds the private runtime and frontend, then opens Gator.
On Windows or without Make, run `npm ci --prefix desktop` followed by
`npm start --prefix desktop`. Installed apps
bundle Electron's Node runtime; users do not need Node or a Gator CLI on PATH.

The app uses the existing local Gator profile and reopens your last selected
chat. **New chat** (Cmd/Ctrl+N) starts a separate transcript and working context.
Saved personal memories carry across chats; file and connection selections keep
their conversation scope. Recent chats and Saved provide access to earlier work.
Following groups goals, configured checks and updates.

Before any chat exists, Gator starts in a private empty workspace.
Conversation information → This conversation provides the folder picker. Choosing
a different folder starts a separate chat. To choose a starting folder during
development:

```sh
GATOR_DESKTOP_WORKSPACE=/absolute/path/to/notes make desktop
```

`GATOR_STATE_DIR` and `GATOR_CONFIG_DIR` select the local profile.
`GATOR_DESKTOP_USER_DATA` selects Electron's separate preferences/drafts directory.
`GATOR_DESKTOP_BINARY` can select a separately built private runtime during
development. These overrides are useful for isolated testing.

## Shared memory and image paste

Useful stable facts and preferences from your own messages can be saved with a
source chat. Browse Saved to inspect, correct or forget them. **Memory & retrieval**
controls automatic learning; turning it off keeps existing memory and explicit
remember/forget requests available. Earlier chat details can be retrieved on
demand. New chat refreshes the working context without deleting saved memory.
This is local profile memory, with no automatic device synchronization or model
retraining. See [the implemented boundaries](../docs/DESKTOP-CONVERSATIONAL-IMPLEMENTATION.md#shared-memory-and-earlier-chat-retrieval).

Paste screenshots or copied PNG/JPEG/WebP images into the message box with
Cmd/Ctrl+V. They become previewable attachments; nothing is sent until Send.
Text paste remains ordinary text. The existing limits apply: four attachments,
4 MiB per file and 8 MiB per message. Image decoding happens on this computer.

## Composer model picker

The model pill beside Send opens a compact Work effort control. Use its model
dropdown to choose a configured cloud or local model, grouped by provider.
Manage models opens the existing setup screen for credentials, downloads and
other model administration. Model display aliases retain their original model IDs.

Low, Standard and High reuse Gator's existing task budgets; they do not change
provider thinking settings. Reset restores Standard effort without switching the
model. Model selection is saved for subsequent requests; effort keeps the existing
conversation/session behavior. Current and queued work must finish before either
setting changes. Drafts and attachments stay in the composer.

## Message recovery

Chats can work independently. Open another chat or browse the app while a task
continues; its follow-ups, steering, approval requests and results stay with the
original chat. The sidebar and the small activity row show other open tasks.
The host permits three running chats and retains at most four chat processes.
It does not evict running tasks or paused queues to make space. Model selections
are captured with admitted messages, so another chat changing the default does
not silently change a waiting request's model.

Closing the app stops tasks in all its open chats. Restart/update checks cover
all those chats, and reconnecting affects the selected chat. This is local
execution; an always-on remote companion is still separate implementation work.

The composer clears ordinary messages only after the runtime confirms admission
to the private conversation journal. Queued messages retain their text, order,
edits, settings and attachment references across a restart. Restored messages
stay paused; choose an individual message to send or remove it.

Interrupted requests are identified separately because earlier actions may have
completed before the connection was lost. They are not replayed automatically.
An explicit retry creates a new attempt, retaining the interrupted receipt.
This does not restore a model's in-flight execution checkpoint. Files referenced
by pending requests are protected from automatic attachment cleanup.

## Files inside chat

Click an attached or generated file to open it directly in its message. The
compact Preview/Edit header supports text, Markdown, code, JSON and CSV/TSV,
plus native Word paragraph/table text, Excel cells/formulas and PowerPoint
slide text. Images and PDFs have visual previews. Office rendering is simplified;
complex layout and drawing tools remain in the native apps.

Save creates a new private version and includes its bytes in your next message,
subject to the existing attachment limits. Ask Gator stages a request with the
file/version and selected text or cell; it does not send automatically. File
edit drafts survive reload independently of message drafts. Save or Discard
before sending so Gator receives the version you intend. Version history can
restore earlier contents as another version. Export saves a copy to a chosen
location; original uploads and historical outputs stay intact.

Files are opened on demand. Previewing and direct editing do not call a model
or upload the file to a document-rendering service. File working versions and
drafts are included in private backups and explicit history/all-content deletion.
Automatic age retention leaves them until you explicitly delete them.
See [format support, architecture and verification](../docs/INLINE-FILES-RESEARCH.md).

## Ask Gator about the app

You can ask questions such as “Where do I change the model?”, “Which model are
you using?”, “What can you access?”, or “How do I manage what you remember?”.
Each request includes a bundled guide to Gator's actual desktop controls and a
limited snapshot of the current model, effective work mode, selected connections,
and supplied saved-context count. The snapshot excludes credentials and arbitrary
app logs. Gator can inspect live settings and apply explicitly requested changes
through app tools, including model selection, usage limits, retention, connection
selections, and shortcut rebinding. Higher-impact changes use the existing review
flows. The composer also supports microphone dictation into a reviewable draft
and optional spoken replies. See [chat controls and voice](../docs/APP-CONTROL.md).

## Check and package

```sh
npm run check --prefix desktop
npm test --prefix desktop
make build
make package
```

`make build` builds the frontend and private `desktop/runtime/gator-runtime`
(`gator-runtime.exe` on Windows).
`make package` creates an unpacked native app under `desktop/release/`.
On macOS, it is `desktop/release/mac/Gator.app`. To exercise that package:

```sh
GATOR_DESKTOP_TEST_EXECUTABLE="$PWD/desktop/release/mac/Gator.app/Contents/MacOS/Gator" \
  npm run test:e2e --prefix desktop
```

### Local Electron test runner note

On 2026-10-23, the memory/restart checks on an Intel Mac running macOS 15.7.7
and Electron 44.9.2 completed with Node 24.19.0. With Node 26.7.0 on the same
machine, the runner repeatedly timed out during Electron shutdown after Gator's
draft/file flushes, runtime stops and native quit event had completed. This is a
local validation result, not a general Node 26 compatibility verdict. If you
encounter that shutdown timeout, select Node 24 LTS before rerunning the tests.

From `desktop/`, with `node --version` confirming the selected runner:

```sh
node node_modules/playwright/cli.js test tests/e2e/shared-chats.spec.ts \
  --grep 'new chats share grounded personal memory|earlier-chat privacy|saved-memory receipts|memory edits survive'
```

The fixtures use isolated profiles and local scripted providers. Their normal
close path must complete before restart; a forced cleanup still fails the test.

### Distribution

`make dist` builds desktop installers. The release workflow produces macOS
DMG/ZIP, Linux AppImage/DEB, and Windows NSIS/ZIP artifacts with matching runtime architectures.
Windows is a preview port; see [capabilities and verification](../docs/WINDOWS.md).
macOS builds are unsigned development builds until signing and notarization
are configured. No standalone CLI archives are built or installed.

The executable under `runtime/` is an internal app component. Bare invocation
and former user CLI commands direct users to the desktop app. App-owned
background services and reviewed system authentication/installation handoffs
retain their existing paths. Such a handoff may open Terminal temporarily;
it does not launch an interactive Gator terminal interface.

For developer-only fixtures and maintenance, `GATOR_INTERNAL_COMMANDS=1`
explicitly enables the existing runtime command handlers. This is not a
supported user interface or a security boundary. Never substitute it for
an action review. See [the retirement decision](../docs/DESKTOP-PRIMARY.md).

The live surface mapping and remaining application work are documented in
[the conversational implementation](../docs/DESKTOP-CONVERSATIONAL-IMPLEMENTATION.md).
