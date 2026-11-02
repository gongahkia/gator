# Reviewed task-plan gate

Status: backend preparation, durable review/admission, scope enforcement, typed owner actions and the Electron composer/review cards are implemented. Noninteractive checks are recorded below. Final branch/plan Electron acceptance remains pending: app-opening tests are suspended at the operator's request. This is not a claim of completed UI acceptance.

## Existing stores and owners

A task plan is optional metadata on a request in the existing `worksession` admission journal. It is separate from ongoing-goal reference plans, the scheduler, and operation-specific approvals. There is no second database, workflow engine, daemon or execution owner.

The private record binds the conversation, task ID, original prompt digest, frozen options digest, source-path digest, selected source IDs, source snapshot and parent revision/update epoch. A retained head moved away and back cannot restore an old review. The initial empty chat may update its title without changing its empty context. Review content has a monotonically increasing revision and a canonical digest. The recorded approval binds that exact revision/digest and timestamp. A budget projection reports manager/coding step bounds where supplied; it is not a dollar estimate or a total bound on all specialists' steps.

Preparing a plan uses one completion invocation on the selected backend, without tools or a Work runner. It does not capture the workspace, fetch a connector, start a coding worker or create a Work execution. The model receives the task, selected source IDs, configured step metadata and bounded complete public conversation text. It does not receive native tool arguments/results or provider replay state through this preparation path. Files and attachments are not inspected merely to propose a plan. Returned content is bounded, strictly decoded, untrusted data; tool calls, invented source IDs, additional fields and completion claims in structured status fields are rejected. No automatic repair completion is requested.

## Desktop workflow

Select **Plan first** beside the composer, then send a message or queue a follow-up. This is a per-request choice and resets after successful admission; it is not a permanent execution-mode preference. The original task, attachments, selected connections and execution settings are frozen by the owner. For a queued task, its conversation boundary is captured when proposal preparation actually begins, after preceding work. A proposal pauses subsequent automatic queue execution for review.

The task card shows the proposed outcome and numbered intentions. **Review plan** fetches the full, exact retained proposal from the current owner; abbreviated card text is never the approval payload. The review contains the original request, attachment names, proposed steps, selected source IDs, foreseeable effects and frozen manager/per-coding-assignment step bounds. Files have not been read just to prepare the proposal. Total cost is unknown; this screen does not invent a price estimate.

**Edit plan** changes outcome, finite steps, source subset and effects. Saving creates a new unapproved revision and closes the old review. Inspect the new revision and explicitly choose **Approve plan & start**. Exact revisions/digests are required at the typed application boundary and native admission. Source selections and execution limits cannot be widened through plan editing. Copying source IDs into text does not authorize a connection.

Work state and observed activity remain separate from the proposed list. A completed or failed run links to its actual retained activity/results where available. Proposed steps never receive checkmarks inferred from narration. A running implementation uses the ordinary Stop control; cancelling or dismissing a retained request does not undo dispatched effects.

Review responses are bound to the current chat/task/revision. Busy, changed, disconnected and stale reviews cannot submit a new decision. A lost acknowledgement times out to an explicitly unconfirmed state, with no automatic mutation retry. Reopening an undispatched proposal never starts it. An approved but undispatched proposal needs an explicit **Start reviewed work** after restart. Interrupted requests are excluded from automatic queue replay: inspect their receipts/partial output, optionally stage their original text as a new composer draft, and explicitly prepare new intent. Attachments must be selected again when staging an interrupted request.

The owner projects bounded summaries through existing state frames and returns full content only for a selected exact review. There is no plan polling service, new database or native worker used just to display the card. The existing 17-pending/128-retained request limits remain authoritative; only terminal journal records may be pruned for new admission.

## Review and execution

Preparing and reviewing never dispatch implementation. Editing content increments its revision and removes approval. Approval is idempotent for an exact review, but does not claim that Work ran. The owner must separately claim the request under the existing execution lease and journal CAS. Both the owner admission path and the native Work boundary recheck the task, options, scope and approved digest. Changing the task, effective source selection, parent boundary or limits fails closed.

The native boundary narrows connector adapters to the connected sources actually named in the reviewed content. Normal connector authentication, account pins, permissions and exact mutation reviews remain authoritative. Plan metadata does not establish a new trusted account, endpoint or operation permission.

The existing runner `BeforeTool` hook revalidates the record and effect scope before dispatch. Supported native effects are read/research, staged deliverables, isolated coding with checks, connected-service writes, Gator settings/saved-memory changes, and separately approved browser interaction. Unsupported or undeclared tools are hidden from the advertised surface. A fabricated call to a hidden registered mutation stops the whole run before this or later calls dispatch. Code specialists require the same declared read/code/check categories; their existing sandbox, write scopes, command approvals and evidence verification remain intact.

Dynamic `code_use_integration` and desktop-control operations are not supported in plan-first scope classification. Ordinary Work retains its existing tools. These limitations must be surfaced rather than treating an unknown operation as a safe read. Native coding controls and run-owned coding PTYs remain distinct from external tmux authority; no external-terminal grant or model tool is added.

Plan-bound runs reject new steering through local operation methods and the authenticated control channel. Stop remains available; pending exact specialist answers and command decisions keep their established identity/preview checks. A changed task requires stopping and reviewing new intent. An approval cannot bypass later exact command, connector, browser, settings or artifact-application reviews.

## Recovery and uncertainty

Matching admission retries retain the same task; conflicting payloads fail. Concurrent approval admits once. A duplicate approval after completion returns the recorded state without admitting another run. Request generation CAS and the execution lease prevent concurrent claims.

A preparing or executing plan whose owner disappears becomes interrupted. It never enters the generic replay queue. Its original approval is retained as evidence when dispatch may have begun; it does not authorize retry. Dismissing an interrupted request frees the pending-request slot without asserting success or undoing effects. Running work must use the normal Stop path.

The existing action journals and receipts remain responsible for individual external-effect uncertainty. A task being admitted or an input being acknowledged is not proof that a program or the task succeeded. Work may leave reviewed partial output. No per-step checkmarks may be inferred from a proposed list, narration or an exit code alone.

## Limits of the guarantee

Effect categories constrain the native operations available to a plan; they are not a formal proof that an arbitrary program matches a natural-language outcome. Coding write scopes, sandbox policy, exact-action decisions and final diff/artifact review are still necessary. Trusted host configuration is authoritative and is not made immutable by a task-plan digest.

Plan records use existing private journal storage and retention. Request/plan text must not be added to ordinary diagnostics. Public activity continues to exclude raw tool arguments, terminal output and private reasoning.

## Verification to date

All tests below use disposable state and scripted models or private local HTTP fixtures. None uses paid API evaluations or opens Electron.

- `go test -race ./internal/worksession ./internal/workapp`: PASS (57.945 s / 28.588 s), including existing admission, recovery and branch tests. Additional focused plan runs passed after the subsequent scope refinements.
- `go test ./internal/workrun ./internal/tools`: PASS (78.738 s / 25.666 s), followed by focused tests for new control restrictions and scope filtering.
- `go test -race ./internal/workrun ./internal/worksession ./internal/workapp -run 'TestTaskPlan|TestQueueChanges|TestManaged'`: PASS (3.409 s / 3.961 s / 4.189 s).
- `go test -race ./cmd/gator -run '^TestDesktopTaskPlan'`: PASS (2.303 s). Exercises a real configured model adapter against a private HTTP fixture, verifies no Work infrastructure is started for preparation, and checks native narrowing to the reviewed connector source.
- `go test ./cmd/gator -run 'Test.*Configuration|Test.*Policy|Test.*Defaults'`: PASS (5.352 s).
- `go vet ./internal/workrun ./internal/worksession ./internal/workapp ./internal/tools`: PASS.
- `npm test --prefix desktop`: desktop 31 PASS / 1 SKIP; application host 76 PASS / 1 SKIP. These are Node checks, not Electron journeys.
- Desktop TypeScript: PASS. New plan-first Electron journeys and final regression verification remain required after the owner/card integration is complete and interactive testing resumes.

No physical microphone/phone, signed installer, or native Linux/WSL acceptance is claimed.

## Added owner and UI verification

- `go test -race ./internal/workapp ./internal/worksession ./cmd/gator -run 'TestTaskPlan|TestDesktopTaskPlan'`: PASS (6.018 s / 7.496 s / 2.882 s). Includes queued frozen settings, cancellation, restart without replay, explicit retained start, wrong owner, bounded pending/terminal retention, and the ordinary profile data-export guard during model preparation.
- `desktop/tests/e2e/task-plans.spec.ts`: four real-Go/private-model journeys added for edit/review/exact approval/idempotency, retained review across restart/cancellation, out-of-scope mutation rejection, and a separately pending exact coding-command review after plan approval. **NOT RUN** while app automation is suspended. TypeScript checking these tests is not acceptance of their UI behavior.
- Branch journeys also still require their corrected final rerun; see [Chat branches](CHAT-BRANCHES.md).

## Interface review boundary

Full source review using the existing React, CSS/Tailwind tokens and Lucide icons. No second styling or animation library was introduced. Interactive state, keyboard and screenshot acceptance awaits permission to resume app tests.

| Category | Evidence inspected | Result |
| --- | --- | --- |
| Typography | `TaskPlans.tsx` original request, proposed list, limits and status; theme font tokens | Plain untrusted text, wrapping and tabular revision numbers retained; actual rendering not verified |
| Surfaces | Existing modal focus hook, bounded scrollable review, 40 px controls, narrow composer | Source checks complete; narrow/light/dark runtime acceptance pending |
| Animations | Plan card/review implementation | No custom motion added; reduced-motion journey added but not run |
| Icons | Existing Lucide clipboard/list/close/add and currentColor tokens | Source checks complete; visual acceptance pending |
| Performance | Derived bounded journal summaries; selected full-review request; async owner result matching | No new background polling or worker; runtime responsiveness not verified |

| Severity | Location | Before | After | Why |
| --- | --- | --- | --- | --- |
| HIGH | `desktop/src/TaskPlans.tsx` | No exact task-plan review surface | Owner-bound full review with explicit approve/start, changed-state rejection and uncertain-result guidance | Prevent approving abbreviated or stale intent |
| MEDIUM | `desktop/src/App.tsx`, `desktop/src/styles.css` | Adding a labeled plan button could crowd the narrow toolbar | Keep its accessible name, collapse its visible label and permit toolbar wrapping below 1000 px | Preserve control reachability; visual confirmation remains pending |
| MEDIUM | `desktop/src/TaskPlans.tsx` | Plan-only status would lack an observed result destination | Link retained Work identity to actual activity/results without inventing per-step completion | Distinguish intention, execution and evidence |

Considered but rejected: a workflow canvas (adds another product surface); animated checkmarks (would imply unsupported per-step evidence); dollar estimates (no reliable complete cost projection). Interface verdict: **acceptance pending**, including actual focus return, stale selection responses, accessibility, narrow layout, themes and reduced motion. TypeScript/build success alone cannot resolve those checks.

## Consolidated noninteractive regression run

The operator asked to stop tests that open/click through the actual Mac app while taking a call. No further Electron window journeys were launched after that instruction. Existing companion browser journeys below use explicit `headless: true` and private local fixtures; they do not open the desktop app or establish physical-device acceptance.

| Check | Observed result |
| --- | --- |
| `go test ./...` | PASS: 72 packages passed, 11 packages have no tests; `cmd/gator` 294.969 s under concurrent verification load |
| `go vet ./...` | PASS, exit 0 |
| `go test -race ./internal/workapp ./internal/worksession ./internal/workrun ./internal/workactivity ./internal/executiontarget/...` | Final confirmation PASS: 7 packages; workapp 23.153 s, remaining packages cached after successful runs |
| `npm test --prefix desktop` | PASS: desktop 31 passed / 1 skipped; application host 78 passed / 1 skipped |
| `npm run check --prefix desktop` | PASS, including authored but unexecuted branch/plan journey types |
| `npm run build --prefix desktop` | PASS: private Go runtime and frontend built; no app launch |
| `GATOR_TEST_BINARY="$PWD/desktop/runtime/gator-runtime" node --test companion/*.test.mjs` | Final confirmation PASS: 37 passed / 5 skipped / 0 cancelled, 45.650 s; includes headless Chromium authentication, inspection, semantic mutations, external targets and recovery |
| Branch/speech Electron journeys | Pending corrected final run; still suspended |
| New plan Electron journeys | Added, not run; still suspended |
| Physical microphone/phone, signed installer, native Linux/WSL | Not verified by these checks |

Two first attempts under parallel load did not pass: the companion health fixture exceeded its 120 s timeout, and `TestExternalNativeSessionContinuesAcrossWorkConversationTurns` hit an existing Git-command context deadline in the Work race suite. No production timeout was widened. The health test passed separately with the already-built runtime (32.564 s), and the full Work race package passed separately (78.269 s). The complete companion and focused race suites then passed their final confirmation runs. This records the timing failures rather than treating the first attempts as green.

Skipped Node checks are the opt-in Electron panel-secret journey and Windows-native argv handoff. Skipped companion checks are the two opt-in real-tmux fixtures, native Linux supervisor/user-manager fixtures, and native macOS launchd fixture. No ordinary user terminal session was inspected or controlled. No dependency/toolchain installation or additional paid model evaluation was performed.

## Exact-action boundary audit

`internal/workrun/task_plan_approval_test.go` additionally exercises an already durably approved/claimed plan through the real Work service and its retained interaction decisions. A private HTTP webhook verifies zero dispatch before the separate exact action review, rejects a mismatched preview digest, and covers denial, one approved dispatch, an HTTP-error unknown outcome, consumed-review rejection and repeated receipt reads without replay. A controlled coding adapter verifies a separate pending exact-argv command review, wrong-preview rejection, allow/deny behavior and a consumed decision. The model and coding adapter are simulations; this does not claim native coding-process, Electron or physical-device acceptance.

The initial new assertion incorrectly expected a mutation receipt for a service-level denial. Existing behavior records a refusal in the resolved interaction before mutation admission; there is no dispatched action to receipt. The test now checks that established behavior explicitly. No production approval or receipt behavior was changed for the assertion.

`go test -race ./internal/workrun -run '^TestTaskPlan'`: PASS (6.445 s), covering the existing plan preparation/scope tests and both new approval tests (five allow/deny/unknown subcases). Final app review journeys remain required while app automation is paused.

The broader focused regression `go test -race ./internal/workrun -run 'TestTaskPlan|TestServiceApproval|TestExecutorConnectedRequestNeedsNoInventedOutputFile|TestExecutorRetainsUnknownExternalOutcomeWithoutDerivingRetryLearning'` also passed (8.134 s); `go vet ./internal/workrun` passed. A fourth real-Go/scripted-model Electron plan journey was added for the later pending exact command review and cancellation before dispatch, but remains unexecuted under the operator's app-test restriction.
