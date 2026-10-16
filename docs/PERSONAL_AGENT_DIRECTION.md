# Gator as a terminal personal agent

Decision recorded 2026-10-16. The intended references are [Meta Muse](https://about.fb.com/news/2026/09/introducing-muse-personal-ai-agent/), [OpenAI dots](https://learn.chatgpt.com/docs/dots), and [Hermes Agent](https://github.com/NousResearch/hermes-agent). The GitHub project named [`plyght/dot`](https://github.com/plyght/dot) is a separate terminal coding agent.

## Product promise

**Gator is a terminal-native personal agent that remembers your ongoing goals, works across local files and selected tools, and checks with you before consequential actions.**

The job is to help a person make progress over days and weeks. A goal may produce a brief, edit a document, answer a question, or prepare an external action; no one output format defines the product. The terminal is the primary conversation and control surface. Local state and provider choice matter, but neither by itself proves usefulness or lower resource use.

## Competitive reality

The earlier “terminal Cowork” category already has a direct [terminal-native file agent](https://github.com/halukerenozlu/bolt-cowork), while [Hermes](https://github.com/NousResearch/hermes-agent), [OpenClaw](https://github.com/openclaw/openclaw), and [Letta](https://github.com/letta-ai/letta) cover substantial parts of the terminal/personal-agent space. A broad “AI agent in your terminal” claim is therefore crowded too. The existence of repositories does not measure their quality or adoption.

Meta describes Muse as a personal agent that develops plans for goals, works across apps on a secure VM, and advances work over time. [OpenAI Docs](https://learn.chatgpt.com/docs/dots) distinguishes dots from one-off ChatGPT Work tasks: dots own an ongoing responsibility, determine next steps as things change, and follow up between conversations. Hermes already offers a terminal entry point, persistent memory, skills, tools, MCP, and messaging. Gator should use these as a capability bar, not claim feature parity with their cloud computers, distribution, or background operation.

## Assets and remaining gaps

| Need | Existing asset | Gap to close |
| --- | --- | --- |
| Continue a conversation | Durable Work conversations, replay, branching, goal records, TUI picker, and structured goal plans | Plan updates are still user maintained; the agent cannot propose a reviewed change |
| Remember preferences and decisions | Scoped learnings, searchable memories, longer Markdown notes, and explicit skills | Lexical retrieval misses paraphrases; automatic suggestions and document-linked recall need evaluation |
| Act through tools | Local files, browser, connectors, approval broker, internal specialists | Tool setup and selection are still technical; no clear goal-level plan/action loop |
| Work over time | Goal-linked jobs, attempts, inbox, macOS opt-in LaunchAgent | No change detection or quiet notification policy; Linux has no installed service command |
| Extend behavior | Extensions, user-created text skills, MCP, role routing, read-only goal reviewer | Skills are explicit text procedures; model-authored suggestions and measured workflow selection need evaluation |
| Prove value | Fifty synthetic Work tasks with artifact graders | No longitudinal benchmark for continuity, changing facts, action choice, or follow-through; scripted pass is not model success |

## Implemented continuity and follow-up

`gator goal` now stores an explicit user-owned goal with a title, optional background context, status, and one retained conversation. Without `--source`, it creates a private empty workspace; with `--source`, it follows a selected local directory. `gator goal chat ID` opens the existing terminal UI bound to that goal. `gator work --goal ID "..."` loads the same context, resumes the goal's conversation, and refreshes the source capture each time. A goal chat defaults to inspect mode so a question does not force a `report.md`; `--artifact` or an explicit Work mode can request a deliverable or action. The context is data, not a permission grant. Paused and completed goals cannot run until resumed.

```sh
gator goal add --context 'Compare venues and decide by Friday' 'Plan the offsite'
gator goal chat GOAL_ID
gator work --goal GOAL_ID 'What decision is still open?'
gator goal context GOAL_ID 'Venue A is unavailable; decide between B and C by Monday'
gator work --goal GOAL_ID 'What changed, and what should I do next?'
gator work --goal GOAL_ID --artifact decision-brief.md 'Draft the decision brief'
```

The terminal home lists active goals and unread follow-up results. `/goals` opens a goal picker; `/memory` manages explicit facts, preferences, decisions, and notes. `gator memory search` finds saved knowledge, while `correct`, `forget`, and `--expires` let the user maintain it. Global memories may be retrieved for any Work turn; goal memories stay within their goal. Retrieval is bounded and lexical, so it may miss a relevant paraphrase. Stored memories are reference data, never tool authority.

`gator goal schedule ID --cron '0 9 * * *' --task 'Check what changed'` creates a goal-linked job. At each run it reloads the latest goal context and source, retrieves relevant memories, continues that job's prior conversation, and places the result in the inbox. Paused or completed goals are skipped. A foreground supervisor is available on Linux and macOS; macOS also has an opt-in per-user LaunchAgent for operation after the CLI closes. Connected mutations still require exact-action live approval and cannot be scheduled.

The goal context and memories are explicitly user maintained. Gator does not infer a durable fact from every conversation, silently rewrite a goal plan, or claim to monitor the machine while it is off. A recurring check repeats the user's saved task; it does not yet automatically choose a new goal-level strategy or suppress an unchanged result.

The knowledge base now stores inspectable Markdown notes and skills under the private state directory. The main agent can search/read scoped notes and active memories, and list/load an enabled skill when relevant. For active goals, it may delegate a read-only `goal_reviewer` to compare current context, recent turns, saved knowledge, follow-up results, and frozen source evidence. [Knowledge and skill commands](KNOWLEDGE.md) describe the lifecycle and boundaries. The model does not automatically write durable knowledge or skills.

A goal also has a structured plan with an outcome, up to 32 steps, status, deadlines, evidence references, and review times. Plan edits use revision checks so a stale conversation cannot overwrite a later correction. The plan is included as bounded reference context in goal-linked Work and goal reviews. For example:

```sh
gator goal plan GOAL_ID outcome 'Choose a venue and confirm the budget'
gator goal plan GOAL_ID add --title 'Confirm room capacity' --due 2026-10-20T09:00:00+08:00 --evidence 'venues.md:12'
gator goal plan GOAL_ID
gator goal plan GOAL_ID step STEP_ID done --evidence 'user-confirmed: room holds 90'
gator goal plan GOAL_ID review-at 2026-10-21T09:00:00+08:00
```

## Next engineering sequence

1. **Measure continuity quality.** Test multi-day goals with corrections, expiry, scope boundaries, and changing source facts. Compare live model behavior against a same-model baseline; passing storage tests alone does not prove agent usefulness.
2. **Improve recall.** Measure lexical misses and false matches, then add source-linked retrieval over retained notes and documents. Preserve explicit correction and forgetting controls.
3. **Goal-aware action loop.** Let the agent propose a next step, select existing tools/skills, execute bounded local or connected reads, and ask for exact approval before external changes. Save completed and pending steps with evidence and timestamps.
4. **Quiet follow-up.** Compare snapshots and prior results so recurring checks notify only on meaningful change or an action due. Keep the lifecycle and resource cost visible.
5. **Measure runtime cost.** Benchmark idle RAM, peak RAM, cold start, first response, and task cost against representative terminal agents on the same machine and model. Treat low RAM as a target until measured.

## Evaluation reset

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
