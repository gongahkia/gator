# Agent continuity v2 — implementation and acceptance plan

Status: active. Scope approved by the user on 30 October 2026: implement the
five gaps below. Passing a subset does not complete this plan. Existing adapters
at baseline `908de7fee` run isolated one-shot jobs and retain summaries/patches;
they do not provide native ongoing sessions.

## 1. Two-way external-agent sessions

- Retain native conversation identity and isolated working files across turns.
- Send follow-up messages, steer active work where supported, interrupt a turn,
  and surface/answer clarification requests through Gator.
- Bind session access to the originating profile, source and permission policy.
  Resumption must not silently broaden access or import unrelated CLI sessions.
- Recover retained sessions across host restarts without blindly replaying an
  ambiguous in-flight mutation. Distinguish acknowledgement from completion.
- Implement Codex app-server, Pi RPC and Claude's documented conversation/input
  protocols; record unsupported operations explicitly rather than pretending.
- Acceptance: protocol fixtures for every adapter, isolated live multi-turn
  Codex/Pi tests with the supplied OpenAI key, cancellation/steering tests,
  permission-boundary tests, and retained session recovery tests. Claude live
  evidence requires an installed CLI and an Anthropic credential; never label
  protocol fixtures as a live result.

## 2. Bounded completion loops

- Persist an explicit work objective and acceptance checks.
- Run implementation, deterministic verification and independent review.
- Feed concrete failures back to the same worker; retain each attempt and stop
  on success, exhausted budgets, repeated failure or a question requiring input.
- Preserve conflicting/failed patches as evidence without applying them to the
  user's original files. A successful process exit is not task success.
- Acceptance: an initially failing implementation repaired in a later turn;
  false-success, retry-exhaustion, conflicting-edit and cancellation cases.

## 3. Responsive conversation and task threads

- Move delegated execution outside the lifetime of a foreground chat turn.
- Let the user keep chatting, inspect a task, answer its question or change its
  instructions without blocking unrelated conversation.
- Show retained task threads/results in the terminal and route ambiguous
  follow-ups to an explicit selection instead of guessing the target.
- Acceptance: real backend/terminal journeys with two simultaneous tasks,
  unrelated foreground chat, a routed follow-up, and a pending clarification.

## 4. Durable background execution

- Own workers through a durable background supervisor with exclusive leases,
  process health checks, bounded recovery and idempotent command admission.
- Resume acknowledged work safely after crashes; reconcile uncertain effects
  before another attempt. Preserve cancellation and pending questions.
- Provide an installable always-on-host workflow using the existing companion;
  explain which host must remain available and surface disconnection honestly.
- Acceptance: supervisor kill/restart, stale lease, duplicate request, cancelled
  work, service restart and remote reconnection tests. A laptop-only test does
  not prove execution while that laptop is asleep.

## 5. Proactive judgment grounded in evolving memory

- Use existing memories, notes, plans, source revisions and reported outcomes.
- Prioritize useful changes, suppress repeated/stale suggestions, honor quiet
  preferences and keep decisions separate from user task completion.
- Learn from explicit acceptance/dismissal/correction without turning model
  guesses into personal facts. Preserve correction/forgetting/scope boundaries.
- Acceptance: multi-day simulated histories plus live model scenarios covering
  useful intervention, silence when nothing changes, correction, forgetting,
  conflicting facts, cross-goal scope and changed priorities. Report failures,
  denominators and limits; do not claim competitor parity from smoke tests.

## Delivery discipline

Use atomic commits per reviewable change. Test boundaries and real journeys,
then run affected suites and platform CI. Live tests use synthetic data and
separate temporary profiles; credentials stay out of logs, fixtures and commits.
Connectors remain deferred. Keep this plan active until every item has direct
implementation and verification evidence; document unavailable external evidence.

Protocol references (checked 30 October 2026):
- https://learn.chatgpt.com/docs/app-server
- https://pi.dev/docs/latest/rpc
- https://code.claude.com/docs/en/headless
