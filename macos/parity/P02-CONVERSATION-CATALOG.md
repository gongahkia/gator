# Gator native parity P02 — retained conversation catalog

Date: 2026-11-18. Selected scope: **CH03 — search retained conversations; CH04 — rename a retained conversation.** P01 transcript Find remains intact. This is parity increment P02, not migration Tranche 09.

## Outcome

Implemented a native All Conversations window backed by Go's existing indexed catalog, with Active/Archived/All scopes, opaque server cursors, previous/next/refresh/clear, exact selection/opening and version-checked rename. Catalog work uses an existing managed Go owner without entering its terminal launcher or changing its running conversation. No second catalog database, search index, server or execution engine was introduced.

**Coverage limitation is explicit:** Go currently indexes retained Work request/revision text, titles and folders, not conversation-only dialogue. Native UI states this and preserves incomplete/skipped/index-fallback notices. “All Conversations” is the catalog workflow; it is not a claim of unlimited or exhaustive content coverage beyond Go's existing bounds.

Candidate: `/tmp/Gator-Native-Parity-P02.app`. P01 bundle preserved at `/tmp/Gator-Native-Parity-P01.app`. No commits, README edits, signing, notarization, dependency installation, paid provider requests or live external actions.

## 1. Baseline and discovery

Initial HEAD was `7e5be43d04ae097670cb4581b70267d1a64e1413`. P01 production/test changes and `macos/parity/` were still locally modified/untracked; they were preserved. External commits subsequently captured ongoing work at `97b16d7ae4f24ad720f7bf0fdc44f7da64911d23`, `07675f2c5dac1fc336446116a0cecc5cf38efb94` and the final HEAD recorded in [integrity.json](evidence/p02/integrity.json). This agent created no commits and did not reset or rewrite those external changes.

Read the current P01 report, parity inventory, safety and UI/UX remediation reports. Inspected the requested workapp bridge/catalog/session/panel code, worksession catalog/storage/index, terminal actions/product actions/client/picker, Swift transport/state/coordinator, recents, P01 Find/reading code and existing tests. Used the installed SwiftUI, AppKit interop, concurrency and SwiftPM workflows. No additional tooling was installed.

Verified constraints:

- Recents expose at most 24 nonarchived entries, not the full catalog.
- `ManagedSessionCoordinator` rejects generic `session_request.view` navigation. That rejection and the existing recent/known-ID opening path remain in place.
- The terminal catalog changes launcher/panel state and can emit management-view session requests. Opening that panel inside an executing native owner would couple catalog navigation to its presentation state.
- Explicit Go conversation loading and `SetActive` allow archived records; opening does not call restore/archive mutation.

Host verification: arm64 macOS 26.5.2, Apple Swift 6.3.3, Swift tools 6.1 / Swift 6 mode, Go 1.27.3 and installed Node 26.7.0. Declared/final Mach-O minimum remains macOS 15.0; SDK 26.5. Physical macOS 15 was not tested.

## 2. Existing Go and terminal contracts

| Contract | Authority reused |
| --- | --- |
| `Config.QueryConversations` | Existing `worksession.Store.PageConversationsContext`; indexed query with canonical bounded fallback. |
| `ConversationQuery` | Literal normalized text, Active/Archived/All, `Content: true`, opaque `After`, maximum 50-row page. |
| `ConversationPage` | Canonical items, optional continuation, incomplete/skipped/index-fallback flags. No exact total. |
| `ConversationItem` | ID, display version, title, source, update time, pinned/archived flags and public indexed snippet. |
| `Config.OrganizeConversation` | Existing `ConversationChange{ID, Version, Action:"rename", Title}` and metadata lock/CAS. |
| `ConversationDisplayVersion` | Canonical presentation identity including metadata revision, title and organization flags; independent of ordinary Work head changes. |
| `SessionStore.Load` | Fresh authoritative metadata for selection, inspection and destination validation. |
| Managed `create/select/focus` | Existing one-owner-per-conversation rules, cached/disconnected handling, four-owner capacity and recovery retention. |

Terminal references: `ui/src/gator/actions.tsx` catalog actions and `/conversations`, `client.ts` navigation/panel result handling, and the picker/panel components. Terminal rename binds a reviewed panel identity and row; the underlying mutation is the same Go CAS operation. Native preserves the authority and semantics through typed page/row evidence rather than reproducing the terminal panel visually.

The canonical catalog/storage/index implementation files are unchanged. The existing SQLite index remains disposable; conversation/revision/journal files and presentation metadata remain authoritative.

## 3. Native bridge architecture

Added a finite `native_catalog` frame handled by `ApplyNativeCatalog`, with these operations only:

| Operation | Behavior |
| --- | --- |
| `query` | Asynchronous canonical query with cancellation/supersession; produces request-specific page evidence. |
| `cancel` | Cancels only the matching query ID; an older cancellation cannot cancel a newer read. |
| `open` | Validates page ID, exact item ID/version and fresh canonical metadata/source; returns evidence, never changes the Go owner's conversation. |
| `inspect` | Refreshes the exact selected item's canonical metadata after a conflict; does not rename or open it. |
| `rename` | Calls the existing organization API with the reviewed ID/version and explicit title. |
| `verify` | The destination owner verifies its own ID/version and startup load outcome before native selection. Refreshes its canonical display title only. |

A separate finite `refresh_conversations` message takes a conversation ID and refreshes display metadata directly from canonical Go storage in other cached owners. It does not run the index, reload history, focus an owner or reset review state.

Typed Swift requests and results live in `NativeCatalogProtocol.swift`; `NativeConversationCatalog` holds presentation/query/editor state. It is not another coordinator. The existing coordinator pins a connected catalog origin/generation, serializes open/rename/inspect through its existing transition chain, and performs owner selection. Read queries do not wait behind executing Work or enter a terminal launcher.

Query and action results occupy separate snapshot fields. Swift waits for the exact request ID/action in the original owner's generation. An `action_applied` nonce is **never** search or rename completion. A bridge test checks the acknowledgement arrives before the deferred query/rename command executes and before a canonical title changes.

Requests are bounded, use fresh UUIDs and retain only the current query/action evidence. Go rejects reuse of an active query ID for a changed request and suppresses duplicate pending/completed rename commands. Swift rejects reuse of the latest completed result identity. There is no unbounded request ledger or automatic mutation retry.

## 4. Scope, pages and search coverage

The native window initially uses Active scope, with explicit Archived and All choices. Every query sets `Content: true`. Query/scope changes clear old rows/selection and reset page anchors. Next uses Go's continuation verbatim; Previous uses the saved prior cursor. Refresh preserves the current cursor; Clear starts the empty query on page one. Only the current 50-row page and a bounded cursor stack are retained. Query changes are coalesced for 180 ms; cancellation/supersession rejects late responses.

Footer text says “Page N · X on this page”. It does not infer an exact total from a continuation token or declare a partial search exhaustive. The Go layer still validates that a cursor belongs to the requested query/scope/content mode.

Existing Go limits include 10,000 records, 16 MiB metadata, 32 MiB search reads, bounded per-record body/revision traversal, and its five-second index context. Missing/corrupt/future-schema indexes retain canonical fallback behavior. Incomplete coverage, skipped records, changed/unavailable inputs and fallback flags reach the typed Swift page and visible warning text. Tests cover these paths, including canonical-file fallback and existing future-index-schema regressions.

Search uses Go's literal case-normalized semantics, not model retrieval. Private replay/options/provider configuration are excluded by the existing Go index. Conversation-only dialogue is outside that index; current loaded dialogue remains searchable through P01. Native does not compensate with another index, load unlimited history or add CH07 paging.

## 5. Rename and exact version behavior

Rename requires a real selected page item. The editor displays its title and exact ID, has Save Title/Cancel, and validates the current Go title rule: trimmed nonempty title, at most 256 UTF-8 bytes and no control characters. Go always performs final validation and version CAS under the existing metadata lock.

Save is pending until Go returns the matching canonical result. Only then is the editor dismissed and the confirmed canonical title shown. The page's item/version/identity updates from Go, and cached connected owners receive direct metadata refreshes. The current owner's title and recents update without changing task/run IDs, queued work, history, permissions, workspace or provider settings.

If another client changes the metadata, CAS fails. The attempted title stays in the editor; Save is disabled until explicit **Refresh and Review**. Inspection returns the current title/version while retaining the attempt. The user must choose Save again; inspection never resubmits. The real-Go test reads the store between inspection and Save to prove the concurrent title remains unchanged.

Filtered results keep the confirmed renamed row and surrounding query/page after Save, with an explicit notice to Refresh to reapply the filter. A title change may make the row no longer match the original query. There is no hidden client-side force update or timestamp-wins policy.

Timeout/disconnection leaves a title attempt marked unconfirmed and requiring review; it is not retried. Rename editor state is transient within the running app; no new durable draft/archive schema was added. Composer drafts retain their existing durability.

## 6. Opening non-recent and unavailable results

The new opening path does not call the recent-list selector with an unrecognized ID or relax generic view requests:

1. Capture the selected page ID, exact item ID/version and catalog origin generation.
2. Go verifies that row belongs to its current page and checks canonical metadata/version/source again. Removed/changed rows fail.
3. The coordinator verifies the presentation/selection/page and foreground generation are still current, then uses existing managed selection.
4. Reuse a valid cached owner, or create one through the existing factory. A disconnected cached owner is selected for inspection with explicit reconnect guidance; no second owner or automatic reconnect is started.
5. A connected destination confirms its own identity/version/source and startup history/settings load outcome before focus/selection is published. A new idle failed candidate is shut down and removed; uncertain/recovery state is retained.

A same-current result verifies metadata without unnecessary focus churn. Verification also refreshes a cached canonical title after another client renamed it. A test first reproduced the stale-header case, then passed after this metadata-only correction.

Go records existing restore errors for the destination check. Valid metadata alone cannot turn a failed retained-history/settings load into successful navigation. The real-Go test corrupts a disposable retained lineage after catalog loading and confirms opening fails, the prior owner/draft remains selected, and no extra idle slot remains.

Real-Go evidence: a 310-record catalog opened topic 0005, which was absent from the 24-row recent projection. The native keyboard session used Tab/Down/Return to open that exact older ID; direct Go child count increased from one to two. Repeating Open reused the same two PIDs. Archived topic 0007 opened in integration without changing its archived flag. No restore mutation was added.

## 7. Owner, concurrency and lifecycle safeguards

| Interleaving | Verified behavior |
| --- | --- |
| Query A pending, query/scope B starts, A finishes late | A's task/context is cancelled or superseded; only B can publish its page. |
| An older cancellation arrives after a newer query | Cancellation ID mismatch leaves the newer query running. |
| Foreground owner changes while a global read is pending | Result updates only the catalog; current owner, drafts, focus request and owner count stay unchanged. |
| Catalog closes during open validation | Final presentation/selection checks reject navigation. |
| Destination metadata/history changes after origin validation | Destination check rejects it; only a safely disposable new idle candidate is removed. |
| Terminal rename v2 follows loaded native v1 | Native v1 Save fails; attempted title persists; fresh review plus a new explicit Save is required. |
| Rename overlaps background task completion | Existing Go metadata CAS survives ordinary Work head updates. Main/child work continues; explicit chosen titles remain canonical. |
| Four owners retain recovery evidence | Opening fails with an actionable capacity explanation; none is silently evicted. |
| Cached owner is disconnected | Cached state is inspectable, no new owner/reconnect/replay is created. |

The catalog origin is retained while its presentation/action/review needs it. It counts among the existing four owners; it is not an extra catalog process. Historical owner-retirement P2 limitations remain: retained uncertain/recovery owners can prevent opening another chat, and safe background owners normally retire only under capacity pressure. No broad retirement-policy refactor was made.

Quit keeps the existing synchronous all-owner closing barrier. It cancels catalog reads and waits for catalog actions through the existing transition/lifecycle before final owner flush/shutdown. Pending title confirmation is included in the quit explanation. Go's restart check rejects an unfinished native title mutation. No arbitrary sleep or synchronous subprocess shutdown was introduced.

Archive transactions/conflict detection, request admission, execution leases, approval tokens, plan digests/revisions, recovery authority, attachment storage and no-replay behavior remain intact. Protected source/test hashes are recorded in the integrity evidence; all prior safety tests remain unchanged and pass.

## 8. UI and keyboard behavior

All Conversations is a separate ordinary macOS window, reachable from the full toolbar, Recent Conversations menu, native Conversation menu and compact More menu. Command-Shift-O opens it. Recents retain their bounded label and path. The notch has no catalog UI.

The native list shows title, source/workspace, indexed snippet when returned, date when parseable, pinned/archived metadata and current-conversation status. No missing metadata is fabricated. Native selection, a contextual Rename action, explicit Open/Rename, segmented scope, loading/empty/error states and bounded scrollable notices use established Gator styling. The editor and controls remain usable at the 540-point minimum width; the tested minimum outer height was 512 points.

Command-F remains transcript Find when the conversation window is active. In All Conversations it focuses catalog search and the Edit-menu action is labeled accordingly. Tab reaches the results list; arrows select; Return on the list opens the exact row. Rename has focused native text editing and explicit Save/Cancel. Escape cancels the editor or dismisses the catalog. Search/rename results do not request keyboard focus.

Closing the catalog does not recreate the conversation presentation, clear P01 Find or discard the composer draft. The new window's AppKit configuration does not replace the existing weak conversation-window registration. Returning to the original conversation restores its reading anchor; explicit owner switching still changes P01 search scope as designed.

Physical VoiceOver, IME, alternate keyboard layouts/preferences, macOS 15 hardware and physical notch interaction were **NOT TESTED**. Native accessibility labels and keyboard behavior were inspected/tested without claiming full accessibility acceptance.

## 9. Headless regression results

P01 baseline: 182 tests. Added 17 Swift tests: nine protocol/state/projection tests, six owner/identity/capacity tests and two real-Go catalog/integration/measurement tests. Existing managed real-Go tests gain assertions for P01 read-only search, catalog operations during work, current rename and rename overlapping background completion. No existing assertion was removed or weakened.

```sh
go build -trimpath -o /tmp/gator-p02-runtime ./cmd/gator
swift build --package-path macos --scratch-path /tmp/gator-native-build
GATOR_TEST_BINARY=/tmp/gator-p02-runtime swift test --package-path macos \
  --scratch-path /tmp/gator-native-build --no-parallel
go test ./cmd/gator ./internal/workapp ./internal/worksession
/Users/gongahkia/.local/share/fnm/node-versions/v26.7.0/installation/bin/node --test \
  app-host/tests/native.test.mjs app-host/tests/sessions.test.mjs \
  app-host/tests/drafts.test.mjs app-host/tests/reading-state.test.mjs \
  app-host/tests/backend.test.mjs
GATOR_TEST_BINARY=/tmp/Gator-Native-Parity-P02.app/Contents/Resources/gator-runtime \
  swift test --package-path macos --scratch-path /tmp/gator-native-build --no-parallel \
  -Xswiftc -strict-concurrency=complete -Xswiftc -warnings-as-errors \
  --filter 'catalogProtocol|catalogDecoding|catalogTitleValidation|catalogNativeAPI|transcriptSearch|transcriptFind|actualGoManagedDialogueKeepsMainWorkIndependentAndRetainsChildResult|catalogActualGoPages'
git diff --check
```

| Check | Result |
| --- | --- |
| Native build / packaging / plist | PASS; declared and Mach-O minimum macOS 15.0 |
| Complete sequential Swift suite after final title correction | **199 PASS**, 122.979 s |
| Strict Swift 6 and actual packaged Go selected integration | **17 PASS**, 16.489 s; warnings treated as errors |
| Go command/bridge/workapp/canonical catalog suites | PASS: cmd/gator 33.943 s; workapp 12.383 s; worksession cached PASS |
| Host native/session/draft/reading/backend suites | **41 PASS**, 0 failed/skipped, 10.177 s; Node 26.7.0 |
| Whitespace diff check | PASS |

The complete pass includes unchanged cross-process archive writers/conflicts, final-Quit/late edits/imports, inherited-pipe bounded shutdown, exact approval/plan/recovery and P01 Find/reading regressions. New Go tests cover native pages/content/scopes/non-recent/archived evidence, CAS/duplicate suppression, cancellation/result isolation, changed/removed/unavailable entries/fallback, cached title refresh, and nonce-before-result/restart protection.

Failed attempts are preserved as evidence rather than counted as passes. Initial compilation required using the existing status-view initializer and removing an unused test import. The first complete run passed 197 tests and failed one new measurement assertion: its second query reused the prior request ID and could read the prior result. The fixture now creates a fresh request, the Swift API rejects reuse of the latest result identity, and Go rejects a changed query under the same active ID. The corrected full run passed 199. A later focused Go test reproduced an externally renamed cached header remaining stale on Open; verified canonical metadata now refreshes that title. The final complete run and Go regressions include both corrections.

## 10. Performance measurements

Deterministic fixtures use canonical Go storage APIs, mixed archived/pinned records, retained Work text and public dialogue. Times below are debug request-to-result handling on a ready owner, not isolated full-index benchmarks or native rendering throughput. Initial owner startup may already reconcile metadata. Queries set `Content: true`; the title query precedes the Work-text query.

| Records | Title query | Retained Work content query | Returned page | Observed fallback |
| ---: | ---: | ---: | ---: | --- |
| 12 | 38.35 ms | 15.39 ms | 12 | No |
| 310 | 1,101.97 ms | 28.39 ms | 50 | No |
| 1,500 | 5,163.55 ms | 5,261.33 ms | 50 | Yes, both queries; Go returned `index_fallback=true` |

The large fixture reaches Go's bounded index path and canonical fallback; it is **not** evidence of a native indexing speedup. `incomplete=false` in that sample does not suppress the separate fallback warning. A deliberately unavailable cache fixture also returns the expected retained Work result with fallback notice. Canonical storage tests cover skipped/changed/future-index inputs.

Swift projection of a 50-row result, including JSONValue encoding plus typed decoding, averaged **0.521 ms** over 100 iterations. A 50-row stable-ID selection lookup averaged **6.69 µs** over 1,000 iterations, including assertions. Confirmed rename plus cached-owner title propagation measured **45.24 ms** in the real-Go fixture. Page navigation traversed six active pages (265 records from the 310 mixed catalog), checking uniqueness, Previous and reset behavior. UI rendering/focus latency was observed functionally, not given fabricated FPS figures.

## 11. Targeted native evidence

Used disposable profile `/tmp/gator-p02-1wwsts32`, separate preference domain `dev.gator.native.p02-disposable`, fixture-only retained records and an unreachable loopback provider. No desktop message was submitted. Real-Go work tests use the existing inert loopback provider fixtures, not paid providers or external connectors.

| Observation | Evidence |
| --- | --- |
| Light native list, scope, page controls and metadata | [Catalog light](evidence/p02/catalog-light.png) |
| Dark candidate UI and exact older query | [Catalog dark](evidence/p02/catalog-dark.png) |
| CAS error keeps attempt and offers review | [Conflict](evidence/p02/rename-conflict-light.png), [observations](evidence/p02/rename-conflict.txt) |
| Minimum-size editor after review | [Small rename](evidence/p02/catalog-small-rename-light.png) |
| Explicit Save and Cancel checked against canonical Go store | [Save/Cancel](evidence/p02/rename-save-cancel.txt) |
| Command-F scopes; closing keeps P01 query/anchor/draft | [Scoped Find](evidence/p02/scoped-find-and-p01.txt) |
| Tab/Down/Return opens exact older record; repeated open reuses PIDs | [Keyboard/non-recent](evidence/p02/keyboard-nonrecent-open.txt) |
| Return restores original draft/older anchor; archived listing | [Continuity](evidence/p02/return-original-and-archived.txt), [archived](evidence/p02/archived-light.png) |
| Packaged UI launch, persisted draft, Command-Shift-O and query | [Candidate launch](evidence/p02/final-candidate-launch.txt) |

The initial desktop checks were interrupted by window closure/preference changes. A subsequent user-authorized controlled session completed the keyboard/non-recent/P01 checks. Screens are app-window captures, not unrelated desktop content. Desktop interaction stopped after that session; final cached-title Go correction was then verified headlessly. The candidate's Swift UI executable is byte-identical to the final desktop-tested copy; its final Go resource is byte-identical to the runtime in the passing complete/selected packaged-runtime tests. This distinction is recorded in the manifest rather than presenting older runtime captures as proof of the last Go correction.

Desktop-only reproduction helpers are in `evidence/p02/`: fixture setup, isolated launch and an AX controller that refuses nonfixture bundle IDs. The Go fixture helper is under test resources and restricts writes to named disposable roots. These helpers are not production features. Physical accessibility and sustained GUI provider-stream stress remain unverified; deterministic/live-Go tests cover the selected concurrency transitions.

## 12. Changed-file inventory

| Area | Files / purpose |
| --- | --- |
| Go native bridge | `cmd/gator/ui_bridge.go`; finite dispatch/metadata refresh. `internal/workapp/bridge_native_catalog.go`; request/page evidence, canonical query/rename/open/inspect/verify, cancellation and metadata propagation. |
| Go integration | `app.go`, `bridge.go`, `update.go`, `models.go`; state/result fields, completion routing and query cleanup. `conversation.go`; captures existing destination load failures. `restart.go`; guards pending native title mutation. |
| Swift Core | `NativeCatalogProtocol.swift`, `NativeConversationCatalog.swift`; typed bounded data and state. `ManagedSessionProtocol.swift`; private finite host commands. `ManagedSessionCoordinator.swift`; origin binding, canonical handoff, safe candidate cleanup, metadata refresh and closing participation. |
| Native UI | `ConversationCatalogView.swift`; window, list, editor and scoped Find. `GatorApp.swift`; scene/menu/shortcuts. `ConversationView.swift`, `RecentConversationsMenu.swift`; entry points. `NativeWindowConfiguration.swift`; catalog window does not replace conversation registration. |
| Go tests | `internal/workapp/bridge_native_catalog_test.go`, `cmd/gator/ui_bridge_test.go`. |
| Swift tests | `NativeCatalogTests.swift`, `NativeCatalogOwnerTests.swift`, `NativeCatalogIntegrationTests.swift`; 17 new tests. `ManagedSessionTests.swift`; fixture support. `ManagedGoIntegrationTests.swift`; concurrent work/title assertions, retaining P01 assertions. |
| Fixture/report | `macos/Tests/GatorCoreTests/Fixtures/native-catalog-seed.go`; canonical disposable setup. This report and `macos/parity/evidence/p02/`. |

Core/Mac/test paths are under `macos/Sources` or `macos/Tests/GatorCoreTests` respectively. Go integration filenames in the second row are under `internal/workapp`. No production JS, package dependency, README, credential/provider configuration, canonical catalog storage implementation or transport replacement.

## 13. Package and remaining limitations

```sh
GATOR_NATIVE_APP_PATH=/tmp/Gator-Native-Parity-P02.app bash macos/scripts/build-dev-app.sh
xcrun vtool -show-build /tmp/Gator-Native-Parity-P02.app/Contents/MacOS/GatorMac
```

Packaging and plist validation passed. After the last Go-only cached-title correction, the candidate's Go resource was replaced with the exact `go build -trimpath` runtime used in the passing final suite; packaged-runtime integration checks followed. Swift UI bytes did not change. No signing/notarization/distribution work. P01 and prior safety/UI/UX candidates are preserved, with hashes in the manifest.

Known limitations:

- Canonical Go bounds, dialogue-index gap, and multi-second large-catalog fallback remain. No full-history or independent index was added.
- The catalog is refreshed on request, not a live subscription. Versions/canonical reads reject stale actions; snippets/filter membership may require Refresh after changes.
- Disconnected owners show cached state and require explicit reconnect. Their offline display can lag canonical metadata; no hidden owner replacement is attempted.
- Four retained recovery/uncertain owners can prevent opening another chat. Catalog origin retention can also keep an otherwise idle background slot while its workflow needs it.
- Rename editor/search state is transient; Go-confirmed titles and existing composer drafts are durable in their respective existing stores.
- Physical VoiceOver/IME/OS variants, macOS 15 runtime and sustained desktop streaming stress are unverified. No new P0/P1 issue remains reproduced by the completed checks; this is not a claim that untested environments are defect-free.

## 14. Completion and P03

CH03 and CH04 are implemented against canonical Go semantics, with explicit coverage/fallback limits. P01 remains covered and its native continuity was checked. No CH12, QT01, QT04, MD03, AS05, FL07, FI02, FI08, PL02, pin/unpin, archive/restore/delete UI, older-history paging or other parity feature was implemented. Fixture organization calls are test setup only.

P02 is ready for use within the documented limits and for a separately selected P03. No P03 decision or implementation is automatic.
