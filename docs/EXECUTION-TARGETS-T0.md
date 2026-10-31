# Execution target foundation (T0)

Final scope: see [T4.5 consolidation](EXECUTION-TARGETS-T4.5.md). This is a
historical tranche record. Current scope is owned Work supervision and optional
external tmux; Ghostty T5/T6 are permanently removed from the current roadmap.

Status: contracts only. No execution adapter, route, discovery, enrollment UI,
grant store, daemon, or lifecycle change is enabled by this tranche.

## Scope and authority

`internal/executiontarget` describes four backend kinds: Gator Work, Gator PTY,
external tmux, and external Ghostty. This is an additive host-owned contract,
not a replacement for Work, `terminal.Manager`, or native coding-agent protocols.
Nothing in the existing runtime imports it yet. The Electron and OpenTUI bridge
and the companion action allowlist deliberately remain unchanged.

Each target has an opaque random Gator ID, backend kind, actual execution owner
or backend-instance ID, random Gator generation, optional native incarnation,
display label, advertised operations, and availability with a required reason
when unavailable. Owner/native identities must be resolved by the trusted host;
display labels, titles, pane ordering, and PIDs alone cannot select authority.
Future display projections should redact owner/native details as appropriate.

The host preserves a binding for the same live target. Replacement, restart,
re-enrollment, or loss of confidence in native identity must rotate its generation
and invalidate old grants. A native incarnation, when available, adds identity
evidence; it does not replace the Gator generation. T0 has no registry or store
that would automatically maintain these bindings.

Inspect, semantic steer, semantic answer, interrupt, raw text input, key input,
and stop/cancel are distinct operations. Work may advertise semantic controls;
PTY/external backends may advertise input. No automatic conversion is allowed.
Interrupt and cancellation are separate: an adapter must advertise only what it
actually implements and explain its effect. T0 launches and controls nothing.

## Authorization contract

`Check` is a pure policy check supplied with current trusted state. It has no
authentication side effects, cached decisions, or persistent authority. Authentication
does not imply target access, even for inspection or owned tasks. Inspect grants
do not authorize input. Grants bind an exact actor (ID, human/model, local/remote),
complete target binding, one operation, and finite expiry. A human grant cannot
authorize a model; a model requires a separately issued grant. This does not
expose targets to model tools in T0.

External targets additionally require current enrollment evidence from an explicit
local human workflow. Enrollment alone grants no operation, and remote enrollment
or model enrollment is refused. The structs are trusted service contracts, not
client-submittable credentials. Callers must resolve the authenticated actor and
load grants/enrollment from host-controlled authority. A caller that lets a client
construct these objects has bypassed the contract.

For T1, the actual execution owner must serialize target operations, resolve fresh
target state, reload current grants/enrollment (including revocation), and invoke
`Check` immediately before dispatch. A review or queue wait invalidates any earlier
check. Hold the same serialization boundary through dispatch; the pure function
alone cannot prevent target replacement races. Native adapters must also revalidate
native identity immediately before their action. An unavailable/replaced target,
withdrawn capability, expired/revoked grant, or missing enrollment fails closed.

Existing Work modes, command/input approvals, sandbox/network policies, and
artifact verification remain additional ceilings. In particular, remote human
input must not directly inherit `terminal.WriteDeveloper`'s local-input bypass.
Do not use `action.Broker` for terminal mutations without an explicit capability
design: today it accepts only connected mutation and publish proposals.

## Delivery and persistence

The delivery vocabulary is `not_dispatched`, `dispatch_acknowledged`, and
`outcome_unknown`. Acknowledgement never proves a shell command's intended effect.
`DeliveryFromAction` projects existing `action.Status`: pending proposals and
denials are not dispatched, executed records acknowledge dispatch, and unknown,
failed, or unrecognized statuses are conservatively unknown. A failed operation
is not proof that no bytes were written. Only proven pre-dispatch rejection may
produce `not_dispatched`; an unsettled actionlog entry remains unknown.

No persistence or duplicate receipt schema is introduced. T1 must use Gator's
receipt conventions: exact request IDs, owner/run binding, payload digests, private
versioned records, durable intent before dispatch, and uncertain outcomes after
interruption. Reuse `internal/actionlog` where its capability/schema fits and the
native `cliagent` admission journal for native controls. Do not force terminal
input into a connected-action proposal or claim the existing actionlog directly
accepts arbitrary terminal records. Any necessary additive schema work belongs
in Gator's existing receipt system, with crash/deduplication tests. No Onibi SQLite
tables, auth, server, or daemon are imported. Input must not be automatically
replayed when acknowledgement is lost, including after host restart.

## Inspected foundations and Onibi lessons

Gator's `terminal.Attachment` is a restricted process-local PTY view; its registry
does not survive host exit. `codeexec` currently creates a run-local PTY manager
without registry/detach wiring. `cliagent` uses structured pipes and retained native
session data rather than PTY input. `workrun.Operation` and its verified loopback
control channel already provide semantic task controls. `workapp.BridgeState`,
`cmd/gator/ui_bridge.go`, and `app-host/contracts` are the future UI seams.
`worksession` admission generations/execution locks and `actionlog` write-before-
dispatch uncertainty must remain authoritative. Companion currently owns separate
Go workers; it does not attach to a desktop-owned worker.

The read-only `../onibi` inspection covered `internal/ghostty`, `internal/tmux`,
`internal/web/receipts.go`, and `LICENSE`. Ghostty identities include process start
and native surface identity; failures distinguish proven pre-dispatch rejection
from uncertain delivery. tmux separates literal text from key input and captures
pane output. Its selector strings are not Gator authorization. Onibi's web receipts
claim request IDs/hashes and preserve uncertain outcomes in SQLite. Those ideas
inform this contract; no Onibi source code was copied.

Onibi is Apache-2.0. Gator currently has no tracked root project license (the
tracked `ui/upstream/LICENSE` covers upstream material). Resolve attribution and
project licensing before later source copying; T0 does not change licensing.
Gator retains `go 1.25.13`; Onibi declares Go 1.26.10/toolchain 1.27.3. T0 uses
Gator-compatible standard library APIs and introduces no dependencies.

## Rollout and verification

T1 should adapt owned Work first: bounded inspection, native answers/steering,
and explicit cancellation routed to the existing owner. PTY input needs durable
mutation admission first. External tmux/Ghostty adapters, explicit enrollment,
model tools, phone routes, and a new long-lived execution owner are deferred.
Do not start T1 automatically.

Electron remains the reference during the OpenTUI parity migration; see
[migration plan](TERMINAL-MIGRATION-PLAN.md). New projections must eventually
cover both without widening companion's generic action allowlist. Recent code
completion checks bind reviews to candidate files: future input must not change
a candidate during trusted verification/review without invalidating that evidence.

Focused tests: `go test ./internal/executiontarget` (also with `-race`). Broader
checks: `go test ./...`, `go vet ./...`, `npm test --prefix app-host`,
`npm run check --prefix desktop`, `npm test --prefix desktop`,
`bun run --cwd ui check`, and the installed terminal test suite. These commands
are verification guidance, not a claim that all environments can run them.
