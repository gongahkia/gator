# Memory audit and requested scheduling follow-up

Status: audit completed locally; two memory defects fixed. New reminder,
scheduled-task management and personal-summary features remain queued after
the current competitive-gap implementation. This does not close its live-use,
browser integration, native-device or release gates.

## Confirmed product request

- Reminders created through the CLI, delivered by desktop notification and kept
  in a CLI inbox even when the interactive application is closed.
- Scheduled work with a saved prompt, schedule, controls and retained results.
- A readable personal summary like ChatGPT's Personalization / About you view.
  The supplied settings link identifies the feature; its private contents were
  not accessible and were not inspected.
- Continue atomic commits and keep progress reports short.

The intended comparison is a high-level, correctable view of remembered context,
not merely a raw memory list. OpenAI describes its summary as incomplete and
separate from the original sources. Gator should make its own coverage explicit
and link each statement to inspectable evidence.
[OpenAI memory documentation](https://help.openai.com/en/articles/8590148-memory-in-chatgpt).

## What already exists

| Layer | Current behavior | Main implementation |
| --- | --- | --- |
| Saved facts/preferences | Local JSON, global/goal scope, optional expiry, source fingerprints | `internal/memory/store.go` |
| Chat learning | Exact current-message quotes, origin chat/run, subject consolidation, revision checks, receipts | `internal/tools/chat_memory.go`, `internal/memory/chat*.go` |
| Retrieval | Lexical fallback plus optional embeddings; scope, expiry and source eligibility filtered before retrieval | `cmd/gator/work_service.go`, `internal/retrieval/` |
| Personal context | Up to four recent preferences or selected profile subjects, then query matches within a 4,000-byte rendering budget | `internal/memory/chat.go`, `store.go` |
| Notes and procedures | Separate scoped notes and reusable skills with review controls | `internal/knowledge/` |
| User guidance | Separate global/project learnings; inferred candidates require confirmation before activation | `internal/learning/` |
| Earlier chats | Bounded selected-ancestry retrieval; current chat and other-goal chats excluded | `internal/tools/chat_history.go` |
| Privacy | Separate chat-learning/history toggles, per-run exclusions, explicit cloud embedding choice | `internal/workapp/recall.go`, `cmd/gator/work_app_settings.go` |
| Data lifecycle | Reviewed retention/deletion and export/restore; subject-forgetting markers included in backups | `internal/lifecycle/` |

Memory and notes do not grant tool authority. Current corrections take precedence
in the model instructions. The context receipt records the bounded items actually
provided. These are useful foundations, but they do not by themselves establish
good recall or correct interpretation of every user statement.

## Findings, ordered by impact

### Fixed: saving and correcting could disable semantic recall

`RememberChat`, receipt corrections and reviewed note corrections called
`retrieval.DropID`. That function creates a permanent exclusion marker intended
for deletion. Fresh chat memories therefore remained in lists/basic recall but
could never enter semantic search. Corrected notes had the same problem.

Commit `48d948298` separates cache invalidation from forgetting. Saves/corrections
invalidate vectors; deletion still blocks a stale caller from reindexing the
forgotten ID. Correction paths also repair an old exclusion marker for the live
record while holding the owner's mutation lock.

Regression tests failed before the fix and passed after it. They cover initial
chat save, chat/receipt/reviewed/CLI/file-backed correction, a legacy marker,
reviewed note correction and deletion with stale search inputs. Synthetic local
embeddings test the integration without a paid provider.

**Remaining migration:** untouched live records affected by the old bug still
need a repair pass. Correcting them now repairs their markers. Do not erase all
retrieval tombstones: some protect genuinely forgotten IDs. A migration must
verify live authoritative records under their store locks and preserve deletions.

### Fixed: “don't save this” could still save a memory

The negative-intent guard recognized a narrow set of “remember” phrases but not
“don't save”, “don't store”, or “don't want you to remember”. A model could supply
an exact quote cropped away from the negation and the host accepted it.

Commit `feca5ef7f` expands that guard over the full current message. Regression
tests cover eight negative formulations, automatic and explicit flags, and
cropped evidence; none may write or ask for save confirmation. This is an English
guard improvement, not a claim of complete multilingual intent understanding.

### High: forgetting is tied to a model-chosen label

A disposable-profile probe saved `profile.city`, forgot it, then submitted the
same fact under `profile.home_city`. The original subject was rejected; the alias
was accepted. Subject keys are model supplied, not a host-owned identity system.
Conflict checks have the same alias limitation.

Earlier-chat retrieval separately returns retained user/assistant text and does
not consult subject suppression. Forgetting a saved record therefore does not
prevent the same information being retrieved from its original chat. The UI does
disclose that history is retained, but a future “don't use this about me” action
needs stronger semantics than the current record deletion.

Needed: stable subject identities, reviewed aliases/conflicts, and distinct
controls for deleting a saved fact, suppressing personalization, and deleting
source history. Do not silently delete original conversations or retain forgotten
plaintext just to implement suppression.

### High: exact quotation does not establish suitability for memory

The host checks that a quote occurs in the current user message. It does not
establish that it describes the user, is enduring, or is safe to retain. Probes
confirmed acceptance of a quoted colleague statement and a synthetic password.
The tool description forbids these, so this is a host-enforcement gap, not proof
that a particular live model would choose to save them.

Needed: deterministic screening for recognizable credentials, stricter handling
of quoted/temporary/hypothetical statements, and a reviewed candidate route when
ownership or meaning is uncertain. Sensitive personal facts need appropriate
user controls; they should not all be treated as credentials. Extend multilingual
and negation evaluations rather than claiming a regex solves natural language.

### Medium: personalization is fragmented across stores

Facts, notes, chat history, goal context and approved learnings have different
selection and deletion rules. `learning.Remove` disables a record and retains
its provenance; `memory.Forget` deletes a fact. Learnings serialize replacements
atomically but lack the memory store's per-record reviewed revision/write-lock
protocol, so concurrent edits need stronger protection before unified editing.

Needed: a summary projection over authoritative sources, with source type,
scope, freshness and revision attached to each statement. Route edits to the
owning store. Show conflicts instead of silently selecting a plausible biography.
Keep inferred patterns distinct from user-stated facts and confirmed guidance.

### Medium: durability and recovery need more work

Memory creation writes directly to the final file; readers do not take the
writer lock. A partial write or crash can leave malformed JSON. `List` stops on
one invalid record and Work preparation propagates that failure. Corrections use
a synced temporary file, but creation and directory durability are inconsistent.
Memory/retrieval roots also have less symlink hardening than the knowledge store.

These are code-inspection findings; this audit did not simulate power loss or
hostile filesystem races. Needed: atomic publication, consistent private-root
handling, explicit corruption diagnostics/recovery and fault-injection tests.
Never silently treat corrupt private data as valid memory.

### Medium: freshness and recall quality remain limited

Chat-created facts have no expiry field. The always-included profile selects only
four records by recency; it is not an exhaustive personal summary. Lexical memory
matching uses different token rules from chat-history search, and the rendering
budget can omit later records. Notes' source references are labels, unlike the
fingerprinted file-backed memory path. These differences need visible treatment.

Needed: “last confirmed”/stale-fact review, configurable summary categories, an
inspectable full list, and longitudinal evaluations for changed facts, aliases,
negation, paraphrases, languages, forgetting, provider outage and restart.
Existing unit/product fixtures are not a measured live recall success rate.

## Scheduling: reuse and missing pieces

Gator already has cron/timezone jobs, missed-run policies, durable attempt
identities, run history, enable/disable/edit/run commands, supervisor integration,
goal follow-ups and a retained inbox. The terminal launcher exposes the retained
`job` and `memory` command families. Goal-plan review reminders already avoid
model calls. Reuse these rather than build a second scheduler.

Desktop delivery currently uses `osascript` on macOS or `notify-send` on Linux.
Errors are discarded; there is no Windows branch in that helper. An attempted
handoff is not evidence that the user saw a notification.

For comparison, OpenAI documents saved schedules, same-chat or independent runs,
management controls and local-project execution requirements.
[Scheduled tasks](https://learn.chatgpt.com/docs/automations).
Claude documents recurring prompts, run review, edit/pause/resume/delete/run-now,
and remote execution with local exceptions.
[Claude scheduled tasks](https://support.claude.com/en/articles/13854387-schedule-recurring-tasks-in-claude-cowork).
Gator's current local host does not provide cloud availability while it is asleep.

## Implementation sequence after current work

1. **Memory foundations:** migrate erroneous live-record exclusions; strengthen
   suppression/alias handling, candidate validation and concurrent corrections.
2. **CLI reminders:** one-shot and recurring time/zone, list/edit/cancel/snooze,
   durable inbox delivery and read/done state. Reminder delivery must not require
   a model, workspace snapshot or provider credentials.
3. **Scheduled work:** saved prompt, selected sources/access, next run, pause,
   resume, run now, expiry/budgets and inspectable results/errors. Preserve existing
   unattended read/draft bounds and separate foreground approval for sending.
4. **Desktop delivery:** supervisor works with the interactive UI closed;
   persisted delivery attempts and bounded retries; clear unsupported/denied
   status. Keep inbox entries when desktop delivery fails. Test sleep/wake,
   restart, duplicate ticks, DST and changed schedules. Mac is the initial host.
5. **About you:** concise categories with evidence, correction, forget/suppression
   and refresh in both the CLI and terminal application. Show coverage and stale
   or conflicting items; keep goal/project details scoped. A cached summary must
   be invalidated when an underlying record changes or is forgotten.

## Verification and limits

- Passing race suites: memory, knowledge, retrieval, tools and workapp.
- Passing focused command tests: memory CLI/projection/source freshness, learning
  controls and prompt projection, memory setup and retained-history wording.
- Passing race suites: learning, jobs, followup, inbox and worksession.
- Lifecycle audit exposed a macOS fixture comparing `/var` with `/private/var`.
  The fixture now uses its canonical source directory, retaining the exact-path
  and no-synthetic-revision checks. The full lifecycle race suite passes after
  that fixture correction; no restore implementation change was needed.
- Probes used disposable synthetic profiles. No private ChatGPT summary, real
  user memory, live model/provider, actual notification or production service
  was used. No new reminder/summary feature is claimed delivered by this audit.
