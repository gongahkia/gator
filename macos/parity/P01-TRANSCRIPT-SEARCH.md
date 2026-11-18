# Gator native parity P01 — CH08 transcript search

Date: 2026-11-18. This is the first selected native parity increment, not migration Tranche 09. Scope: CH08 only, preceded by the explicitly requested menu-bar icon preference.

## Outcome and scope

**P01 is complete within the defined loaded-text scope.** Implemented local Find/reveal in the current conversation's **loaded public transcript**, with Command-F, focused input, occurrence counts, excerpts, results, previous/next selection, stable-message reveal, selected-row outline and Escape dismissal. No model, task admission, work queue or retained-history mutation is involved. Closing Find keeps the reading anchor and composer draft. Switching conversation or reconnecting closes and clears Find.

The menu-bar preference was completed and verified first: [menu-bar verification](../audits/MENUBAR-VISIBILITY-VERIFICATION.md). Hide/restore, persisted hidden startup, normal Quit/relaunch, draft retention and application reopen were checked. The preference is available in More and the native Conversation menu. Hiding opens the existing full window.

P01 is limited to the projection already supplied to Swift. It does not load older history or implement CH07. It does not search all retained history. Native menu-bar chat and notch quick access remain compact and do not expose Find.

## Baseline and preservation

- Initial menu-bar work started at HEAD `c748d94ad39589fcf069cef7fe31b979bfc92787`, with an existing modification to the previous UI/UX evidence manifest. External commits captured in-progress menu work and advanced HEAD to `7e5be43d04ae097670cb4581b70267d1a64e1413`. This agent made no commits, staged nothing and preserved those external changes. P01 began at the latter HEAD.
- Read the parity, UI/UX remediation and safety remediation reports; inspected current Core/Mac sources, reading/navigation, tests, terminal search/reveal, Go projection and development packaging script.
- Cross-process archive locks/conflict detection, closing transaction/late-edit persistence and bounded pipe shutdown exist in the current implementation. Their five Core source files and three safety regression files are byte-identical to the P01 baseline. [Integrity evidence](evidence/p01/integrity.json) records comparisons and bundle hashes.
- No production Go/JS, protocol, package dependency, database, coordinator, execution authority, approval/plan/recovery identity, attachment store or provider setting changed. AppDelegate's menu-stage change adds reopen presentation only; guarded Quit is unchanged. No README changes, signing/notarization or external actions.
- Host: arm64 macOS 26.5.2; Apple Swift 6.3.3; Swift tools 6.1 / Swift 6 language mode; Go 1.27.3. Package and final Mach-O minimum remain macOS 15.0, SDK 26.5. Physical macOS 15 was not tested.

## 1. Terminal reference behavior

Verified against [actions.tsx](../../ui/src/gator/actions.tsx), [activity.tsx](../../ui/src/gator/activity.tsx) and [transcript.tsx](../../ui/src/gator/transcript.tsx):

| Question | Current terminal behavior |
| --- | --- |
| Which messages? | `client.state.messages`, in original array order. The search action does not separately include timeline dialogue or chat-update cards. |
| Which fields? | `message.text`, plus `activityText(message.activity)` only when that message's activity is expanded. Public activity text includes summary/state/duration, public text/tool names and supported public detail fields. No arbitrary JSON traversal. |
| Matching? | Literal `toLocaleLowerCase().includes(...)`; one result per matching message. Empty input happens to match every message. |
| Results? | Original array ordinal, first 100 characters of message text, role and creation time. |
| Reveal? | Emit `reveal-message` with the exact original message index; switch to chat, ensure its virtualized timeline item is loaded, then align that message's start after layout. |
| Older history? | Search does not request another page or fetch global history. It searches the current loaded array. |

Native achieves that local text-finding/reveal workflow, with explicit differences: occurrence-level results, Unicode folding, empty-query guidance instead of all-message results, and eligible projected public dialogue. Collapsed raw activity/details are excluded even if a separate native disclosure was expanded. There is no claim of identical terminal field coverage.

## 2. Native searchable content and limitations

`BridgeSnapshot.transcript` combines received Work messages with public dialogue user text, responses, distinct peer summaries and structured notices. Search consumes the same cached `GatorApplicationState.transcript` used for rows, preserving its order and exact IDs. Work IDs remain `context:index`; dialogue IDs remain `context:dialogue:turnID:component`. Raw Go approval indexes are unchanged.

Go forwards the current Work `m.messages` in `internal/workapp/bridge.go`. Dialogue restore/retention in `internal/workapp/dialogue.go` is bounded to the newest 100 turns and exposes `dialogue_more`. Swift does not fetch earlier turns. The desktop fixture retains 105 A turns: A-006 onward is projected; A-001 remains stored but produces no Find result.

Included:

- Ordinary public You/user and Gator/assistant text, including projected dialogue and public peer summaries.
- Structured result/notice headings actually rendered by the native row, including the visible fallback “Result details”. Their unrendered `message.text` is excluded.

Excluded:

- Hidden/private/analysis/reasoning entries and non-dialogue roles.
- Credential/token fields, raw result/preview JSON, collapsed summaries/details, bundles, attachment paths/content and activity traces. Search never expands them.
- Transient unfinished stream text, inspector/review content, unprojected older history, other owners and global conversation catalogs.

The UI states: **“Loaded text and result headings only. Earlier history and collapsed details are excluded.”** Limits are explicit when reached: 10,000 projected messages, 8 MiB of eligible UTF-8 text, 500 occurrences and a query of up to 256 UTF-16 units. Oversized entries are skipped with the limit indication; nothing is silently advertised as complete. Long-message fixtures cover a match in the middle of a two-million-character message.

Matching is literal, case-, diacritic- and width-insensitive, using Foundation with a fixed locale. Canonical composed/decomposed Unicode is covered; match ranges expand to composed-character boundaries. Results are ordered by transcript position, then non-overlapping occurrence offset. Markdown is searched as retained source text, as in the terminal; a phrase interrupted by Markdown delimiters is not normalized into rendered prose.

## 3. Architecture and concurrency

[NativeTranscriptSearch.swift](../Sources/GatorCore/NativeTranscriptSearch.swift) is a deterministic Sendable value index, importing only Foundation and CryptoKit. It has no transport, owner or mutation dependency. Each match carries scope (context + owner generation), exact message ID, ordinal, UTF-16 range, excerpt and a fingerprint of eligible content. It exposes a current-projection reveal validator.

[NativeTranscriptFind.swift](../Sources/GatorMac/NativeTranscriptFind.swift) holds window-local, main-actor presentation state and the Find bar. It does not retain a Go owner. The index is discarded on close, rebuilt only when Find opens or the transcript changes, and reused for query changes. Unrelated snapshot/activity updates do not rebuild the index. Query/result work is synchronous and explicitly bounded; no detached task, new service or asynchronous result race was introduced.

The only new asynchronous UI operation is a SwiftUI-owned focus task keyed by an explicit Find request. It yields through insertion/native responder updates before focusing. Snapshot updates do not change that request, steal focus or issue a reveal.

## 4. UI, keyboard and accessibility

- Toolbar magnifying glass and native Edit-menu “Find in Current Transcript…” action; Command-F in the full conversation window.
- Find opens above the existing transcript. Count, source ordinal/role and a short excerpt identify the current occurrence. Results… opens a bounded native list with the current result marked.
- Typing selects/reveals the first occurrence. Return selects the next; Shift-Return selects the previous; buttons provide the same navigation. Navigation wraps. Clicking a result reveals that exact message.
- Escape while Find is active closes it, clears the ephemeral query/results, returns focus to the composer and keeps the current reading anchor. It does not resume following automatically. Jump to Latest explicitly resumes following.
- Switching owner or owner generation closes Find and rejects old selections. Find is unavailable during exact approval review or task inspection. It does not search or reinterpret exact-review content.
- The selected message has an outline; source text, Markdown headings/lists/code and text-selection modifiers are unchanged. Inline substring highlighting was deliberately omitted to avoid rewriting the Markdown renderer. The excerpt, count and selected-row indication provide orientation.
- System controls, explicit accessibility names, source labels and keyboard actions are used. No animation is added; the existing Reduce Motion transaction remains effective.

Keyboard-only query/navigation/reveal/dismissal was exercised. Full physical VoiceOver, IME, alternate input layouts, Switch Control and OS keyboard-navigation preference variants were **NOT TESTED**. No accessibility certification is claimed. Escape from the results popover may first dismiss that popover before closing Find.

## 5. Identity and reading-anchor safeguards

Selection is checked against the current index and exact match payload. At the final scroll boundary it is checked again against the live context, owner generation, message ID and eligible-content fingerprint. A stale result cannot pass validation for another owner or a changed message at a reused ID. Search text is never converted into approval, plan or recovery authority.

The scroll-position binding and scroll proxy receive the same message ID. Revealing marks the conversation as reading older content and updates the existing bounded reading cache, which still retains at most 32 contexts. Other conversations' positions are independent. Closing Find retains the selected reading position; missing anchors use the existing graceful fallback. Explicit Jump still updates both the binding and proxy to `latest`.

Desktop integration exposed two issues in the first implementation, corrected before acceptance:

1. Command-F could open Find while the native composer retained focus. Focus is now applied by the insertion-aware SwiftUI task. The final desktop checks verify actual AX focus before typing or pressing Return.
2. With 200 projected messages, cache entries were correct but lazy height estimation/viewport changes could override an early restore. The transcript now initializes with its saved position and reapplies a requested anchor when content/container geometry changes. A tracking/interacting/decelerating user gesture clears that restoration intent; reveals wait until the gesture ends. Closing Find, resizing and returning from another conversation were retested. No arbitrary persistence/shutdown sleep or Go ownership change was used.

The desktop regression records A-050 and B-012 returning visibly to their respective viewports, independent drafts preserved, Find closed on owner switch, and the same two direct Go PIDs reused. Manual scrolling moved from normalized offset 0.440 to 0.472 without being pulled back; Jump reached 1.0. Final candidate keyboard verification also reached 1.0 after Escape.

## 6. Headless verification

Baseline: 170 tests from UI/UX remediation, plus one menu-bar reopen regression = 171. P01 adds eight Core search tests and three Mac presentation-state tests = **182 tests**. The existing real-Go managed integration test gains assertions that search/reveal resolution leaves both working owners, the current owner, draft, transcript identities, pending admission and owner count unchanged. No assertion was weakened or removed.

New coverage: exact/case-insensitive and repeated matches; composed Unicode, sharp-s and Japanese; literal punctuation; empty/whitespace/no-match queries; long text and explicit bounds; visible versus private/structured content; projected dialogue; stable selection; changed/deleted messages; snapshots; conversation/generation changes; stale selection rejection; reading-cache continuity; bounded deterministic benchmarks.

```sh
swift build --package-path macos --scratch-path /tmp/gator-native-build
swift test --package-path macos --scratch-path /tmp/gator-native-build --no-parallel
swift test --package-path macos --scratch-path /tmp/gator-native-build --no-parallel \
  -Xswiftc -strict-concurrency=complete -Xswiftc -warnings-as-errors \
  --filter 'transcriptSearch|transcriptFind|actualGoManagedDialogueKeepsMainWorkIndependentAndRetainsChildResult'
GATOR_TEST_BINARY=/tmp/Gator-Native-Parity-P01.app/Contents/Resources/gator-runtime \
  swift test --package-path macos --scratch-path /tmp/gator-native-build --no-parallel
go test ./internal/workapp \
  -run 'TestBridgeUsesLiveCommandAndMessageState|TestBridgeProjectsCommandPanelsWithoutGoView|TestBridgeReportsOnlyActivePendingOperations'
/Users/gongahkia/.local/share/fnm/node-versions/v26.7.0/installation/bin/node --test \
  app-host/tests/native.test.mjs app-host/tests/sessions.test.mjs \
  app-host/tests/drafts.test.mjs app-host/tests/reading-state.test.mjs
git diff --check
```

Results:

| Verification | Result |
| --- | --- |
| Build and package/plist validation | PASS |
| Complete sequential Swift suite, including augmented real-Go test | 182 PASS, 51.560 s |
| Strict Swift 6 compilation and selected tests | 12 PASS, 2.280 s; warnings treated as errors |
| Complete suite using the actual packaged Go executable | 182 PASS, 53.056 s; [full log](evidence/p01/headless-full-tests.log) |
| Go projection contract tests | 3 PASS, package 0.624 s |
| Host native/session/draft/reading tests | 39 PASS, 0 failed, 0 skipped; Node 26.7.0 |
| Diff whitespace check | PASS |

The original archive, final-Quit and bounded-shutdown regression files are unchanged and remain in the complete passing suite. This includes independent/cross-process writers, conflict/failure preservation, late edits/imports/final flush and inherited output descriptors.

Failed attempts are not counted as passes: initial Swift view type checking required splitting the root expression and an explicitly isolated focused action; the first Unicode test exposed a range excluding a combining mark and the implementation was corrected. The initial host command used default Node 20.19.5 and failed the PDF fixture (`Promise.withResolvers` unavailable); `app-host/package.json` requires Node >=22.12. Re-running with the already installed Node 26 passed all 39. No tools/dependencies were installed. Some early desktop scripts sampled before lazy layout settled or encountered changing focus/unexpected prefix input; final checks poll observed conditions, verify input focus and replace the query explicitly. The settled anchor failure described above was separately reproduced and corrected.

## 7. Search measurements

Debug build, deterministic messages of roughly 900 characters each, single index construction, 20 hit and 20 no-hit queries, 1,000 index resolutions and 100 live-projection resolutions. Resolution measurements include assertions. These are bounded headless search measurements, not full native rendering or whole-app memory benchmarks.

| Messages | Build index | Hit query mean | No-hit query mean | Index resolution mean | Live identity/content validation mean |
| ---: | ---: | ---: | ---: | ---: | ---: |
| 50 | 0.167 ms | 0.270 ms | 0.184 ms | 0.54 µs | 7.31 µs |
| 500 | 1.590 ms | 2.795 ms | 1.853 ms | 0.52 µs | 50.33 µs |
| 2,000 | 6.186 ms | 11.020 ms | 7.375 ms | 0.53 µs | 190.45 µs |

The fixture uses 105 A dialogue turns and 30 B turns for desktop continuity; these are different from the synthetic benchmark projections. A sampled settled desktop process was 0.3% CPU / 172,784 KiB RSS after repeated reveals/switches; this single sample is not a leak or memory-performance assessment. Live search with both real Go owners working is covered by the managed integration assertion; desktop stress under sustained provider streaming was not measured.

## 8. Targeted native evidence

All captures use the current implementation and disposable profile `/tmp/gator-p01-7pck7qc_`, separate preference domain `dev.gator.native.p01-disposable`, and loopback-only unreachable provider configuration. Fixtures seed retained dialogue directly through Go storage APIs; no model is invoked for seeding or desktop search. No message was sent on the desktop. Final Swift/Go executable hashes match the packaged candidate. Older verified bundles were not run against this profile.

| Check | Observation / evidence |
| --- | --- |
| Full Find, source excerpt and selected row | [Light](evidence/p01/find-light.png), [dark](evidence/p01/find-dark.png) |
| Native result list and explicit result selection | [Results](evidence/p01/find-results.png), [selection observation](evidence/p01/results-selection.txt) |
| Minimum full window, 540×532 outer | [Small window](evidence/p01/find-small-light.png); controls remain visible, long content scrolls |
| Command-F, focus, typed query, Return/Shift-Return, Escape and draft | [Keyboard observations](evidence/p01/keyboard-and-reveal.txt) |
| A/B anchors, cleared scope, exact drafts, owner reuse and excluded older turn | [Continuity observations](evidence/p01/conversation-continuity.txt) |
| Compact/full shared anchor, no compact search dashboard | [Compact](evidence/p01/compact-reading.png), [observations](evidence/p01/compact-continuity.txt) |
| User scroll takes over; explicit Jump reaches bottom | [Manual scroll and Jump](evidence/p01/manual-scroll-and-jump.txt) |

The final candidate was launched in both appearances through its isolated-domain copy. Normal Quit exited the client and its direct Go child. The notch receives no search UI; physical notch interaction was not repeated for CH08. Simultaneous physical drag plus Find selection and sustained GUI background-stream updates were not exercised; the gesture guard and deterministic snapshot/owner tests cover the relevant code paths, without claiming those physical checks.

Reproduction helpers are desktop-only, outside routine tests: `seed-transcripts.go`, `desktop-fixture.py`, `desktop-session.py`, guarded `desktop-control.swift`, `verify-continuity.py` and `verify-keyboard.py`. Run from repository root. The controller refuses other bundle identifiers. It is not production code.

## 9. Changed-file inventory

| File | Change |
| --- | --- |
| `Sources/GatorCore/NativeTranscriptSearch.swift` | New bounded read-only index, public-content policy, match identity and live reveal validation. |
| `Sources/GatorMac/NativeTranscriptFind.swift` | Window-local Find state, focused command, search controls/results and focus task. |
| `Sources/GatorMac/ConversationView.swift` | Full-window Find affordance, index refresh/scope changes, exact reveal/outline and layout-aware reading restoration. Also includes the prior menu toggle insertion. |
| `Sources/GatorMac/GatorApp.swift` | Focused Command-F routing; menu-stage preference uses read-only scene insertion binding. |
| `Tests/GatorCoreTests/NativeTranscriptSearchTests.swift` | Eight deterministic content/identity/bounds/benchmark tests. |
| `Tests/GatorMacTests/NativeTranscriptFindTests.swift` | Three state/snapshot/switch/anchor regressions. |
| `Tests/GatorCoreTests/ManagedGoIntegrationTests.swift` | Additional read-only search assertions while two actual Go owners work. |
| Menu stage: `MenuBarVisibilityToggle.swift`, `AppDelegate.swift`, `NotchQuickAccess.swift`, `NativeInputTests.swift` | Preference control, shared presentation startup, existing-scene reopen and one regression; included in external baseline commit. |
| This report, menu verification and `parity/evidence/p01/` | Measurements, native captures, guarded reproduction helpers and integrity/test evidence. |

Paths in this table are relative to `macos/`. `NativeReadingPositions.swift` is unchanged: the existing bounded cache is reused. No unrelated architecture cleanup.

## 10. Package, remaining issues and P02

Development candidate: **`/tmp/Gator-Native-Parity-P01.app`**.

```sh
GATOR_NATIVE_APP_PATH=/tmp/Gator-Native-Parity-P01.app bash macos/scripts/build-dev-app.sh
xcrun vtool -show-build /tmp/Gator-Native-Parity-P01.app/Contents/MacOS/GatorMac
```

The existing script is not executable directly (an initial direct invocation returned permission denied); invoking it with Bash passed without changing its mode. Candidate domain is `dev.gator.native`; final Mach-O minimum is 15.0. No signing/notarization/distribution work. `/tmp/Gator-Native-Safety-Verified.app` and `/tmp/Gator-Native-UIUX-Verified.app` are preserved; hashes are in the integrity evidence.

Remaining limitations are explicit scope choices: loaded projection only; no collapsed raw-detail/activity search; Markdown-source matching; no inline character highlight; no compact/notch search. Physical accessibility/OS variants and streaming desktop stress remain unverified. No new P0/P1 issue was reproduced in the completed headless/native checks; this is not proof of absence in untested environments. Existing UI/UX P2 items, including dense recovery evidence and duplicate document basenames, remain outside this increment.

CH03, CH04, CH12, QT01, QT04, MD03, AS05, FL07, FI02, FI08 and PL02 were not implemented. CH07 paging was not added. There is no global search, rename, branch, queue, provider configuration, file-management or plan-editing workflow here.

The architecture is ready for a separately selected P02, subject to its own contracts and verification. No automatic feature selection or implementation follows P01.
