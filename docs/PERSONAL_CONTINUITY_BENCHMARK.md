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
