# Gator: maintaining understanding across knowledge bases

30 October 2026 · Research hypothesis, not a validated market or a shipped promise.

## Recommendation

Investigate **helping committed personal knowledge-management users keep their
existing notes useful as new evidence arrives across tools**. Retain Gator's
agent foundation. Narrow the user, recurring task, and success measures before
expanding integrations or shared workspaces.

Proposed positioning: **Gator helps keep the knowledge you already maintain
current, connected, and ready to use.**

The initial job is: **“I learned something new. Show me what it changes in my
existing notes and projects, and help me update the right place.”**

This is more specific than a personal assistant with memory. It is also a
competitive hypothesis: other tools already offer parts of this workflow.

## Initial user and problem statement

Recruit people who have maintained an Obsidian/Markdown knowledge base for at
least six months, use at least two additional information sources, and regularly
produce a research brief, article, technical decision, or project update. Possible
sources include Readwise, Zotero, shared documents, and project trackers. These
are recruiting criteria, not implemented Gator integrations.

Start with individual owners of a knowledge base who work on continuing research
or projects. Their collaborators' documents can be inputs where access permits;
shared editing is a separate product responsibility.

**Problem hypothesis:** When new research or project information arrives across
several tools, these users must manually reconstruct its relationship to earlier
thinking, judge whether previous conclusions still hold, and update the relevant
notes without losing their own interpretation or provenance. If they postpone
that work, later writing and decisions can rely on stale or disconnected notes.

The cost to investigate is repeated reconciliation, re-reading, and correction
before producing useful work. A large library alone does not establish this pain.
Some enthusiasts enjoy organizing their systems; others already have a simple
workflow that works. Recruit on recent behavior and consequences, not enthusiasm
for the phrase “second brain.”

## Evidence and its limits

This is a targeted desk review of research, public user accounts, current product
documentation, and Gator's retained evaluation. It is not a systematic literature
review, representative user survey, or hands-on competitor benchmark. Product
pages establish documented capabilities and positioning, not reliability.

| Evidence | What it supports | What it does not establish |
| --- | --- | --- |
| A 2025 study interviewed seven computer-science researchers at one Brazilian lab and observed their Obsidian workflows. Retrieval strategies shaped organization; participants preserved contextual links, processed an inbox, and one described difficulty spanning three vaults. [Study](https://arxiv.org/abs/2509.20187) | Existing structures encode how people think and expect to retrieve knowledge. Cross-vault context deserves investigation. | Population-wide prevalence, willingness to pay, or that AI maintenance improves learning. The authors identify the narrow sample as a limitation. |
| A public user describes keeping topic knowledge in Obsidian and lecture tracking in Notion, with manual links between them. [User account](https://www.reddit.com/r/ObsidianMD/comments/1rsq0nt/is_there_a_way_to_automatically_create_a_notion/) | Multiple tools can be an intentional division of responsibilities. | Everyone wants migration or full two-way content synchronization. This user explicitly wants the content to remain in Obsidian. |
| A November 2024 forum thread describes custom note-review scheduling and difficulty assembling a manageable workflow. A reply describes weak relevance-based resurfacing. [Discussion](https://forum.obsidian.md/t/how-to-periodically-review-entire-notes/92423) | Review timing and relevance are worth asking about. | A current Obsidian defect; this is historical evidence of a task, not a current feature comparison. |
| A maintenance discussion contains both complaints about overhead and replies from people doing little or no maintenance, using templates, scripts, or existing AI agents. [Discussion](https://www.reddit.com/r/ObsidianMD/comments/1x88j8y/vault_maintenance_vs_actual_work_how_do_you_keep/) | Workarounds and negative cases exist. | Demand size or authentic customer intent from every poster. Public posts may be self-selected or promotional. |
| Readwise documents append-only Obsidian highlight export. Changes to previously exported highlights do not automatically propagate in either direction, preserving the user's edits. [Documentation](https://docs.readwise.io/highlights/docs/exporting-highlights/obsidian) | A concrete tension between keeping copies current and preserving local work. | That Readwise is broken, or that blind overwrite would be an improvement. |

**Inference:** a promising task is to explain the impact of a source change and
offer a precise revision in the user's existing structure. Merely importing more
material could increase the maintenance burden.

## Competitive challenge

Documentation checked on 30 October 2026. Absence from these pages is not proof
that a competitor lacks a capability.

| Existing option | Relevant documented capability | Consequence for Gator |
| --- | --- | --- |
| [Mem](https://get.mem.ai/) and its [agent](https://get.mem.ai/product/agent) | Markets continuously updated project understanding, visible memory, and proactive follow-up. Its [Claude connector](https://claude.com/marketplace/connectors/mem) includes note creation and updates. | “Living memory” and “AI that updates notes” are already occupied promises. Gator must demonstrate a better specific workflow. |
| [Khoj](https://docs.khoj.dev/) | Personal AI over documents, Markdown, and Notion, with Obsidian access and self-hosting. | Local control and conversational access across sources are not unique on their own. |
| [Notion enterprise search](https://www.notion.com/help/enterprise-search) | Searches workspace and connected apps, including Drive and Jira, with citations. | Cross-tool search with sources is an established capability. |
| [Gemini Notebook / NotebookLM source management](https://support.google.com/gemininotebook/answer/16215270?hl=en) | Current documentation says Drive sources auto-update. Generated studio outputs do not change when their source changes or is deleted; it cannot edit original Drive files. | Do not use the outdated claim that all Drive sources require manual refresh. Source freshness and maintenance of derived work are separate tasks. |
| [Obsidian Web Clipper](https://obsidian.md/help/web-clipper) and its [Interpreter](https://obsidian.md/help/web-clipper/interpreter) | Capture, templates, and language-model processing of web content. | Capture and summarization alone are weak reasons to adopt another tool. |

An additional baseline is the user's existing general assistant with file access,
connectors, scripts, or plugins. It may already perform the proposed maintenance
well enough. Compare against that configured workflow, not only a bare chatbot.

The hypothesis to test is **lower effort and higher trust when maintaining
knowledge in place**, across sources the user already relies on. This review
does not establish exclusive functionality, a durable competitive advantage, or
an unserved market.

## A narrow first workflow

Illustrative scenario, not a current Gator demo: a person has an Obsidian note
comparing retrieval techniques, paper annotations, and a shared project brief.
A new paper qualifies an earlier conclusion; a project decision also changes
which trade-off matters.

1. The user selects a project, its sources, and its canonical note folder.
2. Gator explains what changed, linking the evidence and affected note passages.
3. It distinguishes a genuine correction from a different context, opinion, or
   unresolved disagreement. A newer date alone does not establish truth.
4. It proposes a small diff to the existing note, retaining links, formatting,
   the user's interpretation, and any unresolved questions.
5. After review, the accepted version is available in the original knowledge
   base and in later conversations. Undo and concurrent-edit protection work.
6. The user uses that understanding to write the project brief or continue
   learning. Measure the resulting work, not how many notes Gator generates.

Initially use one writable home: a selected local Markdown/Obsidian folder.
Other knowledge bases supply selected read-only sources. Prototype with explicit
imports first, visibly marked as snapshots; do not imply live synchronization.
Build one live source adapter only after the pilot identifies the most valuable
source. Preserve folder structure, frontmatter, links, and hand-written content.

Personal preferences, source evidence, the user's interpretations, and accepted
project decisions need distinct meaning and scope. Keeping them in one retrieval
system must not turn a preference into evidence or an unreviewed claim into an
accepted decision. A source of record remains authoritative for its own fields.

Passive behavior: connect an explicitly supplied source to the current topic and
prepare an update during the conversation. Proactive behavior: on an enabled
check, inspect selected sources and surface consequential changes in a bounded
review queue. Both must show what was actually checked and when; inaccessible
sources are unknown, not unchanged. Current background execution still requires
an online host. Do not interrupt merely because a file changed.

Defer arbitrary two-way synchronization, multiple writable homes, team accounts,
collaborative editing, universal web monitoring, and a replacement note editor.
Open-ended web research and learning exercises can support this workflow later;
they are not additional promises needed to validate the initial problem.

## Fit with Gator today

The [live assessment](../LEARNING_KM_ASSESSMENT.md) establishes useful foundations:
retained work, memory correction and withdrawal, scoped retrieval, source
references, reviewed suggestions, artifacts, and scheduling. It does not prove
that arbitrary external knowledge bases are synchronized or maintained.

The nearest concrete blocker is reviewed editing of an existing note through the
agent. The live test and actual acceptance produced another note while leaving
the original stale. The current [suggestion tool](../../internal/tools/suggestion.go)
has no target-note/revision contract, and
[acceptance](../../cmd/gator/command_suggestion.go) creates a new note. The
[note store](../../internal/knowledge/review.go) already supports guarded owner
edits, which can inform the missing agent protocol.

Engineering order if validation supports this direction:

1. Complete reviewed internal note updates with identity, exact revision,
   provenance, conflict handling, and retrieval of the accepted version.
2. Add a selected external Markdown home with narrow, reversible edits that
   preserve user structure. Internal store editing does not establish this.
3. Track selected source versions and propose affected-note updates; validate
   semantic judgments, missing coverage, and notification usefulness.

Existing memory safeguards are enabling infrastructure. The selling point would
be a successful recurring user task. The earlier benchmark also showed sizable
latency/token overhead, so saved user time must include review and correction.

## Validation before committing the roadmap

No interviews, participant outreach, account connections, or pilot have been
conducted as part of this research. The following is a proposed protocol.

**Discovery:** interview eight people meeting the initial criteria and two who
manage similar work comfortably with their current tools. Ask them to show a
recent research/project output and trace the information used, with their consent.

- Show the last occasion when new information changed an existing conclusion.
- Where did the old and new material live? Which note actually needed editing?
- What did you do, how long did it take, and what happened if you skipped it?
- Which organization, wording, or review work helps you think and should stay yours?
- Which assistant/plugin/script already helps? Where did it fail, if anywhere?
- What would you allow a tool to inspect or change? What would justify paying?

Seek evidence from the last few weeks, including successful workflows. Avoid
asking whether an imagined AI second brain sounds useful. Tool enthusiasm and a
large vault are not proof of a recurring costly problem.

**Pilot:** five qualifying users, two weeks, one active project each. Begin with
selected copies or reversible changes and record time spent reviewing and fixing
suggestions. Compare similar tasks with their normal setup and their current
assistant. Counterbalance task order where practical. Ask users to explain the
resulting conclusion and locate its evidence; accepted prose alone is not proof
of understanding. A later follow-up would be needed to measure learning retention.

**Provisional decision gates, chosen before collecting pilot results:**

| Measure | Proposed gate |
| --- | --- |
| Recurring need | At least 6 of 8 target interviews show a recent consequential maintenance episode and a recurring workaround. |
| End-to-end value | At least 30% lower median active completion time, including review/correction, with no reduction in source-grounded correctness. Retain individual results. |
| Useful suggestions | At least 70% judged useful by the owner, including rejected and deferred proposals in the denominator. Also record important changes Gator missed. |
| Trust | No silent overwrites, duplicate canonical notes, cross-project disclosure, or fabricated source support in the pilot. One occurrence blocks widening writes. Passing is not a universal safety guarantee. |
| Continued use | At least 4 of 5 return without researcher prompting in week two; record actual willingness to pay separately from compliments. |

These small-sample gates guide the next investment; they cannot estimate market
size or establish statistical superiority. If the main need is deterministic
formatting or synchronization, a small plugin/script may be the better product.
If review costs exceed savings, or users prefer existing assistants, narrow or
stop this direction. Strong conversational memory by itself does not validate it.
