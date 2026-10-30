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
