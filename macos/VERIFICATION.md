# Native macOS Tranche 01 verification

Verified on macOS 15.7.7 (Intel), Swift 6.1.2 and the macOS 15.5 SDK. This is a development bundle, without distribution signing or notarization. All implementation files are under `macos/`; terminal, host and Go application code were left unchanged.

## Ownership and contracts

`GatorApp` creates one `GatorApplicationState` above its `MenuBarExtra(.window)` and full `Window` scenes. Presentation tasks call an idempotent start method. Neither presentation closes the transport when dismissed. Only explicit reconnect or guarded application shutdown replaces/stops that owner.

`GoBridgeConnection` launches the bundled runtime as `ui-backend <conversation-id-or-empty> ""`, with `GATOR_UI_BACKEND=1` and `GATOR_UI_SESSION_HOST` unset. It uses byte-framed NDJSON, serialized writes and bounded state/diagnostic buffers. There is no production network transport or additional agent runtime.

Snapshots replace state. Text admission uses `upload_submit`, context and durable request IDs, including an empty files array. Transport nonce acknowledgement does not clear drafts: matching `upload_result` does. Model/effort controls similarly await matching `composer_result` and projected values. Stop submits Go's `/cancel` and awaits projected cancellation. Approval sends the original interaction ID and token after verifying generation, conversation, context, message index and exact review data.

Native drafts are stored in a separate `native/drafts` namespace, partitioned by state profile. Pending admission IDs survive disconnect/restart as uncertain; reconnect never resends them or approvals. An explicit original-admission retry reuses the same ID. Unreadable storage disables editing/sending and preserves the original file.

## Reproducible commands

Run from the repository root unless indicated:

```sh
go build -o /tmp/gator-native-runtime ./cmd/gator
swift build --package-path macos --scratch-path /tmp/gator-native-build
swift test --package-path macos --scratch-path /tmp/gator-native-build
bash macos/scripts/build-dev-app.sh
```

All passed. Swift Testing: **21 tests passed**, including subprocess cleanup under a blocked stdin pipe, real Go streaming through a local controlled provider, durable history restoration and real process failure/reconnect. Tests use temporary profiles and dummy credentials; no real provider/account calls are required. Integration tests expect the Go binary above, or `GATOR_TEST_BINARY` pointing to it, and the already-installed Node executable for fixtures.

The default development destination is `/tmp/Gator-Native.app`. Packaging refuses to overwrite an existing destination. The final verified artifact, including the last draft-storage safeguard, was built successfully with:

```sh
GATOR_NATIVE_APP_PATH=/tmp/Gator-Native-Verified.app bash macos/scripts/build-dev-app.sh
```

Both `Contents/MacOS/GatorMac` and `Contents/Resources/gator-runtime` are executable; bundled `Info.plist` passes `plutil -lint`. Runtime lookup does not require a checkout path.

Existing regression commands also passed:

```sh
go test ./cmd/gator ./internal/workapp -run 'TestUIBridge|TestBridgeKey|TestManaged|TestBridgeRecent|TestBridgeUploads|TestComposer|TestPreferenceRefresh|TestDesktopApproval|TestBridgeQueue' -count=1

node --test app-host/tests/backend.test.mjs app-host/tests/sessions.test.mjs app-host/tests/drafts.test.mjs app-host/tests/uploads.test.mjs app-host/tests/policy.test.mjs

# From ui/:
GATOR_TEST_BINARY=/tmp/gator-native-runtime bun test --preload ./scripts/preload.ts tests/client.test.ts tests/approvals.test.tsx tests/recovery.test.tsx
```

Results: both Go packages passed; **63 host tests passed**; **15 terminal tests passed with 121 assertions**. These are targeted regressions, not claims that every repository test was run.

## Actual application interaction evidence

The packaged application was launched locally through LaunchServices with disposable configuration/state/workspace/native-draft paths. Accessibility interaction and targeted keyboard events exercised actual native windows; screenshots were inspected. Controlled local provider fixtures supplied responses through the real Go bridge. These checks are additional to core state fixtures:

| Journey | Observed result |
| --- | --- |
| Menu-bar icon opens compact chat; Expand opens full window | Both native presentations opened. |
| Shared draft and selected conversation | Both native composers showed the same draft; Expand retained the same Go PID and conversation. |
| Submit text and stream | Real Go activity text arrived before completion; final answer appeared without repeated snapshot messages. |
| Dismiss compact and close full window while running | Owned Go PID remained alive; reopening showed completed work and the retained draft. |
| Exact approval | Real Go projected a controlled web-access review; compact and full windows retained its identity. Approve Exact Action completed it using its token. No external web fetch was performed. |
| Quit during active work | Native warning appeared; Keep Gator Open retained the same child process. |
| Stop | Visible Stopping state was followed by stopped state; Go process stayed alive. |
| Disconnect and explicit reconnect | Terminating the owned Go process exposed disconnected state. Explicit reconnect launched a replacement, kept the durable conversation/draft, and sent no automatic work. |
| Restart | Guarded idle Quit reaped the child; relaunch restored conversation/history and draft. |
| Return / Shift+Return | Targeted Return submitted; Shift+Return persisted a newline in the native draft. |

The final artifact was launched again after the last safeguard change and showed the actual recovered Go conversation and editable native draft. Prior stopped Work was truthfully projected by Go as retained failed/interrupted history on restart; native code does not relabel it as resumed or completed.

Temporary evidence includes `/tmp/gator-native-approval.png`, `/tmp/gator-native-final-window.png`, `/tmp/gator-native-swift-tests.log`, `/tmp/gator-native-host-regression.log` and `/tmp/gator-native-tui-regression.log`. These are local session artifacts, not packaged resources or durable repository test expectations.

## Manual inspection on this Mac

```sh
# Menu-bar application; click its Gator speech-bubble icon:
open /tmp/Gator-Native-Verified.app

# Start with the full window visible instead:
open /tmp/Gator-Native-Verified.app --args --open-window
```

Quit a running instance before choosing a different development bundle. Ordinary launch uses existing Go configuration/state and creates its neutral workspace under `~/Library/Application Support/Gator/workspace`; no repository choice is required. Native drafts default to `~/Library/Application Support/Gator/native/drafts`. `GATOR_NATIVE_USER_DATA`, `GATOR_NATIVE_WORKSPACE` and `GATOR_NATIVE_RUNTIME` are optional development overrides; ordinary Go configuration/state environment variables remain authoritative.

For a user with missing model setup, the app should show the backend explanation and preserve the draft. Use existing terminal setup to configure a provider; native provider login/download/secret handling is outside this tranche. A configured label is explicitly not a connectivity-test claim.

Inspect: compact/full draft sharing; existing conversations and New Conversation while idle; model/effort selections; native text send and streamed activity; scroll away and Jump to Latest; exact approval review and expansion; Stop; close/reopen presentations; Quit guard; restart restoration. Use disposable profiles for destructive failure injection.

## Acceptance boundaries and remaining verification

Implemented and verified by controlled tests and actual application interactions: native build/bundle, both presentations, shared owner/state/drafts/reviews, real Go conversation and streaming, exact approval ownership, Go-owned Stop, presentation-independent execution, explicit reconnect without replay, draft restoration and targeted existing regressions.

No known failing required test remains. **Not verified:** a real model account or connected service; physical IME composition; VoiceOver navigation; exhaustive light-mode/contrast/font variants; exhaustive long-history scroll behavior. Native marked-text handling is implemented, but keyboard injection does not prove a physical IME workflow. Semantic colors and native controls are implemented; screenshots inspected during this session used dark appearance. No claim of live account verification or full accessibility acceptance is made.

Explicitly deferred: managed concurrent conversation owners, busy dialogue steering, queue editing, terminal handoffs, native slash-command workflows, Following, advanced memory, voice, importing attachments, file editing/exports and provider setup. Busy ordinary sends are disabled rather than silently queued. Retained plans, external handoffs and queued work expose their limitations. Reconnect starts a replacement owner subject to Go's execution leases; it does not reattach a live process or automatically resume work.

Recommended next tranche: a bounded native session coordinator, driven by one end-to-end dialogue-during-work workflow, using the existing Go ownership contracts. No such work was started here. The separately paused backend goal remains paused.
