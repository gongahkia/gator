# T2: durable remote semantic intervention

T2 extends the T0 capability contracts and T1 verified-owner channel. It adds no
PTY input, terminal keys, external terminal access, provider routing, daemon,
execution ownership, scheduler, or execution-lifetime changes. Onibi is untouched.

## Supported operations

| Operation | Scope | Canonical execution path | Acknowledgement means |
| --- | --- | --- | --- |
| `cancel` | One exact Work owner | Existing Work context cancellation | Cancellation requested; termination has not been established |
| `steer` | One active specialist | `Operation.SteerTask` → supervisor steering | Accepted into the existing semantic steering path |
| `answer` | One specialist and current question token | `Operation.AnswerTask` → supervisor/native session control | Accepted into the existing semantic answer path |
| `decision` | One current command interaction and preview digest | Existing `respondBound` | The exact decision was delivered, not that the approved command completed |

Claude active-turn steering is unsupported, as in the existing native adapter.
Native questions use their existing question token/answer contracts, never PTY
keys. Native command journaling, checkout isolation, candidate validation,
independent review and evidence sealing remain authoritative and unchanged.

Remote approvals initially support bounded, complete command argv previews only
(32 arguments, 8 KiB serialized preview). The owner recomputes the preview digest
and checks its original live pending interaction before dispatch. Synthetic tool
permissions (`terminal_write`, detach, extension/MCP/LSP, web search/fetch), other
approval kinds, oversized previews, and consumed/changed decisions are refused.
In particular, a terminal input digest is not an adequate review of terminal
input. The new `executiontarget.Decision` operation is Work-only: none of the
existing operations represented exact approval decisions. No model tool exposes it.

## Architecture and routing

```text
authenticated phone + reviewed finite semantic action + stable ID
  -> companion strict shape validation / verified-owner authorization preflight
  -> existing bounded serial queue / fresh device validation / owner preflight
  -> fixed private runtime helper (JSON stdin, no input in process arguments)
  -> existing active lease + literal loopback + nonce verification
  -> exact owner / target / generation / scope authorization
  -> actionlog durable claim -> fresh authority and semantic-state checks
  -> existing semantic callback -> durable receipt or retained uncertainty
```

The browser supplies no actor, grant, endpoint, token, path, or expiry. The gateway
derives the actor from current companion authentication. The helper verifies the
full T1 target binding against the actual live control record and health nonce.
No Backend worker is created. A changed/disappeared owner rejects new actions.
Matching retained actions and receipt reads do not require a surviving owner and
never restart execution. Old owners without T2 handlers cannot perform controls.

## Grants and revocation

Inspect never grants control. Explicit host-local operation grants bind the
remote human actor, complete target/owner/generation/incarnation, operation,
specialist or interaction scope, and expiry (maximum 900 seconds). The owner
holds at most 64 control grants in memory; restart removes them. Models cannot
inherit a human grant. Scope is resolved at issuance: answer grants capture the
current native question token; decision grants capture the current preview digest.
Changing the question does not redirect an existing grant to its successor.

```sh
# Use the execution host's GATOR_STATE_DIR and matching built runtime.
# Separately grant Inspect using the T1 procedure when using the phone view.
node companion/control-access.mjs RUNTIME OWNER_TOKEN_FILE RUN_ID DEVICE_ID cancel '' 300
node companion/control-access.mjs RUNTIME OWNER_TOKEN_FILE RUN_ID DEVICE_ID steer subagent-001 300
node companion/control-access.mjs RUNTIME OWNER_TOKEN_FILE RUN_ID DEVICE_ID answer subagent-001 300
node companion/control-access.mjs RUNTIME OWNER_TOKEN_FILE RUN_ID DEVICE_ID decision 1 300
# Same operation/scope with 0 seconds revokes matching grants.
node companion/control-access.mjs RUNTIME OWNER_TOKEN_FILE RUN_ID DEVICE_ID cancel '' 0
```

`owner` may replace DEVICE_ID only for an explicitly authorized current owner-token
principal. Token files and their directories must be private. The host helper
uses the existing enrolled device registry; it creates no pairing/database.
The local administrative endpoint uses the existing private Work control token
and is never forwarded by companion HTTP.

Mutation grants reference the existing local companion token/device records.
The execution owner recomputes the same opaque enrollment-generation principal
from those records before dispatch, so device removal, owner rotation, changed
identity or unavailable authority storage fail closed even after gateway preflight.
The Go check is a projection of the existing registry identity, not a new bearer
credential or authentication service. Gateway authentication still verifies the
actual supplied bearer token before/after waits and before releasing responses.

Dispatch serialization and grant locking are separate: local revocation can take
effect while an action waits or durably admits. The final grant check and entry
into the semantic callback are serialized against local grant revocation. Current
device enrollment is read at that final boundary too. Revocation cannot undo a
semantic operation already dispatched. All original Work policies remain ceilings.

## Durable admission and receipts

`internal/actionlog/control.go` is an additive schema in the existing private
action store, not a connected-service proposal or new action database:

`gator/external-actions/RUN/semantic-TARGET/ACTION_ID.json`

The admission namespace is one exact run/target/action ID. Records contain the
actor, full target generation, operation, scope, canonical payload digest,
timestamps, phase, bounded application-generated note and T0 delivery state.
They contain no raw instruction, answer or approval preview. Existing canonical
specialist checkpoints/native storage may retain input as they do for local
controls; T2 adds no raw-input diagnostic log.

An exclusive per-target file lock protects claim/settlement. Initial uncertainty
is atomically written and file/directory-synced before dispatch permission is
returned. Concurrent matching duplicates get the existing receipt, never a second
dispatch. The same ID with changed canonical payload/actor/target/operation/scope
conflicts. Receipts are bounded to 128 per target; reaching the bound refuses new
admissions rather than deleting idempotency tombstones. Existing private history
backup/deletion/retention owns this subtree. Live profile guards prevent history
maintenance from removing deduplication records beneath an active owner.

After claim, owner authorization and canonical task/question/decision state are
checked again. A proven rejection before semantic dispatch settles as
`not_dispatched`. Successful callback acceptance settles as
`dispatch_acknowledged`. A callback error after dispatch may have started, an
interrupted process, or a failed receipt flush remains `outcome_unknown`.
Generic errors are never interpreted as proof of no effect. Acknowledgement
does not prove native-engine consumption, command success, recipient delivery,
artifact completion, or Work termination.

Receipt reads can additionally report `work_lease_stopped` after observing the
original Work execution lease released. This is separate from cancellation
acceptance and never upgrades unknown delivery to success. It is not proof that
every descendant OS process has exited.

Death after claim but before dispatch may leave no effect and an unknown receipt.
Death after dispatch before settlement may leave an effect and the same unknown
receipt. Neither can replay after restart. Native answer admission remains intact;
the outer journal supplies remote client identity across gateway retries without
replacing the native command journal. Only the original owner dispatches.

## Companion API and mobile behavior

- Existing T1 GET APIs remain; eligible snapshots add grant-filtered `controls`.
- `POST /v1/work/TARGET/actions`: finite, reviewed JSON action, at most 24 KiB.
- `GET /v1/work/TARGET/receipts/ID?run_id=RUN`: actor-bound receipt lookup, including
  after owner exit. This endpoint never dispatches and never needs the raw input.

The action must include its stable 32-hex ID, run, full binding, operation and
only the fields appropriate to that operation. Unknown fields and raw terminal
operations fail closed. Responses are projected explicitly; control credentials
and local grant authority paths never reach the browser. Offers are bounded to
8/24 KiB, omitting a review that cannot fit rather than truncating it.

The Active Work view shows only granted, currently eligible operations. It binds
forms, review and confirmation to the exact selection, and closes obsolete reviews
on target/grant changes. Instructions, questions, argv and progress render as text.
Every action requires a separate exact-payload review and explicit confirmation.
There is no offline mutation queue. Browser session storage retains only receipt
identity before POST; it does not retain the action payload or automatically retry.

Lost responses and unknown receipts show uncertainty, never success. Receipt
retrieval is separate from submission. Pending receipt selections remain reachable
after reload even when their owner disappears. Unknown receipts block further
actions for that selected target in this UI; they have no automatic replay or
administrative force-resolution path. Dismissing a settled known receipt requires
a fresh inspection before another action. Revoked access clears visible controls.

## Verification and limitations

Tests cover exact grant separation, scope/generation mismatch, device/owner-token
revocation, local revocation after claim, expiry, canonical questions/decisions,
concurrent duplicates, conflicting IDs, conservative late errors, receipt reads,
and process death before/after dispatch. Real cross-process fixtures exercise
cancellation via a Go UI backend, steering through the existing Work specialist,
and exact command approval through the original pending interaction. Crash-window
fixtures reproduce OS-process death, not a physical power-loss test. Browser
fixtures test selection/grant races, explicit review, uncertain receipts, reload
and receipt reconciliation without replay. They are not physical-phone acceptance
or live-provider Codex/Pi/Claude acceptance; native protocol regressions use the
repository's existing fixtures and control contracts.

Relevant commands:

```sh
go test ./internal/executiontarget
go test -race ./internal/executiontarget
go test ./internal/workrun ./internal/cliagent ./internal/worksession ./internal/actionlog
go test -race ./internal/workrun ./internal/actionlog -run 'TestRemoteSemantic|TestRemoteApproval|TestSemantic'
go test ./...
go vet ./...
npm test --prefix app-host
npm run check --prefix desktop
npm test --prefix desktop
node --test companion/auth.test.mjs companion/server.test.mjs companion/recovery.test.mjs companion/inspection.test.mjs companion/mutation.test.mjs
node --test companion/client.test.mjs companion/inspection-client.test.mjs companion/mutation-client.test.mjs
node ui/node_modules/typescript/bin/tsc --noEmit -p ui/tsconfig.json
```

Run Bun tests only with the existing required installation; T2 does not install it.
Go toolchain declarations/dependencies are unchanged. T3/T4 may address broader
approval reviews, explicit uncertainty resolution, richer native observation and
separately enrolled external adapters. Raw keys/input, external terminals, SSH,
daemon migration and autonomous model intervention are not enabled here.
