# Gator competitive gap audit — 22 October 2026

Repository baseline: `c7eaea64e`. Scope: product and engineering audit of the
current desktop application, its Go runtime, tests, release configuration and
recorded evaluation evidence, compared with current first-party product docs.
This change records findings and proposed work; it implements no product feature.

## Product intent

Gator is intended to compete for the same everyday responsibilities as
**ChatGPT Work, Claude Cowork, Hermes Agent, Claude Duet, ChatGPT dots, Grok Bot
and Meta Muse**. The desired experience is a capable personal agent a person
messages: it knows relevant preferences, handles useful work, follows through
over time, and returns with results or a decision the person needs to make.

Retain the decisions already made for this product:

- One Gator identity, multiple independent chats, and shared personal memory
  within the local profile. A new chat refreshes working context without erasing
  the person’s saved preferences. Task access remains scoped.
- The desktop application is the primary interface. Keep its current visual
  conventions, command palette, compact chat, inline files and progressive
  disclosure. New capability should usually appear through conversation first.
- Following explains responsibilities and their progress; Saved helps retrieve
  information and work. Neither should become a required control center for an
  ordinary request.
- Prefer complete, useful experiences over preserving an existing abstraction.
  Reuse good execution, provenance and review primitives; replace orchestration
  or screens when they prevent the intended experience.

**Assessment:** Gator has substantial local execution infrastructure and a much
improved chat surface. Its largest competitive gaps concern coordination,
activation, continuity of delivery and demonstrated usefulness. Visual imitation
alone will not close them. Provider choice and local ownership are useful assets,
but need to support a product people can use without operating its machinery.

## How to read this audit

This is a source inspection and documentation comparison, not a new hands-on
benchmark of the competing applications or a full manual tour of Gator. Relevant
implementation and test paths were inspected; existing test results are identified
as recorded evidence, not presented as newly executed tests. No private profiles,
credentials, connected accounts or paid model calls were used.

- **Implemented:** a reachable implementation exists, within stated limits.
- **Partial:** useful components exist, but the complete user journey does not.
- **Absent:** no corresponding supported workflow was found in the inspected code.
- **Unproven:** code/tests exist, but product quality or operational evidence is
  insufficient. This is not the same as a missing feature.
- **Choice:** a deliberate boundary whose user benefit and competitive cost need
  an explicit product decision.

Priorities: **P0** establish evidence before broad claims; **P1** improve the core
daily experience or gate an external beta; **P2** deepen validated workflows;
**P3** expand reach after architecture/product decisions. These are recommended
orders, not delivery dates. Source findings are stronger than inferred user impact;
the latter needs observation with real users.

## Competitive references and what to borrow

Names here refer to the actual products below: Hermes means Nous Research’s
Hermes Agent, dots means OpenAI’s ChatGPT dots, and Muse means Meta’s personal
agent. First-party documentation was opened on 22 October 2026. Documented
features establish expectations, not measured superiority or universal access.

| Reference | Verified product expectation | Implication for Gator |
| --- | --- | --- |
| [ChatGPT Work](https://learn.chatgpt.com/docs/get-started-with-work) | Delegated work produces reviewable files and workflows, uses connected tools, and can be steered. Eligible synced tasks continue across devices; local steps still require the connected computer. | Judge a task by its useful result and review loop. Keep environment details secondary. |
| [Claude Cowork](https://claude.com/product/cowork) | Selected files/tools, an integrated browser, parallel task components, scheduling and review. The page now says Cowork is becoming “Claude,” with rollout qualifications. | Borrow the short path from intent to finished work, and integrated recovery when access is missing. This is a moving product, not a fixed desktop screenshot. |
| [Hermes Desktop](https://hermes-agent.nousresearch.com/docs/user-guide/desktop), [memory](https://hermes-agent.nousresearch.com/docs/user-guide/features/memory) | Desktop uses the shared agent runtime; simultaneous conversations, persistent queues and remote gateway options are documented. Memory and searchable past sessions are separate systems. | This is a relevant engineering reference for an open, configurable runtime. Preserve Gator’s simpler surface rather than adopting every management pane. |
| [Claude Duet](https://support.claude.com/en/articles/17631495-get-started-with-claude-duet) | A main messaging thread and separate channels share memory. Cloud work, morning briefs, quiet task tracking, connected tools and reviewable files are documented. | Useful proactive follow-up belongs in the conversation. Duet is a US Pro/Max waitlisted research preview; its docs explicitly limit browser takeover, local-file access, connectors and some actions. |
| [ChatGPT dots](https://learn.chatgpt.com/docs/dots), [tasks and memory](https://learn.chatgpt.com/docs/dots/tasks-and-memory) | Ongoing responsibilities, background delegation while conversation continues, relevant cross-chat context, and cloud execution. Timing and notification conditions can be specified. | Separate the continuing relationship from individual tasks and schedules. Device-off execution is a real infrastructure difference. Availability is a gradual rollout. |
| [Grok Bot](https://docs.x.ai/grok-bot/overview), [skills and routines](https://docs.x.ai/grok-bot/skills-routines-and-automations) | Persistent bots work on a cloud computer, coordinate across tasks, and can turn a successful workflow into a reusable skill/routine. Demonstration-based teaching is availability-dependent. | Borrow reusable workflows and task coordination. A visible collection of different bots is optional for Gator, whose chosen identity remains singular. |
| [Muse announcement](https://about.fb.com/news/2026/09/introducing-muse-personal-ai-agent/), [design account](https://introducing.muse.ai/) | A messaging relationship, side chats, ongoing goals, selective proactive messages, inspectable activity and editable memory. Cloud work and rich outputs are central. | Borrow the low-friction conversation and high bar for interruption. Personal warmth can come from helpful continuity and thoughtful language, without copying avatars or adding dashboards. |

Do not flatten these into one imaginary competitor with every feature. In
particular, Duet documents interval-based checking and no browser takeover,
whereas other references describe different browser and event workflows.
Gator need not adopt push subscriptions or unrestricted actions to deliver
useful follow-up.

## What Gator already has

These should not reappear as “build from scratch” items in the backlog.

| Existing capability | Implementation evidence | Boundary |
| --- | --- | --- |
| Independent chats with shared identity | [active chat persistence](../internal/worksession/active.go), [chat transitions](../internal/workapp/conversation.go), [desktop chat tests](../desktop/tests/e2e/shared-chats.spec.ts) | New chat, last selected chat and separate drafts exist. One foreground desktop run still blocks switching. |
| Shared personal memory and earlier-chat retrieval | [chat memory tools](../internal/tools/chat_memory.go), [memory store](../internal/memory/chat.go), [chat history tools](../internal/tools/chat_history.go), [context selection](../cmd/gator/work_service.go) | Quoted user evidence, scoped recall, corrections, forgetting and learning controls exist. Recall is selected and bounded. |
| Goals and recurring work | [follow-up setup](../internal/followup/setup.go), [saving checks](../cmd/gator/work_followup.go), [job runtime](../cmd/gator/command_job.go), [connected watch](../cmd/gator/connected_goal_watch.go) | Durable local scheduling, watched inputs, attempts, recovery, quiet decisions and an inbox. Host availability remains necessary. |
| Real files in chat | [file surface](../desktop/src/FileSurface.tsx), [file versions](../desktop/electron/files.mjs), [Office editor](../desktop/electron/office.mjs), [native file tests](../desktop/tests/e2e/inline-files.spec.ts) | Text/Markdown/CSV and bounded Office editing, PDF/image previews, versions and export exist. Original uploads and sealed output remain intact. |
| Attachment and voice input | [uploads](../desktop/electron/uploads.mjs), [image paste](../desktop/electron/pasted-images.mjs), [voice controls](../desktop/src/VoiceControls.tsx) | File picking/drop/paste and dictation exist. Dictation plus spoken replies is not a live voice conversation. |
| Connected tools and exact action review | [connector definitions](../internal/connector/connector.go), [action broker](../internal/action/action.go), [Google operations](GOOGLE_WORK.md) | Bounded first-party operations and remote MCP, provenance and payload-bound reviews. Catalog, setup and action breadth remain limited. |
| Internal delegation and provider choice | [specialists](../internal/workrun/subagents.go), [orchestrator](../internal/orchestrator/supervisor.go), [model factory](../internal/model/provider_factory.go), [local models](LOCAL_MODELS.md) | Real bounded specialists, cloud adapters and local Ollama support. These do not provide independent conversational task lifecycles. |
| Recovery, budgets and release foundations | [lifecycle](../internal/lifecycle/restore.go), [usage](../internal/usage/store.go), [updates](../desktop/electron/updates.mjs), [release workflow](../.github/workflows/release.yml) | Recovery and accounting exist; signed distribution and native platform evidence are separate release gates. |

## Gap register

### G01 — Establish that the agent reliably helps: P0, unproven

The repository contains substantial deterministic and scripted coverage. Its
[50-task Work corpus](USER_WORK_V4_BENCHMARK.md) has no recorded live v4 pass rate.
The [continuity benchmark](PERSONAL_CONTINUITY_BENCHMARK.md) records attempts
limited by provider failures; the matched smoke attempt has no pair in which
both arms completed without provider failure. The
[cross-source benchmark](CROSS_SOURCE_GOALS_BENCHMARK.md) and
[pilot procedure](PROJECT_FOLLOWUP_PILOT.md) are foundations, not proof of
accepted useful follow-up. Recent packaged UI checks demonstrate concrete UI
behavior, not long-term agent judgment.

**User consequence:** a polished, technically correct application can still
miss a deadline change, recall the wrong preference, or produce a document that
takes longer to repair than to make.

**Next work / acceptance:** run a small matched, live development set with a
fixed provider/model and sufficient quota, then an unseen set. Separate provider,
tool, policy and task-quality failures. Blind-review useful completion, factual
support and manual rework. Follow a few bounded real responsibilities over
multiple days, including days when no notification should be sent. Publish
sample sizes and denominators; no composite “parity percentage.”

### G02 — Keep talking while work proceeds: P1, partial

[App.tsx](../desktop/src/App.tsx) disables New chat, recent-chat navigation and
Settings during `state.running`. [OpenBridgeConversation](../internal/workapp/bridge_conversations.go)
also rejects switching during work. [Backend](../desktop/electron/backend.mjs)
owns one Go UI session; [command availability](../internal/workapp/commands.go)
permits only a narrow set of active-run commands. Queue and Guide now are real
controls, but a queued question waits behind the current run. Internal parallel
specialists do not remove this constraint.

**User consequence:** “Keep preparing the deck; separately, what time is my
meeting?” cannot behave like two independent requests in this window.

**Next work / acceptance:** first decouple read-only navigation from execution.
Then give tasks durable identities and independent lifecycles, with explicit
message-to-task routing and shared resource limits. Start a slow task, open
another chat, get a short answer, return and steer the original task without
losing progress. A late result or approval must reach its original task. Merely
removing disabled attributes would be incorrect. Reuse Work execution and
approval services; the UI session should stop owning all active work.

### G03 — Deliver follow-ups back into conversation: P1, partial

[LibraryDrawer](../desktop/src/LibraryDrawer.tsx) sends users to Updates, goals,
checks and Activity & attention. [Inbox detail](../internal/workapp/inbox.go)
shows retained summaries with read/rating controls. There is no general durable
delivery path that inserts a background result into its originating chat and
binds a reply to that responsibility. The current implementation document
[identifies this remaining application work](DESKTOP-CONVERSATIONAL-IMPLEMENTATION.md#truthful-state-and-the-remaining-application-work).

**User consequence:** useful updates can feel like a second application the
person must remember to check. “Do the second option” needs a clear referent.

**Next work / acceptance:** retain origin chat/task IDs, update IDs, delivered
state, read state and resolution state separately. Present a compact, linked
update in chat and keep deeper evidence on demand. Replies should resolve to
the right task; ambiguous replies should ask a short question. Restarting or
opening two windows must not duplicate the update or execute a decision twice.
Reading an update must never imply approval.

### G04 — Turn a normal request into a continuing responsibility: P1, partial

[suggest_change](../internal/tools/suggestion.go) can propose a goal, plan and
schedule for review. [Follow-up forms](../internal/workapp/followup.go) can save
scope, selected sources, timing and alert conditions. These are useful existing
paths, but users still traverse separate suggestion/goal/check surfaces and
may need to start the supervisor separately. An ordinary question does not
automatically become a durable commitment.

**Next work / acceptance:** for “Keep an eye on the venue replies until Friday,”
produce a compact reviewed agreement in chat: responsibility, sources, end date,
when to check, what deserves interruption and permitted actions. Save the actual
records and show current availability plus the next check. “Pause that” and
“Only bother me if the price changes” must target the correct record with stale
edit protection. A local-host dependency must remain visible when relevant.
Retain a form as an optional precise editor.

### G05 — Reach first value without developer setup: P1, partial

Chat/model setup exists, but [Google setup](GOOGLE_WORK.md#user-owned-google-desktop-oauth-app)
requires a user-owned OAuth application and enabled APIs. The
[connector command path](../internal/workapp/commands.go) asks for a client ID,
and [App.tsx](../desktop/src/App.tsx) retains an interactive Terminal handoff for
some connection steps. The [settings categories](../desktop/src/SettingsNavigation.tsx)
do not directly expose every everyday control, including connections and memory.

**User consequence:** someone who just wants meeting help is asked to understand
model credentials, OAuth, source selection and background-service readiness.

**Next work / acceptance:** design a short first-use path around one task;
explain model choice and data processing plainly, offer a working supported
connection flow, verify the selected account and recover inline from expired
access. A managed OAuth app requires provider verification and operational
ownership; it is not just a prettier client-ID field. In an observed trial,
someone unfamiliar with the repo should connect a supported account and finish
the first task without a terminal or developer assistance. Keep advanced
bring-your-own-provider configuration available.

### G06 — Make shared memory dependable and understandable: P1, implemented + unproven

Do not build a second memory system. The current one already records exact user
quotes, canonical subjects, scope, origin, guarded corrections and forgetting.
The foreground capsule selects up to four basic preferences/profile facts;
other memory uses bounded retrieval. Earlier-chat search scans bounded pages
using words of at least three characters. These limits protect context size,
but do not establish recall quality. See [memory tools](../internal/tools/chat_memory.go),
[history retrieval](../internal/tools/chat_history.go) and
[context selection](../cmd/gator/work_service.go).

**Remaining gap:** useful memory selection, temporal corrections, paraphrase
recall and user comprehension across many chats. Learning off stops automatic
saves, not existing recall. Forgetting saved memory retains historical chats;
therefore older text can still be encountered through history. That is an
existing documented boundary, not complete erasure.

**Next work / acceptance:** surface a quiet “remembered” receipt with source and
edit/undo where useful; test corrections across new chats, restart and older
history, including conflicting dates, quoted third-party text and other-goal
exclusions. Clarify separate controls for learning, historical recall and deletion;
consider a private chat option with explicit semantics. Multilingual intent
needs attention: current explicit remember/forget gates use English patterns.
Do not remove scope limits just to make recall appear broader.

### G07 — Earn proactive interruptions: P1, partial + unproven

Gator already has unchanged-input skipping, duplicate suppression, model-chosen
quiet results, quiet hours, urgency options and feedback. See
[watch changes](../cmd/gator/goal_watch_changes.go),
[notifications](../cmd/gator/goal_notifications.go),
[quiet review](../internal/jobs/quiet_review.go) and [jobs](JOBS.md).
Gmail/Calendar watches poll selected bounded sources; the Gmail cursor is an
optimization, not a push subscription. Reference integrity is not proof that a
claim follows from the evidence.

**Next work / acceptance:** evaluate changed and quiet days with ground truth.
Measure useful-alert precision, missed material changes, delay, repeat alerts
and actionability. Let users correct notification conditions in chat and see
when a check failed rather than interpreting silence as success. Start with
one valuable responsibility. Adaptive wake times and broader unsolicited
research should follow measured benefit; increasing notification volume is not
progress.

### G08 — Finish useful connected workflows: P1/P2, partial

[Connector operations](../internal/connector/connector.go) cover Google,
Slack, Atlassian, Notion and typed remote MCP alongside generic transports.
[Google](GOOGLE_WORK.md) can read selected mail/calendar information and mutate
Docs/Sheets. Its Gmail send is deliberately narrow: one plain-text message to
one recipient with foreground approval. Calendar writes and broad mailbox
management are not provided by that connector. A general MCP transport does
not itself establish a supported CRM or complete email/calendar experience.

**Next work / acceptance:** choose complete user jobs before adding logos.
Examples: a source-linked meeting brief and reviewed follow-up; monitor a
project thread and draft a response; revise a document and deliver it to the
chosen destination. Add calendar changes or richer email only when demanded by
those jobs, with account/scope checks, accurate previews and provider receipts.
For a timed-out send, preserve “unknown” and reconcile rather than silently
retrying. Run real-account tests with controlled recipients and reversible data.

### G09 — Recover interrupted work without making users reconstruct it: P1/P2, partial

There are durable conversations, task records, leases, scheduler reconciliation,
file versions, drafts and [recovery/export services](../internal/lifecycle/restore.go).
However, [the desktop connection](../desktop/src/App.tsx) explicitly starts a
new local session on reconnect; it does not resume or resubmit work. The queue
in [bridge_queue.go](../internal/workapp/bridge_queue.go) belongs to the current
UI model. [Update documentation](DESKTOP-RELEASES.md#preserving-work) does not
promise active-run or queue recovery after restart.

**Next work / acceptance:** persist admitted user messages and task status;
distinguish waiting, running, interrupted, needs input, completed and uncertain
external outcomes. Recover queued text with a deliberate resume/discard choice.
Kill the app/backend at each transition, restart and show what remains to do
without duplicate execution. Handle expired auth and provider outage with a
specific next step. Reusing the last transcript alone is insufficient to restore
a live operation; resume needs a safe checkpoint or a reviewed fresh attempt.

### G10 — Make documents feel like finished work: P2, partial

Inline preview/edit/version/export is already implemented. The actual limits
are visible in [office.mjs](../desktop/electron/office.mjs): Word edits text,
not full page layout/tracked changes; Excel edits cells/formulas but relies on
Excel to recalculate; PowerPoint edits existing shape text with a simplified
preview. PDF/images are preview-only. HTML is displayed as source through
[file-content.mjs](../desktop/electron/file-content.mjs). Composer uploads are
limited to four files, 4 MiB each and 8 MiB combined.

**Next work / acceptance:** use representative customer files to select the next
depth improvement: faithful previews, larger bounded intake, comments/selected
edits, trustworthy formula results, or isolated interactive artifacts. A user
should inspect a result, request a precise change, compare versions and export
the intended version without losing unsupported parts. Test native-app opening,
layout and manual rework. Safe interactive rendering requires a separate sandbox
and permissions design; do not run generated HTML in the privileged app renderer.

### G11 — Make web work practical: P2, partial + choice

[Browser tools](BROWSER.md) already support selected local Chromium tabs,
snapshots, screenshots and controlled actions. The user must install the
Playwright/Chromium runtime with external Node/npm, configure allowed origins
and select a session. Managed profiles are temporary. Navigation, click and
fill each require fresh approval. There is no integrated persistent browser
computer or general native-app computer-use workflow.

**Next work / acceptance:** choose one supported web job that APIs do not cover.
Provide install/readiness, visible browser state, private sign-in, user takeover
and a clear return-to-agent action. Review whether a narrow, host-enforced
browsing grant can reduce repetitive prompts while preserving consequential
action review. Test cross-origin login, downloads, expired sessions and site
blocking. Do not promise every website works or remove existing policy checks
as a cosmetic simplification.

### G12 — Find and organize work after months of use: P2, partial

[Recent chats](../internal/workapp/bridge_conversations.go) exposes up to 24
entries. [All conversations](../desktop/src/ConversationsPanel.tsx) searches
titles/folders in up to 50 recent entries, as supplied by
[work_interactive.go](../cmd/gator/work_interactive.go). The model’s earlier-chat
search is a separate capability. [Saved](../desktop/src/LibraryDrawer.tsx)
collects the current conversation’s outputs, not a global file library.
No ordinary rename/archive/pin workflow was found in these chat surfaces.

**Next work / acceptance:** provide paginated history, user-controlled titles,
reversible archive/pin, cross-chat text search and a scoped output index.
Search for an old decision or file among hundreds of chats, open its source,
return to the current draft, and preserve position. Search results must distinguish
chat text, remembered facts, files and task status. Do not replace the existing
command palette with an overloaded global search control.

### G13 — Ship an install people can trust and maintain: P1 release gate, partial

Release and update engineering exists. The [release record](DESKTOP-RELEASES.md)
states that Apple/Windows signing credentials are not configured, and no signed,
notarized published release is established by that work. The
[Windows implementation](WINDOWS.md) is explicitly a preview with native
verification pending. Windows background checks are implemented, but native
toast notifications and strict command sandbox/terminal capabilities have
separate limitations. [CI](../.github/workflows/ci.yml) defines tests; a workflow
file is not evidence a remote run passed.

**Next work / acceptance:** obtain signing identities, pass native install,
upgrade, rollback/recovery and uninstall checks, and publish a controlled beta.
Test profiles, unsent drafts and scheduled checks across updates. Current updates
require manually stopping login/background services and enabling them again;
an integrated drain/restart flow would reduce maintenance burden. Verify native
Windows behavior before marketing parity. Account setup remains an external
dependency, not an unfinished CSS task.

### G14 — Work while the user’s computer is off: strategic choice, absent

Gator’s services can keep configured checks running on an available host. They
cannot operate with that host powered off. The desktop bridge launches a local
process and has no supported authenticated remote backend or hosted executor.
The [Windows login supervisor](../cmd/gator/command_job_windows.go) and macOS/Linux
services do not change this physical dependency.

**Competitive impact:** device-off work is a central documented expectation in
[dots](https://learn.chatgpt.com/docs/dots),
[Duet](https://support.claude.com/en/articles/17631495-get-started-with-claude-duet)
and [Grok Bot](https://docs.x.ai/grok-bot/overview). A local-first launch can still
be useful, but cannot claim this form of always-available persistence.

**Decision and acceptance:** choose between a clearly local product, a
self-hosted companion on an always-on machine, and a managed service. A remote
option requires authenticated task ownership, credential placement, execution
isolation, synchronization and operating costs. Close/power off the client and
prove eligible work completes elsewhere; local-only files must be reported
unavailable. A self-hosted server is an incremental option, not equivalent to a
consumer cloud service.

### G15 — Reach the same Gator away from the desktop: P3, absent

There is no supported mobile/web conversation client or inbound messaging
gateway. A Slack connector gives tool access; it is not a channel for messaging
Gator. The private [backend bridge](../desktop/electron/backend.mjs) is not a
remote user API. This is separate from whether jobs can run in the background.

**Next work / acceptance:** after G02/G03 and an execution-location decision,
consider one small remote surface: read updates, answer a decision, send a
request and pause a task. Authenticate the person/device and keep context scoped
to the audience. A private chat’s memory does not authorize disclosing its
contents in a shared channel. Revocation, offline delivery and duplicate message
handling need tests. Multi-device edits cannot share local files by pretending
the filesystem paths are the same.

### G16 — Make voice and personalization support a relationship: P2/P3, partial

[VoiceControls](../desktop/src/VoiceControls.tsx) and
[voice host](../desktop/electron/voice.mjs) implement explicit dictation and
optional spoken replies. This is useful input, but not continuous, interruptible
voice interaction that can steer ongoing work. The alligator identity, memory
and existing styling already provide a starting point for personality.

**Next work / acceptance:** first improve short conversational responses,
context-aware greetings and understandable status through evaluation. If voice
use is frequent, add actual streaming voice with interruption, transcript repair
and task steering. Stopping a call must have clear semantics for ongoing work.
Optional name/style preferences should be inspectable and should never invent
feelings, progress or recall. Avatar customization is lower priority than useful
continuity; retain the approved Gator visual direction.

### G17 — Turn successful work into repeatable skills: P2/P3, partial

Gator already stores notes/skills, can load them and can propose a skill for
review through [suggest_change](../internal/tools/suggestion.go). It also has
[extension infrastructure](EXTENSIONS.md). What is missing from the everyday
surface is a clear “do it this way next time” loop with discoverable reusable
workflows, versions, evidence of successful use and a safe way to test a routine.

The relevant references are [Hermes skills](https://hermes-agent.nousresearch.com/docs/user-guide/features/skills)
and [Grok Bot routines](https://docs.x.ai/grok-bot/skills-routines-and-automations).
Start with a reviewed procedure from a successful Gator task, not a plugin
marketplace. Acceptance: save it, correct it, run it against new inputs, inspect
the result and revert the procedure. A skill is context, not a grant of tool
authority. Workflow recording/demonstration and public distribution can follow
once this simpler loop is valuable.

### G18 — Prove the experience remains usable, affordable and controllable: P1/P2, partial + unproven

[UsagePanel](../desktop/src/UsagePanel.tsx) exposes shared request/token limits
and incomplete-cost qualifications. [Telemetry](../internal/telemetry/telemetry.go)
exports bounded metadata; provider retries and compaction already exist. The
composer’s [Work effort](../internal/workrun/self.go) controls step budget, not
provider reasoning. Existing UI tests cover focus, reduced motion and several
sizes/themes. None of that establishes task economics, native screen-reader
usability, multilingual intent reliability, or performance with months of data.

**Next work / acceptance:** measure time to first useful response, task completion
latency, idle/active CPU and memory, interruption latency, and cost per accepted
outcome. Include low-resource machines, long chats and large libraries. Verify
keyboard/screen-reader flows and quiet neutral text-field focus, retaining the
user’s requested removal of green frames. Keep unknown cost unknown and expose
a way to stop spending. Choose defaults from measured workflows rather than
assuming a bigger model or higher step budget is better. Add per-responsibility
usage summaries only if people need them.

## Trust and collaboration: preserve boundaries, expand deliberately

Gator has real safeguards: private local state, source snapshots, isolated
output, exact-action review, bounded memory provenance and uncertain-action
states. These are valuable foundations, not evidence of complete security.
Before expanding autonomy, include adversarial source instructions, secret
exposure, stale approvals, cross-task disclosure and recovery in the evaluation
set. In particular, literal memory quote matching proves origin, not that the
quoted sentence is an appropriate durable belief.

There is no comparable shared-account team workspace, collaborative live editor,
organization policy service or remote audit administration in the inspected
product. [Cowork](https://claude.com/product/cowork) documents organizational
controls, but enterprise parity is not required for Gator’s first individual-user
release. Likewise, payment wallets, purchases, autonomous financial actions,
bot group chats and unrestricted desktop control are optional future scopes,
not gaps we should fill reflexively. Standing permissions require explicit
host-enforced policies; model prose is not authorization.

## Surface-by-surface consequence

| Surface | Recommended role in the next product iteration | Relevant gaps |
| --- | --- | --- |
| Chat/composer | The place to request, clarify, steer and receive useful results. Keep independent conversation available during work. | G02, G03, G16 |
| Recent chats / Conversations | Independent contexts, recoverable drafts and reliable retrieval over time. Keep New chat and shared memory. | G02, G06, G12 |
| Following | A compact view of actual commitments: current status, next check, what is waiting, pause/edit. The same records should be controllable from chat. | G03, G04, G07 |
| Saved | Distinguish what Gator remembers from files and historical messages, with understandable scope and search. | G06, G10, G12 |
| File cards / inline editor | Keep the approved inline experience; improve fidelity and iteration based on actual files. | G10 |
| Information / activity | Explain what happened, what was read and where a task runs when requested. Avoid mandatory per-reply metadata blocks. | G02, G09, G14 |
| Connections / onboarding | Request the needed account in context; show identity, scope, health and recovery. | G05, G08, G11 |
| Settings / Recovery / Updates | Reliable administration, understandable privacy controls and verified releases; ordinary requests should not require visiting these screens. | G05, G09, G13, G18 |

## Recommended order of work

1. **Establish a repeatable evidence set (G01).** Use three anchor journeys:
   prepare for a meeting from selected sources; own a project follow-up for a
   week; revise a file over several chats while honoring a corrected preference.
   Record the current failures before changing the architecture. Include a
   separate concurrency scenario because the current restriction is structural.
2. **Make messaging and task ownership coherent (G02, G03, G09).** Separate
   task execution from the visible chat, persist message admission, route
   updates/decisions to their origin and recover without replaying side effects.
   This is the first substantial implementation slice I recommend.
3. **Make one responsibility easy to set up and useful (G04, G05, G07, G08).**
   Start with the existing project-follow-up and Google read foundation. Connect
   the real account, review the agreement in chat, deliver useful updates there
   and prove that irrelevant changes stay quiet.
4. **Improve continuity and output iteration (G06, G10, G12).** Use failures
   observed in the same journeys to prioritize recall, file fidelity and search.
   Maintain separate chat contexts and the existing polished visual language.
5. **Gate outside use on release readiness (G13, G18).** Signing setup can proceed
   alongside product work; native distribution and recovery evidence are required
   before inviting general users. Small consented pilots can use labeled builds.
6. **Choose reach intentionally (G14, G15).** Decide remote/self-hosted/managed
   execution before promising device-off work or mobile access. Keep this design
   in view while introducing task ownership, so it can move later without an
   immediate cloud rewrite. Browser, voice and skills depth follow observed need.

## Acceptance scenarios for the first implementation slices

These are proposed product gates, not claims that they currently pass.

| Scenario | Required observation |
| --- | --- |
| “Keep building that; answer this separately” | A second answer arrives without cancelling the first task; steering and results retain their original task IDs. |
| “Watch this until Friday” | One reviewed agreement is actually saved; next check, expiry, sources and availability match the scheduler. |
| Material change arrives during another chat | One linked update reaches the appropriate conversation; a reply resolves against that responsibility. |
| No important change | The agreed check runs, retains evidence and stays quiet; reviewed missed-change cases remain measurable. |
| “My preference has changed” | A new chat/restart honors the correction; conflicting older evidence is not presented as current fact. |
| Account access expires | The product says the check failed, preserves the last successful observation and gives a working reconnect path. |
| App closes after message admission | Queued text and task state remain inspectable; restart does not resend an uncertain external action. |
| File revision across chats | The selected source/version is explicit; useful edits preserve unsupported content and export the intended version. |
| External send times out | Outcome remains uncertain until supported reconciliation; no automatic duplicate send. |
| Laptop is off | Local work is unavailable honestly. Device-off completion is tested only after a remote execution option exists. |

For each live trial retain app commit, exact model/provider, task/source version,
latency, request/token use, failure category and independent outcome judgment.
Use consented/redacted evidence. Existing automated tests remain engineering
gates; they should accompany these user outcomes rather than substitute for them.

## Evidence coverage and limitations

Inspected entry points span React navigation/composer/drawers/search/files/voice,
Electron backend/upload/file/update handling, Go chat state and queue controls,
memory and historical retrieval, goal/suggestion/follow-up paths, job execution,
connector/action boundaries, browser setup/policy, specialist orchestration,
usage/telemetry, native platform/release configuration, and the recorded Work,
continuity and cross-source evaluations. Links above identify the decisive
implementation or limitation for each finding.

This audit does not establish new model success rates, real-account integration
reliability, current remote CI status, native Windows/Linux pass results,
screen-reader support or competitor latency/cost. No source-code changes, live
service mutations, dependency installs or benchmark model requests were made.
Documentation links and the change diff are checked before committing.

Older [competitive audit](COMPETITIVE_AUDIT.md) and
[Muse/dots map](MUSE_DOTS_FEATURE_MAP.md) remain historical evidence. Their
terminal-first wording and missing-feature classifications should not be used
to plan new work without this update. Some older pilot/release notes also retain
terminal-era instructions; updating those to the desktop journeys is part of
G01/G05/G13, not a reason to bring back the retired CLI.
