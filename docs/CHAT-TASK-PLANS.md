# Reviewed task-plan gate

Status: backend preparation, durable review/admission and Work scope enforcement are implemented and tested. Composer/card wiring, full user journeys and final acceptance are still pending. This is not a claim that the complete Plan first feature has shipped. Interactive Electron tests are currently suspended at the operator's request.

## Existing stores and owners

A task plan is optional metadata on a request in the existing `worksession` admission journal. It is separate from ongoing-goal reference plans, the scheduler, and operation-specific approvals. There is no second database, workflow engine, daemon or execution owner.

The private record binds the conversation, task ID, original prompt digest, frozen options digest, source-path digest, selected source IDs, source snapshot and parent revision. Review content has a monotonically increasing revision and a canonical digest. The recorded approval binds that exact revision/digest and timestamp. A budget projection reports manager/coding step bounds where supplied; it is not a dollar estimate or a total bound on all specialists' steps.

Preparing a plan uses one completion invocation on the selected backend, without tools or a Work runner. It does not capture the workspace, fetch a connector, start a coding worker or create a Work execution. The model receives the task, selected source IDs, configured step metadata and bounded complete public conversation text. It does not receive native tool arguments/results or provider replay state through this preparation path. Files and attachments are not inspected merely to propose a plan. Returned content is bounded, strictly decoded, untrusted data; tool calls, invented source IDs, additional fields and completion claims in structured status fields are rejected. No automatic repair completion is requested.

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
