# User Work benchmark, first iteration

Recorded 2026-10-15. The corpus is
[`internal/eval/testdata/user-work-v1/dataset.json`](../internal/eval/testdata/user-work-v1/dataset.json).
It tests ordinary non-coding requests through the same `work.v1` service as
Gator's terminal UI, CLI, and jobs. Its eight source folders are synthetic and
small enough for repeatable local runs. They are task probes, not a claim that
Gator is useful at production scale.
All eight cases are now marked `development`: each was inspected during the
first live pass, so none is an unseen holdout for future comparisons.

## Cases and intended outcome

| Case | User job | Deterministic checks |
| --- | --- | --- |
| `vendor-choice` | Compare superseding quotes against budget and deadline | Vendor, exact first-month totals, delivery time, and selected source |
| `meeting-actions` | Extract confirmed decisions and owner actions, omit tentative work | Dates, owner-specific tasks, and absence of an unassigned person |
| `expense-audit` | Apply a policy to duplicate, rejected, and disallowed claims | Exact USD total and sorted included/excluded IDs |
| `payout-reconciliation` | Reconcile two CSV exports into a workbook and memo | Checked total, row discrepancy, duplicate status, and source paths |
| `missing-approval` | Answer from incomplete notes without inventing approvals | Explicit unknowns and the one supported month |
| `untrusted-brief` | Extract facts from a file containing an instruction attack | Correct owner/date and no attacker-selected text |
| `plan-revision` | Revise a prior plan while keeping earlier constraints | Final owner, date, and changed channel |
| `ticket-cleanup` | Deduplicate and normalize a ticket export | Exact sorted output CSV |

The new `json_pointer_equals`, `json_pointer_contains`, `json_pointer_absent`,
and `file_excludes` graders check structured fields and known unwanted content.
They let the benchmark avoid requiring one sentence where several wordings
would be useful. Deterministic checks still cannot judge whether every piece
of narrative reasoning is supported; a reviewer must inspect that separately.

## What ran

The corpus validates and its eight scripted calibration cases pass through the
real Work service. These turns are fixed and do **not** measure model quality.
The existing 24-case Work corpus also passes with coding explicitly enabled
for its six coding cases.

One live attempt per new case used `gemini-3.5-flash-lite`. The eight first
attempts were spread across two reports because the first three ran before the
remaining five. Four passed the initial automated graders. Inspection of the
actual outputs found six substantively correct results: vendor choice, meeting
actions, expense audit, payout reconciliation, missing approval, and the
untrusted brief. `meeting-actions` and `untrusted-brief` initially failed only
because their graders required an unspecified JSON nesting, exact article, or
`source/` prefix. Those graders were corrected after the run; the original
scores are not retrospectively rewritten.

`ticket-cleanup` timed out at 180 seconds after 33 model requests. The manager
had sent a simple CSV transformation to the Code patch specialist; no
`clean.csv` was produced. A same-model ablation with the Code role disabled
passed in 45 seconds and six requests. After ordinary Work stopped advertising
Code by default, an unmodified product-path retry passed with the exact CSV in
five requests. Its 136-second latency shows request count improved; one trial
each cannot establish a stable speed gain.

`plan-revision` failed before a usable first plan was produced. Repeats exposed
Gemini's `MALFORMED_FUNCTION_CALL` finish reason. Gator now records finish
reasons and retries empty safe-to-retry turns within its existing three-attempt
limit. The plan still failed after those identical retries. The
evaluator now stops follow-ups after a failed parent instead of attempting a
revision of an incomplete result.

### Provider and tool-surface follow-up

The same `plan-revision` case was rerun to isolate the failure. With delegation
enabled and Gemini's `VALIDATED` function-calling mode, it still failed on a
malformed call after reading the notes. Three independent trials with
delegation disabled passed both the initial plan and follow-up revision.
Google [documents `VALIDATED` mode](https://ai.google.dev/gemini-api/docs/generate-content/function-calling#function-calling-modes)
as reducing malformed calls; it was insufficient for this case.

Gator now classifies malformed provider function calls separately from rate
limits and other transient errors. For ordinary Work, the runner removes
optional specialist controls after the first malformed call and retries the
same turn with local file and artifact tools still available. The reduced tool
set remains in effect for that run, and a `tool_surface_reduced` event makes the
recovery visible in traces and evaluation metrics. Explicit Code Work keeps its
specialist tools.

Two live trials with normal delegation enabled both encountered the malformed
call, both used the new fallback, and both passed the initial plan and revision
with valid `plan.json` artifacts. These are small, repeated trials on the same
fixture and model; they demonstrate recovery for this failure path, not a
general provider reliability rate. The report is under
`/tmp/gator-plan-validated-0GybIt/fallback` and is ephemeral.

One live regression trial each for `expense-audit` and `ticket-cleanup` also
passed with `VALIDATED` mode and the new fallback available. Neither needed a
fallback. Their report is under `/tmp/gator-tool-regression-kBZJyq/report`.

Local live reports from this session are under `/tmp/gator-user-work-live-0083zx12`,
`/tmp/gator-user-work-remaining-nvrbi99m`, and `/tmp/gator-ticket-ablation-ejfi4flm`.
These paths are ephemeral. The benchmark used the configured Gemini credential
and did not run a competitor baseline. It used one attempt per case, so none of
these counts is a reliability estimate.

## Failure buckets and next work

1. **Routing and task fit — improved.** Ordinary file Work no longer exposes
   the Code patch specialist; explicit typed Code Work still can. The CSV case
   moved from timeout to correct output. Continue testing whether tool choice
   stays focused on larger file tasks.
2. **Provider turn reliability — improved.** The plan case now completes after
   a malformed-call fallback. Measure malformed-call and fallback rates on
   previously unseen tasks and another provider; inspect tool schemas and
   prompt size before treating this case as a general fix.
3. **Evaluation quality — improved.** Two first-run false negatives showed why
   exact prose and unspecified JSON shapes make poor graders. Keep answer checks
   tied to fields, numbers, omissions, and source integrity. Add human review
   for support and practical usefulness.
4. **Grounding — open.** An earlier pilot extracted a one-line storage decision
   correctly but invented reasons for it. Add more cases where unsupported
   rationales can be detected independently, including real multi-page files.
5. **End-to-end usefulness — unproven.** These small fixtures do not show that
   Gator saves users time. Run the same ten or more previously unseen tasks
   through Gator and a terminal baseline using the same model and inputs;
   blind-score correctness, unsupported claims, manual cleanup, elapsed time,
   and cost.

For now, the concrete product hypothesis is: Gator should help terminal users
finish messy local file work without losing track of sources or changing the
originals. The corpus makes that claim testable; it does not establish a
market advantage.

## Running the corpus

```sh
gator work eval validate internal/eval/testdata/user-work-v1/dataset.json
gator work eval run internal/eval/testdata/user-work-v1/dataset.json \
  --id scripted-user-work --split all --report-dir /tmp/gator-scripted-user-work
gator work eval run internal/eval/testdata/user-work-v1/dataset.json \
  --id live-user-work --split all --live --provider PROVIDER --model MODEL \
  --max-model-requests 100 --timeout-seconds 1200 \
  --report-dir /tmp/gator-live-user-work
```

Use a new report directory for every run. Live budgets are examples, not a
guarantee that a provider quota will permit completion. See
[`WORK_EVALUATION.md`](WORK_EVALUATION.md) for report fields and rubric handling.
