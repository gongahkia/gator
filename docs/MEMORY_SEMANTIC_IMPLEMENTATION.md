# Semantic memory: implementation and evidence

Current implementation, 30 October 2026. This extends the historical
[memory evolution log](MEMORY_EVOLUTION_IMPLEMENTATION.md).

## What works

| Outcome | Implementation | Evidence |
| --- | --- | --- |
| General identity | Meaning comparison across arbitrary labels; durable digest-only bindings | Semantic store/tool tests; nine live classification cases |
| Updates | Fresh user statements retain exact quotes and provenance; equivalence shares identity; conflicts require inspected revisions; multiple matches require review | Store race tests, production Executor binding/budget tests, live corrections |
| Forgetting | Saved fact removed; abstract topic retained; tracked source chats suppressed; untracked snippets, replies, titles and folders screened before return | Search/direct-read/older-branch/restart tests; live historical privacy checks |
| Migration | Legacy topic/scope gaps visible; guarded owner review supplies metadata without reconstructing values | CLI/store tests and live custom-topic review |
| Restoration | Explicit current consent required; new values allowed; old historical cutoff and tracked-source suppression retained | Store tests and live fresh-context/old-history checks |
| Recovery | Atomic private publication, inspected revisions, complete-snapshot guards, interruption quarantine, backup/restore of abstract rules | Affected race suites and native lifecycle CI |

## Save path

Foreground memory tools use the Work run's budgeted model. A separate, tool-free
classification compares the current exact quote against the complete allowed
scope and returns equivalent, contradictory, distinct, uncertain or withdrawn.
The host validates JSON, references, relation consistency and limits. Unavailable,
malformed or incomplete comparison never defaults to distinct.

Application acquires the write lease and revalidates the entire snapshot,
including records, bindings and withdrawal rules. Direct source edits without a
revision bump also invalidate it. Equivalence preserves identity. A conflicting
replacement requires the inspected ID/revision; multiple matches require a
separate exact-input consolidation review. Current quotes retain source-chat,
run, timestamp and text-hash evidence. Merely reading memory never confirms it.

`memory reconcile --keep ID [--aliases LABEL,LABEL] ID=TIMESTAMP ...` offers the
same inspected identity controls directly. Prepared/applied receipts and
source-chat associations survive interrupted consolidation; remaining duplicates
are quarantined. Explicitly expired entries cannot be revived by comparison.

## Historical privacy

Forgetting stores a neutral attribute description, such as “the user's home
city,” rather than the old value. Known topics are host-defined; unfamiliar
attributes can get a generated proposal whose exact retained wording requires
host review. Declining still deletes the fact and leaves metadata pending.

Every earlier-chat return path screens the exact excerpts and metadata about to
be exposed. Dependent assistant replies and revealing titles/folder names are
included. Only clear verdicts return a row; uncertain or blocked rows are omitted.
Classifier context is transient and never appended to foreground replay. No
persisted clearance cache exists: every retrieval screens current content, so
new turns and edits cannot reuse old clearance. Separate body and metadata dates
preserve old barriers after restoration.

Malformed responses, provider outages and changed withdrawal settings return no
unchecked page. Unknown legacy topic/scope metadata explicitly withholds affected
history until owner review. Normal screening preserves unrelated history; it is
not a permanent blanket history switch. User-visible transcripts remain intact.

## Owner controls

```sh
gator memory withdrawals [--goal GOAL_ID]
gator memory withdrawals review ID --scope personal --expected REVISION --topic "the user's keyboard preferences"
# For a goal use: --scope goal --goal GOAL_ID
gator memory forget MEMORY_ID --topic "the user's keyboard preferences" --expected-updated TIMESTAMP
```

`withdrawals` reports abstract topics, pending metadata, scope, timestamps and
exact revision digests. Reviewing metadata cannot move a known scope, restore a
fact, or erase a historical barrier. Supply an attribute description without its
forgotten value. Explicit new remember requests restore the topic using current
words while old tracked source chats remain suppressed.

## Live evidence

All runs used OpenAI `gpt-5.6`, the US Responses endpoint and synthetic isolated
profiles. The key came from the local environment and is absent from reports.
No real memories, chats, accounts or files were evaluated. Saved provider defaults
were not changed. Cost is unpriced; token/request counts are provider reported.

- The preserved original capture baseline passed 11/12 cases; one ordinary
  preference reached review and was not saved.
- After foreground integration, all 12 small capture cases passed, using 26
  requests. These cover capture, correction, forgetting, learning controls,
  third-party/temporary/hypothetical exclusion and multilingual opt-out examples.
- The first longitudinal development attempt found that the agent acknowledged
  equivalent restatements without recording new evidence. The foreground policy
  now explicitly records new user confirmation; the initial failed run is kept.
- The final longitudinal run passed **55 checks across three sequences**: reply
  detail, home city and an unfamiliar keyboard preference. It used 120 requests,
  96,450 input tokens and 7,808 output tokens. Checks cover fresh contexts,
  attribution, paraphrases, correction, forget, refused passive relearning,
  translated/assistant/metadata copies, benign retrieval, search, explicit
  restoration and old/new history. One case also uses the real Work Executor.
- Nine classification cases passed after correcting fact/preference metadata in
  the fixtures and adding missing-reference uncertainty: nine requests, 3,991
  input tokens and 461 output tokens.

Reports live beside the runners in `scripts/memory-evolution`,
`scripts/memory-semantic-eval` and `scripts/memory-longitudinal-eval`. The latter
retains the dataset digest and both development/held-out labels. These are small
engineer-authored probes, not statistically independent quality estimates or a
competitor benchmark. Custom-topic approvals are simulated and recorded.

A transient evaluation parser bug read the tool payload outside its `result`
envelope and incorrectly marked correct privacy outputs as failures. That was
fixed before the final run; no product acceptance criteria were relaxed.

## Verification and practical limits

Local affected race suites pass for memory, tools, retrieval, personalization,
lifecycle, workapp and workrun. Focused command memory/recall/maintenance tests
and terminal memory/restart tests also pass. Production Executor tests verify
shared budgets and that private classifier context never enters retained replay.
[Native CI run 40995174776](https://github.com/gongahkia/gator/actions/runs/40995174776)
passes Windows, Linux, Intel macOS and ARM macOS at product checkpoint
`4ab982826`, including native memory/lifecycle suites, terminal UI, installation,
upgrade/rollback and Unix terminal handoff. Later benchmark-only files are
validated separately and do not change the packaged product paths.

Comparison currently caps at 128 candidates / 64 KiB of candidate JSON; larger
or excluded incomplete profiles require review rather than a sampled claim of
coverage. History caps at eight returned turns, four field checks per request,
64 KiB per classification payload and a 12-second call deadline. These add
provider latency/cost and can over-withhold during outages or ambiguity.

Semantic judgments remain fallible. Admission is still conservative and uses
host screening/review for sensitive, unfamiliar or ambiguous statements; this
is not a universal multilingual understanding guarantee. Background maintenance
still proposes reviewable lessons from repeated outcome evidence, rather than
autonomously inventing personal facts. This history gate cannot revoke text
already in an active conversation, user-opened transcripts, attachments, external
provider logs, or unrelated copies outside the managed retrieval paths.
