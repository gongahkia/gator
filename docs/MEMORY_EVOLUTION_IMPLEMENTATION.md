# Evolving memory implementation

User request, 29 October 2026: resolve all six findings from the current memory
audit, including working passive capture and proactive maintenance/use. Preserve
the broader competitive-gap goal and monitor the terminal platform run alongside
this work. Changes should be atomic commits with observable acceptance evidence.

Baseline: commit `1766974dc`. A fresh run of `scripts/memory-evolution` reproduced
14 supported capabilities and 8 gaps. These are deterministic host/store probes,
not a live-model quality percentage. Original findings remain in
[the evaluation](MEMORY_EVOLUTION_EVALUATION_2026-10-29.md).

## Required outcomes

| Gap | Required behavior | Evidence required | Status |
| --- | --- | --- | --- |
| Conflicts under aliases | Stable concepts and inspectable alias resolution; contradictory facts cannot silently become simultaneously authoritative; ambiguous meaning gets review | Same concept across labels/paraphrases, old-version refusal, scope separation, migration and restart | Open |
| Incomplete forgetting | Suppression applies to the concept and derived personalization, with explicit restoration; source-history deletion remains a separate choice | Alias/relabel attempts, stale retrieval caches, summary invalidation, history reuse, backup/restore and explicit re-consent | Open |
| Weak freshness | Separate creation, observation and confirmation evidence; repeated confirmation updates provenance; bounded review of stale/changeable facts | Reconfirmation, out-of-order evidence, clock changes, stable versus changeable facts, no refresh from retrieval alone | Open |
| Unsuitable passive saves | Ordinary conversation captures enduring relevant facts; credentials, quoted third-party statements, temporary/hypothetical instructions and uncertain ownership cannot silently become personal facts | Positive/negative capture cases, cropped evidence, multilingual cases, sensitive controls, explicit review, opt-out, live-model evaluation | Open |
| Missing proactive learning | Bounded maintenance reconciles evidence and proposes useful lessons from repeated independent events; inferred rules remain reviewable | Observation aggregation/deduplication, source evidence, approval/rejection, budget/failure handling, scheduled execution and measured usefulness | Open |
| Corruption and fragmented inspection | Atomic durable writes, healthy recall despite an unrelated corrupt record with visible diagnostics; unified source-linked personal summary with corrections and withdrawal | Interrupted publication, corrupt records, private-root handling, concurrent writes, CLI/terminal workflows, summary freshness and restart | In progress |

Passive capture means learning from ordinary user conversation when enabled;
the user should not need to repeat a special command. Proactive memory means
maintenance and relevant use between explicit memory commands, not unbounded
collection or permission to perform external actions. Stored context never grants
tool authority. Uncertain interpretations must not silently rewrite identity.

## Implementation order

1. Durable publication and visible partial-recall recovery.
2. Concept identity, conflict/suppression rules and legacy-state migration.
3. Evidence/freshness lifecycle and passive-save suitability controls.
4. Bounded maintenance and evidence-based candidate generation integrated with
   foreground and scheduled work.
5. Unified inspectable personal summary and owning-store correction paths.
6. Longitudinal integration and live-model evaluation: capture precision/recall,
   stale fact use, unwanted resurfacing, useful proactive suggestions and cost.

This ordering does not narrow completion to storage or deterministic tests. Live
model behavior and user usefulness remain unverified until measured. Do not label
regex screening as general semantic understanding or publish an invented quality
score. External/provider evidence that cannot yet be collected stays explicit.

## Parallel platform verification

GitHub run `40902016728` exercises Linux, Intel/ARM macOS and Windows. It exposed
a Windows PDF.js trailing-slash path failure and an outdated uppercase-palette
assertion on Linux; fixes and subsequent native results belong in the terminal
verification ledger. It is a test workflow, not a deployment.

## Completed storage/recovery checkpoint

- `9f9ae6ace`: complete JSON is written and flushed in a private staging file,
  then published atomically. Exclusive creation cannot overwrite an existing ID.
  Unix directory metadata is flushed; Windows retains the existing `privatefs`
  limitation that directory flushing is not available through its read handle.
  Owned memory directories reject symlinks and use native private permissions.
- `4d7aef2fb`: listing/search/profile can return validated healthy records with
  a typed partial-read diagnostic. CLI output, saved-context views, memory
  settings, chat receipts, model preparation and knowledge-search tools expose
  that warning. `gator memory diagnose` identifies up to 32 unreadable filenames
  and reports the total; it does not modify or delete damaged records.
- Direct reads of damaged records still fail. Ordinary store-access errors stay
  fatal. Automatic subject consolidation remains blocked while an unreadable
  record might contain a conflicting subject; recovery must not invent facts.
- Fresh capability probe: **15 supported, 7 gaps**, versus the preserved 14/8
  baseline. The corruption probe now requires healthy results and accepts an
  explicit partial-read warning; an error with no healthy results still fails.

Verification: race suites for memory, tools, workapp and workrun pass; focused
command race tests matching Memory/Learning/Recall/ContextRetains/SavedKnowledge
pass. Added integration tests cover foreground/background context, CLI list/
search/diagnose, settings, retained-context presentation, healthy chat receipts,
tool exclusions and unchanged damaged bytes. Publication tests cover concurrent
readers, exclusive-ID collision, interrupted staging and owned-directory links.
These do not simulate physical power loss or complete native Windows validation.

Gap six is still open for the unified personal summary. The other five outcomes
remain open; this checkpoint does not establish working autonomous maintenance
or live-model capture quality.

## Evidence checkpoint

New confirmations refresh the existing record's provenance and profile priority.
The latest eight observations retain time, basis, source conversation/run and a
text digest; corrections do not keep another copy of the superseded personal
text. Replays present in this bounded history are idempotent. Revision times stay
monotonic across clock adjustments. Retrieval never refreshes confirmation.
Legacy records keep unknown observation dates until new evidence exists.
`gator memory show ID` exposes these fields and source references.

Memory, tools, workapp and workrun suites and focused command tests pass. The
fresh synthetic probe reports 16 supported checks and 6 gaps; this is not a
model-quality score. Freshness remains incomplete until stale/changeable facts
have bounded maintenance and review behavior. All six required outcomes remain
in scope.
