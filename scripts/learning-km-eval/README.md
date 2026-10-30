# Learning, writing and project knowledge probes

Explicit live evaluation with synthetic sources and isolated state. The runner
uses the real Work service with delegation disabled, then a same-model text-only
baseline with the same files, objectives and conversational follow-ups. It also
asks the foreground agent to update an existing saved note and inspects the
actual store. No real profile, external account or message recipient is used.

```sh
# Make OPENAI_API_KEY available in the child process; never commit it.
go run ./scripts/learning-km-eval --provider openai --model gpt-5.6 --endpoint https://us.api.openai.com/v1/responses --report-dir /tmp/NEW-learning-km-report
# A focused rerun:
go run ./scripts/learning-km-eval --provider openai --model gpt-5.6 --endpoint https://us.api.openai.com/v1/responses --case learning-dialogue --skip-note-probe --report-dir /tmp/NEW-focused-report
```

The directory must not already exist. Calls are bounded by 160 aggregate model
requests, 400,000 tokens and 15 minutes; each Work phase has its own limits.
Reports preserve source/dataset hashes, model, usage, latency, failures, retained
artifacts and pending note proposals. Credentials are environment-only and never
written to reports. Real team/Jira accounts are not tested.

The automated graders cover selected facts, corrected project fields, Markdown
structure, internal-detail exclusion and produced artifacts. They cannot certify
taste, useful teaching, learning retention, robust permissions or team knowledge
management. Inspect the artifacts, follow-ups and source files. The baseline
produces draft text; artifact completion is scored separately. Shared file and
identity services are not simulated into existence.

`results/openai-2026-10-30` preserves the first complete run. Original reports
retain their absolute disposable-state provenance; portable final artifacts are
under `gator/` and text baseline turns under `direct/`. The same report includes
a separate synthetic acceptance of the pending note proposal via the actual
CLI. Acceptance created a new note rather than updating the existing one.

The first learning comparison had an ambiguous follow-up: the direct model had
asked a changed-base-rate practice question, so its different numerical answer
was mathematically valid. Its initial review has an explicit amendment. The
corpus now specifies the original 20/980 example and gets a focused rerun; do not
use the original learning review to claim a Gator advantage. Other initial task
results remain unchanged. The initial raw baseline per-turn usage omitted request
counts; there are six recorded calls. The runner now explicitly records one
request per direct call. Provider token counts are unchanged.

This is a three-case authored probe, one attempt per task, not a representative
benchmark or proof of superiority. The initial split labels are retained for
provenance; the cases have now been inspected. See the product assessment in
[LEARNING_KM_ASSESSMENT.md](../../docs/LEARNING_KM_ASSESSMENT.md).
