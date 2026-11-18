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

### Corrected candidate and final controlled session

**Final verdict: PASS for P03 desktop acceptance.**

Final application: **`/tmp/Gator-Native-Parity-P03-Accepted.app`**. Its Swift and Go executables are byte-identical to `/tmp/Gator-Native-Parity-P03-Accepted-Desktop.app`, used with the isolated preference domain `dev.gator.native.p03.acceptance.fixed`. The final Go resource is byte-identical to `/tmp/gator-p03-accepted-runtime`, which passed the complete regression suite. [Integrity manifest](evidence/p03-desktop/integrity.json).

Before the final session, unrelated desktop input changed an earlier corrected fixture's conversation, opened task inspection and admitted its unsent fixture text. Its loopback provider was unavailable, so no model inference or external operation occurred. That profile was **invalidated for final continuity/no-Send acceptance**; these observations were not treated as a proven Gator defect. The user then reserved an uninterrupted five-minute desktop window. A fresh profile was created and the required interactions were repeated successfully. The earlier session's valid captures are explicitly labeled “original”; they are not the final acceptance evidence.

Final profile: `/tmp/gator-native-test-p03-acceptance-xat9_7tw`. Client PID 31583; retained direct Go PIDs **31584, 31649, 31767** throughout A/B navigation, same-source choice, compact cancellation and reading-position checks.

| Conversation | Exact retained ID | Go-confirmed source |
| --- | --- | --- |
| Original fixture | `work-20261118T064411-ce15bfc5c15ef886` | `/tmp/gator-native-test-p03-acceptance-xat9_7tw/workspace` — existing retained spelling preserved. |
| Workspace A | `work-20261118T064511-28d7569231500461` | `/private/tmp/gator-native-test-p03-acceptance-xat9_7tw/Workspace A` |
| Workspace B | `work-20261118T064549-c3fe983c2eae8b3b` | `/private/tmp/gator-native-test-p03-acceptance-xat9_7tw/A deliberately long parent folder for readable truncated workspace paths with spaces and café 日本/Workspace B` |

The stored B path uses decomposed Unicode; the picker can supply composed text. Go confirms filesystem identity and retains the existing spelling for a same-directory choice. [Final retained records, drafts and owner PIDs](evidence/p03-desktop/controlled-final.json).

### Desktop result matrix

| Required interaction | Result | Observed evidence |
| --- | --- | --- |
| Native menu/action opens standard directory picker | PASS | Conversation menu and toolbar invoked the existing NSOpenPanel. The browser selected actual temporary directories; no raw source command was sent. [Selection log](evidence/p03-desktop/selection-accepted.txt). |
| Select A successfully | PASS | Panel dismissed, new conversation/owner appeared, Go persisted A's canonical source and the composer was empty/available. Original draft/owner remained. |
| Select B successfully, including spaces/Unicode | PASS | Separate conversation and third owner; B source in UI agrees with Go metadata. A source/owner/draft remained unchanged. |
| Return A → B | PASS | Exact native catalog rows opened the original cached owners, with their original sources and independent drafts. No new owner/PID on return. [Controlled log](evidence/p03-desktop/controlled-final.txt). |
| Same current workspace | PASS | Return in the native picker confirmed A again. Go reported “Already using this workspace · conversation kept”; conversation, owner PIDs and exact draft remained unchanged. A prior keyboard Go to Folder check also confirmed B using both Unicode representations without another owner. [Keyboard log](evidence/p03-desktop/keyboard-same-workspace.txt), [same-owner receipt](evidence/p03-desktop/same-workspace.json). |
| Keyboard navigation/confirmation | PASS | With actual window/focused-element checks: Command-Shift-G opened the nested native Go to Folder sheet, Command-A selected its path, keyboard text entry changed it, Return navigated and Return confirmed the native picker. No shortcut system changed. |
| Escape/cancellation and focus | PASS | Full-window Escape dismissed the picker with unchanged archive/selection/draft. After native confirmation the composer had focus. In the final session Escape closed transcript Find and focused the composer without sending. [Cancellation observations](evidence/p03-desktop/return-minimum-cancel.txt), [final controlled observations](evidence/p03-desktop/controlled-final.txt). |
| Compact More → Choose Workspace → Cancel | PASS | The controller resolved the **actual compact More menu's** workspace item, observed the native picker, pressed its native Cancel button and observed dismissal. Reopened compact chat retained B's exact draft and source; archive and all three owner PIDs stayed unchanged. No dashboard or notch change. [Final log](evidence/p03-desktop/controlled-final.txt), [compact capture](evidence/p03-desktop/compact-final.png). |
| Minimum supported full window | PASS | Requesting a smaller frame clamped to **540×532 outer points** (540×480 content minimum). Workspace action and composer remained accessible. Long source visibly truncated in the middle, retaining the Workspace B suffix and read-only label. Full source remains exposed through the existing accessibility/help value. [Minimum capture](evidence/p03-desktop/minimum-final.png). |
| P01/reading position during same-source selection | PASS | Find retained “Retained note 0005-15”, count 1 of 1, the actual older message remained visible with its selected outline, Jump to Latest stayed explicit and the original draft remained. [Before](evidence/p03-desktop/reading-before-final.png), [after](evidence/p03-desktop/reading-after-final.png). |
| No accidental Send/admitted Work in the final fixture | PASS | A/B have no request journal, snapshot or revisions; original/A/B drafts retain their exact texts. The provider points only to unreachable loopback. No pending approval/queue/run authority was inherited by the new chats. |
| Background-work scope | PASS via real-Go regression, not a new GUI model request | The unchanged managed-work integration exercises workspace selection while original Work runs, checking source and parent/child draft isolation. It passed in the targeted and complete corrected suite. Desktop selection did not start Work. |
| Normal Quit | PASS | Final controlled client exited normally and all three direct Go owners were gone. [Cleanup record](evidence/p03-desktop/cleanup.txt). |

The first reading geometry probe matched the Find field because its value equaled the target heading text. That geometry assertion is **not** used as proof of transcript position. The actual message heading is visible in both delivered window captures; a separate headless Vision check identifies the unique exact heading in the transcript region at normalized top positions **0.3068 before / 0.3291 after**. The status row changes layout slightly; pixel-exact preservation is not claimed. [Image-based verification](evidence/p03-desktop/reading-captures.json). The helper's future selector is qualified as `AXHeading`; the actual saved captures remain the primary visual evidence.

### Regression after the focused fix

Code changes justified rerunning regression; no suites were rerun merely for a new count.

```sh
go test ./internal/workapp -run TestNativeWorkspace -count=1
go build -trimpath -o /tmp/gator-p03-accepted-runtime ./cmd/gator
GATOR_TEST_BINARY=/tmp/gator-p03-accepted-runtime swift test --package-path macos \
  --scratch-path /tmp/gator-native-build --no-parallel \
  --filter 'workspace|actualGoManagedDialogue'
swift build --package-path macos --scratch-path /tmp/gator-native-build
GATOR_TEST_BINARY=/tmp/gator-p03-accepted-runtime swift test --package-path macos \
  --scratch-path /tmp/gator-native-build --no-parallel
GATOR_TEST_BINARY=/tmp/gator-p03-accepted-runtime swift test --package-path macos \
  --scratch-path /tmp/gator-native-build --no-parallel \
  -Xswiftc -strict-concurrency=complete -Xswiftc -warnings-as-errors \
  --filter 'workspace|actualGoManagedDialogue'
go test ./cmd/gator ./internal/workapp ./internal/worksession ./internal/workspace
/Users/gongahkia/.local/share/fnm/node-versions/v26.7.0/installation/bin/node --test \
  app-host/tests/native.test.mjs app-host/tests/sessions.test.mjs \
  app-host/tests/drafts.test.mjs app-host/tests/reading-state.test.mjs \
  app-host/tests/backend.test.mjs
GATOR_NATIVE_APP_PATH=/tmp/Gator-Native-Parity-P03-Accepted.app \
  bash macos/scripts/build-dev-app.sh
python3 macos/parity/evidence/p03-desktop/verify-integrity.py
git diff --check
```

| Verification | Result |
| --- | --- |
| Targeted Swift workspace/managed-work | **17 PASS**, 17.671 seconds. [Log](evidence/p03-desktop/swift-targeted.log). |
| Complete sequential Swift | **215 PASS**, 132.673 seconds. Original 214 plus one new real-Go Unicode regression. [Log](evidence/p03-desktop/swift-full.log). |
| Strict Swift 6, warnings-as-errors | **17 selected PASS**, 13.863 seconds; production and test sources compiled. [Log](evidence/p03-desktop/swift-strict.log). |
| Go cmd/gator / workapp / worksession / workspace | **Four packages PASS**; 33.540 s / 8.999 s / cached / cached. [Log](evidence/p03-desktop/go-regression.log). |
| Host native/session/draft/reading/backend | **41 PASS**, zero failures/skips, 2.262 seconds. [Log](evidence/p03-desktop/host-regression.log). |
| Package/plist/minimum platform | PASS. macOS minimum **15.0**, SDK 26.5, no new dependencies. [Package log](evidence/p03-desktop/package.log). |
| Source/bundle/safety integrity and diff whitespace | PASS. Exactly two production and two test files changed relative to this closure's baseline; original P03 and protected prior safety/P01/P02 files preserved. [Manifest](evidence/p03-desktop/integrity.json). |

The first after-fix Swift attempt correctly exercised the handoff but its new test compared `/var` with `/private/var`; its expected path was corrected to the existing filesystem-canonical fixture helper, retaining the source assertion. Existing tests/assertions were not weakened. Initial capture-analysis attempts assumed OCR would recognize both the caret-bearing Find field and the message heading, and then encountered a CGFloat/Double conversion mistake; those helper failures are not application defects or counted regression passes. The final image probe checks the unique exact body heading in the actual transcript region.

Only these implementation/test files changed during closure:

- `internal/workapp/bridge_workspace.go`
- `internal/workapp/bridge_workspace_test.go`
- `macos/Sources/GatorCore/NativeWorkspaceSelection.swift`
- `macos/Tests/GatorCoreTests/NativeWorkspaceIntegrationTests.swift`

All other changes are the requested reports and acceptance evidence/helpers. No production UI, coordinator, transport, archive, Quit or approval/recovery implementation changed during this closure.

### Remaining limits and optional operator checklist

No new P0/P1 defect was reproduced in the clean controlled run or corrected regression suites. The reproduced P2 Unicode reliability defect is corrected and covered by actual-Go, Go unit and native desktop evidence. Existing recovery/owner-retirement P2 limits remain unchanged.

Not tested: physical VoiceOver speech/order, IME, Full Keyboard Access/input-layout variants, macOS 15 runtime, real removable/network-volume loss, TCC revocation, and every compact Escape/focus permutation under competing applications. The earlier compact Escape attempt had no focused input element and is not counted as a keyboard pass. Compact cancellation was conclusively verified through its native Cancel control in the reserved session. A long multiline error was observed on the original candidate before correction; corrected-candidate minimum-size error typography was not separately induced. No blanket accessibility or all-error-layout certification is claimed.

The uncontrolled earlier fixture remained alive after its Go owners had closed while a task inspector was present. Two native Quit attempts were made; its verified, isolated client PID alone was then terminated for cleanup. This did not reproduce in the clean final session, which quit normally. The uncontrolled observation is retained as an evidence limit, not presented as a new confirmed shutdown regression.

Optional human follow-up, always with a disposable profile and temporary folders:

1. Use keyboard-only native menu access under the operator's preferred Full Keyboard Access/input settings; choose A, then B, and inspect the full path via help/accessibility.
2. Repeat compact Escape and focus return while the menu-bar popover is the only visible presentation.
3. With VoiceOver/IME enabled, check the workspace path, read-only explanation, cancellation and composer focus without sending.
4. At minimum width, inspect a deliberately rejected temporary-folder error and verify its full help text remains available.

These optional preference/accessibility variants do not leave any of the four original acceptance gaps unresolved in the tested configuration. **P03 desktop acceptance is PASS for the corrected candidate.** P04 and other parity features were not started.
