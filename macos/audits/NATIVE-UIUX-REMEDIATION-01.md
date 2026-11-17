# Native macOS UI/UX remediation 01

Date: 2026-11-17. Scope: the nine findings in `NATIVE-UIUX-AUDIT.md`, with the engineering audit and safety remediation treated as constraints. No terminal-parity implementation.

## Outcome

**Overall UI/UX assessment: 3.5/5, improved from 3/5.** This is an engineering/design judgment supported by current-source tests and fresh native captures. Warning readability, conversation discoverability, inspection context, Markdown hierarchy, reading continuity and composer feedback are materially better. Dense accumulated recovery evidence and the existing duplicate-filename contract still limit the experience. Physical accessibility acceptance remains outstanding.

Seven original findings are addressed; two have targeted improvements with larger work explicitly deferred. No new P0/P1 defect was reproduced. This does not establish the absence of defects in untested OS/preference/accessibility configurations.

Development candidate: **`/tmp/Gator-Native-UIUX-Verified.app`**. The existing `/tmp/Gator-Native-Safety-Verified.app` was preserved. The final executable was launched in a separate-preference-domain copy with a disposable profile; executable hashes match the candidate. No signing, notarization, distribution or paid provider request was performed.

## Baseline, current worktree and preservation

- Starting HEAD: `0b2cae1df6301d8354a4751ded8fc0d6b29afab5`, with a clean worktree.
- HEAD advanced externally to `677b955f5b75b4cea37244f0bfc700bc35b04829`, then `2a8b22fcbe3efd7f23fe51e551d855052d005465`, and later `a7c910fb11c657c1849161de9199cfb0c5b2bbff`, capturing in-progress UI edits/evidence. **This agent created no commits and did not reset/rewrite those external commits.** Changes and integrity are compared to the starting HEAD, rather than relying only on the final uncommitted diff.
- Read all nine UI/UX findings and the engineering/safety reports; inspected current native views, projections, tests, packaging scripts, older screenshot provenance and relevant Go conversation/upload contracts.
- Verified the archive sidecar lock and conflict/baseline checks, synchronous all-owner closing boundary/final flush, and termination-handler/cancelable pipe cleanup in the current implementation.
- Byte comparisons to the starting HEAD confirm that `NativeDraftStore.swift`, `GatorApplicationState.swift`, `ManagedSessionCoordinator.swift`, `GoBridgeConnection.swift`, `BridgePipeIO.swift`, and `AppDelegate.swift` are unchanged. Their safety tests are unchanged and pass. See [integrity.json](evidence/uiux-remediation-01/integrity.json).
- No README, production Go/JS, package dependency, transport, runtime protocol, database, provider/credential configuration, execution authority or approval/recovery identity change. Only the existing Go source was rebuilt for tests/bundling. No unrelated paused work was resumed.
- Verification host: arm64 macOS 26.5.2, Apple Swift 6.3.3, Go 1.27.3, locally installed Node 26.7.0. Swift tools 6.1 / Swift 6 language mode. Package, plist and final Mach-O deployment minimum remain macOS 15.0; `vtool` reports SDK 26.5. macOS 15 runtime behavior was not physically tested.

## Original nine findings: reconciliation and disposition

No original UI/UX finding was already resolved at the starting worktree. The three separate engineering safety defects were already corrected.

| Finding | Original severity / problem | Initial classification and evidence | Result |
| --- | --- | --- | --- |
| UIUX-01 | P2: small orange warning text has poor light contrast | Fix now. Current source and fresh invalid-image selection reproduce the orange error. Same resolved color pair remains 2.31:1 in light appearance. | **Addressed.** Semantic label-colored status text, severity symbols/accessibility names and explicit error styling; measured 14.94:1 light / 12.23:1 dark on tested native surfaces. |
| UIUX-02 | P2: history is hidden; task inspection replaces the conversation | Fix now. Current More submenu and replacement inspector were reproduced on the baseline build. | **Addressed.** Labeled full-window Recent Conversations control and native Conversation-menu entry; full-window task details become a window-attached, dismissible sheet. Compact inspection reuses the existing view. |
| UIUX-03 | P2: nested Markdown lists flatten | Fix now. Current projection discards depth. | **Addressed.** List depth, original numeric labels, mixed hierarchy, continuation content and native hanging indentation. Parsing and rendering regressions pass. |
| UIUX-04 | P2 inference: returning to a chat loses reading position | Needs reproduction first. With two long seeded conversations, A at notes A-13/A-14 returned to latest A-23/A-24 after visiting B. | **Addressed after reproduction.** Per-conversation message anchor/follow intent, bounded to 32 contexts, shared between compact/full presentations. |
| UIUX-05 | P2: slash-prefixed drafts disable Send without inline explanation | Fix now. Current guard blocks both commands and literal leading paths; baseline screenshot shows the missing reason. | **Addressed.** Inline, accessible guidance next to the editor. Draft/Go admission behavior is preserved; no slash-command implementation or bypass. |
| UIUX-06 | P2: recovery starts with technical/owner/cache prose | Fix modest presentation changes now; defer a larger reorganization of accumulated evidence. Fresh baseline inspector confirms density. | **Partial.** Task summary/activity, preparation IDs and retention details move behind disclosures; outcome/next-step language and uncertain-effect warnings remain visible. Many old result cards can still push a current plan far down the scroller. |
| UIUX-07 | P2 inherited: equal document basenames are rejected | Fix feedback now; defer identity-contract changes. Current real-Go regression reproduces safe rejection with both drafts/copies retained. | **Partial.** Early duplicate-name explanation and existing independent removal. Go/provider support for admitting both names remains deferred; validation is unchanged. |
| UIUX-08 | P3: fixed editor height cramps long drafts | Needs reproduction first. Eight-line baseline draft shows only roughly four lines in the full editor despite available space. | **Addressed.** Actual NSTextView layout drives bounded growth: 56–100 points compact, 76–160 full. Final-build capture shows all eight short lines; longer content still scrolls. |
| UIUX-09 | P3 inference: disabled notch preference leaves an enabled no-op command | Needs reproduction first. On this notched display, disabling the fixture preference hides the panel while baseline Show Notch remains enabled. | **Addressed.** Command and show path share `canShow`, derived from the existing preference and detected geometry. Disabled/enabled states and re-enabled panel were observed. |

## Implementation and responsibility boundaries

### Readable warnings and errors

`NativeStatusMessage.swift` provides an unboxed native label with semantic `NSColor.labelColor` text. Warning, information and error have different symbols/accessibility names; error symbols use system red. Text and meaning do not depend on color alone. This avoids orange paragraphs and an indiscriminate conversion of every message to red.

Applied to correction/attachment feedback, stale task/plan inspection, actionable provider/unavailable-state explanations and recovery uncertainty. `NativeErrorDisclosure` keeps its complete selectable error and disclosure affordance, with primary semantic text and a distinct error symbol. Normal secondary information retains its secondary treatment.

The text foreground was measured, rather than presumed compliant because of a semantic name. This is not a claim that every existing secondary/disabled text color throughout the app has been certified.

### Navigation using the existing Go projection

`RecentConversationsMenu.swift` reads the existing `recent_conversations` array, preserves Go order, deduplicates identities for view stability, marks the selected chat, and adds status for cached activity when known. It calls the unchanged `sessions.openConversation` method. Selecting the already-current row does not start another owner. Native Conversation-menu and toolbar routes reuse the same item implementation.

Go's `bridgeRecentConversations` exposes at most **24 nonarchived conversations**, sorted by pin/update state, with bounded titles (`internal/workapp/bridge_conversations.go:28`). The UI is labeled **Recent Conversations**, explicitly notes that it is Go's recent projection, and does not fabricate complete history. Existing compact More navigation and task-activity routes remain available. No search, rename, branch or full history browser was added.

A labeled toolbar menu fits the established single-window design and the 540-point content minimum without consuming a permanent sidebar column. At narrow widths, the system may place secondary toolbar actions in overflow while keeping recent navigation visible. The existing activity disclosure continues to distinguish other working/attention-needing chats.

Keyboard access uses standard native menus/focusable controls and the application menu. An attempted toolbar-only Shift-Command-H shortcut did not reliably open the menu while the editor had focus, so that advertised shortcut was removed; no nonworking shortcut is shipped. Existing Command-I and Escape were observed opening/closing task inspection. Full keyboard-only menu traversal under all OS keyboard preferences remains an operator check.

### Contextual inspection

The full transcript/composer stay mounted behind a standard window-attached sheet. Its heading names the inspected conversation; Back to Conversation and Escape dismiss details. The existing `TaskInspectorView` is reused in the compact replacement presentation and the full sheet. Width/height follow the current content bounds, with a scrollable body and persistent return control.

No review authority moved into sheet state. Inspections still resolve their original owner/generation through the coordinator. Exact approval and plan/recovery controls retain their existing current-owner, token, revision/digest, scope and admission checks. Opening/closing details never authorizes, grants access, resumes or resends anything. Full and compact exact-action review still use literal structured review content.

### Hierarchical native Markdown

`NativeMessageContent` retains its existing paragraph/heading/code/list kinds and adds list-item metadata: depth, marker and content. The linear line parser tracks indentation across siblings/blank lines, preserves ordered ordinals/delimiters, handles tabs and indented continuation paragraphs, and keeps fenced code literal, including incomplete streaming fences.

`NativeMessageBody` renders separate marker/content columns with hanging wrapping and visible indentation. Foundation's existing inline Markdown API supplies bold/italic/code/link formatting; no HTML parser, WebView, remote image loading or new dependency was introduced. Structured results, user text, commands and exact review fields still bypass assistant Markdown rendering.

This remains a bounded native presentation subset, not a full CommonMark engine. Numeric markers are limited to nine digits; recorded depth is capped at 32 and visual indentation at eight levels to retain usable compact width. Deeper items keep separate rows/markers and accessible level hints. Tables, blockquotes, nested code-block layout inside list items and arbitrary HTML are outside this correction.

### Reading continuity

`NativeReadingPositions` is a presentation-only, main-actor cache with a 32-context retention bound. It holds a stable message ID and follow-latest intent, not a Go owner or durable archive. Context replacement gives the transcript its own view identity. Native scroll-position binding supplies anchors; returning to a context or active presentation restores the saved message when available.

Only user scrolling or explicit Jump to Latest changes saved intent. Background snapshot/activity updates follow the bottom only when the reader was following. Programmatic updates/restoration defer during an active scroll gesture. A missing anchor falls back to the first available message while preserving non-following intent; Jump to Latest remains explicit. No transcript projection was moved into scroll callbacks; the view uses the existing cached `application.transcript`.

Restoration is at message granularity, not exact pixel offset within a long message. The cache is intentionally ephemeral across application launches. These are deliberate limits, not durable history/draft changes.

### Composer and attachment feedback

Slash feedback appears only for a nonempty trimmed leading slash, preserves the draft, explains the boundary, and gives a literal-path example: start with a word such as “Read /path…”. Go's current `upload_submit` rejects every leading slash (`internal/workapp/bridge_uploads.go:49`); there is no existing literal-leading-slash escape in that contract. Sending behavior was therefore left unchanged.

A static meaningful accessibility announcement is associated with the inline reason; it does not announce every keystroke. Physical spoken behavior is not accepted by these checks.

The editor still owns native selection, undo, IME and focus. A small NSTextView layout-height callback lets SwiftUI grow its viewport within caps, with redundant height reports suppressed. Owner/context identity and closing-time editing guards are unchanged. Private attachment staging/import/admission code was not modified. Duplicate-name guidance is advisory; Go remains the admission authority and source files are not renamed.

## Source-level changes

| Files | Change |
| --- | --- |
| `GatorCore/NativeMessageContent.swift` | Hierarchical list metadata/parser; no application state/protocol change. |
| `GatorMac/NativeMessageBody.swift` | Native list rows, indentation/wrapping, reusable inline formatting for tests. |
| `GatorMac/NativeStatusMessage.swift` (new), `NativeErrorDisclosure.swift` | Semantic readable text and distinct status/error presentation. |
| `GatorMac/ComposerView.swift`, `NativeComposer.swift`, `NativeComposerPresentation.swift` (new), `AttachmentSelectionView.swift` | Inline slash/duplicate feedback and measured bounded editor growth. |
| `GatorMac/RecentConversationsMenu.swift` (new), `GatorApp.swift` | Discoverable recent navigation, shared native menu items and app-scoped reading cache injection. |
| `GatorMac/ConversationView.swift`, `NativeReadingPositions.swift` (new) | Contextual full-window inspection and bounded reading/follow presentation state. |
| `GatorMac/TaskInspectorView.swift`, `RecoveryInspectorContent.swift`, `RecoveryPreparationView.swift` | Visible conversation identity, clearer recovery hierarchy and readable uncertainty/unavailability. |
| `GatorMac/NotchQuickAccess.swift` | One observable preference/geometry availability predicate. |
| `Tests/GatorCoreTests/NativeMessageContentTests.swift` | Five additional parser regressions; original test/assertions retained. |
| `Tests/GatorMacTests/NativeUIUXRemediationTests.swift` (new) | Nine tests for colors, feedback, recent identity/order, reading cache, native inline/list rendering and editor measurement. |
| `audits/evidence/uiux-remediation-01/` | Disposable fixture/controller sources, measurements, fresh captures, hashes and filtered reading observations. |

Paths in this table are relative to `macos/Sources/` unless prefixed `Tests/` or `audits/`. No production changes occur outside the listed Swift presentation/model files.

## Contrast evidence

The probe resolves actual AppKit/SwiftUI colors in process-scoped appearances, converts to sRGB, composites alpha over the resolved native background, and computes WCAG relative-luminance contrast. The tested backgrounds are the window, text and control backgrounds used by the remediated composer/inspector. On this host these resolve to the same opaque base within each appearance.

| Appearance | Background sRGB | Old orange text | New label text | Normal-text AA target |
| --- | --- | --- | --- | --- |
| Aqua | `[1, 1, 1]` | **2.3096:1** | **14.9384:1** | 4.5:1 — new text passes |
| Dark Aqua | `[0.117647, 0.117647, 0.117647]` | **7.4673:1** | **12.2258:1** | 4.5:1 — new text passes |
| Named Accessibility Aqua | Same resolved pair on this host | 2.3096:1 | 14.9384:1 | New text passes |
| Named Accessibility Dark Aqua | Same resolved pair on this host | 7.4673:1 | 12.2258:1 | New text passes |

The same invalid-image error was triggered through the native picker in both appearances and captured. Named accessibility appearances are **not** physical Increase Contrast preference testing. Symbol accent colors are redundant severity cues; the contrast claim concerns the text. Probe source and all 24 before/after surface measurements: [contrast-probe.swift](evidence/uiux-remediation-01/contrast-probe.swift), [contrast-measurements.json](evidence/uiux-remediation-01/contrast-measurements.json).

## Fresh visual evidence and provenance

All images below are native desktop captures from this remediation, using disposable state and a loopback-only provider. No historical enhanced-final screenshots are passed off as current evidence. Images are unmodified window captures, including native shadow/transparency margins; pixel dimensions are not the logical window size.

The baseline was built from starting HEAD with the safety corrections already present. Preview generations are explicitly distinguished from the final executable in [screenshots.json](evidence/uiux-remediation-01/screenshots.json). Generation 1 established reading restoration and contextual inspection; generation 2 includes the finalized visible controls. The final three gesture guards change no drawing/sizing code. Two additional captures use the final packaged executable in a byte-identical, separate-preference-domain clone. The candidate's original metadata was not patched.

| Workflow | Before | After |
| --- | --- | --- |
| Warning contrast | [Orange invalid-image text](evidence/uiux-remediation-01/before-warning.png) | [Light](evidence/uiux-remediation-01/after-warning-light.png), [dark](evidence/uiux-remediation-01/after-warning-dark.png) |
| Recent navigation | [More submenu](evidence/uiux-remediation-01/before-navigation.png) | [Visible recent control at minimum width](evidence/uiux-remediation-01/after-small-slash.png); native Conversation-menu route also present |
| Task context/recovery | [Replacement inspector](evidence/uiux-remediation-01/before-inspector.png) | [Contextual light sheet](evidence/uiux-remediation-01/after-inspector-light.png), [dark sheet](evidence/uiux-remediation-01/after-inspector-dark.png), [final minimum-size sheet](evidence/uiux-remediation-01/final-small-inspector-light.png) |
| Reading A → B → A | [Older A](evidence/uiux-remediation-01/before-reading-A.png), [returning A at latest](evidence/uiux-remediation-01/before-return-A.png) | [Return to A's earlier reading area](evidence/uiux-remediation-01/after-return-A.png) |
| Nested mixed Markdown | Original parser audit reproduction plus unchanged baseline parser test | [Light](evidence/uiux-remediation-01/after-markdown-light.png), [dark with nested 1 / bullet / 4 / bullet hierarchy](evidence/uiux-remediation-01/after-warning-dark.png) |
| Slash feedback | [No inline reason](evidence/uiux-remediation-01/before-slash.png) | [Small window](evidence/uiux-remediation-01/after-small-slash.png), [dark](evidence/uiux-remediation-01/after-warning-dark.png) |
| Eight-line draft | [Fixed viewport clips later lines](evidence/uiux-remediation-01/before-long-draft.png) | [All eight short lines visible in final build](evidence/uiux-remediation-01/final-eight-line-editor-light.png) |
| Background completion | Existing held fixture / no historical screenshot used as new evidence | [A stays in older content](evidence/uiux-remediation-01/after-background-reading.png), [compact older reading](evidence/uiux-remediation-01/after-compact-reading-light.png) |
| Exact decisions / plans | Existing exact authority preserved | [Full exact review](evidence/uiux-remediation-01/after-exact-review-dark.png), [compact exact review](evidence/uiux-remediation-01/after-compact-exact-review-dark.png), [scrollable plan review](evidence/uiux-remediation-01/after-plan-review-dark.png) |
| Notch | Baseline command remained enabled after disabling | [Re-enabled lightweight glance](evidence/uiux-remediation-01/after-notch-dark.png); disabled/enabled AX state recorded during checks |

### Reading-position observations

1. Seeded two distinct real-Go conversations with 24 numbered retained notes each. No paid provider was used.
2. Baseline: A was scrolled to notes A-13/A-14; after B and return, the accessible/rendered viewport was at A-23/A-24. This converts UIUX-04 from inference to an observed interaction defect.
3. Updated build: A at A-18/A-19 returned to the same reading area after B. Heading positions shifted by about nine points as layout settled; it did not jump to latest. This is message-anchor restoration, not a pixel-exact promise.
4. Started a controlled held task in B, returned to older A content, and released B through its private fixture gate. A's reported heading positions were unchanged before/after completion. See [before](evidence/uiux-remediation-01/background-before.txt) and [after](evidence/uiux-remediation-01/background-after.txt).
5. Opening compact chat retained older content and the Jump to Latest affordance. Different widths reflow the content, so cross-surface pixel positions are not compared. See [filtered observations](evidence/uiux-remediation-01/compact-reading.txt).
6. Explicit Jump to Latest remains the follow-intent transition; its state behavior and background append semantics have deterministic tests. A separate final physical click-to-latest journey under every presentation/keyboard preference was not repeated.
7. Normal latest-follow behavior was observed for the controlled visual reply. Model tests cover absent anchors, empty available history and bounded cache eviction. Physical deletion/pruning of a live conversation during a scroll was not induced.

## Targeted desktop observations and limits

Desktop checks were batched into baseline, light/dark preview checks and a brief final-build smoke check. Early checks lost focus to other applications; the user explicitly provided a short uninterrupted desktop window. Injection guards avoided targeting an unrelated foreground application. Several controller actions reported missing elements/timeouts or did not complete keyboard traversal; these were not counted as application passes. A preview also exited normally during focus-contended picker automation; its cause was not established and no crash/repeatable application defect was reproduced. The dedicated checks and final smoke completed afterward.

Observed:

- Recent A/B selection, visible current identity and owner-bound draft continuity. Current-row action is a no-op; existing headless integration tests independently cover owner reuse and background work isolation.
- A full-window sheet leaves the transcript/composer behind it, names the inspected conversation, provides a clear return button, and scrolls through long recovery/plan content. Minimum native outer window measured 540×532 points (the source declares a 540×480 content minimum).
- Command-I opens inspection and Escape dismisses it. Native file-picker keyboard entry selected the owned invalid image. Full keyboard-only recent-menu traversal was not established under the focus contention; no unsupported shortcut is advertised.
- Light/dark rejected-image feedback is visible/readable with an error symbol; slash feedback appears by the editor without changing its text. Normal Markdown shows all four fixture hierarchy levels and code remains literal/selectable.
- Compact 440×620 chat/exact review retains explicit owner, scrollable details and separate decisions. Compact/full reading layouts both retained older content. Automated popover focus/expand actions were not consistently completed when external focus changed; a human focus/Full Keyboard Access pass remains useful.
- One inert `web_access` proposal for `https://audit.example.invalid` was inspected in full and compact form, then explicitly **declined**. No access was approved and no URL was fetched. The fixture's retained decline response is visible in the final capture.
- An existing plan-first request produced a revision-1 proposal; its complete native review was loaded and scrolled. **Approve & Start was not chosen.** No plan-editing feature was introduced.
- Notch off: command disabled and panel hidden. On: command enabled and the camera-gap panel expanded. No notch geometry/layout redesign or authorization shortcut was introduced.
- Final candidate clone launched with retained proposals/drafts, accepted the eight-line editing fixture, displayed the minimum-size sheet, and quit. Startup did not add a model request or replay execution. Both fixture provider processes and all launched fixture apps were stopped; disposable files were retained for review.

**NOT TESTED / operator limits:** physical VoiceOver speech/order, physical IME after viewport growth, Full Keyboard Access variants, actual OS Increase Contrast/Reduce Motion/larger-text preferences, macOS 15 runtime, external/flat/multiple displays, Spaces/full screen, exact within-message pixel restoration and sustained live UI profiling. Accessibility identifiers, named appearances and headless marked-text tests do not replace those checks. No application-wide accessibility certification is claimed.

## Headless verification

All routine builds/tests were headless. Desktop interaction was confined to the affected visual/interaction checks above. No new tool/dependency was installed.

### Targeted parsing, rendering and presentation tests

```sh
GATOR_TEST_BINARY=/tmp/gator-safety-runtime swift test --package-path macos \
  --scratch-path /tmp/gator-native-uiux-build --no-parallel \
  --filter 'uiux|nestedMarkdown|markdownBlank|markdownTabs|markdownFences|markdownOrdinals|nativeMarkdownPreserves'

GATOR_TEST_BINARY=/tmp/gator-safety-runtime swift test --package-path macos \
  --scratch-path /tmp/gator-native-uiux-build --no-parallel \
  -Xswiftc -strict-concurrency=complete -Xswiftc -warnings-as-errors \
  --filter 'uiux|nestedMarkdown|markdownBlank|markdownTabs|markdownFences|markdownOrdinals|nativeMarkdownPreserves'
```

Both completed with **15 selected tests passing**. The first attempt to compile the new test file needed `@testable import GatorCore` for fixture construction and an explicit `CGFloat.greatestFiniteMagnitude`; these test compilation errors were corrected. No failing assertion was removed or weakened.

Coverage includes unordered/ordered/mixed nesting, multiple depths and tabs, blank/continuation paragraphs, headings/adjacent paragraphs, original ordinals, inline bold/italic/code, code fences with list-like text, incomplete fences, native compact/full wrapping, sRGB contrast, conditional slash guidance, duplicate-name display, recent identity/order, separate reading anchors, missing/empty anchors, cache bounds and measured editor growth.

### Required complete build/regression and strict compiler

```sh
go build -o /tmp/gator-uiux-runtime ./cmd/gator
swift build --package-path macos --scratch-path /tmp/gator-native-build
GATOR_TEST_BINARY=/tmp/gator-uiux-runtime swift test --package-path macos \
  --scratch-path /tmp/gator-native-build --no-parallel
swift build --package-path macos --scratch-path /tmp/gator-native-uiux-build \
  -Xswiftc -strict-concurrency=complete -Xswiftc -warnings-as-errors
```

**Final build passed. Final complete regression: 170 passed / zero failed, 51.383 seconds. Strict Swift 6 build passed with warnings-as-errors.** The new total is the original **156 plus 14 additional tests** (five Core parser, nine Mac presentation tests); parameterized cases do not imply an arbitrary change in suite accounting. All 23 safety remediation tests remain present and passing, along with existing real-Go owner/approval/plan/recovery integrations.

The first full run passed 170 in 50.030 seconds. A final review added guards deferring programmatic scroll changes during an active gesture; the complete build/test was repeated after that production change. Final logs: `/tmp/gator-uiux-final-build.log`, `/tmp/gator-uiux-final-regression.log`, `/tmp/gator-uiux-final-strict.log`. Strict targeted test compilation is in `/tmp/gator-uiux-strict-targeted.log`.

The existing streaming benchmark also passed: 30 snapshots / 500 retained messages / 2,176,935 wire bytes, median 6.725 ms per snapshot. This is a bounded state-processing measurement, not a GUI frame-rate improvement claim.

### Relevant unchanged contracts and measurements

```sh
go test ./cmd/gator ./internal/workapp ./internal/attachment ./internal/worksession \
  -run 'Test(UIBridge|BridgeUploads|DesktopApprovalCannot|ExecutionOwnerIsExclusive|DialogueAdmission|.*TaskPlan.*Reject|.*RequestRecovery.*|.*Upload.*)' -count=1
/Users/gongahkia/.local/share/fnm/node-versions/v26.7.0/installation/bin/node --test \
  app-host/tests/backend.test.mjs app-host/tests/sessions.test.mjs \
  app-host/tests/drafts.test.mjs app-host/tests/uploads.test.mjs app-host/tests/policy.test.mjs
swift macos/audits/evidence/uiux-remediation-01/contrast-probe.swift \
  > macos/audits/evidence/uiux-remediation-01/contrast-measurements.json
git diff --check
```

All four Go packages passed. Host: **63 passed, zero failed/skipped/cancelled**. Contrast probe passed; all new status-text surface pairs exceed 4.5:1. Whitespace validation passed. The full backend suite was not run because production backend/host contracts were not changed.

### Packaging and actual final executable

```sh
GATOR_NATIVE_BUILD_DIR=/tmp/gator-native-build \
GATOR_NATIVE_APP_PATH=/tmp/Gator-Native-UIUX-Verified.app \
bash macos/scripts/build-dev-app.sh
xcrun vtool -show-build /tmp/Gator-Native-UIUX-Verified.app/Contents/MacOS/GatorMac
```

Packaging and plist validation passed. Candidate identifier remains `dev.gator.native`, executable/resources are present, and Mach-O minimum is 15.0. A copy at `/tmp/Gator-Native-UIUX-Desktop.app` uses a fixture-only preference domain; its Swift and Go executable bytes match the candidate. That final executable was actually launched/checked/quit. Hashes and final screenshots are included in the evidence manifest. The preserved safety bundle's Swift SHA256 remains `427a0bc7039de3ba7770ca4178cb11219e798bee58824fa355a5d29e4aa70a04`.

Fixture/controller sources under the evidence directory reproduce disposable setup, process-scoped appearance, native AX checks and contrast measurement. Capture used the installed screenshot helper with explicit window IDs. No entire desktop, personal file-picker listing or user credentials are included in the delivered evidence. The optional PIL metadata reader was unavailable; PNG dimensions were read directly from the PNG header without installing anything.

## Remaining issues and readiness

| Priority | Remaining item | Decision |
| --- | --- | --- |
| P0/P1 | No newly reproducible defect from these checks. Existing safety corrections are byte-preserved and covered by the complete suite. | No further safety change in this pass. Cooperating native writers must still all use the corrected safety implementation, as documented previously. |
| P2 / UIUX-06 | Many historical recovery cards can still bury the current plan/action below older evidence. The modest copy/disclosure changes improve the first screen but do not fully solve ordering/grouping. | Defer focused evidence prioritization; retain every uncertain receipt and all exact decision context. Do not hide unresolved effects just to shorten the screen. |
| P2 / UIUX-07 | Duplicate basenames remain a Go/provider/retained-original identity-contract limitation. | Early feedback is implemented; coherent identity support requires separately authorized backend/contract work. No validation bypass or silent renaming. |
| P2 / engineering E04, E05 | Restored uncertain receipt reconciliation and historical-failure owner retirement remain unchanged. | Carry forward before expanding recovery/owner-retirement features. |
| P3 | Original fixed-viewport and notch availability findings are addressed. Extremely long drafts intentionally retain an internal scroller; anchor restoration is message-based. | These stated bounds are intentional. Physical accessibility/focus/preference checks remain evidence gaps, not invented confirmed defects. |

The existing architecture is ready for **scoped native terminal-parity development after review of this candidate**, with the carried-forward P2 contract/recovery constraints kept explicit. UI quality is improved, rather than declared fully polished or accessibility-accepted. None of CH03, CH04, CH08, CH12, QT01, QT04, MD03, AS05, FL07, FI02, FI08 or PL02 was implemented. No new parity decisions or subsequent remediation pass were started.
