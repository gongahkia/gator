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
