# Memory evolution evaluation

Implementation follow-up: [the active ledger](MEMORY_EVOLUTION_IMPLEMENTATION.md)
records subsequent fixes. Atomic publication and visible partial-recall recovery
now pass their focused tests; the fresh capability count is 15 supported / 7 gaps.
The original findings below remain the baseline, not a claim that later fixes
were present during that evaluation.

**Verdict: useful foundations, but not yet dependable evolving memory.** Gator
can save, recall and correct information. It does not yet reliably reconcile
meaning across labels, track repeated confirmation, reassess aging chat facts,
or turn accumulated operational evidence into proposed improvements.

This evaluates passive capture, proactive use and ongoing maintenance separately.
It follows the earlier [memory audit](MEMORY_AND_SCHEDULING_AUDIT_2026-10-29.md).
No production memory behavior was changed during this evaluation.

## What currently works

| Layer | Current strength | Important limit |
| --- | --- | --- |
| Passive capture | Foreground conversations can save enduring user statements without a special command when learning is enabled. Exact current-message evidence is required. | The model must choose to call `memory_remember`; capture is not an independent extraction pipeline. Quotation alone does not establish ownership or suitability. |
| Recall | Scoped lexical retrieval, optional semantic retrieval, earlier-chat search and a small foreground profile supply context. Provenance receipts identify supplied items. | Four profile items and a 4,000-byte rendered memory budget are bounded context, not a complete personal understanding. Live paraphrase/language recall quality is unmeasured. |
| Correction | Replacements under the same subject require the inspected revision; stale edits fail. Corrections survive reopening. | Different labels for the same concept bypass conflict detection. Identical reaffirmations do not refresh evidence. |
| Proactive use | Relevant context can influence a task without the user repeating a preference. Scheduled work uses ordinary scoped recall. | The always-included personal capsule is foreground-only. Scheduled monitoring does not itself imply ongoing memory maintenance. |
| Learning from work | Failures and uncertain outcomes record observations. Explicit corrections can create candidates; identical proposals merge evidence. | Observations do not automatically generate lessons. Candidates require confirmation, and stored confidence is not a calibrated probability. |
| Withdrawal | Exact forgotten subjects resist silent relearning; disabled learnings leave future projections. | Aliases can bypass suppression. Saved-memory deletion and retained conversation history have separate semantics. |

Implementation anchors: `internal/workrun/prompt.go` and `executor.go`,
`internal/tools/chat_memory.go`, `internal/memory/chat.go` and `store.go`,
`cmd/gator/work_service.go`, `internal/learning/observation.go` and `store.go`,
and `cmd/gator/command_work_feedback.go`.

## Observed longitudinal gaps

The new [reproducible probe](../scripts/memory-evolution/README.md) uses real
host tools and stores across successive synthetic changes. Its
[retained baseline](../scripts/memory-evolution/baseline-2026-10-29.json) records
14 met capabilities and 8 gaps. This is a capability inventory, not a 64% quality
score; the cases have unequal importance and are not a representative user sample.

1. **Reconfirmation is lost.** A preference aged one year, followed by four newer
   preferences, is repeated verbatim in another chat. Its timestamp and original
   chat provenance remain unchanged; it stays outside the four-item profile.
   It remains saved and may be retrieved by query. Gator lacks a separate
   last-confirmed/evidence trail for this case.
2. **Contradictions can coexist under aliases.** A detailed-replies preference and
   its short-replies opposite save under different subject labels without a
   conflict. Same-label guarded correction does work.
3. **Forgetting can be bypassed by relabeling.** After forgetting `profile.city`,
   the same fact is accepted as `profile.home_city`. The original label remains
   blocked. A stronger “do not personalize using this fact” operation is missing.
4. **A synthetic password is accepted by the host save tool.** The tool instructions
   forbid this; deterministic enforcement does not reject the deliberately bad
   tool choice. No real credential was used.
5. **A colleague's quoted preference is accepted as the user's preference.**
   Exact quotation validates the excerpt, not who it describes.
6. **A one-reply formatting request can become persistent memory.** Again, the
   model is instructed to avoid it; the host does not enforce that distinction.
7. **One corrupt record blocks unrelated healthy recall.** Injecting malformed
   JSON into one synthetic memory makes a search for another valid memory fail.
   The existing direct creation write also remains a durability concern from code
   inspection; this probe does not simulate power loss.
8. **Repeated failures produce observations, not learning candidates.** Recording
   three failures yields zero lessons without an explicit proposal step. Explicit
   user correction → candidate → confirmation → active projection does work.

The three suitability failures (items 4–6) measure host boundaries, not live-model
error frequency. No claim is made that every model would choose to save these
inputs. Repeated failures also need interpretation: generating a *reviewable*
candidate is a missing capability; automatically promoting it would be premature.

Other existing limitations remain: chat saves have no expiry/validity input;
memory creation is less robust than correction publication; learning edits lack
the same reviewed revision guards; stores have different deletion semantics;
and untouched live records affected by the earlier semantic-exclusion bug still
need a careful migration. These are code-inspection/prior-audit findings, not
additional independently measured results from the 22 probes.

## What “ever evolving” should mean here

The desired cycle is **observe → propose → reconcile → use → verify → revise or
retire**. More stored entries alone do not establish better memory.

- Preserve the original statement, source and scope. Track separate creation,
  last-observed and last-confirmed times; repeating a fact should strengthen its
  evidence without inventing a change in meaning.
- Resolve stable subjects and aliases. Apply explicit corrections to the right
  concept; expose genuine uncertainty instead of silently picking a biography.
- Distinguish enduring preferences, changeable facts, temporary instructions,
  third-party facts and inferred patterns. Their freshness rules differ.
- Reassess stale/changeable facts when relevant or during bounded maintenance.
  Do not expire a stable preference merely because it is old, or refresh an
  unsupported claim just because it was retrieved often.
- Accumulate repeated, independent evidence into proposed lessons. Current
  confidence=100 defaults must not be presented as measured certainty.
- Give forgetting meaning across saved facts, derived summaries and retrieval
  caches, while clearly separating source-history deletion. Avoid retaining
  forgotten plaintext merely to implement suppression.
- Make the personal summary a source-linked, correctable projection. Invalidate
  it after edits/forgetting and show conflicts and stale facts explicitly.

## Priority order

| Priority | Work | Why first |
| --- | --- | --- |
| P0 | Host save screening, alias-aware conflicts/suppression, durable creation and visible corrupt-record recovery | Incorrect or unwanted memory must not spread into future personalization. |
| P1 | Last-confirmed metadata, bounded evidence history, stronger concurrent learning edits, safe legacy recall repair | Establish an auditable evolving record before adding more autonomous behavior. |
| P2 | Bounded maintenance and evidence-based candidate generation | Repeated corrections/failures and stale facts can lead to useful proposals without silently rewriting identity. |
| P3 | Unified “What Gator knows about me” view and longitudinal live-model evaluation | Make the system inspectable and measure whether it actually helps across time. |

This extends the existing request for a personal summary; it does not replace the
queued reminders/scheduled-work features or close the ongoing broader goal.

## Verification and next evaluation

- Fresh race runs of `internal/memory`, `internal/learning`, `internal/retrieval`
  and `internal/tools`: **pass**.
- Focused command race run matching `Memory|Learning|Recall`: **pass**.
- Existing learning evaluator, explicitly selecting development and held-out:
  **14/14 probes pass**. Its model is a deterministic stand-in; this proves scoped
  context transfer/control behavior through the Work loop, not LLM generalization.
- Native Mac terminal memory and privacy journeys: **2 pass, 28 assertions**.
  They cover guarded corrections, concurrent changes, cross-chat recall, privacy
  toggle independence and restart.
- New capability audit: **22 probes completed; 8 gaps preserved in the report**.
  No paid provider, personal profile or real account was used. The synthetic
  profiles are removed when the probe exits.

Repeat commands and the retained diagnostic baseline are in the probe README.
Local logs are under ignored `.gator/native-tools/memory-evolution-*`; the existing
learning evaluator's full report is in `learning-evolution-eval/` beside them.

The next quality evaluation should simulate days/weeks of ordinary interaction:
changed names/locations/preferences, paraphrases, multilingual negation, temporary
requests, conflicting evidence, scope changes, forgetting, provider outages and
restart. Measure capture precision/recall, stale-fact use after correction,
unwanted resurfacing, retrieval precision, source attribution, useful proactive
suggestions and token/latency cost. Use held-out scenarios and label model/provider
versions. User acceptance of suggestions needs observed testing; silence is not
confirmation. No live-model recall or proactive-helpfulness rate is established
by the passing deterministic tests above.
