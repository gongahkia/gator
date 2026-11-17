# Tranche 08 — Native UI/UX audit and remediation ledger

Full review of the current SwiftUI/AppKit app, macOS 15+, existing semantic typography and native controls. Baseline HEAD `08726774096118a1f063fd55388372cbcd1f3561`; existing uncommitted Tranche 07 changes are preserved. No README edits, commits or paused backend work are authorized by this audit. This document was created before presentation remediation.

## Method and rubric

Conversation is primary. A successful interface makes the next useful action apparent, keeps reading and drafting comfortable, and exposes precise decisions/evidence progressively. Go remains authoritative; visual simplification cannot create execution authority. Native layout, readable semantic type, standard editing, keyboard reachability, restrained surfaces, selectable review text and clear uncertainty are assessed separately from provider/backend correctness.

Sources reviewed on 2026-11-16:

- [Apple: Designing for macOS](https://developer.apple.com/design/human-interface-guidelines/designing-for-macos.md): use native windowing, keyboard and pointing-device conventions; adapt to larger/resizable displays.
- [Apple: Windows](https://developer.apple.com/design/human-interface-guidelines/windows.md): standard chrome and fluid resizing; avoid excessive auxiliary windows.
- [Apple: Menus](https://developer.apple.com/design/human-interface-guidelines/menus.md): group related actions, label clearly, keep unavailable choices understandable.
- [Apple: Keyboards](https://developer.apple.com/design/human-interface-guidelines/keyboards.md): preserve standard editing shortcuts and keyboard access.
- [Apple: Accessibility](https://developer.apple.com/design/human-interface-guidelines/accessibility.md): perceivable/adaptable semantics, readable contrast and text, multiple feedback channels.
- [Agentrooms](https://github.com/baryhuang/claude-code-by-agents): reference for native chat composition and restrained hierarchy, not its agent/repository product model.
- [Clarc](https://github.com/ttnear/Clarc): reference for inspecting activity/reviews and session continuity, not coding navigation.
- Local interface-polish skill: full review, applying existing native conventions rather than adding a styling framework. CSS-specific advice is inapplicable to SwiftUI/AppKit.

| Review category | Evidence inspected | Initial result |
| --- | --- | --- |
| Typography | `ConversationView`, exact/plan/recovery content; actual ordinary/plan screenshots | Inline-only Markdown destroys code-block layout and exposes heading syntax. System type is appropriate. |
| Surfaces/spacing | Full idle/chat, task inspector, plan inspector, compact AX tree | Excess nested cards/materials, duplicate headings, heavy status stack and large reserved input/selection regions. |
| Icons | Header, composer, package metadata | Existing SF Symbols suitable; inconsistent native button treatment and missing bundled Finder app icon. |
| Motion | Streaming/activity code, Reduce Motion transaction, accessibility announcer | No custom entrance sequence; existing low-motion path appropriate. 10%-speed replay not applicable to absent custom transitions. |
| Performance | Seven headless samples each at 50/500/2,000 messages | Unnecessary date parsing is measured, not inferred. |
| Native input | AppKit editor/drop/paste code, earlier T07 outcomes, baseline compact/full app | Standard paste/drop acceptance remains unresolved and receives targeted component + actual app checks. |
| Safety/reviews | Existing coordinator, exact token/revision/digest guards, native plan screenshot and tests | No P0 identified in baseline. Safeguards must remain intact. |

## Baseline structure and evidence

One `ManagedSessionCoordinator` sits above menu-bar and full-window scenes. Each managed owner has a `GatorApplicationState`; native drafts/uploads are shared services. Both surfaces currently use the same header/body/footer composition with different dimensions. The transcript uses lazy message rows but wraps nearly every assistant/result/context row in a separate card. Composer is an AppKit text view inside a SwiftUI material. Task, action and plan/recovery reviews replace the transcript, preserving execution ownership.

Controlled baseline process: `/tmp/Gator-Native-Tranche07-Ready.app`, disposable root `/var/folders/pq/lllswlg16q396qgx147zv_mm0000gn/T/gator-native-tranche08-ui-y6h8l76a`, local audit provider. No production account or secret is used for screenshots.

Before evidence under `macos/Evidence/Tranche08/before/`:

- `full-idle.png`, `idle.ax.txt`: duplicate app header, bottom-weighted welcome, input/status allocation.
- `full-conversation.png`, `conversation.ax.txt`: visible `##`, collapsed code lines, repeated assistant result/context cards and footer status clutter.
- `task-inspection.png`, `task-inspection.ax.txt`: contextual inspection and nested materials.
- `plan-review.png`, `plan-review.ax.txt`: repeated objective and plan headings, exact content remains scrollable.
- `compact-idle.ax.txt`: actual compact scene exists and shares state; baseline screenshot collection is continued in the verification index.

Headless baseline: Intel i5-8279U 2.40 GHz, 8 GiB RAM, macOS 15.7.7, Swift 6.1.2/macOS 15.5 SDK, debug build. Seven samples; median decode/projection milliseconds: 50 messages 1.569/5.506; 500 messages 13.357/48.399; 2,000 messages 51.545/191.568. Full log `/tmp/gator-native-t08-bench-before.log`. No arbitrary timing acceptance threshold is imposed.

## Prioritized findings

Skill severity HIGH corresponds to P1, MEDIUM to material P2 and LOW to P3. Unverified hypotheses are labeled; they are not declared reproducible product failures.

| ID / severity | Location | Before | Proposed after / remediation | Why and evidence |
| --- | --- | --- | --- | --- |
| 01 / P1 investigation | `ComposerView.swift:139`, `NativeAttachmentActions.swift:29` | T07 synthetic Command-V did not establish image staging | Trace standard responder delivery; test native paste in a headless component and observe actual app staging/text behavior | Primary input acceptance gap; explicit Paste Image must remain available. Do not infer failure from a missed synthetic event. |
| 02 / P1 investigation | `ComposerView.swift:142`, `NativeAttachmentActions.swift:44` | T07 native drop did not establish a chip | Validate file-URL handling with component tests and a correctly targeted actual native drop | Primary input gap; original context/generation and explicit Send must remain. |
| 03 / P2 | `ConversationView.swift:277` | Inline-only parser collapses fenced code and displays Markdown headings | Native block rendering for paragraphs/headings/lists/fenced code with exact code whitespace and selectable text | Actual `full-conversation.png`; readable output and copy fidelity. |
| 04 / P2 | `ConversationView.swift:50`, `GatorApp.swift:22` | Full window duplicates app title and popover header | Standard full-window toolbar with current chat and contextual actions; keep compact header lightweight | Native hierarchy and effective use of window chrome. |
| 05 / P2 | `ConversationView.swift:152` | Empty prompt sits near bottom of a large empty canvas | Centered, bounded welcome using native type/symbol and concise guidance | Actual idle screenshot; calm first task without repository setup. |
| 06 / P2 | `ConversationView.swift:254` | Every assistant/context/result becomes another padded card | Plain readable assistant prose; compact expandable result/source rows; restrained user treatment | Actual chat screenshot, repeated hierarchy competes with the conversation. |
| 07 / P2 | `ConversationView.swift:188` | Completed admission, processing caveat and routine recovery affordance permanently consume composer space | Concise actionable status; move technical evidence into bounded details, keep failure/uncertainty available | Actual chat/plan screenshot; compact transcript/composer space matters. |
| 08 / P2 | `ConversationView.swift:5` | Fixed dark-green tint on dark gray | Adaptive Gator accent with semantic surface colors and contrast checks | Screenshot links/icons lack dark-mode emphasis. Preserve brand without color-only status. |
| 09 / P2 | `AttachmentSelectionView.swift:30`, `ComposerView.swift:36` | A single file can reserve the maximum attachment region; input size fixed | Content-count-bounded selection viewport and balanced compact/full input sizes | Narrow-layout inspection; preserve long selectable names and scrolling. |
| 10 / P2 | `ConversationView.swift:224` | Error text is truncated to three lines | Concise error summary with expandable/selectable complete detail and recovery action | Source-proven loss of visible error context; no information removal. |
| 11 / P2 | `ConversationView.swift:67` | One long flat menu mixes chats, models, effort, work mode and lifecycle | Native grouped submenus, current selections and unavailable explanations | Existing protocol supplies model/effort authority; no invented options. |
| 12 / P2 | `BridgeProtocol.swift:71` | Timestamp parsing for all rows even with no dialogue | Fast no-dialogue projection; parse only while merging retained turns | Measured 191.568 ms for 2,000 rows, before change. Preserve UTC/offset correctness. |
| 13 / P2 | `TaskInspectorView.swift:85`, `ApprovalReviewView.swift:30` | Repeated objectives/nested translucent panels; fixed horizontal decision rows | Flat native review sections and adaptable explicit decision rows | Exact material consequences remain expanded; no Return authorization. |
| 14 / P2 | `Resources/Info.plist`, `scripts/build-dev-app.sh` | No app icon declared/bundled | Code-generated native green conversation icon in ICNS, metadata checked | Bundle presentation/launch identity. No bitmap generation dependency. |
| 15 / P1 investigation | `ComposerView.swift:127` | Editor handles Return but leaves Tab/Escape to generic NSTextView defaults | Verify keyboard loop, marked text and compact dismissal; repair observed traps with native responder behavior | Keyboard access must not depend on pointer/OS preference mutation. |

## Considered but rejected

| Location | Candidate | Rejected because |
| --- | --- | --- |
| App architecture | Replace MenuBarExtra with a custom floating panel | No reproduced requirement justifies replacing standard scenes or risking lifecycle ownership. |
| Reviews | Pin an Approve button outside all review content | Material review context must remain inspectable and exact identity-bound; layout can improve without weakening review. |
| App appearance | Simulated macOS 26 liquid glass | macOS 15 target; native semantic surfaces/materials suffice. |
| Navigation | Repository sidebar or task dashboard | Conflicts with chat-first personal assistant identity. Existing contextual chat/task access is enough. |
| Upload retention | Prune files based on visible draft chips | Go retained requests/recovery can still own those bytes; cleanup remains conservatively deferred. |

## Remediation / after-state ledger

Before evidence is preserved. The remediation below is implemented; desktop acceptance remains explicitly bounded by the verification report. No P0 authorization/data-loss defect was found. Missing interaction proof is not converted into PASS.

| Finding | Implemented resolution | After evidence / assessment |
| --- | --- | --- |
| 01 | Extracted native editor; explicit responder paste, standard Edit commands, plain-text precedence and composition guard | Real hardware-style CG Command-V reached `paste`, staged pixels and required Send. Earlier Unicode-injected event never reached the responder. Headless AppKit event tests also pass. |
| 02 | Typed file-URL/legacy file-list handling; rejects textual paths/web URLs, captures original import context | Component and actual Go staging tests pass. The initial unbundled drag helper had no visible source window. With a bundled AppKit drag source, actual document and PNG destination callbacks and unsent chips were observed; `gui-app-dark.log` records both. |
| 03 | Shared native Markdown block presentation preserves fenced whitespace, headings and lists | `final/full-conversation-light.png`, `final/conversation.ax.txt`; literal commands/reviews bypass the renderer. |
| 04–06 | Standard full-window toolbar/subtitle; centered welcome; plain assistant prose and expandable result/context sections | Compare before/full-idle and full-conversation with final counterparts. No coding sidebar or dashboard. |
| 07,10–11 | Bounded actionable notice region, complete expandable errors, grouped native model/session/work-mode menus | Full/compact attachment and plan screenshots; connection label explicitly does not certify provider authentication. |
| 08–09 | Adaptive accent (tested ≥4.5 contrast), semantic surfaces, count-bounded attachment viewport and balanced native editor | Light final + dark candidate screenshots; full contrast/preference acceptance remains partial. |
| 12 | Avoid date parsing when no dialogue merge is needed; avoid duplicate message projection | 2,000-message median projection 191.568→2.621 ms in early comparable samples. Loaded repetitions are documented separately. |
| 13 | Flat task/plan/action sections; explicit approval row, full exact text retained | Real Go plan inspection/Approve & Start observed; plan top/bottom screenshots. Token/revision/digest checks retained. Final GUI action/correction/recovery screenshot coverage incomplete. |
| 14 | Bundled code-generated ICNS, productivity category, existing macOS15 metadata | Bundle metadata/resources verified. No downloaded art, external dependency or runtime repository path. |
| 15 | Tab/Shift-Tab key-loop, composition/undo lifetime bound to conversation+generation, bounded text paste and compact Escape | Native AppKit tests and actual Tab-to-Attach observation. Physical IME/VoiceOver remain operator-led. |

### Additional reproduced defect: repeated pasted images (P1)

The running app accepted two separate screenshot pastes, but Go rejected admission: `attachment "Pasted image.png" was supplied more than once`. This is a real native naming defect, not a reason to relax Go validation. `NativeUploadStore` now assigns `Pasted image.png`, `Pasted image 2.png`, etc., checking existing draft names. Opaque IDs and immutable bytes remain distinct. Unit and real Go files-only multi-image delivery tests verify the fix. Duplicate user-selected document names still receive Go's accurate rejection; no original is renamed or overwritten silently.

### Presentation lifetime/performance follow-up (P2)

Process-only fixture appearance was being assigned repeatedly. It is now applied only when the name changes. The observed app sample returned to 0% idle CPU. This does not establish that all prior CPU usage was caused by appearance assignment: host load and AX automation were also substantial.

Native configuration also preserves SwiftUI's own window identifier. Weak references identify compact/full presentations for activation and Escape; they do not own application/session lifetime. Full windows explicitly remain visible on deactivation. Headless AppKit tests cover mapping and scene-identity preservation. This is a precaution against scene restoration interference, not a claim that the earlier synthetic window loss has been conclusively attributed.

### Remaining acceptance limits

- Actual document and image drops are now observed. Multiple displays, real Finder variants and physical accessibility still require operator acceptance. No unrestricted path fallback was added.
- Some AX menu/session interactions timed out or lost their target window. Main-thread sampling included an idle AppKit event loop, so these observations do not prove a product deadlock. Broad desktop/menu restoration acceptance remains **PARTIAL**.
- Final screenshots do not cover every requested review/recovery/background/streaming/long-history state. Missing evidence is listed in the verification matrix.
- Physical VoiceOver, physical IME, preference variants, multiple displays and Spaces require operator acceptance.
- Real provider success is blocked by an invalid existing Gemini key; no credential/default was changed.
- Safe upload pruning remains deferred; preserving retained inputs takes precedence over cleanup.

**After-state verdict:** materially improved native chat hierarchy, readable output and safer native input, with strong headless functional evidence. This is not unconditional first-party-quality or final migration acceptance while the listed desktop checks remain incomplete.

### Additional ownership/input findings during remediation

Finding 15 also covers native editor lifetime: reusing one NSTextView across conversations can retain marked text and undo state while changing its edit callback. The editor now has a context/generation-bound SwiftUI identity. A headless NSHostingView test starts marked text in A, switches to B, verifies a different native editor, and commits the old editor without changing B. This is composition API testing, not physical IME acceptance.

Fresh native profiles now request Go's existing `GATOR_UI_BOOT_MODE=new` so the initial editable conversation has a Go-created durable ID before typing/imports. Existing archives/drafts/uncertain workspace admissions retain their original restore path. A real Go test verifies initial identity, one owner, no model invocation/task/plan and idempotent second-surface startup. This uses existing protocol; it neither creates hidden work nor replaces the coordinator.

The standalone marked-text/focus component fixture initially had a zero-size button and default released-on-close NSWindow. Corrected fixture geometry, explicit focusability and owned window lifetime preserve the same Tab/Shift-Tab assertions; six component cases then passed. No production timing assertion was relaxed.

### Final interaction follow-up

A bundled controlled AppKit drag source established both actual native file drops: `drop-document.md` and `drop-image.png`. The application logged `drop received files=1 accepted=true` twice and its AX tree showed both removable unsent chips. This supersedes the earlier inconclusive drag attempts. No Send occurred on drop. The helper itself is test evidence only; it is not bundled in Gator.

The first final127-test run had one existing lost-admission fixture disconnect/timeout. The unchanged focused rerun passed in2.425s, and a complete unchanged127-test rerun passed in72.105s. Initial failure is retained, not erased. Cause is not proven; resource pressure is a possible contributor.

Activation now remains regular after the full app has opened, preserving normal Dock/menu presence until Quit. Menu-bar-only launch still starts accessory. This avoids downgrading activation policy during popover/sheet/Space transitions. It does not change Go ownership, execution or presentation-close semantics.

### Reproduced shutdown hang (P1), remediated

A subsequent sequential regression run hung during `realGoInterruptedPlanRestoresOriginalIdentityWithoutReplay`. A process sample identifies `GoBridgeConnection.shutdown` blocked in Foundation `Process.waitUntilExit()` after an externally killed/reconnected owner; this is stronger evidence than an AX timeout. Removed the redundant synchronous wait and asynchronously confirms Foundation's process-exit observation after the existing bounded graceful/SIGTERM/SIGKILL sequence. No task Stop control uses process termination. Presentation dismissal still never calls shutdown. The existing blocked-stdin shutdown assertion remains under7s (5.823s observed). New killed-owner test passed0.412s; actual interrupted Go-plan recovery passed1.616s. Three focused cases passed7.853s total. Full final suite and final packaging follow in the verification report.

Final post-fix full regression: **128 Swift tests PASS /60.960s**. Settled2,000-row decode50.487ms/projection2.570ms confirms the measured fast path without reducing retained history. Final app launch/connection and two distinct native Command-V image selections observed; screenshots are indexed in the verification report. Physical accessibility and the incomplete broader desktop matrix remain partial.

P2 residual: duplicate user-selected document basenames can still be rejected by Go. Drafts/files are preserved with the exact error; no user original is modified. A reviewed native naming/disambiguation flow is optional follow-up, distinct from the fixed generated screenshot-name bug.

## Final acceptance closure supplement — 2026-11-17

The original audit above is preserved. The final assessment is in [TRANCHE08-ACCEPTANCE-CLOSURE.md](TRANCHE08-ACCEPTANCE-CLOSURE.md); exact image provenance/exclusions are in the [closure screenshot index](Evidence/Tranche08-Closure/continuation/SCREENSHOT-INDEX.md). Current host: M3/macOS26.5.2, distinct from the earlier Intel/macOS15 sessions.

Additional actual screenshots establish full/compact partial token streaming through the existing local Responses fixture, exact action top/bottom, background work and completion beside an unsent foreground draft, full/compact dark recovery, and minimum-size recovery scrolling. Actual Tab→Decline, Return without approval, then explicit Space decline closes the native exact-decision gap. Window close/reopen, compact Escape/expand, resizing, task/back, Cmd-L and the pending-review Quit warning were observed. Physical Spaces/multi-display focus, IME, VoiceOver and preference variants remain unverified.

Review against the same Apple HIG/Agentrooms/Clarc references found no new reproducible P0/P1 or identified visual defect requiring redesign. Typography, native toolbar grouping, semantic light/dark colors, SF Symbols, wrapped selectable review/error content and bounded composer/attachment layout remain coherent. A blank unsettled compact recovery frame did not reproduce after reopening and settling; the final compact screenshot shows actual recovery. Some old files labelled recovery/streaming did not show those states and were excluded rather than accepted by name.

At the user's explicit follow-up request, replaced menu-bar and app artwork with an original black-and-white alligator head. A36px template PNG at18pt and ICNS use the same deterministic drawing; actual menu-bar/bundle captures verify it. No decorative material or control redesign was added. Backend/coordinator/security authority was unchanged.

Final current Swift result:130 PASS. Duplicate user-selected basenames remain a safe P2 rejection in shared Go provider/original-retention name validation, verified by the existing new closure regression; distinct native opaque IDs/immutable bytes are preserved. Broader safe support requires backend identity work, so rejection was retained within the user's allowed fallback.

Final reassessment: C/F/L/P/Q/T close to PASS for their stated local acceptance scope; B/G/R/S remain PARTIAL only for the precise operator/profiling requirements in the closure matrix. Functional migration implementation is complete within the existing scope; unconditional physical accessibility and remote-provider acceptance are not. Both historical Gemini and the one newly authorized `.env` OpenAI request returned401. Signing/notarization remains DEFERRED. No Tranche09 was started.
