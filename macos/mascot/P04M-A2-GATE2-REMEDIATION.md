# P04M-A2 — Gate 2 resource remediation

**Gate 2: PARTIAL. Production integration remains stopped.** The optimized renderer passes focused geometry and lifecycle checks and a final-build 11m31s trial. Settled memory improves, but the 40 MiB goal is not consistently met, especially by physical footprint during/after activity. Strict pixel-equivalence also fails. Historical A2 figures are retained in the earlier reports and are not represented as measurements of this revision.

## Baseline, scope and experiment identity

Starting HEAD: `a8b96697a1f7bbf853303c51f386e3d1bd879db5`, with the existing A2 reports/evidence and final drawable correction already in the worktree. [Initial worktree, source and bundle hashes](evidence/p04m-a2-gate2/baseline.json). Original A1 geometry, `validation/`, the accepted P04 application and the original Renderer-Lab application are protected by hash comparisons. No production Swift, Go, JS, resources, icon, package manifest or native build script is changed.

The approved USDZ remains **58,290 bytes**, SHA-256 `f85db8a1c80ec5f17d8de8102e77c3b4774357548ee26a582c689be32e4103ca`. The original authoring USDA remains SHA-256 `356d1cabfba9562e34a62a43123eb850b6f389ff2d9efa63cf55e51852334ed3`.

Host: Apple M3, eight CPU / ten GPU cores, 16 GiB RAM, macOS 26.5.2 (25F84), Apple Swift 6.3.3 / SDK 26.5. Builds use `-O -swift-version 6 -strict-concurrency=complete -warnings-as-errors -target arm64-apple-macosx15.0`. This does not certify physical macOS 15 runtime compatibility.

| Experiment group | Purpose and comparability |
| --- | --- |
| Initial A2, historical | 42–48 MiB incremental RSS, about 51 MiB retained after multi-view use; final then-current presenter ran only 125 seconds. Not a final-build endurance result. |
| Profile v2 | Staged framework/device/renderer/model/draw attribution. The run ended after hiding the host and is excluded from endurance acceptance. Its completed earlier phases remain diagnostic evidence. |
| Profile v3 | One identical executable compared original offscreen-224 presenter, direct 96/256, explicit MetalKit 96 and MSAA. No native mesh-cache optimization yet. |
| Profile v4 | One identical executable compared USDZ loading, native mesh-cache loading, 96/256, hybrid policy, two live scenes and staged native attribution. Preliminary comparisons, not final acceptance. |
| Final candidate v10 | Matched final-build fresh-process measurements and endurance. Full source copies, flags, original USDZ/cache hashes and executable hash are in the bundle's `Contents/Resources/revision.json` and `SourceRevision/`. Each final run copies that revision record. No renderer edit is carried into an already-running acceptance trial. |

Native measurement runs are sequential. Disposable launch environments contain only the fixture home, PATH, TMPDIR and locale; no provider credentials or Go backend are needed. The final baseline warms both full and compact **native fixture windows** before initializing a RealityKit scene. These fixtures are not production Gator windows or a MenuBarExtra integration.

## Measurement method and limits

- CPU: delta `getrusage` user+system time divided by elapsed wall time; 100% means one fully occupied core. The same one-second measurement sampler exists in baseline/live/hidden phases.
- RSS and physical footprint: `TASK_VM_INFO` resident bytes and `phys_footprint`; the latter includes charges that RSS alone misses, including compressed/graphics memory. Peak RSS is `ru_maxrss`. External `footprint` provides physical-footprint peak where available.
- Heap: `malloc_zone_statistics` for the **default zone**, plus native `vmmap -summary`/`footprint` snapshots. Default-zone totals are not claimed to cover every allocation zone. VMmap's aggregate mapped-image resident column is not substituted for per-task RSS.
- Metal: `MTLDevice.currentAllocatedSize`, actual drawable texture dimensions/`allocatedSize`, allocation trace records and application frame counts. Metal allocated backing, RSS and footprint overlap and **must not be added together**. Allocated backing is not GPU utilization or energy.
- Ownership: weak probes of engine, RealityRenderer, cloned scene root, view, layer and temporary textures. An intentionally retained closed engine wrapper in one headless test distinguishes object lifetime from resource release.
- Phase records retain summaries while raw samples are written to files, avoiding an unbounded in-memory sample history. Standalone phase files retain raw observations. Sampling/metadata overhead remains part of the isolated host.
- Timing: model/first-display and resume measurements include the stated engine work. Offscreen GPU completion timing is not complete UI frame time, typing latency, GPU power or battery drain.

RSS varied as pages became nonresident. Therefore an apparent RSS decrease is not called a deallocation without ownership/footprint evidence. Active physical-footprint peaks are reported separately from settled idle values.

## Attribution: why drawable resizing alone was insufficient

The first staged USD run, in MiB, measured:

| Stage | Median RSS | Median footprint | Default heap in use, approximately |
| --- | ---: | ---: | ---: |
| Native host, no explicit RealityKit object/scene initialization | 77.22 | 19.36 | 6.64 |
| Empty Entity, no model | 79.44 | 21.02 | 7.69 |
| Metal device only | 79.48 | 21.02 | 7.70 |
| Empty RealityRenderer | 103.47 | 39.58 | 20.29 |
| Empty renderer released | 102.70 | 38.63 | 19.01 |
| USDZ template loaded | 117.47 | 52.47 | 27.82 |
| Scene attached, not drawn | 119.02 | 54.27 | 31.15 |
| First static 96-pixel render | 121.91 | 65.92 | 32.47 |

The executable already links RealityKit before A0. Consequently **unmapped versus mapped framework-only residency is not separately measured** by this binary. Empty-Entity, device and empty-renderer initialization are distinct, measurable boundaries; they must not be mislabeled as framework download/load cost.

The observations support several separate costs:

1. An empty RealityRenderer incurs substantial initialization/retained heap cost before any model is drawn. Releasing the instance recovers only part immediately. This is **fixed-in-this-path residency**, not proof that every private allocation is unavoidable or a leak.
2. The USD loading path adds further heap/residency. The experiment below tests a different input representation inside RealityKit, rather than assuming all of this is vertex data.
3. Presentation textures are much smaller. The Metal trace observed a **48 KiB 96×96 BGRA8-sRGB drawable**. Removing the persistent 224-pixel offscreen image/blit path and reducing drawables cannot by itself explain or remove roughly 40–50 MiB.
4. Internal Metal backing is larger than resident CPU memory: even the empty-renderer path reported roughly 288 MiB through `currentAllocatedSize`. This is an allocation counter, not 288 MiB measured RSS. Internal residency and active footprint need separate treatment.

In v3, original offscreen presentation added approximately **43.86 MiB** RSS above its matched baseline; direct 96 added approximately **43.84 MiB**. Direct 256 was similar. This rejects the hypothesis that a small color target alone caused the bulk of the reported overhead. Explicit MetalKit uses `isPaused=true`, `enableSetNeedsDisplay=false`, sample count 1, no depth/stencil attachment and delegate-scoped drawable acquisition. It is tested as an alternative, not assumed cheaper.

| Same-v3-executable configuration | Idle incremental RSS | Idle incremental footprint | Observed application drawable bytes |
| --- | ---: | ---: | ---: |
| Original offscreen 224 | 43.86 MiB | 46.38 MiB | Persistent offscreen path, not counted by the new direct-draw field |
| Direct layer 96, no MSAA | 43.84 MiB | 45.95 MiB | 37,376 |
| Direct layer 256, no MSAA | 43.47 MiB | 45.80 MiB | 264,192 |
| Explicit MetalKit 96, no MSAA | 43.27 MiB | 45.27 MiB | 37,376 |
| Direct layer 256, 4× MSAA | 44.47 MiB | 187.53 MiB | 264,192 color target; internal MSAA resources are additional |

No-MSAA/direct drawing avoids the measured MSAA cost, but none of these drawable-only alternatives brings the USD loading path within the requested budget. Per-resource internal depth/MSAA attachment allocation call stacks are not completely attributed.

The earlier active-loop experiments also showed substantial physical-footprint increases not apparent in RSS. Low CPU and low RSS alone are insufficient to approve GPU memory/energy behavior. The final policy therefore releases the engine after bounded gestures, rather than holding it through repeated idle periods between text updates.

## Targeted optimizations

### Original geometry, native immutable mesh cache

[Baker](gate2/bake-mesh.py) reads the **trusted A1 USDA exporter format**, preserving all 26 meshes, 8,609 vertices, 14,164 triangles, normals, indices, local transforms, rig names and material values. It emits a versioned, reproducible 626,128-byte asset cache. It is not a general USD parser, a database, a new rendering engine or a model redesign.

[NativeMesh.swift](gate2/NativeMesh.swift) reads the bounded archive and builds actual RealityKit MeshResources/ModelEntities asynchronously. Identical material values share material resources. The original USDZ remains bundled for reference comparisons; no original asset is overwritten. The rendered cache derives from the original model rather than approximating it with spheres or flat artwork.

The v4 same-executable comparison measured 96-pixel idle increments of approximately **36.44 MiB RSS** for this path versus **46.67 MiB** for the USD loading path. Those preliminary values are not substituted for the final candidate's results.

[Headless native equivalence](evidence/p04m-a2-gate2/native-equivalence.json) passed **342 node/pose comparisons** with matching bounds and the existing 21 rig assertions/1,000 cycles. The comparison establishes the explicit neutral pose used at runtime before checking visibility; the USD importer does not map authored visibility directly onto `Entity.isEnabled`. Full PNG-level equality is a separate check and is not implied by graph equivalence.

### Direct drawing and bounded surface lifetime

[Engine.swift](gate2/Engine.swift) renders directly into the current drawable. It removes the permanent offscreen target, extra blit encoder and presentation queue. The direct layer uses two drawables, BGRA8-sRGB, transparent background and framebuffer-only render attachments. Color size is explicit; the selected full/compact policy uses 192/96 pixels for approximately 104/56-point slots.

RealityKit owns any internal depth/MSAA allocations. The application creates no depth/stencil or MSAA attachment in its direct host. The explicit MetalKit experiment calls `releaseDrawables()`, disconnects the delegate/device and releases the view. Direct-layer teardown detaches the layer/device only after engine work drains. `framebufferOnly=true` is appropriate for direct render-target use; the old blit presenter remains unchanged with `false`.

### Event-driven presentation and one active renderer

[Presentation.swift](gate2/Presentation.swift) is cosmetic resource ownership only. It imports no GatorCore and has no execution, approval, provider or request-admission path.

- One immutable unattached template is decoded once. Each live scene receives a clone; a mutable entity is never attached to two renderers.
- Idle/reduced-motion/hidden presentations use a cached transparent image. No autonomous high-frequency draw loop or SceneEvents subscription exists.
- Work, attention, disconnect, blink and response gestures use genuine 3D entities. A bounded transition then releases the renderer and surface, leaving its static pose where appropriate. This is a **hybrid live-3D-burst/static-between-events** design, not a continuously animated scene or GIF.
- Response requests are throttled with an injectable monotonic clock. Same-slot/configuration updates inside 250 ms do not recreate an engine. Handoff to another slot remains explicit.
- A cached image covers the new native surface until its first frame completes. Stale/cancelled operations cannot reveal a superseded surface.
- Visibility is explicit. Hidden updates do not recreate a renderer; animation cancellation drains GPU work before teardown. OS Reduce Motion takes precedence over animation requests.
- Full/compact fixture windows have separate view lifetimes. Only one receives the animated surface; the other displays a cached pose. The endurance sequence includes actual controlled full-window closes/reopens, hiding, compact handoff, renderer destruction and static/reduced-motion periods.

The direct renderer also applies the exact terminal pose on the final interpolation step. This corrects a floating-point endpoint discrepancy found by the new concurrent-transition test; the assertion was retained.

## Verification and acceptance gaps

The focused headless lifecycle suite passes **33 assertions**, with an additional **21 rig assertions and 1,000 cycles**. It covers actual offscreen rendering, jaw-open cancellation, concurrent supersession, exact neutral/terminal poses, pause/resume, target release, hidden updates, controlled-clock throttling, full/compact handoff, static modes and repeated view lifetime. No production assertion is changed.

The first visual pixel probe used a one-level per-channel tolerance. Its captured diagnostic output shows that **strict PNG equality did not pass**; graph equivalence must not be presented as bit-identical rendering. Visual/native acceptance and the exact diagnostic differences are recorded with the final evidence below. No test failure is concealed by simply raising that tolerance.

Routine checks are headless. Native windows/profiling are used for actual drawable, alpha, handoff, input and lifecycle observations. Production Swift/Go/host suites are not repeatedly run because production code is unchanged, as requested.

## Exact final-build comparison

Final executable SHA-256: `9887643419b6d54196512d9f140cc54f703d20d90a7e177ad5996c9448159015`. The delivered `/tmp/Gator-Mascot-P04M-A2-Gate2.app` is byte-identical to the qualified `/tmp/Gator-Gate2-FinalCandidate-v10.app`, including its sources and resources. The baked mesh-cache SHA-256 is `ca627304250fa060974d1d18e293c9baa85d9755d7086d261cd498eaf0ecc13f`.

The same final executable performed fresh-process USD/direct-96 and native-mesh/direct-96 measurements. These two cases retain an engine between gestures to isolate the loading path. They are distinct from the final hybrid endurance policy, which destroys the engine between gestures.

| Same-build 96-pixel case | Host RSS / footprint | Visible idle RSS / footprint | Idle increments RSS / footprint | Active footprint |
| --- | ---: | ---: | ---: | ---: |
| Original USD loading path | 78.75 / 21.42 MiB | 121.88 / 66.70 MiB | 43.12 / 45.28 MiB | 162.39 MiB |
| Native mesh loading path | 75.31 / 20.45 MiB | 111.08 / 58.86 MiB | 35.77 / 38.41 MiB | 155.22 MiB |

The matched result supports a useful reduction in loading-path residency, rather than a claim that all RealityKit costs disappear. Active physical footprint remains much larger than idle RSS suggests. Raw phases and exact build identities are under [final-v10](evidence/p04m-a2-gate2/final-v10/).

[Reproducible asset bake](evidence/p04m-a2-gate2/bake-reproducibility.json) regenerated the mesh cache from a temporary copy of the trusted source and obtained byte-identical output. Asset/model load plus first drawable latency is recorded separately from warm resume: the final native96 fresh scene's `create` event includes construction, while the template is prepared before that event. Do not call that event a full application-startup measurement. Production startup and production transcript performance are outside this isolated pass.

That fresh native96 construction/first-display event measured **61.77 ms**. Template/model-load costs are included in the staged attribution boundary, not in this event. No background token update reloads the original USDZ or rebakes meshes.

### A0–A10 coverage

| Requested boundary | Evidence and limitation |
| --- | --- |
| A0 native host | Each fresh process samples the native text/window fixture before explicit RealityKit scene initialization. |
| A1 framework/device | Staged empty Entity, Metal device and empty RealityRenderer measurements; framework mapping alone cannot be separated by the linked host. |
| A2 static model/scene | Separate template-only and attached/not-drawn stages in profile-v2 and profile-v4/attribution-native. |
| A3 live 96 | Same-final-build USD/native comparisons above, including idle, active, return-to-idle, hidden and teardown. |
| A4 live 256 | Profile-v3 direct256 and profile-v4 native256. In v4, native256 adds 36.25 MiB RSS / 39.11 MiB footprint at idle. These are preliminary, separately revisioned runs. |
| A5 returned idle | Same-final native96: 111.09 MiB RSS / 58.88 MiB footprint; final hybrid settled endpoint below. |
| A6 hidden | Zero frames in all ten final all-hidden sampling periods; explicit surface/layer release. |
| A7 destroyed | Engine, RealityRenderer, scene, surface and layer weak counts zero; bounded passive texture/framework residency remains. |
| A8 repeated lifetime | 82 engine/scene/surface lifetimes in the final trial, plus headless repeated-lifetime tests. |
| A9 full/compact | Ten handoffs, five actual controlled full-window close/reopen events, 192/96-pixel configurations. |
| A10 simultaneous views | Profile-v4/native-two created two real engines/scenes/surfaces: idle +38.98 MiB RSS / +41.39 MiB footprint; both engines/scenes/surfaces zero after teardown. It is diagnostic, not the chosen policy. Only the first surface was paused at its intermediate hidden checkpoint; do not claim both were hidden there. Final policy uses one animated slot, static secondary presentation. |

Profile-v3 also exercises no-MSAA versus 4× MSAA, direct-layer versus explicit MetalKit, and 96 versus 256 pixels. Raw samples are preserved rather than treating lower resolution as an assumed optimization. Sample count 1/no application depth attachment is selected for small-size cost; small silhouettes have visible pixel stepping without MSAA. No asset tessellation or rig part was removed.

## Final-configuration endurance: 690.835 seconds

[Complete final report](evidence/p04m-a2-gate2/final-v10/endurance/results.json), [raw samples](evidence/p04m-a2-gate2/final-v10/endurance/samples.ndjson), and [reproducible summary](evidence/p04m-a2-gate2/results-summary.json).

- One asset decode; **589 explicit renders**, **82 engine lifetimes**, ten full/compact cycles, five controlled full-window close/reopen events. No crash or unhandled renderer error was recorded. Completion includes a duration assertion of at least 600 seconds.
- Final teardown weak counts: engine **0**, RealityRenderer **0**, scene root **0**, surface **0**, CAMetalLayer **0**, immutable template **0**. One temporary poster texture remains weakly observable. Its retaining system/private owner is not fully attributed; it is not concealed as a fully freed GPU cache.
- All ten all-hidden phases: **0 frames**, no pending animation task/work, no live renderer/surface/layer at checkpoints. State updates while hidden did not recreate an engine. Static and settled idle phases likewise issued zero frames during their sampling windows.
- No renderer update subscriptions are installed. Bounded gestures use cancellable task steps; GPU completion is awaited before releasing targets. There is no autonomous high-frequency idle loop or autonomous blink timer in this lab. The harness requests occasional blinks.
- Resume/first-frame work, last 64 bounded samples: median **29.81 ms**, percentile-95 **47.18 ms**, maximum **54.90 ms**. This is engine work, not complete input-to-photon latency or a promise for every future blink.

| Final trial checkpoint | RSS | Physical footprint | Default-zone heap in use | CPU, one core = 100% |
| --- | ---: | ---: | ---: | ---: |
| Host baseline | 78.39 MiB | 21.27 MiB | 7.18 MiB | 0.108% |
| After initialization/static pose | 115.34 MiB | 68.09 MiB | 22.34 MiB | 0.135% |
| Final settled idle | 100.94 MiB | 59.44 MiB | 26.00 MiB | 0.114% |
| After all presentation/template teardown | 100.94 MiB | 59.44 MiB | 25.85 MiB | 0.096% |

Final settled increments are **22.55 MiB RSS / 38.17 MiB footprint**. Peak task RSS is **120.20 MiB total**. The late external `footprint` sample reports **167 MB peak physical footprint**, retaining that tool's displayed units. The lower late RSS partly reflects nonresident/compressed pages; it is not proof of equivalent released memory.

| Ten-cycle phase group | Weighted CPU | Range of phase-median incremental RSS | Range of phase-median incremental footprint |
| --- | ---: | ---: | ---: |
| Static idle | 0.160% | 19.98–40.67 MiB | 37.25–131.23 MiB |
| Working pose, sampled after its finite transition | 0.219% | 20.59–41.28 MiB | 37.77–144.25 MiB |
| Responding, approximately 1 Hz gesture requests | 7.390% | 20.05–40.72 MiB | 43.91–144.21 MiB |
| Compact presentation | 0.340% | 20.12–41.05 MiB | 37.44–144.08 MiB |
| Settled idle between activity | 0.180% | 20.38–41.17 MiB | 37.20–74.81 MiB |
| Both fixture windows hidden | 0.250% | 19.23–40.72 MiB | 37.22–43.86 MiB |
| Reduced-motion/static periods, including window return | 0.892% | 19.53–40.73 MiB | 37.23–43.89 MiB |

These are phase medians, not maximum individual samples. Significant graphics-related footprint can linger even with zero live renderer owners. The private residency is not fully attributed to individual RealityKit/driver allocation sites. The observations reject a continuously growing renderer-owner leak in this trial; they do **not** prove all retained memory is necessary, harmless or bounded over arbitrarily long sessions. The test host itself retains bounded experiment metadata, and all-zone allocation call-stack attribution remains incomplete.

Destroying an engine after each burst trades lower quiescent ownership for roughly 7.4% one-core CPU during this 1 Hz response fixture. The 250 ms throttle is tested for correctness; sustained 4 Hz resource behavior is **NOT TESTED**. This is another reason not to approve production integration from the idle endpoint alone.

## GPU evidence and drawable release

The final executable received a separate **75-second Metal System Trace** profiling run. It is not mixed into the uninstrumented endurance memory budget. Selected sanitized exports contain:

- **330 Gate2Lab command-buffer submissions**, each matched by ID to a completion; median submission interval duration about **0.972 ms**, maximum **6.591 ms**. These trace durations are not asserted as pure GPU execution time or end-to-end frame time.
- **3,377 Gate2Lab resource events**: 1,715 allocation and 1,662 deallocation events within the bounded trace window. A count difference does not prove a leak: trace boundaries include resources whose lifetime extends outside the capture.
- **Zero captured Metal command-buffer error records**. The kernel-resource allocation table exported no rows, so detailed per-resource kernel residency is unavailable from this trace.
- Earlier diagnostic trace: a 96×96 drawable occupied 48 KiB; the app's `allocatedSize` varies with the driver allocation (one native96 observation is 37,376 bytes). Do not equate `width × height × bytes` with allocated/resident footprint.
- Final teardown `MTLDevice.currentAllocatedSize` remains approximately **290.11 MiB**, despite zero renderer owners. This is allocated backing, not resident memory or evidence that the whole amount is actively used.

[Submission export](evidence/p04m-a2-gate2/gpu-submissions.json), [completion export](evidence/p04m-a2-gate2/gpu-completed.json), [resource export](evidence/p04m-a2-gate2/gpu-resources.json), [error export](evidence/p04m-a2-gate2/gpu-metal-errors.json), [sanitized trace schema](evidence/p04m-a2-gate2/final-metal-toc-sanitized.xml). Raw trace bundles stay private under `/tmp` because trace metadata can capture the launching environment; environment/device identifiers are omitted from repository exports.

GPU work is genuinely observed. **GPU utilization percentage, GPU power, energy and battery impact: NOT TESTED.** Hidden application frame counters are zero, but this is not a claim of zero system GPU activity or zero watts.

Apple documents [RealityRenderer as macOS 15+](https://developer.apple.com/documentation/realitykit/realityrenderer) and [releaseDrawables](https://developer.apple.com/documentation/metalkit/mtkview/releasedrawables()) as releasing MetalKit depth/multisample textures for inactive views. The latter does not promise that all driver allocations disappear. [Metal memory analysis](https://developer.apple.com/documentation/xcode/analyzing-memory-usage) distinguishes allocation configuration and resource residency. Strict target-15 compilation passes; **physical macOS 15 runtime acceptance remains NOT TESTED**.

## Visual and native evidence

The inspected full/compact fixture captures retain the blunt snout, round green head, nostrils, distinct eyes and separate jaw. They use reserved space without covering the editable fixture text. These are renderer fixtures, not a redesigned or production-integrated Gator interface.

- [Full fixture](evidence/p04m-a2-gate2/full-fixture.png), [compact fixture](evidence/p04m-a2-gate2/compact-fixture.png).
- [28.335-second window-only recording](evidence/p04m-a2-gate2/live-3d-bursts.mp4), no microphone, audio or desktop capture. It has 40 captured frames. The capture shows pose/layout changes but does not conclusively resolve every brief blink/jaw gesture; do not substitute it for deterministic articulation tests.
- A second same-final-executable [22.827-second direct-render recording](evidence/p04m-a2-gate2/live-direct-articulation.mp4), 222 captured frames, visibly shows the lower jaw opening and eyelids closing independently of the head. [Actual-frame contact sheet](evidence/p04m-a2-gate2/live-direct-articulation-contact.png). This uses the retained-engine `native96` diagnostic to make articulation observable; it is not substituted for hybrid-policy resource numbers. The hybrid's brief gesture capture remains less conclusive.
- Native input probe activated only the exact fixture PID/window and verified the typed marker in its AXTextArea. No message was sent. Earlier attempts while hidden were refused by the guard. A screenshot application-name mismatch was corrected using exact window IDs; it was a capture-selection error, not an established application focus defect.
- Formal scrolling/typing latency, production transcript anchors, production menu-bar behavior, dark appearance, physical VoiceOver, IME and preference variants were not accepted in this isolated resource pass. No system-wide preference or accessibility setting was changed.

Alpha checks find transparent background plus visible geometry; no opaque rectangle. No-MSAA test images have binary silhouette alpha rather than partially covered edge pixels. Geometry/pose equivalence passes, but strict PNG equivalence **FAILS**:

| Same-final-build PNG comparison | Changed pixels | Maximum channel delta /255 | Mean channel delta /255 |
| --- | ---: | ---: | ---: |
| Neutral native versus USD | 930 / 9,216 | 69.558 | 0.04682 |
| Open jaw / blink native versus USD | 792 / 9,216 | 5.174 | 0.02811 |

The probe's unchanged acceptance threshold is ≤1.01 maximum channel levels. Both diagnostic failures and stderr are retained. Material inspection finds equivalent PBR scalar settings and tiny color round-trip differences; it does not conclusively explain the maximum isolated pixel delta. [Inference] Mesh/importer floating-point or rasterization differences may contribute, but a precise visual-difference root cause remains unverified. No original A1 design is overwritten or silently simplified to satisfy memory goals.

## Test commands and outcomes

Run from the repository root. Build logs and JSON results are in the evidence directory.

```sh
# final isolated build (completed before the final endurance trial)
GATOR_GATE2_APP=/tmp/Gator-Gate2-FinalCandidate-v10.app \
  bash macos/mascot/gate2/build.sh

# final matched comparisons and final-config endurance, sequentially
python3 macos/mascot/gate2/run-experiments.py \
  --app /tmp/Gator-Gate2-FinalCandidate-v10.app \
  --root /tmp/gator-gate2-v10-final native96 direct96 endurance

# strict headless lifecycle compilation and Metal API validation
swiftc -parse-as-library -swift-version 6 -strict-concurrency=complete \
  -warnings-as-errors -O -target arm64-apple-macosx15.0 \
  macos/mascot/validation/MascotRig.swift \
  macos/mascot/validation/RealtimeRenderer.swift \
  macos/mascot/gate2/{Metrics,Engine,NativeMesh,Presentation,CheckLifecycle}.swift \
  -o /tmp/gator-gate2-check-lifecycle-final-verified
MTL_DEBUG_LAYER=1 /tmp/gator-gate2-check-lifecycle-final-verified \
  macos/mascot/gate2/Assets/B-native-meshes.json

# exact authored graph/pose/bounds comparison
swiftc -parse-as-library -swift-version 6 -strict-concurrency=complete \
  -warnings-as-errors -O -target arm64-apple-macosx15.0 \
  macos/mascot/validation/MascotRig.swift \
  macos/mascot/gate2/{NativeMesh,CheckNative}.swift \
  -o /tmp/gator-gate2-check-native-final-verified
/tmp/gator-gate2-check-native-final-verified \
  macos/mascot/validation/Assets/Gator-B.usdz \
  macos/mascot/gate2/Assets/B-native-meshes.json

python3 macos/mascot/gate2/summarize-results.py
```

The failing image probe was compiled with the same strict flags from `gate2/CheckImages.swift` and run against `final-v10/direct96/{neutral,articulated}-direct96.png` versus `final-v10/native96/{neutral,articulated}-native96.png`. Its JSON and stderr are retained as `visual-{neutral,articulated}-final.*`. `InspectMaterials.swift` produced `materials.json`; it is diagnostic rather than a pixel-fidelity acceptance assertion. `bash -n gate2/build.sh`, Python syntax compilation of the three harness scripts, and `git diff --check` pass. No production lint/test assertion was removed or weakened.

Profiler commands used their captured fixture PID only:

```sh
vmmap -summary FIXTURE_PID
footprint --noCategories -p FIXTURE_PID
xcrun xctrace record --template 'Metal System Trace' \
  --attach FIXTURE_PID --time-limit 75s \
  --output /tmp/gator-gate2-final-metal.trace
xcrun xctrace export --input /tmp/gator-gate2-final-metal.trace --toc
```

Selected schema exports and ID matching produce the GPU results above; raw TOC environment metadata is excluded from repository evidence. No Instruments privacy/developer setting was changed.

| Verification | Result |
| --- | --- |
| Strict Swift 6 / warnings-as-errors / macOS-15 target | PASS, final lab and focused check binaries |
| Lifecycle assertions | PASS: 33, plus 21 rig assertions and 1,000 cycles; repeated with Metal API Validation enabled, no validation error reported |
| Native graph/pose/bounds | PASS: 342 comparisons, 26 meshes / 8,609 vertices / 14,164 triangles; 21 rig assertions / 1,000 cycles |
| Final native lifecycle | Completed 690.835s; duration assertion passes, ownership/frame evidence above |
| Strict PNG identity probe | FAIL, two comparisons; diagnostics retained, assertions not relaxed |
| Metal trace | Collected; 330 matched submissions/completions, zero captured error records; not an energy measurement |
| Production Swift/Go/host suites | NOT RERUN: production files unchanged, following this pass's test policy. Earlier P04 counts are historical only. |

The headless lifecycle test deliberately retains one closed engine wrapper locally; its RealityRenderer/scene are zero. One headless target remains weakly observable in the framework cache. This is distinguished from a running engine or pending animation task. Headless tests create no windows and no Go process.

## Preservation and changed-file inventory

[Preservation record](evidence/p04m-a2-gate2/preservation.json): **1,927 protected existing files unchanged**; original Renderer-Lab and accepted P04 bundle file hashes unchanged; delivered bundle equals qualified v10; runtime source hashes match the qualified source snapshot. No commits were created by this work. HEAD advanced externally from `a8b96697a1f7bbf853303c51f386e3d1bd879db5` to `8697c0f4db04062c1171bcf50a4444fa6253165f` while work was in progress; it is recorded without resetting or rewriting that history. Acceptance is tied to executable/source hashes, not an assumption that HEAD stayed fixed.

All implementation additions are isolated:

- `gate2/Engine.swift`, `NativeMesh.swift`, `Presentation.swift`, `Metrics.swift`, `Gate2Lab.swift`: renderer, exact-mesh loading, finite presentation policy, measurement and fixture host.
- `gate2/bake-mesh.py`, `Assets/B-native-meshes.json`, `build.sh`, `run-experiments.py`, `summarize-results.py`: reproducible asset/build/measurement pipeline.
- `gate2/CheckLifecycle.swift`, `CheckNative.swift`, `CheckImages.swift`, `InspectMaterials.swift`: focused correctness/fidelity diagnostics.
- `gate2/DesktopControl.swift`, `RecordWindow.swift`: guarded fixture-only input and window-only recording helpers, not production dependencies.
- This report, addenda to the two A2 reports, and `evidence/p04m-a2-gate2/`: measurements, preserved exact revisions, screenshots and recording. Original A1 assets and `validation/` are unchanged.

## Gate decision and remaining tradeoff

**PARTIAL. Do not integrate into production chat yet.** Genuine articulated 3D, strict compilation, hidden-work cancellation, bounded owner lifetimes, final-config endurance and a lower idle footprint are demonstrated. The existing memory goal is not consistently established: some quiescent/hidden phases exceed 40 MiB incremental memory, and active/just-finished phases show roughly 144 MiB additional physical footprint. Strict pixel equality and formal input/scroll latency remain unresolved. Low idle CPU is encouraging but cannot approve energy or graphics residency.

Recommendation: retain this isolated hybrid prototype and perform further **targeted attribution of private RealityKit/Metal residency and importer/raster differences** before integration. The evidence does not justify calling all remaining overhead unavoidable framework residency. No arbitrary budget increase is made. If accepting a different budget is preferable, an explicit reviewer decision would need to cover at least the observed approximately 44 MiB quiet footprint and approximately 145 MiB active increment, plus measurement margin and active-CPU tradeoff; these are observed values, not guaranteed caps. Another native renderer requires an evidence-based comparison before adoption. No SceneKit/raw-Metal/2D substitution is performed.

Optimized isolated artifact: **`/tmp/Gator-Mascot-P04M-A2-Gate2.app`**. This report closes the requested optimization/measurement pass as PARTIAL; it does not authorize or start production integration, a desktop pet or another parity increment.

For review, launch the selected hybrid policy explicitly with a fresh output directory:

```sh
open -n /tmp/Gator-Mascot-P04M-A2-Gate2.app --args \
  --experiment hybrid --output /tmp/gator-gate2-hybrid-review
```

The lab is a finite diagnostic harness and quits after the chosen run. A plain launch defaults to the retained original-USD `direct96` comparison; it is not the optimized hybrid resource-policy run. This distinction is preserved in the frozen qualified executable rather than silently changing it after the final endurance trial.
