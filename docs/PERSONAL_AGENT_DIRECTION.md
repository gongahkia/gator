# Gator as a desktop personal agent

**Interface status:** Gator is a desktop app. Shell examples below are source-checkout developer operations with an explicit internal opt-in; see [development instructions](DEVELOPMENT.md). They do not install or enable a public CLI.

**Interface decision, 22 October 2026:** desktop is the primary supported
interface; the standalone CLI and React terminal renderer are retired. See
[Desktop as the primary interface](DESKTOP-PRIMARY.md). Command-line examples
below are historical implementation/maintenance references, not current user
onboarding instructions. In-app slash commands remain available where offered.


## Architecture decision — 2026-10-19

Gator remains a local, user-controlled agent runtime: one user-facing manager,
bounded internal specialists, persistent goals, reviewable Work, and a host-owned
authority boundary. Improve the decisions around the existing Go runner rather
than replace its provider-neutral tool loop. A specialist assignment is not a
grant of autonomous decision authority. Unattended work may inspect, recommend,
and prepare drafts; consequential connected actions require a fresh, exact
foreground approval.

The next foundation connects request interpretation, goal and memory context,
tool selection, watch decisions, and visible authority in one retained Work
decision. Explicit user controls always override inferred intent. The product
shows what Gator is working on, what it remembered, when it will check again,
and what it may do. Internal prompts, model routing, and specialist mechanics
remain advanced diagnostics. Present timing and authority as separate controls,
not a single numbered autonomy slider.

Semantic retrieval defaults to an explicitly installed local Ollama embedding
model, with separately opted-in OpenAI or Gemini embedding credentials. Missing
or failed embeddings degrade to source-aware lexical search. Neither selecting
a chat model nor merely storing a cloud API key authorizes uploading saved
knowledge for indexing. Improve goal selection and alert relevance against
matched live evaluations before claiming the agent is smarter.

The first implementation pass retains a validated Work decision, uses a bounded
model route only for ambiguous terminal prompts, indexes eligible memories and
notes privately with lexical fallback, and retains checked goal-watch reasons.
The existing Claude Code Kit TUI shows the decision in History and presents
timing (`/jobs`) separately from authority (`/mode`) in Status. These are
inspectable controls and diagnostics; model quality, missed alerts, and
retrieval relevance still require matched live evaluation.

Decision recorded 2026-10-16. The intended references are [Meta Muse](https://about.fb.com/news/2026/09/introducing-muse-personal-ai-agent/), [OpenAI dots](https://learn.chatgpt.com/docs/dots), and [Hermes Agent](https://github.com/NousResearch/hermes-agent). The GitHub project named [`plyght/dot`](https://github.com/plyght/dot) is a separate terminal coding agent.

The [Muse and dots feature map](MUSE_DOTS_FEATURE_MAP.md) compares first-party product claims with Gator's implemented boundaries and sets an evidence-based order for closing the most valuable gaps.

## Product promise

The first guided product workflow is project follow-up: watch selected changes,
identify the next useful step, and draft a response for review. `/followup`
reviews the outcome, sources, notification conditions, frequency, permissions,
and limits before saving; `/memory` explains storage, retrieval, and chat
processing separately. Context and sources beside a result support correction,
forgetting, and one-run omission. Background health comes from durable attempts
and a fresh supervisor probe. The [project follow-up pilot](PROJECT_FOLLOWUP_PILOT.md)
defines the human review procedure and evidence required before success claims.

**Gator is a desktop personal agent that remembers your ongoing goals, works across local files and selected tools, and checks with you before consequential actions.**

The job is to help a person make progress over days and weeks. A goal may produce a brief, edit a document, answer a question, or prepare an external action; no one output format defines the product. The desktop app is the primary conversation and control surface. Local state and provider choice matter, but neither by itself proves usefulness or lower resource use.

## Competitive reality

The earlier “terminal Cowork” category already has a direct [terminal-native file agent](https://github.com/halukerenozlu/bolt-cowork), while [Hermes](https://github.com/NousResearch/hermes-agent), [OpenClaw](https://github.com/openclaw/openclaw), and [Letta](https://github.com/letta-ai/letta) cover substantial parts of the terminal/personal-agent space. A broad “AI agent in your terminal” claim is therefore crowded too. The existence of repositories does not measure their quality or adoption.

Meta describes Muse as a personal agent that develops plans for goals, works across apps on a secure VM, and advances work over time. [OpenAI Docs](https://learn.chatgpt.com/docs/dots) distinguishes dots from one-off ChatGPT Work tasks: dots own an ongoing responsibility, determine next steps as things change, and follow up between conversations. Hermes already offers a terminal entry point, persistent memory, skills, tools, MCP, and messaging. Gator should use these as a capability bar, not claim feature parity with their cloud computers, distribution, or background operation.

## Assets and remaining gaps

| Need | Existing asset | Gap to close |
| --- | --- | --- |
| Continue a conversation | Durable Work conversations, replay, branching, goal records, TUI picker, structured goal plans, and reviewed plan suggestions | Live tests must show that suggestions actually help users complete goals |
| Remember preferences and decisions | Scoped learnings, memories and source-linked notes with optional semantic recall, lexical fallback, explicit skills, and reviewed model suggestions | Retrieval relevance, suggestion quality, and source-link accuracy need live evaluation |
| Act through tools | Local files, browser, connectors, approval broker, internal specialists, reviewed goal-plan suggestions | Bounded connected polls can propose a next step; autonomous approved action and proven judgment are still absent |
| Work over time | Goal-linked jobs and watches, due review checks, attempts, inbox, exact duplicate-result suppression, local/connected content gates, a bounded Gmail history cursor, opt-in macOS and Linux per-user services | Watches poll while the host is online; no push event trigger, Calendar cursor, or proven semantic change detection |
| Extend behavior | Extensions, user-created text skills, MCP, role routing, read-only goal reviewer | Skills are explicit text procedures; model-authored suggestions and measured workflow selection need evaluation |
| Prove value | Fifty Work tasks and 25 two-turn personal continuity cases with a same-model baseline switch | No live continuity comparison yet; scripted pass is not model success |

## Implemented continuity and follow-up

A timed goal check can select configured connectors with repeated `--connector ID` flags. It rereads those services on each run; this is not an event subscription or durable remote cursor. The local `--change-only` gate is unavailable for an ordinary timed check with connectors because remote data can change independently of files. A connected goal watch uses a separate bounded remote content gate. Connector read permissions and credentials are checked again at execution.

A goal watch can also poll up to four explicitly selected connected read surfaces. For example, `gator goal watch GOAL_ID --connector google-work --gmail-query 'label:offsite newer_than:7d' --calendar-days 14` combines local files and due steps with a bounded Gmail search and Calendar window. It polls about every five minutes while the supervisor is running. A hash of the returned content is retained in job history, so an unchanged check skips the model even after restart. Completed and unchanged checks retain per-item fingerprints and Gmail's mailbox history cursor; the next check still lists the selected Gmail query and reuses unchanged message fingerprints when the account or grant is pinned. Older unpinned authenticated checks use full detail scans. Expired or overflowing history and a 24-hour interval also trigger a full scan. Scheduler state does not store message bodies. More than 500 matching Gmail messages or 500 Calendar events fails for review rather than silently dropping results. Calendar's moving time window cannot use Google's `events.list` sync token, so it rereads every bounded page on each poll. This remains polling, with no proof that Gator judges which changes matter; live evaluation is still needed. The connector remains read-only during unattended work.

`gator goal` now stores an explicit user-owned goal with a title, optional background context, status, and one retained conversation. Without `--source`, it creates a private empty workspace; with `--source`, it follows a selected local directory. `gator goal chat ID` opens the existing terminal UI bound to that goal. `gator work --goal ID "..."` loads the same context, resumes the goal's conversation, and refreshes the source capture each time. A goal chat defaults to inspect mode so a question does not force a `report.md`; `--artifact` or an explicit Work mode can request a deliverable or action. The context is data, not a permission grant. Paused and completed goals cannot run until resumed.

```sh
GATOR_INTERNAL_COMMANDS=1 go run ./cmd/gator goal add --context 'Compare venues and decide by Friday' 'Plan the offsite'
GATOR_INTERNAL_COMMANDS=1 go run ./cmd/gator goal chat GOAL_ID
GATOR_INTERNAL_COMMANDS=1 go run ./cmd/gator work --goal GOAL_ID 'What decision is still open?'
GATOR_INTERNAL_COMMANDS=1 go run ./cmd/gator goal context GOAL_ID 'Venue A is unavailable; decide between B and C by Monday'
GATOR_INTERNAL_COMMANDS=1 go run ./cmd/gator work --goal GOAL_ID 'What changed, and what should I do next?'
GATOR_INTERNAL_COMMANDS=1 go run ./cmd/gator work --goal GOAL_ID --artifact decision-brief.md 'Draft the decision brief'
```

The terminal home lists active goals and unread follow-up results. `/goals` opens a goal picker; `/memory` explains recall readiness and data destinations; `/context` reviews saved inputs and supports corrections, forgetting, and one-run omission. `gator memory search` finds saved knowledge, while `correct`, `forget`, and `--expires` let the user maintain it. Global memories may be retrieved for any Work turn; goal memories stay within their goal. Work combines bounded semantic and lexical retrieval when embeddings are available, with lexical fallback when they are not. Relevant items can still be missed. Stored memories are reference data, never tool authority.

In an ordinary Work conversation, a request to take on an ongoing responsibility can now queue a **goal proposal** with a title, context, outcome, and up to eight initial steps. The proposal can attach the run's selected local source and, if the user requested recurring follow-up, include a five-field cron schedule, timezone, and inspect-only task. No goal, source attachment, or job becomes active until the user inspects and accepts the proposal with `/suggestions` or `gator suggestion show/accept`. The accepted result prints `gator goal chat GOAL_ID` so the work can continue in a goal-bound conversation. A proposed schedule runs only while the local supervisor is available; it cannot send or publish. This is a conversational creation path, not a claim that a live model reliably recognizes every ongoing responsibility.

`gator goal schedule ID --cron '0 9 * * *' --task 'Check what changed'` creates a goal-linked job. At each run it reloads the latest goal context and source, retrieves relevant memories, and continues that job's prior conversation. The run records a bounded summary and a digest of its final answer and artifact content. An identical completed result under the same job policy stays out of the inbox; a changed result, failure, or recovery is surfaced. This is exact duplicate suppression, not a semantic claim that two different phrasings mean the same thing. Paused or completed goals are skipped. A foreground supervisor is available on Linux and macOS; both platforms have opt-in per-user services for operation after the CLI closes. Connected mutations still require exact-action live approval and cannot be scheduled.

`gator goal watch ID --task 'Watch venue changes' --artifact venue-reply.md` adds a local goal loop for files and due steps. While the supervisor runs, it checks each minute, calls the model only after input or due state changes, and asks for a next step with source evidence. Work may queue a reviewed plan suggestion and produce a draft; it cannot send the draft or silently change the plan. Quiet completed checks stay out of the inbox, and restarts retain the last completed input hash. This is bounded polling, so it requires a running local supervisor and is not an app event subscription. `--artifact` requires a draft on every model run and is best for tasks where every source change warrants one.

The goal context and memories remain user controlled. Gator can now queue a suggested note, skill, typed goal-plan change, or recurring check with a retained Work ID. `/suggestions` or `gator suggestion list/show` lets the user inspect it; `accept` applies it and `reject` discards it. A proposed plan change carries the revision it saw, so it fails if the user updated the plan meanwhile. A queued suggestion is not active knowledge, completed progress, or a running schedule. Gator does not infer a durable fact from every conversation, silently rewrite a goal plan, or claim to monitor the machine while it is off. A recurring check repeats the user's saved task; `--change-only` can skip a model call when local files and saved context are identical to the last successful run. The watch adds a bounded, model-proposed next step after a local change or due transition; it does not autonomously apply the proposal.

The knowledge base now stores inspectable Markdown notes and skills under the private state directory. The main agent can search/read scoped notes and active memories, and list/load an enabled skill when relevant. For active goals, it may delegate a read-only `goal_reviewer` to compare current context, recent turns, saved knowledge, follow-up results, and frozen source evidence. [Knowledge and skill commands](KNOWLEDGE.md) describe the lifecycle and boundaries. The model does not automatically write durable knowledge or skills.

A goal also has a structured plan with an outcome, up to 32 steps, status, deadlines, evidence references, and review times. Plan edits use revision checks so a stale conversation cannot overwrite a later correction. The plan is included as bounded reference context in goal-linked Work and goal reviews. For example:

```sh
GATOR_INTERNAL_COMMANDS=1 go run ./cmd/gator goal plan GOAL_ID outcome 'Choose a venue and confirm the budget'
GATOR_INTERNAL_COMMANDS=1 go run ./cmd/gator goal plan GOAL_ID add --title 'Confirm room capacity' --due 2026-10-20T09:00:00+08:00 --evidence 'venues.md:12'
GATOR_INTERNAL_COMMANDS=1 go run ./cmd/gator goal plan GOAL_ID
GATOR_INTERNAL_COMMANDS=1 go run ./cmd/gator goal plan GOAL_ID step STEP_ID done --evidence 'user-confirmed: room holds 90'
GATOR_INTERNAL_COMMANDS=1 go run ./cmd/gator goal plan GOAL_ID review-at 2026-10-21T09:00:00+08:00
```

Inside a goal chat, `/plan` shows that plan. `/plan outcome TEXT`, `/plan add TITLE | RFC3339_DUE | EVIDENCE`, `/plan step STEP_ID done | EVIDENCE`, and `/plan review-at RFC3339` make common edits in place; `/plan help` shows the syntax. `/activity` (or `gator goal activity ID`) brings together open steps, the last and next scheduled checks, supervisor status, unread follow-ups, pending goal suggestions, and commands to pause or resume. `gator goal access ID` makes bounded live reads of up to eight selected watch sources and reports whether each read succeeds, needs login, or lacks permission; it does not retain response bodies or update watch fingerprints. A successful probe confirms access at that moment, not future reachability or relevance. `gator goal notifications ID --quiet 22:00-08:00 --timezone Asia/Singapore` (or `/notifications` inside its chat) suppresses desktop interruptions for that goal during quiet hours; `--desktop off` mutes its desktop interruptions entirely. These settings narrow an enabled global desktop notification setting. The inbox always retains results and failures. `gator job inbox rate ENTRY_ID useful|noise|wrong|clear` records explicit feedback on shown job alerts, and goal activity counts those ratings; quiet checks and missed changes are outside that count. The activity view reads retained state and makes no claim that a scheduled check will run while the host is off.
When the local job supervisor is running, a due `review-at` creates one inbox
reminder without a model call unless an enabled watch owns that goal's review.
The goal's desktop mute and quiet hours apply to this reminder too. Optional
`gator goal notifications ID --urgent-only on` limits desktop alerts to failed
jobs and watch results the model marks urgent; routine results and due reviews
remain in the inbox. The urgent label is visible but unverified, so this
setting needs real-world missed-alert evaluation before being a default.

`/plan reviewed` records that the user reviewed
the goal and clears a due review time. The reminder will appear after the CLI
closes only while the local supervisor is running (or its per-user service is
installed and the user session remains active).

For an explicit quiet watch decision, `gator job quiet-audit JOB_ID` shows the
retained check and lets the user label a missed important change or a correct
quiet result after inspecting its sources. Unchanged-input skips and alerted
checks are excluded. This complements inbox ratings for shown alerts, but no
real review sample or missed-change rate has been established.

`gator work list`, `gator work show WORK_ID`, and the TUI History view report
whether a running Work record has an active process-held lease, a stopped lease
after interruption, or no lease from an older run. For an active foreground
run, another CLI can queue steering with `gator work steer WORK_ID TEXT` or
request cancellation with `gator work stop WORK_ID`.
`gator work activity [--json]` brings running Work records, specialist
checkpoints, and pending decision IDs into one read-only view; it labels stale
records for recovery. If the run continues, queued steering applies at the next
model boundary; it cannot interrupt an in-flight provider request or guarantee
that a nearly finished run uses it. Interactive `/activity` includes the same
Work view alongside the selected goal, or shows it alone when no goal is selected.
`gator work cancel-task WORK_ID TASK_ID` requests cancellation of one active
specialist and its descendants; its parent run can continue. The retained task
record shows whether cancellation completed.
`gator work steer-task WORK_ID TASK_ID TEXT` queues an instruction for one
active LLM specialist from another terminal; `/steer-task TASK_ID MESSAGE`
does the same for the current foreground run. It can apply only at the next
model boundary if the specialist continues. Hosted Code cannot be steered live.
Pending interactive Work approvals can also be answered from another terminal
with `gator work answer WORK_ID DECISION_ID` after reviewing the exact escaped
preview. The answer is tied to the original in-memory request and is accepted
once. One-shot CLI Work still asks in its own foreground terminal.

## Next engineering sequence

1. **Measure continuity quality.** Test multi-day goals with corrections, expiry, scope boundaries, and changing source facts. Compare live model behavior against a same-model baseline; passing storage tests alone does not prove agent usefulness.
2. **Measure recall.** Compare optional semantic recall and lexical fallback for relevant matches, omissions, and source accuracy. Preserve explicit correction and forgetting controls; expand indexing only from measured needs.
3. **Broaden the goal loop.** Local files, due times, bounded JSON/Gmail/Calendar polling, and a Gmail history cursor are wired. Measure next-step and draft quality, missed changes, query caps, and read failures; then add a Calendar cursor and a clear review flow for proposed actions. A separately scoped Gmail send can complete a foreground approved action; unattended watches remain read and draft only. Validate receipts and uncertain-send recovery with a real account before expanding action support.
4. **Improve quiet follow-up.** The watch avoids unchanged model calls and honors an explicit attention decision. Users can rate shown job alerts and review explicit quiet decisions, but no real ratings or complete missed-change review have been collected. Measure false alerts and missed material changes before claiming semantic change detection.
5. **Measure runtime cost.** Use the [runtime measurement tool](RUNTIME_BENCHMARK.md) to compare idle RAM, peak RAM, cold start, first response, and task cost against representative terminal agents on the same machine and model. Treat low RAM as a target until comparative runs exist.

## Evaluation reset

The [personal continuity benchmark](PERSONAL_CONTINUITY_BENCHMARK.md) now supplies
25 two-turn cases and a `--baseline-no-goal` ablation under the same model.
Scripted agent and baseline both passed 25/25, which validates the harness only;
the live comparison has not been run for this corpus.
The local watch has deterministic tests for changed files, due crossings,
quiet restarts, a retained conversation after many quiet attempts, and an
offsite reply draft with a pending plan suggestion. These tests verify the
workflow and approval boundary, not live model judgment or user value.

Keep the existing Work corpus as regression coverage for files and deliverables. Add a separate longitudinal corpus of multi-turn, multi-day simulations. Each case should supply an initial goal, source changes, later user messages, allowed tools, expected next actions, and forbidden actions. Include at least:

- resuming an interrupted plan after a week with the correct next step;
- detecting a changed deadline or deleted option instead of repeating stale advice;
- recalling a preference across conversations and obeying a correction;
- retrieving a fact with its source while abstaining when evidence is missing;
- drafting a message and keeping sending behind explicit approval;
- selecting a relevant skill or connector without exposing unrelated tools;
- recovering from a failed tool call without duplicating an external action;
- scheduled checks that are quiet when nothing changed;
- source and prompt injection attempts in notes, web pages, and connector results;
- long-running goals with multiple substeps and a measurable completed outcome.

Score task completion, continuity accuracy, justified tool selection, forbidden actions, user rework, model requests, wall time, and peak RSS. Run each scenario with a live model and a same-model baseline. A scripted fixture passing only demonstrates wiring, as documented in [the existing fifty-task benchmark](USER_WORK_V4_BENCHMARK.md).

## Recovery, usage, and data controls

[Recovery, shared usage, and private data](RECOVERY_USAGE_DATA.md) documents
/recovery, /usage, /data, their CLI equivalents, conservative defaults, and
verification limits.

[Everyday overview, restore, and migration](OVERVIEW_RESTORE.md) describes the
four-question home view, exact actions, fresh-profile recovery, and reviewed
source-folder relinking.
