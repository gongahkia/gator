# P03 desktop acceptance closure — AS05

Date: 2026-11-18. Scope: acceptance of existing workspace selection only. No P04, additional parity feature, README edit, commit, dependency installation, signing or notarization.

## Baseline and exact applications

Starting HEAD: `07675f2c5dac1fc336446116a0cecc5cf38efb94`. Existing P02/P03 implementation, reports and local changes were preserved. [Acceptance baseline](evidence/p03-desktop/baseline.json) records the worktree status and hashes before this closure.

Original candidate: `/tmp/Gator-Native-Parity-P03.app`. It was exercised through `/tmp/Gator-Native-Parity-P03-Acceptance-Desktop.app`, with byte-identical Swift and Go executables and fixture-only bundle ID `dev.gator.native.p03.acceptance`. The clone isolates native preferences; it is not a different implementation. The candidate and prior bundles were preserved.

A reproducible Unicode handoff defect was found in that candidate. The focused correction requires a new candidate at `/tmp/Gator-Native-Parity-P03-Accepted.app`. Final executable hashes and desktop results are recorded below after verification.

## Original candidate observations and reproduced defect

The original candidate opened the standard directory panel from the Conversation menu. Selecting temporary Workspace A through the native browser returned to a new empty chat, showed the canonical Go source, exposed the composer and retained the original process/draft.

- Profile: `/tmp/gator-native-test-p03-acceptance-hewe1mpn`.
- Original conversation: `work-20261118T060750-01519f44ccceba61`, owner PID 11403.
- Workspace A: `work-20261118T061508-dd2ad5e43041d017`, owner PID 15937.
- Confirmed A source: `/private/tmp/gator-native-test-p03-acceptance-hewe1mpn/Workspace A`.
- The native archive retained both the exact original draft and “Workspace A draft stays with A”.
- A's Go conversation had an empty snapshot ID and no requests, dialogue, revisions or admitted work.

Evidence: [selected native panel](evidence/p03-desktop/picker-a-selected-original.png), [A UI/draft](evidence/p03-desktop/workspace-a-original.png), [before](evidence/p03-desktop/before-a.json), [after](evidence/p03-desktop/after-a.json).

Selecting B, whose parent contains spaces, café and Japanese text, actually returned from the panel, but failed the destination source check. The UI showed “The workspace transition does not match the original source”; A and its draft remained selected. Go had created B's empty durable conversation, then native cleanup stopped only that failed idle candidate. [Error screenshot](evidence/p03-desktop/unicode-error-original.png), [metadata and identity observations](evidence/p03-desktop/original-candidate-observations.json).

This was not classified as a focus failure: an actual Go/native result reached the application and produced a deterministic source-check error. A new real-Go Swift regression reproduced it without any GUI; a Go test separately demonstrated the inconsistent `New:true` result for two Unicode spellings naming the same directory. Both failed before the correction. [Swift before](evidence/p03-desktop/swift-unicode-before.log), [Go before](evidence/p03-desktop/go-unicode-before.log).

## Focused correction

The native source endpoint previously compared Go source strings byte-for-byte. On macOS, a panel path and the destination environment/filesystem projection can contain composed versus decomposed Unicode. Swift string equality treats those spellings as equal, causing an inconsistent `new` check; even without the client check, a same-folder choice could request another owner.

Two focused changes:

1. `internal/workapp/bridge_workspace.go`: after canonical resolution and the existing directory-access probe, compare the selected directory and retained source with `os.SameFile` when spellings differ. Only when Go confirms they name the same directory does it return `new:false` and retain the exact existing source spelling. Swift does not approximate filesystem identity, and different directories are not merged merely because their strings look equivalent.
2. `macos/Sources/GatorCore/NativeWorkspaceSelection.swift`: use UTF-8 byte equality for the wire consistency check, matching Go's retained source spelling rather than Swift's Unicode-normalizing equality.

No terminal command parser, execution authority, run admission, permissions, approval tokens, archive transactions, source snapshot/instruction policy, owner limit or process lifecycle changed. A new Swift real-Go regression checks both Unicode forms through new-owner handoff and repeated same-workspace selection. A new Go regression checks exact retained spelling on a normalization-insensitive filesystem. Existing assertions remain intact.

## Controlled desktop method and focus diagnosis

Fixtures use temporary directories, separate preference domains, app-owned native data, and only an unreachable loopback provider (`127.0.0.1:9`). No production credentials, production workspace, desktop message submission or external action is involved. Existing headless managed-work fixtures use inert local providers.

The acceptance controller only accepts the explicit disposable bundle IDs. It resolves fresh AX elements for each operation, checks the target focused window before keyboard input, uses control/window readiness conditions, and refuses input if the target is not frontmost. No system-wide accessibility, focus, input or privacy setting was changed. Read-only permission preflight reported Accessibility, Screen Capture and Post Events access already available.

Observed causes/limits:

- The original full window initially appeared at negative display coordinates `(-164,-895)`. Fixed-coordinate clicks from an earlier layout could target another location. The test moved only the fixture window to a known visible position; no production window-layout change was inferred or made.
- Foreground process snapshots identified unrelated applications, including a Java UI probe, taking focus. That is positive evidence of shared-desktop interference.
- The native file panel uses AppKit's remote view service. A reported `AXOpen` action on its filename text field returned unsupported; setting the browser list's selected children successfully selected and opened the intended directory. A stale or unsupported AX action is not an application bug.
- One-shot synthetic keyboard commands initially returned without an observed native transition. They are not counted as passes. Later guarded checks keep the controller alive until an observable control/window/focus transition or deadline.

Reproduction helpers: [setup](evidence/p03-desktop/setup.py), [launch](evidence/p03-desktop/launch.py), [guarded controller](evidence/p03-desktop/control.swift). These are acceptance-only; they are not application features.

## Final candidate results

Final regression and remaining desktop observations are recorded here after the corrected application is packaged and exercised.
