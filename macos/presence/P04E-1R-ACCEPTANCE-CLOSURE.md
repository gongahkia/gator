# P04E-1R — emoji presence stability and acceptance closure

**Verdict: CONDITIONAL. Native acceptance remains open.** The historical approval negative test is **INVALID**: Go accepted an approval whose initiating input cannot be attributed. A fresh headless Go regression preserves the exact pending record through presentation navigation and shutdown. One partial native settings sequence remained responsive and quit normally, but the monitored batch aborted on unowned input. The user then explicitly directed: **“Continue headless; leave desktop acceptance unverified.”** No production fix is justified by the available reproduction evidence.

The requested development candidate exists at `/tmp/Gator-Native-P04E1R-Verified.app`. “Verified” is the requested filename, **not a claim of complete desktop acceptance**. It contains the unchanged emoji/GIF implementation. No later feature increment was started.

## Baseline and scope

The recorded baseline is HEAD `22a2a71bcf5754e926bbcc34fcf3603607bd62d8`, captured 2026-11-20 04:06:55 UTC. The worktree/HEAD advanced through external background commits during this investigation; no commit, reset or checkout was made by this task. File hashes, rather than a moving HEAD, identify the tested implementation. [Baseline/source hashes](evidence/p04e-1r/baseline.json), [candidate manifest](evidence/p04e-1r/candidate-manifest.json).

Host: Apple M3, 16 GiB, macOS 26.5.2 (25F84). Swift version, build mode, all bundle hashes and final recorded HEAD are in the manifest. SwiftPM deployment remains macOS 15. Development build, no signing/notarization operation.

Candidate hashes:

| Component | SHA-256 |
|---|---|
| `Contents/MacOS/GatorMac` | `4f1740a4ebb360fe5e15332906ddada4a752237861e1464f686a617ae33ea917` |
| `Contents/Resources/gator-runtime` | `5eb910001bb27b6b7d9216fae7fddd9151fd1cfd998c01391fa60716ab3c5713` |

No production Swift, GatorCore, Go, resource, build-script, approval, provider, credential, archive or shutdown source changed from the captured baseline. The original sources/assets/evidence and earlier bundles remain. The only new Swift test is `PresenceApprovalClosureTests.swift`. Runtime resources still contain `Presence/attention.gif` and `Presence/completed.gif`; obsolete mascot artwork is absent. The prior source artwork remains in the repository.

## Historical stall: established facts and uncertainty

The original sample identifies PID **12492**, bundle **dev.gator.presence.finalcheck**, launch 11:33:44 +0800, sample **11:41:56 +0800**. It records **1.4G physical footprint** and a main thread repeatedly inside SwiftUI/AttributeGraph update/layout, including `SelectionOverlay.updateNSView`. Guarded Quit returned `quit_requested=true terminated=false`. That fixture failed; force-stopping it in P04E-1 was not a passing Quit.

The stack establishes the busy subsystem, not the view or mutation that caused the loop. SwiftUI text-selection overlays and an ordinary nonselectable `NativeMascotCanvas` label are different objects. There is no reproduced evidence supporting a change to unchanged emoji text/font assignments; the earlier standalone invalidation probe did not support that hypothesis. Selection, review controls and the canvas were therefore preserved.

A material timeline correction: Go's retained trace places the historical `web_access` return at **11:38:12.855883 +0800**, and Work completion at **11:38:19.943513 +0800**. By the 11:41:56 sample the approval had already been resolved. A test that only holds an approval pending does not recreate that entire accepted-decision/response/settings interleaving. [Original sample](evidence/p04e-1/unresponsive-sample.txt), [failed Quit](evidence/p04e-1/final-guarded-quit.log), [forensic audit](evidence/p04e-1r/historical-approval-audit.json).

**Root cause remains unresolved.** Neither an application defect nor an external/platform cause has been conclusively established. Nonreproduction in a short sequence is not a correction.

## Native reproduction and controller

The fresh package was copied into private temporary `gator-native-test-presence-r1-*` directories. Copies changed only Info.plist bundle/name metadata (`dev.gator.presence.r1`) to isolate preferences and accessibility targeting. All binary/resource bytes match the deliverable; the manifest verifies this for the settings-tested copy.

Fixtures used temporary native archives, Go state/configuration and workspaces; inert credentials; a loopback-only deterministic provider; inspect mode; no configured real services, paid inference, fetch implementation or external-write operation. The provider can request an inert scope for `https://mascot.example.invalid` and return fixture text. It contains no outbound client. Canonical Go interaction records, command observations and captured run outcomes are inspected independently of provider counters.

The controller is a separate test executable. It:

- Requires the exact fixture bundle identifier and temporary bundle path.
- Checks the foreground PID, exact window identity and expected focused AX element before each ordinary control action.
- Uses targeted AX actions; it does not post global keyboard/mouse events.
- Allows a finite list of settings/navigation/fixture-Send/Quit controls. Approve and Decline are absent.
- Records controller/app PIDs, timestamps, target controls, window/focus identities and elapsed AX call times.
- Compares conversation, interaction ID, token hash, message index, result hash and the durable Go pending record around actions.
- In the monitored batch, listens passively for input kinds/source PIDs without recording characters. Foreign or unattributed input aborts acceptance input. No new OS permission was requested.

Native Swift generation UUID was not exposed by the packaged UI. Process/wrapper incarnation and Go review identity are recorded; this is not mislabeled as full native generation observability. The headless regression captures and verifies the actual Swift generation.

### Observed runs

| Run | Evidence and outcome |
|---|---|
| Setup 1–2 | Fixture-launch/path-guard preparation failures; not acceptance cycles. No result promoted to PASS. |
| 3 | Seeded fixture request, pending approval, guarded Quit exit 0. Initial review-target assumption failed because Work belonged to a different owner; sequence incomplete. |
| 4 | Opened the exact Work owner/review; Settings; Emoji/GIF/Off changes; Animate off; desktop on; chat off; Settings close/reopen; Tuck Away; review Back/reopen; guarded Quit exit 0. Approval identity remained unchanged and Go recorded no approval command. Responsive during this partial sequence. No passive input monitor was installed yet, so it is not certified as the exclusive-input acceptance run. |
| 5 | Passive monitor active, controller PID **25289**, app PID **25302**. Batch aborted on source-PID-zero click/scroll input while preparing the review. Excluded from acceptance. Guarded cleanup used a separately logged controller after abort; it is not counted as a clean cycle. |

For run 5, Send at **05:32:57.473086 UTC** and Open Original Task at **05:32:57.907014 UTC** came from the known controller. The first monitored unowned event was **05:32:58.208222 UTC**. Earlier commentary and the follow-up question incorrectly described an unowned Send; the timestamped record corrects that claim. Source PID zero does **not** identify a person, application or accessibility client. [Input trace](evidence/p04e-1r/controlled-input.jsonl), [input events](evidence/p04e-1r/input-events-25278.jsonl), [aborted batch](evidence/p04e-1r/batch-abort.json), [cleanup](evidence/p04e-1r/aborted-fixture-cleanup.json).

The ordinary sequence used 350 ms transaction-settling intervals plus observable snapshot/window readiness and manual pauses. No rapid stress result is claimed. These runs did not reproduce the 1.4G stall, but did not establish its cause. **0 of the required 10 clean end-to-end cycles were certified.** No matched P04M-B2/current-Off/current-Emoji/current-GIF/desktop/review A–F matrix was completed before the user requested headless-only continuation.

## Approval-input forensics

Historical identity:

- Native PID 12492; bundle `dev.gator.presence.finalcheck`.
- Wrapper PID 12626; conversation `work-20261120T033452-ec96d92e176b4efc`.
- Run `work-20261120T033715-9b901523bfc5`; interaction **1**.
- Exact token SHA-256 `d48c90d90c8abf913427ac564bacc4833dd31aca73c7e05f1023a3535add8a94`.

The bridge observation contains an `approval` frame with `approved: true` at line 864. The canonical Go interaction file independently contains `status: "resolved"` and `approved: true`. Retained replay includes the successful `web_access` tool result with the granted test origin; Work subsequently completed. These establish **command generation, Go receipt, acceptance and retained approval-state mutation**, not merely disappearance of a UI prompt.

The old controller lacked per-input event timestamps/source linkage. The raw `decisionSubmitted: false` field described controller intent and cannot refute the Go record. The triggering physical/AX/native control event is **UNKNOWN**. It cannot be assigned to another automation process, a user, an OS event or a specific button from the available logs.

Research counters are requests **0**, searches **0**, scope reviews **1**. Retained replay has no external-write call; the captured contract forbids external actions, the sandbox denies network commands and no connectors were selected. This supports **no recorded external dispatch**, separately from the accepted permission grant. No syscall-level historical network trace exists. Zero effects do not establish a negative approval result.

The historical negative is **INVALID**, not PASS. No unauthorized decision was reproduced in the new controlled actions; the historical accepted decision remains unattributed and therefore cannot be certified as authorized. Exact native and Go identity checks were not changed. [Audit and source-file hashes](evidence/p04e-1r/historical-approval-audit.json).

## New negative evidence

Runs 3, 4 and the aborted run 5 each have zero observed native-to-Go approval frames. Their durable interaction files remain pending, without an `approved` member, including after normal guarded shutdown. Both Go children in each fixture exited status 0; no recorded Go PID survived. These are useful facts, but an incomplete or contaminated sequence is not a passing exclusive-input test. [Native summary](evidence/p04e-1r/native-summary.json).

The new headless regression, `realGoPresentationNavigationAndShutdownDoNotDecidePendingApproval`, uses the actual candidate Go runtime and deterministic loopback provider. It holds unrelated work, creates an inert pending review in a separately owned task, then performs **20** owner-switch/review-open/focus/dismiss cycles. It checks:

- Exact review equality, conversation identity and actual Swift generation.
- Preservation of both owners' drafts and the two-owner count.
- No pending-decision submission state.
- Byte-identical canonical Go interaction JSON through every navigation cycle.
- Unconfirmed shutdown is refused while review/work is retained.
- Explicitly confirmed shutdown closes the owner without manufacturing an approval **or decline**; the durable pending record remains byte-identical.

It passed individually, in the complete suite and under strict Swift 6. This is **headless authority/lifecycle evidence**, not native Settings/SelectionOverlay acceptance.

## Resource measurements and sampler correction

A borrowed sampler labeled `ri_user_time + ri_system_time` as nanoseconds. On this Mac those counters require Mach-timebase conversion. A bounded, headless 250 ms CPU calibration measured:

- POSIX process CPU: **0.249838 seconds**.
- Raw counter divided by 1e9: **0.005996401 seconds**.
- Raw counter converted by measured **125/3** timebase: **0.249850042 seconds**.

The original raw JSON is preserved. Its `cpuNanoseconds` field is misnamed; the corrected summary converts those values. `native-summary-uncalibrated.json` is preserved explicitly as superseded, not valid CPU evidence. A corrected sampler is isolated under `acceptance/sample.c`; no previous mascot evidence/source was rewritten. [Calibration](evidence/p04e-1r/cpu-units.json), [corrected results](evidence/p04e-1r/native-summary.json).

| Partial run | Sampled duration | Max RSS | Lifetime peak physical footprint | Median CPU, one core |
|---|---:|---:|---:|---:|
| 3 | 27.1 s | 132.47 MiB | 53.52 MiB | 19.51% |
| 4 | 88.4 s | 157.70 MiB | 61.47 MiB | 24.25% |
| 5, aborted | 124.5 s | 137.38 MiB | 54.94 MiB | 23.06% |

These are **whole native-process** measurements, not incremental emoji cost or native-plus-Go totals. RSS and physical footprint are distinct. Launch samples before normal process initialization are not settled idle baselines.

Run 4's phase breakdown: pre-request idle, six samples, median **0.50% CPU / 46.53 MiB physical footprint**; pending-before-Settings, 15 samples, **23.14% / 52.45 MiB**; Settings interaction period, 50 samples, **24.62% / 58.85 MiB**. Only one post-Settings-before-Quit sample exists (**27.41% / 60.39 MiB**), insufficient to call settled. Working-only and post-GIF-settled phases were not separately resolved. [Phase measurements](evidence/p04e-1r/cycle-04-phase-metrics.json).

The higher pending/review CPU requires a matched baseline and native sampling before attribution to presence or a layout defect. It was already elevated before Settings opened. No unbounded growth is established by these short, unequal trials; nor do they establish the required absence of long-term growth. GPU/energy, observer/player retain counts and exact panel teardown instrumentation were not measured in the packaged app. Post-process Go PID checks passed; in-process lifetime tests pass separately.

## Native visual evidence and remaining checks

No new screen recording completed: the monitored batch aborted before recorder startup, then the user requested headless-only continuation. No screenshot or animation from a different binary is presented as new acceptance.

The preserved [P04E-1 live reaction recording](evidence/p04e-1/keyboard-live-reaction.mp4) and [pending review image](evidence/p04e-1/unique-pending-review.png) remain **historical visual evidence with the contaminated-session limitations above**. They do not prove safe negative approval behavior or resolution of the layout stall.

Still unverified natively in P04E-1R: matched B2 comparison; ten exclusive-input launch/settings/GIF/review/window-close/reopen/Quit/relaunch cycles; new settings/approval recording; settled post-GIF CPU; long-term native window/player/observer counts; compact/full/light/dark regression and reading-anchor/focus continuity through the complete sequence. Physical macOS 15, VoiceOver, IME and external-display acceptance remain **NOT TESTED**. No system preference was changed.

## Headless verification

All final commands use `GATOR_TEST_BINARY=/tmp/Gator-Native-P04E1R-Verified.app/Contents/Resources/gator-runtime` for real-Go fixtures.

```sh
swift build --package-path macos --scratch-path /tmp/gator-native-build
swift test --package-path macos --scratch-path /tmp/gator-native-build --no-parallel
swift test --package-path macos --scratch-path /tmp/gator-native-build --no-parallel \
  -Xswiftc -strict-concurrency=complete -Xswiftc -warnings-as-errors \
  --filter 'mascot|presence|desktopPet|desktopPresence|realGoPresentationNavigationAndShutdownDoNotDecidePendingApproval|approvalGuardsExactIdentity'
```

- Build: **PASS**.
- Complete final Swift suite: **261 tests PASS**, 143.000 s test time / 143.615 s command time.
- Strict Swift 6: **35 focused tests PASS**, 2.090 s test time. All package/test sources compiled with strict concurrency and warnings as errors.
- New negative Go-backed test: **PASS**, individual 1.737 s, then full/strict passes.
- Focused Go: **10 tests PASS across two packages**, interaction persistence/digest/run-lock plus exact desktop token/run identity, web-review identity, bound foreground review, deny/revoke/cancellation and private-DNS boundary tests.
- Controller/recorder: strict Swift 6 and explicit `arm64-apple-macos15.0` compilation **PASS**. This is not macOS 15 runtime acceptance.
- Python helper parsing and corrected C sampler/calibrator warning-as-error compilation: **PASS**.
- Host suite: **not rerun**; host source/contracts are unchanged. Historical 41-test evidence is not counted as a new run.

The full Swift suite retains the P01–P04, exact approval, archive cross-process/CAS, late-Quit and inherited-pipe shutdown assertions. No assertion was weakened. [Commands, durations and runtime hash](evidence/p04e-1r/verification.json), [full log](evidence/p04e-1r/swift-full.log), [strict log](evidence/p04e-1r/swift-strict.log), [Go exact authority tests](evidence/p04e-1r/go-exact-approval.jsonl), [Go interaction tests](evidence/p04e-1r/go-approval.jsonl).

Development verification failures are retained honestly: an initial full-suite attempt used an incorrect runtime environment-variable name and was interrupted, not counted; a corrected baseline 260-test run passed. The first new test assumed a second owner without first holding original work and timed out. The fixture was corrected to establish that scenario, matching the existing inert-review fixture; no production behavior or assertion was weakened. The final 261-test and strict runs passed after that correction.

## Changed files and reproduction

- `macos/Tests/GatorCoreTests/PresenceApprovalClosureTests.swift`: new actual-Go negative navigation/shutdown regression.
- `macos/presence/acceptance/Controller.swift`: nondecision AX allowlist, fixture/window/focus checks, passive input provenance.
- `fixture.py`, `Driver.py`, `Batch.py`: isolated profiles, timestamped transparent bridge observation, canonical pending-record checks, abort-on-interference and bounded reproduction sequence.
- `RecordFixture.swift`: fixture-application-only recording helper; compiled, but no completed new recording.
- `CheckCPUUnits.c`, `sample.c`: local CPU-unit calibration and corrected read-only sampler.
- `audit-history.py`, `summarize.py`, `verify.py`: read-only historical audit, evidence aggregation and pinned-runtime headless verification.
- This report and `evidence/p04e-1r/`: measured data, hashes, failures and explicit acceptance status.

No production application source was changed. The new test does not change the packaged executable. Earlier external/uncommitted changes were preserved.

Reproduce headlessly:

```sh
PYTHONDONTWRITEBYTECODE=1 python3 macos/presence/acceptance/audit-history.py
PYTHONDONTWRITEBYTECODE=1 python3 macos/presence/acceptance/verify.py
clang -Wall -Wextra -Werror macos/presence/acceptance/CheckCPUUnits.c -o /tmp/gator-presence-r1-cpu-units
/tmp/gator-presence-r1-cpu-units
```

The historical audit requires the retained disposable profile; the report stores its relevant findings/hashes. Future native runs require a new uninterrupted desktop session and fresh evidence directory. Compile the controller/recorder with Swift 6, strict concurrency, warnings as errors and target `arm64-apple-macos15.0`; compile `acceptance/sample.c` as `/tmp/gator-presence-r1-sample`, then run `Batch.py`. The batch is prepared for ten ordinary cycles, aborts on interference and creates a fresh timestamped evidence subdirectory. Its final rerun/abort-cleanup refinements were syntax-checked only, **not exercised after the user switched to headless-only**. A failed fixture must receive explicitly logged guarded cleanup; a killed fixture cannot count as Quit.

## Operator closure checklist

1. Pause every other GUI controller and manual input; record controller, app and child PIDs. Verify the fresh bundle hash and private profile before launch.
2. Establish the inert review; capture selected owner/generation and exact Go pending identity. Do not use real services or click either decision control in the negative sequence.
3. Run the A–F comparison and ordinary settings sequence, with readiness checks before each targeted input. Capture a main-thread sample immediately if responsiveness degrades. Also investigate the historical **post-accepted-decision** transition in a separately authorized positive fixture, never by silently approving the negative fixture.
4. Complete at least ten full native cycles, including window close/reopen, actual finite GIF completion, Tuck Away, normal guarded Quit and same-profile relaunch. Inspect Go command streams and durable records before shutdown and after relaunch; confirm no replay and no surviving owned children.
5. Capture a fresh application-only recording and matched resource phases using the calibrated sampler. Investigate the pending-review CPU against B2 and presence Off before attribution.
6. Retest full/compact layout, anchors, search/composer/review focus, static/Reduce Motion and explicit Off choices. Leave physical accessibility/platform variants untested unless actually exercised.

## Decision

**CONDITIONAL — not fully accepted.** The historical permission grant is conclusively established, but its input provenance is unavailable. The layout stall's root cause remains unresolved; no production remediation is claimed. New headless authority and safety tests pass, ordinary partial native actions did not reproduce the stall, and recorded fixture children terminated. The clean native negative test, ten-cycle lifecycle gate, matched resource comparison and visual closure remain incomplete after the explicit headless-only direction. The existing P1 investigation stays open. No new unauthorized decision was reproduced; the historical accepted, unattributed decision is not reclassified as safe.
