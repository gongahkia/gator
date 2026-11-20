# P04E-1 — Emoji-driven agent presence

## Outcome and scope

Implemented native Unicode emoji presence in the existing in-chat and passive desktop presentations. The two previous character styles are no longer selectable or loaded by the production app. Original 3D/2D authoring assets, prototypes, evidence and previously accepted apps remain preserved.

The implementation passed the full **260-test Swift suite**, plus **33 focused tests with strict Swift 6 concurrency checking and warnings treated as errors**. Final native acceptance details and limitations are recorded below. This increment changes presentation only: Go execution, provider credentials, approvals, task ownership, archives, recovery, workspace scope and shutdown authority are unchanged. No scheduling, push notifications, P05 or other feature increment was started.

Initial HEAD was `8052f0c2a50809bf04c9eb070a4c069fec785663`, with the preceding L1 reports/evidence and a test-only audit refinement already present. HEAD advanced through external commits during the task; this session made no commits, resets or checkouts. The [final scope manifest](evidence/p04e-1/final-manifest.json) identifies the exact source and bundle hashes and preserves those external changes.

## Public-state mapping

The existing `NativeMascotProjection`, value signal and reducer remain the presentation boundary. Internal class names are retained to avoid unrelated renaming. The view observes only the selected owner's context/generation, canonical connection/running/dialogue status, bounded public text activity, pending review/plan-review flags and finished public Work outcomes. It does not parse model text, tool arguments, credentials or hidden reasoning.

| Expression | Emoji | Required public evidence |
|---|---|---|
| Idle | 🐊 | Connected, no higher-priority projected state |
| Working | ⚙️ | Go running or dialogue busy, without an active published text entry |
| Writing | 📝 | Working plus a public `kind=text`, `state=running` entry with nonempty public text |
| Attention | 👀 | Actual pending approval, opened exact review or projected plan phase `review` |
| Completed | ✅ | New same-owner completed public activity with nonempty run ID and finish time |
| Failed | ⚠️ | Latest confirmed failed public activity with run ID and finish time, when not working/reviewing |
| Disconnected | 🔌 | Selected owner disconnected |

Planning, researching and waiting are **not invented**: the selected projection does not provide the distinct evidence required for those expressions. Generic work is used instead. Public text closure returns writing to generic work. A stopped outcome suppresses an older completion/failure rather than celebrating it. A new owner/generation does not replay an historical completion. A completion denotes the confirmed Work outcome; existing textual results/evidence remain authoritative for what was actually accomplished.

Precedence: disconnected → attention → active public writing → work → confirmed failure → new confirmed completion → idle. Completion is not inferred from a timer, an empty queue, transport acknowledgement or the disappearance of a spinner. The reducer can retain the latest completion expression until a new relevant state; a GIF reaction itself is finite and never loops. Background owners cannot replace the selected owner's presence.

## Native rendering and finite GIFs

Ordinary states use an unselectable native `NSTextField` and Unicode emoji, with system font fallback. They are not prerendered emoji sprites. The existing AppKit canvas, actor-owned frame store, ticket/cancellation guards, Core Animation player and animation delegate are reused. No RealityKit, SceneKit, WebView, model asset loading, remote GIF service or new animation engine was added.

The optional reaction mode adds an original geometric accent around the native emoji for **new attention and completion transitions only**. The GIF does not replace the emoji with a character image. Work/writing/failure/disconnection remain plain emoji. No inferred speech or token-by-token animation is present.

- [Editable original reaction exporter](source/ExportReactions.swift): Core Graphics drawing of eight restrained expanding accent dots; no downloaded art or rasterized Apple emoji.
- [Completion GIF](../Resources/Presence/completed.gif): 1,818 bytes.
- [Attention GIF](../Resources/Presence/attention.gif): 2,179 bytes.
- Each has 12 logical 96×96 frames at 0.06 s/frame, 0.72 s total. Transparent first/last frames and borders were verified; both GIFs total **3,997 bytes**.
- Core Animation submits one discrete contents animation with `repeatCount=0`; it ignores any file looping convention. Completion clears the animation, retained frame array and layer image. The native emoji remains the static fallback.
- The actor loads only one allowlisted reaction. Bounds: 256 KiB compressed input, 2–16 frames, ≤128×128 dimensions, ≤1 MiB decoded frames and ≤1.6 seconds. The current clip decodes to 442,368 bytes (0.422 MiB); this is a frame-array bound, not total application memory.
- New state, owner, mode, visibility, animation setting or Reduce Motion cancels old playback and clears its cache. Stale decode/completion tickets cannot replace a new state. Reopening or returning from hidden does not replay a previous reaction.
- Emoji-only/static/Off requests decode no assets. There is no idle blink timer or high-frequency application timer. Repeated public text updates in the same nonreactive state do not schedule additional animation work.

The offline exporter compiled with strict Swift 6 and an `arm64-apple-macos15.0` target; a fresh export produced byte-identical GIFs. [Asset/alpha measurements](evidence/p04e-1/assets.json), [reproducibility](evidence/p04e-1/export-reproducibility.json).

## Preferences and conservative migration

Settings and the existing Conversation/compact menus now say **Agent Presence**. The choices are **Emoji only**, **Emoji + GIF reactions**, and **Off**. Emoji only is the new default.

Separate **Show In-Chat Presence** and **Show Desktop Presence** controls remain independent. Desktop defaults to OFF. Global Off suppresses both presentations without rewriting either visibility choice. Returning from Off restores only those existing choices; it does not enable a previously tucked desktop presence.

Existing keys `gator.mascot.shown`, `gator.mascot.animated`, `gator.desktopPet.shown` and `gator.desktopPet.display` are reused. Thus explicit old Show OFF, Animate OFF, Tuck Away and display selection survive without bulk deletion or a migration write. The new mode key is `gator.presence.mode`. Old `gator.mascot.style` values may remain stored but are not read by production presentation. Unknown/obsolete mode strings fall back to emoji, never legacy character art.

**Animate Reactions** retains the previous explicit animation setting. Choosing the reaction mode does not silently override a prior Animate OFF. macOS Reduce Motion overrides animation and keeps the corresponding static emoji. No preference enters Go agent state or conversation/draft archives, and no system-wide setting is changed.

## Layout, accessibility and desktop policy

The in-chat slot remains slightly offset above the transcript, outside the scrolling message rows. It is now **48 points full-window / 32 points compact**, replacing the old large header. Normal view sizes are 42 / 28 points; reading/review/constrained sizes are 26 / 20 points. Native glyph size is scaled within those views. Collapsing changes the glyph size inside a stable slot, not the transcript viewport height. No presence code writes a reading anchor, invokes Jump to Latest, changes selection or requests focus.

The desktop panel retains its 72×72-point, notch-safe, selected-display geometry and existing nonactivating/click-through policy. No click, drag or approval action was added. One selected conversation is represented. The controller still releases its surface when disabled, sleeping, closing or unable to place it safely; teardown removes observers and playback. Missing displays fall back without overwriting the selected display identity.

Manual fullscreen/screen-sharing hiding remains the existing policy: **Tuck Away** is available from the main controls and persists until explicit re-enabling. Universal screen-sharing detection and automatic fullscreen suppression were not added.

The emoji/canvas is decorative, not focusable and accessibility-hidden; its text field is not selectable. Existing visible status, task/result and exact review text remains authoritative. There are no per-frame VoiceOver announcements. Physical VoiceOver, IME, Increased Contrast and actual OS Reduce Motion toggling are NOT TESTED; Reduce Motion request handling and cancellation are covered headlessly.

## Preservation and packaging

The new app packages `Contents/Resources/Presence/{attention,completed}.gif`, not `Contents/Resources/Mascot`. A runtime-source scan found no remaining old style selection or mascot-frame root. The old frame resources are still used by historical authoring/lab material and remain in the repository, not in the newly packaged runtime. Additional pre-change copies of the four implementation files and original mascot tests are under `macos/mascot/2d/legacy-p04e/` for reference. Older standalone labs should use those archived implementations if reproducing the old API; they are not current production targets.

Application/menu-bar icons remain unchanged and actively used; the accompanying generated `GatorHead.png` branding file is also retained unchanged; this task does not replace those icons. There is no runtime checkout path or network dependency for emoji/reaction presentation. Missing reaction assets leave the native emoji and chat usable.

The final development app is `/tmp/Gator-Native-P04E-Emoji.app`. The verified candidate is promoted without changing its executable/resources. The earlier candidate is preserved separately, along with accepted P04, A2 and B2 bundles. Minimum binary deployment target remains **macOS 15.0**, built on macOS 26.5.2; physical macOS 15 runtime acceptance is NOT TESTED.

## Headless verification

Final commands (the historical `…-Verified.app` staging path was subsequently promoted byte-for-byte to `/tmp/Gator-Native-P04E-Emoji.app`; use that final path when repeating tests):

```sh
swift build --package-path macos --scratch-path /tmp/gator-native-build
GATOR_TEST_BINARY=/tmp/Gator-Native-P04E-Emoji-Verified.app/Contents/Resources/gator-runtime swift test --package-path macos --scratch-path /tmp/gator-native-build --no-parallel
swift test --package-path macos --scratch-path /tmp/gator-native-build --no-parallel -Xswiftc -strict-concurrency=complete -Xswiftc -warnings-as-errors --filter 'mascot|presence|desktopPet|desktopPresence'
GATOR_NATIVE_APP_PATH=/tmp/Gator-Native-P04E-Emoji-Verified.app bash macos/scripts/build-dev-app.sh
```

Final full suite: **260 tests PASS**, 139.021 seconds test time, 148.315 seconds command wall time. Strict focused run: **33 tests PASS**; all package/test sources compile with strict concurrency and warnings as errors. The candidate package build passed. [Exact commands/exits](evidence/p04e-1/final-verification.json), [full log](evidence/p04e-1/full-final.log), [strict log](evidence/p04e-1/strict-final.log), [package log](evidence/p04e-1/package-final.log).

The earlier 255-test baseline expanded to 260. Obsolete product tests for offering two character styles/decoding their production PNG clips were replaced with the new mode, migration, GIF-bound/cancellation and no-old-loader contracts; the original tests were archived. Owner isolation, completion, reading positions, no process creation and desktop lifetime assertions remain covered. No approval/archive/quit/shutdown assertion was weakened. The full pass includes the existing P01–P04 and P0/P1 engineering safety regressions.

Early new tests incorrectly required an unattached headless CALayer to retain a running animation after an actor hop. A standalone reproduction observed submission followed by `animationDidStop(finished:false)` on the unattached layer. Deterministic tests now capture the submitted finite animation through a narrow installer seam, rather than depending on render-server scheduling. They still assert exact keyframes, bounded retained bytes, cancellation and cache release. Actual visible playback requires native evidence, separately reported below. A floating-point duration equality was corrected to a 1 ms tolerance. Earlier failed logs are retained; those attempts are not reported as passes.

Go and host suites were not repeated for this presentation increment: their source/contracts and GatorCore remained unchanged. The complete Swift suite exercised the bundled real-Go fixtures using the candidate runtime. The prior L1 Go results are historical evidence, not relabeled as new P04E results. No paid key/provider was used; native acceptance used a disposable state/config, inert key and loopback-only fixture service.

## Actual native evidence and acceptance

The exact packaged executable/resources were tested in disposable bundle copies with a fixture bundle identifier. A later uniquely named copy additionally changed only bundle display/name metadata to separate it from other desktop automation. Neither copy changed production binaries or resources. Native acceptance tooling is allowlisted to those fixture identifiers and disposable paths. It does not request new accessibility/screen-recording permissions or capture unrelated application windows. System-wide Apple-menu contents are omitted from retained AX evidence.

**Final verdict: implementation/headless verification PASS; native acceptance PARTIAL because of the unresolved full-app layout stall described below.** Captured background-window contents are not proof that a hidden presence was visibly animating; window occlusion intentionally stops and clears its presentation. Shared-desktop focus interference, unexpected typed draft/query text and missing/moved windows are documented separately from demonstrated application defects.

| Native check | Result and evidence |
|---|---|
| Full-window light | PASS for final candidate writing/review layout: [writing](evidence/p04e-1/unique-working.png), [review](evidence/p04e-1/unique-pending-review.png). The older `full-light.png` is an **initial candidate** image, not final state-mapping evidence. |
| Full-window dark | PASS for final-candidate idle glyph: [dark foreground](evidence/p04e-1/dark-front-emoji.png). Background-window captures can omit the intentionally paused glyph. |
| Compact | PASS for final-candidate 440×620 idle presentation: [compact light](evidence/p04e-1/compact-light.png). Transient GIFs in compact chat were not separately recorded. |
| Desktop | PASS for visible native emoji: [desktop idle](evidence/p04e-1/desktop-emoji.png), [attention static](evidence/p04e-1/desktop-attention-static.png). No new pointer action was added; the established click-through panel policy is unchanged. A new physical click-through/monitor-migration test was not repeated. |
| Go-backed work and writing | PASS: real bundled Go plus inert loopback provider produced running Work and active public text; the writing glyph is visible in the final recording/screenshot. Generic cog mapping is tested headlessly; its brief pre-text interval was not separately accepted as a native still. |
| Confirmed completion and real finite GIF | PASS: [native recording](evidence/p04e-1/keyboard-live-reaction.mp4), [contact sheet](evidence/p04e-1/reaction-contact-sheet.png), [motion measurements](evidence/p04e-1/native-motion-verification.json). Writing changes to ✅; accent dots expand and disappear, leaving a static checkmark. |
| Pending exact approval | PASS: Go created the inert web-access request; native review shows small 👀 with the exact owner and decision controls. [Review](evidence/p04e-1/unique-pending-review.png). **The harness requested no decision, but the final bridge audit found one unattributed `approved: true` frame after the captured review. Presentation passed; the no-decision assertion is CONTAMINATED.** |
| Static fallback / separate visibility | PASS for observed case: Animate Reactions OFF, in-chat OFF, desktop ON preserves a static desktop 👀. Headless tests additionally cover missing/corrupt GIFs and Reduce Motion. |
| Settings and persistence | PASS for observed mode selection and relaunch: [settings](evidence/p04e-1/settings-light.png), [reopened AX state](evidence/p04e-1/settings-relaunch.ax.txt). Emoji + GIF selection persisted. |
| Tuck Away / Quit / relaunch | PASS in the first final-candidate profile: native Tuck Away followed by successful termination and dark relaunch left the desktop flag false and no desktop window. A later pending-review preference sequence stalled; guarded Quit in that instance was **not confirmed**. |
| Keyboard/focus | PASS for the uniquely named fixture's focused select-all/type/send sequence and ordinary composer focus while observing Work; decorative presence has no key responder. Earlier Find/focus probes were interfered with and are PARTIAL, not proof of a new focus defect. |
| Reading/review collapse | Headless exact-anchor/stable-slot and full prior reading regressions PASS; final native review visibly shrinks the glyph. A controlled older-history anchor/Jump-to-Latest round trip was not completed in this desktop session. |
| Global Off | Headless suppression/preservation tests PASS. Isolated real native preferences/lifecycle probes committed Off and closed normally. Full Gator global-Off acceptance is PARTIAL due the stalled preference sequence. |

The accepted recording has approximately 23 seconds of window-only, audio-free output. At 30 Hz analysis, completion first appears at 7.6 seconds; the accent's outer ring changes in 16 sampled frames between 7.8 and 8.3 seconds. The final two seconds have zero changed outer-ring pixels against their settled reference. This is evidence of a bounded actual application reaction, not an exported GIF preview, frame-time benchmark or GPU/energy measurement. The earlier recordings/probes remain as historical attempts; use `keyboard-live-reaction.mp4` and `keyboard-native-results.json` for the successful coupled sequence.

### Unresolved native layout stall

One final-candidate fixture became unresponsive after rapid settings changes/closure following the inert approval-review scenario. The final bridge audit shows that an unattributed input resolved that approval; its exact relationship to the subsequent stall is unconfirmed. Sampling showed the main thread occupied by SwiftUI view-graph/layout work, including `SelectionOverlay`, rather than a Go shutdown wait or a running GIF loop. `ps` observed approximately 63–99% of a core; `sample` reported **1.4G physical footprint**. Those are measurements of the stalled full app, not normal emoji resource cost. [Sample](evidence/p04e-1/unresponsive-sample.txt), [unconfirmed guarded Quit](evidence/p04e-1/final-guarded-quit.log).

The tentative theory that repeated unchanged emoji text/font assignments invalidate intrinsic sizing was **not supported**: a standalone native probe observed zero such invalidations for 100 unchanged assignments. The proposed canvas change was discarded, restoring the already tested production source exactly. A preferences-only probe and a second probe reusing the real canvas/player/desktop controller both committed Off and closed/quit normally. This narrows the investigation but does **not** establish the full-app cause or prove the issue resolved.

The stalled disposable instance was force-stopped only after preserving the sample; its recorded fixture Go children were terminated. No successful guarded Quit is claimed for it. No user-owned Gator instance, credentials or data was involved. The retained fixture manifests show zero web requests, zero searches and no external-action entries, including after the unattributed approval ([effects audit](evidence/p04e-1/fixture-effects-audit.json)). The local fixture returned text rather than requesting a fetch after that grant. Treat this as an **open P1 native-availability investigation item**, with new-versus-preexisting/automation-induced origin unconfirmed. Do not treat desktop interference alone as the explanation for a sampled busy main thread. Complete native acceptance requires reproducing that sequence under controlled input and either correcting the cause or conclusively separating an accessibility/platform issue from application behavior.

The script’s historical `decisionSubmitted: false` field described its intent rather than auditing subsequent input; it is superseded by the final bridge audit. No approval action exists in the presence subsystem or in this harness, but I cannot identify the source of that input and cannot claim a clean no-decision session. No production approval policy was changed.

Other diagnostic failures were distinguishable: an AX text-value assignment did not reliably deliver a normal text-edit change, a window opening animation briefly changed capture dimensions, and buffered recorder output delayed readiness detection. The final successful fixture used actual focused keyboard editing, dynamic own-window bounds and an explicitly flushed recording-start signal. No production input, focus or execution policy was weakened to accommodate those probes.

### Remaining operator checklist

1. With other desktop automation stopped, open the final app using a disposable profile; confirm emoji-only, reaction and Off modes in full/compact chat.
2. With an inert Go approval pending, switch Animate, in-chat and desktop visibility; close/reopen Settings; verify bounded CPU/memory and responsive controls. Reproduce the sampled stall before assigning its cause.
3. Tuck Away, perform guarded Quit, relaunch and confirm no replay or re-enabled desktop presence. Recheck focused Find, older-message reading, collapse and Jump to Latest without concurrent input.
4. Physical VoiceOver, IME, OS Reduce Motion/contrast variants and external-display/Spaces behavior remain NOT TESTED unless exercised by an operator.

No further product feature was started. The requested app is a **development candidate with partial native acceptance**, not an assertion that the unresolved stall is acceptable.

## Exact changed-file inventory

- `macos/Sources/GatorMac/NativeMascotModel.swift`: finite emoji/mode mapping, public failure/text-active facts and smaller stable layout values.
- `macos/Sources/GatorMac/NativeMascotPlayback.swift`: native text canvas, bounded GIF input and finite existing-player playback/cancellation; old PNG loading removed.
- `macos/Sources/GatorMac/NativeMascotView.swift`: strict public outcome/text-active projection, mode preference and small in-chat presentation.
- `macos/Sources/GatorMac/NativeMascotPreferences.swift`: Agent Presence controls and independent desktop visibility/display/Tuck wording, preserving old keys.
- `macos/Sources/GatorMac/NativeDesktopPet.swift`: shared presence mode, global Off suppression, emoji resource root/title; established panel/ownership behavior retained.
- `macos/Sources/GatorMac/{ConversationView,GatorApp}.swift`: Agent Presence settings labels only.
- `macos/scripts/build-dev-app.sh`: package Presence GIFs instead of old mascot frames.
- `macos/Tests/GatorMacTests/{NativeMascotTests,NativeDesktopPetTests}.swift`: current presence/migration/finite-GIF/safety/lifecycle contracts.
- `macos/Resources/Presence/{attention,completed}.gif`: two original offline-generated reaction accents.
- `macos/mascot/2d/legacy-p04e/`: pre-change implementation/test reference copies; original prior assets/evidence are untouched.
- `macos/presence/source/`: editable exporter and isolated, allowlisted acceptance/diagnostic tools. These are not production package targets or a second playback engine.
- This report and `macos/presence/evidence/p04e-1/`: source/bundle manifests, tests, GIF validation, actual captures/recordings and diagnostic findings.

No README, Go, host, GatorCore, provider, credential, archive, request-admission, approval or task-ownership implementation changed in P04E-1. No commits, paid calls, external effects, distribution signing or notarization were performed. External commits were preserved. Stop after P04E-1.
