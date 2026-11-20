# P04U-1 — Streaming motion and native presentation

Development implementation with headless verification. Full packaged-native acceptance is **NOT TESTED**; release readiness remains **BLOCKED**. The user explicitly deferred P04E-1R2 desktop acceptance. This increment does not resolve or waive that debt.

## Baseline and boundaries

Starting HEAD: `5258dab779a9ecb2e95f761224fed88b32cca3f1`, initially clean. Source, toolchain and host hashes are in [baseline.json](evidence/p04u-1/baseline.json). HEAD advanced externally to `dd9790d4a` during the work; no commit was made by this agent. Final identities and inventory are recorded separately below. No Go authority, bridge transport, coordinator, credential, draft, approval, recovery, plan, workspace or mascot asset implementation was changed.

Host: Apple M3, 16 GiB, macOS 26.5.2. SwiftPM debug configuration; macOS 15 minimum retained. The SDK/compiler are newer than macOS 15. Physical macOS 15 runtime acceptance is **NOT TESTED**.

Read the P04E-1, R, R2 reports and their open conditions; Tranche08 and native UI/UX audit/remediation reports; current transcript/body/composer/reading-position/presence/preferences/desktop-pet/review/inspection/application code; current core public projections, connection and coordinator. Earlier remediated Markdown, nested lists, contrast, toolbar, exact review and draft findings were not reclassified as new defects.

## Baseline assessment and remediation ledger

“REPRODUCED” below means source/fixture or deterministic component reproduction unless explicitly stated otherwise. No shared-desktop input was sent in this increment.

| ID | Severity / evidence | Concrete finding and before evidence | Implemented change / after evidence |
|---|---|---|---|
| U1 | P2, REPRODUCED in source and synthetic streams | Public text rendered the complete arriving snapshot at once; large chunks had no presentation interpolation. [Baseline source](evidence/p04u-1/baseline-ConversationView.swift.txt). | Adaptive grapheme/word micro-batches for the active plain-prose tail. Engine, multilingual, burst, replacement and completion tests; strategy benchmark. Formatted Markdown retains the existing native renderer. |
| U2 | P2, REPRODUCED in source; visual scroll consequence INFERRED | Every activity change invoked `scrollToLatest` while following, including changes unrelated to text. Adding per-character scrolling would amplify this. | Remove that activity callback. Use macOS 15 native `.sizeChanges` bottom anchoring only while explicitly following, not scrolling or searching. Retain initial restore, search target, hysteresis, saved anchors, explicit Jump to Latest and completed-transcript handling. Actual window-system follow behavior remains NOT TESTED. |
| U3 | P2, REPRODUCED by pure condition test | Empty transcript selected the welcome screen even when public work/dialogue preparation was active. | Welcome requires empty **and** no running/considering activity. Work status remains visible in the transcript path. |
| U4 | P3, REPRODUCED by source/projection tests | “Work activity” disclosure existed with no tool activity; busy spinner and considering notice could duplicate status. | Omit an empty tool disclosure, retain inspector access in existing controls, use one active-work status and a static exact-review waiting label. No approval control was modified. |
| U5 | P2, REPRODUCED by source/copy test | Readiness notice sent the user to terminal provider setup despite native P04 support. | Link the existing Models and Providers workflow, preserve the authoritative issue, and distinguish local configuration checks from live provider verification. |
| U6 | P3, REPRODUCED in hosted component comparison | Continuous busy spinner was the ordinary work indicator even though emoji presence already supplies an additional activity cue. | A restrained text-only opacity pulse with a static SF Symbol replaces that spinner. Static waiting/stopping labels do not pulse. Light/dark and high-contrast semantic-label contrast tests pass. |

Assessment of remaining surfaces: retained prose/code/list selection remains on the native block renderer; composer input, IME and keyboard shortcuts are unchanged; emoji/GIF and desktop presence retain their preferences/assets; task/plan/recovery/exact reviews retain their literal evidence and controls; model/workspace/catalog controls retain existing ownership and flows; notch, menus and window sizing remain unchanged. Their physical interaction, minimum-window and accessibility variants are NOT TESTED here. Pending-review layout/CPU debt remains P1 investigation; provider/runtime/queue work stays in its separate tranches. P11 remains the final whole-product audit.

### Surface review and remaining P11 items

| Area | Current-source assessment | Evidence / remaining acceptance |
|---|---|---|
| Conversation and history | Native lazy rows, block Markdown and structured details retained; active prose tail isolated from history. | Deterministic renderer/content tests and 400/780 pt component images. Long-history actual scrolling NOT TESTED. |
| Status and welcome | Conditional welcome, exact-review waiting and single work status implemented above. | Pure projection tests; native component layout/contrast. Actual pending-review CPU remains unresolved. |
| Selection and code | Existing native selectable content retained; full confirmed Copy Response action added. | Private pasteboard exact-byte test; code/Markdown regressions. Physical live selection NOT TESTED. |
| Composer and focus | Existing AppKit editor, submission guards, IME path and focus requests unchanged. New pulse/probe cannot become first responder. | Existing input tests plus new nonfocusable-probe assertions. Actual typing, IME and transition focus NOT TESTED. |
| Reading and Find | Existing context anchors, search target/reveal and explicit Jump preserved. Per-activity imperative follow removed. | Policy, old Find/reading regressions and no-anchor-write tests; actual follow behavior remains a P11/native acceptance item. |
| Emoji/GIF and desktop | Current public-selected-owner presence, finite GIF assets, independent controls and Tuck Away preserved. | Existing presence/desktop suite; two packaged GIFs match prior accepted bundle hashes. Actual overlay/window behavior NOT TESTED. |
| Review, task, plan and recovery | Their evidence, decision buttons, digests, tokens and presentation ownership untouched; motion suppressed for active review/inspection. | Existing exact-identity tests and real-Go negative navigation/shutdown regression. Historical approval provenance remains INVALID, not “fixed.” |
| Model/provider, workspace and catalog | Existing menus/actions/search/rename and owner handoffs retained. Readiness guidance corrected to existing native provider setup. | Full current suite including P01–P04 integrations. Fresh native picker/setup/catalog interactions NOT TESTED. |
| Menu bar, notch and resizing | Existing window/menu/notch ownership and size conventions retained. Compact transcript uses the same bounded presenter. | Existing geometry tests, 380/540/780 pt component sizing; minimum-window controls and physical monitor behavior NOT TESTED. |
| Appearance/accessibility | System semantic colors/type retained; Reduce Motion disables both new effects; no per-character announcement. | Native light/dark and increased-contrast color resolution. Physical VoiceOver/IME/larger-text acceptance NOT TESTED. |

No new P0 was reproduced. The existing P1 layout/guarded-Quit debt and pending-review resource uncertainty remain open. The principal new acceptance risks for P11 are live selection stability and native follow anchoring under actual input, full-app streaming cost beside a local model, and physical accessibility. These are unverified conditions, not newly proven product failures.

## Chosen reveal strategy

Compared strict character reveal, word reveal and adaptive micro-batching using the same cursor and datasets. [Reproducible benchmark](lab/RevealBenchmark.swift), [results](evidence/p04u-1/reveal-strategies.json).

Strict characters generate excessive updates and mechanical delays. Strict words help English, but segmentation alone remains slow on a large backlog and is not an adequate CJK pacing strategy. The selected adaptive method combines a grapheme-count backlog budget with measured arrival-rate smoothing, then snaps within a bounded word window where available. It uses Apple's [NLTokenizer](https://developer.apple.com/documentation/NaturalLanguage/tokenizing-natural-language-text) with grapheme fallback, including non-whitespace scripts.

Final maximum presentation cadence: **66 ms**, approximately 15 updates/second while a reveal is actually pending. This is a presentation cadence, not model tokens/second. No permanent display link, timer per word, glyph view, glyph layer or idle polling. One cancellable task per eligible visible text stream sleeps only while the cursor has pending text or a final entrance-color settlement. Key-window/application visibility gates avoid simultaneous full/compact animation. Go updates are never throttled or discarded.

The trailing batch is slightly softened at 0.88 opacity and becomes fully legible on the next cadence. Earlier text remains fully opaque. There is no bouncing, scaling, flashing or animation applied to a completed answer.

### State machine and identities

1. First observation of an entry establishes a fully visible baseline. History, restored content, navigation and reconnect therefore do not replay an answer.
2. Same-identity prefix appends may reveal incrementally. Identity includes native context/generation, Go activity start, entry ID and entry start. Duplicate/missing identity or multiple running text entries stays static; positional fallback IDs are used only for static reconciliation.
3. Non-prefix replacement, shortening, clearing, earlier-block revision, completion, failure/stopped entry, cancellation, review, disconnect, preference/Reduce Motion change, reading mode or hide settles confirmed text immediately and cancels the task.
4. Owner/run replacement cancels the previous cadence immediately. A new baseline is shown, never a stale backlog.
5. View disappearance clears parsed blocks/cursor buffers and cancels its task. Weak-model tests establish that the sleeping task does not retain its owner.

The cursor indexes extended grapheme clusters and UTF-16 boundaries. Appending a combining mark, skin tone/ZWJ continuation or flag component rebuilds the previous last cluster plus the delta. It does not build a fresh full-string prefix every tick. Cursor storage is capped at 8,192 graphemes / 32,768 input bytes; excess text stays immediate. Backlog above 2,048 graphemes flushes instead of accumulating a long typing queue. Large completed answers have no queued finish delay.

Only existing public `kind=text`, running entries are eligible. Parent narration from specialist-mediated work is handled through the same public projection; private child task traces, user messages, structured results, tool arguments, source evidence, identifiers, approvals and recovery payloads never enter this presenter. Provider type is irrelevant; local and cloud models use the same bridge contract. No response is predicted or inferred.

### Markdown, copying and accessibility

Only the final **plain paragraph** is progressively revealed. Headings, lists, emphasis, links, inline/fenced code and paragraphs containing Markdown delimiters immediately use the existing renderer. Earlier blocks are equatable subviews, with byte-sensitive equality so a Unicode replacement is not hidden. Blocks are parsed once per authoritative source change, not once per cadence. Upstream incomplete Markdown may still reflow under the existing renderer; this increment does not claim a new complete Markdown parser or perfectly stable partial formatting.

`Copy Response` explicitly copies the complete confirmed public entry with exact original Markdown/Unicode bytes, independently of the presentation cursor. The test uses a private pasteboard, not the user's clipboard. Ordinary selected text uses the existing native selection behavior; physical mouse selection/copy during a live stream is NOT TESTED. Code block whitespace and formatting remain unchanged.

The current paragraph has a stable authoritative accessibility label rather than character announcements. No announcement callback is added. System Reduce Motion displays text immediately and stops the pulse. Semantic label colors, system typography and native focus remain in use. Physical VoiceOver/IME, system text-size preferences and operator keyboard acceptance are NOT TESTED.

## Thinking pulse and preference

Settings adds **Animate Streaming Responses**, default ON, in Conversation Motion. It is independent of Agent Presence mode, Animate Reactions, in-chat/desktop visibility and Tuck Away. Private-domain persistence tests preserve explicit OFF settings. No migration rewrites existing preferences, archives or Go state.

A single Core Animation opacity animation belongs to the native status label while genuinely active: 0.82–1.0, 1.3 s each direction, ease-in/ease-out. It produces no application update timer or bridge events. Precedence: exact approval → static waiting; cancelling → static stopping; Go dialogue busy → considering; Go running → authoritative status or generic working; otherwise absent. No invented research/planning stages or elapsed-time completion signal.

Hide, window close/resign/occlusion, app deactivation, owner change, review/inspection, closing, disconnect, animation OFF and Reduce Motion stop motion. The small visibility probe has no first-responder behavior, weak observers and explicit dismantling. Pulse lifecycle tests inspect the installed animation contract; they do not pretend an unattached test layer proves physical rendering.

## Presentation experiments and rejected optimization

The initial 42 ms selectable SwiftUI tail showed higher hosted streaming CPU than immediate text. An isolated AppKit text-store experiment reduced layout cost, but its 100-view release probe did not establish complete teardown within the bound. Even an unconfigured NSTextView exhibited delayed retention. The exact framework retaining path was not established; it is not asserted to be a production leak.

That experiment is **not shipped**. Its source/tests are preserved under [rejected-native-prose](lab/rejected-native-prose/), with original failures in the evidence directory. No failing assertion was weakened to accept it. The final path uses existing selectable SwiftUI Text and a 66 ms bounded cadence; the earlier 42 ms and native-text-store measurements remain separately identified, not mixed into the final comparison.

## Scroll and execution preservation

Presentation state does not write NativeReadingPositions, draft stores, snapshot data or coordinator state. Search disables reveal/follow motion; user scrolling disables follow immediately. A native `.sizeChanges` anchor replaces imperative per-activity scrolling; no cadence calls `scrollTo`. Saved anchors/search reveal/Jump to Latest remain explicit. Headless policy and reading-position tests pass; physical scrolling/focus acceptance remains open.

`hasSameNativeContent` and the existing 100-spinner/zero-owner-callback regression are unchanged. All non-spinner meaningful fields, Unicode source identity, elapsed time, unknown fields, exact approval/recovery identities and owner generations retain existing publication/validation. Two presentation instances create no Go owner, submission or directory; real-Go loopback streaming checks preserve generation, context and unsent draft.

## Native evidence and deferred acceptance

The PNGs below are **headless NSHostingView component renders**, not screenshots of the packaged app or recordings. They exercise native Markdown/status layout at full/compact-like widths in light/dark appearance, without activation or desktop input:

- [Before, 400 pt light](evidence/p04u-1/component-before-400-light.png), [after](evidence/p04u-1/component-after-400-light.png).
- [Before, 780 pt dark](evidence/p04u-1/component-before-780-dark.png), [after](evidence/p04u-1/component-after-780-dark.png).

No trustworthy actual packaged-app motion recording was obtained; none is fabricated. No old ten-cycle acceptance work was attempted. Full/compact interaction, live selection, Find reveal scrolling, actual pulse/stream visual smoothness, physical accessibility and hidden-window compositor behavior remain NOT TESTED. GPU activity/energy are NOT TESTED.

Carry-forward blockers remain exactly: historical accepted approval input has unknown provenance and its negative test is INVALID; prior 1.4 GiB layout stall/failed guarded Quit is unresolved; approximately 23–25% pending-review CPU has not been independently explained or accepted; ten complete native cycles are unverified. New headless results do not resolve those facts.

## Verification, resources and delivery

Final measured results, commands, hashes and exact inventory follow below after verification. Production native acceptance remains deferred regardless of the headless outcome.

### Final performance evidence

[Matched trials](evidence/p04u-1/hosted-final-comparison.json), [all phase summaries](evidence/p04u-1/hosted-resource-summary.json), [harness](lab/MeasureHosted.py), [measured-source identity](evidence/p04u-1/benchmark-source-identity.json).

Three fresh processes per mode, order alternated, same debug test binary and fixture. The reference is **immediate NativeMessageBody presentation using the current common block renderer**, not the prior packaged app. Each process has 3 s idle, 240 scheduled hosted-layout iterations (42 ms fixture interval; authoritative append every second iteration), 3 s settled and 3 s hidden-component policy. Final reveal cadence is independently 66 ms. Layout cost extends actual wall time; phase timestamps are logged. The external read-only R2 sampler uses calibrated Mach timebase conversion, percentages of **one core**, and one-second samples. No model TPS is inferred.

These measurements include a headless NSHostingView, source updates and native layout. There is no visible NSWindow, real compositor, user scroll or typing measurement. They cannot certify actual application responsiveness, pending-review CPU, window hiding or GPU energy. The deliberately disabled pulse in the hosted fixture excludes visible Core Animation cost; pulse's physical compositor cost is NOT TESTED.

Values below are medians of the three trial medians; peak is the largest sampled lifetime peak. RSS and physical footprint are distinct:

| Measurement | Immediate reference | Motion candidate |
|---|---:|---:|
| Idle CPU, % one core | 0.000 | 0.000 |
| Streaming CPU, % one core | 9.521 | 17.269 |
| Settled CPU, % one core | 0.000 | 0.015 |
| Hidden-component CPU, % one core | 0.000 | 0.007 |
| Streaming RSS, MiB total | 54.414 | 60.656 |
| Streaming physical footprint, MiB total | 14.555 | 15.532 |
| Settled physical footprint, MiB total | 15.188 | 16.563 |
| Hosted layout median, ms | 2.168 | 3.663 |
| Largest layout iteration, ms | 37.227 | 44.084 |

The candidate costs more active CPU: about **+7.75 percentage points** in this fixture; it is not presented as a CPU optimization over immediate display. Incremental streaming physical footprint is approximately **+0.98 MiB**, streaming RSS **+6.24 MiB**, settled physical footprint **+1.38 MiB**. Largest sampled lifetime footprint is 16.59 MiB candidate versus 15.25 MiB reference. Idle and hidden-component work settle near the sampler floor. No sustained post-stream layout loop was observed in these bounded trials. Three fresh-process trials cannot establish unlimited long-session stability; repeated stream/task cancellation tests provide a separate lifecycle boundary.

The initial 42 ms SwiftUI-tail and rejected native-text-view trials are retained in `initial-swiftui-tail/` and `initial-native-tail/`. They are not final candidate measurements. No high-memory test or cleanup failure was relabeled PASS. The active cost remains a native acceptance/performance consideration for the complete app beside a local model.

Strategy comparison uses 10 deterministic runs per dataset (not a real model benchmark). For the 1,680-grapheme burst, strict character/word/adaptive paths require respectively 1,681 / 282 / 5 ticks, including final color settlement. At completion every strategy has zero pending characters; the largest measured synchronous cursor completion reconciliation across datasets is 149.04 μs in the optimized standalone benchmark. This is not whole-app final-paint latency. Oversized content bypasses animation entirely.
