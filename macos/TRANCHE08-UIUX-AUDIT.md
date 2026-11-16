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

Presentation implementation, targeted native checks, benchmark deltas and remaining accessibility boundaries will be appended here after the fixes. Before evidence above is preserved. No P0 is known; investigation items are not passed until actual observation supports them. Full acceptance is blocked by any reproducible unresolved P0/P1.
