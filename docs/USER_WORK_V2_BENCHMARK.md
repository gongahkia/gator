# User Work benchmark, twenty-one-task corpus

**Interface status:** Gator is a desktop app. Shell examples below are source-checkout developer operations with an explicit internal opt-in; see [development instructions](DEVELOPMENT.md). They do not install or enable a public CLI.

Recorded 2026-10-15. The product target for this iteration is a terminal-comfortable knowledge worker who wants to prepare a daily brief, a presentation, or an email draft from local files and saved web material. The primary interaction is an ongoing terminal conversation. Connected apps are a later benchmark layer. Coding is not a user job in this corpus.

The [dataset](../internal/eval/testdata/user-work-v2/dataset.json) has twenty-one tasks: the eight development probes from `user-work-v1`, nine new development tasks, and four held-out tasks. Every case uses a fixed local source folder so runs can be compared. The new source folders are synthetic and small; this corpus tests task coverage and specific failure modes, not production-scale document quality. The saved web pages test source interpretation, while live web retrieval remains a separate untested capability.

| Group | Cases | What the result must preserve |
| --- | --- | --- |
| Prior probes | `vendor-choice`, `meeting-actions`, `expense-audit`, `payout-reconciliation`, `missing-approval`, `untrusted-brief`, `plan-revision`, `ticket-cleanup` | Exact numbers, current source, omissions, source integrity, and revision constraints |
| Daily briefs | `morning-ops-brief`, `morning-sales-brief`, `release-status-brief`, `meeting-prep-brief`, `support-digest` | Current status, arithmetic, owners, deadlines, and source references |
| Saved web brief | `daily-market-watch` | Confirmed announcement scope versus an unsupported forum claim |
| Email drafts | `customer-escalation-email`, `renewal-email`, `launch-update-email` | Recipient-safe facts, next action, no unsupported promise or internal-only claim |
| Presentations | `weekly-metrics-deck`, `vendor-review-deck`, `risk-review-deck` | Valid PPTX, slide count, slide-level figures and decision facts |
| Conversation follow-up | `daily-brief-revision` | Preserve confirmed facts while removing internal details for a customer audience |

The benchmark now checks visible PPTX slide text instead of only checking that a ZIP-shaped `.pptx` exists. These checks are deliberately narrow. Passing them does not prove that a deck looks good, that every sentence is supported, or that it is ready to present. A [four-criterion review rubric](../internal/eval/testdata/user-work-v2/review-rubric.json) covers factual support, completion, audience fit, and manual rework. Presentation layout still requires visual human review.

## Current evidence

All 21 scripted calibration runs pass through the production Work service. Scripted turns contain the intended actions and therefore test fixture wiring, tool execution, artifact validation, and graders; they do **not** measure model capability. Live provider results must be recorded separately, including failures. Four cases are marked `held-out` and should not be used for routine prompt or harness iteration.

The first live `morning-ops-brief` produced the requested facts but initially failed a grader that required a `source/` citation prefix. The output named `overnight.md` instead; the grader now accepts that source reference. The original first-run score remains a failure in its report under `/tmp/gator-v2-ops-smoke-20261015`.

The first live pass on the eight new local-file development tasks used `gemini-3.5-flash-lite` and scored **3/8** with the original deterministic graders. Manual inspection judged **6/8 substantively correct**. Four failures were exact-phrase checks on correct output (`release-status-brief`, `support-digest`, `renewal-email`, and `launch-update-email`). `meeting-prep-brief` proposed a six-hour export option despite a maximum four-hour interval, and `customer-escalation-email` said replayed exports were delivered without source support. The latter passed the original grader and shows a false positive. The report is `/tmp/gator-v2-new-development-20261015`.

The first held-out pass scored **1/4** with the original graders. Manual inspection judged **3/4 substantively correct**: `vendor-review-deck` passed, `risk-review-deck` put its top risk on slide two instead of the grader's assumed slide one, and `daily-brief-revision` preserved the requested facts using different wording while omitting an incident ID the follow-up did not require. `weekly-metrics-deck` was a real failure: it placed metric facts only in speaker notes, leaving visible slides with titles alone. The report is `/tmp/gator-v2-heldout-first-20261015`. These four cases have now been inspected and are no longer unseen for later runs.

The first-pass findings led to narrower regex and slide-placement checks, a source-and-constraint instruction, a presentation tool schema that exposes slide body fields, and a renderer rule that rejects notes-only new slides. These changes need fresh live results; the original reports remain unchanged. The held-out evaluation process peaked at **24,141,824 bytes resident memory** (macOS `/usr/bin/time -l`, one 309-second run). This measures Gator only, not the desktop app comparison.

The first saved-web `daily-market-watch` output correctly distinguished the vendor announcement from an unsupported forum claim, but scored 0/1 because its grader required the word “unverified.” That grader now accepts equivalent uncertainty wording; the original report is `/tmp/gator-v2-market-smoke-20261015`. A fresh weekly-deck run after the renderer change put all metrics on visible slides. Its original slide-position checks still failed; the revised checks look across visible slide text while retaining a final-slide caveat check. That report is `/tmp/gator-v2-weekly-fixed2-20261015`. An intermediate run with an invalid presentation tool schema failed before model execution and is retained at `/tmp/gator-v2-weekly-fixed-20261015`; a schema parsing test now guards that boundary.

The first full 21-case live regression scored **11/21** under its original graders in 531 seconds and peaked at **25,931,776 bytes resident memory**. It is retained at `/tmp/gator-v2-full-regression-20261015`. The score mixes task and infrastructure failures: `plan-revision` hit repeated malformed Gemini calls; `meeting-prep-brief` again suggested a six-hour interval against a four-hour maximum; `morning-sales-brief` used the opposite valid date order; and after case fourteen, Gemini returned HTTP 429 with explicit retry delays, leaving several later cases without output. Gator previously retried those 429s immediately. It now honors a bounded `Retry-After` header or Gemini's “Please retry in” hint, with context cancellation. [Google's troubleshooting guide](https://ai.google.dev/gemini-api/docs/troubleshooting) recommends backoff for 429 responses. Repeated live verification of this change is still required.

Two live plan-revision trials still failed after retry guidance alone. Gator now also removes rich artifact writers that cannot produce the required output when a malformed call forces tool-surface reduction. **Both subsequent plan-revision trials passed**, including their follow-up edits (`/tmp/gator-v2-plan-narrow-20261015`). A stronger general instruction about checking every example against hard constraints produced one passing meeting-prep trial, then Gemini exhausted its free-tier **500-request daily quota** before the second trial could finish (`/tmp/gator-v2-meeting-constraint-20261015`). That error asked for an eight-hour wait. The runner now parses hour/minute/second hints and returns long quota resets to the caller without waiting inside an interactive task. Gator has no other provider credential configured in this environment, so the remaining live cases and repeatability checks are pending a quota reset or another configured provider.

## Acceptance criteria before a usefulness claim

1. Run all twenty-one tasks live with a fixed provider and model. Report first-attempt pass rate by family and inspect every failure. Keep the held-out split separate from development tuning.
2. Repeat live runs to estimate reliability. A single success on a short synthetic fixture is not a reliability rate.
3. Have independent reviewers score factual support, useful completeness, deck readability, email tone, and manual cleanup on representative outputs. The deterministic graders only cover selected facts.
4. Compare against a plain terminal workflow using the **same model, inputs, output requirements, and time budget**. Record elapsed time, model requests, cost, and reviewer preference.
5. Measure peak resident memory for Gator's interactive workflow and the desktop alternatives on the same machine and task. Current process snapshots alone cannot establish a lower-RAM claim.

## Commands

```sh
GATOR_INTERNAL_COMMANDS=1 go run ./cmd/gator work eval validate internal/eval/testdata/user-work-v2/dataset.json
GATOR_INTERNAL_COMMANDS=1 go run ./cmd/gator work eval run internal/eval/testdata/user-work-v2/dataset.json \
  --id user-work-v2-scripted --split all --report-dir /tmp/gator-user-work-v2-scripted
GATOR_INTERNAL_COMMANDS=1 go run ./cmd/gator work eval run internal/eval/testdata/user-work-v2/dataset.json \
  --id user-work-v2-live-development --split development --live \
  --provider PROVIDER --model MODEL --max-model-requests 650 \
  --timeout-seconds 2400 --report-dir /tmp/gator-user-work-v2-live
```

Use a new report directory per run. Live runs require a configured provider credential and enough request quota for the chosen split; a 21-case run can exceed a free-tier daily allowance. Keep provider failures separate from correctness scores.
