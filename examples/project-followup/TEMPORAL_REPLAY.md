# Sequential follow-up replay

This developer harness runs the production watch input gate, Work execution,
decision validation, immutable attempt recording, and inbox delivery in a fresh
local profile. It is a repeatable test mechanism, not evidence that a live model
reliably monitors real work.

`episodes.json` defines ten observations of one project. Each observation copies
one bounded file into the isolated workspace as `project.md`. Future source
versions, the script, and the ground-truth event list stay outside that workspace.
The model receives the same continuing watch conversation as normal scheduled
checks. The harness does not run a supervisor or send desktop notifications.

| Episode | Purpose |
| --- | --- |
| Initial | Establish a quiet baseline. |
| Deadline | Surface an earlier reply deadline. |
| Unchanged | Exercise the production unchanged-input skip without a model call. |
| Noise | Re-evaluate changed text and stay quiet for a formatting update. |
| Provider failure, recovery | Retain a failed model call, then report a changed owner. |
| Source failure, recovery | Temporarily make the disposable source unavailable, then report confirmed access. |
| Tool failure, recovery | Execute a failing file read and cancel that check, then report reduced capacity. |

Fault injection remains explicit in both scripted and live modes. These cases
test recovery wiring; they do not measure real provider failure frequency.
Every scheduled attempt, including an unchanged skip or injected failure, is
retained under the report's `state/gator/jobs` directory. The aggregate report
records each durable inbox ID; its retained inbox entry links to the replay's
originating conversation, identified by the report ID.

## Run without a provider

Use a source checkout. The internal command opt-in does not restore the retired
public CLI. Set disposable bootstrap paths so runtime startup maintenance cannot
touch your ordinary profile. The report directory itself must not exist yet.

```sh
replay_root=$(mktemp -d)
GATOR_INTERNAL_COMMANDS=1 \
GATOR_CONFIG_DIR="$replay_root/config" GATOR_STATE_DIR="$replay_root/bootstrap" \
go run ./cmd/gator work eval watch run examples/project-followup/episodes.json \
  --report-dir "$replay_root/report"
```

The default provider is an in-process script. No model credentials, external
accounts, network calls, or external mutations are needed. Reusing an existing
report directory is rejected. Use a fresh directory for every comparison.

`watch validate DATASET` checks ordering, material-event windows, source bounds,
and fixture tool restrictions. `watch score REPORT` reports execution counts
and leaves subjective quality unreviewed.

## Use a selected provider later

The same `run` command accepts `--live --provider PROVIDER --model MODEL
--max-model-requests N --timeout-seconds N`. All four values are required for
live execution. Model requests share one budget across episodes and retries.
The provider configuration and credentials must be deliberately configured in
the selected bootstrap profile. Do not copy credentials into fixtures or reports.

Ordinary episode scripts are ignored in live mode. The agent reads current
sources and decides whether to surface an update. Injected failures still use
their controlled test mechanism. No connected accounts, web origins, code
execution, sending, or publishing are added by this replay. The Work contract
permits inspection only. The model factory is injectable for offline integration
tests, so tests never need to call a real provider.

If the overall wall budget expires, completed observations remain on disk with
`complete: false`. Future, unobserved event windows remain unassessed.

## Review outputs and score quality

Execution success does not establish relevance or source support. An independent
reviewer should read the current source, retained preceding observations, and
the actual alert. For each surfaced **completed** alert, copy its episode ID,
attempt ID, and output digest from `watch-report.json` into a separate file:

```json
{
  "version": 1,
  "report_id": "COPY_ACTUAL_REPORT_ID",
  "evidence": "human",
  "reviewer": "reviewer-pseudonym",
  "items": [
    {
      "episode_id": "deadline",
      "attempt_id": "COPY_ACTUAL_ATTEMPT_ID",
      "output_sha256": "COPY_ACTUAL_OUTPUT_DIGEST",
      "reported_event_ids": ["deadline-earlier"],
      "useful": true,
      "actionable": true,
      "note": "The source moved the deadline to 3 November; the alert identifies that change and a concrete next step."
    }
  ]
}
```

An empty `reported_event_ids` array explicitly means the alert reported none of
the material events in this dataset. Use `useful: false` for irrelevant or
incorrect alerts. `actionable` asks whether the recipient can tell what to do
next. Omit either boolean when that judgment was not made; omissions never
become negative ratings. Every review needs a source-supported note. Reading,
delivery and resolution are not quality judgments.

Score with `work eval watch score REPORT --reviews REVIEWS_JSON` under the same
internal command opt-in. The scorer binds reviews to the exact report, attempt,
and output digest. It rejects future/unknown event IDs and duplicate reviews.
Tests label their synthetic judgments `scripted_fixture`; those labels cannot
be applied to a live report. Even human review of a scripted report remains
explicitly marked as scripted execution.

| Metric | Numerator / denominator |
| --- | --- |
| Useful alert precision | Alerts explicitly judged useful / alerts with a useful judgment. |
| Actionability | Alerts explicitly judged actionable / alerts with an actionability judgment. |
| Missed material changes | Material events not reported by their deadline / assessable material-event windows. |
| Repeated alerts | Reviewed alerts mentioning at least one already-reported material event / reviewed alerts whose repetition status is assessable. |
| Mean detection delay | Sum of first-notification delay for assessed event IDs / unique detected event IDs with an assessed first notification. |

Every metric includes the raw numerator and denominator; an empty denominator
produces `null`. A material-event window is unassessed when all observations
failed, its alert outputs remain unreviewed, or the replay ended before its
deadline. A completed quiet decision or unchanged skip can establish a miss
against the dataset's explicit ground truth. A late notification remains a
deadline miss and contributes its actual delay. Health notices are excluded
from alert-quality denominators. Provider, tool, source, limit and other
execution failures are counted separately, including recovered tool/provider
error counts.

An earlier unreviewed alert might already contain a later-reviewed event.
Those events are excluded from first-notification delay, and uncertain repeat
judgments are excluded from the repeat denominator. The report lists these
unassessed cases instead of assuming the earlier alert contained nothing.

Episode offsets are **simulated elapsed time**. Detection delay measures the
fixture's observation schedule, not real notification latency or uptime. Real
multi-day monitoring, unseen source changes, and independently reviewed live
outputs are still needed before making product-quality claims.
