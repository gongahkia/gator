# Semantic memory implementation

Active user objective: implement semantic reconciliation, suppression of
untracked historical paraphrases, and live quality testing. This extends the
[earlier implementation](MEMORY_EVOLUTION_IMPLEMENTATION.md); completing a store
API alone does not complete this objective.

## Required outcomes and evidence

| Outcome | Acceptance evidence | Current state |
| --- | --- | --- |
| General concept identity | Arbitrary reviewed labels share durable identity across restart; corrections and withdrawal apply to every bound alias; scope and inspected revisions are enforced | Store and CLI implemented; local race suites pass |
| Semantic relation planning | Actual model-assisted comparison of meaning distinguishes equivalent, contradictory, unrelated and uncertain statements across domains and languages; original quotes remain intact; incomplete/failed analysis cannot silently add conflicting authoritative facts | Bounded analyzer implemented; small live classification baseline obtained; production application pending |
| Reconciliation authority and recovery | Contradictions require an inspected choice; no stale or cross-scope changes; an interrupted merge quarantines remaining duplicates; prepared/applied receipts and source associations survive | Store protocol implemented; semantic-tool integration pending |
| Untracked historical paraphrases | Forgetting also prevents historical paraphrases and dependent assistant replies from entering agent context through search, direct reads, titles, pagination and older branches; source transcripts remain user-controlled | Pending |
| Withdrawal migration and privacy | Legacy provenance gaps and classifier outages are visible; restart, changed transcripts, backup/restore and explicit restoration preserve suppression; implementation must not retain forgotten fact plaintext just to recognize it later | Pending |
| Live evaluation | Real provider runs with synthetic isolated state; separate development/held-out longitudinal cases; capture, correction, conflict, withdrawal, retrieval, scope, restart, uncertainty and provider failure coverage; source attribution and request/token/latency reporting | Initial development baseline obtained; expanded evaluation pending |

## Implementation sequence

1. General digest-only concept bindings and inspected reconciliation, including
   source-history association transfer and interruption behavior.
2. Bounded semantic relation planner and host-controlled application of its
   decisions. A small alias list or admission filter is insufficient.
3. Semantic withdrawal representation and screening of all agent history return
   paths, with migration and source-version-bound cache invalidation. A blanket
   history shutdown is not a substitute for identifying relevant paraphrases.
4. Longitudinal development and held-out live evaluation, then fixes driven by
   observed failures. Deterministic stand-ins validate host boundaries separately.

No semantic quality percentage or universal language guarantee is established
by a few passing cases. Model errors remain possible; uncertain analysis must
be observable and require reconciliation rather than becoming hidden biography.

## Initial checkpoints

- General bindings store only scope/subject digests, rather than saved personal
  text or an expanding hard-coded alias list. Bindings remain after forgetting.
- `memory reconcile --keep ID [--aliases LABEL,LABEL] ID=TIMESTAMP ...` keeps the
  chosen inspected text verbatim, consumes reviewed duplicates and preserves its
  confirmation date. Alias-only binding is also supported. Unreviewed group
  members, stale records, cross-scope inputs and forgotten concepts are refused.
- Hash-only source-chat associations are transferred before duplicates are
  removed. Prepared/applied reconciliation receipts retain IDs, revisions and
  binding digests. Identity publication precedes deletion, so a surviving file
  after interruption remains quarantined until reconciled.
- The live harness can explicitly select provider, model and endpoint without
  changing Gator's saved default. The user's key is read locally and omitted
  from logs/reports; `.env` and `.env.local` are ignored by Git.

The first successful live-provider run used OpenAI `gpt-5.6` and isolated
synthetic state. **11 of 12 small development cases passed**, using 23 provider
requests. One ordinary concise-answer preference reached review and was not
saved. This is an observed capture miss, not an established cause or a general
memory score. Location capture, inspected preference correction, forgetting,
temporary/third-party/hypothetical exclusions, synthetic credential exclusion,
Chinese/Spanish negative requests and learning controls passed those cases.

The project required a US regional endpoint. The initial global endpoint failed
with a geography error; the US endpoint worked. Endpoint selection follows the
[official regional data controls](https://developers.openai.com/api/docs/guides/your-data).
The baseline is retained in `scripts/memory-evolution/openai-development-baseline-2026-10-30.json`.

Live opt-in variables:

- `GATOR_LIVE_MEMORY_REPORT`: a new private report file, required to run.
- `GATOR_LIVE_MEMORY_PROVIDER`, `GATOR_LIVE_MEMORY_MODEL`: explicit test selection.
- `GATOR_LIVE_MEMORY_BASE_URL`: full adapter endpoint, e.g. the US Responses URL.
- `GATOR_LIVE_MEMORY_CASE`: optional exact case name for focused diagnosis.

Run `go test ./cmd/gator -run '^TestLiveMemoryEvaluation$' -count=1 -v` with those
variables and the provider key available in the child process. The harness does
not automatically source environment files. Review previews and called tool
arguments are retained only for these synthetic evaluation cases.

A focused repeat of the concise-answer case saved it successfully (three model
requests). That variability does not erase the earlier miss or establish its
cause. The harness now retains synthetic tool arguments and review previews to
support diagnosis. The general reconciliation checkpoint passes local race
suites for memory, tools, retrieval, personalization, lifecycle, workapp and
workrun, plus focused memory/recall/maintenance command tests. Model-driven
planning and historical-paraphrase screening remain required next steps.

## Meaning comparison checkpoint

`Store.AnalyzeChat` now uses an actual model to compare a proposed quote against
the complete supplied scope, rather than infer identity from labels alone. It
returns equivalent, contradictory, distinct or uncertain with references to
existing candidate IDs. It supplies no action tools, rejects fabricated IDs,
duplicate references, malformed or inconsistent JSON and unexpected fields, and
has a 12-second model-call deadline. Unavailable or incomplete comparison returns
an error for the host's review path; it never defaults to distinct.

Snapshots include exact scoped records and identity bindings in their digest.
Direct edits without a revision bump also invalidate a snapshot. Excluded
records are never supplied to the comparison model, and incomplete coverage
prevents classification. The current bound is 128 candidates and 64 KiB of
candidate JSON. These are explicit resource limits, not exhaustive support for
larger profiles. Analysis alone never modifies saved facts or aliases.

The independent live classifier evaluation passed eight synthetic cases with
OpenAI `gpt-5.6`: paraphrase, contradiction, separate topics, languages spoken
versus preferred reply language, translated residence, home versus travel,
different ownership and embedded instructions. The development and explicitly
selected held-out-labelled examples are recorded separately in the report;
this small developer-authored set does not establish broad generalization.

```sh
go run ./scripts/memory-semantic-eval --provider openai --endpoint https://us.api.openai.com/v1/responses --split all --report /tmp/NEW-semantic-report.json
```

The source report is retained at
`scripts/memory-semantic-eval/openai-classification-baseline-2026-10-30.json`.
Host-boundary tests cover scope isolation, fabricated references, inconsistent
relations, attempted tool invocations, cancellation, exclusions and source
snapshot changes. This checkpoint still requires guarded application through
the real memory tools, semantic withdrawal/history screening, migrations and
longitudinal live evaluation before the active objective can be complete.
