# Gator native parity P03 — AS05 workspace selection

Date: 2026-11-18. Scope: AS05 only. This is selected native parity increment P03, not native migration Tranche 09. P01/P02 remain supported; no P04 work is authorized or started.

## Outcome and baseline

**AS05 is implemented and headlessly verified. P03 desktop acceptance is incomplete.** Successful picker selection, keyboard navigation, compact presentation and minimum-window interaction still require an uninterrupted desktop session. Do not treat the candidate as fully desktop-accepted from the evidence below.

Implementation adds a native directory picker, contextual workspace display, and a finite Go-confirmed source resolution/new-owner handoff. A different canonical source opens a new conversation; selecting the current Go source keeps the conversation. Selection does not submit a message, admit Work, capture a snapshot, grant permission or replay retained execution.

Starting HEAD: `07675f2c5dac1fc336446116a0cecc5cf38efb94`. The worktree already contained P02's cached-title correction, regression assertions, report and evidence. These were preserved. [Baseline](evidence/p03/baseline.json) records status and protected source/bundle hashes. No README edit, commit, installation, production provider/configuration change, signing or notarization was performed. Test configuration is confined to temporary profiles and inert loopback providers.

The implementation was interrupted once by the repeated P03 request and resumed from the same worktree. No earlier requested increment was restarted.

## 1. Existing Go resolver and source ownership

Verified current code:

- `internal/workapp/commands.go`, `/source`: calls `Config.ResolveSource`; when the resolved string differs, managed mode requests `BridgeSessionRequest{New:true, Source:resolved}`. Same source retains the conversation. The native workflow deliberately does not expose terminal instruction-ignore/source-refresh subcommands.
- `cmd/gator/work_interactive.go`: `ResolveSource` calls canonical `workspace.Open`, resolving relative paths against runtime configuration for the terminal. Native selection requires an absolute path and calls that same resolver.
- `internal/workspace/root.go`: `Open` obtains an absolute path, evaluates symlinks, stats the canonical directory and rejects nonexistent/file paths. Existing descriptor-rooted containment and instruction policies remain unchanged.
- `internal/workapp/conversation.go`: `beginChat` creates the durable conversation identity first, resets messages, queue and request-specific options, and retains the new source through the canonical session store. Restore uses the conversation's retained source.
- Native launch keeps `Process.currentDirectoryURL = configuration.workspace`; managed `startSource` is supplied separately as `GATOR_UI_START_SOURCE`. The Go child validates that source before startup. No `GATOR_NATIVE_WORKSPACE` or private state/upload root is rewritten.
- Existing snapshot capture/materialization under `internal/snapshot` remains the Work authority. The picker does not refresh snapshots or read file contents. Read-only means the existing Work snapshot policy; it does not chmod the selected project.

Go currently compares canonical resolved source strings with its retained source. Native uses that same rule, rather than imposing a competing filesystem identity policy on historical records. A historical source stored with a different symlink spelling can therefore follow Go's existing new-conversation rule.

## 2. Exact bridge and managed handoff

New finite frame:

```json
{"type":"native_workspace","native_workspace":{"request_id":"fresh UUID","path":"absolute selected directory"}}
```

Request-specific snapshot completion:

```json
{"native_workspace_result":{"request_id":"same UUID","conversation_id":"initiating conversation","source":"Go canonical path","new":true}}
```

A failure has `error` and omits `source`. Paths are data, never slash commands or shell arguments. The existing NDJSON transport and protocol version remain unchanged. `action_applied` alone does not complete resolution.

`ResolveNativeWorkspace` runs filesystem resolution in a Bubble Tea command rather than blocking the Go model event loop. The model admits one resolution at a time, rejects reuse of its completed request identity, and publishes only the matching completion for the initiating conversation. A read/enumeration probe rejects directories that stat successfully but cannot be read; at most one entry name is read and discarded, and no directory contents enter the result. No session request is emitted automatically by this read-only operation: it returns the exact `new`/`source` evidence to the existing managed coordinator, which performs the equivalent new-conversation handoff after checking the still-current initiating owner.

Swift captures the initiating object, generation and picker ticket before presenting the panel. A single ticket can be completed only once; a second selection/callback while resolution is pending is rejected. Swift waits for exact request identity, original conversation/generation, supported canonical source and a coherent `new` flag. A changed foreground or reconnect rejects navigation. There is no automatic retry.

For a different source, the existing serialized transition creates an owner with `bootMode=new` and `startSource=canonical`. Before foreground selection, that owner must resolve the same source itself and report the expected source, a different conversation ID and no active/queued/review/unresolved request state. Only then does existing focus and durable selection run. This second resolution catches moved, removed, inaccessible or redirected folders at startup. It does not pin a directory inode indefinitely; later filesystem changes remain subject to the existing Go Work snapshot validation.

## 3. Conversation, draft and owner isolation

- Existing owners keep their conversation, captured source, tasks, queue, approvals, recovery identities, drafts and immutable attachment references.
- New owner configuration carries the validated source, not an old run, task, approval, queue or attachment draft. Native private upload storage remains in the original app-owned state namespace.
- Existing four-owner capacity, three-working-task admission, uncertain/recovery pinning and explicit reconnect rules remain in force. The initiating owner is retained while its picker/resolver is pending.
- Source selection uses the existing transition queue; unrelated catalog queries and working Go owners continue. Canonical resolution itself neither changes focus nor selects a conversation.
- Failed validation leaves native archive bytes unchanged in deterministic tests. Successful selection uses the unchanged durable archive transactions and per-conversation contexts; no new app-wide source preference or archive schema is added.
- Returning through Recent/All Conversations reuses the original owner where cached. Restart resumes the durable conversation source; it does not apply the most recently picked source to every conversation.

If child startup has been attempted but the handoff is unconfirmed, the UI requires opening a retained conversation for review before another choice. Safely disposable failed candidates are shut down; a candidate with unexpected review/uncertain state is retained and exposed through the existing activity/catalog navigation. No authorizing action is inferred from source selection. A capacity rejection before startup remains directly retryable after making room.

## 4. Native UI and picker

`NativeWorkspacePicker` is the small AppKit boundary:

- Standard `NSOpenPanel`: one existing directory, ordinary files disabled, directory creation disabled, aliases resolved, cancellation supported.
- Explicit Choose Workspace… in the full-window toolbar, Conversation menu and compact More menu. No launch-time prompt or notch dashboard.
- Panel explains new-conversation behavior, unchanged background scope and read-only Work snapshots.
- Asynchronous panel completion; no modal event loop or synchronous process wait is introduced.
- Security-scoped access, when actually supplied, is balanced around resolution/startup. No bookmarks are persisted. This development app is not sandboxed; no assumption is made that a sandbox extension transfers to a Go child.
- Contextual canonical path truncates in the middle with full path help/accessibility text and text selection. Read-only semantics, validation/opening progress and bounded visible errors appear beside the conversation. An uncertain result offers the existing All Conversations control.

Paths are not added to telemetry or unrelated chat preview strings. Existing catalog source metadata remains canonical Go-owned data.

## 5. Filesystem and uncertainty boundaries

| Case | Behavior / evidence |
| --- | --- |
| Spaces, accented text, Japanese and command-like punctuation | Passed through typed data and resolved by real Go; no command parser. |
| Directory symlink | Canonical target returned by Go; choosing it again retains the same owner when Go source matches. |
| Regular file, missing folder, unreadable directory | Resolver/read-probe error, old foreground/draft retained; no directory creation. |
| Folder moved after validation, before startup | Deterministic factory hook moves the temporary folder before the real Go child launches; startup fails, old owner retained, explicit review required. |
| Control characters, including newline/NUL | Explicit unsupported-path error. No ambiguous multiline label or terminal command construction. |
| Long existing path | Go regression resolves a 35-component directory; bounded native input remains 8,192 UTF-8 bytes and OS path limits still apply. |
| Independent instances | Separate actual Go owners create distinct durable conversations for the same selected directory; shared native archive retains both exact independent drafts. |
| Removal/revoked access after success | No persistent access grant or inode pin is claimed. Historical conversation source stays retained; later Go resolution/snapshot access must succeed. |
| Network/removable volume | Missing/unavailable directory paths use the same resolver error path. Physical unmount, network filesystem stalls and macOS TCC revocation are NOT TESTED. |

The resolver is read-only. A timed-out/disconnected resolution is not retried automatically and cannot itself admit a conversation/task. A Go process blocked inside a filesystem syscall may outlive the UI's result deadline; shutdown still uses the existing bounded direct-process lifecycle. Hard real-time filesystem syscall progress is not claimed.

## 6. Verification

Final commands, counts, results, available screenshots and candidate integrity are recorded below. Desktop acceptance limits are explicit in section 8.

### Headless commands and results

```sh
swift build --package-path macos --scratch-path /tmp/gator-native-build
GATOR_TEST_BINARY=/tmp/gator-p03-runtime swift test --package-path macos \
  --scratch-path /tmp/gator-native-build --no-parallel
GATOR_TEST_BINARY=/tmp/Gator-Native-Parity-P03.app/Contents/Resources/gator-runtime \
  swift test --package-path macos --scratch-path /tmp/gator-native-build --no-parallel
GATOR_TEST_BINARY=/tmp/Gator-Native-Parity-P03.app/Contents/Resources/gator-runtime \
  swift test --package-path macos --scratch-path /tmp/gator-native-build --no-parallel \
  -Xswiftc -strict-concurrency=complete -Xswiftc -warnings-as-errors \
  --filter 'workspace|actualGoManagedDialogue'
go test ./cmd/gator ./internal/workapp ./internal/worksession ./internal/workspace
/Users/gongahkia/.local/share/fnm/node-versions/v26.7.0/installation/bin/node --test \
  app-host/tests/native.test.mjs app-host/tests/sessions.test.mjs \
  app-host/tests/drafts.test.mjs app-host/tests/reading-state.test.mjs \
  app-host/tests/backend.test.mjs
GATOR_NATIVE_APP_PATH=/tmp/Gator-Native-Parity-P03.app bash macos/scripts/build-dev-app.sh
python3 macos/parity/evidence/p03/verify-integrity.py
git diff --check
```

| Verification | Result |
| --- | --- |
| Swift build | PASS. Swift tools 6.1 / Swift 6 language mode; macOS minimum 15, no new dependencies. |
| Complete sequential Swift suite, final packaged Go | **214 PASS**, 136.316 seconds; [log](evidence/p03/swift-packaged-full.log). |
| Earlier complete suite | 212 PASS before two additional boundary tests; then 214 PASS, 136.889 seconds. The latter used the earlier runtime, so the final packaged run above is the acceptance result. |
| Strict Swift 6, warnings-as-errors, final filesystem implementation | **16 selected PASS**, 12.554 seconds; all production/test sources compiled. [Log](evidence/p03/swift-strict-final.log). |
| Broad strict P01/P02/P03 selection before final traversal-probe correction | 44 PASS, 82.172 seconds. [Log](evidence/p03/swift-strict-p01-p02-p03.log). |
| Go cmd/gator | PASS, 30.489 seconds. |
| Go workapp | PASS, 7.604 seconds. |
| Go worksession / workspace | PASS, cached. [Four-package log](evidence/p03/go-regression.log). |
| Host native/session/draft/reading/backend | **41 PASS**, zero failed/skipped/cancelled; 3.539 seconds, installed Node 26.7.0. [Log](evidence/p03/host-regression.log). |
| App packaging, plist and deployment target | PASS. Final executable minimum macOS 15.0 / SDK 26.5. [Package log](evidence/p03/package.log). |
| Protected source, safety regressions, prior bundles and closing barrier | PASS, [integrity manifest](evidence/p03/integrity.json). |

P02 had 199 tests. P03 adds **15**: ten Core protocol/owner tests, four real-Go integration tests and one nonpresenting panel-configuration test. The existing real-Go managed-work test gains assertions that a workspace change leaves the original Work running at its captured source and preserves parent/child drafts, while the new owner is idle and empty. Existing assertions/counts remain in place before the additional fourth-owner step.

Coverage includes picker ticket cancellation/duplicate completion, data encoding, exact response identity, same/different folders, a picker surviving reconnect, stale foreground results, resolver disconnect, startup failure, unexpected retained review state, cache exhaustion, attachment/draft/approval/recovery isolation, two independent instances and canonical source restoration after restart. P01 Find/anchors and P02 paging/version conflicts/non-recent selection remain in the passing complete suite. All cross-process archive, same-context conflict, Quit and inherited-pipe shutdown tests remain unchanged and pass.

No assertion was weakened. Initial compile errors were in the new test's optional dictionary access and were corrected. An early mock emitted a new snapshot after its simulated disconnect; it was corrected to model the closed workspace transport instead of reviving the fixture owner. Initial real-Go assertions incorrectly used Foundation's `/var` spelling instead of the filesystem/Go `/private/var` identity, and explicitly reopened the original fixture ID rather than exercising normal startup; the fixtures were corrected.

The additional traversal-permission regression **failed before correction**: an empty `0400` directory was accepted because enumeration alone did not require traversal. [Before log](evidence/p03/permission-before.log). The probe now opens `canonical/.` and enumerates at most one entry, requiring read and traversal access. The Go regression and final actual-Go Swift test reject both `0000` and `0400` temporary directories. [Targeted Go result](evidence/p03/go-targeted.log). No resolver, snapshot or instruction containment policy was relaxed.

One final packaged-fixture handoff measured **86.789 ms** from selection request to confirmed selected owner. This is a single local deterministic fixture measurement, not a claim about large project snapshots, network volumes or UI frame rate.

## 7. Changed-file inventory

| Files | P03 change |
| --- | --- |
| `internal/workapp/bridge_workspace.go` | Finite read-only asynchronous resolver, request/completion identity, canonical source/new-owner evidence, bounded path and access validation. |
| `internal/workapp/app.go`, `bridge.go`, `update.go` | Pending/result fields, display projection and completion dispatch. Existing P02 load-issue field preserved. |
| `cmd/gator/ui_bridge.go` | Typed workspace frame dispatch; no command parser. |
| `macos/Sources/GatorCore/NativeWorkspaceSelection.swift` | Typed request/result, exact identity/generation/source checks and request-specific wait. |
| `ManagedSessionProtocol.swift` | Private finite workspace host command. |
| `ManagedSessionCoordinator.swift` | Shared picker ticket, stale-owner guard, existing new-owner transition, destination validation, failure/review state and origin retention. Existing closing barrier and task admission remain. |
| `macos/Sources/GatorMac/NativeWorkspacePicker.swift` | Standard asynchronous directory panel, scoped-access lifetime, shared Choose action and contextual workspace status. |
| `ConversationView.swift`, `GatorApp.swift` | Existing toolbar/Conversation/More entry points and restrained workspace context. |
| `macos/Tests/GatorCoreTests/NativeWorkspaceTests.swift` | Ten deterministic state/ownership/protocol regressions. |
| `NativeWorkspaceIntegrationTests.swift` | Four actual-Go temporary filesystem, persistence, move-at-start and independent-instance tests. |
| `ManagedSessionTests.swift` | Finite workspace mock/gate support only. |
| `ManagedGoIntegrationTests.swift` | Existing background Work test extended with explicit workspace isolation assertions. |
| `macos/Tests/GatorMacTests/NativeWorkspacePickerTests.swift` | Headless configuration assertions; panel is not displayed. |
| `internal/workapp/bridge_workspace_test.go`, `cmd/gator/ui_bridge_test.go` | Six Go contract/permission tests including ack-before-resolution; canonical filesystem and stale results. |
| This report and `evidence/p03/` | Commands, logs, integrity and disposable desktop reproduction/evidence. |

`NativeDraftStore`, `GatorApplicationState`, `GoBridgeConnection`, `BridgePipeIO`, `NativeUploadStore`, P01 search/reading implementations and the three safety regression files are byte-preserved. Existing P02 source/load corrections and evidence remain separate in the worktree; they are not claimed as P03 edits. No archive schema, database, coordinator, transport server, dependency, credentials or provider configuration was added.

## 8. Desktop evidence and remaining limits

The candidate was launched through a byte-identical separate-preference-domain copy, `/tmp/Gator-Native-Parity-P03-Desktop.app`, using profile `/tmp/gator-native-test-p03-60anfx0w` and temporary fixture folders. The provider is unreachable loopback `127.0.0.1:9`; no desktop message was sent and no live user project was selected.

| Desktop check | Actual observation |
| --- | --- |
| App launch/current workspace | PASS: retained topic 0005, its original source, Read-only in Work and its exact unsent draft appeared. Outer window observed at 820×740. |
| Standard folder picker | PASS: Choose Workspace… opened the native directory panel at the fixture source with the new-conversation/background/read-only explanation. [Picker screenshot](evidence/p03/picker-light.png). |
| Cancellation | PASS: panel closed; original conversation, source and exact draft remained visible. [Filtered observations](evidence/p03/desktop-observations.txt). |
| Successful selection and updated source label | **NOT VERIFIED on desktop.** Real-Go handoff/source/draft checks passed headlessly; native panel selection could not be reliably completed during focus contention. |
| Keyboard, P01 Find continuity, return to older reading anchor | **NOT VERIFIED in this desktop session.** The controller refused keyboard delivery when the fixture was not frontmost. The complete/strict regression suites retain P01/P02 coverage. |
| Compact/full and minimum-window interaction | **NOT TESTED in this increment.** Source layout bounds and existing tests pass; those do not establish a current desktop interaction result. |
| Quit | PASS: normal Quit exited the fixture app and its direct Go child. |

The first hand-built desktop archive omitted NativeDraft's required revision and correctly failed closed. Only that disposable fixture was repaired, then the app relaunched with the valid schema; production persistence and safety checks were untouched. The reproduction helper now includes the required revision/date.

Foreground repeatedly moved to another application. An asynchronous availability question requested a five-minute uninterrupted desktop window; no reply was received during this work. These incomplete checks are not counted as passes. The app was quit after recording the verified cancellation state; no fixture app or provider was left running.

Physical VoiceOver, IME, actual macOS 15, OS preference variants, network/removable-volume unmount and TCC revocation remain NOT TESTED. The saved picker screenshot and executable provenance are recorded by [integrity.json](evidence/p03/integrity.json). No earlier bundle screenshot is passed off as P03 evidence.

## 9. Remaining risks and readiness

- No new P0/P1 defect was reproduced in the completed headless and limited desktop checks. This is not proof of absence; final desktop acceptance remains outstanding.
- The read-without-traversal validation defect found during implementation was reproduced and corrected, with Go and actual-Go Swift regressions passing against the packaged runtime.
- Existing engineering P2 recovery reconciliation/owner-retirement constraints and P02 bounded catalog/index-fallback limitations remain unchanged. A pinned four-owner cache still rejects a new workspace owner explicitly.
- Folder availability can change after confirmation. No persistent filesystem authorization or inode pin is claimed; Go's existing snapshot/containment authority remains responsible for later Work. Physical volume/TCC revocation was not induced.
- A failed startup can leave a durable empty conversation in Go's catalog. It is not silently deleted or replayed. The UI requires retained-conversation review after an attempted unconfirmed handoff.
- The current development workflow does not persist security-scoped bookmarks or implement a sandboxed child permission broker. Sandboxed distribution would require separate verification; folder selection is not general tool authorization.

Final development candidate: `/tmp/Gator-Native-Parity-P03.app`. P01 and P02 bundles are byte-preserved. Swift and Go executable hashes match the tested desktop copy; the bundled Go binary also matches a fresh current-source build. No signing/notarization or distribution work was performed.

**P03 is not fully accepted until the outstanding targeted desktop checks are completed.** The implementation and headless verification are ready for that review. P04 was not started.

## 10. Desktop acceptance closure — supersedes the earlier pending verdict

See [P03 desktop acceptance](P03-DESKTOP-ACCEPTANCE.md) for the complete final evidence, focus diagnosis and limits.

**Final P03 verdict: PASS for `/tmp/Gator-Native-Parity-P03-Accepted.app`.** The original `/tmp/Gator-Native-Parity-P03.app` and previous bundles are preserved. Earlier sections describe the original implementation/evidence; this closure records the subsequent focused correction.

Actual desktop testing found a reproducible Unicode source-spelling defect: composed/decomposed macOS paths could cause destination verification to reject a valid new owner. It failed both a real-Go Swift regression and a Go unit regression before correction. The native Go resolver now confirms directory identity with `os.SameFile` when source spellings differ and retains the exact current source for a same-directory selection; Swift checks wire-byte consistency. Directory identity remains Go-owned. No source snapshot, execution, approval, archive, coordinator, transport or Quit implementation was changed.

After the correction: **215 Swift tests PASS**, **17 targeted strict Swift 6 tests PASS**, **four Go packages PASS**, **41 host tests PASS**. The one additional Swift test covers Unicode forms through handoff and repeated same-source selection. [Regression and integrity evidence](evidence/p03-desktop/integrity.json).

A fresh, user-reserved desktop session conclusively verified A/B selection, Go source/conversation identity, cached-owner and exact draft continuity, same-workspace Return confirmation, keyboard Go to Folder/Return and Escape, compact More/picker/Cancel, the 540×532 outer minimum, long-path truncation, original-transcript Find/older reading continuity, no new request admission and normal Quit. [Compact](evidence/p03-desktop/compact-final.png), [minimum](evidence/p03-desktop/minimum-final.png), [reading before](evidence/p03-desktop/reading-before-final.png), [reading after](evidence/p03-desktop/reading-after-final.png), [controlled observations](evidence/p03-desktop/controlled-final.txt).

Initial focus-contended attempts and an earlier fixture changed by shared input were excluded from final continuity/no-Send acceptance. Physical VoiceOver/IME/input preference variants, actual macOS 15, physical volume/TCC revocation and all compact Escape permutations remain untested as documented in the closure report. No new P0/P1 was reproduced in the clean run. No P04 work or another feature increment was started.
