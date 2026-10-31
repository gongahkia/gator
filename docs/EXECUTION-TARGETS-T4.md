# T4: authorized external tmux intervention

T4 adds reviewed human input to explicitly enrolled tmux panes. It leaves Work
owners, native specialist controls, approvals/artifact verification, scheduling,
memory, providers and execution lifetimes unchanged. No model tool, SSH, new
server/daemon, database, dependency or toolchain is added. Onibi is read only.

## Architecture and authority

The existing companion authenticates the device, validates a finite action,
performs queued Go preflight, then rechecks after queue waits and invokes the
short-lived `companion-external-control` Go helper. The helper opens the existing
external catalog and actionlog, verifies device authority and exact target, claims
the action durably, then dispatches through the existing tmux adapter. No Work
worker/run or specialist identity is manufactured for an external pane.

Five independent decisions remain: backend read trust, backend control trust,
exact target enrollment, device Inspect grant, and device operation grant.
`Config.ControlReview` copies the verified backend's executable/socket/process
incarnation evidence but has a distinct `trusted_server_control_command_behavior`
scope, local human reviewer and UTC timestamp. Read review is insufficient.
Control grants use T0 TextInput, KeyInput or Interrupt, exact actor/binding and
expiry at most 900 seconds. Work control/coding permissions cannot substitute.
The human remote actor is derived from companion authentication, never request
fields. Go reloads the existing companion registry reference on authorization.

Inspection remains independently authorized; the mobile controls require an
Inspect-authorized recent snapshot as well as observed operation grants. A
mutation grant itself does not grant access to output. Backend control review
alone grants no device authority. Models have neither a tool nor actor permission
for these operations. Trusted local administration assumes the OS owner's action;
it is not a defense against arbitrary malicious code running as that same user.

## Actual host-local setup and withdrawal

Use the companion's same state profile and an existing tmux server/private socket.
Existing T3/T3.1 enrollment is unchanged:

```sh
GATOR_INTERNAL_COMMANDS=1 /path/to/gator-runtime external-tmux enroll --reviewed-read-config /absolute/path/to/tmux /private/path/socket '%3' 'Build monitor'
node companion/external-access.mjs /path/to/gator-runtime /private/path/access.token TARGET_ID DEVICE_ID 300
GATOR_INTERNAL_COMMANDS=1 /path/to/gator-runtime external-tmux review-control TARGET_ID
node companion/external-control-access.mjs /path/to/gator-runtime /private/path/access.token TARGET_ID DEVICE_ID text_input 300
node companion/external-control-access.mjs /path/to/gator-runtime /private/path/access.token TARGET_ID DEVICE_ID key_input 300
node companion/external-control-access.mjs /path/to/gator-runtime /private/path/access.token TARGET_ID DEVICE_ID interrupt 300
```

Grant only the operations intended. `text_input` permits both literal text and
text followed by Enter: either can exercise host-user authority. `key_input`
includes Enter, which may execute already typed text. Zero seconds revokes the
specified device/operation grant. Device revocation and owner-token rotation also
invalidate authority. The local script accepts the existing explicit `owner`
principal when owner-token access is intended.

```sh
node companion/external-control-access.mjs /path/to/gator-runtime /private/path/access.token TARGET_ID DEVICE_ID text_input 0
GATOR_INTERNAL_COMMANDS=1 /path/to/gator-runtime external-tmux unreview-control TARGET_ID
```

`unreview-control` clears control review/grants across all enrollments of that
server incarnation and preserves Inspect. Re-review clears selected target's
control grants; issue fresh grants deliberately. Existing `unreview` withdraws
read AND control trust and tombstones all that server's enrollments. Existing
single-target `revoke` also removes both kinds of authority. Known configuration
changes require withdrawal of relevant trust before the change and renewed review.
No remote route can issue review, enroll a pane or modify tmux configuration.

## Narrow native operations and targeting

`tmux/input.go` adapts the already selected Onibi attach.go literal SendText/SendKey
patterns. It uses only explicit argv `send-keys -t %PANE -l -- TEXT`, followed by a
separate `send-keys -t %PANE -- Enter` for `text_enter`. Key input maps only Enter,
Escape, Tab, Backspace, Up, Down, Left, Right to known native names (Backspace uses
BSpace). Interrupt is separately authorized and sends C-c. No repeat, prefix,
mouse, arbitrary key string, lifecycle command or OS shell command is exposed.

Text is valid UTF-8, 1–2048 bytes, without NUL, CR/LF, Unicode control/format
characters or line/paragraph separators. Tab is a separate key. HTTP rejects
malformed UTF-8 and unknown fields. tmux's argv parser treats a final semicolon
specially; the adapter escapes just that byte for tmux, preserving the exact
reviewed payload rather than dropping it. No shell interpolation is involved.
Sources: [send-keys literal handling](https://raw.githubusercontent.com/tmux/tmux/3.8/cmd-send-keys.c)
and [argv parser](https://raw.githubusercontent.com/tmux/tmux/3.8/cmd-parse.y).

Every send revalidates server/socket OS incarnation, exact native pane/session
membership and pane process-start evidence, control review and device grant.
The final invocation boundary rechecks authority after the offline version probe.
No active-pane fallback is used. Native IDs, executable/socket selection and
credentials stay on the host. Installed live verification used tmux 3.8; existing
supported-version/platform checks and capture limits remain in force.

Preflight and tmux mutation are NOT atomic. The local keyboard, other clients and
configuration can race with Gator. Gator serializes its own mutations to an exact
Gator target across helper processes; distinct enrollments of the same physical
pane are distinct targets and should not be used as concurrent control channels.
No exclusive ownership of the external program is claimed.

The reviewed-backend limitation remains: canonical aliases and hooks can translate
intended commands into other effects. Gator does not attest live configuration or
automatically detect arbitrary changes. It issues intended finite operations;
it does not prove arbitrary server behavior safe. Local trust acceptance is
required and cannot defend against malicious same-user configuration/processes.

## Durable admission, duplicate identity and uncertainty

`actionlog/terminal.go` adds terminal receipts to the existing private action
store. Global terminal action IDs live in `external-actions/.terminal/receipts`;
metadata includes exact target/generation/incarnation, actor, operation, action
kind, canonical payload SHA-256, timestamps, phase and delivery. It contains no raw
terminal text, stdout, credentials or native control configuration. Target-scoped
`.dispatch.lock` files coordinate helpers through existing cross-process filelock
semantics. A separate admission lock protects claims and settlement.

The canonical digest covers the typed action, authenticated actor and complete
binding. A claim is atomically written and fsynced as `claimed/outcome_unknown`
before mutation. Matching duplicates return that action's retained receipt;
changed payload/actor/target conflicts. Concurrent helpers cannot both win.
A crashed claimant is never given dispatch permission again. Receipt retrieval
cannot dispatch. The hidden `.terminal` namespace cannot collide with legacy run IDs and is explicitly excluded from connected-
service/Work receipt scanning; no connected-service approval classification is
borrowed. Limits are 128 receipts per target and 4096 globally; nothing is
silently pruned to admit more actions.

A per-enrollment admission identity marker is created only during initial local
control review. If normal history deletion removes receipts, old catalog records
cannot silently recreate admission authority. Review/control is refused for that
old enrollment: enroll again with a new opaque ID/generation and fresh grants.
Backups include receipts/identity metadata under existing lifecycle conventions.
Manual partial rollback/tampering by the OS owner is outside this guarantee.

Public delivery semantics:

- `not_dispatched`: validation/preflight rejection or an admitted action positively
  rejected before a mutation could start. Gateway rejection may have no receipt.
- `dispatch_acknowledged`: tmux returned acknowledgement for all native sends;
  the foreground program's receipt, interpretation and completion are unproven.
- `outcome_unknown`: durable unresolved claim or a failure after mutation may have
  started. Partial text/Enter is unknown, including text accepted but Enter
  refused, timed out or lost. Neither part is retried automatically.

A crash before claim cannot dispatch. A crash after claim may have no effect or
an unknown effect. Failed settlement retains the prior durable uncertain claim.
An unresolved predecessor blocks new action IDs on that target; there is no
"mark successful" button. Inspect and reconcile, and use deliberate local
re-enrollment/new authority only after the owner has reviewed external state.
Different action IDs are separate human intentions, not automatic retries.
Revocation prevents undispatched work, not already-dispatched effects.

## Companion APIs, contracts and UI

- `POST /v1/external/:target/actions`: `{id,target,generation,input}`; input kind
  `text`, `text_enter`, `key` or `interrupt`, with the corresponding finite payload.
- `GET /v1/external/:target/receipts/:id?generation=...`: metadata reconciliation,
  including after control withdrawal or native owner disappearance.

Both reuse existing authentication, Host/Origin validation, owner-token rotation,
device revocation, bounded serial queues and response limits. Requests are at most
8 KiB and Go helper replies at most 16 KiB. Actor, grants, review and backend paths
are never accepted from the browser. Gateway preflight is read-only; authorization
is checked after waiting, before helper execution, in Go, and immediately before
native invocation. A lost helper/connection is unconfirmed, never assumed safely
not dispatched. Existing Work/chat routes remain unchanged.

External snapshots advertise only currently authorized finite control offers.
The mobile view presents a literal composer, separate Enter/no-Enter choices,
finite keys and Ctrl-C, with an explicit review dialog. It shows the exact target,
backend, snapshot age, operation, payload, Enter behavior and host-user-authority
warning. Confirmation is bound to that target/generation and reviewed bytes;
selection changes invalidate it. Snapshots older than 60 seconds cannot authorize
UI submission. Snapshot text never authorizes anything itself.

Drafts are isolated by target/generation and held only in page memory. Session
storage contains only action ID and target/generation for receipt reconciliation,
never text or credentials. Reconnect/reload does not POST again. Unknown receipts
remain uncertain and block input, with GET reconciliation only. Acknowledged
receipts explicitly say program outcome is not established. Retained receipt
identities remain selectable if the live target disappears. Revoked/offline/hidden
views clear snapshots and disable controls. Progress, labels and review text use
textContent, never HTML. Existing Active Work/conversation views are preserved.

Terminal contents/input are not logged, added to memory or sent to models. Native
text argv can be observable to privileged/same-user OS process inspection; this
interface does not claim OS-level secrecy against the host owner.

## Manual verification and T5 boundary

Use only a disposable server/socket for acceptance. Enroll a pane running a
harmless input-recorder program, review control locally, grant Inspect and only
required mutation operations. Compare its snapshot, send a literal sentinel
without Enter, and confirm no line is submitted. Review a separate Enter, then
text-plus-Enter. Change the active pane and confirm the selected pane remains the
recipient. Exercise a harmless Ctrl-C handler; acknowledgement must not be called
process cancellation. Withdraw control and confirm input fails while Inspect
continues. Retrieve the old receipt; it must not redispatch. Revoke the device and
confirm both live responses and undispatched actions are refused.

Native tests create private isolated servers and clean up only their own sockets.
They use fixture lifecycle commands; production exposes none. No physical-phone
acceptance is claimed from browser/native fixture results. Windows preview remains
unsupported for native tmux control; Linux/WSL runtime acceptance is separate.

T5 is deferred. It must preserve separate trust/grants, exact identity, bounded
capture, durable receipts, unknown-outcome blocking and the trusted-server limits.
No Ghostty/Zellij/Screen, new terminal protocol, supervisor, generic shell,
autonomous model control, push notifications or prompt automation is included.

## Implementation manifest

Added:

- `internal/actionlog/terminal.go`, `terminal_test.go`: existing-store terminal
  receipts, global action ID binding, fsynced namespace/identity, process locks.
- `internal/executiontarget/tmux/input.go`, `input_test.go`: finite native sends,
  exact payload validation, control scope and conservative partial delivery.
- `internal/executiontarget/external/control.go`, `control_test.go`: local control
  trust, operation grants, fresh authorization, durable owner-side admission.
- `companion/external-control.mjs`, `external-control-access.mjs`: typed gateway
  projection/helper and trusted local grant command.
- `companion/public/external-actions.mjs`: exact review and receipt-only recovery.
- `companion/terminal.test.mjs`, `terminal-client.test.mjs`: adversarial gateway,
  real isolated native/helper/device flow and Chromium intervention journey.
- This document.

Adapted:

- `internal/actionlog/store.go`: ignore the new hidden terminal namespace when
  enumerating connected-service/Work effects; retain existing run semantics.
- tmux `controller.go`, `review.go`, `live_test.go`: distinct control review,
  final invocation authorization and native finite-key/alias fixtures.
- external `catalog.go`: independent control grants/review, admission initialization
  state and redacted authorized operation offers on snapshots.
- `cmd/gator/{companion_external.go,desktop_runtime.go}`: fixed local administrative
  commands and bounded, maintenance-free Go mutation helper.
- `companion/{server.mjs,external.mjs,README.md}` and
  `companion/public/{index.html,external-view.mjs}`: existing transport and view.
- `app-host/contracts/{external.ts,protocol.ts}`: typed finite actions/receipts.
- `third_party/onibi/NOTICE.md`, `desktop/licenses/DEPENDENCIES.txt`: attribution
  for the adapted input methods; Apache text/project licensing remain unchanged.

## Verification commands

```sh
go test ./internal/executiontarget/...
go test -race ./internal/executiontarget/... ./internal/actionlog
go test ./internal/actionlog ./internal/workrun
GOMAXPROCS=4 go test ./...
go vet ./...
npm test --prefix app-host
npm run check --prefix desktop
npm test --prefix desktop
npm run check --prefix ui
node --test companion/auth.test.mjs companion/recovery.test.mjs companion/inspection.test.mjs companion/mutation.test.mjs companion/external.test.mjs companion/server.test.mjs companion/health.test.mjs
node --test companion/client.test.mjs companion/inspection-client.test.mjs companion/mutation-client.test.mjs companion/external-client.test.mjs
node --test companion/terminal-client.test.mjs companion/terminal.test.mjs
GATOR_TMUX_LIVE=1 node --test companion/terminal.test.mjs companion/external.test.mjs
GATOR_TMUX_LIVE=1 go test ./internal/executiontarget/... -count=1
```

The concurrent duplicate fixture permits observing an in-flight claimed/unknown
receipt, then reconciles final acknowledgement and verifies exactly one copy of
the sentinel. It does not assume that an in-flight claim is already acknowledged.
Go subprocess fixtures reproduce both surviving uncertain claims and a process
exit after a possible-dispatch sentinel; they do not claim physical-program
acceptance. Partial text/Enter is reproduced with a fake runner.

## Acceptance evidence — 31 October 2026

**PASS under the reviewed, user-trusted backend model.** Physical-phone acceptance
is separate and was not performed.

| Check | Result |
| --- | --- |
| `go test ./...` (final default invocation) | PASS, 72 packages; 11 without tests |
| Bounded-concurrency full Go runs (`GOMAXPROCS=4`) | PASS, same package set |
| `go test -race ./internal/executiontarget/... ./internal/actionlog` | PASS, four packages |
| `go test ./internal/actionlog ./internal/workrun` | PASS |
| `go vet ./...` | PASS, no diagnostics |
| app-host tests | 77 pass, one existing skip |
| Desktop TypeScript | PASS |
| Desktop tests | Seven pass, one existing skip; nested app-host 77 pass, one skip |
| OpenTUI TypeScript | PASS |
| Existing companion auth/recovery/inspection/mutation/external/server/health | 22 pass, one opt-in native skip |
| Existing Chromium journeys | Nine pass |
| New terminal Chromium journey | One pass |
| New terminal gateway/lost-reply tests | Two pass |
| Combined opt-in terminal/external HTTP suites | Six pass, no skips |
| Opt-in Go target suites, including native finite keys/alias fixtures | Three packages pass |
| Linux and Windows external package cross-compilation | PASS; not native runtime acceptance |
| Formatting and diff checks | PASS |
| Bun | Unavailable; not installed/run |
| Physical phone / native Linux / WSL | Not performed |

An initial unbounded-parallel full run hit the existing three-second runner
fixture deadline with no captured bytes. Focused reruns, bounded-concurrency full
runs and the final default full run passed without relaxing production limits.
The new native key fixture was corrected to use an explicitly private short
socket directory. Concurrent duplicate assertions were corrected to reconcile
an in-flight claimed receipt rather than mislabel it as final acknowledgement.

The final native fixture uses its own private socket and two disposable panes,
changes active pane and swaps pane positions, sends literal UTF-8 text ending in
semicolon, proves no implicit Enter, sends reviewed Enter and text-plus-Enter,
observes exactly one sentinel from six helper invocations sharing an action ID,
exercises a harmless Ctrl-C handler, then withdraws control and verifies inspection
and historical receipt lookup continue. Existing native inspection fixtures still
verify replacement/stale-server rejection. No ordinary developer server is used.

Operational limits remain deliberate: no silent pruning, no successful-outcome
recovery button, no exclusive control against another keyboard/client, and no
claim of automatic alias/configuration attestation. Unknown outcomes require
inspection and local intervention. T5 was not started.
