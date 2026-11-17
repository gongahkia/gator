# Tranche 08 — final acceptance closure

Closure continuation, not Tranche09. Initial HEAD5897fe52b6e5280266b180b6f72fc44bc5eeac73. Existing modified/untracked files and the original final bundle are preserved. Original-report checksums are in `Evidence/Tranche08-Closure/original-report-sha256.json`. This report supplements the original evidence rather than replacing it.

## Reconciliation before targeted checks

| Original category | Exact reason / evidence already available | Missing requirement / classification | Headless sufficient? | Native/operator requirement | Closure action / severity |
| --- | --- | --- | --- | --- | --- |
| B Menu-bar | Open/shared drafts observed; broader dismissal/focus/Spaces incomplete | Verification gap, no reproduced defect | Ownership yes; presentation no | Targeted open/Escape/expand; physical multi-display/Spaces operator | Batch native journey; P2 evidence |
| C Full window | Toolbar/chat/plan screenshots; AX target/menu timeouts | Window reopen/resize/navigation evidence; cause unproven | Identity/owner isolation yes | Actual close/reopen, resize, review return | Native check; fix only reproduced defect; P2 investigation |
| F Streaming | Protocol/real Go snapshots green, no final native streaming capture | Visual stream + bounded handling measurement | State/performance yes | One held/streamed native reply screenshot | Measure real framing/state replacement; P2 evidence |
| G Composer | Actual paste/Tab; AppKit composition/lifetime tests | Full keyboard sequence, physical IME | Safety API yes; physical IME no | Keyboard batch; operator IME/FKA | Separate automated from physical; P1 if accidental Send reproduced |
| L Exact review | Real Go token/owner/correction safeguards green | Native exact-action screenshot and explicit keyboard decision | Authority yes; layout/focus no | Controlled pending action, long scroll, Return/Space decision | No real email; P1 if stale/Return authorization reproduced |
| P Multi-owner | Actual Go dialogue/capacity/drafts green; final AX navigation timed out | Native switch/return while held work, no focus steal | Execution ownership yes | Actual foreground switch and background finish | Batch native; P1 if cross-owner control reproduced |
| Q Appearance | Light/dark screenshots, contrast tests | Review/recovery and resized coverage; preference variants | Semantic contrast yes | Process-only light/dark captures; operator contrast/text variants | No OS setting writes; P2 evidence |
| R Accessibility | Labels/roles, Tab, marked-text APIs, Reduce Motion code | Spoken VO, physical IME, FKA/preferences | Cannot establish physical acceptance | Operator checklist required | `TRANCHE08-OPERATOR-ACCEPTANCE.md`; not automatic PASS |
| S Performance | Reproducible projection, owner startup/RSS/thumbnail samples | Streaming handling, clearly bounded startup/rendering method | Most measurements yes | Small native render/resize observation; true cold-cache cannot be guaranteed | Report medians + boundaries; P2 evidence |
| T Bundle/lifecycle | Four-file bundle validated, idle Quit + child cleanup; core guards | Actual close presentations while held, active Quit guard | Guards/cleanup yes | One held-work presentation-close/Quit journey | Distribution signing separately DEFERRED; P1 if work stops from close |

## Duplicate basename investigation

Native staging, native draft validation and `upload_submit.files` use distinct full opaque IDs. Go `internal/attachment/uploads.go` loads/validates full IDs then replaces provider input names with `filepath.Base`. `cmd/gator/attachments.go` subsequently calls shared `attachment.ValidateLoaded`, which rejects duplicate names. Retained originals also use basename identities. Thus an unchanged protocol does not safely support two equal provider/retention display names even though staged copies are distinct.

No Go authority or original filename was changed to bypass this. A targeted real Go regression verifies two independently selected `report.md` files retain separate immutable bytes/IDs, are rejected before model execution, preserve the complete draft, and can be removed independently. Supporting duplicates is explicitly deferred pending a coherent backend provider/retention identity contract; not a Swift path/overwrite defect.

## Provider diagnosis — earlier closure attempt

Redacted read-only inspection: default gemini/gemini-3.5-flash; standard HTTPS `generativelanguage.googleapis.com/v1beta`; no URL credentials/query override; stored nonempty `api_key`; no GEMINI_API_KEY environment override, other stored provider or custom provider. Existing Go adapter sends `x-goog-api-key`. Previous HTTP401 explicitly reports invalid API key. No new calls or credential changes.

This is a provider credential/configuration blocker, not a native visual failure. Model compatibility cannot be inferred before authentication succeeds. Operator repair uses existing hidden-prompt flow: `gator provider login gemini --prompt` (or the bundled Go executable with the same arguments). Configuration: `~/.config/gator/config.json`; credentials: `~/.local/state/gator/auth.json`, subject to the existing GATOR/XDG overrides. Do not edit secrets into reports or command arguments. A valid replacement credential must be supplied by the operator; no alternative working configured provider is available for another smoke test.

## Closure evidence, changes and final matrix

Completed by the continuation below. The reconciliation rows above describe original gaps, not final statuses.

## Final closure continuation — 2026-11-17

Continuation HEAD: `8cb98099ed626641f27118ca4450b5da7341cfd6`, initially clean. This host is Apple M3 / Mac15,12 / 16 GiB, macOS 26.5.2 (25F84), Swift 6.3.3, Go 1.27.3. The earlier Intel macOS 15 measurements and screenshots are historical evidence, not measurements on this host. The earlier `/tmp/Gator-Native-Tranche08-Final.app` is absent here; it was not deleted or overwritten. Rebuilt the local bundle at `/tmp/Gator-Native-Tranche08-Acceptance.app`. The pre-icon closure candidate is preserved at `/tmp/Gator-Native-Tranche08-PreIcon-Evidence.app`.

New evidence is under `Evidence/Tranche08-Closure/continuation/`; paths below are relative to that directory unless stated otherwise. The initial reconciliation table above remains the record of why each category needed investigation. The matrix below is the final assessment, superseding earlier statuses without erasing the underlying evidence.

### Native interaction and visual result

Observed actual MenuBarExtra opening/dismissal, expansion, standard full-window close/reopen, 820×740 to minimum 540×480 resizing, current-task inspection/back, recovery inspection/scroll, conversation selection, and a distinct foreground draft while the original owner worked and completed. Closing the full window left the held task running; reopening retained the same app process. Full and compact presentations shared the selected chat and unsent draft. Cmd-L returned focus to the composer. A Quit warning was observed while an exact review was pending; Keep Gator Open dismissed it. The exact final bundle subsequently quit and its sampled Go children exited (`headless/final-quit.json`). Presentation identity/owner creation and background-focus invariants also passed in the 130-test suite; the interrupted native automation script did **not** establish an exact before/after PID-count assertion and is not cited as such.

Native Tab moved from composer to Attach Files; Shift-Tab returned. Shift-Return inserted a newline. Return reached submission during the early unavailable-fixture session, which is input/admission evidence, not successful inference. Prior actual Command-V text/image, picker and drop acceptance remains supported by the original report and unchanged component/Go integration tests. The final picker was opened then cancelled without selecting private data. These observations do not replace physical IME or comprehensive Full Keyboard Access acceptance.

Exact action review: real Go produced a pending loopback Gmail proposal with `.invalid` addresses and a long body. Captured its top and bottom, including `FINAL REVIEW LINE`. Setting AX focus alone did not reliably focus the decision; that attempt is not keyboard acceptance. Actual Tab then reached Decline. Return left Approve Exact Action enabled and the proposal pending; Space explicitly declined. No `external-effect` entry occurred in the fixture trace. Screenshots and `headless/keyboard-review-traversal.json` supplement the exact token/owner/stale/correction integration tests. The prior compact exact-review screenshot is genuine layout evidence.

Recovery: captured controlled HTTP 503 failure details, retained evidence, and minimum-window scrolling; after restart captured settled compact and full dark recovery. The initial compact light capture was blank/unsettled. It did not reproduce in the settled final compact view; no production layout fix is claimed. The old `screenshots/full-recovery-narrow-light.png` outside `continuation/` is actually a conversation, and the old compact recovery image likewise is not adequate recovery evidence. Their filenames alone are not acceptance proof.

Streaming: custom Chat Completions fixtures do not implement Gator's Responses streaming path. Initial files named streaming showed completed replies, so the new files were renamed as conversation captures. Actual token streaming was then captured through the **existing** `Fixtures/provider.mjs` Responses fixture, with only its 350 ms inter-delta pause changed to 5,000 ms in a disposable copy. It ran on loopback with a non-secret placeholder credential and a disposable endpoint override. Both `final-token-stream-full-dark.png` and `final-token-stream-compact-dark.png` show `Native integration` before the second delta, a running request and responsive native controls. This is real bundled Go/Swift streaming with synthetic model output, not remote provider inference or an FPS benchmark.

Visual review used actual screenshots and the established references: [Apple macOS HIG](https://developer.apple.com/design/human-interface-guidelines/designing-for-macos.md), [Apple accessibility HIG](https://developer.apple.com/design/human-interface-guidelines/accessibility.md), [Agentrooms](https://github.com/baryhuang/claude-code-by-agents), [Clarc](https://github.com/ttnear/Clarc). Standard unified toolbar/window proportions, restrained native materials, system type, SF Symbols, selectable exact text, wrapped addresses/errors and bounded attachment rows remain consistent between surfaces. Navigation stays contextual through the chat/task disclosure rather than introducing a new sidebar. Minimum-window recovery requires scrolling, and its lower evidence remains reachable. Native button/toolbar appearance differs between macOS 15 and 26; this is not evidence of a Gator regression. Ordinary light/dark visual review passes; preference variants and complete hover/focus/assistive traversal remain bounded as below. No confirmed new P0/P1 or reproducible native visual defect was found, so no speculative UI redesign was made.

### Application changes

The user's continuation explicitly requested a black-and-white alligator-head menu/app icon. Replaced the bubble status symbol with a bundled 36 px PNG template displayed at 18 pt with the existing Gator accessibility label. The same original silhouette produces the black/white ICNS and 1,024 px PNG. It is distinct artwork, not a copy of Crocs' mark. Source is `scripts/make-app-icon.swift`; PNGs are in `macos/Resources/`; packaging copies both PNGs. No paid image generation was needed. Actual status-item capture and extracted bundled icon are indexed below. This was the only production behavior/presentation change in this continuation; Go and the coordinator/protocol were unchanged.

The existing closure duplicate-document regression ran both narrowly and in the final suite. Investigation remains conclusive: native filename validation, staging and draft validation accept distinct full upload IDs; `upload_submit` preserves those ordered IDs. Go loads distinct immutable paths, then `LoadUploadsWithOriginals` reduces provider names to basenames. `loadWorkPromptAttachmentSelection` invokes `ValidateLoaded`, rejecting equal names; retained originals also use basename identity and `ValidateOriginals`. Removing that check would not supply a coherent provider/retention identity contract. Supporting duplicates therefore requires broader backend changes. Safe rejection is retained as allowed by the request: no overwrite, rename, execution, draft loss, or weakened Go authority. Known P2 limitation, with distinct bytes/IDs and independent removal tested.

### Provider verification and operator repair

Earlier Gemini evidence remains **FAIL**: HTTP 401 explicitly reported an invalid API key, sent by the existing `x-goog-api-key` adapter to the standard Gemini endpoint. This continuation has no default config/auth store and no Gemini environment key, so the earlier credential cannot be inspected here. No Gemini retry was made. Unsupported model/endpoint behavior cannot be established before successful authentication.

After the user's explicit follow-up authorization, read `.env` privately: nonempty `OPENAI_API_KEY` and `OPENAI_MODEL=gpt-5.6-terra`. One actual request through `internal/model/openai.Responses.Complete` to `https://api.openai.com/v1/responses` returned **HTTP 401** in 1,446.4 ms. No output, tool calls or usage was returned. A disposable transport capped `max_output_tokens=512`, allowed one request only, refused redirects, and used a 45 s timeout. No retry, key/config change, new provider setup or persistent authentication store was made. The CLI's reported-token budget is not misrepresented as an output cap. Sanitized result and reproducible harness source: `headless/openai-env-smoke.json`, `headless/openai-smoke.go.txt`. No actual credential appears in either. Authentication rejection is verified; the specific OpenAI credential cause and model compatibility are **NOT TESTED**. No dollar cost is invented from missing usage.

Operator repair paths on this Mac: `/Users/gongahkia/Desktop/coding/projects/gator/.env` for the authorized OpenAI environment values; default Gator settings `/Users/gongahkia/.config/gator/config.json`; credential store `/Users/gongahkia/.local/state/gator/auth.json` (the last two currently absent). Existing Gemini repair flow is `gator provider login gemini --prompt`, or the acceptance bundle's `Contents/Resources/gator-runtime` with those arguments. Honor existing GATOR/XDG overrides on another host. Never put keys in screenshots, reports or CLI arguments. Authentication is an integration blocker, not a macOS design failure.

### Bounded performance

Debug builds. Fixture sizes/repetitions are emitted in `headless/swift-final.log`; environment in `headless/environment.json`. Existing projection optimization is unchanged. No arbitrary pass threshold or whole-UI speedup is asserted.

| Metric | Method / repetitions | Result | Limit / baseline |
| --- | --- | --- | --- |
| Process-cold native startup | One LaunchServices launch of final bundle with controlled retained profile; monotonic AX polling | First accessible window 1,230.594 ms; connected observed 1,230.597 ms | Includes polling/query cost. Not cache-cold startup or time to first rendered pixel; no comparable baseline. |
| Go time to connected | Four sequential fresh coordinator/Go starts | 62.498, 79.122, 69.328, 73.237 ms; median 71.282 ms | Headless; host/caches differ from original Intel samples. |
| Transcript decode / projection | Seven repetitions each; 50/500/2,000 retained rows; 24,084/240,934/964,934 JSON bytes | Median ms: 0.506/0.022; 5.249/0.215; 20.595/0.853 | Same fixture/method as earlier fast path. Earlier same-host Intel projection 191.568→2.570 ms remains historical; current M3 cannot supply a new comparable before/after ratio. No UI rendering claim. |
| Streaming state handling | Five batches, each 30 snapshots ×500 retained messages, 2,176,935 wire bytes; production framing and state replacement | Median 202.436 ms/batch; 6.748 ms/snapshot | No visible SwiftUI rendering, network, frame-rate or sustained-stream claim. |
| Four Go owners | Settled four-owner headless `ps` sample | RSS 35,984–36,592 KiB each; reported CPU 0.0% each | One sample, no peak/sustained-energy claim. |
| Native idle CPU/memory | Three actual app +two-Go-owner `ps` samples 1 s apart | Native CPU 12.8→0.3→0.0%; RSS 138,080 KiB; Go RSS 39,088/41,984 KiB | First sample includes recent interaction. macOS `ps` CPU is a decaying average over up to a minute, not instantaneous. Samples are not an energy profile. |
| Thumbnails | One batch of12 deterministic 512×512 PNGs, 11,116,140 staged bytes, 160 px bounded previews | 67.732 ms; 930,144 total thumbnail bytes | Peak test-process RSS 49,037,312 B includes earlier tests; isolated thumbnail memory NOT TESTED. |
| Native long history / resize | Original200-revision full-window screenshots; current minimum-size recovery scroll | Qualitatively readable/scrollable | Scroll frame time, p95/p99 interaction latency, true cache-cold launch and long-duration CPU/energy: NOT TESTED. |

### Verification and original failures

Final production edits were the icon changes. One full Swift regression pass followed them; no assertion was weakened. Other implementation areas are unchanged and are backed by the original regression record plus relevant current checks.

| Command / scope | Final result / evidence |
| --- | --- |
| `go build -o /tmp/gator-native-runtime ./cmd/gator` | PASS; `headless/go-build.log` (empty after successful warm builds would also be valid). |
| `go test ./cmd/gator ./internal/workapp ./internal/attachment -count=1` | 3 packages PASS; 32.129 /10.180 /1.582 s; `headless/go.log`. |
| `swift build --package-path macos --scratch-path /tmp/gator-native-build` | PASS; `headless/swift-build.log`. |
| `swift test --package-path macos --scratch-path /tmp/gator-native-build --no-parallel` | **130 PASS /38.350 s**, zero failures; `headless/swift-final.log`. |
| Focused duplicate-basename +stream benchmark | 2 PASS /2.257 s before final icon edits; `headless/swift-targeted.log`. |
| `node --test app-host/tests/drafts.test.mjs app-host/tests/uploads.test.mjs` under Node26.7 | **16 PASS**; `headless/host-rerun.log`. |
| `GATOR_TEST_BINARY=/tmp/gator-native-runtime bun test app-host/tests/preparation-recovery.test.ts` under Bun1.4.2 | **9 PASS**, 208 assertions; `headless/host-integration-rerun.log`. |
| `bun test --cwd ui --preload ./scripts/preload.ts tests/approval-review.test.tsx tests/draft-synchronization.test.tsx tests/attachments.test.tsx` under Bun1.4.2 | **2 PASS, 1 existing platform skip**, 45 assertions; `headless/tui.log`. |
| Packaging, plist lint, Mach-O/module metadata, executable hash comparison, actual bundled Go connection | PASS; `headless/package.log`, `bundle.json`, `app-metadata.txt`, `runtime-metadata.txt`, `native-libraries.txt`. |
| `git diff --check`; `bash -n macos/scripts/build-dev-app.sh` | PASS. No separate repository Swift formatter/linter was configured. |

Original failed attempts are retained: host run under Node20 with absent `fflate`: 7 passed/1 loader failure (`headless/host.log`); host recovery without `GATOR_TEST_BINARY`: 9 ENOENT setup failures (`headless/host-integration.log`); global Bun1.3.11 rejected lockfile v2 (`ui-install-initial.log`). Repaired prerequisites using installed Node26.7, pinned `npm ci`, isolated npm-exec Bun1.4.2 with `--frozen-lockfile`, and the existing runtime override. Lockfiles and tests were unchanged. Full unrelated host/TUI suites were not rerun; their earlier72/21/40+1 results remain historical, not current totals.

Desktop tooling limits: one fixture process died before being kept in a tool session (connection-refused capture, followed by successful controlled retry); menu targets were missed during shared-desktop input; an AX focus setter returned success without focusing the native decision. The settled/menu and actual Tab journeys resolved those specific evidence gaps. An interrupted lifecycle automation batch did not complete its compound assertion. These are not silently converted into PASS or diagnosed as app deadlocks.

### Final screenshot index

See [complete index](Evidence/Tranche08-Closure/continuation/SCREENSHOT-INDEX.md), which separates exact final-binary images, pre-icon continuation images, older supporting captures and excluded/mislabeled frames. The request's representative empty/ordinary/long/streaming/background/attachments/action/plan/recovery/light/dark/resized states all have actual app evidence across those clearly identified generations. This is not a complete Cartesian matrix of every state ×surface ×appearance ×accessibility preference.

### Final acceptance matrix

PASS is scoped to the behavior/evidence named, not a blanket claim about all platform variants.

| ID | Category | Final status | Evidence / exact outstanding requirement |
| --- | --- | --- | --- |
| A | Native foundation | PASS | SwiftUI/AppKit; actual bundled Go executable. |
| B | Menu-bar interaction | PARTIAL | Open/Escape/expand/shared draft observed. Remaining: physical Spaces, multi-display movement and focus restoration across those transitions. Operator-only environment coverage; no reproduced defect. |
| C | Full window | PASS | Close/reopen/resize, toolbar/task/back and composer focus; minimum-size scrolling. |
| D | Shared state / managed sessions | PASS |130-test suite, actual shared presentations; one existing coordinator. |
| E | Conversation rendering | PASS | Native prose/code/long-history evidence and fidelity tests. |
| F | Streaming / responsiveness | PASS | Actual partial Responses deltas on both surfaces, active work, bounded production state-handling benchmark. Quantitative rendering limits are under S. |
| G | Composer / keyboard | PARTIAL | Tab/Shift-Tab, Shift-Return, explicit review decision, prior paste/picker/drop and composition APIs pass. Remaining: physical IME composition/Enter safety and full keyboard-only traversal under operator-selected Full Keyboard Access. Operator-only. |
| H | File picker | PASS | Original actual immutable selection/Go admission; current open/cancel. |
| I | Clipboard image paste | PASS | Original actual Command-V and repeated-image Go regressions, current full suite. |
| J | Drag-and-drop | PASS | Original actual document/PNG drops; unchanged current component/integration suite. |
| K | Attachment persistence | PASS | Immutable/restart/ordered-ID/isolation tests; duplicate basename limitation separately recorded. |
| L | Exact action review | PASS | Real pending proposal/top-bottom/compact layout; actual Tab→Return-safe→Space decline; exact Go authority tests. |
| M | Action corrections | PASS | Existing owner-bound/stale/replacement real-Go integration in current suite. |
| N | Plan review / execution | PASS | Original native exact plan inspection/start plus current revision/authorization/recovery tests. |
| O | Recovery / uncertainty | PASS | Actual failure/full/compact/resized inspection, receipts/access/preparation/no-replay integrations. |
| P | Multi-owner navigation | PASS | Foreground draft remains while other owner works/completes; current owner/capacity/isolation tests. Physical Spaces focus remains B/R. |
| Q | Ordinary light/dark | PASS | Actual native standard-appearance screenshots and semantic contrast tests. Accessibility preferences are R, not silently included. |
| R | Accessibility | PARTIAL | Semantic/AX/control/composition tests and targeted desktop keyboard evidence. Remaining: all ten physical/operator checklist rows, including VoiceOver speech, IME, Full Keyboard Access, Increase Contrast, Reduce Motion, larger text and cross-surface focus. AX metadata does not verify spoken behavior. |
| S | Performance | PARTIAL | Bounded startup/projection/stream/idle/owners/thumbnail evidence obtained. Remaining: measured native long-scroll/render frame times and sustained interaction/energy profile; true cache-cold startup and isolated thumbnail allocation are NOT TESTED. Profiling/evidence gap, no demonstrated P1 regression. |
| T | Local bundling / lifecycle | PASS | Actual bundle launch/connection, held-work presentation close, review Quit guard and final native/child exit. Signing is separate. |
| U | CLI/TUI compatibility | PASS | Required3 Go packages and relevant current host/TUI checks; earlier broad evidence preserved. |
| — | Duplicate user document basenames | FAIL | Known P2 unsupported selection; safely rejected by shared Go provider/retention validation. Draft/bytes/removal preserved. Backend identity change deferred, no silent renaming. |
| — | Physical VoiceOver / IME | NOT TESTED | Human operator required; checklist remains unsigned. |
| — | Remote provider inference | FAIL | Historical Gemini401 plus one explicitly authorized OpenAI `.env`401. Successful remote inference NOT TESTED. |
| — | Signed/notarized release | DEFERRED | No distribution signing/notarization performed or claimed. |

### Six separate verdicts

1. **Functional migration completeness — PASS for the implemented eight-tranche scope**, with the explicit safe duplicate-basename P2 limitation. Managed sessions, conversation during work, attachments, exact decisions/corrections, plans, recovery/access/preparation/receipts, no replay and Quit guards retain passing regression evidence. No new reproducible P0/P1 was found.
2. **macOS-native visual and interaction quality — PARTIAL final acceptance.** Ordinary native visual review, exact/recovery inspection, controlled streaming and targeted window/navigation journeys pass. Spaces/multiple displays and the operator's broader focus/keyboard variants remain unverified; no redesign or new tranche is justified by those gaps.
3. **Accessibility acceptance — PARTIAL.** Functional migration implementation is complete while physical accessibility acceptance remains outstanding. Spoken VoiceOver, physical IME and operator preference tests cannot be signed off by this run.
4. **Real-provider integration evidence — FAIL / configuration blocker.** Real authentication requests occurred, but both available provider paths returned401; successful inference remains unverified. Repair locally before a new bounded smoke.
5. **Local development bundle readiness — PASS.** `/tmp/Gator-Native-Tranche08-Acceptance.app`, arm64 on this host, metadata/native executable/real bundled runtime/PNG/ICNS verified. Six files, no credentials or config. Native executable SHA256 `61bdfc5d3f3198e2cee9ec2b762869c481c676dcaeede5347b6b1fc922e89fc2`; Go SHA256 `40b788eeb831711a278e09e80365346830d1d305a86f852fda64e4460740d1fa`. Go build metadata correctly records the existing HEAD with uncommitted icon work. This is not an Intel/universal build claim.
6. **Signed/notarized distribution readiness — DEFERRED.** No Developer ID/notarization/release-distribution acceptance is claimed; compiler/linker ad-hoc signatures, if present, are not that acceptance.

Closure work and the final assessment are complete within available tools. Acceptance remains conditional on the specifically named operator, profiling and credential prerequisites. No README change, commit, Tranche09, new runtime/coordinator/protocol, credential implementation, or backend-goal resumption was performed.

## User-requested provider and notch continuation — 2026-11-17

The closure above is preserved. The user subsequently requested all-provider diagnostics, installation and cheap live tests of Pi/OpenCode, plus menu-bar motion and a conditional notch interaction referencing boring.notch. This remains Tranche08. Implementation/evidence details are in [provider diagnostics](PROVIDER-DIAGNOSTICS.md) and [menu bar/notch verification](MENUBAR-NOTCH-VERIFICATION.md).

The current final app is **`/tmp/Gator-Native-Tranche08-Enhanced-Final.app`**. Earlier bundles were not overwritten. It includes safe rejection diagnostics for all38 factory provider IDs plus custom/local endpoints; host-known Codex/Claude/Pi CLI failure metadata; the original alligator branding; discrete menu-bar status; and a real-notch-only, nonactivating quick-access panel using the existing owners. Keyboard entry was corrected after a native check; the final ⌘⇧G/Tab/Shift-Tab/Escape journey preserves the composer draft and one Go child. No approval or replay decision was added to the notch.

The `.env` OpenAI key/model pair **does produce real output** through Chat Completions. The new bounded Responses request still returned401; its exact provider-side cause remains unresolved. Additional real output passed through standalone Pi, Gator's sandboxed Pi adapter and standalone OpenCode using disposable text-only Chat configurations. This supersedes the earlier blanket statement that successful real inference was blocked. It does not establish working Responses, Gemini or OpenCode Zen/Go gateway access. No credential or saved default was changed, and no exact USD charge was measured.

Final checks: Go build PASS; all15 relevant Go packages PASS; Swift build PASS; final132 Swift tests PASS /37.262s. Additional authenticated/keyless/managed-local rejection and private CLI-trace persistence tests PASS (cmd/gator and telemetry). Host16 PASS; recovery integration9 PASS; selected TUI2 PASS +1 existing platform skip. Original failures, final reruns, live results and bundle/hash/credential inspection are retained in the two new evidence directories. Existing projection/performance results remain historical; no global rendering-speed claim or new sustained energy measurement is made.

| Acceptance area | Current judgment | Exact limit |
| --- | --- | --- |
| Functional migration | PASS within original scope | Existing owner, admission, approval, recovery and Quit contracts remain covered. No reproducible new P0/P1 established. |
| Menu-bar/native presentation | PARTIAL | Final notch focus/draft/close-reopen journey observed; physical assistive access and notch→pending exact-action end-to-end journey remain unverified. The latter fixture ended with a retained result rather than a pending action. |
| Full window / streaming / exact review / multi-owner / appearance / lifecycle | PASS for original recorded contract | Earlier closure evidence plus current regressions; no claim that every representative state was recaptured on this final binary. |
| Composer and keyboard | PARTIAL | Final ordinary notch traversal passed; physical IME and Full Keyboard Access variants still need the operator checklist. |
| Accessibility | PARTIAL | Physical VoiceOver, physical IME and preference variants remain operator-only. AX metadata does not establish spoken behavior. |
| Performance | PARTIAL | Earlier bounded measurements retained. Sustained Instruments energy/long-session rendering and physical display changes were not tested. |
| Real provider integration | PARTIAL | Actual OpenAI Chat/Pi/OpenCode output PASS. Responses401 unresolved; other credentials/providers unavailable for live checks. |
| Local development bundle | PASS | Actual final native executable and Go runtime verified, native hash matches build, no literal repo credential embedded. |
| Signed/notarized distribution | DEFERRED | No signing/notarization claim. |

Separate final verdicts: **functional migration complete within its established scope; native presentation improved with bounded observed interaction evidence; accessibility acceptance still partial; real-provider evidence now positive for the working Chat route and its two tested CLIs but incomplete across routes/providers; local development bundle ready; distribution deferred.** No Tranche09, automatic provider fallback, new runtime coordinator, README change or agent-created commit.

## Final LOCAL-DEVELOPMENT acceptance — 2026-11-17

The focused follow-up is complete; [TRANCHE08-LOCAL-DEV-ACCEPTANCE.md](TRANCHE08-LOCAL-DEV-ACCEPTANCE.md) is the current local-development decision. Original evidence and earlier conditional judgments above remain historical.

**Pending-approval navigation is now PASS.** The actual existing enhanced-final bundle, an inert loopback advertised-tool fixture and actual bundled Go runtime established pending approval on a child owner while unrelated parent work ran. Dismissal, notch attention/owner navigation, compact review and full expansion preserved the same interaction/token/review hashes. Actual Tab focus on Approve → Return sent no decision; Shift-Tab → Space on Decline sent exactly one original-owner decline. The parent draft remained, unrelated work continued until explicitly released, and no external effect occurred. The previously unverified notch approval gap is closed.

**Responses401 is now diagnosed as the provider's current project-geography rejection:** “Attempted to access resource from outside project geography US.” One permitted minimal canonical request obtained this detail, with32 output tokens maximum and no retry/fallback. This does not establish a missing Responses API or invalid key. Credentials, `.env`, defaults and the working explicit Chat path are unchanged; provider/project administration is required to resolve that route's geography authorization. No app workaround was added.

Final relevant checks: required Go build PASS;15 Go packages PASS; Swift build PASS; **133 Swift tests PASS /48.335s**, including a new real-Go inert approval/navigation regression. Host16 PASS; recovery9 PASS /208 assertions; TUI2 PASS /45 assertions with1 existing platform skip. No headless failure occurred; an unreliable AX focus setter was excluded and replaced by verified native Tab traversal. See `Evidence/Local-Dev-Acceptance/` for commands, assertions, images, request comparison and the single redacted live result.

No production Swift/Go change was needed, so `/tmp/Gator-Native-Tranche08-Enhanced-Final.app` is retained; all six bundle hashes match the earlier candidate and its native binary matches the fresh build. A duplicate development bundle was unnecessary. Changes are test fixtures/helper/regression and acceptance documentation only.

Practical local-development acceptance **can close for the verified native workflow and working explicit Chat route**. Physical VoiceOver/IME/accessibility preference variants remain human NOT TESTED; the concise operator table is updated. Responses geography remains a provider blocker. No reproduced P0/P1; the existing safe duplicate-document-basename P2 limitation remains documented. No Tranche09, distribution work, README change, commit or backend-goal resumption.
