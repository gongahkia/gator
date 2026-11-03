# Personal-agent product implementation — November 2026

User-authorized scope: implement all fifteen findings from the 3 November audit,
verify each change, document its evidence and make atomic commits for rollback.
Starting checkout: `6814eabf2`. README files must remain untouched. This record
tracks the entire scope; completing a slice does not close its broader finding.

The user authorized **US$10 total for new live model/search evaluations** on
3 November. Existing October/November evaluation allowances are not reset or
reused. New calls need durable conservative reservations before dispatch, actual
usage afterward, and a cumulative ledger retaining failures and unknown charges.
Free fixtures do not establish live-model or user acceptance.

New allowance authority: `scripts/competitive-eval/authorization.json`; cumulative
ledger: `scripts/competitive-eval/results/budget.json`. The existing opt-in Work
evaluation transport is reused with this new directory, a US$9.50 admission
ceiling and durable unknown-request reservations. Conservative model ceilings
are US$6/M input and US$24/M output, covering cache-write and regional uplift.
The selected `gpt-5.6` alias and rates were checked against [the official model
page](https://developers.openai.com/api/docs/models/gpt-5.6-sol) and
[API pricing](https://developers.openai.com/api/docs/pricing) on 3 November.

## Requirements and acceptance

| Audit item | Implementation and required acceptance | State |
| --- | --- | --- |
| 1. Proactive usefulness | Fix pause/health and unnecessary-alert failures; complete evolving responsibility and coding repair/review journeys; unseen cases, human repair assessment and multi-day precision/miss evidence | In progress |
| 2. Availability | Supported always-on installation, discoverable enrollment, health and recovery; client-off work and host restart verified; precise local-resource ownership | Open |
| 3. Durable execution | Persist safe workflow boundaries, reconcile uncertain effects, resume eligible work without repeating confirmed effects; crash/update/provider-loss journeys | Open |
| 4. First value | Task-led defaults, model/account/search readiness and inline recovery; clean installation to useful result without developer intervention | Open |
| 5. Connected jobs | Complete calendar and email workflows, including threading, recipients, attachments, drafts, reviews, account scope, reconciliation and real-service acceptance | Open |
| 6. Research | Reviewed public discovery beyond preselected sites, sensible shared budgets and in-task scope expansion; unknown-source research with provenance and network protections | Open |
| 7. Browser | Bounded task/session permissions, private sign-in and useful session continuity; representative multi-page jobs with measured interruption count | Open |
| 8. Delivery | Durable notification outcomes/retry/reconciliation, channel choices and mobile delivery; disconnect, OS failure and actual-device acceptance | Open |
| 9. Conversation | Responsive questions and task routing during work, preserving owner-bound steering, approvals and resource limits | Open |
| 10. Memory/adoption | Scalable scoped reconciliation, outage/ambiguity behavior, realistic accumulated profiles, multilingual history and reviewed competitor-data import | Open |
| 11. Costs | Task/recurring money estimates, shared enforceable ceilings and visible unknown costs across model/search/delegation | Open |
| 12. Deliverables | Representative template revisions and independently reviewed briefs, sheets and decks; factual/layout defects and repair time recorded | Open |
| 13. Release/platform | Signed install/update/rollback lifecycle; supervisor drain/restart; acceptable Windows command isolation and native evidence | Open |
| 14. Privacy/assurance | Precise threat/data-flow model, platform credential protection, and adversarial cross-tool/content evaluations before expanding authority | Open |
| 15. Capability truth | Current shared capability descriptions for onboarding/help/model guidance; eliminate calendar contradictions and test user-visible claims | Open |

Acceptance depending on provider registrations, signing identities, physical
devices, deployment or independent human judgment stays explicitly unverified
until that evidence exists. Implement independent engineering work while those
acceptance steps remain open. Do not treat a green narrow test as product parity.

## Implementation evidence

### Paused scheduling and observed-success continuity (item 1)

The retained live watch report exposed two independent defects: `Due` ignored
`Enabled`, and pausing changed the digest used to match observed successful checks.
The scheduler and legacy claim path now reject disabled definitions. A separate
check-policy digest normalizes only the enable flag for evidence comparisons;
the reviewed-settings digest still binds pause/resume so concurrent edits are
rejected. Existing enabled-definition digests remain compatible.

Regression coverage uses persisted real job attempts and reopening stores. It
checks exact cron slots, missed-slot catch-up, stale claims, unchanged watermarks,
pause/resume success, and invalidation after substantive source/objective edits.
Validation: `go test ./internal/jobs ./internal/workhome` and both full package
race suites pass. Focused command integration tests matching
`Test(GoalWatch|ChangeOnly|ChatFollowup|ReviewedFollowup|DailyFollowup|Check)` pass.
`git diff --check` passes. No model or search requests were made for this slice.

Rollback: revert its code/documentation commit. No new persisted fields or data
migration are introduced. An already admitted check may finish after pause; this
change prevents future scheduler admission, not cancellation of active work.
