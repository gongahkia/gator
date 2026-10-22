# Product surfaces for a personal Gator

Proposal, 22 October 2026. Builds on the first-party
[research note](CONVERSATIONAL-PRODUCT-DIRECTION.md). The
[clickable prototype](prototypes/conversation/index.html) uses fictional data and
local interactions; it does not execute agent work or represent shipped features.

## The product choice

Gator should open to a familiar place to talk with the same personal agent.
Users should be able to ask a small question, share a change of plan, hand over
some work or respond to a request without first categorizing the interaction.

The meaningful change is who does the organizing. Today, the user often chooses
between Overview, Goals, Inbox, Follow-up checks, Conversations and Knowledge,
then finds the right action inside that destination. Proposed: the conversation
is the front door, and Gator helps organize the responsibility behind it. Users
can inspect and change that organization at any time.

This is a hypothesis about usefulness and comfort, not proof from competitors'
marketing. A pleasant messenger can still fail if it forgets commitments, invents
progress, interrupts unnecessarily or cannot finish useful work.

There is direct design evidence as well: [Muse's designers](https://introducing.muse.ai/)
describe a continuing main chat with optional side chats and use message bubbles
to make out-of-turn updates and several consecutive messages readable. Gator's
prototype now explores those thought boundaries for both participants, with
files and reviews beside the message rather than nested inside a speech bubble.

## One primary conversation, optional topics

Use one **persistent primary conversation with Gator** as the default. A user
can greet it, change plans, delegate work and return to decisions in that familiar
place. Recent named topics remain available for work that benefits from its own
space. Starting a separate topic is optional; it is not required for every ask.
A user may choose to resume a specific topic on launch. Existing independent
conversations remain separate.

Do not concatenate every conversation into an infinite model prompt. Personal
continuity comes from scoped memory, linked responsibilities and explicit source
selection. Sensitive topics and distinct folders retain their boundaries. A
user can see and change what a topic can access. A general conversation can link
to delegated work without silently acquiring that work's permissions.

The persistent primary thread identity, topic-to-goal links and background-result routing
need durable application state. A cosmetic single-thread shell cannot establish
those guarantees. Migration must preserve existing IDs, branches and history;
offer existing conversations in recent topics rather than rewriting their text.

## Surface-by-surface decisions

| Current surface | Proposed experience | Keep, change or retire | Required product contract |
| --- | --- | --- | --- |
| Home prompt plus Overview cards and rotating starter dashboard | First visit: a short introduction and one place to type. Returning: the familiar conversation, with a concise catch-up only when there is something useful to report. | Retire the home dashboard as the default front door; reuse its real attention data for updates. | Empty, returning, unavailable and disconnected states must be distinct. No fabricated greeting, unread count or progress. |
| Main navigation with six functional destinations | Gator, search and recent topics dominate. “Following” and “Saved” are secondary drawers. Settings stays at the bottom. | Replace the current navigation hierarchy; retain deep links during migration. | Every old capability remains reachable without memorizing commands. Keyboard and screen-reader navigation work. |
| Recent conversations grouped by filesystem paths | Human topic titles, last useful update and genuine attention state. Folder grouping is optional for coding work. | Replace mandatory workspace grouping. Keep search, reading position, draft recovery and stable IDs. | Different scopes cannot be merged because their labels look similar. Rename/archive/undo need saved state before appearing as working controls. |
| Chat transcript | Short natural exchanges; work, output and requests are contextual cards within the exchange. | Keep readable prose, citations, copy, streaming and anchored scrolling; simplify report-heavy presentation. | Long answers remain possible. “Done” must mean the promised result exists. System errors use plain language and retain technical details. |
| Composer with model identity and slash-command prompt | “Message Gator…”; a small attach action and unobtrusive conversation options. Messages continue reaching Gator while delegated work happens. | Change the emphasis. Retain send/stop and IME handling. Move execution-queue mechanics behind the conversation. | Requires separation of conversation turns and work execution. Attach must really intake files. Voice remains absent until it works. |
| Ongoing goals and plan editors | “Following”: the few things the agent has agreed to keep track of, what happens next, when it checks, and whether it can currently run. Requests start in conversation; detailed editing remains available. | Combine goal and follow-up discovery; retain separate goal/check records underneath. | A goal is not automatically monitored. Clearly distinguish a saved intention from an enabled check. |
| Inbox and Overview attention sections | Useful updates return to the originating topic; a small cross-topic updates list helps find requests and results. | Retire Inbox as the default place users must remember to check. Retain an activity archive. | Stable event IDs, origin links, read/resolved state, deduplication and review authorization. Reading a message does not approve an action. |
| Follow-up checks configuration | A conversational setup review: “Watch these sources, until this date, tell me when this changes.” A compact details sheet allows precise editing. | Keep the scheduler; replace command-like setup as the ordinary path. | Save and confirm actual source scope, schedule/time zone, expiry, interruption preferences and local availability. |
| Saved knowledge | “What Gator remembers”, with source and scope, reachable from conversation information and Saved. Correct or forget specific items. | Reframe the existing knowledge surface around user comprehension. | Explain whether removal affects future retrieval, stored history, snapshots or external data. Do not promise global erasure. |
| Right work-details rail | Hidden by default. Open the selected file as a preview; open conversation information for sources, memory and connections. | Split file preview from diagnostics. Remove the unmeasured context-capacity section from the ordinary personal-agent surface. | Show verified artifact states and actual access state. Never relabel selected connection IDs as authenticated accounts. |
| Files and proposed outputs | Compact attachment cards in chat; click for a readable preview; ask for revisions in the same topic. Saved provides a cross-topic index. | Reuse artifact verification/delivery data; change how users encounter it. | Distinguish generated draft, checked file, proposed change and externally delivered item. Preserve revision/source links. |
| Approvals and external actions | A short question with the recipient, action and exact content available for review. Edit, decline or authorize in context. | Keep canonical reviews; redesign their presentation. | No ambiguous “Looks good” action that silently sends. Approval applies to the reviewed action digest, not later edits. |
| Connections and environment | Ask for access when a real request needs it. Settings manages all accounts, scopes and revocation. Environment is a secondary “Runs on this computer” detail. | Remove infrastructure choices from the ordinary chat journey. | Explain unavailable capability and the minimum access needed. Connecting an app is separate from selecting it for a request and approving an action. |
| Settings, usage, data, privacy and Recovery | Calm conventional settings; advanced details are searchable. Recovery explains the specific problem and a safe next action. | Keep these screens; change terminology and grouping where needed. | Spend limits and destructive data controls remain precise and reviewable. Never bury a limit or an urgent access problem for visual simplicity. |
| Update/version information | About & updates, accessible from the application menu/settings; an understated notice when an explicitly requested download is ready. | Keep release mechanics outside the conversation transcript. | No forced restart, background interruption or claim that in-memory work automatically resumes. |

## Three journeys that decide whether this is better

### 1. First ten minutes

User opens Gator and asks: “Help me keep my portfolio launch moving.” Gator
asks one useful next question. If model setup is required, explain that plainly,
preserve the request and return to it. The user chooses a source only when it is
needed. Gator proposes a bounded responsibility in normal language, then shows
the actual saved scope for review.

Success: the user gets a useful exchange without being taught goals, workspaces,
specialists, context windows or cron. Do not simulate the conversation with a
scripted model response in production.

### 2. Return after a day

The same topic opens at the retained position. If a configured check actually
ran, Gator says what changed, why it matters and the decision needed. A file is
one click away. If this computer was asleep, say the check did not run and offer
the real next action. Do not backfill a reassuring status based on elapsed time.

Success: the user can understand the update and act without navigating between
Overview, Inbox, History and Follow-up checks.

### 3. Change priorities while work happens

User says: “Actually, keep it under $2,000.” Gator visibly distinguishes guiding
current work from taking on a separate request through its response. Clear
corrections go to the relevant work only after acceptance; ambiguous messages
get a short clarifying question. Users should not routinely choose between
“Queue” and “Guide”. Execution queues remain inspectable in activity. Other
topics remain readable; limited execution concurrency should not freeze reading
or the ability to ask Gator a question.

Success: the user can see whether the correction affected the current task and
what is still waiting. Backend ownership and authority stay explicit.

This is an application change, not a rename of the current queue buttons. The
existing single foreground Work loop and captured guidance/queue contracts must
be reconsidered where they prevent conversational availability. Accepted messages
need durable identities and acknowledgment; a queued job is not the same thing
as a delivered conversation message. The prototype demonstrates the desired
surface without claiming that concurrency or routing is implemented.

## Conversation behavior is part of the product

A messenger shell needs the right turn-taking behavior. A greeting should be a
normal exchange; a small correction should update the relevant discussion
without requiring the user to create a goal. An ongoing responsibility should
lead to a precise, reviewable agreement about what will happen next.

Ask one useful clarifying question when it is enough. Keep detailed plans and
execution traces available when needed. A brief progress update should refer to
real work; a friendly tone cannot substitute for knowing whether a check ran.
For example, “I couldn't check availability while this computer was asleep” is
a more useful return message than an unchanged green status.

The primary conversation can coordinate several responsibilities, while actual
work keeps separate task/source boundaries. Cross-topic summaries need explicit
links and permitted context. A copy of a model's long transcript is insufficient
for a reliable personal memory or commitment ledger. Implement and evaluate this
behavior with the conversation surface, rather than treating it as a final coat
of copywriting.

## What to throw away, and what earns its place

Delete or replace the default `OverviewCards`/`StarterDashboard` composition,
mandatory workspace grouping in `RecentConversations`, repeated status/report
boilerplate, the placeholder context-capacity disclosure and duplicate navigation
paths once replacements cover their real tasks. Prior implementation effort is
not a reason to keep those user-facing structures.

Retain the difficult guarantees: durable conversations and branches, verified
artifacts, scoped memory, review digests, bounded checks, lease/usage controls,
saved drafts, keyboard access and recovery. Their presentation can be rewritten.
No visual component needs to survive merely because it already exists.

Do not turn every setting into a conversation. Native file selection, account
sign-in, an exact schedule, usage limits and deletion reviews often work better
as focused controls. Chat should help users reach the right control without
forcing them to describe a form in prose.

## Boundaries before production implementation

Two product decisions need more than a new layout:

- **Model onboarding.** The current app requires provider/local-model setup.
  A consumer should be able to write a first request before learning provider
  IDs or API-key terminology, then get a clear, accurate setup path that keeps
  the request. Decide whether Gator stays bring-your-own-model or eventually
  offers managed access. Do not label an API-key form as account sign-in when
  no supported sign-in exists. Naming/customizing Gator can be optional later;
  an avatar wizard should not block the first useful exchange.
- **Availability.** The current local host cannot offer the same availability
  as a hosted personal agent. Show actual last checks and missed checks, and
  explain when this computer must be awake. Phone access and an always-on remote
  option are product opportunities with real authentication/execution work.
  A green presence dot cannot supply those capabilities.

**Can change now with existing behavior:** simplify navigation and language;
reduce dashboard prominence; show topic titles before folders; hide diagnostic
details; improve artifact previews; move model controls; preserve existing forms
as secondary screens. Existing read/search/queue/draft functions stay functional.

**Needs application work:** preferred conversation on launch, topic-to-goal and
result links, durable cross-topic attention, inline resolution of requests,
attachment intake, scoped memory correction, shared goal/check presentation and
conversational setup that saves a reviewed definition, plus conversation delivery
that remains available while work runs. Build these vertically:
one responsibility through the complete journey, with no mock backend states.

**Later experiments:** voice notes, a lightweight quick-capture window, phone or
messaging access and remote execution. These need evidence of demand plus clear
availability, authentication and data boundaries.

## Prototype and evaluation

Open [the prototype](prototypes/conversation/index.html). Try returning to the
offsite topic, opening its draft, reviewing the proposed email, changing topics
with an unsent message, looking at Following and Memories, and the first-visit
and computer-asleep scenarios. All data and confirmations are local examples.

Compare this proposal with the current app using the same tasks and at least
three new users. Record unaided completion, wrong turns, whether the user can
explain what is remembered/authorized/running, usefulness of the first result,
and whether the interaction feels approachable. Ask “What happens next?” after
each handoff. A warmer visual impression alone is insufficient.

Suggested delivery: first the conversational shell and preserved topic browsing;
then one real responsibility with an inline result/request; then memory and file
intake. Each slice gets its own rollback commit. Avoid a release that changes the
navigation while leaving the core journeys to future backend work.
