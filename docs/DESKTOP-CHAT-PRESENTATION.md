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
| Completed response | Fold work above the answer, preserving the answer outside the disclosure. Show measured work duration. An answer without retained work gets a static summary instead of an empty disclosure. |
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


## Follow-up: one primary chat entry

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
| LOW | `desktop/src/App.tsx:257`, `useDrafts.ts:52` | An alternate blank-chat introduction and New topic draft label | Common Gator introduction and neutral Conversation draft label | Match the simplified conversation model. |

Considered and rejected: deleting retained topics or source-reset infrastructure
would lose existing scope boundaries; repurposing Cmd/Ctrl+N to reset the main
chat would undermine continuity. Neither is part of this presentation change.

Verification for this follow-up is recorded after the scoped packaged checks.
