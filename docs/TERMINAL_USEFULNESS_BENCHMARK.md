# Terminal usefulness pilot

Run on 2026-10-15 from commit `1914a5516` on macOS. This is a small product
pilot, not a claim of comparative model quality or general user usefulness.

## Question and method

Can a user give Gator ordinary document and table tasks in the terminal and get
an accurate result without repairing it? I selected five non-coding cases from
the checked-in `work.v1` corpus: conflicting sources, missing evidence, document
extraction, duplicate keys, and decimal precision. All inputs were tiny synthetic
fixtures. The scripted run used the repository's fixed model turns; the live run
used Gator's configured Gemini credential. Results were inspected against each
source and objective, in addition to the repository's deterministic graders.

| Run | Result | Interpretation |
| --- | --- | --- |
| Scripted, five cases | 5/5 grader passes | The execution and grading machinery handled the prescribed turns. This does not measure model usefulness. |
| Live, `gemini-3.5-flash`, five cases | 0/5 grader passes | The Gemini adapter sent tool results with the unsupported `tool` role, and later requests hit the provider's free-tier quota. This run is invalid as a model-quality measure. |
| Live retry, `gemini-3.5-flash-lite`, three cases | 1/3 grader passes | The adapter was fixed and all three tasks executed, but automatic grading and manual review disagree about what succeeded. |

The live retry used one attempt per case, no connected services or web research,
and a bounded request and wall budget. It did not compare Gator with another
terminal agent. The retry used a different model from the initial live run, so
the two live scores are not a before/after quality comparison.

## Live retry by task

| Task | Time / model requests | Observed result | Assessment |
| --- | --- | --- | --- |
| Identify conflicting Q1 revenue figures and cite both files | 23.5 s / 7 | Brief correctly says 100 versus 120 and cites both paths. The grader failed only because it required the literal phrase `Sources disagree`. | Useful result; grader false negative. |
| Extract a one-line plan and explain the storage decision | 67.2 s / 7 | Correctly extracted `Retain local reports`, then asserted unsupported reasons about boundaries, network dependency, integrity, and security. | Partial result; the passing grader missed unsupported rationale. |
| Reconcile duplicate CSV keys, produce workbook and memo, then inspect prior workbook | 106.9 s / 13 | Produced a workbook with the checked cell correct and a memo; also produced an oddly named extra file. The memo was vague, omitted the grader's source digest, and the follow-up did not inspect `previous/checked.xlsx`. | Partial result; continuation and output quality need work. |

The retry's automatic 1/3 score is therefore not a sound summary of user
usefulness: one useful result failed a literal-word check, while one factually
unsupported explanation passed. The initial live failures also exposed a real
product defect. The Gemini adapter now encodes tool responses with the `user`
role shown in [Google's function-calling documentation](https://ai.google.dev/gemini-api/docs/generate-content/function-calling),
and its focused tests pass.

## Product reading

The most concrete potential value is **doing file-heavy terminal work in one
place**: inspect local material, transform or reconcile it, and produce a usable
result while preserving the original files. This pilot demonstrates that Gator
can complete a small conflicting-source brief and produce a checked workbook.
It does not establish reliable end-to-end usefulness or an advantage over Claude
Cowork, Pi, or a model plus shell tools. The current emphasis on manifests and
verification describes a safety property; users still need the answer and the
file to be substantively right.

Before adopting a stronger pitch, run at least 10 realistic, previously unseen
non-coding tasks across research, messy tables, document creation, and follow-up
work. Use the same inputs and model with Gator and a terminal baseline, and
have a reviewer score task completion, factual support, cleanup needed, time,
and cost without knowing which tool produced each output. Include source files
large enough to expose the actual workflow friction and report failures and
quota limits separately.

## Reproduction notes

The selected corpus cases came from `internal/eval/testdata/work-v1/dataset.json`.
The local experimental data and generated outputs were retained in
`/tmp/gator-terminal-benchmark-f6a4dz0t` for this session. That path is
ephemeral. The main commands were `gator work eval validate DATASET` and
`gator work eval run DATASET --live --provider gemini --model
gemini-3.5-flash-lite --max-model-requests 30 --timeout-seconds 300
--report-dir DIR`; see `docs/WORK_EVALUATION.md` for the full syntax.
