# Personal agent continuity benchmark

This corpus tests whether Gator can resume an ongoing goal after facts or user
direction change. It is separate from the deliverable-focused Work corpus.
There are 25 two-turn cases: 20 replace a local source document between turns;
five correct saved goal context, including one plan outcome. Cases cover briefs,
meetings, email drafts, events, writing, travel, procurement, finance, hiring,
and personal preferences. Eighteen cases are development and seven are held out.

Each trial creates a private copy of the source fixture, a goal record, a
structured plan, and scoped memory. The second turn reuses the conversation,
applies the source or goal change, and grades the final answer for the updated
fact. The original fixture must stay unchanged, the Work transaction must be
retained, and the outcome must complete. The grader checks a concrete phrase;
human review is still needed for usefulness, nuance, and safety.

## Run it

```sh
gator work eval validate internal/eval/testdata/personal-continuity-v1/dataset.json
gator work eval run internal/eval/testdata/personal-continuity-v1/dataset.json \
  --report-dir /tmp/gator-continuity-agent --id continuity-agent \
  --split development --live --provider gemini --model gemini-3.5-flash \
  --max-model-requests 500 --timeout-seconds 7200
gator work eval run internal/eval/testdata/personal-continuity-v1/dataset.json \
  --report-dir /tmp/gator-continuity-baseline --id continuity-baseline \
  --split development --live --provider gemini --model gemini-3.5-flash \
  --baseline-no-goal --max-model-requests 500 --timeout-seconds 7200
gator work eval compare /tmp/gator-continuity-baseline/experiment.json \
  /tmp/gator-continuity-agent/experiment.json
```

Use the same provider, model, case split, attempts, and request budget for both
arms. The baseline still has the same Work conversation, source files, and
model. It omits only the durable goal ID, plan, and saved memory context. The
corpus uses private copies so each trial starts from the same facts. Run the
held-out split only after changes are fixed against development cases.

## Evidence so far

On 2026-10-16, both scripted arms passed 25/25 cases. Those scripts supply the
answers directly; this proves fixture mutation, goal setup, conversation
continuation, transaction retention, and grading work. It does **not** measure
model task success or a product advantage. A live, same-model comparison was
left pending because the user previously reported that the configured Gemini
daily request quota was exhausted; no live call was attempted for this corpus. A live
report should include success by case, provider failures, tool failures, model
requests, latency, and manual review of incorrect or misleading answers.

The corpus does not yet test connected app reads, background trigger latency,
or whether a user would accept an agent suggestion. Those require account
fixtures or a supervised live trial; passing local scripted cases cannot prove
them.

## 2026-10-17 live smoke run

`internal/eval/testdata/personal-continuity-v1/smoke-dataset.json` selects three
development cases from the same corpus: a changed venue, a corrected brief
preference, and a changed goal priority. The agent arm requested Gemini
`gemini-3.5-flash` with an 80-request and 20-minute cap. It passed the venue
case. The other two cases failed on HTTP 429; the provider error named
`gemini-3.6-flash` and reported free-tier request limits. The local raw report
is at `/tmp/gator-continuity-smoke-agent-20261017/experiment.json`. No baseline
arm ran, so this result does not show an advantage over the same model without
goal state, nor does it grade the two interrupted cases as agent failures.

After quota is available, rerun both arms with the same explicit model,
dataset, limits, and number of attempts, then compare the retained reports.
The connected local/Gmail/Calendar brief path also has a scripted integration
test, but no live relevance result yet.

## Local-model comparison

Gator's `/model` → **Local model** screen can install and select a reviewed
Ollama model. After Ollama and that model are available, the same continuity
evaluation can run without a cloud API quota. For a small smoke run, replace
`qwen3:4b` below with the exact installed model tag if needed:

```sh
gator work eval run internal/eval/testdata/personal-continuity-v1/smoke-dataset.json \
  --report-dir /tmp/gator-continuity-local-agent --id continuity-local-agent \
  --split development --live --provider gator-local --model qwen3:4b \
  --max-model-requests 80 --timeout-seconds 1200
gator work eval run internal/eval/testdata/personal-continuity-v1/smoke-dataset.json \
  --report-dir /tmp/gator-continuity-local-baseline --id continuity-local-baseline \
  --split development --live --provider gator-local --model qwen3:4b \
  --baseline-no-goal --max-model-requests 80 --timeout-seconds 1200
gator work eval compare /tmp/gator-continuity-local-baseline/experiment.json \
  /tmp/gator-continuity-local-agent/experiment.json
```

Compare local and cloud models as separate experiments; provider and model
quality can change the result. On 2026-10-17 this machine has no Ollama
executable or downloaded Ollama model, so these commands have not been run.
