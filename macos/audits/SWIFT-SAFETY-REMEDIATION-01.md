# Swift engineering safety remediation 01

Date: 2026-11-17. Scope: engineering audit E01 (P0), E02 (P1), and E03 (P1), with shutdown-result propagation needed to complete E02/E03. No parity implementation or unrelated remediation pass.

## Outcome and baseline

The three original failures were reproduced against the unchanged implementation before production edits. The changes serialize native archive transactions across actors/processes, establish an application closing boundary, and separate direct-child exit from inherited-pipe EOF. Final verification and candidate details appear below.

- HEAD: `137fb7ea923d14149badbb02e102ffdfb6e34bed`; no commit created.
- Inspected the current worktree and all three requested audits before implementation. Existing UI/UX evidence changes and untracked audit/evidence files were preserved. In particular, `macos/audits/evidence/uiux/headless/swift-audit.log` was already modified; it is not a remediation edit.
- Environment: arm64 macOS 26.5.2; Apple Swift 6.3.3; Go 1.27.3; host verification used the already installed Node 26.7.0. Package tools version 6.1, Swift 6 language mode, declared macOS minimum 15, no external Swift package dependencies.
- No production Go/JS, README, credentials, provider/model configuration, signing, notarization, or terminal state changes. Test providers/configurations are confined to disposable fixture directories and loopback endpoints. No real external operation was performed.

## 1. Root causes and before/after evidence

| Defect | Root cause and observed before result | Corrected behavior and verification |
| --- | --- | --- |
| E01 / P0 | Each store actor independently loaded, merged and atomically renamed a whole archive. Neither actor isolation nor atomic rename protected the complete transaction against other actors/processes. **16 successful owner writes retained only 2 drafts** in the pre-change probe. | Stable sidecar locking encloses read/check/merge/encode/write/rename/sync. Independent-actor and independent-process tests verify **all 16 exact texts**, selection and private permissions. Stale same-context writes fail explicitly. |
| E02 / P1 | Quit flushed a captured save task, awaited transport cleanup, accepted another edit during that await, then returned success without checking the newer failed save. Pre-change probe: `canEditDuringShutdown=true`, `saved=false`, `quitReportedSuccess=true`, with new input still only in memory. | Every existing owner closes admission synchronously before coordinator suspension. Already accepted edits join the save chain. Final transport events are consumed and persisted before success. Paused/failed writes, failed final-disconnect persistence, attachments, multiple owners, repair/retry and repeated shutdown have regression coverage. |
| E03 / P1 | EOF and reader-task completion depended on every inherited descriptor closing. Direct-child exit had no independent observer, and shutdown awaited blocking readers. A controlled eight-second descendant produced **8.027881375 seconds** shutdown time after the direct owner exited. | Foundation termination notification initiates bounded output draining. Nonblocking descriptors and cancellation-aware finite polls permit reader completion without descendant EOF. Tests assert direct PID reaping, completed reader/input cleanup, bounded diagnostics and still-live descendants. |

Recompiling and rerunning the same three scratch probes after the corrections produced `successful=16 retained=16`, `canEditDuringShutdown=false` with `text=saved before quit`, and descendant shutdown of **0.360023792 seconds** (`/tmp/gator-safety-after.log`). The late request is rejected; the previously accepted text remains the durable input. This probe externally moves the archive directory to induce a storage failure, so the dedicated closing tests also check the stored contents directly without moving the directory. In the final fixture suite, stdout/stderr/both inherited-descriptor shutdown took 0.303489 / 0.309195708 / 0.3578625 seconds respectively.

The pre-change harness was compiled from the then-unchanged Core sources and the audit's scratch probe, not from a reconstructed historical checkout. The current test fixtures are checked into the worktree for repeatable regression verification.

## 2. Cross-writer archive transaction

Entry points: `load`, `save`, `saveOwner`, new internal `saveOwners`, and `selectConversation` all acquire the same stable `<profile-digest>.json.lock` sidecar. Production writers are the owners' serialized persistence tasks and coordinator selection updates. Owners in one coordinator share a store; separate coordinators, convenience initializers and separate application processes can create independent actors for the same profile. Terminal drafts use a separate namespace and are never opened here.

The lock is a private regular file, opened with `O_NOFOLLOW`/`O_CLOEXEC`, checked for current-user ownership, and retained across archive replacements. It is never unlinked. Nonblocking `flock` retries suspend rather than blocking the main actor; contention beyond five seconds is an explicit save failure. Once acquired, the complete transaction has no suspension point. Kernel descriptor cleanup releases locks after process termination.

`load` attaches in-memory expectations to the snapshot. They are excluded from Codable keys and do not change archive versions 1–7. Each expectation fingerprints the complete conversation record: draft/revision/correction, pending submission, plan, recovery preparation and receipt operation. Fingerprints derive from raw disk state before restart normalization marks unresolved operations uncertain.

`saveOwner(s)` checks each affected context against its last successful baseline while retaining current unrelated disk contexts. A batch publishes once, including workspace-to-conversation promotion contexts. `save` computes a three-way patch relative to its load baseline, rather than replacing unrelated records from its old snapshot. Explicit `selectConversation` serializes selection only; a stale full-snapshot selection change fails comparison. This avoids coupling all owner writes to whichever conversation is selected.

Same-context policy is deliberately conservative: a write succeeds only when the disk record matches the writer's expected baseline, or already exactly matches the proposed record (idempotent retry). Otherwise it throws before publication and leaves the proposed input in memory. There is no timestamp-wins rule or speculative text/approval/recovery merge. Reloading/reconciling the competing owner is required. Successful receipts advance only the caller's affected baselines; chained application saves use the latest successful receipt rather than stale captured expectations.

Publication preserves the existing retention rules. It validates encoded output, writes a unique private temporary file, synchronizes/closes it, renames within the same directory, and synchronizes that directory before acknowledging success. Read intake checks regular-file type/size and is bounded to 6 MiB plus an overflow byte. Corrupt/newer archives fail closed on every entry point. Failed encoding, failed replacement and a real permission-denied temporary-file creation leave the previous bytes unchanged. Kill-before-rename and kill-after-rename fixtures establish old-or-new complete contents and lock reuse after the direct child is reaped.

Limits: the lock is advisory; all participating native processes must use this corrected implementation. An older binary or unrelated editor can ignore it. A crash before rename can leave an unreferenced temporary file; it cannot replace the archive. Such files are not automatically deleted because this change does not introduce a cross-process retention/cleanup policy. Hardware power-loss behavior and physical disk exhaustion were not induced. Synchronization failures are propagated rather than acknowledged as durable success.

## 3. Application closing transaction

1. Existing active-work/pending-approval confirmation remains the entry condition. Coordinator shutdown synchronously sets `closing` on every recorded owner before its first await; navigation/new-owner admission remains gated by coordinator `stopping`.
2. Core mutation/admission methods enforce the boundary, independently of view button state. Text, attachment selection, reply selection, correction and plan-first changes are rejected after it. Authorization/recovery/receipt/submission gates also check closing after relevant suspension points.
3. Every accepted pre-boundary draft and retained operation identity enters the existing ordered save chain. `flushDrafts` rechecks the save sequence after each await and includes saves queued while it was waiting. Owner-context batches include promoted and pending contexts.
4. Unfinished file selection/import identities are invalidated synchronously. A late staging completion cannot append attachments. Previously accepted references remain in the draft; staged private copies are conservatively retained. On aborted Quit the user can edit and select the unfinished files again. Already accepted imports are persisted with the rest of the draft.
5. All owners preflight their latest accepted state before any owner is stopped. Unresolved submission/plan/recovery/receipt identities become uncertain without replay. A prior recoverable save error is retried with current memory state.
6. Each transport must confirm direct-owner termination. Final events finish consumption before the final flush. Failure at preflight, termination confirmation, or final persistence returns false and keeps Quit unsuccessful. The app-level alert presents the storage/shutdown error.
7. Failed Quit clears closing on all retained owners. A live owner remains usable; an already stopped owner exposes disconnected editing and explicit reconnect. A repaired save can be retried. No execution is automatically retried or replayed. Successful repeated shutdown is idempotent and does not reopen storage or a process.

A successful close therefore covers the accepted state at the closing boundary and subsequent final owner events. Edits requested after the boundary are rejected, rather than accepted and silently discarded. A mock transport test also verifies that an unconfirmed exit blocks both Quit success and replacement-owner creation.

## 4. Direct owner and pipe cleanup

`GoBridgeConnection` retains the existing NDJSON commands and nonce/result distinction. `action_applied` still acknowledges transport receipt only. No new Go transport was introduced.

- `Process.terminationHandler` detects direct-owner termination independently of stdout/stderr EOF. Foundation reaps the direct child; no synchronous `waitUntilExit` is used in production.
- Shared owned shutdown/cleanup tasks handle concurrent callers. Existing escalation windows remain: 2.5 seconds graceful, 2.5 seconds after SIGTERM, then SIGKILL and up to 2 seconds confirming the direct process exit. Only the tracked direct PID is targeted; no process-group or descendant kill was added.
- After direct exit or output EOF, readers get up to 250 ms to drain final frames/diagnostics. This bounds the EOF wait; it does not shorten process escalation timeouts. Partial trailing frames remain explicit unconfirmed protocol outcomes.
- Reads run off the main/cooperative executor on a utility queue, using nonblocking descriptors with 100 ms finite `poll` waits. A synchronized cancellation bit and task cancellation end readers even while descendants retain descriptors. Reader tasks close their own read handles after the last operation; cleanup awaits both tasks.
- Writes remain on the existing serial queue, with nonblocking partial writes and cancellation-aware finite polling. Cancellation interrupts blocked stdin; input close runs behind its writer and cleanup awaits that queue boundary. Handles are not closed concurrently with an in-progress read/write.
- Acknowledgement deadline tasks are canceled and cleared along with waiter continuations. Stderr retains at most 4096 bytes. Tests check resource-owner references/continuations are cleared and the PID is gone; this is not a claim of whole-app leak profiling.
- `shutdownConfirmed() -> Bool` lets production callers distinguish confirmed exit from cleanup without exit confirmation. The old `shutdown()` method remains compatible. The protocol default supports existing test doubles; the production Go transport implements the actual confirmation.

The bounds above assume ordinary OS scheduling/system-call progress; they are not hard real-time guarantees under machine suspension or kernel failure. The pipe fixtures deliberately leave descendants alive until the test releases them, proving completion does not depend on killing them.

## 5. Changed files

| File | Purpose |
| --- | --- |
| `macos/Sources/GatorCore/NativeDraftStore.swift` | Cross-writer locking, context comparison/merge, batched ownership writes, bounded reads, durable publication and internal fault stages. |
| `macos/Sources/GatorCore/GatorApplicationState.swift` | Successful-save baselines, stable flush, closing admission barrier, final event/persistence handling, exit confirmation and usable abort/retry. |
| `macos/Sources/GatorCore/ManagedSessionCoordinator.swift` | All-owner closing boundary, preflight before stopping owners, failed-close restoration and idempotent completed shutdown. |
| `macos/Sources/GatorCore/GoBridgeConnection.swift` | Direct-exit notification, bounded reader cleanup, owned task/deadline cleanup, serialized cancelable writer and confirmed shutdown result. |
| `macos/Sources/GatorCore/BridgePipeIO.swift` (new) | Small nonblocking pipe adapter and lock-protected cancellation bit. |
| `macos/Sources/GatorMac/AppDelegate.swift` | Present coordinator shutdown error when persistence itself has no error. |
| `macos/Tests/GatorCoreTests/NativeArchiveSafetyTests.swift` (new) | Nine archive tests with parameterized migration/interruption/corruption cases and 16 independent writers. |
| `macos/Tests/GatorCoreTests/NativeClosingSafetyTests.swift` (new) | Seven deterministic closing/flush/import/failure/confirmation tests, including repeated completed shutdown. |
| `macos/Tests/GatorCoreTests/BridgeShutdownSafetyTests.swift` (new) | Seven subprocess tests, including inherited descriptors, external exit, partial frame, concurrent callers and independent owners. |
| `macos/Tests/GatorCoreTests/Fixtures/native-archive-writer.swift` (new) | Independent executable writer with filesystem gates and interruption stages. |
| `macos/Tests/GatorCoreTests/Fixtures/pipe-lifecycle.py` (new) | Controlled direct owner/descendants, streaming and partial-frame exits. |
| `macos/audits/SWIFT-SAFETY-REMEDIATION-01.md` (new) | This report. |

## 6. Swift concurrency implications

Application state/coordinator remain main-actor isolated. Store transactions run on the store actor; lock contention suspends, while the locked transaction does not reenter. The lock provides serialization beyond the actor/process boundary. Save chaining advances expectations only on acknowledged writes.

Bridge lifecycle remains actor-owned. Blocking waits are confined to finite utility-queue pipe polls or the serial writer, with explicitly owned cancellation and cleanup. `@unchecked Sendable` is limited to the internally locked cancellation bit (and internally locked test gates); no state class was made unchecked to silence a diagnostic. No language-mode relaxation or new dependency was used.

Test gates pause disk/upload/transport owners instead of relying on arbitrary sleeps to trigger the interleaving. Small polling intervals wait for explicit fixture conditions. Cross-process children all signal that they loaded their snapshots before their common write gate is released.

## 7. Verification commands and results

All routine verification was headless. Exact final results are recorded below; logs are local `/tmp/gator-safety-*.log` artifacts, not modifications to earlier audit evidence.

### Before reproductions

```sh
swiftc -swift-version 6 -parse-as-library macos/Sources/GatorCore/*.swift /tmp/gator-swift-audit-probes.swift -o /tmp/gator-safety-before-probes
python3 /tmp/gator-safety-before.py > /tmp/gator-safety-before.log 2>&1
```

After production edits, the same harness/controller was rebuilt with the executable name changed to `gator-safety-after-probes`:

```sh
swiftc -swift-version 6 -parse-as-library macos/Sources/GatorCore/*.swift /tmp/gator-swift-audit-probes.swift -o /tmp/gator-safety-after-probes
python3 /tmp/gator-safety-after.py > /tmp/gator-safety-after.log 2>&1
```

All three original probes exited zero and exposed the failures in section 1. A zero probe exit was not treated as success; reported retained contents, persistence outcome and elapsed time were inspected.

### Targeted implementation verification

- Existing targeted transport/admission/persistence tests: six passed (`/tmp/gator-safety-targeted.log`), including blocked stdin escalation in 5.820 seconds and externally killed direct owner in 0.189 seconds.
- Nine archive tests passed (`/tmp/gator-safety-archive.log`), including sixteen gated independent processes and exact final values.
- Thirteen lifecycle/fault tests passed (`/tmp/gator-safety-lifecycle.log`).
- A later 28-test regression run found a new test's mismatched error wording. The test now checks the actual specific shutdown-confirmation message; no production behavior/assertion was weakened.
- The first complete suite found a real compatibility regression: a second call after successful coordinator shutdown returned false. The existing real-Go retained-plan test reproduced it independently. Idempotent completed shutdown was restored; the unchanged existing test plus two closing tests passed (`/tmp/gator-safety-idempotent.log`, 3 tests).
- The next complete run passed 155 tests in 48.757 seconds (`/tmp/gator-safety-full.log`). An additional independent-owner concurrency test and a real permission-denied write assertion were then added and passed under strict compilation (`/tmp/gator-safety-strict-tests.log`, 2 tests).

Commands used for targeted runs:

```sh
GATOR_TEST_BINARY=/tmp/gator-safety-runtime swift test --package-path macos --scratch-path /tmp/gator-safety-build --no-parallel --filter 'safety|shutdownEscalates|shutdownOfExternallyKilledOwner|independentUncertainAdmissions|attachmentDraftsShareSurfaces|firstWorkspaceAttachment|planUncertaintyPersists'
GATOR_TEST_BINARY=/tmp/gator-safety-runtime swift test --package-path macos --scratch-path /tmp/gator-safety-build --no-parallel --filter 'realGoRestoredApprovedPlanDoesNotAutoStartAndExplicitStartExecutesOnce|safetyLateEdit|safetyAllOwners'
GATOR_TEST_BINARY=/tmp/gator-safety-runtime swift test --package-path macos --scratch-path /tmp/gator-safety-build --no-parallel -Xswiftc -strict-concurrency=complete -Xswiftc -warnings-as-errors --filter 'safetyIndependentOwners|safetyFailedEncoding'
```

### Final full suite and strict compiler

```sh
go build -o /tmp/gator-safety-runtime ./cmd/gator
GATOR_TEST_BINARY=/tmp/gator-safety-runtime swift test --package-path macos \
  --scratch-path /tmp/gator-native-build --no-parallel
swift build --package-path macos --scratch-path /tmp/gator-safety-build \
  -Xswiftc -strict-concurrency=complete -Xswiftc -warnings-as-errors
```

The final complete Swift suite passed **156 tests in 50.241 seconds**, zero failures, including 23 new safety tests and the existing real-Go security/ownership tests (`/tmp/gator-safety-final-suite.log`). The Go runtime build and strict production build passed with no compiler warnings/errors. The strict targeted test invocation above also compiled both test targets with complete strict concurrency and warnings-as-errors.

### Unchanged contract checks

```sh
go test ./cmd/gator ./internal/workapp ./internal/attachment ./internal/worksession -run 'Test(UIBridge|BridgeUploads|DesktopApprovalCannot|ExecutionOwnerIsExclusive|DialogueAdmission|.*TaskPlan.*Reject|.*RequestRecovery.*|.*Upload.*)' -count=1
/Users/gongahkia/.local/share/fnm/node-versions/v26.7.0/installation/bin/node --test app-host/tests/backend.test.mjs app-host/tests/sessions.test.mjs app-host/tests/drafts.test.mjs app-host/tests/uploads.test.mjs app-host/tests/policy.test.mjs
git diff --check
```

Go: all four packages passed. Host: 63 passed, zero failed/skipped/cancelled. Whitespace validation passed. These are focused contract checks, not a claim that the entire backend suite was run.

## 8. Remaining risks and readiness

No unresolved instance of E01–E03 remains in the exercised scenarios. This is not a new comprehensive audit or a claim that every possible P0/P1 has been excluded.

- **P0/P1 operational compatibility condition:** older/nonparticipating native writers ignore advisory locking. Use the corrected binary for every simultaneous native instance sharing a profile. Same-context conflicts deliberately block durable replacement and Quit until reconciled; input is retained in memory, not automatically merged. Conflicting input has no new export/reconciliation UI in this pass.
- **P2 E04:** an uncertain restored receipt operation can remain blocked; reconciliation policy remains deferred.
- **P2 E05:** historical failure evidence can pin owner capacity; retirement policy remains deferred. Session capacity rules were preserved.
- **P2 E07:** duplicate document basenames still fail Go/provider admission safely. This is a shared identity-contract limitation; no Swift validation bypass or renaming workaround was introduced.
- Other previously recorded P2 risks remain outside this pass, including provider callbacks that never complete during ordinary use, cancellation-insensitive application polling, retained upload/cache growth and stop-outcome presentation. Closing can invalidate an unfinished import, but does not add general-purpose import timeouts or storage garbage collection.
- Pending approval tokens, plan revision/digest identity, recovery/access-review authority, Go execution ownership/leases, admission, duplicate suppression, no replay, attachment validation and connector permissions were not relaxed. Existing real-Go tests ran in the full Swift suite.

Unverified/operator-only: actual UI Quit confirmation/error presentation, IME composition during physical Quit, live NSItemProvider/file-picker behavior, macOS 15 runtime execution, kernel refusal to confirm a SIGKILLed real process, physical disk exhaustion/power failure, and long-running Instruments/descriptor profiling. Mock exit-confirmation failure is covered; no unkillable process was fabricated. No GUI or distribution acceptance is claimed.

**Readiness:** the verified transaction and lifecycle behavior supports continuing controlled native development after this remediation is reviewed and all native instances use the corrected writer. Resolve E04/E05 policy before expanding recovery controls or owner-retirement behavior. This report selects no terminal features and authorizes no subsequent remediation pass.

## 9. Verified local candidate and final handoff

After all checks passed, the existing development bundling script ran successfully:

```sh
GATOR_NATIVE_BUILD_DIR=/tmp/gator-native-build \
GATOR_NATIVE_APP_PATH=/tmp/Gator-Native-Safety-Verified.app \
bash macos/scripts/build-dev-app.sh
```

Candidate: `/tmp/Gator-Native-Safety-Verified.app`. Bundle plist validation passed; identifier `dev.gator.native`, minimum macOS `15.0`. Both executables and all three icon/image resources are present and nonempty. Packaged Swift executable bytes match the verified `/tmp/gator-native-build/arm64-apple-macosx/debug/GatorMac` build. The script built the bundled Go runtime from the unchanged current worktree. No signing, notarization, distribution or GUI launch was performed. Packaging log: `/tmp/gator-safety-package.log`.

Final complete suite: **156 passed / 0 failed**, in **50.241 seconds**. Strict Swift 6 production and test compilation passed; targeted Go packages and **63 host tests** passed. The existing blocked-stdin test still passed at **5.521 seconds**, without raising its timeout. Independent-owner concurrent shutdown passed in **1.725 seconds** including its fixture setup/cleanup. `git diff --check` passed and HEAD remained unchanged. Existing tests were not removed or weakened.

All three requested corrections and their regression checks are complete. External worktree changes remain preserved. No next remediation pass or terminal feature work was begun.
