# Internal-agent live lifecycle

An opt-in test uses the actual Work service, suggestion acceptance command,
knowledge store and goal plans in a fresh synthetic profile. No connectors,
external account, browser or real user memory is loaded. Regular tests skip it.

Export `OPENAI_API_KEY` locally. Then, from the repository root:

```sh
GATOR_LIVE_MODEL=gpt-5.6 \
GATOR_LIVE_ENDPOINT=https://us.api.openai.com/v1/responses \
GATOR_LIVE_INTERNAL_REPORT="$PWD/internal-agent-report-new.json" \
go test ./cmd/gator -run '^TestLiveInternalAgentLifecycle$' -count=1 -v -timeout 12m
```

Choose an endpoint appropriate to the account. The example is the regional
endpoint used for these retained runs. The report path must not already exist.
The test budgets 100 aggregate requests and 300,000 tokens, checked between
requests (a response may cross the token budget); each Work phase
has a 12-request, 120-second limit. It stops after an execution failure and
retains a partial report. Credentials are read only from the process environment.

The workflow corrects an existing note, schedules an existing plan step, adds a
dependency/intentional overlap, starts fresh chats, applies reported completion,
and checks quiet versus overdue decisions. Suggestions are applied through the
real acceptance command, then state is inspected independently of model prose.
The two decision baselines get the same plan and the same model without tools.
They test judgment only, not persistence or an equivalent complete product.

Reports are synthetic. Keep failed runs alongside later passes. Unit tests cover
crash recovery, stale edits, scope isolation and scheduler change gates; live
checks complement those tests rather than replacing them. A handful of cases
is not evidence of real-world reliability or a competitor comparison.

## Continuity, recurrence and conflict evaluation

`TestLiveAgentContinuity` exercises the production Work tools and acceptance
command in isolated synthetic state. It checks weekly recurrence, completing
one occurrence without ending the series, cross-project blockers, exact-source
conflict findings and stale acceptance, plus a waiting watch retained across
reopening the store. A further phase seeds a previously checked ready decision
and verifies that the normal runner writes and checks a draft artifact, then
holds for owner review without marking the task done.

```sh
GATOR_LIVE_MODEL=gpt-5.6 \
GATOR_LIVE_ENDPOINT=https://us.api.openai.com/v1/responses \
GATOR_LIVE_CONTINUITY_REPORT="$PWD/continuity-report-new.json" \
go test ./cmd/gator -run '^TestLiveAgentContinuity$' -count=1 -v -timeout 12m
```

This test has an aggregate 80-request / 250,000-token budget (checked between
calls), 12 requests and 120 seconds per phase, and ten minutes overall. It loads
no real user memory or connected accounts. Acceptance is automated only for the
specific synthetic proposals. Browser access and delegation are disabled.

Retained October 30 reports:

- `openai-2026-10-30-continuity-1.json`: 12/13 checks, 15 requests,
  103,696 input + 1,733 output tokens, no tool errors. The recurrence assertion
  incorrectly compared a UTC-normalized timestamp to offset-preserving text.
  The runner now compares instants; the failed report is retained.
- `openai-2026-10-30-continuity-2.json`: 13/13 checks, 16 requests,
  111,676 input + 2,214 output tokens. One invalid plan proposal included `body`;
  the host refused it and the model corrected it. The tool description now
  explicitly says to omit `body` for plan edits. This was a successful workflow,
  not an error-free first attempt.

These short, directed workflows do not measure open-ended planning, broad
contradiction recall, or weeks of memory/proactive usefulness. The seeded ready
case tests execution from retained state, not autonomous goal discovery.
