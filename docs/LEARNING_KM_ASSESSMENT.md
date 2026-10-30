# Learning and knowledge management: current assessment

30 October 2026. Recommendation: retain Gator’s local agent foundation and
validate learning, writing and project knowledge as its primary workflows.
A shared team knowledge platform is a separate expansion, not a smaller build.
This assessment does not authorize or implement that platform.

The [PKM problem research](research/pkm-problem-statement-2026-10-30.md) narrows
the proposed audience to people maintaining knowledge across existing tools. It
records supporting and contrary evidence, competitor overlap, and a validation
plan; the direction remains a hypothesis rather than a validated market.

## User intent

A conversational companion that helps someone understand a subject, develops
useful notes, writes with clear structure and good taste, and maintains current
project understanding with other people. Eventually this includes a shared
editable knowledge base and Jira-like source integrations. Gator should explain
what the project currently knows, with visible evidence and uncertainty.

## Evidence obtained

The [live runner](../scripts/learning-km-eval/README.md) uses the real Work service
with OpenAI `gpt-5.6`, isolated synthetic state and delegation disabled. It runs
three two-turn cases and a same-model text baseline with all the same files and
user follow-ups supplied inline. Deliverable creation is scored separately from
the quality of the resulting prose. No user accounts or real team data were used.

| Probe | Observed result | What it establishes |
| --- | --- | --- |
| Explain a misconception, then give feedback | Gator completed the lesson and revision with correct source arithmetic and a fresh question | A bounded teaching interaction works; learning retention is unmeasured |
| Collate project notes, then apply a date correction | Markdown and JSON preserved current dates, owners and unapproved status | Source synthesis and artifact continuation work in this fixture |
| Write and shorten a public draft | A 128-word final Markdown draft kept approval qualifiers and excluded internal details | Basic writing/revision and structure work; taste still needs review |
| Update an existing saved project note | Agent queued a note proposal; original note stayed unchanged | A proposal is not an applied knowledge update |
| Accept that proposal via the actual CLI | Acceptance created a second note containing update instructions; original date remained stale | Existing-note identity/replacement is unsupported by this proposal protocol |

All three tasks passed their selected mechanical graders. That is not a full
usability score. Initial output review used anonymous A/B labels, then inspected
sources and prior turns; it is one assistant review, not an independent human
study. The project brief repeats status and citations heavily (477 words plus a
large JSON object), while the direct draft is more compact. The short public
writing drafts are similar in quality. Both add a small unsupported promise that
future details will be shared. There is no demonstrated general prose advantage.

The initial learning follow-up was ambiguous about whether the learner was
answering the original example or a changed-base-rate practice question. The
baseline’s different calculation was valid for its own question. That comparison
must not be used to claim a Gator accuracy advantage. A focused rerun explicitly
names the original example: **both Gator and direct chat then calculate 24.62%
correctly**, explain the denominators and give a fresh question. Gator passes the
same content checks. Original results and the review amendment are retained.

## Time and resource use

| First complete run, both turns | Gator | Same-model text baseline |
| --- | ---: | ---: |
| Provider calls | 33 | 6 |
| Total elapsed | 122.5 seconds | 29.8 seconds |
| Reported input tokens | 235,164 | 3,326 |
| Reported output tokens | 6,367 | 3,030 |
| Tool errors / manual interventions | 0 / 0 | No tools |
| Real deliverable files | Yes | Draft text only |

Gator used about 38 times as many reported tokens in this small run. These are
not equivalent tool workloads: Gator reads files, retains state, creates and
validates artifacts, and revises staged outputs. The baseline receives files
inline and returns text. The figures still expose substantial workflow overhead
that must earn its cost through practical value. Monetary cost and caching
discounts were not measured. One trial per task cannot establish stable latency.

The clarified teaching rerun used 10 Gator calls and 24.8 seconds across its two
turns; direct chat used two calls and 10.8 seconds. These are repeat-run evidence,
not replacements for the original task timings.

## Knowledge-base gap

The note store already supports edits and reviewed changes through owner
controls. The foreground agent gets `knowledge_search` and `knowledge_read`.
Its `suggest_change` note protocol creates a new note on acceptance: it carries
no inspected target-note ID or expected revision for replacement. Putting
“update the old note” in the proposal body does not change that behavior.

The live probe found exactly this mismatch. The agent accurately said review
was pending, but also described preservation of identity/source history that the
acceptance protocol could not perform. Synthetic acceptance left two notes,
including the unchanged original. This is a concrete product gap for a knowledge
source of truth; it must not be disguised as a successful update.

The next implementation milestone should provide inspected, reviewable note
updates with stable IDs, revision checks, source references, explicit conflict
handling, and fresh-chat retrieval of the accepted current version. Deletions,
merges and external-source changes need their own clear semantics. A source
ledger and corrected project brief should be visible to the owner.

## Project and collaboration audit

Existing foundations include goal-scoped notes/memories, plans, source references,
retained conversations/artifacts, scheduling, and Atlassian connector operations
for Jira search/read/create/comment and Confluence search/create. Connector code
is not evidence of successful real-account integration; account sign-in remains
deferred. These tests did not access Jira or message anyone.

Shared editable knowledge is absent: local goal scope is not team membership or
per-person access control. A team version needs authenticated members, durable
shared versions, concurrent edit resolution, permission-aware retrieval and
source synchronization. Personal memory must not silently become shared project
knowledge. These are architectural changes beyond a new prompt or connector.

“Source of truth” should mean accepted, versioned project knowledge with source,
owner, freshness and unresolved conflicts. Jira can remain authoritative for its
ticket fields while Gator maintains and explains the project understanding.
A generated synthesis must not silently override the source or invent approval.

## Direction recommendation

Focus the next milestone on **learn → capture notes → revise project knowledge →
produce a useful draft**, initially for one person and a few real projects.
Measure corrections, fresh-chat retrieval, stale contradictions, formatting
cleanup, time/cost and whether people keep using it after the first conversation.
Use a compact writing route and inspect the current broad tool/context overhead.

Keep a broader shared workspace as a later option, after the personal workflow
proves useful. The current evidence supports a narrower product focus, not a
rewrite into a team platform. It also does not establish that Gator should become
only a web-search product: these probes used provided files, not open web search.
The foreground web catalog currently exposes selected-origin fetching; general
search discovery needs separate evaluation and product plumbing.

Current primary product documentation shows that
[Notion enterprise search](https://www.notion.com/en-gb/help/enterprise-search)
already combines workspace/connected-app search with citations, and
[Rovo](https://support.atlassian.com/rovo/docs/explore-rovo-features/) combines
Atlassian and third-party sources while respecting source permissions. **Inference:**
matching that full team surface is a much larger undertaking than improving a
personal learning/project companion. These feature descriptions are not a
head-to-head quality benchmark, and no competitor superiority claim is made.

## Reproducible records

First run: `scripts/learning-km-eval/results/openai-2026-10-30`.
Clarified teaching run: `scripts/learning-km-eval/results/openai-learning-clarified-2026-10-30`.
Source corpus: `internal/eval/testdata/learning-km-v1`.
The original corpus, failed interpretations, raw reports and final artifacts are
retained. Initial split labels describe the first selection; all cases have now
been inspected. Larger sources, actual long-running projects, real collaboration,
open-ended search, connected-account fidelity, accessibility and human learning
outcomes remain unmeasured.
