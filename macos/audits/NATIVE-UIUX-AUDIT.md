# Gator native macOS UI/UX audit

Date: 2026-11-17. Review only; no remediation implemented. Scope: the current local eight-tranche application, including menu bar, notch, full window, managed conversations, composer, tasks, approvals, plans and recovery.

## 1. Executive assessment

**Overall judgment: 3/5 — a credible native Mac application with a strong functional foundation, but noticeably uneven navigation, information hierarchy and edge-case feedback.** It is suitable for the already accepted local-development workflow. The available evidence does not support calling it an exceptionally polished macOS experience or accessibility-accepted.

The strongest decisions are native text input, real system window chrome and menus, restrained branding, selectable review content, explicit owner-bound approval controls, and a small notch glance that navigates rather than authorizes. The application does not generally look like an embedded website. Its principal design weakness is that compact and full presentations share almost the same information architecture: additional width expands content without providing equally useful navigation or inspection context.

The five highest-priority findings are low-contrast warning text (UIUX-01), full-window navigation/context loss (UIUX-02), flattened nested lists (UIUX-03), reading-position reset on conversation changes (UIUX-04, source-backed inference requiring desktop reproduction), and disabled Send feedback for slash-prefixed drafts (UIUX-05). Recovery copy density and the existing duplicate-document-name limitation also merit attention.

No new P0/P1 was reproduced by the checks performed. This is not a claim that untested physical accessibility or window-system behavior cannot contain one. There are seven P2 findings, including one explicitly inferred interaction, and two P3 findings. Design judgments are distinguished from functional failures below.

**Desktop limitation:** the operator was using the Mac and explicitly requested headless work, then authorized use of existing evidence with fresh checks marked untested. No fresh desktop automation, app activation, screenshot capture, clipboard operation or window manipulation was performed in this audit. Existing screenshots were actually opened and reviewed; they are not represented as fresh captures. This constraint limits the independence of the interaction assessment.

## 2. Baseline and evidence method

Starting HEAD: `13134d5f2c81ae6f4ed8c4bea1948774d15307e6`. The initial working tree already contained report edits and local-development evidence; these were preserved. HEAD later advanced externally to `137fb7ea923d14149badbb02e102ffdfb6e34bed`. This agent created no commits. Source/resource/report hashes and initial status are recorded in [baseline.json](evidence/uiux/headless/baseline.json); final preservation is recorded in `evidence/uiux/headless/final-integrity.json`.

Current application: **`/tmp/Gator-Native-Tranche08-Enhanced-Final.app`**. All six inspected bundle hashes match the accepted candidate, including the native executable and actual bundled Go runtime; see [candidate.json](evidence/uiux/headless/candidate.json). A running user instance was observed but not controlled or inspected. No older remote snapshot was substituted for current source.

Host: Apple M3, 16 GiB RAM, macOS 26.5.2, Swift 6.3.3, Go 1.27.3, Node 26.7.0. Supported deployment minimum: **macOS 15**, from [Package.swift](../Package.swift) and application metadata. Host screenshots show macOS 26 system styling. macOS 15/earlier supported-host behavior was not physically tested; current-host toolbar materials must not be generalized to it.

Inspected all 20 files under `Sources/GatorMac/`, relevant `GatorCore` projections/coordinator/drafts/uploads/review/plan/recovery state, package manifest, resources, development scripts, native tests and current verification reports. All four requested reports exist: [closure](../TRANCHE08-ACCEPTANCE-CLOSURE.md), [local-development acceptance](../TRANCHE08-LOCAL-DEV-ACCEPTANCE.md), [menu bar/notch](../MENUBAR-NOTCH-VERIFICATION.md), and [operator acceptance](../TRANCHE08-OPERATOR-ACCEPTANCE.md). Earlier evidence was not overwritten.

Evidence classes used throughout:

- **Fresh headless:** current-source Swift tests, actual bundled-Go integrations, two Go packages, parser reproduction and process-scoped color resolution.
- **S01–S06:** prior screenshots from the unchanged enhanced-final candidate. Current-candidate evidence, but not a fresh independent interaction run.
- **H01–H10:** historical native screenshots from earlier generations. Useful for design comparison; cannot establish the final candidate's changed views or interaction behavior.
- **[Inference]:** a consequence derived from inspected code without fresh desktop reproduction. A reproduction recipe is not a claim that it was executed.
- **NOT TESTED:** unavailable physical behavior, preference variants, or fresh desktop checks.

Screenshot paths, pixel dimensions, SHA256 and provenance are in [screenshot-provenance.json](evidence/uiux/headless/screenshot-provenance.json). No screenshots were generated from SwiftUI previews or mocked HTML.

## 3. Architecture and UI inventory

| Surface/component | Implementation and ownership | Native quality assessment |
| --- | --- | --- |
| Menu-bar chat | `GatorApp.swift:12–20`; SwiftUI `MenuBarExtra(.window)`, 440×620 points | Appropriate lightweight chat panel; expandable, system menus, template alligator icon. Management/recovery content can overwhelm its small viewport. |
| Full conversation | `GatorApp.swift:22–30`; one `Window`, default 820×740, minimum 540×480, unified toolbar | Real Mac window and deliberate single selection; lacks persistent history/navigation despite available width. |
| Notch glance | `NotchQuickAccess.swift`; AppKit nonactivating panel and SwiftUI content | Hardware-derived location, restrained black camera-area integration, explicit keyboard entry; not a second agent runtime. |
| Conversation | `ConversationView.swift`; lazy message stack, bounded 780-point text column, native selectable text | Clear You/Gator hierarchy, restrained user tint, public stream and collapsed tool detail; limited Markdown structure. |
| Composer | `ComposerView.swift` + `NativeComposer.swift`; AppKit `NSTextView` bridge | Native selection, undo, responder commands, marked-text guards, paste/drop; fixed text viewport and weak disabled-state explanation. |
| Attachments | `NativeAttachmentActions.swift`, `AttachmentSelectionView.swift`, `NativeUploadStore.swift` | Native picker; independent removal, bounded thumbnails, immutable staging and owner isolation; duplicate basenames safely rejected downstream. |
| Tasks/approvals | `TaskInspectorView.swift`, `ApprovalReviewView.swift`, `ExactReviewContent.swift`, `ExactDecisionButton.swift` | Exact owner/token/generation guard, literal selectable review; decisions outside the review scroller, deliberate Space/click activation. |
| Plans/recovery | `PlanReviewContent.swift`, `RecoveryInspectorContent.swift`, `RecoveryPreparationView.swift` | Accurate proposed/retained/uncertain distinctions and no replay; technical language and stacked cards impede scanning. |
| State/lifecycle | Existing `ManagedSessionCoordinator` and authoritative Go owners; native per-owner draft archive | Presentation shares selection without creating another coordinator. Window close and guarded Quit have deliberately different semantics. |

Normal launch suppresses the full window unless `--open-window` is provided. Full presentation changes the app from accessory to regular activation and retains Dock/menu presence until Quit (`NativeWindowConfiguration.swift:38–54`). There is one full window, not independent conversation windows. This is a valid scope choice, not automatically a defect. Fresh Dock-click reopening, full screen, Spaces behavior and repeated notch-command focus restoration remain untested.

## 4. Apple-native design assessment and references

System typography and native controls are appropriate. Apple's macOS typography guidance lists a 13-point default and 10-point minimum; iPhone type or touch-target rules were not imposed on this desktop application. Body, caption and heading styles largely establish readable hierarchy; monospaced code and literal review text serve their different purposes. The transcript's bounded width and 16-point row spacing are coherent. Fixed 56/76-point composer text height and repeated nested paddings are less adaptable. Not every numeric layout value is arbitrary or a defect. [Apple Typography](https://developer.apple.com/design/human-interface-guidelines/typography)

Real titlebars, traffic lights, standard toolbar placement and resize affordances work in Gator's favor. Full-window space should help maintain orientation and compare context; the present replacement inspector loses that opportunity. A sidebar is a recommendation for this multi-conversation workflow, not an HIG requirement that every app must have one. [Apple Windows](https://developer.apple.com/design/human-interface-guidelines/windows), [Apple Sidebars](https://developer.apple.com/design/human-interface-guidelines/sidebars), [Apple Toolbars](https://developer.apple.com/design/human-interface-guidelines/toolbars)

The monochrome alligator template is a recognizable identity without recoloring the menu bar. A chat popover is a reasonable complex-interaction exception to a simple menu. The notch supplements menu-bar/full-window entry and does not contain authorization shortcuts. It should remain a glance and route, rather than accumulating management controls. [Apple The menu bar](https://developer.apple.com/design/human-interface-guidelines/the-menu-bar)

Semantic system foregrounds/backgrounds, SF Symbols, separators and restrained green accents generally look native. The rendered toolbar material on this host is supplied by the system; no broad custom glass stack was found. Rounded composer/code/recovery containers are not individually web-like, but nesting large cards and many equal-weight paragraphs produces a dashboard-like recovery page. Additional glass would not solve that hierarchy problem. Warning orange is the specific color exception established by the headless probe. Apple's small-text contrast guidance supports a 4.5:1 target. [Apple Accessibility](https://developer.apple.com/design/human-interface-guidelines/accessibility)

Reference review used current public repositories and their published screenshots/source, not installed reference applications:

- [Agentrooms / claude-code-by-agents](https://github.com/baryhuang/claude-code-by-agents): the current README describes a native Swift rewrite. Its published screenshot demonstrates persistent room navigation and a distinct main conversation. This is relevant to orientation, not a requirement to copy its agents, terminology or visible thinking UI. No private reasoning presentation is recommended.
- [Clarc](https://github.com/ttnear/Clarc): the current README describes native macOS 15+ project conversations; the inspected repository screenshot itself says **1.0.3**, while the README describes **1.3.3**. Its useful reference is separation of navigation, conversation and secondary work. Terminal panes, permission bypass and project features are not proposed for Gator.
- [boring.notch](https://github.com/TheBoredTeam/boring.notch): hover-driven glance and explicit activation are relevant; media controls and feature breadth are not. Its source/reference is not physical evidence that Gator behaves identically, and no code was imported during this audit.

## 5. Scores by dimension

Scale: 1 seriously broken/inconsistent; 2 significant issues; 3 usable but noticeably unpolished; 4 convincing native quality with minor gaps; 5 exceptionally coherent and polished. These are design judgments, not automated acceptance statuses. No dimension receives 5 merely for using SwiftUI.

| Dimension | Score | Concrete basis and qualification |
| --- | --- | --- |
| 1. Visual design | **3** | System typography, restrained green identity, SF Symbols and light/dark backgrounds are coherent. Warning contrast and recovery density hold it below 4; some theme evidence is historical. |
| 2. Menu bar | **3** | Useful immediate chat with composer, attachment controls and expansion. Exact review is safely usable, but consumes nearly the whole compact panel; history/model/effort management is fragmented across More and status row. Fresh dismissal/hover inspection not repeated. |
| 3. Notch quick access | **4, provisional** | Real-gap detection, small glance, attention text/icon, owner routing and no inline authorization. Prior same-candidate focus/approval evidence is strong. Repeated-command focus, physical multi-display and preference variants remain untested; disabled-command inconsistency UIUX-09. |
| 4. Full window | **3** | Native titlebar/toolbar, bounded transcript and sensible minimum size. Same vertical replacement layout as compact; navigation stays hidden and inspection removes conversation context (UIUX-02). |
| 5. Conversation | **3** | Selectable prose, readable code, public streaming and collapsed tool activity. Nested lists flatten, heading levels share styling, native old-history access is limited, and context changes reset the reading position. |
| 6. Composer | **4, provisional** | Native editing/undo/IME API guards, familiar attach/paste/drop, explicit Send and per-owner continuity. Slash-prefix disabled feedback and fixed multiline viewport need improvement; physical IME is NOT TESTED. |
| 7. Task and approval UX | **3** | Excellent authority distinctions and exact controls; prior owner/token/decline journey plus fresh regression evidence. Dense technical recovery language and replacement inspection make the state harder to understand than necessary. |
| 8. Accessibility | **3, provisional** | Labels, header traits, keyboard-safe decisions, selectable text and reduced-motion branches exist. Warning contrast is deficient in resolved light colors. Spoken behavior, physical IME and preference variants have no human sign-off. This score is not accessibility acceptance. |
| 9. Performance/responsiveness | **4 for measured state work; rendering unscored** | Fresh projection/stream/owner/thumbnail measurements are bounded and good. No fresh native frame latency, hover stutter, app-idle stability or long-session memory measurement. |
| 10. User journeys | **4 for documented functional paths; fresh desktop NOT TESTED** | Current bundled-Go integrations and prior same-candidate pending-review journey preserve owner/draft/authority semantics. Native presentation journeys were not independently repeated while the desktop was occupied. |

## 6. Screenshot and screen/interaction matrix

S-images are from the current unchanged bundle's previous verification. H-images predate that bundle. Both were visually reviewed during this audit. The sizes below are logical scene sizes where documented, not the outer PNG dimensions; screen crops may include margins and retina scaling. Unrecorded sizes are identified rather than guessed.

| ID / state | Surface / appearance / size | Fixture and expected behavior | Actual evidence and result |
| --- | --- | --- | --- |
| H01 empty | Compact / light / 440×620 | Empty conversation; clear purpose and visible composer | Welcome, icon, neutral space, composer and attach controls readable. Historical visual support; fresh current-candidate check NOT TESTED. |
| H02 empty | Full / light / default 820×740 | Empty full scene uses space coherently | Clear welcome and anchored composer, substantial intentional empty space. Historical pre-icon generation only. |
| H03 normal | Full / light / default 820×740 | Controlled reply with heading, list, code and long URL | Prose/code/URL wrap legibly; model/status separated from draft. Historical generation only. |
| H04 normal | Full / dark / default 820×740 | Controlled retained reply | Same hierarchy, restrained green user bubble, code surface and selectable prose. Historical closure candidate, not enhanced-final. |
| H05 long/resized | Full / dark / narrow; exact recorded size unavailable here | Long controlled conversation; text wraps and composer remains reachable | Readable narrow wrapping and visible composer. Does not establish current-binary resizing, full screen or reading-position restoration. |
| H06 attachments | Full / dark / default 820×740 | Two controlled pasted images, independently removable | Separate thumbnails/names/remove controls; selected-not-sent label. Historical candidate. Long filenames and large-text variants NOT TESTED. |
| H07 plan | Full / light / default 820×740 | Proposed itinerary and ordered steps; distinguish proposal from completion | Clear steps and phase; numerous subsequent metadata sections. Historical screenshot supports hierarchy assessment, not fresh plan authorization. |
| H08 streaming | Compact / dark / 440×620 | Paused local token fixture, partial text before final answer | Partial `Native integration` and working status visible with composer. Genuine older partial-stream evidence, not the completed image renamed as streaming. |
| H09 background work | Full / light / default 820×740 | Held original task with a separate foreground draft | Activity list shows working original chat and retained foreground draft. Historical pre-icon image. |
| H10 working glance | Notch / black / nominal 420×246 | Controlled running task | Working row and neutral glance visible; intermediate enhanced binary before focus correction. Excluded from final focus claims. |
| S01 pending attention | Notch / black / 420×246 | Inert `web_access` request; show Needs review | Explicit attention icon/text and owning row. Prior same-candidate native PASS; not independently repeated here. |
| S02 exact review | Compact / dark / 440×620 | Same inert request; read full content, explicit decision | Destination, scope, scrollable detail and persistent Decline/Approve visible. Material content extends below fold without removing decisions. Prior same-candidate PASS. |
| S03 expanded review | Full / dark / default 820×740 | Same owner, interaction/token and review | Full exact content, Back, Correction and explicit decisions. Prior observer verifies identity. Image precedes accepted keyboard traversal and does not prove Approve focus. |
| S04 recovery | Full / dark / default 820×740 | Retained stopped/completed records, no replay | Distinct statuses and expandable evidence, but long owner/cache prose and repeated cards. Same-candidate visual evidence for UIUX-06. |
| S05 recovery | Compact / dark / 440×620 | Same retained records | Viewport mostly occupied by overview and warnings; records begin near the bottom, scrollable. Same-candidate evidence, not clipping proof. |
| S06 idle glance | Notch / black / 420×246 | Ready/idle chat | Clean camera-area layout, native focus ring, Open Chat; no approval shortcut. Same-candidate evidence. |
| No new image: parser | Headless / appearance-independent / N/A | Three nested bullet levels | All projected to one level; FAIL, UIUX-03. |
| No new image: colors | Headless / Aqua, Dark Aqua and named high-contrast appearances / N/A | Resolve warning/background colors without OS preference edits | Light 2.31:1; dark 7.47:1. Color-pair check only, not physical Increase Contrast acceptance. |
| No new image: return to reading | Compact/full / both appearances / N/A | Scroll upward, switch chat and return | Source resets to latest; [Inference] UIUX-04. Fresh desktop reproduction NOT TESTED. |

Direct screenshot index:

- S01 [notch pending approval](../Evidence/Local-Dev-Acceptance/notch-pending-approval.png); S02 [compact exact review](../Evidence/Local-Dev-Acceptance/compact-exact-review.png); S03 [full exact review](../Evidence/Local-Dev-Acceptance/full-exact-review-window.png).
- S04 [full recovery](../Evidence/Notch-Quick-Access/final-full-recovery-screen.png); S05 [compact recovery](../Evidence/Notch-Quick-Access/final-compact-recovery-screen.png); S06 [expanded notch](../Evidence/Notch-Quick-Access/final-expanded-screen-region.png).
- H01 [compact empty light](../Evidence/Tranche08/final/compact-idle-light.png); H02 [full empty light](../Evidence/Tranche08-Closure/continuation/screenshots/full-empty-light.png); H03 [full conversation light](../Evidence/Tranche08/final/full-conversation-light.png); H04 [full conversation dark](../Evidence/Tranche08-Closure/continuation/screenshots/final-conversation-complete-dark.png).
- H05 [narrow history](../Evidence/Tranche08/final/full-history-narrow-dark.png); H06 [attachments dark](../Evidence/Tranche08/final/final-full-attachments-dark.png); H07 [plan light](../Evidence/Tranche08/final/plan-review-light.png); H08 [actual compact token stream](../Evidence/Tranche08-Closure/continuation/screenshots/final-token-stream-compact-dark.png).
- H09 [background work and draft](../Evidence/Tranche08-Closure/continuation/screenshots/full-background-work-light.png); H10 [intermediate notch working](../Evidence/Notch-Quick-Access/final-working.png).

Window-only notch captures documented as missing compositor layers, preview transition frames and known unsettled recovery frames were excluded from visual completeness judgments. They were not reported as application defects.

### Complete journey coverage

| Journey | Fresh headless evidence | Existing native evidence / exact limitation |
| --- | --- | --- |
| 1. Open/send normal message | Actual bundled-Go submit/receipt integrations PASS; 40 disposable fixture turns also seeded successfully | H03/H04/H08 show older normal/stream presentation. Fresh launch/send NOT TESTED. |
| 2. Converse while task runs | Managed conversation/independent-work tests PASS | H09 and prior managed native checks; not freshly repeated. |
| 3. Switch and return | Coordinator, isolation and per-owner draft tests PASS | Prior native switching passes; reading-anchor preservation is not established and has UIUX-04. |
| 4. Attach document/image | Immutable stage/upload, paste projection, removal and duplicate-name tests PASS | Prior native picker/paste/drop journeys, H06. Fresh actual clipboard/picker/drop NOT TESTED. |
| 5. Notch/menu → original task | Owner navigation and real-Go inert approval regression PASS | S01 plus same-candidate native checkpoints; same owner maintained. |
| 6. Inspect/decline exact action | Fresh real-Go inert-owner test PASS | Prior actual Tab → Approve → Return sends no decision; Space on Decline sends exactly one original-token decline. S02/S03; not freshly repeated. |
| 7. Review plan | Revision/authorization/scope guards and integrations PASS | H07 and prior plan review/Approve & Start journey; no new plan was authorized here. |
| 8. Inspect recovery | Recovery/preparation/access/read-only receipt/no-replay tests PASS | S04/S05 current-candidate visual support; no new external lookup or retry. |
| 9. Reopen without losing draft | Atomic archive/restart/reconciliation tests PASS | Earlier actual restart evidence; fresh process/window reopening NOT TESTED. |
| 10. Close presentations while work continues | Owner/approval lifecycle and unrelated-work tests PASS | Prior same-candidate close/reopen preserves draft, token and held unrelated task. No new GUI close performed. |

The controlled fixture used for audit preparation is under `evidence/uiux/provider.mjs` and `prepare.py`. It only serves loopback synthetic output; the disposable profile seeded 40 work turns. It did **not** become a fresh GUI journey. Fixture approval/plan/recovery branches prepared for desktop use were not exercised through the application in this audit. No paid model call or external action was made.

## 7. Findings

### UIUX-01 — P2: warning text has inadequate resolved light-mode contrast

- **Surface/source:** compact/full composer correction and attachment errors, `Sources/GatorMac/ComposerView.swift:19,24`; stale inspector/plan warnings, `TaskInspectorView.swift:24,89`.
- **Reproduction:** resolve `NSColor(Color.orange)` against the corresponding light system text/control backgrounds using `contrast-probe.swift`; for visual confirmation later, trigger a rejected attachment or stale review in a disposable light-mode profile.
- **Expected / actual:** small warning text should remain readily legible. The fresh process-scoped color probe measures **2.3096:1** against its resolved white backgrounds, below 4.5:1; dark resolves to **7.4673:1**. Named high-contrast appearances alone return the same pairs. This is a color-pair measurement, not a screenshot-pixel or actual preference-variant result.
- **Evidence:** [probe source](evidence/uiux/headless/contrast-probe.swift), [resolved colors/results](evidence/uiux/headless/contrast-colors.json), direct source. Exact in-window compositing and physical Increase Contrast remain unverified; [Inference] the light warning treatment is insufficient on the near-white native surfaces shown in historical light screenshots.
- **Recommended correction:** keep warning text in a tested semantic text treatment; use an icon and accessible colored accent to express severity. Verify actual surfaces in both themes before choosing a replacement. Do not recolor all system text or hide safety messages.
- **Confidence / human testing:** high for measured colors and affected styles; medium-high for final pixel contrast. Human light/dark/Increase Contrast verification required after any correction. No production change made.

### UIUX-02 — P2: full-window navigation and inspection underuse desktop space

- **Surface/source:** full window, `ConversationView.swift:23–49,84–85,114–121,145–178`; `TaskInspectorView.swift:10–16`.
- **Reproduction:** open several conversations, choose an older conversation from More → Recent Conversations, then Inspect Current Task. Compare the same operations in compact and full layouts.
- **Expected / actual:** the full window should make conversation location and task context easier to retain. History remains a submenu, only cached activity appears in a collapsed disclosure, and task inspection replaces the transcript and composer. The `sidebar.right` symbol suggests a side inspector but opens replacement content. The large full-window recovery capture remains one stacked column.
- **Evidence:** S03/S04/S05, H04/H09, exact shared view branches. This is an information-architecture/design defect; the existing navigation commands function and do not create approval authority.
- **Recommended correction:** adapt existing conversation navigation and inspection to the available full-window width, with a clear selected conversation and contextual detail placement; retain compact replacement behavior at small widths. Align the inspector icon with actual behavior if layout remains replacement-based. Reuse current owner selection and review guards.
- **Confidence / human testing:** high for structure and screenshot observations; human comparative workflow review required to validate any revised arrangement. Do not add a separate session coordinator, runtime, or mandatory multiwindow feature.

### UIUX-03 — P2: nested Markdown lists lose hierarchy

- **Surface/source:** conversation and public stream, `Sources/GatorCore/NativeMessageContent.swift:17,29–31`; `Sources/GatorMac/NativeMessageBody.swift:19–20`.
- **Reproduction:** project `- Parent\n  - Child\n    - Grandchild` with the current parser; the audit probe records every block's kind and text.
- **Expected / actual:** subordinate bullets retain a distinguishable relationship. Whitespace is trimmed before list conversion, all blocks have the same list kind and no depth, and the renderer places them at the same left edge. Ordered sublists also lose their indentation. The fresh probe reproduces this without a GUI.
- **Evidence:** [markdown-probe.swift](evidence/uiux/headless/markdown-probe.swift), [markdown-projection.json](evidence/uiux/headless/markdown-projection.json), source. No screenshot is claimed for the reproduction. Tables and blockquotes additionally remain literal text; those are rendering limitations, not separately counted defects here.
- **Recommended correction:** preserve and render list depth in the bounded native presentation layer, including incomplete streaming blocks. Keep exact approvals/structured fields literal and outside Markdown interpretation. Verify mixed nested bullets/numbering; do not replace the native conversation with a web renderer for this fix.
- **Confidence / human testing:** high; no human interaction needed to establish data loss. Native wrapping/selection checks would be needed after remediation.

### UIUX-04 — P2 [Inference]: switching conversations resets reading position

- **Surface/source:** compact/full transcript, `ConversationView.swift:15–17,216–224`.
- **Reproduction recipe, not freshly executed:** scroll away from the bottom of a long conversation, switch to another conversation, then return.
- **Expected / actual:** returning should restore the previous reading position unless the user explicitly jumps to latest. The context-change handler unconditionally sets `following = true` and scrolls to `latest`; no per-conversation scroll anchor is stored in this view. [Inference] the user loses their place on return even while their draft survives.
- **Evidence:** exact handler and absence of an anchor archive in the inspected view/coordinator. H05 only establishes long-content wrapping; it does not prove this interaction. Existing draft-isolation tests do not establish reading-position preservation.
- **Recommended correction:** retain a visible message anchor and follow-latest intent per conversation/presentation, restoring intentionally on selection. Keep Jump to Latest explicit and verify incoming background updates do not move a reader who scrolled up.
- **Confidence / human testing:** high for unconditional reset; medium for observed window behavior until reproduced. Targeted native reproduction is required before implementation. This is not a reported loss of durable messages or drafts.

### UIUX-05 — P2: slash-prefixed drafts disable Send with only tooltip feedback

- **Surface/source:** compact/full composer, `GatorApplicationState.swift:103–120`, especially `116`; `ComposerView.swift:56–61`.
- **Reproduction recipe:** in an otherwise configured idle chat, type `/Users/example/notes.txt — summarize this path`. Also try an unsupported slash command. Source deterministically returns a disabled reason for either leading slash.
- **Expected / actual:** the input should explain why it cannot be submitted and how to proceed. All trimmed slash-prefixed text is blocked; the reason is bound to Send's help tooltip, with no generic inline unavailable-reason presentation. A legitimate path-like draft is indistinguishable from a command at this boundary. Keyboard users can encounter an inert Return/Send without a visible explanation.
- **Evidence:** direct state and view bindings. No fresh disabled-button hover or VoiceOver announcement was tested; tooltip delivery is not assumed to solve keyboard feedback.
- **Recommended correction:** expose the existing actionable disabled reason near the composer when relevant. Clarify the command-vs-literal boundary through the existing authority contract before changing accepted input. Implementing new native slash commands is not proposed.
- **Confidence / human testing:** high for guard and missing inline presentation; human keyboard/assistive feedback check required. No submission bypass is recommended.

### UIUX-06 — P2: recovery hierarchy foregrounds backend terminology over the user's next decision

- **Surface/source:** task/recovery inspection, `TaskInspectorView.swift:25,31–44`; `RecoveryInspectorContent.swift:12–45,47–76`; `RecoveryPreparationView.swift`.
- **Reproduction:** inspect a task retaining stopped/uncertain evidence in compact and full views; read from the title to the first actionable record.
- **Expected / actual:** quickly identify what happened, whether anything external is uncertain, and the next safe action. S05 spends almost the initial compact viewport on public-retained-summary, owner/cached-slot and no-replay prose before the first evidence card. S04 spreads the same hierarchy across full width. Distinctions are accurate, but multiple similar text levels and repeated caveats compete with outcome and next step.
- **Evidence:** current-candidate S04/S05 and literal source strings. This is a material comprehension/scannability judgment, not evidence of replay, false success or unsafe authorization.
- **Recommended correction:** lead with a short outcome and specific safe next step; retain material external-effect uncertainty in the primary view. Move cache/owner implementation explanations and exact IDs into existing detail disclosures. Consolidate repeated caveats while keeping exact evidence, no-replay semantics and access review reachable/selectable.
- **Confidence / human testing:** high for visual density and terminology; medium for user-comprehension impact without usability participants. Validate with a task-based human readback before removing or moving safety content.

### UIUX-07 — P2, inherited: independently selected equal document basenames are rejected

- **Surface/source:** attachment submission; `NativeUploadStore.swift:168`; `internal/attachment/attachment.go:194–207`, `internal/attachment/uploads.go`, `cmd/gator/attachments.go`; `NativeClosureAcceptanceTests.swift:5`.
- **Reproduction:** select two different documents from different directories, both named `report.md`, then submit.
- **Expected / actual:** users expect distinct selected files to remain usable. Native opaque upload identities/bytes are distinct, but Go converts provider/retention names to basename and shared validation rejects duplicates. Draft, bytes and individual removal survive; no silent overwrite or provider execution occurs.
- **Evidence:** fresh passing `duplicateDocumentBasenamesPreserveDistinctBytesAndFailGoAdmissionWithoutExecution` test and prior detailed closure investigation. This remains a functional limitation, independent of the visual redesign judgment.
- **Recommended correction:** pending a coherent backend provider/retention identity contract, explain the conflict at selection/review with original names and independent removal. Do not remove shared validation, silently rename source files or weaken exact durable admission. Supporting both names requires separately authorized backend work; it was not attempted here.
- **Confidence / human testing:** high, reproduced by real-Go regression. Human picker feedback evaluation remains useful but is not required to establish rejection.

### UIUX-08 — P3: fixed composer viewport is cramped for long drafts

- **Surface/source:** compact/full composer, `ComposerView.swift:40`; native scroll editor in `NativeComposer.swift`.
- **Reproduction recipe:** paste an eight-line harmless draft; inspect it in the default full window and after resizing larger.
- **Expected / actual:** additional window space should help edit a multiline draft. Text viewport is always 56 points compact or 76 points full; the native editor scrolls internally instead of expanding. No text-loss claim is made.
- **Evidence:** explicit fixed frame; H06/H04 show a generous outer composer card with a small text-editing region. The eight-line desktop recipe was not executed in this audit.
- **Recommended correction:** consider bounded content-driven growth or an existing-layout resize affordance in the full scene, preserving a useful transcript viewport and marked-text ownership. Avoid adding controls until the long-draft interaction is verified.
- **Confidence / human testing:** high for fixed geometry, medium for editing impact. Human resize/IME/long-draft confirmation required before a change.

### UIUX-09 — P3: disabled notch preference leaves an enabled no-op command

- **Surface/source:** Conversation menu on notched hardware, `GatorApp.swift:43–44`; `NotchQuickAccess.swift:53–56,83–84`.
- **Reproduction recipe:** disable More → Notch Quick Access on a notched display, then inspect/invoke Conversation → Show Notch Quick Access.
- **Expected / actual:** command availability should match whether it can act, or explicitly re-enable the feature. Disabling hides the panel but retains detected geometry; command disablement tests geometry only, while `show()` rejects a hidden panel. [Inference] it remains enabled and does nothing.
- **Evidence:** direct inconsistent guards. Physical invocation was not performed. This is an availability-polish issue, not a broken enabled-notch primary path.
- **Recommended correction:** derive command availability from the same existing presentation-enabled state, or make its re-enable behavior explicit. Do not add another notch preference source.
- **Confidence / human testing:** high for predicates; medium for final menu-state manifestation. One targeted native preference/command check required before remediation.

## 8. Accessibility findings and operator-only conditions

Fresh automated PASS evidence covers AppKit marked-text API guards, native input semantics, dynamic green accent contrast tests, review content preservation, exact decision Return safety, owner/generation freshness, attachments and draft isolation. Existing same-candidate native evidence establishes ordinary Tab/Shift-Tab to exact decisions, explicit Space decline, notch attention and composer focus restoration. These establish their measured scope, not full accessibility acceptance.

The new warning-color finding is outside the passing green-accent tests. AX metadata, `.accessibilityLabel`, header traits and `NativeAccessibilityStatus` are helpful structure; they do not establish spoken VoiceOver grouping, stream announcement frequency or reading order. Only public state is exposed by the inspected presentation; no private chain-of-thought UI was identified.

No persistent accessibility preferences were changed. Selecting named `NSAppearance` values inside a color probe is not enabling Increase Contrast in the OS. Reduce Motion branches remove animation and replace working motion with a symbol; physical behavior is still untested. macOS larger-text/display settings should be tested as supported by the host; iOS Dynamic Type behavior is not asserted for macOS.

Use the existing [operator checklist](../TRANCHE08-OPERATOR-ACCEPTANCE.md), with the following audit-specific sign-off. Record app path, OS/display, date/operator: __________________. Use a disposable profile and inert proposal; decline it. The operator chooses/restores settings.

| Operator check | Steps / expected result | Result and notes |
| --- | --- | --- |
| VoiceOver conversation | Read compact/full messages, headings, code and composer while a local reply streams. Order is coherent; whole transcript is not repeatedly announced. | NOT TESTED — ______ |
| Approval review/decline | Read owner, scope, consequences and full content; Return does not decide; intentionally decline through VoiceOver/Space. No external effect. | NOT TESTED — ______ |
| Notch opening/dismissal | Hover while another app is focused; explicitly open with ⌘⇧G, traverse rows, Escape. Hover does not steal focus; purpose/status announced; return restores intended focus. Repeat shortcut twice before Escape. | NOT TESTED — ______ |
| Physical IME | Compose marked text; Return commits composition without Send. Repeat after compact/full and conversation changes; Shift-Return inserts a newline. | NOT TESTED — ______ |
| Tab/Shift-Tab / Full Keyboard Access | Traverse composer, attachments, task navigation, notch and both decisions under the chosen preference. Visible focus, no traps or unreachable critical action. Inspect disabled Send feedback. | NOT TESTED — ______ |
| Reduce Motion | Operator enables it, then opens/dismisses notch, streams and switches chats. Motion suppressed; state understandable through text/symbols. | NOT TESTED — ______ |
| Increase Contrast | Operator enables it in both themes; trigger an attachment warning and stale review. Critical text, focus and boundaries legible; check UIUX-01 specifically. | NOT TESTED — ______ |
| Larger text/wrapping | Use supported larger-text/display preference, resize to 540×480 and wider. Long addresses, content and effects remain readable/scrollable; decisions reachable. | NOT TESTED — ______ |
| VoiceOver attachments | Stage document and image, read names/types and remove only one. Correct file removed, draft intact, no Send. | NOT TESTED — ______ |
| VoiceOver recovery | Navigate stopped/completed/uncertain records and evidence; return to chat. Distinctions understood; no automatic retry, receipt lookup or replay. | NOT TESTED — ______ |

Physical external/flat-display fallback, rearranged displays and Spaces remain NOT TESTED. Geometry-unit-test PASS is not a substitute. Actual hardware-notch detection was previously exercised on the built-in display only.

## 9. Performance and responsiveness

Fresh measurements come from the current debug test suite, on the host listed above. See [swift-audit.log](evidence/uiux/headless/swift-audit.log) and `Tests/GatorCoreTests/NativePerformanceTests.swift` / `NativeClosureAcceptanceTests.swift`. These are state/projection/processing measurements, not SwiftUI rendering benchmarks. Timings use `Date.timeIntervalSinceReferenceDate`, not an Instruments signpost or frame clock; system load and debug compilation affect comparison. No performance thresholds were invented.

| Metric / fixture | Repetitions | Fresh result | Meaning and limitation |
| --- | --- | --- | --- |
| Transcript, 50 messages /24,084 B JSON | 7; median | Decode 0.858 ms; project 0.03695 ms | Synthetic public messages; no Markdown layout/window rendering. |
| Transcript, 500 messages /240,934 B | 7; median | Decode 6.797 ms; project 0.26691 ms | Same methodology. |
| Transcript, 2,000 messages /964,934 B | 7; median | Decode 20.641 ms; project **0.83399 ms** | Test-process peak RSS 45,187,072 B; not isolated allocation or app RSS. |
| Streaming update handling, 30 snapshots /500 retained messages /2,176,935 wire B | 5 batches; median | 202.433 ms/batch; **6.74777 ms/state** | Decode/state processing fixture. Not measured UI frame latency or 148 frames/s. |
| Managed owner time to connected | 4 sequential starts | 58.340, 65.110, 74.998, 64.804 ms; median 64.957 ms | Local fixture and warm environment; excludes app launch and real-provider network. |
| Four connected idle Go owners | One `ps` snapshot after 2 s | 36,544–36,992 KiB RSS each; displayed CPU 0.0% | Point/lifetime-average sample, not energy measurement or proof of zero idle CPU. |
| Thumbnails, 12 ×512² PNG; 11,116,140 staged B | One sequential batch | **71.228 ms**; 930,144 thumbnail B; process peak RSS 49,561,600 B | Thumbnail call includes immutable-byte validation and actor processing; excludes picker/staging and native image layout. Peak is process cumulative high-water mark. |
| Cold native startup, menu/notch/full opening, switching render latency | None this audit | **NOT TESTED** | Requires controlled GUI timing; historical startup values were not relabeled as current. |
| Native app idle CPU/memory, long-session memory/energy, observable layout stutter | None this audit | **NOT TESTED** | No Instruments run, 10-minute idle sample or sustained multi-task GUI workload. |

The historical 2,000-message **191.57 →2.57 ms** result concerns transcript projection. The inspected current implementation preserves the linear projection and dependency-based recomputation (`BridgeProtocol.swift:70–101`, `GatorApplicationState.swift:203–207`) and the new projection result remains bounded. The old before implementation was not restored and remeasured. Different fixture/load/build conditions preclude treating this new number as another before/after speedup. Neither measurement supports saying that all rendering is 74× faster.

Code review found lazy transcript rows, equatable message rows, bounded activity detail and ImageIO thumbnail downsampling off the main actor. Current code still processes snapshot decoding and observable state on the application path, so very large streaming histories deserve profiling if stutter is reported; this is a measurement proposal, not an invented reproduced performance defect.

## 10. Exact verification and results

Fresh commands run from the repository root:

```sh
GATOR_TEST_BINARY=/tmp/Gator-Native-Tranche08-Enhanced-Final.app/Contents/Resources/gator-runtime \
  swift test --package-path macos --scratch-path /tmp/gator-native-build --no-parallel
# PASS: 133 Swift Testing tests /37.005 s. XCTest's separate zero-test banner
# is not the Swift Testing count. Includes actual bundled-Go local integrations.

go test ./internal/workapp ./internal/attachment -count=1
# PASS: 2 packages; 5.625 s and 0.323 s.

swift macos/audits/evidence/uiux/headless/markdown-probe.swift
# Reproduces flattened nested list structure; output archived, not an app test change.

swift macos/audits/evidence/uiux/headless/contrast-probe.swift
# Resolves process-local color pairs; output archived, no OS setting mutation.

python3 macos/audits/evidence/uiux/prepare.py
# Disposable bundled-Go/loopback profile: 40 seeded work turns.
# Provider stopped during cleanup; no native GUI run and no paid calls.

git diff --check -- macos/audits
# PASS at final report verification.
```

The parser probe carries the inspected parser in an audit-only standalone file; its body was compared with current source. It does not introduce an alternative production renderer. The color probe resolves APIs rather than captured pixels. Probe outputs document defects/limits; their successful execution does not mean the UI passed those criteria.

No failed or skipped Swift/Go test occurred in this audit. The previous acceptance's host 16 PASS, recovery 9 PASS, selected TUI 2 PASS/1 platform skip and 15 Go packages were **not rerun here**: this task changed no production application or shared protocol, and fresh native/core tests were the relevant bounded verification. Those earlier counts remain historical. No build/package or full unrelated suite was repeated. Fresh desktop, VoiceOver, IME, preference variants and physical multi-display checks were omitted for the reasons stated, not counted as PASS.

No real-provider request was made. Prior Chat/Pi/OpenCode inference evidence remains in existing reports. Responses' recorded HTTP 401 was diagnosed there as provider project-geography authorization, with a regional endpoint candidate unverified live; it is not a macOS visual failure and was not retested, bypassed or reconfigured for this audit.

## 11. Recommended direction and remediation proposals

Keep the existing native foundation and ownership model. Make each surface earn its space: compact for immediate conversation, notch for passive attention and intentional routing, full window for sustained reading, visible conversation location and contextual inspection. Retain the alligator mark, restrained accent, system typography, native menus and platform-provided window treatment. No wholesale restyling or decorative glass work is justified by this evidence.

Recommended order: first verify/correct warning contrast and disabled-state explanation; preserve nested list hierarchy; reproduce and address reading-anchor loss; then evaluate full-window navigation/context and simplify recovery hierarchy with a human readback. Small composer and command-availability polish can follow. Keep authoritative Go admission, exact review content, original owner/token binding, draft isolation and no-replay behavior as review constraints throughout. Duplicate-basename support requires a coherent backend identity decision; do not solve it with a frontend validation bypass.

Acceptance for proposed corrections should be narrow and evidence-based: headless parser/state regressions first; a short controlled compact/full keyboard check for input/focus/scroll changes; actual light/dark warning screenshots for contrast; and operator sign-off for spoken/physical accessibility. These are remediation proposals only. No code or tests were altered to implement them.

## 12. Limitations and final judgment

This is a complete source/evidence review across the requested dimensions, with fresh bounded headless verification and an explicitly incomplete fresh desktop matrix. The operator chose to keep using the Mac. No direct observation of the running user's private conversation was necessary or performed. Current-candidate review/recovery/notch screenshots are strong evidence for those views; empty/normal/long/stream/attachment/plan/theme/resizing examples remain historical generations and their confidence is correspondingly lower.

Unverified conditions include VoiceOver speech/grouping/announcement frequency, physical IME, Full Keyboard Access variants, actual Increase Contrast/Reduce Motion/larger-text settings, physical flat/multiple displays, Spaces/full screen, Dock reopening, repeated notch shortcut focus, complete hover/focus appearance, long filename layout, sustained energy/memory and real-time frame behavior. UIUX-04 and the UI manifestation of UIUX-09 require targeted native reproduction. No claim of independently retesting all ten desktop journeys is made.

All audit-created files are under `macos/audits/`. Earlier evidence, bundles, README files, credentials, provider defaults and production application sources are preserved. There was no signing/distribution work, commit, remediation, new feature tranche or backend-goal resumption. The existing application path remains unchanged.

**Final macOS-native quality verdict: recognizably native and functionally credible, with a coherent restrained identity; not yet consistently polished for prolonged multi-conversation work.** Existing local-development acceptance need not be overturned by P2/P3 design findings, but exceptional native-quality and physical accessibility claims should remain open until the specified evidence and corrections exist.

## 13. Prioritized remediation backlog

Effort is relative: S = bounded local presentation/state change; M = coordinated view/state work and interaction validation; L = shared contract work. It is an estimate, not a commitment or authorization to implement. Priority is execution order, separate from severity.

| ID | Severity | Problem | Evidence | Recommended Fix | Effort | Priority |
| --- | --- | --- | --- | --- | --- | --- |
| UIUX-01 | P2 | Low-contrast orange warning text in light colors | Resolved 2.31:1; Composer 19/24, Inspector 24/89 | Tested semantic text treatment plus severity icon; actual theme/preference checks | S | 1 |
| UIUX-05 | P2 | Slash-prefixed draft disables Send with tooltip-only reason | State 116; Composer 60 | Show actionable existing reason inline; preserve command authority | S | 2 |
| UIUX-03 | P2 | Nested Markdown list hierarchy flattened | Fresh parser reproduction | Preserve depth in native projection/rendering | S–M | 3 |
| UIUX-04 | P2 [Inference] | Reading position reset on chat switch | Conversation 224; native repro pending | Reproduce, retain per-chat anchor/follow intent | M | 4 |
| UIUX-02 | P2 | Full window hides navigation and replaces conversation for inspection | S03–S05; shared view branches | Adapt existing navigation/detail placement to width | M | 5 |
| UIUX-06 | P2 | Recovery overview is dense and technical | S04/S05; recovery strings | Outcome/next-step hierarchy; disclose cache/ID detail | M | 6 |
| UIUX-07 | P2, inherited | Distinct documents with same basename rejected | Fresh real-Go regression | Explain conflict now; coherent provider/retention identity before support | S feedback /L contract | 7 |
| UIUX-09 | P3 [Inference] | Show Notch command enabled when preference hides panel | GatorApp 44; Notch 56/84 | Reproduce; align availability with enabled state | S | 8 |
| UIUX-08 | P3 | Long draft has fixed small editing viewport | Composer 40; historical composer images | Validate long-draft need, bounded adaptive editing height | S–M | 9 |
