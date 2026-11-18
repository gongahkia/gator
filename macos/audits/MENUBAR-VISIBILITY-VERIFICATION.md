# Menu-bar icon preference verification

Date: 2026-11-18. Completed before CH08 implementation. Starting HEAD for this addition was `c748d94ad39589fcf069cef7fe31b979bfc92787`; external commits advanced HEAD to `7e5be43d04ae097670cb4581b70267d1a64e1413`. No agent-created commit or README change.

“Show Menu Bar Icon” is available in More and the native Conversation menu. It defaults on, persists in the application preference domain, and opens the existing conversation window before hiding the icon. Hidden-icon startup presents that window. Reopening the application presents the existing window, including when a notch panel is also visible. This is presentation state only; no owner replacement or execution request is involved.

The scene consumes a read-only binding to the preference. A direct AppStorage insertion binding reproduced a SwiftUI scene-update loop; a guarded writable binding still allowed stale scene insertion callbacks to overwrite the preference. Both intermediate versions were rejected. The final read-only scene binding passed hide/restore, Quit/relaunch and reopen checks.

Desktop verification used `/tmp/Gator-Native-MenuBar-Desktop.app`, preference domain `dev.gator.native.menubar-test-37d0637e`, disposable profile `/tmp/gator-menubar-test-r92gipgw`, and an unreachable loopback-only provider. No message was sent. Process 62128 retained direct Go child 62129 across hide/restore and window close/reopen. Both exited after guarded Quit. Relaunch process 65345 started with the icon absent, the full window present and the exact draft “Draft survives hiding the menu-bar icon” intact. Restoring showed the status item again; hiding it removed it again. Stored preference was false after Quit and relaunch. Final idle launch did not reproduce the scene loop. Screenshot: [setting](../parity/evidence/p01/menu-bar-setting.png).

Changed sources: `GatorApp.swift`, `MenuBarVisibilityToggle.swift`, `ConversationView.swift`, `NotchQuickAccess.swift`, `AppDelegate.swift`; the latter adds only reopen presentation routing, preserving Quit. `NativeInputTests.swift` adds the existing-scene reopen regression.

Verification:

```sh
swift test --package-path macos --scratch-path /tmp/gator-native-build --no-parallel
swift build --package-path macos --scratch-path /tmp/gator-native-build -Xswiftc -strict-concurrency=complete -Xswiftc -warnings-as-errors
```

171 tests passed in 50.211 seconds (`/tmp/gator-menubar-verified-tests.log`); strict build passed in 10.31 seconds (`/tmp/gator-menubar-verified-strict.log`). The package remains Swift 6 / macOS 15. Physical macOS 15, VoiceOver and forced termination during preference propagation were not tested. The normal Quit/relaunch persistence path was tested. Prior verified bundles were preserved.
