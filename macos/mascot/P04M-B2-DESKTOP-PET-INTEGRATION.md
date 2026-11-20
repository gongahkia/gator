# P04M-B2 — Approved passive desktop pet

## Outcome and approval

**Implemented and development-verified on the current Mac**, under the approved manual-hiding policy. After the B1 recommendation, the user replied “yeah ok approved.” This increment implements that recommendation: one passive pet below the reserved notch region, one selected display, default OFF, selected-conversation state, independent in-chat visibility, shared appearance/motion settings, and persistent Tuck Away.

The pet may remain visible in fullscreen and shared screens. The Settings explanation explicitly says so; automatic suppression and screen-sharing detection are not implemented. Interactive click/drag behavior from the B1 experiment is not included.

Development bundle: `/tmp/Gator-Native-P04M-B2.app`. The exact final binary was launched from a disposable bundle, showed its pet from a menu-bar-only launch, responded to Tuck Away, and exited through guarded Quit. Its SHA-256 is `a92b8c85a8a782b3d803efe7c51e5c2fa9f7fe9e945680adbfc4d497c4b97857`; see [manifest](evidence/p04m-b2/final-manifest.json).

The accepted P04/A2 apps, B1 lab, original assets and previous prototypes remain preserved. No P05 or other parity feature was started.

## Source and authority boundary

Initial HEAD: `8bd1bcbce6c98cb968314fb1f96fbccd6498f493`, clean worktree. HEAD advanced externally to `dd45abc7766decd0280cf44c5ce3d87310dfe268` during this work, incorporating some intermediate edits. This session made no commits and did not reset or revert external changes. See [scope check](evidence/p04m-b2/scope-check.json).

`NativeDesktopPet` receives two MainActor closures: a public `NativeMascotSignal` projection and an application-closing flag. It has no coordinator, connection, transport, submission, approval or provider capability. AppDelegate supplies weak captures of the existing shared coordinator. The signal uses the **same** `NativeMascotProjection` and `NativeMascotStateMachine` as the in-chat mascot, including selected owner/generation identity and exact public completion transitions.

The only persistent additions are native presentation preferences:

- `gator.desktopPet.shown`, default false.
- `gator.desktopPet.display`, default empty, meaning primary display.

Style and animation reuse `gator.mascot.style` and `gator.mascot.animated`; OS Reduce Motion wins. The desktop controller does not read the in-chat `shown` preference. All four combinations of in-chat/desktop visibility are supported and tested. Tuck Away writes the desktop enable flag false; activity, completion, wake and relaunch cannot enable it. No preference goes into a conversation or draft archive.

Go ownership, request admission, provider credentials, workspace scope, approval/plan/recovery identities, archive transactions, and shutdown transport are unchanged. No extra Go owner is created by this presentation. The disposable native session produced zero work-history files and zero request archives; no model credential was supplied. A loopback-only endpoint was configured as additional protection in that test profile. No paid provider request or external action was made.

## Native panel and lifecycle

The production panel uses the B1-validated configuration: transparent, borderless, nonactivating, shadowless, nonrestorable, ordinary floating level, and `.canJoinAllSpaces`, `.transient`, `.ignoresCycle`, `.fullScreenNone`. It cannot become key or main and always ignores mouse events. It has no click target, dragging, context menu, hover tracker or focus action. Existing Dock activation and notch behavior are unchanged.

One app-owned controller creates at most one desktop panel. Its narrow surface protocol separates AppKit from windowless deterministic tests. The panel embeds the existing `NativeMascotCanvas`, player and frame store, loading the same bundled PNG assets. There is no new animation engine, 3D renderer, database or service. Existing per-presentation frame-store limits and finite clips remain in force.

Observation is event-driven using `withObservationTracking`, MainActor callbacks and a cancellable/coalesced refresh task. The controller reads only the selected public signal. Preferences, screens and accessibility display changes also reconcile presentation. No periodic public-state poll or new high-frequency timer is added.

When disabled, sleeping, closing, without a usable display, or without a signal, the panel is detached/closed and the controller releases its presentation reference. The existing canvas stops animation and clears decoded frames. The observer bag removes registrations on teardown/deallocation; callbacks capture the controller weakly. `stop()` is idempotent and is called by `applicationWillTerminate`.

The guarded Quit transaction itself is unchanged. Reading its closing flags suppresses the pet; if closing is aborted and the original flags are restored, the enabled pet can reappear from the current public state. That does not replay a past completion or any task. Successful termination closes the overlay. A dedicated test checks close/abort/resume and preservation of the enable preference.

## Position and display policy

The 72 × 72 pt panel is centered in the selected screen's usable region, with a 12 pt margin. On notched screens it reserves the **existing** `NotchGeometry.panelFrame(expanded: true)` envelope; no second copy of the notch constants is introduced. The tested built-in geometry yields `(699,626,72,72)` in AppKit coordinates, matching approved B1 placement B.

Display choice uses the Core Graphics display UUID. Missing chosen displays fall back to the primary display without overwriting the saved UUID, allowing the choice to recover when that screen returns. No absolute coordinates are stored. If the panel cannot safely fit, it remains hidden. Settings show an explicit unavailable-display/fallback option when appropriate.

Negative origins, a secondary display, missing/no displays, tiny geometry, notch clearance and preserved chosen identity are covered headlessly. Physical monitor unplug/replug, Spaces/Mission Control and actual sleep/wake remain **NOT TESTED**. Sleep/wake notification handling is tested with process-local notifications; that is not a physical sleep trial. There is one pet per running Gator instance, not a cross-process singleton across independently launched apps.

## Controls and actual desktop checks

Settings → Mascot Appearance now distinguishes **Show In-Chat Mascot** from **Show Desktop Pet**, provides display selection, and includes Tuck Away. Both the full-window Conversation menu and compact chat's existing menu expose Show/Tuck Away through the same reusable controls. Shared style and Animate controls remain where they were.

The desktop checks used a disposable copy of the packaged app with only its bundle identifier changed to `dev.gator.pet.b2.fixture`, isolated native/state/config/workspace directories, no credentials and no paid endpoint. Existing B1 allowlisted tools were compiled into `/tmp` with the B2 fixture identifier/title substitutions; original B1 source/evidence was not modified. No system settings or real profile preferences were changed.

Observed:

| Check | Result |
|---|---|
| Fresh native desktop preference | OFF; no pet window initially |
| Enable from Settings | Transparent rendered pet appears, 72 × 72 pt native bounds |
| In-chat OFF, desktop ON | Desktop remains visible independently |
| Switch to Classic | Flat Classic artwork appears; shared picker remains usable |
| Disable animation | Static appearance remains |
| Passive click | Center click reaches underlying inert peer button; peer click count becomes 1 |
| Focus | Peer remains frontmost with its text field focused; composer focus observed before peer test |
| Tuck Away from menu | Native pet window disappears |
| Quit/relaunch | Tucked preference remains false; no pet on relaunch |
| Close chat while pet enabled | After window-close transition settles, only notch/pet windows remain |
| Tuck while chat closed | Works through ordinary menu controls |
| Quit | Disposable app terminates; no pet/peer fixture remains |
| Exact final binary, menu-only startup | Pet appears without a chat window; Tuck and Quit succeed |

Actual evidence:

- [Rendered settings and pet](evidence/p04m-b2/settings-and-pet.png).
- [Classic settings and independent desktop visibility](evidence/p04m-b2/classic-settings.png).
- [Pet after chat closure settles](evidence/p04m-b2/chat-closed-settled.png).
- [Exact final bundle background/menu-only pet](evidence/p04m-b2/final-background-pet.png).
- [14.41-second native recording](evidence/p04m-b2/idle-motion.mp4), 423 frames, no audio, plus [blink contact sheet](evidence/p04m-b2/blink-contact-sheet.png). Eyelid change is visible; this is actual finite Core Animation playback.
- [Native results](evidence/p04m-b2/native-results.json), [final binary smoke](evidence/p04m-b2/final-smoke.json), and frame/AX evidence in the same directory.

The decorative panel is deliberately absent from the AX window list. An initial AX `wait-window` therefore returned false despite an onscreen pet; native window-server bounds and captures confirmed it. Early close captures also retained the closing-window fade; the settled capture is the evidence for chat closure. Neither observation was treated as an app defect.

Screenshots/video include only explicitly allowlisted disposable windows over a neutral background. No real chat, provider secret, clipboard data or unrelated application window was captured. No text was sent during these checks. Physical VoiceOver/IME, dark-mode B2 acceptance, compact popover traversal, physical display migration and Go-backed responding motion were **NOT TESTED in B2**. Existing A2/B1 evidence for shared animation components is preserved rather than relabeled as new B2 desktop acceptance.

## Resource evidence and limitations

The matched integrated-app static check used the same relaunched process/profile, in-chat hidden, settings closed and animation disabled. `vmmap -summary` reported physical footprint **37.9M tucked → 38.9M static visible**, approximately +1M in the tool's rounded units. The earlier settings-open/rendered capture was 47.4M total and is not a comparable incremental baseline. See `vmmap-matched-hidden.txt`, `vmmap-matched-static.txt` and `vmmap-visible.txt`.

These are short, rounded **physical-footprint** observations, not precise RSS, sustained-active, GPU or energy measurements. B2 active CPU, GPU/compositor energy and an integrated endurance trial are **NOT TESTED**. Do not substitute B1 CPU figures as new integrated measurements.

The unchanged B1 player/panel experiment previously measured +3.47 MiB peak footprint, 0.21% idle CPU, 1.20% rendered activity and no tucked playback/cache; those are historical isolated results. B2's preliminary static observation is consistent with the approved low-cost direction, while full integrated activity profiling remains a documented limit. No resource budget was raised.

New deterministic tests exercise 100 show/tuck cycles with weak probes and no surviving fake surface after each release. Real B2 native opening/hiding/Quit was observed, but that test is not a substitute for a long resource trial. The original player tests continue to verify bounded cache, finite clips, hidden cancellation, Reduce Motion and missing-asset behavior.

## Verification

All commands ran headlessly except the explicitly described native checks. Build target remains macOS 15; host is arm64 macOS 26.5.2. macOS 15 physical runtime acceptance remains **NOT TESTED**.

| Command/check | Result |
|---|---|
| `swift build --package-path macos --scratch-path /tmp/gator-native-build` | PASS |
| Initial `swift test … --no-parallel --filter desktopPet` | 10 focused tests PASS |
| Final `GATOR_TEST_BINARY=/tmp/Gator-Native-Parity-P04.app/Contents/Resources/gator-runtime swift test --package-path macos --scratch-path /tmp/gator-native-build --no-parallel` | **255 tests PASS**, 139.065 s: 243 baseline + 12 new desktop tests |
| `swift test --package-path macos --scratch-path /tmp/gator-native-build --no-parallel -Xswiftc -strict-concurrency=complete -Xswiftc -warnings-as-errors --filter 'desktopPet\|mascot'` | **28 tests PASS**; all package/test sources compile strictly |
| `go test ./cmd/gator ./internal/workapp ./internal/worksession ./internal/workspace ./internal/workactivity ./internal/modelcatalog ./internal/auth ./internal/config` | **Eight packages PASS**, cached |
| Node 26: `--test app-host/tests/backend.test.mjs app-host/tests/native.test.mjs app-host/tests/sessions.test.mjs app-host/tests/drafts.test.mjs app-host/tests/reading-state.test.mjs` | **41 tests PASS** |
| `GATOR_NATIVE_APP_PATH=/tmp/Gator-Native-P04M-B2-Final.app bash macos/scripts/build-dev-app.sh` | PASS; final stage promoted to requested B2 path |
| `xcrun vtool -show-build …/Contents/MacOS/GatorMac` | minimum macOS 15.0 |
| `git diff --check` | PASS |

The first full pass had 253 tests before two more lifecycle tests were added; the final full pass includes all 255. The final strict check recompiles after the only subsequent production edit, removal of an extra trailing blank line. The final bundle was rebuilt and its exact executable smoke-tested. The earlier GUI-checked bundle is preserved at `/tmp/Gator-Native-P04M-B2-GUI-Checked.app`.

The initial draft build failed on unsupported synthesized equality for `NSEdgeInsets` and an incorrectly unannotated actor-bound closure type. Equality was unnecessary and removed; closure isolation was made explicit. Final build/strict tests pass without weakened checks. A preliminary host run covered 32 tests; the final 41-test run includes the existing draft and reading-state regressions.

Logs: [full final Swift](evidence/p04m-b2/swift-full-final.log), [strict final Swift](evidence/p04m-b2/swift-strict-final.log), [Go](evidence/p04m-b2/go-regression.log), [host](evidence/p04m-b2/host-final.log), [final package](evidence/p04m-b2/package-final.log). Safety tests remain in the full pass: cross-writer archive durability/conflicts, late-Quit protection, inherited-pipe shutdown, owner isolation and P01–P04 regression boundaries.

## Changed files and scope closure

Production changes:

- `NativeDesktopPet.swift`: read-only presentation controller, safe placement/display model and passive panel.
- `AppDelegate.swift`: owns the optional presentation and stops it on final termination; guarded Quit body unchanged.
- `GatorApp.swift`: initializes that one presentation with public selected-owner projection and adds menu controls.
- `ConversationView.swift`: one shared menu-control insertion; transcript/composer/review layout unchanged.
- `NativeMascotPreferences.swift`: independent desktop setting, display picker, Tuck Away and manual-hiding explanation; clearer in-chat label.

Tests/documentation:

- `NativeDesktopPetTests.swift`: 12 tests covering safe geometry, default OFF, independent persisted preferences, selected-owner identity/state, one-surface reuse, style/static/Reduce Motion, persistent Tuck, closing/abort, display fallback, observation, 100 release cycles and sleep/wake policy.
- This report and `evidence/p04m-b2/` logs, captures, measurement summaries and manifests.

Existing player/model code, original assets, Go, host contracts, Package.swift, icon sources, README files and production storage/security implementations were not changed. The new bundle's menu-bar/head/icon artwork is byte-identical to the accepted A2 bundle; all 119 mascot resource files match the original asset hashes. The 61 historical bundles checked against B1's inventory are unchanged.

No new reproducible P0/P1 regression was found in this verification. Remaining limitations are the explicit manual fullscreen/sharing policy, untested physical OS/display/accessibility variants and incomplete integrated activity profiling described above. B2 stops here; no later feature is authorized by this implementation.
