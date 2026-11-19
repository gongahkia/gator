# P04M-B1 — Optional desktop pet feasibility

## Decision

**CONDITIONAL.** A small, transparent, nonactivating AppKit pet is technically viable with the existing PNG/Core Animation player. Resource measurements pass the proposed B1 targets. Passive click-through, bounded interactive dragging, independent hiding, two simultaneous mascot presentations, and teardown were exercised. No production integration is authorized or included.

The condition is primarily UX: **the pet remained visible over the native fullscreen fixture**. `.fullScreenNone` did not suppress this all-Spaces floating panel on the tested OS. Production must either offer an explicitly accepted manual presentation-hide policy or validate a reliable suppression design before B2. Interactive click delivery worked, but foregrounding the lab controls from that click was not conclusively successful. Recommend passive interaction.

Recommended design for user approval: **one passive pet, placement B, fixed user-selected display with primary-display fallback, representing the selected conversation, default OFF**. Preserve independent in-chat visibility. Share style, animation preference, and OS Reduce Motion. Provide Show/Hide in ordinary app and menu-bar controls. These are proposals; no production preferences were added.

Prototype: `/tmp/Gator-Mascot-P04M-B1-Desktop-Pet-Lab.app`.

## Baseline, provenance and scope

Started at HEAD `b5991b5ac729945d783f69d5b28d433de39491f8` with a clean worktree. HEAD subsequently advanced externally to `d904ef6f08e5717398e6c970260a1a06eef83fcd` during this session; that commit includes intermediate B1 work. This session did not run a commit, reset, checkout, or revert. The final report and remaining lab edits supplement that state.

Read A1 and the accepted A2 2D report; inspected mascot model/playback/view/preferences, native scenes/window configuration, AppDelegate, notch geometry, tests, resources and build scripts. Production Gator is unchanged. The final hash check found **all 2,330 protected existing files and all files in 61 pre-existing development bundles unchanged**, including A1/live-3D evidence, the accepted P04 app and `/tmp/Gator-Native-P04M-2D.app`. See [preservation check](evidence/p04m-b1/preservation-check.json).

Host: Apple M3, Mac15,12, 16 GiB, arm64; macOS 26.5.2 (25F84); Swift 6.3.3; optimized standalone builds, SDK 26.5, deployment target **macOS 15.0**. Physical macOS 15 runtime acceptance is **NOT TESTED**.

The final bundle has the exact binary used in the successful final resource/lifecycle run: SHA-256 `845e66c7f763629f384899e09ef57f9ac0e42b12bd18860629bd471301931842`. Source and asset hashes are in [final manifest](evidence/p04m-b1/final-manifest.json). The bundle contains 118 original PNGs plus the manifest, 2,019,560 bytes; all bundled asset hashes match the accepted sources. It has no runtime dependency on the source checkout or a model file.

## Overlay architecture and component reuse

`PetPanelController` owns one `PetPanel`, one `NativeMascotCanvas`, and an optional bounded hit view. It directly compiles and reuses production `NativeMascotModel.swift` and `NativeMascotPlayback.swift`: `NativeMascotStyle`, `NativeMascotExpression`, `NativeMascotStateMachine`, `NativeMascotPlayer`, and `NativeMascotFrameStore`. It does not copy the animation implementation or regenerate assets.

The panel is borderless, transparent, nonopaque, shadowless, nonrestorable and nonactivating. It cannot become key or main. Showing or animating uses ordering, without application activation. Window level is ordinary `.floating` (3), below the existing notch panel's `.mainMenu + 1` (25). No custom elevated security-overlay level is used. This is not a universal claim about ordering relative to every third-party window or system dialog.

Collection behavior is `.canJoinAllSpaces`, `.transient`, `.ignoresCycle`, `.fullScreenNone`. The lab does not change production Gator's activation policy. Its own normal control app has a standard app menu and a status-menu escape route. No global event monitor is installed.

All controller/view mutation is MainActor-isolated. Existing frame decoding remains in the accepted frame-store actor. Requests retain the existing generation/cancellation checks and finite Core Animation clips. There is no added high-frequency application playback timer. The 1 Hz sampler and scripted stimulus tasks exist only for lab measurements. Observers use weak callbacks and are removed during teardown.

Dynamic-library inspection found AppKit/Foundation/ImageIO/QuartzCore and normal Swift overlays; no RealityKit, SceneKit, WebKit, GatorCore or Go runtime dependency. No agent, network client, prompt input, approval action, conversation storage, or credential facility is linked by this prototype.

## Passive versus interactive

| Behavior | Passive | Interactive |
|---|---|---|
| Pointer handling | `ignoresMouseEvents = true` | Entire 72 × 72 pt rectangle intercepts input |
| Keyboard focus | Cannot become key/main | Cannot become key/main; menu alternative remains available |
| Native test | Center click reached underlying fixture button | Click reached the explicit open-fixture callback |
| Dragging | None | Local mouse events; observed frame move from x=699 to x=705, without activation |
| Click action | None | Orders/requests activation of **lab controls only** |
| Hide | Main/status-menu Tuck Away | Same, plus contextual Tuck Away |
| Remaining gap | Decorative surface can visually overlay underlying text | Successful foreground handoff is unverified; rectangular transparent padding also intercepts input |

The interactive hit area is **5,184 square points**, not a silhouette mask. A click approximately 4.3 pt outside its moved left edge was posted within the underlying fixture; its click counter subsequently advanced. Dragging is clamped to the selected display and clear of the expanded notch envelope. The observed test establishes a short drag, not arbitrary multi-display repositioning. No dragging preference or position persistence was implemented.

The click callback was logged, but the inert peer remained the frontmost application in the subsequent sample. That may involve synthetic-event activation restrictions; the evidence does not establish an AppKit production defect or a successful open-chat experience. **Do not accept interactive focus handoff on this evidence.**

**Recommendation: passive.** Its click-through behavior is verified and avoids a floating rectangular control over other applications. Production interactive behavior would need a separate approved policy and physical/native focus acceptance.

## Placement and existing notch surface

Geometry uses `NSScreen.frame`, `visibleFrame`, safe-area insets and auxiliary top areas; it intersects usable rectangles, respects negative origins, and clamps the complete panel. Apple's [safe-area documentation](https://developer.apple.com/documentation/appkit/nsscreen/safeareainsets) supports treating the camera area separately from ordinary screen bounds. No physical notch pixels are inferred from screenshots.

The built-in test display reported frame `(0,0,1470,956)`, visible frame `(0,0,1470,923)`, safe top 32 pt, scale 2, and camera gap 179 pt. Current production notch geometry yields the expanded envelope `(525,710,420,246)` in AppKit coordinates.

| Placement | Actual experiment | Tradeoff |
|---|---|---|
| A — below menu bar | Top-center panel `(699,839,72,72)` | Closest to requested top center, but competes with an expanded notch panel |
| B — clear quick access | Top-center panel `(699,626,72,72)` | Always reserves the existing expanded notch envelope plus 12 pt; lower than A |
| C — offset on notch | Moves to the side of the existing notch envelope, with below-envelope fallback on narrow displays | Closer to top edge, but no longer centered |

B is the simplest independent policy: no production notch changes or cross-controller ownership. Its cost is conspicuous vertical clearance even when quick access is collapsed. A future coordinated A policy could tuck the pet while quick access expands, but that is not implemented or implicitly approved here.

Actual native screenshots, filtered to disposable windows:

- [A: menu-adjacent](evidence/p04m-b1/A-placement-final.png).
- [B: center below reserved region](evidence/p04m-b1/top-center-B-final.png).
- [C: offset, Classic style](evidence/p04m-b1/C-placement-final.png).
- [Accepted Gator, existing expanded notch surface and separate pet](evidence/p04m-b1/with-gator-and-notch.png).
- [Gator chat closed, pet still visible](evidence/p04m-b1/chat-closed-pet-visible.png).

The last two are visual coexistence checks only. The copied accepted Gator binary/resources were unchanged; its bundle identifier and private profile were disposable. Shared desktop input entered unexpected fixture messages and caused two failed task admissions. The saved failures identify **connection refused at `http://127.0.0.1:9/responses`**; only an inert credential was provided. See [fixture isolation record](evidence/p04m-b1/visual-fixture-isolation.json). This session is **not** evidence of an interference-free composer/task interaction test. The standalone pet has no Go admission capability. No real account or paid endpoint was used.

## Displays, Spaces and presentation safety

The lab selects a fixed display by the Core Graphics display UUID and recomputes frames on screen-parameter changes. It defaults to the built-in display for reproducible tests. The proposed production fallback is the primary display when a chosen UUID disappears; no absolute coordinates are persisted. There is only one pet controller, not one per screen.

Deterministic tests cover negative origins, a display above the primary, portrait-sized geometry, multiple scales, missing auxiliary camera areas, tiny/unusable displays, no screens, preferred-display lookup and removed-display fallback. During the session macOS also reported an external 1920 × 1080 screen at `(-714,956)`, scale 1. **No controlled physical external-display selection, unplug/replug, rearrangement or migration acceptance was performed.** Simulated geometry is not physical multi-monitor acceptance.

The native fullscreen fixture was actually entered and exited. The panel remained visible: [fullscreen evidence](evidence/p04m-b1/fullscreen-final.png). Apple's [collection-behavior reference](https://developer.apple.com/documentation/appkit/nswindow/collectionbehavior-swift.struct) is not evidence of universal fullscreen suppression; the observed result governs this prototype. Do not ship a claim that `.fullScreenNone` alone hides this overlay in fullscreen apps.

Ordinary independent Spaces traversal and Mission Control presentation are **NOT TESTED**. `.transient` expresses the intended Mission Control treatment, not desktop acceptance. Screen sleep/wake suppression is wired to native notifications and simulated in policy tests; a physical sleep/wake trial is **NOT TESTED**. Non-native fullscreen games, presentation apps, real screen sharing and recording exclusion are **NOT TESTED**.

No screen-sharing detector or privileged monitoring is added. Do not promise universal suppression during sharing or recording. Proposed production policy: an explicit, easily accessible Tuck Away/presentation-hide control, with fullscreen behavior resolved before B2. Screen-recording and Accessibility permissions were already available for the **test tools**; the pet itself does not request them. No system settings were changed.

## Public state and user-control proposal

The lab feeds the actual accepted reducer with explicit fixture values, including owner changes, working, public-text revision, confirmed completion ID, attention and disconnect. It is not connected to real Go streaming. Existing precedence is reused: disconnected, then pending attention, then responding/working, with completion only on a new confirmed outcome for the same owner. Owner replacement resets the prior owner's transition identity.

Recommend representing the **selected conversation**, matching the in-chat mascot. App-wide attention remains in the existing explicit surfaces, including notch quick access. Do not silently merge background owners' private details or make their completion impersonate the selected chat. Future B2 can observe the existing public projection; no additional Go owner is needed.

The pet contains no message text, filename, secret, provider setting or task trace. Its gestures cannot send, approve, replay, start plans, grant access or configure providers. Explicit status and review controls remain in Gator.

Proposed settings, not production implementation:

| Setting/policy | Proposal |
|---|---|
| Show Desktop Pet | Separate, persisted OFF by default |
| In-chat Show Mascot | Existing independent setting; all four visibility combinations supported by policy tests |
| Appearance | Reuse current rendered/Classic style |
| Animation | Reuse Animate setting; OS Reduce Motion wins |
| Display | One explicitly chosen stable display ID, primary fallback |
| Tuck Away | Persist OFF until explicit reenable; never wake on work completion/relaunch |
| Escape route | Both normal app controls and status menu; pet is never the only hide target |

The lab uses **memory-only** desktop settings, defaults OFF every ordinary launch, and provides explicit test flags. JSON encode/decode tests demonstrate the proposed OFF policy can survive a roundtrip; they are **not a claim of implemented persistent preferences**. Reconciliation never reenables a tucked pet. Main/status-menu hiding was exercised in passive and interactive modes, with exit code 0. Closing chat is independent of the pet's enable state.

## Resource measurements

Same machine, optimized source, assets and panel/player configuration. `TASK_VM_INFO` provides resident bytes, physical footprint, kernel lifetime peak and the app's graphics ledger; `getrusage` gives cumulative user/system CPU. CPU is whole-lab percent of **one core**, including the 1 Hz sampler and atomic JSON diagnostics. Each phase is 6–14 seconds; table memory is the median of per-second samples. This is a bounded feasibility trial, not an energy study or a whole-Gator performance benchmark.

The **final 139.97-second run** completed 66 native assertions and exited 0. [Raw samples](evidence/p04m-b1/final-run/verified-trial/samples.ndjson), [state/events](evidence/p04m-b1/final-run/verified-trial/state.json), [process exit](evidence/p04m-b1/final-run/verified-trial-exit.json), [derived table](evidence/p04m-b1/resource-summary.json).

| Phase | Total RSS MiB | Total footprint MiB | Footprint delta vs host MiB | CPU % |
|---|---:|---:|---:|---:|
| Host, pet/in-chat absent | 93.16 | 24.47 | 0 | 0.57 |
| Pet static | 87.16 | 24.66 | +0.19 | 0.19 |
| Idle, including occasional blink | 86.69 | 24.63 | +0.16 | 0.21 |
| Finite working gesture, then settled | 86.73 | 24.72 | +0.25 | 0.12 |
| Rendered responding pulses | 83.47 | 25.89 | +1.42 | 1.20 |
| Classic responding pulses | 83.31 | 25.86 | +1.39 | 1.00 |
| Animation disabled | 83.34 | 25.88 | +1.41 | 0.06 |
| Simulated Reduce Motion | 83.30 | 25.83 | +1.36 | 0.05 |
| Tucked, while fixture signals continue | 85.98 | 25.22 | +0.75 | 0.35 |
| Pet + in-chat fixture | 86.58 | 25.47 | +1.00 | 0.94 |
| Pet animating while app inactive | 86.56 | 25.45 | +0.98 | 0.96 |
| All pet-owned objects released | 87.09 | 25.52 | +1.05 | 0.14 |

Peak kernel physical footprint was 27.94 MiB, **+3.47 MiB** above the matched host median; highest sampled footprint was 26.11 MiB. RSS deltas were negative as startup pages aged out, so they do not establish negative or zero pet cost. They are preserved separately in the JSON; RSS and footprint are not interchangeable. Settled framework/native caches can remain after application objects are gone.

Earlier comparable runs in `resource-trial/` and `verified-trial/` show short-trial variability: approximately 0.25–0.28% idle, 1.83–1.95% rendered responding, 1.56–1.70% Classic responding, and 2.40–2.92% with both slots. The earlier measured peak incremental footprint was approximately 4.23 MiB. Do not present any one short CPU phase as a precise universal constant.

One frame store is capped at **3 MiB**, with at most one selected clip. Maximum observed cached/active clip was **2.296875 MiB**; two presentation stores have a combined 6 MiB cap. After Tuck Away every sampled pet animation-key count and decoded-cache byte count was zero; continued fixture signals caused no animation restart. Hidden sampler CPU is not hidden animation activity. Idle uses only finite occasional gestures, not a permanent drawing loop.

[Latency measurements](evidence/p04m-b1/latency.json): first panel show through layer-image readiness **41.10 ms**. Ten neutral-to-working submissions per style: rendered median **1.30 ms**, Classic **1.97 ms**; maxima 9.01 and 7.09 ms. These measure decode/application submission, **not** compositor presentation, frame-perfect timing, or production app startup. OS Reduce Motion was false; the latency probe refuses to animate if it is true.

The proposed 15 MiB settled / 25 MiB activity footprint goals pass in these trials without inheriting a live-3D budget. GPU utilization, WindowServer/compositor energy and wattage are **NOT TESTED**. App graphics-ledger values and low CPU do not prove low GPU energy. No Instruments energy result is claimed.

## Ownership, teardown and lab corrections

Each lifecycle run performs 12 create/destroy cycles, changes style/placement, hides/shows, and releases controller, panel, hit view, canvas and player. Weak probes do not strongly retain their subjects. Frame decoding cancellation and hidden cache clearing use the existing player behavior.

An initial assertion found one empty closed panel still alive after the controller/player/canvas had disappeared. Diagnostics showed `NSApp.currentEvent.windowNumber` referred to that panel. After dispatching one marked **app-local** mouse-move event to the lab controls through the native application loop, the last panel disappeared too. Final run: last pet event window 52592, then control window 52485; **zero surviving weak subjects**. This is observed bounded AppKit event retention, not evidence of unbounded mascot ownership. No global event interception or private API is involved.

An earlier lab shutdown using `.terminateLater` did not exit despite successful teardown. Only the **isolated lab delegate** was corrected to cancel/await its owned trial task, clean up once, then explicitly reenter termination with a ready flag. The final 139.97-second run and a separate 25.67-second/60-assertion lifecycle run exited successfully. Final full-trial cleanup completed in approximately **2.8 ms**. The earlier timeout logs remain as failed historical evidence; they are not reported as passing Quit tests. Production Gator's guarded Quit implementation was not changed.

Observers are removed, status item removed, finite animation/decode tasks stopped, content views detached and windows closed. Final process checks found no running pet/peer/latency probe. No orphaned overlay remained. This bounded result is not a proof of indefinite leak freedom.

## Native evidence and accessibility

- [18.93-second actual native recording](evidence/p04m-b1/native-motion-accepted.mp4), 1,470 × 956, 524 frames, no audio. It records the actual Core Animation panel with explicit fixture signals, not Go-backed inference.
- [Jaw motion contact sheet](evidence/p04m-b1/jaw-motion-contact-sheet.png), sampled at 10 Hz from two seconds of that recording. Distinct jaw positions are visible. The slower [state contact sheet](evidence/p04m-b1/motion-contact-sheet.png) alone would alias short gestures and is not used as proof of every animation frame.
- [Native acceptance events](evidence/p04m-b1/native-acceptance.json) and [guarded input transcript](evidence/p04m-b1/native-acceptance-retry.log).
- [Interactive drag](evidence/p04m-b1/drag-final.png), [passive tucked](evidence/p04m-b1/B-passive-hidden-final.png), [interactive tucked](evidence/p04m-b1/B-interactive-hidden-final.png).

Screenshots/recording use ScreenCaptureKit with an explicit allowlist of disposable lab/peer/Gator-fixture windows, neutral replacement background, no cursor or audio, and no unrelated application windows. These are actual captured windows in display coordinates, not schematic mockups. Filtering and display reconfiguration can affect captured framing; numeric placement comes from native panel/screen metadata. The physical camera cutout itself is not rendered into these captures.

Passive center clicks reached the peer without focusing Gator. Peer text/empty secure-field focus and app inactivity were observed during animation. Interactive drag did not activate the lab; interactive click focus remains partial. No real password was entered. The panel is decorative in passive mode; interactive mode has a labeled action and a standard menu alternative. Reduce Motion and static playback assertions pass. Physical VoiceOver, IME, actual password-manager behavior, and broader OS accessibility preference variants are **NOT TESTED**. Accessibility metadata is not VoiceOver acceptance.

Earlier desktop probes saw stale element references, focus changes and unexpected shared input. Drivers were changed to retain their own fixture processes, verify PID/window/focus before posting input, await observable state, and perform capture sequentially. SCK probes with a prohibited capture-app activation policy failed and were reverted. Rejected/stale probes are retained as diagnostic logs, not counted as application failures or passes. Production keyboard shortcuts were not changed to accommodate automation.

## Tests, compatibility and reproducibility

Final commands from repository root:

```sh
bash macos/mascot/desktop-pet-lab/verify.sh
GATOR_PET_LAB_APP=/tmp/Gator-Pet-B1-Packaged-Final.app \
  bash macos/mascot/desktop-pet-lab/build.sh
PET_APP=/tmp/Gator-Pet-B1-Acceptance-Final.app \
PET_EVIDENCE=macos/mascot/evidence/p04m-b1/final-run \
  python3 macos/mascot/desktop-pet-lab/tools/trial.py
xcrun vtool -show-build /tmp/Gator-Mascot-P04M-B1-Desktop-Pet-Lab.app/Contents/MacOS/PetLab
otool -L /tmp/Gator-Mascot-P04M-B1-Desktop-Pet-Lab.app/Contents/MacOS/PetLab
bash -n macos/mascot/desktop-pet-lab/build.sh macos/mascot/desktop-pet-lab/verify.sh
git diff --check
```

Build destinations must be fresh; the build script refuses overwrite. The final packaged binary matches the measured Acceptance-Final binary. Older lab bundles were preserved before promoting the final copy to the requested path.

`verify.sh` creates a fresh `/tmp/gator-pet-b1-check.*` directory containing logic/player checks, native latency tool, allowlisted Control/Pointer/Capture tools, and a packaged inert `FocusFixture.app`. It uses `-swift-version 6 -strict-concurrency=complete -warnings-as-errors -O -target arm64-apple-macosx15.0` throughout. For another controlled trial set `PET_APP` to a freshly built lab, `PET_PEER_APP` to that generated fixture app, and `PET_EVIDENCE` to a fresh evidence directory. The fixed-path `acceptance.py` driver records the exact desktop sequence; inspect its fixture/tool paths before reusing it.

| Verification | Result |
|---|---|
| Headless geometry/policy/owner reducer | **63 assertions PASS**, zero windows/backend processes |
| Existing A2 player/model checks, unchanged source | **63 assertions PASS**, zero windows/backend processes |
| Strict Swift 6 + macOS 15 target, lab and tools | **PASS** |
| Final native resource/lifecycle trial | **66 assertions PASS**, 139.97 s, exit 0, zero weak subjects after event dispatch |
| Separate final teardown/Quit probe | **60 assertions PASS**, 25.67 s, exit 0 |
| Native passive click, drag, Tuck Away, both styles | **PASS** for the specific observed behaviors above |
| Interactive foreground handoff | **PARTIAL** |
| Automatic fullscreen suppression | **FAIL** for the tested collection-behavior policy |
| Physical macOS 15 runtime | **NOT TESTED** |
| Shell syntax / diff whitespace | **PASS** |
| Production Swift/Go/host suites | **NOT RERUN**, production unchanged; permitted by B1 test policy |

Logs: [final compilation/checks](evidence/p04m-b1/final-headless.log), [app build](evidence/p04m-b1/final-build.log). Initial verification-script attempts had an inappropriate library flag for the top-level AX helper, unsafe-cast diagnostics, then omitted fixture geometry inputs. The helper now has an explicit MainActor entry point and appropriate downcasts; compile inputs were corrected. Those failed attempts remain in `verify-script-*-attempt.log`; no assertion was removed or weakened. macOS 15 compilation uses the installed newer SDK, not an older runtime. No obsolete high window-level or private fullscreen API was introduced.

## Exact changes and remaining decisions

New files relative to the initial baseline, all isolated under `macos/mascot/`:

- `desktop-pet-lab/Geometry.swift`, `Screens.swift`: deterministic safe placement/display identity and proposed policy.
- `desktop-pet-lab/Panel.swift`: passive/interactive native panel experiment using the existing player.
- `desktop-pet-lab/Lab.swift`: controls, public fixture signals, sampling, observers, weak lifetime probes, lab-only Quit.
- `desktop-pet-lab/CheckLogic.swift`, `FocusFixture.swift`, `Latency.swift`: deterministic checks and inert native fixtures.
- `desktop-pet-lab/build.sh`, `verify.sh`: isolated packaging and strict/headless verification.
- `desktop-pet-lab/tools/Control.swift`, `Pointer.swift`, `Capture.swift`: allowlisted native evidence tools.
- `desktop-pet-lab/tools/acceptance.py`, `trial.py`, `scenarios.py`, `gator-visual.py`: disposable controlled drivers; exploratory failures remain documented.
- This report and `evidence/p04m-b1/` captures, recordings, raw samples, JSON, manifests and logs.

No production panel/preferences, app icon, notch interaction, Go execution, credentials, archive, approval, plan, recovery, queue or native shutdown code was edited. No assets were regenerated. No new dependency, signing, notarization or distribution step was performed.

Before B2 the user must choose:

1. **Position:** recommend B's reserved center clearance; approve its extra vertical offset, or select an explicitly coordinated alternative.
2. **Interaction:** recommend passive; interactive foreground handoff needs further acceptance if chosen.
3. **Display:** recommend one user-selected stable display with primary fallback; physical multi-monitor behavior remains to validate.
4. **Fullscreen/sharing:** explicitly accept manual Tuck Away/presentation hide, or require validated automatic suppression. No universal sharing detector is promised.
5. **Preferences:** approve independent default-OFF desktop enablement, shared style/motion, and persistent Tuck Away until explicit reenabling.

B1 concludes with a viable isolated prototype and an explicit **CONDITIONAL** production recommendation. It does not authorize B2, P05 or P11.
