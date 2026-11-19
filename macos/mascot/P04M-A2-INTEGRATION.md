# P04M-A2 — integration disposition

**PARTIAL — production integration not authorized by the validation result.** Concept B is preserved and a genuine native 3D pilot exists, but Gate 2 has not been accepted. The user's explicit stop-before-production condition is applied. [Renderer validation, measurements and recording](P04M-A2-RENDERER-VALIDATION.md).

## What changed

All new work is under `macos/mascot/`:

- `validation/MascotRig.swift`: independent named RealityKit part binding, finite poses and deterministic articulation/neutral checks.
- `validation/RealtimeRenderer.swift`: async USDZ load, stable RealityRenderer scene, transparent native Metal presentation, explicit drawing and teardown.
- `validation/RendererLab.swift`: separate native fixture application, scripted comparisons, static fallback, lifecycle/cancellation checks and measurements.
- `validation/ReproduceAsset.swift`: reproducibility probe using original A1 authoring code and original editable SCN archive.
- `validation/CheckRig.swift`: headless RealityKit geometry, independence, missing-asset and articulation checks.
- `validation/RecordWindow.swift`: window-only ScreenCaptureKit recorder with readiness/completion handling.
- `validation/build.sh`: strict Swift 6/macOS-15 isolated bundle build. It does not build or alter Gator.
- `validation/Assets/`: exact B USDA/USDC/USDZ and static poster copies. A1 originals remain unchanged.
- `evidence/p04m-a2/`: asset checks, build/test logs, measurements, native screenshots and genuine live-window recording.
- The two P04M-A2 reports. No README or existing report was rewritten.

`evidence/p04m-a2/baseline.json` captures hashes of 1,916 existing production/test/resource/script/A1 files and all six P04 bundle files. Final preservation results are in `evidence/p04m-a2/preservation.json`. Existing external P04 changes are not reverted, committed or attributed to this increment.

HEAD advanced externally during this task from `12ee4a43b6677f4d01a4ab6882951a35d09ac678` to `a8b96697a1f7bbf853303c51f386e3d1bd879db5` at the first final integrity check. All protected file hashes still matched. This agent created no commit; final observed HEAD is recorded in the preservation JSON.

## Production work deliberately blocked

| Requested behavior | Current disposition |
| --- | --- |
| Genuine Concept B geometry, rig and finite animation | Demonstrated in the isolated RealityKit lab. |
| Chat header placement, full and compact | **NOT IMPLEMENTED**; original `ConversationView` is unchanged. |
| Reading/review shrinking | **NOT IMPLEMENTED**; anchors/scroll geometry have not been modified. |
| Canonical selected-owner state projection | **NOT IMPLEMENTED**; lab states are explicitly fixtures, not Go state. |
| Show/Animate mascot preferences | **NOT IMPLEMENTED**; existing native preferences are unchanged. |
| Production Reduced Motion observation | **NOT IMPLEMENTED**; isolated static preset works, physical OS preference variants are not tested. |
| Automatic production visibility/minimize/sleep lifecycle | **NOT IMPLEMENTED**; explicit isolated teardown is tested. |
| Chat layout, typing, Find, exact reviews and focus acceptance | **NOT TESTED WITH A MASCOT**; no integrated candidate exists. |
| Production asset packaging/build script | **UNCHANGED**; only the isolated bundle contains copied model resources. |
| Actual Gator baseline comparison and integrated ten-minute trial | **NOT TESTED**; cannot be substituted by the renderer-only lab. |
| `/tmp/Gator-Native-P04M-A2.app` | **NOT CREATED**; an unaccepted integrated mascot is not packaged. |

The existing full-window toolbar/workspace/Find/activity, compact 440×620 presentation and 540×480 full-window minimum were inspected. A later integration should reserve a slightly offset header slot outside the transcript, never move exact review controls, and derive shrink intent from existing reading/inspection state. Changing viewport height while reading must be tested with actual stable message anchors and active scroll gestures, not inferred from source.

Future state mapping must read only `sessions.current`, its owner generation and public Go snapshot. Existing `BridgeSnapshot.running` and visible transcript projection are available, but there is no typed generic streaming Boolean. A scoped public assistant-content delta must baseline on owner replacement/history loading. A confirmed relevant Go completion, not a timer or `running == false`, is required for a completion expression. Exact review/recovery attention must take precedence over a happy animation. No approval token, raw reasoning, secret or execution method belongs in this adapter. None of that mapping is claimed implemented here.

## Safety and regression boundaries

No code change to Go execution, provider credentials, managed owners, workspace identity, admission, queues, approvals, plan digests, recovery, attachments, draft transactions, closing transactions or subprocess shutdown. No extra Go process is created by the isolated renderer. It does not import `GatorCore` and has no prompt/action path.

The production build/full Swift/Go/host suites were **not rerun solely for this isolated lab**. Historical P04 evidence remains 227 Swift tests, 14 strict checks/tests, seven Go packages, three Go race packages and 41 host tests. These are **historical**, not new A2 regression results. A production integration cannot be called accepted on those historical counts; the user-required full regression commands remain outstanding if integration later proceeds.

New verification consists of strict standalone compilation, reproducible asset checks, native renderer output, deterministic rig/lifecycle checks and process-resource measurements. Failed/interrupted early experiments and the final successful recording are distinguished in the renderer report. No existing assertion is weakened.

## Accessibility and native acceptance limits

The Metal surface is decorative and not an accessibility element or focusable action. Fixture status is ordinary text. The static poster remains a separately identified 2D fallback. No production status label was removed or replaced by facial expression.

Physical VoiceOver, IME, Increase Contrast, dynamic system Reduce Motion, sleep/wake, minimum-size integrated chat, compact integrated chat, typing/search/review focus, per-owner collapse continuity and macOS 15 runtime acceptance are **NOT TESTED**. The recording is an isolated native window and must not be presented as full/compact Gator acceptance.

## Next step and reuse

Keep the accepted Concept B identity and the P04 application. Resolve/accept the renderer resource gate before promoting code. The current pilot can inform a shared presentation-only rig and static asset pipeline, but is **not yet a reusable production mascot subsystem** and does not authorize P04M-B. A different native renderer would require its own measurements; SceneKit's current deprecation and a custom Metal engine's maintenance cost must be weighed rather than assuming either is cheaper.

No desktop pet, P05, terminal-parity work, new dependency, paid request, signing or notarization was started. This agent created no commit. The final isolated artifact is **`/tmp/Gator-Mascot-P04M-A2-Renderer-Lab.app`**. It is not the requested production-integrated `/tmp/Gator-Native-P04M-A2.app`; that bundle was not created. The accepted `/tmp/Gator-Native-Parity-P04.app` remains byte-for-byte preserved.

Final resource decision: **Gate 2 NOT ACCEPTED**. The final idle mascot added approximately 42 MiB versus median static RSS, or 48 MiB versus settled static RSS. After exercising multiple presentations, about 51 MiB remained above the median baseline despite renderer-owner release. CPU was low, and an earlier ten-minute trial settled rather than growing each cycle, but this does not convincingly satisfy the requested memory budget. Production integration stopped at that boundary. No production P0/P1 regression is attributed to these lab-only changes; the resource target and remaining acceptance gaps are not claimed resolved.
