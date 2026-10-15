# User Work benchmark, thirty-task corpus

Recorded 2026-10-16. [Version 3](../internal/eval/testdata/user-work-v3/dataset.json) preserves the 21-case [version 2 corpus](USER_WORK_V2_BENCHMARK.md) and adds nine tasks for a terminal-based knowledge worker. The prior live reports remain version 2 results; they are not scores for this new manifest. The new fixtures are synthetic and local. They exercise decisions and deliverables from source material, not live web retrieval or connected apps.

| New case | Split | New user job and failure mode |
| --- | --- | --- |
| `inbox-triage` | development | Rank competing messages, recognize a resolved case, and draft a bounded status reply without inventing a fix time. |
| `calendar-conflict` | development | Convert UTC to SGT, check full meeting duration against a hold and availability, and propose a workable alternative. |
| `policy-change` | held-out | Explain an effective-date change while preserving a grandfather rule and unchanged requirements. |
| `survey-analysis` | held-out | Classify eight free-text comments, count each once, and avoid calling four of eight a majority. |
| `budget-scenarios` | development | Separate monthly cost from first-month cash, apply two caps, and recommend the feasible plan. |
| `file-catalog` | development | Select current approved files over superseded drafts and produce a usable index. |
| `transcript-reversal` | development | Extract a final decision after an earlier proposal is withdrawn and leave an unapproved date unknown. |
| `faq-drafting` | development | Answer customer questions from a feature spec without turning a review date into a launch promise. |
| `stakeholder-pack` | held-out | Create consistent internal and customer outputs from one incident while keeping private analysis out of the customer draft. |

The deterministic graders check concrete fields and facts, including forbidden claims and leaks. They do not score clarity, tone, layout, or whether an actual user would save time. The [review rubric](../internal/eval/testdata/user-work-v3/review-rubric.json) covers factual support, completion, audience fit, and manual rework. The three new held-out cases should stay out of prompt tuning. The four inherited version 2 held-out cases were already inspected in the earlier run and are not unseen cases.

## Status

All 30 cases pass `TestUserWorkV3CorpusThroughProductService`; scripted runs validate fixture loading, product execution, artifact contracts, and graders. These scripts include reference actions and therefore do not measure model performance. The version 2 live baseline remains 11/21 on the first complete attempt, with task and provider failures mixed; see the version 2 report for details. No live version 3 score is claimed. Gemini quota was exhausted in the prior run, and the user requested leaving live trials pending until it resets.

## Run after provider quota resets

```sh
gator work eval validate internal/eval/testdata/user-work-v3/dataset.json
gator work eval run internal/eval/testdata/user-work-v3/dataset.json \
  --id user-work-v3-scripted --split all --report-dir /tmp/gator-user-work-v3-scripted
gator work eval run internal/eval/testdata/user-work-v3/dataset.json \
  --id user-work-v3-live-development --split development --live \
  --provider PROVIDER --model MODEL --max-model-requests 900 \
  --timeout-seconds 3600 --report-dir /tmp/gator-user-work-v3-live-development
```

Use a fresh report directory for each run. Report provider errors separately from task errors; inspect live output for unsupported claims that narrow graders may miss. Only then run `--split held-out` and have a reviewer inspect representative deliverables. A pass on the scripted reference actions is not a usefulness claim.
