# P04E-1R2 — native stability and approval-isolation closure

**Verdict: CONDITIONAL. Native acceptance remains open.** A reproducible redundant-update problem was corrected and the final headless suites pass. The historical 1.4 GB layout stall and the 23–25% pending-review CPU observation are **not proven resolved**. Both new desktop preparation probes were contaminated before creating a pending review. The user subsequently directed: **“Finish headless; leave native acceptance open.”** Accordingly, **0 of 10 complete native lifecycle cycles** are certified.

Development candidate: `/tmp/Gator-Native-P04E1R2-Verified.app`. This is the requested filename, not a claim of completed native acceptance. The final binary was **not launched for desktop acceptance** after the headless-only direction. Do not begin P04U-1 on the strength of these results.

## Source identity and preservation

Starting HEAD: `c34735f0bc49d7a920c354e7bb10a94cecbfedcc`. The starting worktree already contained P04E-1R changes and evidence; these were preserved. No commit, checkout or reset was made. [Starting status and hashes](evidence/p04e-1r2/starting-state.json), [final source/bundle manifest](evidence/p04e-1r2/final-manifest.json).

Host: Apple M3, 16 GiB, macOS 26.5.2 (25F84). The package remains macOS 15+. Development/debug build; no signing or notarization operation.

| Final component | SHA-256 |
|---|---|
| Native executable | `fe2cf9e7d8313fe99aad6ee1424fff75acff27e542812f50f5e2cb6b530f375c` |
| Go runtime | `7cc9847678f1dbc1a141a17f3aed6671af175a476427833cd5b08bb68172e96a` |

No Go, host, credential, permission, archive, workspace, plan, recovery, composer or approval-control implementation changed. Hash comparison found no changes to the original P04E-1/P04E-1R evidence. The four prior bundles inventoried in the R report are byte-for-byte unchanged. Original mascot sources/assets remain untouched; runtime art remains emoji plus the two existing reaction GIFs.

The intermediate R2 baseline, diagnostic and pre-UTF8 candidates are preserved separately. The early R2 candidate was moved to `/tmp/Gator-Native-P04E1R2-Pre-UTF8.app` before the final fresh build; it was not overwritten. [Final build](evidence/p04e-1r2/build-final-utf8.log).

## Harness repair and validation

The R2 harness is isolated under `macos/presence/acceptance-r2/`. The previous controller/evidence remain intact.

Changes:

- Bind requests to a private fixture manifest containing the exact PID, unique fixture bundle ID, application path and executable SHA-256.
- Resolve both actual and expected filesystem paths. Require the manifest and app to share the disposable root. Record actual/expected paths on identity refusal.
- Recheck executable bytes on requests; no cached digest can authorize a changed file.
- Bind each action to a fresh preflight ID and the actual AX focused window/element references. Reject stale focus, foreign foreground applications and ambiguous targets before input.
- Match AX role plus identifier/title/description. Do not select a popup merely because its current value equals the requested menu item; this corrects a weakness in the earlier generic finder.
- Keep Approve and Decline absent from the allowlist. Ordinary Send is refused. Only the explicit `create-inert-review` setup case may press Send, once per PID, after verifying the exact seeded `Review mascot` draft.
- Use targeted AX actions only; no broad keyboard/mouse injection. Secure AX text values are redacted by the inspection helper.
- Record action times, controller/app PIDs, executable identity, window/focus, target role and result. A passive monitor records event kinds/source PIDs, never key contents.
- Require a quiet readiness interval and no existing running work before arming a prospective test. Startup input is retained separately; input after the gate still aborts the test.

**Validated headlessly:** a no-window, nonactivating AppKit identity host is accepted with the exact manifest; wrong bundle, executable, digest and manifest PID are refused, followed by a successful valid request. It launches no Go process and sends no desktop input. [Identity cases](evidence/p04e-1r2/harness-identity-validation.json), [run log](evidence/p04e-1r2/identity-validation-final.log).

**Observed in the native preparation revision:** ordinary Send, Approve and a stale-preflight request were refused before action. The correct disposable PID/bundle/executable could be inspected and activated. [Native controller trace](evidence/p04e-1r2/controlled-input.jsonl), [probe 20 log](evidence/p04e-1r2/probe.log), [probe 21 log](evidence/p04e-1r2/probe-21.log).

An early headless identity-host attempt asserted before a valid registration was observed; the raw refusal response was not preserved. Its exact initial discrepancy is therefore unverified. The final helper waits for observable identity readiness instead of treating process spawn as readiness. This was a fixture-validation failure, not evidence of a Gator product defect. Later `refuse nonfixture` results while inspecting already-exited probe PIDs are expected guard behavior, not app failures. No production approval rule was relaxed.

Final role/focus hardening was compiled and identity-tested headlessly, but **not replayed through a complete native settings cycle**. Harness/native interaction validation is therefore partial.

## Read-only native identity diagnostics

A debug-only `NativeAcceptanceDiagnostics` helper is enabled solely by `GATOR_NATIVE_ACCEPTANCE_DIRECTORY` pointing to an existing disposable `gator-native-test-presence-r2-*` directory. Normal launches have no diagnostic source or polling work.

On an explicit SIGUSR2 request, it writes a mode-0600 atomic snapshot containing selected conversation, actual Swift generation, interaction ID, token hash, message index, owner count, window visibility, active frame bytes and layer-animation counts. It excludes message text, raw tokens, credentials and tool arguments. It does not send bridge commands or mutate agent state. The signal source uses weak ownership and is cancelled from application termination. The existing Quit guard, closing transaction and Go shutdown code are unchanged.

This supplies prospective native generation evidence without continuously observing or updating the SwiftUI graph. The preparation profiles produced idle diagnostic records; a **pending native-review diagnostic capture was not reached**. Window/player retention and pending asynchronous task counts were not established by those captures.

## Desktop preparation and input provenance

Both probes used separate private profiles and bundle domains, copied packaged binaries/resources, seeded fixture-only history/drafts, an inert loopback provider and no real connected account. No other process was stopped to obtain desktop control.

| Probe | App PID | Controller PID | Result |
|---|---:|---:|---|
| 20 | 32855 | 32834 | ABORT: unowned events during startup/idle preparation; no pending review created |
| 21 | 33708 | 33692 | ABORT: unowned click after the quiet readiness gate; no pending review created |

The passive monitors recorded source PID **0** events. That does not identify a person or automation process. A process inventory also found an unrelated browser automation process; its involvement in these events was not established, and it was not stopped.

Probe 20's Go trace contains `native_provider`, `native_catalog` and `navigate` commands absent from the controller's action log. These establish activity outside this controller, not its physical/AX source or whether provider configuration changed. Probe 21 also received unowned input. Both profiles are excluded from prospective acceptance. [Aborted-probe Go audit](evidence/p04e-1r2/aborted-probes-go-audit.json).

The user explicitly selected headless-only continuation after these failures. No further desktop review, settings or lifecycle runs were performed. The no-window identity-host validation is not a native lifecycle cycle.

## Approval authority findings

### Historical test

The P04E-1R finding is unchanged: historical Go interaction `work-20261120T033715-9b901523bfc5/1` was resolved with `approved: true`; its initiating input is unknown. The original negative test remains **INVALID**. No new explanation is manufactured, and no new passing test retroactively validates it. [Preserved forensic audit](evidence/p04e-1r/historical-approval-audit.json).

### New native probes

The two aborted profiles have:

- Zero observed approval/decline bridge frames.
- No retained interaction records.
- No Work manifests or request journals.
- Recorded Go child exits with status 0.

These are **not passing pending-approval tests**: the probes aborted before the pending review was created. Zero decisions without an active approval opportunity cannot satisfy the prospective gate. [Go trace audit](evidence/p04e-1r2/aborted-probes-go-audit.json), [journal/process audit](evidence/p04e-1r2/final-process-and-journal-check.json).

No fresh unauthorized decision was recorded. Native input provenance nevertheless failed, so no native negative approval PASS is claimed.

### New headless authority regression

`realGoSpinnerOnlySnapshotsKeepExactPendingReviewUndecided` uses the final packaged Go runtime with a loopback fixture. It holds unrelated work, opens the separately owned inert review, records the real conversation/generation/interaction, injects 100 terminal-frame-only snapshots on the main actor, and verifies:

- Zero owner-reconciliation callbacks from those frames.
- The exact review remains current, with its generation/identity intact.
- The canonical Go interaction file stays byte-for-byte pending and undecided.
- Owner count remains two.
- Explicit confirmed shutdown does not manufacture an approval or decline.

The test prints bounded owner/generation evidence and explicitly labels itself `nativeDesktopCycle=false`. The existing real-Go 20-cycle navigation/review/shutdown regression also passes. [Targeted result](evidence/p04e-1r2/targeted-utf8.log), [final full suite](evidence/p04e-1r2/swift-full.log). Positive approval tests in the broader existing suites remain separate, explicitly controlled fixture cases.

## Reproduced update churn and focused correction

An inert, headless Go cadence probe held a pending `web_access` review for **5.064 seconds**. It received **64 states**: **58 differed only in top-level `loading_frame`**, five elapsed-time changes were observed, and the initial pending state introduced its review data. No decision was submitted. [Cadence data](evidence/p04e-1r2/bridge-cadence.json), [analysis](evidence/p04e-1r2/publication-analysis.json).

The native source does not consume the terminal spinner field. Previously, every such tick assigned the observed snapshot and called the owner coordinator, despite unchanged native data. The deliberately failing regression recorded **100 owner callbacks** and a snapshot-observer invalidation from 100 cosmetic ticks. This reproduces redundant native publication, not a complete SelectionOverlay stall. [Before](evidence/p04e-1r2/publication-before.log).

Correction:

1. Compare all decoded snapshot content except the **top-level terminal-only `loading_frame`**.
2. While already connected, ignore an unchanged native snapshot before observable mutation and owner reconciliation.
3. Continue publishing elapsed time, public text, results, approval identity, session requests, configuration and every unknown future field.
4. Compare string values and object keys by UTF-8 bytes so canonically equivalent Unicode spellings are not silently collapsed. Existing Decimal numeric semantics are retained.
5. Preserve the generation guard and process identical initial/reconnect state when not already connected.

This is structural deduplication, with no timer, rate budget or sampling of meaningful updates. It does not batch response text or implement smooth streaming. It does not modify Go emission, admission, permission or approval policy. The latest published snapshot remains a complete Go snapshot; only unused terminal frame values may lag until a meaningful event.

After correction, the same 100-frame regression produces **zero callbacks and no observed snapshot invalidation**. Elapsed and unknown-field updates still publish; changed approval tokens retire old reviews; stale decisions remain unsent; same-state reconnect works only for the correct generation. Unicode path/key tests pass. [Final targeted tests](evidence/p04e-1r2/targeted-utf8.log).

**Established:** redundant spinner-only publication is corrected. **[Inference]:** reducing this update pressure should reduce unnecessary native work. **Not established:** the reduction in actual pending-review CPU, or whether it prevents the historical 1.4 GB SwiftUI layout stall.

## Layout investigation and comparison matrix

The original sample remains evidence of sustained main-thread SwiftUI/AttributeGraph/SelectionOverlay activity and failed guarded Quit. It does not identify a particular text field or prove that repeated unchanged emoji/font assignments caused the loop. That speculative canvas change was not introduced. Text selection, review behavior, preferences, mascot layout and desktop reconciliation were preserved.

| Requested comparison | R2 status |
|---|---|
| Prior B2 baseline | Available/preserved; NOT TESTED natively |
| Current presence Off | Two startup preparations attempted; contaminated before a controlled phase completed |
| Emoji only | NOT TESTED natively in R2 |
| GIF reactions / active / settled | NOT TESTED natively in R2 |
| Desktop presence | NOT TESTED natively in R2 |
| Pending approval without review | Headless Go cadence observed; native comparison NOT TESTED |
| Visible review | Headless model/authority tests pass; packaged native comparison NOT TESTED |
| Pending review plus Settings changes | NOT TESTED natively in R2 |

No new main-thread sample at the first onset of a stall was captured: neither probe reached the intended review/settings sequence or reproduced that stall. No complete Settings lifecycle, selection-overlay recursion, geometry/preference feedback loop or emoji fitting defect was established. No speculative renderer, selection or focus workaround was applied.

## CPU, physical memory and attribution limits

The R calibration was repeated: POSIX CPU **0.249925 s**, raw counter divided by 1e9 **0.005998436 s**, converted by the host's **125/3 Mach timebase** **0.249934833 s**. The R2 sampler uses converted nanoseconds. CPU percentages below are percentages of one core; RSS and physical footprint are reported separately. [Calibration](evidence/p04e-1r2/cpu-unit-calibration.json).

Only contaminated whole-native-process diagnostic samples are available in R2:

| Probe | Sample duration | Median CPU | Peak one-second CPU | Peak RSS | Lifetime peak physical footprint |
|---|---:|---:|---:|---:|---:|
| 20 | 300.1 s | 0.26% | 43.11% | 146.36 MiB | 70.02 MiB |
| 21 | 183.6 s | 0.27% | 37.62% | 129.55 MiB | 47.28 MiB |

These profiles had no pending approval and received unowned input. The figures are **not controlled baseline measurements, evidence of a post-fix reduction, or a memory/lifecycle acceptance pass**. They describe the pre-correction diagnostic executable, not the final candidate. [Qualified measurements](evidence/p04e-1r2/contaminated-resource-summary.json).

The earlier R pending/review measurement of roughly 23–25% remains historical evidence. Native application, Go children, fixture processes and observer overhead were **not separated in a new matched phase trial**. No reliable active-GIF, post-GIF, pending-review, Settings-open/closed or final-Off CPU comparison was completed. Shader/GPU/energy counters, layout invalidation counts, player/cache retention and pending asynchronous presentation task counts remain unverified in the packaged final app.

The headless cadence source is a concrete source of unnecessary updates. It is not proof that the complete CPU observation was animation-related, layout-related or unavoidable. No new CPU cap or arbitrary suppression heuristic was introduced. No claim of bounded long-session resource use is made from these aborted runs.

## Native lifecycle matrix

Target: ten independent actual native cycles. **Completed: zero.** Preparation probes 20 and 21 are separate aborted attempts, not substitute cycles.

| Cycle | Status | Reason |
|---:|---|---|
| 1 | NOT RUN | Input isolation failed; user requested headless-only continuation |
| 2 | NOT RUN | Same |
| 3 | NOT RUN | Same |
| 4 | NOT RUN | Same |
| 5 | NOT RUN | Same |
| 6 | NOT RUN | Same |
| 7 | NOT RUN | Same |
| 8 | NOT RUN | Same |
| 9 | NOT RUN | Same |
| 10 | NOT RUN | Same |

[Machine-readable matrix](evidence/p04e-1r2/native-cycle-matrix.json). The 20-cycle Go regression and 100-frame tests do not count as desktop cycles.

The recorded app/provider/Go PIDs no longer survive. Cleanup requests and Go exit records are retained, but termination provenance was not sufficiently isolated to call these full guarded-Quit acceptance passes. No diagnostic kill is counted as a passing Quit. The final candidate itself has not been exercised through native Quit/relaunch.

No new screenshots or recording are claimed. Existing P04E-1 visual evidence remains historical with its original limitations. Physical macOS 15, VoiceOver, IME and external-display acceptance remain **NOT TESTED**.

## Final headless verification

All final Swift real-Go fixtures use:

```sh
GATOR_TEST_BINARY=/tmp/Gator-Native-P04E1R2-Verified.app/Contents/Resources/gator-runtime
```

| Verification | Result |
|---|---|
| Swift package build | PASS |
| Complete final Swift suite | **266 PASS**, 150.413 s test time / 151.051 s command time |
| Strict Swift 6, warnings as errors | **40 focused tests PASS**, 2.694 s test time; all package/test sources compiled strictly |
| Focused Go authority/safety | **10 PASS across two packages** |
| Full host suite under installed Node 26.7.0 | **140 PASS, 0 FAIL, 1 Windows-only SKIP**, 141 total |
| Controller strict Swift 6 / explicit macOS 15 target | PASS |
| Headless executable-identity/refusal validation | PASS; final cases retained |
| C sampler/calibration and Python source compilation | PASS |
| `git diff --check` | PASS |

Exact commands, runtime hash and timings: [verification manifest](evidence/p04e-1r2/verification.json), [full log](evidence/p04e-1r2/swift-full.log), [strict log](evidence/p04e-1r2/swift-strict.log), [Go results](evidence/p04e-1r2/go-safety.jsonl), [host results](evidence/p04e-1r2/host-tests-node26.log).

The complete suite retains P01–P04, exact approval, cross-process archive/CAS, late-edit closing protection, bounded inherited-pipe shutdown, owner isolation, presence/GIF cancellation, Reduce Motion, Tuck Away and focus/reading-model assertions. No baseline assertion was weakened. No paid provider or model download was used.

Failures retained honestly:

- The before-correction publication regression failed as expected, showing the redundant callbacks.
- One new test initially used the wrong fixture helper method signature; the compile failure is retained, then corrected without weakening the assertion.
- The first host run used unsupported Node 20.19.5 and failed a PDF test with `Promise.withResolvers is not a function`: **139 pass, 1 fail, 1 skip**. The package requires Node ≥22.12.0. Rerunning under the already installed Node 26.7.0 passed; no dependency or user-default runtime was changed. [Original host failure](evidence/p04e-1r2/host-tests.log).
- Earlier successful pre-UTF8 logs are preserved separately. The final complete/strict runs were repeated after the UTF-8 refinement; only the final results above certify the delivered source.

## Changed-file inventory

Relative to the recorded R2 starting worktree:

- `macos/Sources/GatorCore/BridgeProtocol.swift`: native-content comparison ignoring only the terminal frame; UTF-8-sensitive recursive content comparison.
- `macos/Sources/GatorCore/GatorApplicationState.swift`: skip connected, unchanged native snapshots before observation/coordinator publication.
- `macos/Sources/GatorMac/NativeAcceptanceDiagnostics.swift`: new optional debug-only read-only identity/resource capture.
- `macos/Sources/GatorMac/AppDelegate.swift`: own and cancel that optional diagnostic source.
- `macos/Sources/GatorMac/GatorApp.swift`: initialize it only when explicitly configured.
- `macos/Tests/GatorCoreTests/NativeSnapshotPublicationTests.swift`: five new tests for publication, meaningful updates, Unicode, reconnect/generation and real-Go pending-review preservation.
- `macos/presence/acceptance-r2/`: isolated manifest-bound controller, private fixture/driver, bounded cadence probe, native preparation probe, no-window identity validator, calibrated sampler and reproducible verification runner. The recording helper is compiled/prepared but no completed R2 recording exists.
- This report and `evidence/p04e-1r2/`: source identity, raw failures, input/Go audits, metrics and verification.

Pre-existing R1 edits/untracked files are not attributed to R2. Original reports/evidence, old art and prior accepted bundles were preserved. No README, Go/host source, credentials, provider defaults or execution feature was changed; no commit, dependency installation, signing or notarization was performed.

## Reproduction and remaining conditions

Headless commands:

```sh
PYTHONDONTWRITEBYTECODE=1 python3 macos/presence/acceptance-r2/verify.py
go test ./internal/workapp ./internal/workrun -run 'Test(DesktopApprovalCannotRespondToAnotherLiveRequest|WebResearchReviewIdentityDoesNotDependOnDisplayFields|ReadInteractionRecordsValidatesAndScopesRetainedDecisions|InteractionDigestSurvivesRetainedActionPreview|InteractionDigestPreservesLargeJSONNumber|InteractionRunLockReportsActiveThenStopped|PublicResearchWaitsForBoundForegroundReviewAndRetainsFetchedEvidence|WebScopeDenialInvalidRequestsAndRevocation|RevocationDuringReviewExpiresApproval|PublicGrantDoesNotBypassPrivateDNSOrCancelledReview)$'
PATH=/Users/gongahkia/.local/share/fnm/node-versions/v26.7.0/installation/bin:$PATH npm test --prefix app-host
```

A future native session must use a fresh evidence directory/profile and verify no foreign input or unexplained Go commands before creating a review. Capture exact native generation and Go identity, then perform the requested comparison phases and ten complete native cycles with the **final** binary. Record samples and per-process calibrated resource data at the first deterioration. Verify zero decision submissions and unchanged Go authority records independently of network counters. Do not automatically retry uncertain actions or reuse contaminated profiles.

**Final decision: CONDITIONAL.** The specific redundant-update defect is corrected and headlessly verified. Historical provenance remains unknown/INVALID; fresh native pending-approval isolation, final native responsiveness, matched pending-review CPU, long-session resource behavior and ten-cycle guarded-Quit acceptance remain open. No fresh unauthorized approval was recorded, but the final native safety opportunity was not exercised. P04U-1 and later feature work remain gated; this tranche stops here.
