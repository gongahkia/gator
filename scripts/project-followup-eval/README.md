# Evolving project follow-up evaluation

This opt-in live test exercises the production Work service and model adapter,
using a disposable project and knowledge store. It reads the live public Go
release endpoint, writes a cited update, revises it after a changed deadline and
new blocker, and reopens context after an exact saved-note correction. Embedded
source instructions must not authorize a send. No real user profile/account or
terminal session is used. Automated factual checks are not human draft ratings.

With an existing `OPENAI_API_KEY` in the process environment, run from the root:

```sh
GATOR_LIVE_PROJECT_REPORT="$PWD/scripts/project-followup-eval/results/live-NEW.json" \
  go test ./cmd/gator -run '^TestLiveProjectFollowup$' -count=1 -v -timeout 10m
```

The report must be new. All reports **must share the same results directory**
for this goal: its `budget.json` is the cumulative spending boundary. Do not
delete/reset it to get another run. Unknown responses retain cost reservations
across restart. A cross-process file lock serializes cost admission. The user
approved at most $10; this harness stops at $9.50 including unresolved reserved
cost, leaving margin. It accepts only `gpt-5.6` at the fixed verified rate bound,
text/function tools and approved OpenAI Responses endpoints. It caps input at
128 KiB and output at 4096 tokens, reserves before sending, and stops if the
bound would be exceeded. No automatic request retry is installed. Failed API
responses keep status but suppress raw provider diagnostics and secret fragments.

On 1 November 2026, official [model pricing](https://developers.openai.com/api/docs/models/gpt-5.6-sol)
lists $4/M input and $20/M output, with input cache writes at 1.25 times input.
The harness conservatively estimates all input at $5/M. Reported usage settles
the reservation; missing usage retains it. This is an upper estimate based on
published rates, not a provider invoice. Reverify rates before later reuse.

The default model endpoint is `https://us.api.openai.com/v1/responses`, matching
earlier repository evaluations. No endpoint is read from the source project.
It must be available to the supplied account. A credential failure is not a
workflow-quality result.

If `BRAVE_SEARCH_API_KEY` is present, the same Work request also receives scoped
native search and the test requires a search call. Otherwise the report records
that search is unconfigured; a successful page read is not described as search.
Brave billing is separate from model token usage. Do not run against a billable
search account without confirming its allowance/cost within the goal budget.

```sh
go test ./cmd/gator -run '^TestProjectEvaluationBudgetBoundary$' -count=1
go test ./internal/workrun -run 'Test(WorkSearch|EvolvingWeb|ResearchBrief)' -count=1
```

## Attempts

- `live-2026-11-01-01.json`: **failed**, retained. The real model fetched current
  public information, wrote the first brief and revised it for the changed
  deadline/blocker. The third phase stopped at the harness's original 64 KiB
  request cap after growing retained context. There were no recorded tool errors;
  17 model requests, 159,711 input and 2,327 output tokens, $0.845095 conservative
  cumulative estimate, no unresolved provider reservation. This is an evaluator
  limit, not proof of a production execution failure. The cap was revised to
  128 KiB with correspondingly larger pre-dispatch dollar reservations, retaining
  the same total ceiling. Budget boundary tests passed after that revision.
- `live-2026-11-01-02.json`: **PASS for the three live phases**, 16 additional
  model requests, 165,760 input and 2,779 output tokens; approximately 44.3 seconds.
  It used actual local/knowledge reads, the live official release URL, artifact
  writing/status checks, source refresh and corrected-note re-reading in the
  retained conversation. The changed deadline and blocker survived the revision;
  no external actions were recorded. No search key was present. Cumulative
  conservative model estimate across both attempts: **$1.729475**, 33 requests,
  325,471 input and 5,106 output tokens, zero unresolved reservations. The final
  draft is retained for review; human editing effort remains unreviewed. Its
  self-reported email word count is not independently accepted as a quality score.

No final workflow or multi-week consumer acceptance is claimed while required
scheduled/recovery/UI phases remain open.

The complete milestone also requires scheduled change/noise checks, failure and
recovery, chat delivery, revision/export, and pause controls. This initial live
sequence complements those tests; it does not replace them. See
[the acceptance ledger](../../docs/PROJECT-FOLLOWUP-ACCEPTANCE.md).
