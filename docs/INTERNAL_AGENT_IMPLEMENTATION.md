# Gator's internal agent: implementation and verification

30 October 2026. User direction: retain a general-purpose terminal agent. Make
personal tracking useful with user-provided updates and no external connectors.
External integrations can return to the roadmap after the internal primitives
are dependable. OpenAI model calls remain authorized for isolated live tests.

## Existing primitives and actual gaps

| Primitive | Present | Gap to close |
| --- | --- | --- |
| Goals and plans | Stable goal/step IDs, task status, deadlines, evidence, review times, revision-guarded changes | Main-chat access to a coherent local agenda; inspect/update a selected project without manually changing chats |
| Scheduling | One-off reminders, cron jobs, a local supervisor, missed-run policy, quiet hours, retained Inbox | Explain user-maintained state and host availability; prove due/quiet/completed behavior without connected sources |
| Progress | Pending/doing/blocked/done plan steps and reviewed proposals | Task dependencies, scheduled time blocks, and correct summaries after user corrections |
| Notes | Local Markdown, guarded owner edits, scoped retrieval, reviewed new notes | An agent's accepted update currently creates a duplicate instead of editing the inspected note |
| Memory | Evidence-linked capture, corrections, withdrawals, history filtering | Broader real use and relevant retrieval; user preferences are not task records |
| Continuity | Durable chat queues, separate running chats, origin-linked updates, experimental remote companion | Reliability evidence for complete internal workflows; remote deployment is separate work |

Reuse the existing goal/plan, knowledge, jobs and Inbox stores. Do not introduce
a second independent task database that can disagree with the goal plan. A user
update is an observation, not evidence that an external action occurred. Stored
status stays as last reported until new information arrives. A deadline passing
does not imply completion, and silence does not imply progress.

## Connector inventory

This inventories built-in code, not the user's configured accounts or credentials.

| Family | Available operations before this change | Direction |
| --- | --- | --- |
| Google Calendar | Bounded read-only event window; no calendar writes | Remove active support, new OAuth requests, readiness probes and follow-up configuration. Preserve old records with a clear unavailable-source error. |
| Google Tasks | No built-in operations remain | Nothing to remove; use native plan steps. |
| Google Gmail | List/read selected messages; reviewed single-recipient plain-text send | Keep optional; not needed for internal tracking. |
| Google Drive | Search and read file metadata | Keep optional. |
| Google Docs | Read/create/update documents | Keep optional. |
| Google Sheets | Read sheets/ranges, create/update/batch writes | Keep optional. |
| Slack | Identity, search/history, reviewed post/update | Keep optional. |
| Atlassian | Jira search/read/create/comment; Confluence search/create | Keep optional. |
| Notion | Search, page/block reads, reviewed create/update | Keep optional. |
| HTTP JSON | Read an exact configured JSON resource | Keep optional. |
| HTTP webhook | Reviewed publication to an exact endpoint | Keep optional. |
| Remote MCP | Explicitly mapped search/read/action tools | Keep optional. |

Browser automation, local files, model providers and embeddings are separate
capabilities. “No connector dependency” does not mean hosted model inference is
offline. This change must not erase existing credentials or pretend to revoke
permissions already granted at Google. Previously granted access can be revoked
in the owner's Google account.

## Work sequence

1. Retire Calendar's reachable operations and setup while preserving actionable
   diagnostics for legacy schedules. Add regression coverage proving no request
   is sent for the removed operation.
2. Finish reviewed note updates: inspected identity/revision, preserved source
   references, repeat-acceptance recovery, and retrieval after restart.
3. Extend existing local plan records for useful scheduling/dependencies and
   expose scoped, bounded agenda inspection and reviewed conversation updates.
4. Exercise reminders and proactive checks over internal records, including
   due items, unchanged/quiet days, blocked and completed tasks, and user updates.
5. Run isolated live OpenAI lifecycle tasks with no selected connectors. Retain
   failures, actual applied state, model calls, tokens, and latency. Compare a
   matched baseline where meaningful; fixture passes are not a quality score.
6. Document remaining availability, remote-host and broader capability limits.
   Do not claim the six broad competitive gaps are solved by one local feature.

## Verification log

Implementation in progress. Record actual checks, commits and live results below
as they complete. No live-account test, external message, or deployment is part
of this phase.

### Calendar retirement

- Removed operation routing, OAuth scope requests, readiness probes and new
  follow-up selection. Legacy definitions fail before credentials or network access.
- Preserved optional Gmail checks, including a workflow using a user-maintained
  local schedule. No configured account or provider grant was deleted.
- Passed `go test ./internal/connector ./internal/followup ./internal/workapp`
  and the Google/Calendar/Followup/ConnectedGoal tests in `./cmd/gator`.
- Full `./cmd/gator` run reported a slide-deck fixture failure in
  `TestUserWorkV4CorpusThroughProductService` (49/50). Investigate separately;
  this is not a clean full-suite result.

### Reviewed updates to existing notes

- `knowledge_read` exposes the inspected revision. `suggest_change` can propose
  a replacement body for that exact note; acceptance preserves its ID, scope and
  prior references. Review output includes the current body and revision match.
- Stale, excluded and cross-goal proposals fail. Acceptance retries do not create
  another note or overwrite a later owner edit.
- A durable journal recovers interrupted body/metadata writes. A conflicting
  direct edit is preserved and reported instead of overwritten.
- Passed note/knowledge/suggestion tests across knowledge, suggestion, tools,
  workrun and CLI packages, including restart, interruption and acceptance retry.
  Live workflow verification follows with the local tracking work.

### Local plans and agenda

- Steps now support explicit start/end blocks and up to eight same-plan
  dependencies. Invalid intervals, missing dependencies, duplicate dependencies
  and cycles are rejected. Removing a referenced step requires unlinking it first.
- `/agenda` and `gator goal agenda [--days N] [--goal ID]` show unfinished work,
  overdue dates, blocked dependencies and overlapping blocks. Paused/completed
  goals and completed steps are omitted. This is derived from the plan store.
- Main chat can inspect an existing project by title with `local_plan`, then
  propose an exact revision-checked update without switching chats. A goal chat
  can only inspect/update its own plan. Ambiguous titles require disambiguation.
- Plan acceptance has a persisted receipt, so an interrupted acceptance cannot
  add the same step twice. A later owner edit still invalidates a stale proposal.
- Existing goal watches detect scheduled start/end boundaries once; unchanged
  checks remain quiet across restarts. Dates never imply completion. Watches
  still require an explicit reviewed agreement and an available supervisor.
- A block is a user-maintained plan, not an externally booked event. It does not
  silently create a reminder. One-off reminders and recurring jobs remain the
  existing notification mechanism. Repeating calendar-style blocks are not added.

Example: “In my workshop project, schedule the outline for 2 November from
10:00 to 11:00 Singapore time. It depends on budget approval. Keep it pending.”
Gator reads that project and proposes the change. After accepting it through
`/suggestions`, `/agenda` reflects the new block and unresolved dependency.

Direct terminal edits also support `gator goal plan ID add --title TEXT --start
RFC3339 --end RFC3339 --depends-on STEP_ID` and the corresponding `step` flags.
Use an explicit timezone offset. `--clear-schedule` and `--clear-dependencies`
remove those fields without deleting the task.

### Limits still open across the six broader gaps

1. **Ongoing coordination:** native planning is stronger, but applying plan
   changes still needs owner review. No claim of autonomous multi-week execution.
2. **Everyday capability:** local dates/progress/notes work without connectors.
   Recurring time blocks, richer calendar views and cross-project dependencies
   remain separate future work. Other connector families are optional, not removed.
3. **Proactive judgment:** quiet/overdue decisions can be tested, but a small
   synthetic sample cannot establish usefulness or low noise over real weeks.
4. **Availability:** closing the terminal can leave the supervisor working, but
   a sleeping/offline Mac does not become an always-on host. The companion still
   needs deployment, device and real-world recovery validation.
5. **Maintained knowledge:** existing notes can now be corrected in place.
   Resolving contradictions across separate notes/plans is not automatic, and
   shared editing by multiple people is not implemented.
6. **Speed/evidence:** title lookup avoids a list/read round trip. Live reports
   retain latency, calls and tokens. No claim of better reasoning than the same
   base model; persistence and checked actions are the product's added capability.

The repository still contains historical desktop-era and connector-heavy
research documents. This ledger and the connector inventory describe this phase;
old plans should not be read as statements of current support.

### Live OpenAI lifecycle (30 October)

Retained reports and reproduction instructions:
[`scripts/internal-agent-eval`](../scripts/internal-agent-eval/README.md).
All runs used `gpt-5.6`, synthetic profiles and zero selected connectors.

| Run | Result | Requests | Input/output tokens | Interpretation |
| --- | --- | ---: | --- | --- |
| 1 | Failed, budget exhausted | 39 | 291,037 / 10,305 | Exposed omitted `strict:false` in the Responses adapter; optional unrelated suggestion fields were being filled. Failure retained. |
| 2 | 13/13 checks passed | 27 | 158,081 / 2,357 | Real note acceptance, plan edits, fresh-chat recall and quiet/overdue decisions worked. ~56.5s total. Tool retries were not separately recorded in this version. |
| 3 | 14/14 checks passed | 22 | 125,464 / 1,962 | Added evidence-reference check, tool-error capture and direct title lookup. Zero recorded tool errors; ~51.8s total. |

Request/token totals include two text-only decision baselines. Both baselines
also made the correct quiet/overdue decisions; there is no measured reasoning
advantage here. The final Gator workflow used 20 calls, versus 2 calls for the
baseline's narrower decision task. Those workloads are not interchangeable.
A single rerun with fewer calls is not a statistically established latency gain.

The adapter now explicitly sends `strict:false` for function tools, preserving
host-validated optional fields. This matches the documented behavior that
[omitting strictness can normalize Responses schemas into strict mode](https://developers.openai.com/api/docs/guides/function-calling).
Computer tools remain separate. No credentials or real user state are retained
in the reports.

The broad-suite deck failure was traced to an old generated PPTX without a slide
layout. Regenerating that synthetic fixture with the current renderer preserved
its content and made the isolated edit scenario pass; no assertion was weakened.

Existing profiles load without migration. Newly written plan scheduling fields
and note receipts require this version of the reader; restoring an older binary
alone does not remove those fields from a profile. Use a matching profile backup
when rolling back data formats.

### Final local verification

- `go test ./internal/... ./cmd/...` passed after updating the remaining legacy
  access-probe tests. The regenerated deck scenario passes in the full corpus.
- `node ui/scripts/build-runtime.mjs` rebuilt the local runtime successfully.
  The rebuilt binary's `goal agenda --days 7` passed an isolated empty-profile
  smoke test.
- The terminal CI workflow now exercises native goal, note, suggestion and
  follow-up lifecycles on Linux, both macOS architectures and Windows. Current
  run status is recorded in GitHub Actions, not inferred from this Mac's tests.
