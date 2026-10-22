# Conversational chat presentation

Decision recorded 22 October 2026. The user selected **A / A / A** in the
three design questions: Tide, hybrid activity, and all three additional
improvements (dated chat search, contextual files, and activity polish).

Keep Gator's dark neutral surfaces, sage accent, typography, Lucide icons,
command palette, existing review flows and React/CSS stack.

## Surfaces

| Surface | Decision |
| --- | --- |
| Landing page | Ambient Tide: two crossing, noise-warped ASCII swells across the empty chat area. Mask the greeting for readability. No pointer-triggered effects or pattern picker. |
| Live response | Public parent-agent narration and tool activity in delivery order. Tool completion updates the original row; consecutive tools share a disclosure. Narration separates tool groups. |
| Completed response | Fold retained activity above the answer and show its measured duration. A successful answer without retained activity has no work summary. Keep failed/stopped cues visible. |
| Recent topics | Date buckets in the flat view. Keep optional folder grouping, active-topic preservation and recent-only filtering. Highlight literal title/folder matches; support arrows and Enter. |
| Conversations | Search titles and folders in the existing Go-supplied list of up to 50 recent saved conversations. Today, Yesterday, Previous 7 days, Older and Undated headings. Escape clears search before leaving the screen. Failed refreshes explain that loaded rows may be stale and offer a retry. |
| Files | Show a contextual Files button when the conversation has retained outputs. Open the existing rail, filter filenames, preview retained text/Markdown, and link back to the exact loaded output card. |
| Activity details | Quiet working mark and restrained shimmer while active; static approval/stopping cues, explicit failed/unfinished tool states and completion indicators. |

The command palette remains separate from chat search. Neither chat search
surface searches message bodies. Recent filtering checks only its loaded
recent metadata (at most 24 entries); the main browser retains the existing
50-conversation service boundary. Date groups use the device's local calendar.

Files are identified by retained bundle plus file/candidate identity, so
repeated filenames from different runs stay distinct. “Show in chat” appears
only when that exact output card exists in the loaded transcript. It focuses
and reveals the card while preserving the unsent message. Narrow windows use
the existing modal drawer and focus management. Files remain available through
conversation information and Saved as well. Existing transcript search updates
when work disclosures open or close; collapsed activity is excluded until
expanded.

## Display contract

`internal/workactivity` is a bounded projection of existing public runner
events. It does not change agent execution, prompts, capabilities, policy,
approvals, routing, scheduling or budgets.

- Up to 160 entries and 64 KiB of narration per run. Shortened activity is labeled.
- Stable entry identities follow delivery order. Tool start/finish events pair
  by step and tool-call ID. Narration deltas append to the current text entry.
- Retain public parent-agent text, tool names and lifecycle states. Tool
  arguments/results/errors, child payloads, provider data and private reasoning
  are excluded. A failed tool gets a failure state without copying raw errors.
- Actual start/finish timestamps supply elapsed work time. Unfinished tools
  become **Unfinished**, including when a run stops. The trace's final state
  distinguishes completed, failed and stopped work.
- Remove the final answer's identical streamed text from completed work; the
  canonical response stays in its existing transcript position.
- Store the optional `activity` projection in private display sidecars alongside
  new immutable revisions. The strict revision JSON schema remains unchanged.
  Missing, damaged, newer or unwritable display data cannot invalidate the
  canonical answer or turn successful work into a failure.
  Restore only activity belonging to the selected revision ancestry. Earlier
  revisions without this field remain readable and receive no invented history.
- The live desktop bridge and durable revision use the same projector. No
  existing telemetry channel gains narration payloads.

Gator currently has no provider thinking event in this contract. The interface
shows public narration and tools; there is no synthetic thinking trace. Rich
file viewers, full-content history search, computer control, context occupancy
meters and connector authorization controls remain separate capabilities.

## Reference

Read-only source review used InvestmentGini core-web:

- Cowork `c1044b9fa423f8bdfb4c81943f7a2404ce000465`: Tide geometry,
  contextual files and dated search.
- Develop `850826f4c32cffc43218c4712a9ef7e53dcd9166`: ordered activity,
  paired tool rows, bounded details and completed-work disclosure.

No InvestmentGini source, branch, dependency or profile was changed during
this implementation.

## Interface review

Full review using `make-interfaces-feel-better`, scoped to the surfaces above.
React 19, native CSS and Web Animations; no additional UI framework.

| Category | Evidence | Result |
| --- | --- | --- |
| Typography | Landing greeting, public narration, date headings, search matches, file metadata, elapsed times | Existing tokens and font stack; wrapping and tabular durations retained. |
| Surfaces | Activity disclosures, date rows, file rail, source previews, empty/error states | 40 px desktop controls; structural separators; exact retained file identities. |
| Animations | Ambient field, working mark, shimmer, interruptible disclosure, rail transitions | Reduced motion, 10% playback inspection, closing controls made inert immediately. |
| Icons | Lucide tools/search/files/check/failure icons and existing Gator symbol | Existing 1.5 px currentColor styling and explicit text states. |
| Performance | Glyph atlas, canvas clock, capped activity, bounded details, search | Canvas outside React, 20 fps, DPR capped at 2, hidden-page pause, observer cleanup; no token-driven entrance animation. |

| Severity | Location | Before | After | Why |
| --- | --- | --- | --- | --- |
| LOW | `desktop/src/AsciiWaves.tsx:21`, `App.tsx:257` | Small concentric ornament | Masked ambient Tide field, static reduced-motion frame, theme/resize updates | Selected direction; greeting and composer stay legible. |
| MEDIUM | `internal/workactivity/activity.go:75`, `workapp/bridge.go:132`, `worksession/activity.go:18` | Live events reduced to a status and lost from the chat display | Bounded public projection retained with each new revision | Stable ordered activity and truthful replay. |
| MEDIUM | `desktop/src/ActivityTimeline.tsx:62`, `App.tsx:262` | One live status row | Interleaved narration/tools, folded completed work, actual duration and explicit outcomes | Progressive disclosure while keeping the answer easy to read. |
| LOW | `desktop/src/styles.css:689` | Spinner only | Subtle mascot/shimmer and static completion/failure cues | Restrained motion with reduced-motion and high-contrast text fallbacks. |
| MEDIUM | `desktop/src/RecentConversations.tsx:6`, `ConversationsPanel.tsx:9`, `chatPresentation.tsx:3` | Undated title lists and unhighlighted filtering | Dated, literal, keyboard-accessible search with explicit loaded scope and refresh retry | Find the right topic without overstating search coverage. |
| MEDIUM | `desktop/src/WorkDetails.tsx:38`, `OutputCards.tsx:4`, `App.tsx:111` | Outputs accessed through the inspector/library | Contextual Files entry, filename filter and exact Show in chat navigation | Faster file retrieval without losing draft or reading context. |

### Considered and rejected

| Candidate | Reason |
| --- | --- |
| Click ripples or a permanent ASCII picker | The user explicitly removed click ripples and selected one pattern. |
| Copying Chakra/Framer, fonts or company branding | Gator already has a coherent React/CSS system and mascot. |
| Thinking labels or inferred context percentages | The current data does not support these claims. |
| Rich Office/PDF viewers | Existing retained previews support text/Markdown and metadata; other viewers require additional capabilities. |

## Verification

All checks use isolated profiles and local fixture providers.

| Check | Observed result |
| --- | --- |
| `npm run check --prefix desktop` | TypeScript passes, including the new E2E fixtures. |
| `npm run build:runtime` and `npm run build:ui` in `desktop` | Runtime and UI build successfully. |
| `electron-builder --dir --publish never` from `desktop` | Current macOS x64 application rebuilt; signing remains the existing manual-development configuration. |
| `go test ./internal/workactivity ./internal/worksession ./internal/workapp ./internal/workrun` | Passes, including bounded projection, tool pairing, retained sidecars, malformed display data and display-write failure. |
| `go test ./cmd/gator -run TestLoadWorkTUIConversation` and targeted interactive-work tests | Passes, including selected-ancestry restoration and older revisions. |
| `go vet` for the four affected internal packages and `cmd/gator` | Passes; `worksession` checked again after the sidecar refinement. |
| `npm test --prefix desktop` | 27 passed, one Windows-native test skipped on macOS. |
| Packaged `npm run test:e2e -- --reporter=line` | All 21 tests passed, including real runner events and reopening after a full process restart with no additional model requests. |
| Final targeted packaged checks | Search retry, literal highlighting, dates and keyboard selection; settled Files overlay and focus return; stopped/empty activity and disclosure/search integration rechecked. |
| Isolated Tide review | Changing frames; background clicks add no effects; reduced motion freezes the field; static theme redraw; 10% animation playback; visibility-handler pause/resume; 1440×960 and 800×600 layouts; zero page errors. |

The Tide visibility check exercises the handler with simulated document
visibility events; native OS hidden-window event delivery was not reverified.
Screenshots were inspected for the wide landing page, short window with setup,
chat search, live/completed activity and settled wide/narrow file previews.
Activity disclosure was inspected at 10% speed and interrupted during its
transition; closing content becomes inert immediately. Reduced motion and the
existing command palette, approvals, queue controls and draft recovery passed.

No native Windows/Linux execution, screen-reader session or native Windows
high-contrast session was run in this pass. High-contrast shimmer text has a
static CSS fallback, inspected in source. Prior cross-platform support is not
re-certified by the macOS checks.

**Verdict: Approve for the exercised macOS presentation scope.** The unverified
platform and assistive-technology checks above remain outside this verdict.

## Atomic implementation commits

| Commit | Change |
| --- | --- |
| `2c4f46e03` | Ambient Tide field |
| `b91e967aa` | Bounded public activity projection |
| `47c2284cd` | Hybrid activity UI |
| `8154bba4c` | Dated chat search |
| `32d01264a` | Contextual Files and Show in chat |
| `71b05b891` | Accurate activity summaries and accessible motion |
| `531a47744` | Optional display sidecars preserving the revision schema |
| `449c8f2ef` | Chat-list refresh recovery and search polish |
| `29288d5fb` | Transcript matches follow work disclosure visibility |

The activity UI depends on the display contract; Files shares the literal
highlight helper with chat search. Revert dependent UI changes before their
shared infrastructure. Optional display sidecars can remain when rolling back;
the canonical revision JSON is unchanged.


## Historical follow-up: one primary chat entry (superseded)

The later shared-memory decision restores New chat / Cmd/Ctrl+N and replaces the
privileged primary chat with last-active reopening. See the current
[implementation and memory boundaries](DESKTOP-CONVERSATIONAL-IMPLEMENTATION.md).
The following records the earlier removal for rollback context.

22 October 2026. The user removed the standalone **New topic** entry because it
competed with the continuing main chat. Retire its sidebar button, Cmd/Ctrl+N,
command-palette entry, slash suggestions and help references together. Empty
conversation greetings now use Gator's common introduction. Existing saved
conversations and their independent drafts remain accessible. Folder changes
retain their existing source boundaries; the runtime reset path remains an
internal service.

Full scoped review with the existing interface skill (React and native CSS):

| Category | Evidence inspected | Result |
| --- | --- | --- |
| Typography | Empty-chat heading, subtitle, help and draft titles | Common Gator introduction and existing tokens. |
| Surfaces | Sidebar, saved-conversation navigation and help | Remove the redundant control and its unused margin. |
| Animations | Sidebar layout and disclosure guards | No motion added or changed; 10% playback not repeated for this removal. |
| Icons | Sidebar imports and surviving navigation | Remove the unused Plus icon; retain the existing icon system. |
| Performance | Command discovery in App, Help and slash suggestions | One memoized presentation filter shared by all command surfaces. |

| Severity | Location | Before | After | Why |
| --- | --- | --- | --- | --- |
| MEDIUM | `desktop/src/App.tsx:74`, `DesktopHelp.tsx:20`, `styles.css:119` | New topic button, shortcut and reset command advertised alongside the main chat | Remove the duplicate entry points, shortcut help, unused icon and spacing | Keep the main conversation as the clear starting point. |
| LOW | `desktop/src/App.tsx:256`, `useDrafts.ts:52` | An alternate blank-chat introduction and New topic draft label | Common Gator introduction and neutral Conversation draft label | Match the simplified conversation model. |

Considered and rejected: deleting retained topics or source-reset infrastructure
would lose existing scope boundaries; repurposing Cmd/Ctrl+N to reset the main
chat would undermine continuity. Neither is part of this presentation change.

Verification: `npm run check --prefix desktop`, the UI build and the macOS
package build passed. The three scoped packaged Electron tests passed
(recent conversations/drafts, queued messages, and real Go state/restart).
They exercise the retired shortcut and palette command, saved-topic draft
isolation, queue-switch confirmation, main-conversation restoration and native
800/1024/1440 px layouts. The current home screenshot was inspected and the
New topic button is absent. No new inference provider or real user profile was
used. Windows/Linux execution and assistive-technology checks were not repeated.

The implementation is retained in the existing atomic commit `d59484280`.
**Verdict: Approve for this scoped macOS removal.**

## Follow-up: quiet ordinary replies

22 October 2026. A plain answer should read as a message. The runtime retains
a bundle and appends a context shortcut after every successful run; neither
record is itself a reason to add another chat card.

- Omit empty bundle messages, including their heading and spacing, for live
  and restored conversations. Actual files and proposed changes keep their
  existing preview cards, check states and exact Show in chat identities.
- Remove the duplicate Result details disclosure from file groups. Retained
  file locations remain in output previews; task-run screens retain run details.
- Omit the automatic Context and sources shortcut. Sources & memory in
  Conversation information still opens saved context. Explicit context panels,
  warnings, errors and approvals remain visible.
- Omit successful work timing when no activity was retained. Meaningful public
  activity stays in its disclosure; failed/stopped runs keep a status cue even
  without entries. Live progress is unchanged.

These are renderer decisions. Raw messages, bundles, source records, activity,
task history and agent execution are unchanged. Filtering before rendering
also keeps empty articles and repeated metadata out of transcript search.

Full scoped review using `make-interfaces-feel-better`, React and existing
native CSS. This covers completed replies and their information/file paths.

| Category | Evidence inspected | Result |
| --- | --- | --- |
| Typography | Message labels, answer body, timing and file metadata | Existing tokens; ordinary replies lose repeated headings. |
| Surfaces | Empty bundle, automatic context link, actual output and approval cards | Omit boilerplate articles; keep real outputs and review surfaces. |
| Animations | Completed activity and existing disclosure | No new motion; existing disclosure/reduced-motion checks retained. |
| Icons | Existing activity indicators, file previews and approvals | Same Lucide system; failed/stopped icons keep text labels. |
| Performance | Message renderer and transcript search | Skip unused cards rather than mounting hidden diagnostics; remove dead bundle formatter and CSS. |

| Severity | Location | Before | After | Why |
| --- | --- | --- | --- | --- |
| MEDIUM | `desktop/src/App.tsx:59`, `Result.tsx:5`, `styles.css:207` | Every reply received empty Files & changes, Result details and Context cards | Only render actual outputs; use the optional inspector for metadata; remove unused fallback formatter/styles | Repeated chrome interrupts reading without adding information. Explicit results retain their review and warning behavior. |
| LOW | `desktop/src/ActivityTimeline.tsx:62` | Empty successful activity added a standalone Worked for line | Show timing with inspectable activity; retain failure/interruption cues | Routine answers need no empty progress recap. |

Considered and rejected: dropping records from the runtime would lose useful
retained evidence and change backend behavior; suppressing all result messages
would hide warnings and actionable approvals.

Verification:

- `npm run check --prefix desktop` and `npm run build:ui --prefix desktop`
  passed. The build retains the existing highlight/Lucide warnings.
- From `desktop`, `./node_modules/.bin/electron-builder --dir --publish never`
  rebuilt the macOS app successfully.
- With `GATOR_DESKTOP_TEST_EXECUTABLE` pointing to the rebuilt
  `desktop/release/mac/Gator.app/Contents/MacOS/Gator`, ran
  `npm run test:e2e --prefix desktop -- activity.spec.ts files.spec.ts work-details.spec.ts desktop.spec.ts --grep 'public activity interleaves|real runner narration|contextual files reuse|compact work details|desktop uses real Go state|review presentation preserves' --reporter=line --output=node_modules/.cache/quiet-replies-results`.
  Five checks passed. The full Go-state check initially closed an already-open,
  remembered Sources section; corrected the test to assert its persisted state.
  Reran `npm run test:e2e --prefix desktop -- desktop.spec.ts --grep 'desktop uses real Go state' --reporter=line --output=node_modules/.cache/quiet-replies-recheck`
  with the same packaged executable: passed in 17.4 seconds.
- The six passing checks cover plain replies, saved context inspection and
  draft preservation, real runner activity before/after restart, empty
  failed/stopped cues, explicit context warnings, exact live approvals, real
  output previews and Show in chat. Existing activity motion was inspected at
  10% speed and with reduced motion; file drawers also ran at 800 px.
- Inspected the current quiet-reply, completed-activity and file-preview
  screenshots. Plain replies have no empty file/context cards or timing line.
  Tests used isolated profiles and a local inference fixture.

**Verdict: Approve for the exercised macOS presentation scope.** Windows/Linux
execution and assistive-technology checks were not repeated.

## Follow-up: command palette row icons

22 October 2026. Remove the repeated decorative Command/⌘ icon from every
palette row and its unused import. Existing labels, descriptions, advanced
badges, chevrons and keyboard hints retain their meaning.

Full scoped review with `make-interfaces-feel-better`, React and native CSS:

| Category | Evidence inspected | Result |
| --- | --- | --- |
| Typography | Command names and descriptions | Existing fonts and tokens; text starts at the row's normal padding. |
| Surfaces | Row layout and selected/hover states | Existing flex layout closes the removed column; row hit areas retained. |
| Animations | Existing palette/disclosure styles | No motion changed; 10% playback not repeated for an icon removal. |
| Icons | Palette rows, search header and shortcut hints | Repeated decoration removed; meaningful navigation and shortcut cues retained. |
| Performance | App icon import and row markup | Remove unused SVG instances and import. |

| Severity | Location | Before | After | Why |
| --- | --- | --- | --- | --- |
| LOW | `desktop/src/App.tsx:2`, `App.tsx:320` | Every command began with the same Command icon | Text begins directly at row padding | The repeated symbol adds no distinction and competes with the labels. |

Considered and rejected: replacing these with individual command icons would
introduce a new visual taxonomy; retaining an empty icon column would leave
unnecessary indentation. Neither is needed for this removal.

Verification: `npm run check --prefix desktop`, `npm run build:ui --prefix desktop`
and macOS packaging passed. An isolated packaged-app inspection verified absent
row icons, initial search focus, arrow navigation, filtering/empty results,
Enter opening Saved, Escape closing it and preservation of the unsent message.
The current palette screenshot was inspected. No permanent tests were added
for this reversible decoration removal.

**Verdict: Approve for this scoped macOS removal.** Windows/Linux and
assistive-technology checks were not repeated.

## InvestmentGini reference work closed

22 October 2026. The user ended the reference exploration and declined further
ports except the composer model and reasoning picker recorded below. The Tide
and hybrid activity features above were already implemented; this decision
does not remove existing Gator functionality.

Cleanup completed on the development Mac:

- Stopped the separate sample-data previews on ports 3002 and 3003 and removed
  their external launcher, caches, temporary logs and screenshots.
- Removed the temporary detached Cowork worktree. The original frontend stays
  on `develop` at `850826f4c`, matching its saved `origin/develop` reference.
- Restored the backend's exact original local `.env` and removed the empty
  database and auth tables created by the setup attempt. Original local
  containers are stopped. Existing dependency directories remain installed;
  they are ignored development artifacts.
- InvestmentGini's original frontend, backend and auth-proxy working trees are
  clean. No InvestmentGini commits or pushes were made.

Reference locations inspected:

| Surface | InvestmentGini reference | Gator counterpart |
| --- | --- | --- |
| Landing atmosphere | `src/components/PageBackground/PageBackground.tsx` | `desktop/src/AsciiWaves.tsx`; the approved Tide field already exists |
| Completed activity and prose | `ChatMessageResponseDisplay/CollapsedReasoningSteps.tsx`, `ThinkingAnalysisMessage.tsx`, `HistoricalChatMessages.tsx` under `src/pages/ProjectDetailPage/ChatPage/` | `desktop/src/ActivityTimeline.tsx`; retain public activity and the canonical answer without duplicating it |
| Composer controls and attachments | `src/components/Chat/HomePageChatInput/HomePageChatInput.tsx` and adjacent source/attachment components | Existing composer in `desktop/src/App.tsx` and `desktop/src/Attachments.tsx` |
| Motion | `src/themes/FramerMotion.ts` | `desktop/src/motion.tsx` and shared CSS; preserve reduced-motion behavior and the existing stack |

These locations remain reference history, not a backlog of additional UI ports.

## Composer model and work-effort picker

22 October 2026. The user selected InvestmentGini's compact model picker as a
reference for Gator's chat composer, using two supplied screenshots:

- A composer pill shows the selected model and reasoning effort. Opening it
  shows the effort label, a model dropdown, a discrete effort slider, and a
  reset control.
- Choosing the model opens a second view in the same popover, with a back
  control, models grouped by provider, and a clear selected-row checkmark.
- Adapt the pattern to Gator's existing palette, typography and icons. Use
  Gator's actual configured models and supported reasoning levels; the sample
  provider/model names and available effort stops are reference content.
- Reference InvestmentGini's frontend implementation as needed.

**Authorized and implemented on 22 October 2026.** This is the only additional
InvestmentGini UI port selected by the user. Other proposed ports remain declined.
The implementation uses Gator's real work-effort settings rather than presenting
unsupported provider thinking levels.


### Implementation and review

The composer pill opens a compact popover with Low, Standard and High work
effort, a model dropdown and Reset. The second view groups configured models by
provider, marks the current selection and offers local filtering for longer
lists. Manage models opens the existing setup screen. Display aliases preserve
raw model identity. Reset restores Standard work effort and keeps the selected
model. Model defaults persist for subsequent requests; effort retains its
existing conversation/session semantics. No provider API reasoning parameters
were introduced.

Go supplies the choices and acknowledges each change. Fresh configuration,
current view identity and idle-state checks prevent stale or active-work
changes. Current and queued work must finish first. Changes preserve draft,
attachments, workspace and conversation, and add no transcript notices. The
bridge projection holds up to 256 choices and retains the selected model when
shortened; the UI labels that limit. Opening the picker performs no network
requests, downloads or credential writes. Credential material and endpoint
configuration are excluded from its display contract.

Full scoped review using `make-interfaces-feel-better`: React, Lucide and the
existing native CSS/theme tokens. This covers the new picker, its composer
integration and the supporting display/action contract, not a new app-wide audit.

| Category | Evidence inspected | Result |
| --- | --- | --- |
| Typography | Model aliases, long labels, group headings, slider labels and unavailable explanations | Existing UI fonts; wrapped model names, truncated trigger and explicit work-effort meaning |
| Surfaces | Both popover views, search, selected/disabled rows, viewport placement, 800×650 window | Bounded 304 px width / 420 px height, scrolling list, theme tokens and 40–44 px controls |
| Animations | Open/close at 10% playback, nested view changes, reduced motion | Interruptible restrained transitions; slider thumb/fill update together without a trailing fill animation |
| Icons | Trigger, model rows, checkmark, reset, back and setup link | Existing Lucide set and shared 1.5 px strokes |
| Performance | Cached choices, bounded bridge projection, native range and local filtering | No provider lookup in the streaming projection and no added UI dependencies |

Resolved findings:

| Severity | Location | Before | After | Why |
| --- | --- | --- | --- | --- |
| MEDIUM | `desktop/src/App.tsx:287`, `desktop/src/ModelPicker.tsx:13` | Choosing a model required leaving the composer | Compact model/effort pill with setup link, canonical acknowledgement and inline errors | Keep the draft and task context in place |
| MEDIUM | `desktop/src/ModelPicker.tsx:101`, `desktop/src/styles.css:197` | A long choice list could consume the entire area above the composer | Viewport placement and a 420 px height cap with scrolling and filtering | Preserve the compact presentation and readable rows |
| MEDIUM | `desktop/src/ModelPicker.tsx:92` | Nested model view needed explicit focus and dismissal handling | Roving option focus, arrow/Home/End selection navigation, Escape back/close, outside dismissal and focus return after failed saves | Keyboard access must not navigate or cancel unrelated work; failed saves must remain retryable |
| MEDIUM | `desktop/src/styles.css:201` | Generic disabled-button opacity made setup explanations faint | Readable disabled labels and explanatory text with unavailable controls disabled | Communicate availability without relying on opacity alone |
| LOW | `desktop/src/styles.css:183`, `styles.css:216` | Small trigger and independently animated fill | 40 px trigger and immediate native-slider/fill updates | Preserve hit areas and keep the indicator synchronized |

Considered and rejected:

| Candidate | Reason |
| --- | --- |
| Copy InvestmentGini's provider thinking labels/levels | Gator exposes work budgets, not those provider parameters; presenting them would misstate the behavior |
| Import its Chakra/Framer styling or provider-logo system | Gator already has native CSS, theme tokens, Lucide and motion helpers |
| Continuously animate particles in the effort bar | Repeated decorative motion adds attention cost to a frequently used control |

Verification:

- `go test ./internal/workapp ./cmd/gator -count=1` passed.
- Targeted composer tests cover budgets, reset, draft preservation, persisted
  model callbacks, rejected stale/busy/unavailable requests, save failures,
  display aliases, private metadata exclusion and bounded choices.
- `go test ./internal/workrun -run 'TestApplicationContext|TestConversationReceivesCurrentAppKnowledge' -count=1` passed after updating the bundled app guide.
- `node --test desktop/tests/policy.test.mjs` passed (six checks), including
  rejection of unsupported fields, invented effort levels and invalid IDs.
- `npm run check --prefix desktop` and `npm run build --prefix desktop` passed.
- `npm run test:e2e -- app-control.spec.ts --reporter=line` from `desktop`
  passed all four checks, including existing chat controls and voice behavior.
- The new Electron scenario verifies the selected model in an actual local
  inference request, persisted defaults, keyboard effort/reset, alias filtering,
  unavailable rows, draft/conversation preservation, outside dismissal, Escape
  focus return, active-work locking and reduced-motion placement at 800×650.
  On macOS, a real filesystem permission failure also verifies the inline save
  error, unchanged defaults, retained keyboard focus and successful retry.
- Isolated manual Electron review inspected 10% popover motion, nested Escape,
  long labels and Contrast/Mono themes with zero renderer errors. Screenshots
  for both views and the narrow window were inspected. Normal theme was covered
  by the end-to-end scenario.
- `npm run pack --prefix desktop` rebuilt the local macOS app; the existing
  highlight/Lucide and bundle-size notices remain build warnings.
- The final picker scenario also passed against the rebuilt packaged
  `desktop/release/mac/Gator.app` (one check, 9.7 seconds), including the real
  save-failure and retry path.

**Verdict: Approve for the exercised macOS scope.** Windows/Linux execution and
screen-reader testing were not repeated. No additional InvestmentGini UI port
is authorized by this work.

## Inline dictation and attachment feedback

22 October 2026. The user requested the compact recording treatment shown in
their ChatGPT screenshot and removal of the paperclip spinner and “Preparing
files…” label. Full scoped review using `make-interfaces-feel-better`, covering
the React composer, native CSS, Lucide icons, voice lifecycle presentation and
attachment-picker feedback. Backend transcription, permissions, limits and
attachment validation remain on their existing paths.

The voice menu retains provider/language setup and the explicit Start recording
action. Active capture replaces that menu with a rounded pill beside the model
picker: cancel, nine microphone-responsive bars, elapsed time and a square
finish control. The bars sample the existing stream locally through Web Audio,
without another recording request, audio output or a waveform dependency.
Opening the microphone and transcribing have short inline status labels and a
cancel control. Finish appends editable text to the existing draft. Cancel or
Escape discards the recording and returns focus; capture errors reopen setup
with the error focused. The compact window hides the timer and temporarily
gives the recording controls the model pill's space. Reduced motion uses a
static recording mark.

The attachment button keeps its paperclip while the native file chooser is
open or files are being staged. Duplicate-action and send guards, accessible
busy state, file chips, drop feedback and errors remain available. Removing
the preparation label also avoids moving the composer for a native dialog.

Reference: the user's screenshot supplies the visual direction. Official
[ChatGPT dictation guidance](https://learn.chatgpt.com/docs/prompting) confirms
the speech-to-editable-composer interaction; the implementation uses Gator's
existing transcription services.

| Category | Evidence inspected | Result |
| --- | --- | --- |
| Typography | Inline opening/transcribing labels, errors, elapsed timer, setup menu | Existing type scale and theme tokens; tabular time; no repeated preparation label |
| Surfaces | Idle, recording, silence, transcribing, capture error, native picker pending, 800×650 layout; Normal/Contrast/Mono screenshots | Compact pill, 40 px button targets and visible structural border; no clipping in the exercised window |
| Animations | Actual pill opacity/transform transitions paused at their midpoint at 10% playback; microphone level rise/fall; reduced motion | Restrained entrance; waveform responds to sound, settles in silence and becomes static for reduced motion |
| Icons | Paperclip, microphone, cancel, finish and spoken-reply controls | Existing Lucide system; steady paperclip; filled square differentiates finish within the recording group |
| Performance | Local analyser, nine transform updates at about 16 Hz, cleanup and canceled captures | No added dependency or extra media capture; no React state update per audio frame; analyser/context cleaned up when recording ends |

Resolved findings:

| Severity | Location | Before | After | Why |
| --- | --- | --- | --- | --- |
| MEDIUM | `desktop/src/VoiceControls.tsx`, `desktop/src/App.tsx` | A large settings panel remained open during recording | Active recording pill next to the model control, with finish/cancel and inline progress; setup remains in the idle menu | Keep attention on the message and make recording state visible in its destination |
| MEDIUM | `desktop/src/DictationWaveform.tsx` | No visible input-level feedback | Locally sampled audio bars with silence and reduced-motion states | Show actual microphone activity rather than implying activity through a decorative loop |
| MEDIUM | `desktop/src/VoiceControls.tsx` | Moving controls could lose focus on completion/cancellation/errors | Phase-appropriate focus, Escape cancellation, microphone focus return and focused capture error | Keyboard users need a clear continuation after each state change |
| MEDIUM | `desktop/src/Attachments.tsx`, `desktop/src/App.tsx` | Opening the native file chooser immediately showed a spinner and “Preparing files…” | Steady paperclip and no preparation label; pending actions remain guarded | Avoid claiming preparation before a file is even selected |
| LOW | `desktop/src/styles.css` | A widened recording control could crowd a narrow toolbar | Compact timer/model treatment at 1000 px and below; right-aligned voice menu | Preserve reachable controls and keep the menu within the window |

Considered and rejected:

| Candidate | Reason |
| --- | --- |
| Simulated continuously bouncing bars | They would suggest microphone activity during silence; use measured input instead |
| Automatic microphone start while opening setup | Keep the existing explicit start action and provider disclosure |
| A second recording modal or new transcription service | The requested change is presentation; the current permission/transcription path already supplies it |

Verification:

- `npm run check --prefix desktop` and `npm run build:ui --prefix desktop` passed.
- `node --test desktop/tests/voice.test.mjs desktop/tests/uploads.test.mjs`
  passed all five checks.
- The updated Electron dictation scenario passed against the rebuilt packaged
  macOS app: measured sound/silence, finish-to-draft, blocked Send, spoken replies,
  cancel during capture/opening/transcription, late microphone release, capture
  error and retry, keyboard focus, draft preservation and narrow reduced-motion
  placement. Audio and permissions use isolated fixtures, not a physical mic.
- The attachment cancellation/removal/validation Electron scenario passed.
- A separate isolated packaged-app inspection checked recording hover/focus,
  10% entrance transitions (two actual CSS transitions), Contrast and Mono,
  the paperclip while a native-picker request was pending, and cancellation
  preserving the draft. Screenshots were inspected; zero renderer errors.
- The model-picker regression check exposed a timing assumption: its label can
  change before the acknowledgement returns to the effort view. The test now
  waits for that view before checking Escape, rather than treating the earlier
  label update as completion. Its outside-click check uses an uncovered part
  of the textarea now that the control sits farther left. The updated scenario
  passed against the completed packaged app (one check, 4.7 seconds).
- `npm run pack --prefix desktop` rebuilt the local macOS app. An early test
  launch overlapped packaging and failed before the executable was available;
  subsequent tests use the completed bundle.

**Verdict: Approve for the exercised macOS presentation scope.** Windows/Linux,
physical-microphone accuracy and screen-reader execution were not repeated.

## ASCII atmosphere throughout chat

22 October 2026. Restored the existing pulsating Tide field after the first
message, as requested. One viewport-sized canvas now lives behind the transcript
scroller in both home and chat; it keeps its animation clock when the first
message arrives. Chat uses 45% of the landing field's intensity, fading between
the two treatments. Text bubbles, controls and the composer remain above it.
Click ripples remain removed. Settings keep their existing presentation.

Full scoped `make-interfaces-feel-better` review: `desktop/src/App.tsx` and
`desktop/src/styles.css`, using the existing React/native CSS/canvas stack.

| Category | Evidence inspected | Result |
| --- | --- | --- |
| Typography | Short and long chat replies, Markdown headings/lists, composer draft | Existing type and opaque message surfaces remain readable |
| Surfaces | Landing/chat transition, scrolled chat, 800×650 screenshot and composer controls | One isolated background layer; no pointer interception or horizontal overflow |
| Animations | Existing canvas clock at 10% playback, live pixel updates and reduced-motion frames | Pulse continues into chat; reduced motion is stationary after layout settles |
| Icons | Existing message actions and jump-to-latest control over the field | No icon changes; controls remain clickable above the canvas |
| Performance | Canvas bounds with 18-message inspection and 300-message regression | Canvas stays viewport-sized; no canvas per message or React render per frame; existing hidden-window pause retained |

| Severity | Location | Before | After | Why |
| --- | --- | --- | --- | --- |
| MEDIUM | `desktop/src/App.tsx:280` | The atmospheric field disappeared when chat replaced the landing view | Shared canvas outside the home/chat content branch | Preserve continuity without restarting the effect |
| LOW | `desktop/src/styles.css:618` | Landing strength would compete with sustained reading | Quieter background with an isolated stacking context and no pointer events | Keep the requested atmosphere while preserving the conversation's hierarchy |

Considered and rejected: a canvas inside each message would multiply work;
placing it on the whole app would affect settings/sidebar; restoring click
ripples would reverse the user's earlier removal request.

Verification: TypeScript check, UI build and macOS packaging passed.
`npm run test:e2e -- desktop.spec.ts --grep 'long conversations retain context'
--reporter=line` passed (one check, 5.0 seconds). Isolated packaged-app inspection
verified canvas identity across home/chat, changing pixels at 10% playback,
viewport-sized bounds, typing, jump-to-latest, narrow layout and stationary
reduced-motion rendering. The initial static-frame comparison preceded a pending
resize redraw; comparing after layout settled passed. Screenshots were inspected.

**Verdict: Approve for this macOS scope.** Windows/Linux and screen-reader
execution were not repeated. File preview/editing remains a separate proposal
in [Files inside Gator chat](INLINE-FILES-RESEARCH.md), pending user verification.

## Follow-up: independent chats with shared personal memory

22 October 2026. The approved product model is one Gator identity across several
chats. New chat and Cmd/Ctrl+N return, launch reopens the last selected chat,
and a fresh working context keeps access to shared personal memory. Existing
chat styles, command palette, files, queue and progressive disclosure remain.
Image paste stages an ordinary previewable attachment without sending it.
The [implementation note](DESKTOP-CONVERSATIONAL-IMPLEMENTATION.md) defines the
memory, provenance, retrieval and local-profile boundaries.

### Scoped interface review

Full mode using `make-interfaces-feel-better`: React 19, the existing native CSS
variables/utilities and Lucide icons. Scope is new-chat navigation, recents,
composer paste/recovery, command discovery and memory controls. This is not a
new audit of every settings page or a native Windows/Linux review.

| Category | Evidence inspected | Result |
| --- | --- | --- |
| Typography | `App.tsx`, `RecentConversations.tsx`, memory panel summaries, pending image names; normal/compact layouts | Reuse the system font, existing text sizes, truncation and clear chat labels. |
| Surfaces | Existing `.nav-button`, `.composer`, `.inline-file`, panel summary/toolbar styles; empty, populated, failure, hover and focus states | New chat keeps the 40 px target; pasted images use existing preview/remove cards and inline errors. |
| Animations | Existing sidebar, details and palette transitions; 10% playback and reversal in the motion test | No new per-message or per-paste motion; existing interruptible transitions and reduced-motion behavior retained. |
| Icons | Plus, paperclip, image/file cards and memory/history activity labels | One Lucide/currentColor system, existing 1.5 px regular stroke; no new icon library or image asset. |
| Performance | Draft identity/readiness, ClipboardEvent payload, bounded native decode and selected memory capsule | Independent drafts; no clipboard polling or extra background extraction call. Counts/bytes checked before reading pasted images. |

| Severity | Location | Before | After | Why |
| --- | --- | --- | --- | --- |
| MEDIUM | `desktop/src/App.tsx:296`, `desktop/src/RecentConversations.tsx:41`, `desktop/src/useDrafts.ts:6` | A privileged main chat with separate topics and no New chat | Last-active reopening, explicit New chat and recent chats with independent draft identities/titles | Match the approved mental model and preserve context while changing conversations. |
| HIGH | `desktop/src/App.tsx:176`, composer textarea | Typing could race asynchronous draft restoration | Wait for draft readiness; focus the ready composer after New chat | Prevent an immediately typed message from being overwritten and maintain keyboard continuity. |
| MEDIUM | `desktop/src/App.tsx:289` | Empty retained chats displayed the generic New chat header | Use the retained conversation title | Stable labels make saved-chat navigation understandable. |
| MEDIUM | `desktop/src/App.tsx:360` | A raw reset command, with slash-prefixed palette queries producing no match | New chat label; accept both `/new` and `new chat` search | Visible command syntax and actual discovery should agree. |
| MEDIUM | `desktop/src/Attachments.tsx:31`, composer paste handler | Copying a screenshot could not attach its image bytes | Paste stages the image in the existing preview/remove workflow; errors retain the text and attachments | Make clipboard intake predictable, reviewable and consistent with chosen files. |
| MEDIUM | `internal/workapp/recall.go:149`, `cmd/gator/work_context.go:37` | Retrieval configuration without automatic-learning status or chat provenance | Learn from chats On/Off, a reversible control and source-chat/corrected labels | Users can understand and change what carries across chats without another dashboard. |

Considered but rejected:

| Location | Candidate | Rejected because |
| --- | --- | --- |
| Navigation | Keep a special forever main chat alongside New chat | The approved model makes continuity a memory property; last-active reopening is sufficient. |
| Composer image cards | Add another preview rail or always expand every pasted image | The existing inline file preview already provides inspection while keeping the composer compact. |
| Paste/New chat | Add staged entrance effects or another loading spinner | These are frequent actions; existing state, focus, disabled controls and error text provide useful feedback. |
| Memory | Add a permanent dashboard/counter in the chat | Saved and Memory & retrieval already provide the controls; no truthful context-capacity metric is needed for this feature. |

Verification results follow. Native assistive-technology and Windows/Linux
execution are outside this scoped review.

### Verification and verdict

- `go test ./internal/workapp ./internal/workrun ./internal/lifecycle ./internal/config ./internal/memory ./internal/tools ./internal/worksession -timeout 120s`: passed.
- `go test ./cmd/gator -timeout 180s`: passed. Subsequent foreground-capsule/exclusion changes passed `go test ./cmd/gator -run 'Test.*Memory|TestChatProfile' -timeout 90s` and `go test ./internal/memory ./internal/tools -timeout 90s`.
- `npm run check --prefix desktop`: passed. `npm test --prefix desktop`: 44 passed, with the Windows-native handoff test skipped on macOS.
- `npm run pack --prefix desktop`: built the unsigned macOS app. Independently compared 21 packaged Electron/UI files and the bundled Go runtime with their build inputs; all bytes matched.
- Source desktop regressions passed for attachment/queue ownership, cancellation/errors, inline text/version recovery, native Office edits, shared memory and native paste. Two navigation findings from the regression pass (slash search and empty-chat titles) were fixed and passed in the package.
- Packaged Electron checks passed for recent-chat/draft navigation, real Go state/forms/restart, interruptible motion, cross-chat memory and forgetting/learning controls, native image paste/preview/send/restart, and Gator/contrast/mono themes at 1440/800 px with reduced motion and collapsed navigation. The memory restart fixture was rerun successfully after waiting for the new conversation ID before recording it; this corrected an asynchronous test race.
- Native Cmd/Ctrl+V used a generated PNG. Prior OS clipboard contents stayed inside the test main process and were restored immediately. No real provider credentials or user profiles were used. Plain accompanying text and unsupported-image errors preserved the draft; send carried the staged PNG bytes through the actual Go runtime.
- Visually inspected the normal/narrow paste screenshots, contrast/monochrome compact screens and 10% sidebar-transition midpoint. Existing focus indicators, 40 px New chat target, restrained disclosure and responsive layout remain consistent.

**Verdict: Approve for the scoped macOS implementation.** Not verified: native
Windows/Linux execution, OS assistive-technology navigation and live-provider
memory-selection quality. The deterministic provider verifies the real storage,
context and tool paths; it does not establish recall accuracy for every model.

Roll back dependent application/UI commits before their memory/chat-store
infrastructure. Legacy primary designations and existing conversation IDs remain
intact; no transcript merge or destructive migration was performed.

## Follow-up: quieter focus, keyboard-only command search and aligned chat rows

22 October 2026. Implements the user's three screenshot corrections. Command
search remains available through its configured shortcut (Cmd/Ctrl+K by default).
The sidebar entry is removed, including its obsolete focus-return target and
spacing rule. Text fields no longer draw green outlines or duplicate frames.
Native forms retain quiet neutral border feedback; text editing keeps its caret.

### Full scoped review

React 19 with the existing CSS tokens/utilities and Lucide icons. Scope:
conversation list rows, command discovery/focus return, all text inputs,
textareas/selects/contenteditable editors and their existing containers. No
backend, memory, workspace or scheduling behavior is changed.

| Category | Evidence inspected | Result |
| --- | --- | --- |
| Typography | Wrapped conversation titles, source ellipsis and tabular dates at 1440/800 px | Clear; title/source and trailing date retain their existing typography. |
| Surfaces | Shared input rule, search/composer/queue containers, native forms, Word/Excel/PowerPoint/CSV and Markdown editing | Remove the shared and specialized green frames; neutral structural borders remain. |
| Animations | Existing palette/sidebar transitions at 10% playback, reversals and reduced motion | No new animation; removing the sidebar entry preserves keyboard focus and modal exit behavior. |
| Icons | Conversation chevron layout and removed sidebar Search icon | Date and chevron share one normal-flow, vertically centered group; existing Lucide/currentColor convention retained. |
| Performance | CSS layout/selectors and command-return ref | No dependency, measurement loop, model call or backend work added. Geometry is measured only by the regression test. |

| Severity | Location | Before | After | Why |
| --- | --- | --- | --- | --- |
| MEDIUM | `desktop/src/ConversationsPanel.tsx:51`, `desktop/src/styles.css:768` | An absolutely positioned chevron centered on the whole row, above the date | Flexible title/source column plus an inline date/chevron group | Optical alignment stays consistent for long titles, differing dates and narrow windows. |
| MEDIUM | `desktop/src/App.tsx:226`, `desktop/src/styles.css:118` | Sidebar Search row with keyboard focus returned to that button | Shortcut-only palette; return to the original control and retain intentional group spacing | Honor the requested navigation simplification without a removed or invisible focus target. |
| MEDIUM | `desktop/src/styles.css:44`, `:162`, `:255`, `:451`, `:491` and file editor rules | Green text-field outlines, container underlines/shadows and an Office-specific green outline | No text-field outline, neutral form/search borders, neutral queue/drop frames and existing text caret | One consistent focus treatment covers the full request, including nested and light document surfaces. |

Considered but rejected:

| Location | Candidate | Rejected because |
| --- | --- | --- |
| Conversation rows | Move the absolute chevron by a few pixels | A fixed nudge would still depend on row height and title wrapping. |
| Sidebar | Hide Search while keeping its button for focus return | That would leave a misleading invisible control; focus should return to the real originating element. |
| Text fields | Patch only the Saved knowledge filter | The shared outline and specialized composer/Office rules caused the same issue on other screens. |

Verification results follow. Native screen-reader and Windows/Linux checks
are outside this scoped macOS review.

### Verification and verdict

- `npm run check --prefix desktop` and `npm run build:ui --prefix desktop`: passed.
- `npm run test:e2e --prefix desktop -- chat-search.spec.ts polish.spec.ts desktop.spec.ts inline-files.spec.ts --grep 'dated chat search|text fields use quiet|motion is interruptible|Word Excel' --reporter=line`: four source-app scenarios passed. They cover row geometry at 1440/800 px, keyboard discovery and focus return, quiet focus in Gator/contrast/mono, native forms, Markdown and native Office/CSV editing, plus 10% motion reversal/reduced motion.
- `npm run pack --prefix desktop`: built the unsigned macOS app. Compared 21 Electron/UI files and the runtime with the packaged bytes; all matched.
- `GATOR_DESKTOP_TEST_EXECUTABLE="$PWD/desktop/release/mac/Gator.app/Contents/MacOS/Gator" npm run test:e2e --prefix desktop -- chat-search.spec.ts polish.spec.ts --grep 'dated chat search|text fields use quiet' --reporter=line`: two packaged scenarios passed.
- Visually inspected aligned rows with wrapped titles at 800 px, focused Saved knowledge/native form fields, the compact inline editor and the 10% sidebar midpoint. Search is absent from sidebar navigation. Text fields have no green outline or duplicate frame; normal structural borders, caret and responsive layout remain clear.

**Verdict: Approve for the scoped macOS changes.** Not verified: native
Windows/Linux execution and native screen-reader interaction. Backend behavior
is unchanged; checks used isolated temporary profiles.

Atomic UI commits: `ee464aede` (chat row alignment), `4a3c2af5c`
(shortcut-only command search), and `21aa22a01` (shared quiet text-field focus).
