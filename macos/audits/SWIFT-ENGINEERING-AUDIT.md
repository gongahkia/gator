# Gator Swift engineering and architecture audit

Date: 2026-11-17 (Asia/Singapore). Read-only application review; the two audit reports are the only files authored in the repository by this review. No implementation, README edit, commit, dependency installation, credential/provider change, real external action, signing work or backend-goal resumption occurred. Test providers used loopback and disposable profiles.

## 1. Executive assessment

**The architecture is sound enough to retain, and the codebase remains maintainable, but persistence and shutdown need corrections before substantial native expansion.** The 33 production Swift files total 4,458 lines: 2,738 in GatorCore and 1,720 in GatorMac. The important separation is already present: Go owns execution and durable authorization; an application-wide coordinator owns selection and capacity; one state object owns each runtime; SwiftUI/AppKit present that state through typed methods.

The current complete Swift suite passes **133 tests in 38.762 seconds**. An explicit Swift 6 strict-concurrency build with warnings treated as errors also passes. These results coexist with new failure evidence: concurrent archive writers lost 14 of 16 unrelated records; an owner reported successful shutdown despite a new draft save failing during shutdown; a direct child's inherited output pipes extended shutdown to 8.027 seconds. The first is logical archive corruption under multiple independent writers, not malformed JSON. No unintended external execution or approval bypass was reproduced.

Five priority risks are E01–E05 below. E01–E03 should be corrected and regression-tested before broad feature work; E04–E05 need a coherent recovery/owner-retirement policy before adding more controls. Avoid a wholesale rewrite, a new transport, or another persistence engine. The parity inventory is a selection document, not authorization to implement its candidates. It catalogs 105 capabilities: 37 FULL, 21 PARTIAL, 40 MISSING and 7 INTENTIONALLY DIFFERENT (zero DEPRECATED).

### Review baseline and moving worktree

- Initial HEAD: `13134d5f2c81ae6f4ed8c4bea1948774d15307e6`.
- Initial local changes: modified `macos/PROVIDER-DIAGNOSTICS.md`, `macos/TRANCHE08-ACCEPTANCE-CLOSURE.md`, `macos/TRANCHE08-LOCAL-DEV-ACCEPTANCE.md`; untracked `macos/Evidence/Local-Dev-Acceptance/headless/{final-integrity.json,offline-regional-override.go.txt,offline-regional-override.jsonl}`.
- During review an external commit advanced HEAD to `137fb7ea923d14149badbb02e102ffdfb6e34bed`. It incorporated those report/evidence changes and added `macos/audits/evidence/uiux/headless/{baseline.json,swift-audit.log}`. That other audit's log continued changing. This review did not write those files or create that commit.
- `git diff 13134d5f2c81ae6f4ed8c4bea1948774d15307e6 HEAD -- macos/Sources macos/Tests macos/Package.swift macos/Resources macos/scripts cmd/gator/ui_bridge.go internal/workapp internal/attachment app-host ui docs/CLI-TUI-PORT.md docs/TERMINAL.md docs/DEVELOPMENT.md` was empty. Source/test/contract evidence and the fresh runtime therefore cover both revisions.
- Apple Swift 6.3.3 (`swiftlang-6.3.3.1.3`), arm64 host, Darwin 25.5.0. Package tools version 6.1; build description confirms `-swift-version 6` and target `arm64-apple-macosx15.0` for both production and both test modules. Minimum deployment target and Info.plist both specify macOS 15.
- Go 1.27.3 locally; `go.mod` requires 1.25.13. Native package has **no third-party Swift dependencies**. System dependencies: Foundation, Observation, CryptoKit, Security, ImageIO, UniformTypeIdentifiers, Darwin, SwiftUI/AppKit.
- Node 20.19.5 and Bun 1.3.11 were on the shell PATH. Node is below app-host's declared `>=22.12.0`; Bun is below the 1.4.2 documented terminal development version. The selected Node host tests passed, but this does not establish full supported-toolchain terminal acceptance. No installation was attempted.
- SwiftLint 0.65.1 and SwiftFormat were present. No repository Swift formatter/linter policy was found. Their default-style findings are reported separately from correctness.

## 2. Complete codebase and module responsibility map

Every production Swift file in the requested two directories was inspected. Paths below are relative to `macos/`.

### GatorCore — application/host implementation, no SwiftUI dependency

| File (lines) | Responsibility and dependency boundary |
| --- | --- |
| `BridgeProtocol.swift` (231) | Lossless JSONValue display envelope; snapshot/message/transcript projections; exact action review identity; NDJSON byte framing; internal finite outbound command encoding. Does not confer tool authority. |
| `GoBridgeConnection.swift` (198) | Launch configuration, transport protocol and actor; subprocess, queued writes, pipe readers, nonce waiters, diagnostics, disconnection and exit escalation. |
| `ManagedSessionProtocol.swift` (69) | Internal managed navigation, dispatch, task control and host configuration DTOs/validation. Not exported to GatorMac. |
| `ManagedSessionCoordinator.swift` (507) | Main-actor observable application singleton: selected owner, bounded owner registry, serialized navigation/admissions, background dispatch, capacity, inspection/correction routing, focus and all-owner shutdown. |
| `GatorApplicationState.swift` (885) | Main-actor observable single-owner state: transport generation, projection, drafts/imports, admission/settings/stop/approval state, plan reads/authorization, recovery/access/receipt state and durable client receipts. |
| `NativeDraftStore.swift` (161) | Actor for native-only JSON archives, versions 1–7, owner-context merging, retention, protected operation IDs and atomic file replacement. |
| `NativeDraftAttachments.swift` (16) | Captured attachment import identity/context/generation and explicit clipboard policy. |
| `NativeUploadStore.swift` (188) | Shared upload actor; descriptor-relative private immutable staging, hashing, validation, bounded image normalization/thumbnails. |
| `NativeTaskReview.swift` (156) | Work modes; task/plan/correction identities; literal exact-review field projection; keyboard decision semantics and bounded public task presentation. |
| `NativePlanExecution.swift` (65) | Typed plan authorization operations/phases, capacity reservation, review completeness and scope comparison. |
| `NativeRecovery.swift` (159) | Read-only recovery projection, exact sealed receipt identities, receipt operations and conservative outcome labels. |
| `NativeRecoveryPreparation.swift` (65) | Recovery-token/access-selection identities, explicit preparation intents and durable preparation status. |
| `NativeMessageContent.swift` (38) | Presentation-only paragraph/list/heading/code-fence projection; structured review data bypasses Markdown. |

### GatorMac — scenes, presentation and narrow platform adapters

| File (lines) | Responsibility |
| --- | --- |
| `GatorApp.swift` (87) | One shared coordinator above MenuBarExtra and full Window; commands, native environment/profile paths and brand image lookup. |
| `AppDelegate.swift` (61) | Activation/window observers, guarded Quit, draft/owner cleanup before application termination; owns notch presentation. |
| `ConversationView.swift` (342) | Main conversation, menus, recents/activity routing, lazy transcript, follow-latest scrolling, notices/model details; private message row. |
| `ComposerView.swift` (76) | Draft/composer controls, explicit Send, plan-first, attachment and correction presentation. |
| `NativeComposer.swift` (157) | NSViewRepresentable/NSTextView editing, IME, standard shortcuts, bounded paste, Tab traversal, file drops and token-based focus observer lifetime. |
| `NativeAttachmentActions.swift` (61) | NSOpenPanel, pasteboard image intake and NSItemProvider file URL loading; invokes owner-captured imports. |
| `NativeDropInput.swift` (22) | Typed pasteboard file URL extraction and opt-in bounded DEBUG metadata tracing. |
| `AttachmentSelectionView.swift` (43) | Count-bounded attachment list, removal, async validation and per-row thumbnail state. |
| `NativeWindowConfiguration.swift` (56) | NSViewRepresentable window registration; weak presentation references; sticky regular/accessory activation policy and DEBUG appearance override. |
| `NotchQuickAccess.swift` (239) | Display camera-gap geometry; application-lifetime nonactivating panel, hover delays, keyboard entry, attention/working projections and original-owner navigation. No decision authority. |
| `ApprovalReviewView.swift` (51) | Exact pending action inspection and guarded correction/decline/approval intents. |
| `ExactReviewContent.swift` (22) | Literal exact fields plus complete raw review disclosure. |
| `ExactDecisionButton.swift` (86) | SwiftUI accessibility/keyboard wrapper and NSButton adapter; Return rejected, explicit Space/click/AX press supported. |
| `TaskInspectorView.swift` (107) | Original-owner inspection, public activity, retained reviews, plan reads and recovery content. |
| `PlanReviewContent.swift` (90) | Expanded material plan context and revision-bound authorization controls. |
| `RecoveryInspectorContent.swift` (84) | Retained requests/actions, exact eligible receipt-check UI, recovery plan navigation. |
| `RecoveryPreparationView.swift` (80) | Ordered configured-account selection, fresh Go access review and explicit plan preparation. |
| `NativeAccessibilityStatus.swift` (16) | Meaningful AX status announcements only in a key presentation, without moving focus. |
| `NativeMessageBody.swift` (28) | Selectable native Markdown prose/code; plain fallback on inline parse failure. |
| `NativeErrorDisclosure.swift` (12) | Bounded collapsed error summary and complete selectable error. |

### Package, resources, scripts and contracts

`Package.swift`: executable GatorMac → GatorCore; GatorCoreTests → GatorCore (copied Fixtures), GatorMacTests → both. No cycles or external Swift package resolution. AppKit-dependent tests exist despite the requested test-path focus being GatorCoreTests; they were included in the full run.

`Resources/Info.plist`: accessory app (`LSUIElement=true`), bundle ID `dev.gator.native`, macOS 15; no microphone feature declared. `GatorHead.png` is 1024×1024; `GatorMenuBar.png` is 36×36. `scripts/make-app-icon.swift` generates the original silhouette PNG/iconset. Its bitmap/argument force unwraps concern a bounded development generator, not user-supplied production attachment decoding. `scripts/build-dev-app.sh` builds Swift and Go, stages a fresh app, copies resources, checks plist and refuses an existing destination. No signing/notarization audit was performed. Normal SwiftPM resource-free execution uses the brand's system-symbol fallback; the development bundle copies actual images.

| External area inspected | Authority/responsibility |
| --- | --- |
| `cmd/gator/ui_bridge.go` | Version-1 NDJSON decoder/encoder and canonical workapp routing. State is emitted before nonce acknowledgment; acknowledgments confirm dispatch through the model, not business success. |
| Relevant `internal/workapp/` bridge, commands, sessions/dialogue, requests/run, plan, recovery/access, receipts and catalog files/tests | Canonical state, admission/dedup, exact current review, task origin/run controls, execution leases, plan/recovery identity, queues and terminal panels. Unrelated Go packages were not exhaustively read. |
| `internal/worksession/requests.go:269` and lease test | Targeted boundary expansion: execution ownership uses a nonblocking exclusive lock held for the actual run and released on process exit. This was necessary to assess native multi-instance authority. |
| `internal/attachment/{attachment,uploads,originals,pdf}.go` and selected tests; `cmd/gator/attachments.go` | Go attachment loading, provider/retained-original naming, bounds and validation; backend remains authoritative after Swift checks. |
| `app-host/contracts/` | Complete contract inventory: protocol, frontend, files, voice, source/code previews, inspection and external-terminal DTOs. Contract availability alone is not native reachability. |
| `app-host/src/{backend,application,sessions,drafts,uploads}.mjs` | Reference terminal host: one process per session, host-private commands, profile/data coordination, file/export/evidence services and native helpers. Swift does not import this JS layer. |
| `ui/src/`, terminal docs, relevant app controller/tool definitions | Actual terminal entry points and conversational coverage, detailed in the companion parity report. |

Recent verification reviewed: Tranches 01–08 records, Tranche08 acceptance closure/local-development acceptance and menu-bar/notch verification. Historical 128/130/132 totals are superseded by the current 133-test run. Latest local-development report closes its scoped synthetic/inert-approval and explicit Chat-route acceptance, while physical VoiceOver/IME/preferences remain NOT TESTED. It records a Responses geography/configuration limitation and an explicit working Chat route; earlier blanket provider-blocker statements are historical. No live provider request was repeated here.

## 3. Architecture assessment

The dependency direction is appropriate. GatorCore imports no SwiftUI; GatorMac cannot construct internal BridgeCommand/HostCommand or raw-send protocol frames. The coordinator holds application selection/capacity, while GatorApplicationState holds one owner's transport, snapshot, exact tokens and drafts. Dismissing a presentation does not stop an owner. The notch reuses the same application coordinator rather than owning a runtime. Go execution journals and locks remain the final authority.

Repeated checks have different purposes: native UI eligibility supplies current affordances; synchronous typed methods reject stale callbacks; post-persistence checks handle actor reentrancy; Go revalidates actual scope/leases/tokens. Removing these layers as “duplicate validation” would weaken a demonstrated security invariant. Frame text/attachment bounds similarly protect independent boundaries.

The broadest maintenance burden is **five independently hand-coded operation state machines inside one owner**, not the 885-line count itself. Plan authorization can reconcile a new-generation read, while receipt operations cannot; see E04. Status strings, durable receipts, deadlines and matching-result rules are manually repeated. Extract owner-local operation reducers/helpers only after defining each domain's distinct reconciliation semantics. Do not add an actor per view or a generic operation framework that erases the difference between admission, authorization and delivery.

State duplication is mostly deliberate: raw `snapshot`, `messages`, and joined `transcript` preserve raw approval indexes and offer presentation caches; `archive.pending` controls durable admission blocking while `submission` retains the terminal display outcome. These cannot simply be merged without preserving those behaviors. The owner loads a complete archive even when it edits one context; an owner-local persistence snapshot would reduce accidental cross-context coupling while preserving the shared store's merge authority.

Views do some domain presentation (plan eligibility selection, source/account text projection, task-link checks), but executable routing/validation generally remains in Core/Go. The file picker/drop/clipboard adapters contain platform I/O because that is their responsibility. A model-view-viewmodel rewrite or a protocol around every concrete type has no demonstrated benefit. Existing BridgeTransport injection is useful and sufficient for many deterministic tests; the missing seam is deterministic persistence/closing failure control.

## 4. Swift concurrency and async ownership

- Both observable shared state classes and the notch controller are `@MainActor`; store/transport/upload mutation is actor-isolated. Sendable values cross boundaries. Production `@unchecked Sendable` is limited to the NotificationCenter-token holder in `NativeComposer.swift:153`; removal is thread-safe and the callback is main-queue isolated. Other unchecked fixture pools lock their arrays.
- Detached tasks in `GoBridgeConnection.swift:76,87` own pipe consumption, not presentation state. POSIX blocking reads run on a utility Dispatch queue, away from Swift's cooperative executor. The actor receives bounded byte chunks and performs decoding; MainActor projection then updates views. This avoids the previously identified idle-stderr starvation path.
- Checked continuations have one result path for each actual POSIX read. Nonce continuations are removed before resume on ack, timeout or fail. NSItemProvider completion has one callback branch per result but lacks a cancellation/timeout escape (R02).
- Generation/context/revision checks cover stale transport events, newer text during admission, attachment completion, plan/recovery decisions after persistence and queued coordinator actions. Examples: state `receive` at 191; submission recheck at 375; imports at 527; plan at 620; access/preparation at 726; coordinator submit at 214. Existing tests exercise these interleavings.
- Application transitions use an explicit chain of unstructured tasks, rather than caller-owned task lifetime. This makes scene dismissal unable to cancel a host admission/navigation. That is a reasonable application-lifetime policy. Their termination and cancellation semantics should be explicit, not mechanically converted to view `.task` cancellation.
- Save tasks also form a serialized chain (state 329–341). Correct ordering is useful, but every edit retains a full archive value and schedules an fsync/rename transaction. This is an optimization opportunity, not a measured leak; see R03.
- MainActor reentrancy is material during Quit: E02 has a reproduced late edit/save interleaving. Actor isolation prevents data races, but does not make a multi-await shutdown transaction atomic.
- Cancellation is incomplete: send continuations and blocking reads are not cancellation handlers. Sleep cancellation is intentionally ignored in several owner wait loops. R01 explains a specific starvation interleaving; there is no current production transition-cancel call proving reachability.
- Strict checking found no warnings/errors under the actual Swift 6 mode. This checks isolation, not multi-instance transactions, FD inheritance, logic staleness, or GUI focus correctness.

## 5. NDJSON transport and lifecycle

`BridgeFramer` buffers bytes, decodes complete newline-delimited JSON only, rejects malformed/unsupported frames and partial EOF, and requires protocol version 1. A 16 MiB aggregate buffered-data limit bounds input; reads are 64 KiB. Outbound commands have a 256 KiB encoded limit, 128 KiB message text limit and finite cases. Go permits a 4 MiB inbound scanner token and rejects unknown fields/version mismatches (`ui_bridge.go:275–285`). Thus current Swift outbound bounds fit Go. The framer limit applies before extracting lines, so a valid nearly-limit frame plus a coalesced following frame can be rejected; this is a conservative boundary behavior, not evidence of an ordinary-size failure.

Writes are serialized on a dedicated queue, including exit. Nonce waiters resolve in the transport actor independently of its newest-32 presentation event buffer. Replaced snapshots are generally safe because Go retains the pending host request/result in following snapshots until changed. An application consumer falling behind can still miss an intermediate event; request-specific matching plus uncertainty timeouts must remain the rule. Do not infer success from having enqueued a write.

**`action_applied` is not treated as a successful operation by the implemented critical workflows.** Submission clears only on matching `upload_result.request_id`; settings compare `composer_result`; plans compare request ID and canonical plan state; recovery requires matching request ID plus accepted flag; receipts require a matching check result. Exact action decisions wait for Go's pending review to change; the UI never claims delivery from that change. Host bootstrap/control inspect their specific result identities/errors. Focus is special: Go emits state without an ack, and Swift waits for foreground projection. Tests explicitly send wrong request IDs and ack-only outcomes. Stop has no matching-result deadline; R04 records the limitation.

Process launch directly executes an executable URL, without a shell. Configuration supplies cwd/env; inherited managed host flags are stripped and rebuilt, and native uploads receive a distinct root. Environment paths are trusted local configuration, not secrets to print. Working directory creation is synchronous small filesystem work on the transport actor. Bundling builds the current Go executable with Swift, so compatibility is established for the current pair, not arbitrary old runtime binaries. There is only a protocol version, not a capability handshake or executable-hash pairing check at launch. The runtime override is an explicit local development trust choice.

The previous `Process.waitUntilExit` hang is correctly removed; the implementation yields while confirming exit. Existing regression tests freshly passed for blocked stdin and external SIGKILL. Shutdown still awaits both reader tasks without a bound **after** the direct process is gone (E03). There is also no independent Process termination observer: EOF controls disconnect. A surviving descendant holding pipes can therefore delay both disconnect and cleanup. No new transport is recommended; repair pipe ownership/draining within the current one.

`fail` closes stdin and sends SIGTERM, retaining the process reference. Explicit shutdown/reconnect performs escalation/reaping; presentation dismissal does not call shutdown. Shutdown confirmation failure is not returned as a typed outcome through BridgeTransport, so callers cannot distinguish it from successful cleanup. This is a separate plausible failure-reporting gap (R05), not an observed kill failure on this machine.

## 6. Security and ownership

Within one coordinator: one cached owner per conversation, maximum four records, maximum three working/capacity-reserved owners. Navigation changes foreground status rather than transferring operation state. Background dispatch checks parent/child/turn/run identity; only recognized origins/activity/ancestry permit direct activity navigation. Go checks retained task origin and live control identity independently (`dialogue_owner.go`, `task_plans.go:208`, `request_recovery.go:64`). Execution leases remain Go-owned (`worksession/requests.go:269`). Native process uniqueness is application-local; it is not a global singleton (E01). Go run exclusivity does not serialize the native draft archive.

Approval identity includes generation, context, conversation, interaction, exact token, raw message index and exact result equality. Retired tokens cannot become eligible from an older snapshot. Plan review includes revision/digest, captured summary/scope and a foreground epoch; transitions invalidate stale reviews. Recovery binds full recovery state/token and positional account choices; choosing an account invalidates the access review before any async request is queued. Post-save checks defend owner/capacity changes during persistence. The fresh suite includes Go rejection independent of native UI guards.

Pending and uncertain client identities survive restart as uncertainty; reconnect never sends a message/approval/authorization. Explicit admission retry retains the same original ID and Go checks retained text/options/uploads. Ordinary sends do not set `queue_only`. Go authority must continue to distinguish queued work and recovery preparation from execution. No automatic replay was found in the inspected native paths.

Views can call public owner methods (including the legacy single-owner navigation methods), so access control is a compile-time guard against raw host frames, not a sandbox around malicious application code. GatorMac currently uses coordinator-mediated managed navigation. Preserve this route for new features; do not introduce raw command escape hatches. A disabled button is not the authorization check.

## 7. Persistence correctness

Native archives are separate from terminal drafts. Profile filename is SHA256 of the resolved state-root profile; owner keys distinguish conversation/workspace contexts. Optional fields preserve versioned older native archives, and current loads conservatively downgrade unresolved/admitted in-flight operations to uncertainty. They retain request IDs, exact recovery/plan/receipt identities, immutable upload references, correction target and draft revision. There is no automatic terminal-draft migration. Core tests verify distinct profiles, malformed archive preservation, uncertainty, first-workspace promotion and mode/version boundaries.

Save writes an exclusively created mode-0600 temporary file, synchronizes it and atomically renames it; the containing directory is created with mode 0700. This protects complete-file publication within one writer; it does not make read/merge/write transactional between actors/processes (E01). Existing directory permissions are not repaired. Parent-directory fsync/power-loss durability and hostile ancestor replacement were not established by these tests; do not call atomic rename a power-failure guarantee.

A corrupt archive is kept unchanged and editing/sending are disabled; safe quit is allowed if it never loaded. A later save failure leaves edited text in memory, exposes `persistenceIssue`, blocks sends and ordinarily blocks Quit. **The exception is a new edit after the closing flush, E02.** The size limit is checked after loading all Data (E06). Text-only drafts are capped to 32 recent entries/seven days; attachments and pending identities are pinned up to 128. That is an existing retention policy, not a new defect just because it prunes old drafts. Native currently lacks a retention-management UI; the parity report records that separately.

## 8. Attachments and file safety

NativeUploadStore has a strong implementation for the current scope: opaque random 128-bit directory IDs; exclusive descriptor-relative creation; owned private root; no-follow root/child/file opens; regular/nonempty/type/size/count/total checks; streamed 64 KiB copying; input size/mtime check; fsync; SHA256 verification before send; atomic batch rollback. Source paths are not sent as arbitrary filesystem authority. Root parents resolve macOS aliases, while root/children reject symlinks. This is not a defense against an already malicious same-user process rewriting all state.

Explicit picker selections balance security-scoped access in the upload actor. Clipboard text wins over pixels unless Paste Image is explicit; file URLs are not treated as images. Drop URL loading has byte/count limits and captures an import request. Final imports check captured generation/context rather than following the newly selected owner. New-conversation selection can complete a valid import in its original background owner without leaking it to the foreground. Cancellation/replacement retains unused private copies conservatively. Late thumbnail work may finish after view cancellation, but reads immutable ID-bound bytes.

ImageIO checks complete format and dimensions before thumbnail raster allocation: at most 16,777,216 pixels, width/height ≤16,384; source bytes ≤4 MiB for selected images and ≤16 MiB for clipboard decode. Thumbnails max 160px. Clipboard normalization can allocate a full image within that pixel cap; the limit is bounded, not tiny. NSPasteboard supplies Data before the byte check, so its initial framework allocation is not controlled by this implementation. Symlink, changed-bytes, corrupt-image and bounded-image tests pass. No actual leak was demonstrated.

**Known duplicate-document-basename limitation (E07, P2): broader Go/provider/retained-original identity contract, not a Swift overwrite defect.** Native opaque IDs and hashes are distinct; the ordered `upload_submit.files` retains them. `internal/attachment/uploads.go:67` reduces provider names to `filepath.Base`; `cmd/gator/attachments.go:43` calls shared ValidateLoaded, which rejects equal names (`attachment.go:194`); retained originals also use basenames and ValidateOriginals. Two independent `report.md` selections are safely rejected before inference, preserving the draft and separate immutable bytes. The real-Go regression passed. Renaming user files or removing a validator in Swift is not a coherent correction. Repeated image pastes already use distinct generated names and pass their regression.

Native uploads are intentionally never pruned after selection/removal/admission. That is conservative retained storage, not an in-memory leak. It can grow without a disk quota across sessions; R03 records the need for a future reference-aware lifecycle, not permission to delete copies now.

## 9. Memory and performance

Current headless measurements, debug SwiftPM build on this arm64 host:

| Existing benchmark | Observed result | What it does and does not establish |
| --- | --- | --- |
| `nativePresentationBenchmark`, 7 samples/size | 50/500/2,000 messages: 24,084/240,934/964,934 bytes; decode medians 0.580/6.248/23.921 ms; transcript medians 0.025/0.237/0.906 ms | JSON decode + projection, not SwiftUI layout. Peak runner RSS at print: 45,006,848 bytes; cumulative process peak, not isolated allocation. |
| `nativeStreamingSnapshotHandlingBenchmark`, 5×30 snapshots, 500 retained rows | 2,176,935 wire bytes/batch; median 207.834 ms/batch, 6.928 ms/snapshot | Production framing/state replacement on MainActor with fixture data; no FPS, dialogue-heavy cost or screen responsiveness threshold. |
| `nativeOwnerStartupBenchmark` | 66.275, 49.379, 51.442, 59.825 ms to connected; four idle Go owners RSS 36,496/36,496/37,136/36,624 KiB, sampled CPU all 0.0% | Process-start/readiness and one settled ps sample, not sustained energy or cold filesystem cache. |
| `nativeThumbnailBenchmark` | 12×512px PNGs, 11,116,140 staged bytes; 930,144 thumbnail bytes; 90.758 ms; runner peak RSS 50,479,104 bytes | Bounded ImageIO encode/decode batch; no full application memory profile. |

The transcript optimization already caches messages and only rebuilds joined transcript when its raw inputs change. Raw and derived Swift value storage can share buffers; do not sum serialized sizes and call that actual resident memory. Lazy transcript rows and equatable message rows reduce unnecessary body work. MainActor still performs equality/projection and several computed properties repeatedly scan snapshot history (`recovery`, `activity`, plan scope). Their long-session/large-dialogue costs are not isolated by the no-dialogue benchmark. Optimize only after measuring those workloads.

No actual memory leak was established. Weak transport/event callbacks avoid a straightforward owner-retain cycle. WindowFocusObservation unregisters on deinit/window replacement. NativeApplicationPresentation holds windows weakly. Notch → panel → hosting view → notch is a real ownership cycle, intentionally application-lifetime and broken by `stop()` clearing panel; it is not evidence of an accumulating leak in the current singleton lifecycle. Pending task/stream readers may retain resources until explicit cleanup, and E03 shows cleanup can be delayed indefinitely.

Unbounded lifetime structures: retired approval-token set, planReviews/recoveryAccess dictionaries (per owner), and native upload disk storage. Owner count, attention rows (64), stream buffer (32), diagnostics (4 KiB), archive records and attachment count are bounded. Distinguish these finite retained caches from the per-conversation structures without explicit eviction. No hours-long soak, Instruments allocation/energy run, TSan or leak tool was run.

## 10. SwiftUI/AppKit, scene and accessibility quality

Shared `@State` coordinator lifetime above both scenes is correct; environment references bind to selected owner. Composer identity includes context and generation, replacing NSTextView's IME/undo state on owner change. Coordinator.parent is refreshed in updateNSView. The input-context replacement test passes and demonstrates that the old editor cannot edit the new owner. Native NSTextView owns selection/undo/plain editing; token-based focus requests do not refocus on every streamed snapshot. Observer cleanup is explicit. A custom NSTextView is justified by IME/key/drop behavior; a forced replacement with generic TextEditor is not recommended.

Exact decisions combine SwiftUI focus/AX behavior with an NSButton adapter; no default Return shortcut. Current unit tests cover semantic key rules and AppKit input; historical actual keyboard acceptance verifies focused Return did not decide and explicit Space declined the inert original-token approval. Full physical IME, VoiceOver and Full Keyboard Access are still separate operator checks. The two layers of focus/keyboard handling are a brittle integration surface deserving maintained component tests, not an established duplicate-action defect: guards retire/deduplicate decisions.

Menus/controls have accessible labels; exact material text is literal/selectable, with complete disclosures. Announcements avoid token deltas. Reduce Motion disables transitions and replaces ongoing progress with static feedback. Dynamic type/contrast/large-text layouts and screen-reader spoken behavior were not physically verified here. No extra GUI automation was run; GatorMacTests instantiate small AppKit components/windows headlessly as part of the requested suite.

macOS 15 availability is checked by compilation targeting 15.0. Local SDK SwiftUI interface explicitly marks `defaultLaunchBehavior` available at macOS 15 (`GatorApp.swift:29`); scroll APIs similarly compile at the declared target. This is API validation, not a macOS 15 runtime test. The current host runs a newer OS; Intel execution was not tested.

Notch geometry uses real `safeAreaInsets` and auxiliary areas, accepts nonzero/negative screen origins and rejects displays without camera-gap evidence. It chooses the first eligible NSScreen and centers on the display midpoint, assuming symmetric built-in camera placement. No physical multi-display/Spaces/hotplug/fullscreen configuration was tested here. Existing geometry tests cover symmetric and negative-origin fixtures, not off-center camera gaps. The panel cannot become main, hover does not make it key, and explicit entry remembers a weak previous window. Keep these constraints; consider display policy only when an actual multi-display requirement is defined.

## 11. Errors and code quality

Error presentation distinguishes configured model readiness from actual provider access; admission, accepted work, external outcome uncertainty and read-only check results are distinct. Core/Go failures usually reach visible owner/coordinator issues. Draft/import failures preserve text/selections. Full diagnostic text is selectable, avoiding the prior two-line-only disclosure problem.

`try?` is appropriate for JSON union probes, literal Markdown fallback, best-effort handle cleanup, optional thumbnails and sleep cancellation followed by an explicit cancellation guard. The unguarded polling loops are R01. Production runtime has no `try!`, `as!` or fatalError call. Force unwraps in the archive nil-initialization path, validated optional reply IDs, sorted dictionary keys and nonempty unsafe buffers follow adjacent invariants. The brand system-symbol fallback still force-unwraps a system image; this is a low-impact fallback invariant, not a demonstrated crash. Do not generate an issue for every unwrap.

Potentially sensitive subprocess stderr is capped and displayed locally, not sanitized. NativeInputTrace logs fixed metadata only, gated by DEBUG/environment and capped to 64 entries. No secrets were observed or copied into this audit. This does not establish a guarantee that every future backend diagnostic is redacted.

Dynamic JSONValue is appropriate for exact literal previews and compatible display envelopes. Its default-empty accessors make misspelled/missing fields silent, which is a maintenance risk for protocol-critical fields; incremental typed envelopes for operation-specific result identity/errors would be useful. Do not turn all display payloads into a giant rigid Codable schema. Public mutable operation DTOs are not Go authority, and native input never grants execution simply by constructing a DTO.

Files are compact, sometimes excessively compressed onto single lines. Default SwiftLint reports 886 diagnostics (201 error-level, 685 warning-level), predominantly 744 line-length reports. SwiftFormat default lint rejects 31 of 33 files. These are **unconfigured tool defaults**, not a failed repository-defined style gate and not 886 engineering defects. No automatic formatter ran. E08 records a real stale-copy inconsistency rather than generic style advice.

## 12. Test quality and current verification

### Complete Swift test inventory

Counts are `@Test` declarations; parameterized argument cases are additional executions within their tests. All are included in the observed 133-test result.

| File in `macos/Tests/` | Count | Main evidence / boundary |
| --- | ---: | --- |
| `GatorCoreTests/ApplicationStateTests.swift` | 10 | Mock transport; ack/result separation, newer drafts, settings, approval, disconnect/reconnect and corrupt store. |
| `GatorCoreTests/BridgeProtocolTests.swift` | 5 | Byte-split UTF-8/coalescing, malformed/version/size/EOF, outbound admission and literal JSON. |
| `GatorCoreTests/GoIntegrationTests.swift` | 5 | Real Go stream/restart/reconnect; actual Node subprocess transport; blocked stdin; killed-owner regression. |
| `GatorCoreTests/ManagedGoIntegrationTests.swift` | 1 | Real Go independent work and retained child results. |
| `GatorCoreTests/ManagedSessionTests.swift` | 12 | Mock SessionFixture: serialized transitions, host sequence dedup, origin/run controls, capacity, uncertainty, owner isolation. |
| `GatorCoreTests/NativeAttachmentDisconnectTests.swift` | 1 | Actual Go lost-admission identity and no replay. |
| `GatorCoreTests/NativeAttachmentDraftTests.swift` | 8 | Mock owners + real filesystem; imports, stale context, versions and promotion. |
| `GatorCoreTests/NativeAttachmentIntegrationTests.swift` | 3 | Real Go files-only/document/image admission and durable initial owner. |
| `GatorCoreTests/NativeClosureAcceptanceTests.swift` | 2 | Real Go equal-basename safe rejection; headless state-stream benchmark. |
| `GatorCoreTests/NativeDraftStoreTests.swift` | 2 | Actual private files, profile isolation, uncertainty and corruption preservation. |
| `GatorCoreTests/NativeInertApprovalIntegrationTests.swift` | 1 | Actual Go inert web review, owner/draft/background-work isolation. |
| `GatorCoreTests/NativeMessageContentTests.swift` | 1 | Native Markdown/code whitespace. |
| `GatorCoreTests/NativePerformanceTests.swift` | 3 | Existing decode/projection, real process startup and thumbnail measurements. |
| `GatorCoreTests/NativePlanAuthorizationTests.swift` | 13 | Mock authorization state machine, wrong identity/results, capacity, post-save checks, restart reconciliation, retention. |
| `GatorCoreTests/NativePlanIntegrationTests.swift` | 4 | Actual Go execution once, revised review, approved restore without start and Go rejection independent of native guards. |
| `GatorCoreTests/NativeRecoveryIntegrationTests.swift` | 3 | Real interrupted plan and exact receipt/Sent checks against inert local service; foreign identity rejection. |
| `GatorCoreTests/NativeRecoveryTests.swift` | 12 | Recovery presentation, distinct evidence, exact bundle identity and mock receipt uncertainty/retention. |
| `GatorCoreTests/NativeReviewIntegrationTests.swift` | 2 | Real Go action correction and revised plan ownership. |
| `GatorCoreTests/NativeTaskReviewTests.swift` | 14 | Mock task/correction/approval/plan reads, literal public content, focus/generation and decision semantics. |
| `GatorCoreTests/NativeUploadStoreTests.swift` | 6 | Real filesystem copies, permissions, symlinks, hashes, dimensions/limits, repeated paste names. |
| `GatorCoreTests/RecoveryPreparationIntegrationTests.swift` | 4 | Actual Go failure/access/retry preparation, positional account/token boundaries, no execution before new plan approval. |
| `GatorCoreTests/RecoveryPreparationTests.swift` | 10 | Mock eligibility, selection invalidation, late/wrong result, capacity and explicit-decision freshness. |
| `GatorMacTests/NativeInputTests.swift` | 9 | Real small AppKit objects/pasteboards/windows; key/paste/IME APIs, owner editor replacement, contrast and window references. |
| `GatorMacTests/NotchGeometryTests.swift` | 2 | Pure camera-gap/negative-origin geometry. |

Mocks are not the only security evidence: the full suite runs current Go processes with isolated local config, fixture credentials and loopback providers. Several fixtures seed canonical journals using existing helper Go programs. Trace assertions check lack of execution where only review/preparation was requested. No remote provider/account action is necessary to validate these transitions.

Important gaps: E01 concurrent writers; E02 write/edit after closing flush; E03 inherited output handles; E04 eventual receipt recovery; E05 terminal-evidence retirement. Existing tests intentionally establish uncertainty/retention but do not establish an eventual usable path. Missing other cases: slow/no-completion item-provider drop; permission/full-disk failures during save; archive read before size guard; cancellation of wait loops; near-frame-limit coalescing; process shutdown confirmation failure; real display/focus/AX settings; hours-long session growth.

Some names overstate their assertions: `tabLeavesComposerAndShiftTabReturnsWithoutInsertingText` manually makes the editor first responder before testing Shift-Tab; it does not demonstrate an actual button-to-editor reverse round-trip. `imagesDecodeBoundedly...` changes PNG dimension bytes without repairing the PNG checksum, so rejection alone does not isolate the pixel-limit branch. The owner-switch test asserts no edits reach B but not that A's retained callback receives the committed old composition. These are useful tests, with bounded claims. Polling has 3–12 second timeouts, Node port reads and fixture Process waits can hang if startup breaks, and fixed 20/30ms negative-assertion sleeps can be load-sensitive. No flake occurred in this sequential run. No redundant security tests should be removed merely because mock/Go variants cover similar transitions: they exercise different boundaries.

### Exact commands and outcomes

All commands ran at repository root unless stated. Full logs stayed in `/tmp/`, separate from the other audit's tracked log.

```sh
go build -o /tmp/gator-swift-audit-runtime ./cmd/gator
GATOR_TEST_BINARY=/tmp/gator-swift-audit-runtime swift test --package-path macos --scratch-path /tmp/gator-swift-audit-build --no-parallel
swift build --package-path macos --scratch-path /tmp/gator-swift-audit-build -Xswiftc -strict-concurrency=complete -Xswiftc -warnings-as-errors
go test ./cmd/gator ./internal/workapp ./internal/attachment ./internal/worksession -run 'Test(UIBridge|BridgeUploads|DesktopApprovalCannot|ExecutionOwnerIsExclusive|DialogueAdmission|.*TaskPlan.*Reject|.*RequestRecovery.*|.*Upload.*)' -count=1
node --test app-host/tests/backend.test.mjs app-host/tests/sessions.test.mjs app-host/tests/drafts.test.mjs app-host/tests/uploads.test.mjs app-host/tests/policy.test.mjs
swiftlint lint --quiet macos/Sources
swiftformat --lint macos/Sources
plutil -lint macos/Resources/Info.plist
bash -n macos/scripts/build-dev-app.sh
```

- Go runtime build PASS. Sequential Swift suite PASS: 133, zero failures, 38.762s; test build 24.17s on this fresh scratch tree. No skipped Swift tests reported.
- Explicit strict build PASS in 8.96s; no compiler warnings/errors; language mode unchanged.
- Selected Go tests PASS in cmd/gator 0.627s, workapp 1.104s, attachment 0.754s, worksession 1.217s. Regex selects contract tests, **not full Go-package or repository coverage**.
- Selected host tests PASS: 63, zero fail/skip, 130.741ms reported by Node. Runtime-version caveat above. Full terminal/host/Go suites were not required or run.
- SwiftLint exits 2 for its default violations; SwiftFormat exits 1 (31/33 need default formatting), with a no-Swift-version-configured warning. An initial `swiftlint lint --quiet --path macos/Sources` failed because installed 0.65.1 uses positional paths; the positional rerun above completed inspection. These are explicitly failed optional style probes, not silently omitted checks.
- Plist and shell syntax PASS. No development app was rebuilt/launched or signed in this audit.
- Additional scratch probes compile production Core directly under Swift 6; E01/E02/E03/E04/E05 results below. The first receipt-probe edit had a bracket syntax error in the temporary harness; corrected scratch harness compiled, and the receipt/history probes then ran. Production sources were never edited to make probes pass.

## 13. Scored assessment

Scale: 1 unsafe/unsustainable, 2 material defects, 3 adequate with meaningful gaps, 4 strong with bounded issues, 5 unusually complete verified evidence. Scores apply to current native scope, not projected parity.

| Dimension | Score /5 | Concrete evidence |
| --- | ---: | --- |
| Architecture quality | 4 | Correct dependency/authority direction, one shared coordinator, typed Core methods, no third-party Swift stack; operation lifecycle inconsistency warrants focused decomposition. |
| Swift concurrency safety | 3 | Swift 6 complete checking passes; actor/generation/revision guards strong; reentrant Quit and cancellation/I/O lifetime gaps remain. |
| Lifecycle reliability | 2 | Existing blocked-input/killed-process fixes freshly pass; late-edit successful-Quit and inherited-pipe drain problems reproduced. |
| Security boundaries | 4 | Exact Go token/revision/run/access/lease checks independently integrated; no native bypass found. New host/UI features must preserve these boundaries; multi-instance archive loss is separate serious state integrity risk. |
| Persistence correctness | 2 | Atomic 0600 saves/profile/owner migration and uncertainty covered; cross-writer logical corruption, closing transaction hole and post-allocation size check remain. |
| Code maintainability | 3 | Small dependency footprint and 33 coherent files; repeated manual operation states, long compressed control flow and terminal-history-as-live-blocker semantics impede growth. |
| Performance | 3 | Bounded benchmarks are reasonable; no measured leak; heavy decode is off MainActor but long-session recovery/projection/retention and GUI rendering remain unmeasured. |
| Test quality | 4 | 133 passing tests with meaningful real-Go/subprocess/negative authorization evidence; missing newly reproduced failure classes and physical acceptance keep it below 5. |

## 14. Ranked issue register

Severity follows the requested categories. “Confirmed” means code flow or bounded probe establishes the condition, not that a real user's private content was damaged. P0 E01 is **logical durable-record corruption under independent concurrent writers**; no external execution was involved. No speculative authority bypass is classified as confirmed.

### Confirmed defects and current limitations

#### E01 — P0 — Concurrent native archive transactions silently lose unrelated durable records

- **Location:** `macos/Sources/GatorCore/NativeDraftStore.swift:83` (read/merge/write), `:149` (publish); `macos/Sources/GatorMac/GatorApp.swift:70` (same profile chooses same filename).
- **Evidence:** 16 independently instantiated stores wrote distinct conversation drafts concurrently to one profile: all 16 returned success, but only **2** records survived. JSON remained valid. One shared actor serializes the current coordinator; no cross-instance lock or native single-instance admission was found.
- **Interleaving:** A reads archive X; B reads X; A merges owner A and renames X+A; B merges owner B into its old X and renames X+B. A's unrelated draft/operation records disappear. Separate native processes are not isolated by Swift actors. Lost pending identities would be more serious than the synthetic text-only sample, but that extension was not executed.
- **Verification:** scratch `writers` probe in Appendix; 16 writers is below both retention caps, so eviction is not the explanation.
- **Correction:** serialize profile archive read/merge/write with a process-safe lock, or enforce and verify one native writer per profile before loading mutable state. Prefer the lock if independent app instances are supported. Keep per-owner merge and atomic rename inside the transaction.
- **Benefit:** retains unrelated drafts and operation identities under supported independent writers. **Effort:** MEDIUM (1–3 engineering days plus subprocess regression). **Confidence:** HIGH, reproduced. Scope requires multiple stores/instances; normal one-coordinator operation is already serialized.

#### E02 — P1 — Quit can report success after accepting a new draft that failed to save

- **Location:** `GatorApplicationState.swift:90`, `:320`, `:343`, `:484`; `ManagedSessionCoordinator.swift:498`; `AppDelegate.swift:51`.
- **Evidence:** scratch transport held shutdown after the initial flush. `canEditDraft` remained true. A new edit was made while a disposable filesystem obstruction forced its save to fail. `flushDrafts()` returned false, `persistenceIssue` was visible, but the in-progress `shutdown()` still returned **true**. AppDelegate terminates on that boolean.
- **Interleaving:** close flushes save A → awaits transport shutdown → MainActor accepts edit B and schedules save B → B fails → transport exits → shutdown returns success without rechecking B. `canEditDraft` does not depend on closing status, and the coordinator's stopping flag disables sends/navigation, not editor mutation.
- **Verification:** `late-edit` probe below; real private profiles untouched. The probe proves false-success and unsaved accepted input; actual NSApp termination was inferred directly from AppDelegate, not triggered.
- **Correction:** make closing an explicit mutation barrier before the final flush; invalidate imports/edits consistently, await the final revision's save, and only return success if that closing transaction is durable. If editing is deliberately allowed while closing, drain/recheck the latest revision before terminating. Restore usable state when closing aborts.
- **Benefit:** honest Quit outcome and preservation of accepted new input. **Effort:** MEDIUM (1–2 days + deterministic interleaving test). **Confidence:** HIGH.

#### E03 — P1 — Shutdown/disconnect remain dependent on unbounded inherited-pipe EOF

- **Location:** `GoBridgeConnection.swift:76`, `:87`, `:97`, `:195`.
- **Evidence:** an inert child spawned an 8-second descendant inheriting stdout/stderr, then exited on the normal `exit` frame. Shutdown returned after **8.027s**, waiting for the descendant rather than the direct child's exit. No drain deadline follows the process escalation loop. A descendant that never closes those descriptors would keep the await pending [Inference].
- **Interleaving:** reader blocks in POSIX read → direct child exits → Foundation sees direct process gone → shutdown passes process-exit checks → awaits reader tasks → descendant keeps pipe writer open. Since disconnect is driven by stdout EOF, unexpected owner death can also remain invisible until EOF.
- **Verification:** `descendant` probe; both subprocesses are disposable and terminate naturally. This confirms the transport condition, not a claim that a particular current Go tool leaks handles in normal operation.
- **Correction:** bound output/error draining independently of direct-process exit; coordinate descriptor/reader termination safely, observe process termination independently, and propagate cleanup failure. Avoid `waitUntilExit` and avoid reintroducing synchronous cooperative-executor reads. Do not kill unrelated processes based solely on stale PIDs.
- **Benefit:** bounded cleanup/reconnect and truthful owner death detection when pipes outlive the owner. **Effort:** MEDIUM (2–4 days with blocked-read/write/descendant regressions). **Confidence:** HIGH for reproduced transport hole; production tool-specific occurrence unverified.

#### E04 — P2 — A persisted uncertain receipt operation has no native reconciliation path

- **Location:** `NativeDraftStore.swift:73`, `NativeRecovery.swift:46`, `GatorApplicationState.swift:784`, `:819`.
- **Evidence:** restored receipt phase is `uncertain`; every future receipt request is refused while unresolved. Result consumption requires `pending.generation == generation`, which cannot hold for the restored old generation. No receipt reset/reconciliation method exists. A fresh-generation scratch snapshot with the original matching result did not clear it; even **Inspect Saved Check** stayed blocked.
- **Failure scenario:** disconnect/restart during local receipt inspection or read-only Sent check permanently prevents later native checks in that owner, despite copy telling the user to reconnect and inspect. It also pins the cached owner.
- **Verification:** `receipt` probe and existing `receiptDisconnectPersistsUncertainty...` test, which verifies no replay but stops short of eventual recovery.
- **Correction:** preserve the old identity/evidence while allowing an explicit fresh canonical *inspection* to reconcile/retire the client operation. Keep Sent lookup explicit and distinct from inspection, and never resend the original external action.
- **Benefit:** recoverable uncertainty without weakening no-replay behavior. **Effort:** MEDIUM (1–3 days). **Confidence:** HIGH, reproduced state/guard behavior.

#### E05 — P2 — Historical failures pin owner capacity as if they were live unresolved work

- **Location:** `NativeRecovery.swift:67`, `:94`; `ManagedSessionCoordinator.swift:139` and `:149`.
- **Evidence:** projection considers any old failed/review/interrupted activity unresolved forever. Scratch snapshot with idle current work, one old failed message and a later completed answer still yields `unresolved=true`. The coordinator forbids disposing any such owner. It is based on display-status strings rather than canonical outstanding controls/effects.
- **Failure scenario:** four conversations with retained past failures fill all cached slots; a new independent chat is refused even when none is working or has a pending authorization. Existing tests deliberately establish retention for unknown receipts, but do not distinguish a resolved historical failure.
- **Verification:** `HISTORY idleWithOldFailureUnresolved=true`; inspect `disposable` guard. This is a confirmed conservative capacity limitation, not proof that all failures are safe to evict.
- **Correction:** separate historical evidence from live/uncertain ownership blockers; allow deliberate retirement only after Go/canonical state establishes safe closure. Keep durable failure evidence on disk and never cancel pending work to free a slot.
- **Benefit:** continued navigation after ordinary terminal failures, without deleting evidence or loosening active-work protection. **Effort:** MEDIUM (2–4 days, owner/capacity tests). **Confidence:** HIGH for current classification/block; safe retirement policy needs design.

#### E06 — P2 — Archive size guard runs after whole-file allocation

- **Location:** `NativeDraftStore.swift:55`.
- **Evidence:** `Data(contentsOf:)` reads the complete file before checking 6 MiB. A corrupted/externally enlarged archive can allocate far beyond the advertised read limit before rejection.
- **Failure scenario:** large same-profile archive causes startup memory pressure even though decoding eventually fails closed. No OOM or large-allocation benchmark was induced.
- **Verification:** direct source ordering; test currently covers invalid bytes/version, not a bounded read. **Correction:** descriptor-based bounded read (limit + 1) and regular-file/size validation before allocating, handling growth between stat/read. **Benefit:** actual bounded intake. **Effort:** SMALL (<1 day + test). **Confidence:** HIGH for unbounded read; impact size unmeasured.

#### E07 — P2 — Equal document basenames cannot be admitted together

- **Location:** `internal/attachment/uploads.go:67`, `internal/attachment/attachment.go:194`, `cmd/gator/attachments.go:43`; native path `BridgeProtocol.swift:201` correctly retains IDs.
- **Evidence/scenario:** two distinct opaque IDs ending in `report.md` pass Swift staging but fail Go duplicate provider-name validation. Real-Go test passes for safe rejection/no inference/draft retention.
- **Verification:** `duplicateDocumentBasenamesPreserveDistinctBytesAndFailGoAdmissionWithoutExecution` in the full suite. **Correction:** coherent backend/provider/retained-original identity contract with unique identity separate from display basename. Do not bypass validators or silently rename originals in Swift. **Benefit:** selecting same-named independent documents safely. **Effort:** MEDIUM/LARGE across contracts. **Confidence:** HIGH. Broader contract limitation, not a Swift copy defect.

#### E08 — P3 — Unsupported-workflow strings lag the managed implementation

- **Location:** `BridgeProtocol.swift:111`, `GatorApplicationState.swift:111`, `ConversationView.swift:294`.
- **Evidence/scenario:** snapshot text says managed sessions are unavailable although the coordinator implements them; another message says conversation-during-work arrives later. Most managed paths suppress those notices, but connection details can show the generic snapshot notice directly. This confuses diagnosis/parity interpretation during transient dispatch.
- **Verification:** source conditions/copy; no GUI reproduction claimed. **Correction:** capability-aware copy based on current managed projection; retain honest queue/external-handoff limitations. **Benefit:** accurate user-facing diagnostics. **Effort:** SMALL. **Confidence:** HIGH for contradictory strings; transient visibility source-derived.

### Plausible risks / optional improvements (not confirmed production defects)

| ID / priority | Exact location, evidence and specific scenario | Verification boundary and recommendation | Benefit / effort / confidence |
| --- | --- | --- | --- |
| R01 / P2 risk | `GatorApplicationState.swift:667,781,871`: polling ignores cancellation errors. If an executing MainActor waiter is canceled while its pending condition remains true, Task.sleep throws immediately; the non-suspending loop can starve the MainActor deadline/result task that would clear it. | No current caller cancels the private coordinator transition handle, so reachable production cancellation was **not confirmed**. Honor cancellation or use an explicitly noncancelable, event-driven application-owned waiter with bounded termination. | Avoid future cancellation starvation; SMALL/MEDIUM; high confidence in canceled-loop behavior, medium in product exposure. |
| R02 / P2 risk | `NativeAttachmentActions.swift:47`: item-provider continuation has no timeout/cancellation escape. A provider never invoking completion leaves attachmentImport active and sending unavailable in that owner. | Misbehaving provider not induced. Add owned cancellation/timeout that invalidates import identity; handle any late completion once. | Recover stuck imports; MEDIUM; medium. |
| R03 / P2 improvement | `GatorApplicationState.swift:329`; `NativeUploadStore.swift:37`; `GatorApplicationState.swift:43,27,31`: one full save per edit, disk copies never pruned, owner caches have no lifetime cap. | No measured memory leak or sustained disk/backlog problem. Measure burst edits/long sessions; coalesce unsent draft saves while retaining immediate durable pre-send operation writes. Design reference-aware upload maintenance only with Go/journal ownership evidence. | Bound future retained growth/I/O; MEDIUM; high source confidence, impact unmeasured. |
| R04 / P2 risk | `GatorApplicationState.swift:437`: successful transport ack with no cancelling/terminal state leaves `stopping=true`; no stop-result deadline. | Not reproduced with real Go. Add a bounded “stop outcome unconfirmed” presentation without automatic repeat; never call ack cancellation success. | Avoid misleading permanent Stopping state; SMALL; medium. |
| R05 / P2 risk | `GoBridgeConnection.swift:191`; `GatorApplicationState.swift:492`: unconfirmed process exit only calls fail; transport shutdown returns Void and owner unconditionally marks closed/success. | SIGKILL confirmation failure not reproduced. Return a typed cleanup result and propagate it through coordinator/Quit. Keep old owner retained when exit is not confirmed. | Honest cleanup ownership; SMALL/MEDIUM; high source confidence, low observed incidence. |

No stylistic P3 backlog is manufactured from unconfigured analyzer defaults. E08 is the only registered P3 issue.

## 15. Concrete architectural improvements, ranked

1. **Make the archive transaction exclusive across independent writers (E01).** The demonstrated problem is loss of unrelated durable records; a transaction lock/single-writer policy solves that without a database or a new store layer.
2. **Define one owner closing transaction (E02/E03/R05).** Capture a closing revision, gate new mutations and coordinate draft/transport cleanup outcomes. The demonstrated problem is success being reported before all accepted state is durable/cleaned up. This belongs in owner/coordinator lifecycle, not in a visually disabled Quit button.
3. **Extract focused receipt/recovery/plan operation state reducers from the owner after defining reconciliation (E04).** Repeated manually managed phases, deadlines, persistence and result matching already diverge. Keep semantic differences explicit, retaining one owner and one shared draft store; do not introduce a universal generic executor or per-feature actor hierarchy.
4. **Separate canonical live owner blockers from recovery presentation (E05).** Current cache retirement depends on human-readable historic status strings. A small typed blocker projection, grounded in Go pending/uncertain identities, would allow history rendering to evolve without unexpectedly exhausting execution-owner slots.
5. **Add deterministic failure seams/tests at the persistence/lifecycle boundary.** Inject a small save/close gate or controlled filesystem fixture so tests can pause after flush, fail a save, hold inherited pipes and run independent writer processes. Do not wrap every production dependency in a protocol; target the failures the present tests cannot orchestrate.

Incremental follow-ups: typed result identity/error envelopes while keeping raw literal review content; explicit application-owned timeout/task cleanup; bounded archive read. These are lower-risk coherent diffs. No production changes were made here.

## 16. What should remain unchanged

- Go as sole execution/permission/replay authority, retained execution leases and identity/token checks.
- One coordinator shared by menu/full/notch presentations; one state/stream per original owner.
- Internal finite host commands, typed public intents, exact action/plan/recovery review semantics and no automatic replay.
- Existing NDJSON transport and bundled Go executable; no HTTP daemon, alternate agent loop or new transport.
- Native archive namespace/profile partition and terminal compatibility; current safe corrupt-store preservation.
- Immutable private upload staging and conservative retention until reference-aware cleanup is designed.
- Raw Go message indexes separate from joined dialogue presentation, literal exact reviews and explicit Return-resistant decisions.
- Existing AppKit editor/window bridges, which serve concrete native input/lifecycle requirements.
- Current standard controls/materials and measured transcript optimization; no UI framework or persistence migration.

## 17. Evidence gaps and operator-only checks

No access to a macOS 15 runtime or Intel execution was used; I cannot verify those runtime behaviors. Compiler deployment-target validation passed. Physical VoiceOver, real IME, Full Keyboard Access variants, increased contrast/text sizes, display hotplug/Spaces/fullscreen/notch placement and sustained energy/long-session performance remain unverified. Historical screenshot/AX/operator reports retain their original scope; they do not replace these checks.

No real account sign-in, provider inference, mail/Sent lookup, browser/computer control, schedule/service change, backup/delete/restore or other external action ran. Loopback providers and fixture Go actions establish protocol behavior only. No signing/notarization assessment was performed. No code-coverage percentage was measured. Full Go, host and terminal suites were intentionally not rerun; only contracts relevant to this audit were selected. Optional lint failures are disclosed above. External worktree activity means this report identifies its exact stable source scope rather than claiming exclusive workspace ownership.

## 18. Readiness for substantial native feature development

**Maintain and extend the current architecture after closing E01–E03 and defining the eventual recovery/retirement behavior in E04–E05.** These issues concern durability and application ownership, so they should precede features that add more operations, files or owners. Current small read-only native improvements can be reviewed independently, but the passing suite is insufficient evidence for broad lifecycle expansion. The parity inventory identifies reachable conversational workflows, concrete native gaps and independently selectable candidates. No next tranche or feature implementation is selected by this audit.

## Appendix — bounded reproduction evidence

The probes compiled **unchanged production GatorCore sources** with an isolated `@main` harness. Temporary output only; no test files or package manifests were changed:

```sh
swiftc -swift-version 6 -parse-as-library macos/Sources/GatorCore/*.swift /tmp/gator-swift-audit-probes.swift -o /tmp/gator-swift-audit-probes
python3 /tmp/gator-swift-audit-run-probes.py
/tmp/gator-swift-audit-probes receipt /tmp/gator-audit-receipt-probe
```

Observed outputs:

```text
writers exit 0 WRITERS successful=16 retained=2 expected=16
late-edit exit 0 LATE_EDIT canEditDuringShutdown=true
LATE_EDIT saved=false quitReportedSuccess=true ... text=new input after initial quit flush
descendant exit 0 DESCENDANT shutdownElapsed=8.026827333 seconds
HISTORY idleWithOldFailureUnresolved=true
RECEIPT phase=uncertain freshGenerationBlocked=A prior check is unconfirmed. Its request identity is retained; no automatic retry. Reconnect and inspect retained evidence.
```

The filesystem failure is created by replacing the scratch draft directory with an ordinary file after the closing flush; the original scratch archive is moved aside. It simulates failure without changing permission settings or user data. The descendant sleeps eight seconds and naturally exits; the controller bounds the probe subprocess to 15 seconds. Concurrent-writer probe uses distinct conversation IDs, all recent, with no attachments/pending entries and below the retention count.

Full reproducible scratch harness (audit fixture, not a proposed production implementation):

```swift
import Foundation

actor AuditGateTransport: BridgeTransport {
    nonisolated let events: AsyncStream<BridgeEvent>
    let continuation: AsyncStream<BridgeEvent>.Continuation
    var shutting = false
    var release: CheckedContinuation<Void, Never>?
    init() { let c = AsyncStream<BridgeEvent>.makeStream(); events = c.stream; continuation = c.continuation }
    func start(_ c: BridgeLaunchConfiguration) { continuation.yield(.state(BridgeSnapshot(.object(["conversation_id": .string("audit"), "mode": .string("chat"), "source": .string("/audit"), "messages": .array([])])))) }
    func send(_ c: BridgeCommand, nonce: String) {}
    func shutdown() async { shutting = true; await withCheckedContinuation { release = $0 }; continuation.finish() }
    func resume() { release?.resume(); release = nil }
}
@main struct AuditProbes {
    static func main() async throws {
        let root = URL(fileURLWithPath: CommandLine.arguments[2]); try FileManager.default.createDirectory(at: root, withIntermediateDirectories: true)
        switch CommandLine.arguments[1] {
        case "writers":
            let results = await withTaskGroup(of: String.self) { group in
                for i in 0..<16 { group.addTask {
                    do { let store = NativeDraftStore(directory: root, profile: "writers"); var a = NativeArchive(); a.drafts["conversation:owner\(i)"] = NativeDraft(text: "owner \(i)"); try await store.saveOwner(a, context: "conversation:owner\(i)"); return "ok" } catch { return error.localizedDescription }
                } }
                var r: [String] = []; for await v in group { r.append(v) }; return r
            }
            let a = try await NativeDraftStore(directory: root, profile: "writers").load()
            print("WRITERS successful=\(results.filter { $0 == "ok" }.count) retained=\(a.drafts.count) expected=16")
        case "late-edit": try await lateEdit(root)
        case "receipt": try await receipt(root)
        case "descendant":
            let t = GoBridgeConnection(); try await t.start(BridgeLaunchConfiguration(runtime: root.appendingPathComponent("fixture.py"), workspace: root))
            var events = t.events.makeAsyncIterator(); _ = await events.next()
            let start = ContinuousClock.now; await t.shutdown(); print("DESCENDANT shutdownElapsed=\(ContinuousClock.now - start)")
        default: break
        }
    }
    @MainActor static func receipt(_ root: URL) async throws {
        let t = AuditGateTransport(), store = NativeDraftStore(directory: root, profile: "receipt")
        let identity = NativeReceiptIdentity(conversationID: "audit", runID: "run-one", actionID: "action-" + String(repeating: "a", count: 16), correlationID: String(repeating: "b", count: 32), manifestSHA256: String(repeating: "c", count: 64))
        var a = NativeArchive(); var pending = NativeReceiptOperation(id: "original-check", nonce: "nonce", identity: identity, generation: UUID(), action: .inspect); pending.phase = "uncertain"
        a.selectedConversationID = "audit"; a.receiptOperationsByContext = ["conversation:audit": pending]; try await store.save(a)
        let app = GatorApplicationState(configuration: BridgeLaunchConfiguration(runtime: URL(fileURLWithPath: "/unused"), workspace: root), store: store, factory: { t })
        await app.start(); while !app.isConnected { await Task.yield() }
        let bundle: JSONValue = .object(["RunID": .string(identity.runID), "ManifestSHA256": .string(identity.manifestSHA256), "Actions": .array([.object(["ID": .string(identity.actionID), "CorrelationID": .string(identity.correlationID), "Operation": .string("gmail_send"), "Status": .string("unknown")])])])
        let historical = BridgeSnapshot(.object(["conversation_id": .string("audit"), "running": .bool(false), "messages": .array([.object(["activity": .object(["state": .string("failed")])]), .object(["text": .string("Later successful answer"), "activity": .object(["state": .string("completed")])])])]))
        print("HISTORY idleWithOldFailureUnresolved=\(NativeRecoveryProjection(historical).unresolved)")
        let next = BridgeSnapshot(.object(["conversation_id": .string("audit"), "messages": .array([.object(["bundle": bundle])]), "receipt_result": .object(["request_id": .string(pending.id), "check": .object(["status": .string("not_checked")])])]))
        app.receive(.state(next), generation: app.generation)
        let receipt = app.recovery!.receipts[0]
        print("RECEIPT phase=\(app.receiptOperation!.phase) freshGenerationBlocked=\(app.receiptUnavailable(receipt, generation: app.generation) ?? "none")")
    }
    @MainActor static func lateEdit(_ root: URL) async throws {
        let t = AuditGateTransport(), directory = root.appendingPathComponent("drafts"), store = NativeDraftStore(directory: directory, profile: "late")
        let app = GatorApplicationState(configuration: BridgeLaunchConfiguration(runtime: URL(fileURLWithPath: "/unused"), workspace: root), store: store, factory: { t })
        await app.start(); while !app.isConnected { await Task.yield() }
        app.editDraft("saved before quit"); _ = await app.flushDrafts()
        let close = Task { await app.shutdown(confirmed: true) }
        while !(await t.shutting) { await Task.yield() }
        try FileManager.default.moveItem(at: directory, to: root.appendingPathComponent("original-drafts"))
        try Data("blocks saving".utf8).write(to: directory)
        print("LATE_EDIT canEditDuringShutdown=\(app.canEditDraft)")
        app.editDraft("new input after initial quit flush")
        let saved = await app.flushDrafts(); await t.resume(); let quit = await close.value
        print("LATE_EDIT saved=\(saved) quitReportedSuccess=\(quit) issue=\(app.persistenceIssue) text=\(app.draftText)")
    }
}

```

Scratch controller:

```python
import os, tempfile, subprocess, json, pathlib
binary='/tmp/gator-swift-audit-probes'
for mode in ['writers','late-edit','descendant']:
    with tempfile.TemporaryDirectory(prefix='gator-audit-'+mode+'-') as root:
        if mode=='descendant':
            script=pathlib.Path(root)/'fixture.py'
            script.write_text('''#!/usr/bin/python3
import os,sys,json,subprocess
subprocess.Popen(['/usr/bin/python3','-c','import time; time.sleep(8)'],stdin=subprocess.DEVNULL)
print(json.dumps({'version':1,'type':'state','state':{'conversation_id':'audit','messages':[]}}),flush=True)
for line in sys.stdin:
    if json.loads(line).get('type')=='exit': break
''')
            script.chmod(0o700)
        r=subprocess.run([binary,mode,root],capture_output=True,text=True,timeout=15)
        print(mode, 'exit', r.returncode, r.stdout.strip(),r.stderr.strip())

```

## Final decision handoff

**A. Top five genuine risks:** E01 concurrent-writer logical archive corruption (P0, independent writers required); E02 successful Quit despite failed late-edit persistence (P1); E03 inherited-pipe shutdown/disconnect wait (P1, synthetic subprocess reproduction); E04 permanently blocked restored receipt operation (P2); E05 historical failure evidence exhausting owner slots (P2).

**B. Top five justified improvements:** exclusive archive transaction; explicit durable owner-closing transaction and bounded cleanup result; domain-specific operation reducers with reconciliation; canonical live blockers separate from recovery display; deterministic persistence/lifecycle failure seams and regressions. These are recommendations, not implemented changes.

**C. Maintainability verdict:** maintainable foundation with appropriate Go/Core/Mac boundaries; correct durability/lifecycle defects before substantial expansion. No wholesale rewrite or new transport/database is justified.

**D. Parity counts:** 37 fully supported / 21 partially supported / 40 missing; 7 intentionally different, zero deprecated; 105 total. Counts describe workflow availability and remain subject to the documented engineering defects.

**E. Feature selection:** use the independently selectable IDs and user-facing table in [TERMINAL-PARITY-INVENTORY.md](TERMINAL-PARITY-INVENTORY.md#6-user-selectable-candidates--no-automatic-implementation-decision). Choosing one row does not select unrelated features. No implementation decision was made.
