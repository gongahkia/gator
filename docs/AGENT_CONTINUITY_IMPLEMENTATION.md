# Agent continuity implementation

Requested 30 October 2026. Preserve the general-purpose CLI direction and local
tracking without mandatory connectors. Implement the five product gaps with
atomic commits and bounded, synthetic OpenAI evaluations. No real-account
sign-in, external messages or remote deployment are authorized by this work.

## Deliverables

1. Durable next-action coordination through the existing job supervisor: checked
   continuation state, wake times, blockers, restart-safe resumption and existing
   authority/budget limits. A model cannot mark user work complete by inference.
2. Cross-record reconciliation: inspect scoped notes/plans together, retain
   source-linked conflicts for review, and correct through existing revision guards.
3. Recurring native blocks and cross-project dependencies, with bounded expansion,
   timezone semantics, cycle detection and explicit scope boundaries.
4. Remote companion recovery and installation improvements that can be exercised
   locally. Actual host deployment and physical device-off operation remain separate.
5. Local regression checks and live synthetic evaluations recording failures,
   usage and latency; no superiority or production reliability claims from fixtures.

## Progress

- Baseline: `1151e95db`; working tree clean.
- Implementation underway. Record completed behavior and verification here as it lands.
