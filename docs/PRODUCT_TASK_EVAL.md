# Product task comparison: brief, email, and deck

**Interface status:** Gator is a desktop app. Shell examples below are source-checkout developer operations with an explicit internal opt-in; see [development instructions](DEVELOPMENT.md). They do not install or enable a public CLI.

This is a **live-model evaluation plan and attempt log**, not a product-quality score. The three selected cases in `internal/eval/testdata/user-work-v4/dataset.json` use synthetic but task-shaped source folders. They cover a recovered incident versus open work, a customer-safe email versus internal notes, and a slide deck with a measurement caveat. These are useful smoke tasks; actual user material and independent reviewers are still needed for a market claim.

## Matched procedure

Use the same provider and model for both arms. Run Gator's Work service first and the direct one-turn baseline with the exact case objective and all selected source files. Use fresh report directories. Preserve failed trials and provider errors. The baseline has no tools and writes a text response; score factual draft quality for both arms, and score file completion separately. A text slide outline is not a completed PPTX deck.

```sh
DATASET=internal/eval/testdata/user-work-v4/dataset.json
GATOR_INTERNAL_COMMANDS=1 go run ./cmd/gator work eval run "$DATASET" --live --provider gemini --model gemini-3.5-flash --split all --case morning-ops-brief --case customer-escalation-email --case weekly-metrics-deck --max-model-requests 40 --timeout-seconds 900 --report-dir /tmp/gator-product-work-1 --id product-work-1
GATOR_INTERNAL_COMMANDS=1 go run ./cmd/gator work eval baseline "$DATASET" --provider gemini --model gemini-3.5-flash --case morning-ops-brief --case customer-escalation-email --case weekly-metrics-deck --timeout-seconds 900 --report-dir /tmp/gator-product-direct-1
```

If the provider has a small quota, run each `--case` separately and retain the case ID and requested model in the report. Do not treat provider 429 or 503 as a task-quality failure. A completion score requires both arms to receive model responses. The configured Gemini model name may resolve to a different provider model ID; record the actual ID from any provider error or response metadata before making cost or model-specific claims.

## Review sheet

Blind the reviewer to the arm. For each draft, record factual omissions, factual corrections needed, unsafe claims, and whether the first output would be usable with no edits, light edits, or a rewrite. Count corrections as discrete changes to facts, omissions, audience tone, or structure; record them in a notes file with the source line. Record elapsed wall time from each report, and note any time spent on manual repair separately. Ask one follow-up in the same conversation and mark it useful only if it moves the task forward without reintroducing a corrected error.

| Case | First-turn completion check | Follow-up to test | Main risk |
| --- | --- | --- | --- |
| `morning-ops-brief` | Correctly separate INC-214 recovery from open T-91 work, name owner and next meeting, cite files | “What should I raise before the Arbor recovery call? Keep open work separate from recovered incidents.” | Inventing closure or confusing meeting time |
| `customer-escalation-email` | Draft with subject, impact, recovery and next update; no internal-only details or claim that the warning is fixed | “Shorten this for the customer and tell me which uncertain claim you kept out.” | Leaking internal analysis or overpromising |
| `weekly-metrics-deck` | Three-slide PPTX for Gator; slide-by-slide draft for direct chat; signups, activation, churn and caveat | “Revise the executive takeaway so the tracking caveat remains a caveat, not a confirmed cause.” | Treating measurement caveat as a cause |

For each arm and case, fill: `provider outcome`, `first-turn content complete?`, `artifact complete?`, `corrections required`, `elapsed seconds`, `follow-up useful?`, and `reviewer notes`. If either arm fails at the provider layer, leave comparative fields unscored.

## Attempt on 18 October 2026

Gator's `morning-ops-brief` first turn returned Gemini HTTP 429 before a draft. The provider reported a 20-request free-tier limit for `gemini-3.6-flash` and a retry delay of about 21 hours 34 minutes. The configured request model was `gemini-3.5-flash`; that alias difference needs confirmation when quota is available. The failure was recorded in `/tmp/gator-product-brief-20261018/experiment.json`: 0/1 completed, one provider error, 1,077 ms to failure, one attempted model request. No content corrections, task completion time, or follow-up usefulness can be scored. The email, deck, and direct baseline calls were not attempted after the quota response, to avoid treating a known quota block as additional task failures.

## Next-action usability check

The TUI now presents model setup before the first prompt and a `Next up` list for proposals, unread results, running work, active goals, and recent work. Automated terminal-model tests check that a new user can see `/model` and a starter task, that `/model` opens setup, that a typed first task survives setup, and that the home screen exposes the existing routes. These are discoverability checks on the interface; they are **not observed human behavior**.

For a human check, give at least three new users an empty state and a seeded state without teaching commands. Ask them to (1) configure a model, (2) find a pending proposal, (3) find an unread follow-up result, (4) resume an active goal, and (5) review a recent draft. Record first route chosen, success without help, time to action, wrong turns, and words they expected on the home screen. Change labels or ordering only after those observations; repeat the same tasks with new users.
