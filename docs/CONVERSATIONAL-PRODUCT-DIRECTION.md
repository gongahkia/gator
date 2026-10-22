# Gator as a personal agent you message

Research and implementation constraint: 22 October 2026.

Gator's intended audience wants an ongoing relationship with a useful personal
agent. Messaging is the main product surface. Goals, files, connections and
activity explain or support that conversation. This refines the
[personal agent direction](PERSONAL_AGENT_DIRECTION.md) and retains the
[desktop-primary decision](DESKTOP-PRIMARY.md).

## References and what they establish

These are observations from first-party documentation, not a hands-on review of
every installed app. Desktop availability, cloud execution and messaging-channel
support are separate claims. Product recommendations below are our synthesis.

| Reference | Documented behavior | Application to Gator |
| --- | --- | --- |
| [Dots](https://learn.chatgpt.com/docs/dots) | A continuing personal agent; users can keep talking while tasks run and receive results or requests for decisions. Activity is inspectable separately. | Keep a familiar conversation easy to return to; show background results and requests with enough context to respond. |
| [Muse](https://ai.meta.com/muse/) and [announcement](https://about.fb.com/news/2026/09/introducing-muse-personal-ai-agent/) | Personal assistance presented through messaging; ongoing goals and tasks can continue in its hosted environment. | Favor everyday language, a recognizable agent identity and short, useful updates. Gator must explain its local availability accurately. |
| [Poke](https://poke.com/docs) | Email, calendar, reminders and search requested through existing messaging apps. | Minimize setup before the first useful exchange; introduce connections when a request needs them. |
| [Lindy](https://www.lindy.ai/product) | Conversational requests, editable memory, routines, drafts and follow-up nudges across communication channels. | Let users correct preferences and revisit commitments; make approval and follow-through legible in conversation. |
| [T3 Code threads](https://github.com/pingdotgg/t3code/blob/main/docs/user/thread-sidebar.md) | Separate threads, reversible archive/snooze actions, retained drafts and stable sidebar scroll positions. | Borrow predictable conversation navigation and reversible organization. Repository/worktree controls are relevant to coding tasks only. |
| [OpenHands Agent Canvas](https://github.com/OpenHands/docs/blob/main/openhands/usage/agent-canvas/overview.mdx) | Conversations with inspectable files, tools and execution environments; native desktop is documented as a preview. | Clearly distinguish where work runs and let users inspect its evidence. Environment management belongs in secondary details. |

Poke and Lindy are useful additional references for the interaction model even
though their messaging surfaces are not equivalent to a standalone Electron app.

## Product rules

1. **Make talking the obvious action.** A fresh user should see a composer and
   enough guidance to begin. Returning users should easily resume a familiar
   conversation. Choosing a repository is unnecessary for everyday assistance.
2. **Keep continuity visible.** A result should say what request it belongs to,
   what changed and what needs the user's attention. Changing screens must
   preserve unsent messages and reading position.
3. **Use progressive disclosure.** Files and concrete requests belong beside the
   conversation; logs, token accounting, environment configuration and technical
   provenance are available on demand. Keep the details rail closed by default.
4. **Ask naturally and precisely.** Connection requests and approvals explain the
   actual next action. A conversational presentation retains the existing
   authority, recipient, scope and approval checks.
5. **Make the agent approachable.** Use the alligator identity, readable prose,
   considerate empty states and restrained motion. Personality must not invent
   progress, memory, availability or human-like feelings.
6. **Respect attention.** Return for useful results, genuine blockers and
   decisions. Read/unread, dismiss and notification controls need durable state
   before the UI promises them. Avoid decorative dashboard counters.
7. **Keep the local promise honest.** Foreground work stops when Gator closes;
   configured background checks require an online host and running supervisor.
   Do not imply cloud availability, phone access or device synchronization.

## Feature opportunities in order

The current implementation already has recent conversations, saved drafts,
ongoing goals, an Inbox, reviewed actions, follow-up checks and compact work
details. This list identifies what to connect or validate, rather than claiming
those underlying capabilities are missing.

| Priority | Experience | Existing foundation | Work needed before promising it |
| --- | --- | --- | --- |
| 1 | Return to my conversation with Gator | Durable conversations, recent list, drafts | Decide and test resume-on-launch behavior and privacy expectations; do not silently send recovered text. |
| 1 | See what Gator needs from me, reply in place | Inbox, approval records, goal-linked work | Link each result/request to its originating conversation; retain read and resolved state, avoid duplicate delivery. |
| 1 | Give Gator a continuing responsibility in plain language | Goals, plan suggestions, bounded follow-up checks | Review scope, timing and source access conversationally; confirm what was actually saved and when checks can run. |
| 2 | “What do you remember about this?” and “Forget that” | Saved knowledge and retrieval provenance | A clear review/correction flow with explicit scope and truthful deletion semantics. |
| 2 | Share a file or screenshot as naturally as a message | Selected sources and verified output metadata | Attachment intake, permissions, previews and supported-format feedback; clipboard/drag-and-drop must use host-owned file access. |
| 2 | Preview a draft and ask for a revision in the same exchange | Retained artifacts and verified delivery states | Connect artifact preview/revision to the right conversation and preserve the draft-versus-delivered distinction. |
| 3 | Talk or send voice notes | Text conversation and guidance queue | Capture permissions, transcription, cancellation and cost behavior; no inert microphone button. |
| 3 | Reach the same agent from my phone | Durable local state | Authentication, transport, device access and availability design. A desktop wrapper alone does not provide this. |

Success measures: first useful exchange without help, successful return to an
ongoing responsibility, time to answer a request for input, corrected facts,
missed/irrelevant follow-ups, and user-reported comfort. Test with at least three
new users before expanding navigation or adopting competitors' full feature sets.

## Effect on the current engineering work

- Keep update controls quiet and discoverable in an application dialog/menu.
  Check and download only when requested; install only after review and when
  active work and unsaved forms are protected.
- Keep the desktop host and shared application model independent of obsolete
  terminal product names. Preserve saved profiles and application behavior.
- Refresh current setup/evaluation instructions after CLI retirement; distinguish
  historical audit records from executable developer instructions.
- This research does not change the agent loop, merge independent conversations,
  enable messaging services, add voice, or claim new background availability.
  Those experiences need their own implementation and acceptance checks.
