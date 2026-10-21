# Desktop screen audit: compact visual direction

21 October 2026. Source baseline: `a14182fb9`. **Full review** using the
`make-interfaces-feel-better` skill. This is an audit of the existing application;
the findings below are open work, not implemented fixes.

## Answer

**No: the visual direction is established, but adoption is uneven.** The home,
conversation, queue, recent-work navigation, compact inspector, output preview,
and daily-usage page provide the strongest examples. Several other destinations
share their colors and typography but still expose terminal-oriented controls,
long diagnostic blocks, and generic confirmations.

One issue goes beyond polish: a populated Inbox renders rows as buttons with
navigation chevrons, but activating them does not open a result. That prevents
an app-wide approval of the current presentation.

## Scope and evidence

React 19, Tailwind v4 tokens, semantic CSS, Lucide icons, and the existing
Go-projected panel/result contract are the implementation conventions. No new
styling framework, font, or animation dependency is proposed. The reference is
the compact direction already accepted in [Work details](DESKTOP-WORK-DETAILS.md),
with the earlier comparison recorded in [the presentation audit](DESKTOP-CODEX-AUDIT.md).
This pass does not claim a fresh measurement of a current Claude or Codex build.

The packaged macOS app was inspected at 1440×960 and 800×600. Existing tests
also exercise intermediate desktop widths. Two isolated, one-off audit walks
captured **108 states**, including repeated navigation checks; this is not a
count of 108 distinct screens. The ten existing packaged end-to-end paths
passed and produced another 35 screenshots. Source, DOM state, computed styles,
and representative screenshots were inspected across the screen families below.

The audit walks used temporary configuration, state, Electron user data, and
workspaces. Notes and goals were created only in those temporary profiles.
Conversation metadata, an Inbox entry, and a disabled job were local fixtures
loaded through the real Go stores. The queue/conversation tests use local fake
provider servers. Rich output and approval examples use display fixtures, with
their existing separate backend verification recorded in the earlier audit.
No real account authorization, paid inference, background-job execution,
service installation, restore, export, or deletion was performed by the walks.

### Review categories

| Category | Evidence inspected | Result |
| --- | --- | --- |
| Typography | Global font/type rules; home, transcript, lists, native forms, long reviews, narrow views, output labels, contrast-theme computed text colors | F7, F8, F10, F11, F12 |
| Surfaces | All primary destinations; nested settings; empty and populated examples; dialogs, search focus, action bars, disclosures, 40px control measurements | F1–F11 |
| Animations | Existing 10% playback check for sidebar/menu/details interruption; routine navigation; dashboard interaction pause; reduced-motion audit state | Clear in exercised paths; not a frame-time benchmark |
| Icons | Lucide/currentColor/stroke rules; navigation, action, disclosure, queue, approval, and output icons | F1 and F3: navigation/toggle affordances do not always describe the action |
| Performance | CSS transition specificity; absence of blanket `will-change`; stable transcript behavior; 300-message search; bounded inspector previews; scrolling and page overflow | Clear in exercised paths; very large collections and restore workloads not benchmarked |

## Screen-by-screen results

**Aligned** means the inspected state follows the direction. **Partial** means
its foundation is consistent but an identified issue remains. **Needs work**
means a substantial control or hierarchy pass is needed. **Blocked** identifies
a misleading or inaccessible primary interaction. These are presentation
judgments, not a certification of every backend operation.

### Shell and conversation

| Screen or surface | States checked | Assessment | Remaining gap |
| --- | --- | --- | --- |
| Application shell and sidebar | Expanded/collapsed, repeated interruption, narrow window, selected destination, settings navigation | Aligned | No separate shell redesign identified |
| Recent work in sidebar | Real saved conversations, workspace grouping, duplicate folder names, filter/no match, show more/fewer, active conversation | Aligned | Use this concise metadata pattern elsewhere |
| Home / new conversation | First run, configured fake model, overview cards, starter carousel, interaction pause, reduced motion | Aligned | No new finding in the exercised home states |
| Conversation and composer | User/assistant layout, Markdown/code, copy feedback, running, reading older messages, jump to latest | Aligned | Transcript search has the shared focus issue, F2 |
| Transcript search | 300 messages, multiple/no/literal matches, next/previous, Escape, narrow window | Partial | F2 |
| Message queue and editor | Enqueue, edit/cancel/save, reorder, remove, paused queue, narrow window, preserved separate draft | Aligned | No additional finding from this pass |
| Pending action approval | Exact details, current versus historical action, narrow presentation | Aligned | Keep exact action details and the existing explicit approval boundary |
| Work-details sidebar and drawer | Empty/rich results, collapsed sections, sources, selections, inherited references, narrow modal focus, Escape | Partial, minor | F12; core information architecture is aligned |
| Output preview | Earlier and failed results, Markdown, inert HTML source, unavailable binary preview, metadata, back/focus/scroll restoration | Aligned | Do not infer delivery from file checks |
| Saved drafts dialog | Recovery, populated/empty, retention preference, deletion/clear, restore, close focus | Partial | F7: full source paths dominate draft metadata |
| Leave-view dialog | Keep editing, leave, focus containment, credential form isolation | Aligned | No new finding |
| Disconnected/reconnected state | Send disabled, draft editable, reconnect, fresh-session notice, retained work available | Aligned | No new finding |
| Startup and renderer-failure fallback | Source inspection of `App.tsx:210` and `main.tsx:10` | Not fully verified | Failure states were not deliberately triggered in this pass |

### Workspace destinations

| Screen or surface | States checked | Assessment | Remaining gap |
| --- | --- | --- | --- |
| Overview | Empty/populated root, four sections, empty section, model blocker detail, narrow window | Partial | F7: long operational footer and generic action/detail hierarchy |
| Ongoing goals | Empty/populated picker, search field, selecting an existing goal | Partial | F2, F10; empty state does not explain the existing creation/setup path |
| Conversations: Chats | Empty, populated, query/no results, switching view, opening saved work | Partial | F2, F7: full paths make it less compact than sidebar recents |
| Conversations: Task runs | Empty/populated, cyclic filter, keyboard selection, opening a run, narrow window | Partial | F3, F4, F7; technical status sentence remains prominent |
| Task-run detail | Real run opened by existing end-to-end test; detailed projection inspected in source | Partial | F7: Work/source/snapshot/delivery/context blocks need a stronger reading order |
| Inbox | Empty and real-store fixture, unread/urgent state, mouse activation, narrow window | **Blocked** | F1: row activation leaves the panel unchanged; no visible open/rate actions |
| Follow-up checks list | Empty, disabled-job fixture, selecting a check, narrow window | Needs work | F4, F10 |
| Follow-up setup/edit form | All 11 projected fields, native text fields, cycle controls, review, narrow window | Needs work | F3; sources, timing, permissions, and limits need grouped native controls |
| Follow-up agreement review | Outcome/sources/timing/notification/permission/limits, explicit save action | Partial | F8: improve structure while retaining the complete agreement |
| Follow-up detail | Paused check, health, timing, permissions, usage and actions | Needs work | F7, F10: raw cron, paths, shortcut prose, and equal-weight actions |
| Saved knowledge list | Empty/populated, null source references, disclosures, refresh, narrow window | Partial | F9; previous crash did not recur |
| Saved knowledge detail | Retained excerpt, scope/provenance, omit/correct/forget entry points | Partial | F7: supporting metadata competes with the saved content |
| Knowledge correction | Native textarea, current excerpt and provenance, save/cancel presentation | Partial | F8: old content is repeated beneath the editor |
| Forget-knowledge review | Exact item, effect and retained-history caveat, cancel | Partial | F8: destructive action uses generic positive styling |
| Memory setup | Basic recall readiness, unavailable Ollama, local/cloud setup reviews, narrow and contrast views | Needs work | F6, F10, F11 |
| Learnings | Empty list and native add form at two widths; detail projection in source | Needs work | F3, F4, F7, F10; populated learning lifecycle was not executed |

### Settings and secondary surfaces

| Screen or surface | States checked | Assessment | Remaining gap |
| --- | --- | --- | --- |
| Settings navigation | Four categories, nested pages, back to app | Aligned | Existing categories and shell can remain |
| Models setup chooser | Cloud/local choices and collapsed processing disclosures | Aligned | Catalog and local setup below are separate findings |
| Cloud-model catalog | 119 projected rows, selection, configure/custom-provider entry points | Needs work | F5: no search; broad administrative toolbar dominates selection |
| Cloud credential / custom provider forms | Native fields, masked credential, cancel/leave guard, no credential in display state | Aligned | Existing forms are a useful base for F3 |
| Model help | Browse/cloud/local shortcut blocks | Partial | F10: terminal-style help rather than desktop actions |
| Local-model setup | Missing-Ollama confirmation; catalog projection inspected in source | Partial | F8, F10; installed/running/download progress not exercised |
| Usage & limits | Empty/active usage, finite/uncapped budgets, breakdown, pause review, accounting disclosure, narrow window | Aligned | Daily usage stays distinct from model context occupancy |
| Usage-limit form and review | Native fields, review/cancel, existing test confirms temporary-profile policy changes | Partial | F8: repeated description and generic confirmation labels |
| Data & backups overview | Actions, storage, counts, retention, narrow window | Needs work | F7: management actions precede a long unstructured diagnostic description |
| Relink / restore / export / verify / retention forms | Opened each native form; export and retention review; narrow restore/retention | Partial | F3: filesystem and `OLD=NEW` mappings need better field presentation; F8 in reviews |
| Deletion choices / removal plan / cleanup preview | Category choices, scope/counts, review validity, no-eligible-file state, explicit cancel | Needs work | F8: duplicate destructive affordance and positive action styling |
| Recovery overview | Offline state, service configuration, paused-check entry, coverage caveats, narrow window | Needs work | F7, F10: service internals obscure health and next action |
| Start-at-login review | Exact service destination and replacement caveat, normal/narrow, cancel | Partial | F8: entire long description repeated in subtitle and body |
| Recovery credential / restore completion variants | Factories and shared renderer inspected in source | Not fully verified | No real connector credential replacement, valid restore, or completion receipt generated |
| Workspace menu | Current source, refresh, instruction controls, search | Partial | F2, F7, F10 |
| Effort chooser | Three options, current state, select action | Partial | F3: use a selection affordance rather than navigation chevrons and a redundant Set action |
| Status-line editor | 17 options, visibility/order state, toggle/reorder/default/save actions | Needs work | F3, F10: terminal configuration vocabulary and indirect controls |
| Command search | Desktop modal, query/filter, selection/dismissal, focus/exit behavior | Partial | F2; core modal and motion behavior are aligned |
| Help and command-result surfaces | Rendered help, authority, specialist-unavailable results; common Result renderer and command projections inspected | Partial | F10; notes/skills/connectors/goal-plan operations do not have dedicated native management screens |

Bare `/jobs` currently opens Follow-up checks. The older generic `jobs` panel
factory is still in source but was not counted as a separately verified native
destination. Notes, skills, connector management, plans, and advanced operations
must similarly be distinguished from pages that actually exist: the desktop
offers command/result or terminal-handoff paths for these capabilities.

## Findings

Twelve consolidated findings: **1 HIGH, 10 MEDIUM, 1 LOW**. Related occurrences
are grouped so a shared fix can cover several pages.

| Severity | Location | Before | After | Why |
| --- | --- | --- | --- | --- |
| HIGH · F1 | `desktop/src/Panel.tsx:96`; `internal/worktui/bridge_panel.go:101`; `internal/worktui/update.go:455` | Inbox rows are buttons with chevrons. A real stored entry remains on the identical panel after activation. Slash-command hints are filtered out of the toolbar. | Present a working Open result action using the existing read/show path; expose deliberate Mark read and rating actions; show a chevron only when activation navigates. Keep opening distinct from marking read. | Truthful affordances: the desktop implies an interaction that does nothing, repeatedly blocking the primary Inbox path. |
| MEDIUM · F2 | `desktop/src/styles.css:342`, `:361`, `:366`; `desktop/src/Panel.tsx:91` | Picker, transcript, and command search remove input outlines. Picker/command inputs match `:focus-visible` but compute `outline-style: none`; their containers have no replacement focus treatment. | Add a shared visible input or `:focus-within` treatment using the accent token, without changing layout. | Keyboard focus must be visible; the polished recent-chat filter already demonstrates a clearer field boundary. |
| MEDIUM · F3 | `desktop/src/Panel.tsx:97`; `internal/worktui/followup.go:388`; `internal/worktui/bridge_panel.go:38`, `:92`, `:223`; `cmd/gator/work_data.go:211` | Form enums use Previous / Change / Next; task-run filters cycle without showing choices; effort uses navigation chevrons; status-line toggles/order are indirect; restore mappings use one `OLD=NEW` string. | Use grouped, labeled select/radio/toggle/number controls, explicit filter choices, direct reorder controls, and structured mapping rows. Serialize through existing validation/review paths; expose display metadata if required. | Native control conventions make current values, available choices, and effects understandable without learning terminal keys. |
| MEDIUM · F4 | `desktop/src/Panel.tsx:10`, `:46`, `:104`; `internal/worktui/followup.go:412`; `internal/worktui/bridge_panel.go:212` | Empty follow-ups and learnings still show Details; learnings use an amber notice for ordinary emptiness; task runs repeat an empty notice and empty illustration. | Give each empty family one neutral explanation and one relevant available action. Omit selection/detail actions when there is no item. Keep unsupported actions out of the interface. | Empty states should explain the next step and accurately reflect action availability. |
| MEDIUM · F5 | `internal/modelcatalog/panel_snapshot.go:46`; `desktop/src/Panel.tsx:90` | The cloud catalog projects 119 rows with no search and a top-level Configure / New provider / Discover / Rename / Forget / Remove toolbar. | Add a compact filter over the available catalog, make model choice/setup primary, and place provider administration in a contextual menu. Preserve stable selected-row identity when filtering. | Choice overload and equal-weight administration make first-time setup harder than the Models chooser suggests. |
| MEDIUM · F6 | `internal/worktui/recall.go:132`, `:140`; `desktop/src/Panel.tsx:6`, `:61` | Memory uses numbered block titles that miss the disclosure whitelist, leaving all processing details expanded. `O/G` produces two buttons both labeled OpenAI/Gemini with a key suffix. | Use explicit section roles for readiness versus supporting processing details, collapse the latter, and render distinct OpenAI and Gemini choices tied to the existing actions. | Progressive disclosure and explicit labels should match the compact inspector and avoid duplicate-looking controls. |
| MEDIUM · F7 | `desktop/src/Panel.tsx:101`; `desktop/src/DraftManager.tsx:18`; `internal/worktui/bridge_panel.go:73`, `:163`, `:237`; `internal/worktui/history.go:301`; `internal/worktui/context.go:157`; `internal/worktui/followup.go:396`; `cmd/gator/work_data.go:200`; `cmd/gator/work_recovery.go:175`; `cmd/gator/work_overview.go:77` | Conversation and draft lists emphasize full paths; task rows contain a long technical status sentence; overview, follow-up, knowledge, learnings, data, and recovery details render equal-weight diagnostic paragraphs. | Standardize concise title/summary/status/workspace/time rows and compact detail sections. Put health, user content, and next action first; move raw IDs, paths, cron, storage and diagnostic explanations into supporting disclosures. Keep exact destinations and consequences visible in reviews. | Information hierarchy should help users scan work and act; matching colors alone does not make these pages compact. |
| MEDIUM · F8 | `internal/worktui/controls.go:244`; `desktop/src/Panel.tsx:88`, `:104`; `internal/worktui/context.go:157`; `cmd/gator/work_data.go:243`; `internal/modelcatalog/panel_snapshot.go:91` | Generic reviews repeat the description in subtitle and body and use Confirm / Edit Or Cancel. Removal also renders a destructive action as a generic row plus a positive green footer button. Correction repeats the old excerpt below its editor; local model confirmation falls outside the common confirmation kind. | Use one summary plus labeled parameters, explicit verbs such as Save limits or Export backup, a single destructive action with appropriate visual treatment, clear Back/Edit/Cancel semantics, and consistent confirmation presentation. Keep all exact scope, target, permission, retention and replacement facts accessible before confirmation. | Clear decision hierarchy reduces duplicated reading and makes consequential actions distinguishable. |
| MEDIUM · F9 | `internal/worktui/context.go:185`, `:190`; `desktop/src/Panel.tsx:91` | Saved knowledge has no search field, may show the full excerpt in each row, and hides the first-100-items boundary in Work context. | Add a visibly scoped filter for loaded items, concise previews, and an always-visible count/scope. Offer the existing full-search command path separately; do not label a loaded-item filter as a search of all knowledge. | A growing library must remain findable without overstating search coverage. |
| MEDIUM · F10 | `internal/worktui/commands.go:1487`; `desktop/src/App.tsx:183`; `internal/worktui/followup.go:368`; `internal/modelcatalog/panel_snapshot.go:13`; `desktop/src/Panel.tsx:88`, `:104` | Help advertises terminal-only editor/navigation shortcuts, while the desktop uses Cmd/Ctrl+K/F/N. “Follow-up checks” becomes “Project follow-up”; several pages say Press N or P. The Back button is always named Back to conversation even inside nested workflows. | Provide desktop-aware command/help presentation and platform shortcuts, consistent destination names, sentence-case action labels, contextual Back labels, and explanations of existing setup paths where no native creation screen exists. Retain terminal instructions in terminal-specific help. | Terminology and navigation cues must describe the interface the user is actually using. |
| MEDIUM · F11 | `desktop/src/styles.css:83`, `:337`; related hardcoded secondary text in the same stylesheet | Contrast theme raises `--color-muted` from `#979797` to `#b8b8b8`, but generic block text remains `rgb(146,146,146)` in both modes. | Route secondary text, surfaces and hover states through the existing semantic tokens, then verify normal and contrast views. | A contrast preference should apply consistently to the content-heavy pages that need it. This is a consistency finding, not a claimed WCAG contrast failure. |
| LOW · F12 | `desktop/src/WorkDetails.tsx:122`; `.work-output small` in `desktop/src/styles.css` | A long output status can wrap “Result” and its number onto separate lines in the compact rail. | Keep the result label and number together, allowing breaks between status segments. | Small typography refinement preserves scanability without widening the inspector or hiding provenance. |

### Runtime confirmations behind the highest-priority findings

- **Inbox:** a fixture loaded by the real store produced one unread/urgent row.
  Clicking it left the complete panel projection, including its identity,
  unchanged. Source confirms Inbox falls through the section handler without
  Enter behavior; its read/rate hints are slash commands excluded by the generic
  toolbar parser. This is not a fixture with a fabricated stale panel ID.
- **Focus:** the Chats picker search and command-search inputs both matched
  `:focus-visible` while their computed outline style was `none`. The containers
  provided no alternate focus outline. Transcript search has the same source rule.
- **Memory:** the actual visible headings were `1 · Storage`, `2 · Retrieval`,
  and `3 · Chat processing`; the renderer checks exact unnumbered strings.
- **Contrast:** the muted token changed, but a visible Memory block paragraph
  kept the same `rgb(146,146,146)` color.
- **Catalog:** 119 real projected rows were rendered without the picker search
  control, because this screen uses the `models` kind.

## Recommended implementation order

Each item is a separate reviewable change and atomic commit. These are proposed
next steps; this audit changes documentation only.

1. **F1: Inbox activation and explicit actions.** Start with the misleading
   primary interaction; reuse canonical read/show/rate behavior.
2. **F2, F4, F6, F11: shared presentation corrections.** Separate commits for
   focus treatment, empty-state availability, Memory sections/provider labels,
   and theme-token consistency.
3. **F3: native controls.** Split follow-up configuration, list filters/effort,
   status-line configuration, and restore mapping presentation into separate
   commits. Keep existing persisted values and validation unchanged.
4. **F5 and F9: catalog and library finding.** Separate model filtering from
   saved-knowledge filtering, with explicit scope and stable selection.
5. **F7 and F8: detail and review templates.** Establish shared primitives, then
   migrate Conversations/Task runs, follow-ups, data, recovery, and knowledge in
   distinct commits. Keep privacy and consequence facts at decision time.
6. **F10 and F12: terminology/help and output-label refinements.** Update the
   desktop help independently from terminal help and retain current output
   provenance semantics.

Most work can stay in desktop presentation. A small additive display projection
may be needed for structured option/action identities. Any capability that does
not already exist—such as a new native goal-creation or connector-account
management flow—needs its own product/backend scope rather than a decorative
button that promises it works.

## Considered but rejected

| Location | Candidate | Rejected because |
| --- | --- | --- |
| `WorkDetails.tsx` | Add a context percentage ring to make the rail look complete | Context occupancy remains unmeasured; daily token totals are not a valid substitute |
| `WorkDetails.tsx` connections section | Add enabled/authenticated switches | Selected connector IDs do not establish account access or authorize an operation |
| `Result.tsx` approval presentation | Collapse exact action details by default for compactness | The exact action must remain reviewable before the existing explicit approval |
| `motion.tsx` and shared CSS | Add more motion to make older pages feel consistent | The motion checks pass; the remaining gaps are control semantics and information hierarchy |
| Shared panel styles | Add heavier cards/shadows around every block | This would add visual weight without resolving the reading order or misleading controls |

## Verification

From `desktop/`, against the existing packaged macOS executable:

```sh
GATOR_DESKTOP_TEST_EXECUTABLE="$PWD/release/mac/Gator.app/Contents/MacOS/Gator" npm run test:e2e
node playwright-report/screen-audit/audit.mjs
node playwright-report/screen-audit/nested.mjs
```

- **10 end-to-end paths passed in 59.8 seconds.** These cover drafts/reconnect,
  long transcript search, real saved conversations, 10% motion playback and
  interruption, queue editing/reordering, real Go navigation/forms, dashboard
  behavior, usage review, current-only approval, native credentials, and compact
  work details/output previews.
- **Both one-off audit walks completed successfully:** 56 and 52 captures,
  zero renderer errors, no skipped steps, and no document-wide horizontal
  overflow in their sampled normal/narrow layouts.
- **Reduced motion:** the checked knowledge-navigation state had no active
  animations; the existing focused motion test also passed.
- **Capture refinement:** the walks were repeated with finite screenshot
  animations disabled to avoid treating intermediate navigation opacity as a
  text-contrast defect. Motion behavior was assessed separately at 10% speed.
- Each isolated audit process exited through its normal quit path with the
  test-only debugger detached, and its temporary profile/workspace was removed.
- `git diff --check`: passed for the audit documentation.

The one-off scripts, JSON records, and 108 captures are local ignored artifacts
under `desktop/playwright-report/screen-audit/`. The end-to-end captures are in
`desktop/test-results/`. They are not production code or committed fixtures.

**Not verified:** Windows/Linux runtime; signed distribution; native screen
reader behavior; OS folder-dialog interaction; real OAuth/connector account
states; installed/running local-model catalogs and downloads; executing a
background check or changing login services; valid restore/relink completion;
live export/deletion; every advanced command or terminal handoff; every learning
lifecycle state; deliberately triggered boot/render crashes; very large store
or restore performance. Source-only and unavailable states are marked in the
screen tables instead of being counted as passing runtime checks.

## Verdict

**Block app-wide presentation approval pending F1.** After that, the open
MEDIUM and LOW findings still make the broader polish verdict **Needs changes**.
The checks above support the existing conversation and compact-inspector
direction; they do not establish that every Gator screen has adopted it.
All unverified boundaries listed above remain outside this verdict.
