# T7 — final Onibi migration acceptance

Acceptance date: 31 October 2026. This is verification and documentation of the
completed migration, not another implementation tranche. Gator remains a
terminal-agnostic, local-first personal AI agent. Remote supervision supports that
product; it does not turn Gator into a shell service or terminal-session manager.

| Acceptance category | Verdict | Evidence boundary |
| --- | --- | --- |
| Engineering integration | **PASS** | T4.5 regression/build evidence remains applicable; implementation unchanged; focused T7 checks below |
| Local operational acceptance | **PASS** | Disposable real Go owners, authenticated loopback companion, isolated native tmux and Chromium fixtures; initial deadline-test failure and successful reruns recorded below |
| Physical-phone acceptance | **NOT VERIFIED** | No operator-led iPhone procedure performed |
| Cross-platform acceptance | **PARTIAL** | macOS native fixtures and prior Linux/Windows cross-builds; native Linux/WSL and signed installations not verified |

These verdicts do not assert physical-device, live-provider specialist, unattended
host or signed-release acceptance. No further integration features are required
by this review. T5/T6 Ghostty work is permanently outside the roadmap.

## Repository and review boundary

Starting HEAD: `dd2eeffad39ee4965ded3880387f5a264ed94f48` (T4.5 consolidation).
Two existing uncommitted changes were present and preserved:

- `app-host/tests/execution-independence.test.mjs`: retain the actual HOME and
  isolate Git configuration in the disposable startup fixture.
- `docs/EXECUTION-TARGETS-T4.5.md`: the existing detailed verification record.

T7 changes only this document and an acceptance link in `companion/README.md`.
No implementation, dependency, toolchain declaration, user state or history is
changed; no commit is created. The sibling `../onibi` directory is absent. T7 did
not modify, delete, archive, move or recreate it.

The review covers the historical T0/T1/T2/T3/T3.1/T4 records, the final
[T4.5 scope and regression record](EXECUTION-TARGETS-T4.5.md), companion setup,
target policy, owner routing, external catalog/trust/grants, adapter dispatch,
actionlog admission and browser controls. The reserved `external_ghostty` T0
enum/tests remain for contract compatibility only. No Ghostty adapter, enrollment,
route, Automation/Accessibility code or executable integration exists. External
runtime projections accept `external_tmux` only.

## Final supported capability matrix

| Capability | Supported scope | Qualification |
| --- | --- | --- |
| Live Work inspection | Desktop, OpenTUI and companion Go execution owners | Exact live owner, lease, nonce and target generation; no replacement worker |
| Work progress/status | Bounded structured activity, specialists and pending interactions | Not fabricated stdout or a terminal transcript |
| Work cancellation | Separately granted semantic cancellation request | Acceptance does not mean immediate termination or all descendants exited |
| Specialist steering | Eligible running/queued specialist via existing semantic control | Engine capability applies; active Claude steering is unsupported |
| Specialist answers | Exact current native question identity and reviewed answer | Consumed/stale questions rejected; no PTY response simulation |
| Remote command approval | Exact current bounded, complete argv preview and decision | Existing digest/expiry/token protections; synthetic tool previews and unsupported approval kinds refused |
| Work receipts | Owner-authoritative durable semantic admission and lookup | Stable identity/digest, duplicate suppression, retained uncertainty |
| External tmux inspection | Explicitly enrolled exact existing pane and device Inspect grant | Separate host-local read review; bounded plain-text snapshots |
| External tmux input | Literal text, text plus Enter, finite keys, Ctrl-C | Separate local control review and operation-specific device grants |
| Terminal receipts | Existing actionlog, cross-process claims and target serialization | No automatic replay; partial text/Enter delivery remains uncertain |
| Companion conversations | Existing authenticated conversation workflows | Device access is still single-owner profile access, not multi-tenant account isolation |
| Ordinary local operation | Work, local notes, desktop/OpenTUI backend hosts | No tmux, Onibi, phone, external grants, Tailscale or external catalog required |
| Phone PTY attachment / arbitrary process supervision | Unsupported | T0 PTY contracts do not implement remote attachment |
| Ghostty, other muxes, generic SSH or model terminal tools | Unsupported / excluded | No adapter, feature or follow-on tranche authorized |

Finite terminal keys are Enter, Escape, Tab, Backspace, Up, Down, Left and Right.
Text is 1–2048 UTF-8 bytes without control/format characters, NUL, CR/LF or Unicode
line/paragraph separators. A text grant permits both text modes; a key grant can
send Enter. Both represent host-user execution authority.

## Accepted architecture and independence

`internal/executiontarget` supplies identity/capability/authorization contracts.
`internal/workrun` retains actual Work ownership and existing semantic controls.
Its private control records locate a live owner; authenticated loopback nonce and
lease verification establish that retained metadata still refers to that owner.
Inspection never launches a replacement worker or resumes history as an OS process.

External panes remain independently owned. `executiontarget/external` retains
enrollment, separate trust scopes and grants; `executiontarget/tmux` performs
bounded native identity checks, snapshots and finite input. The companion invokes
a short-lived Go helper rather than a Work worker or new daemon. External panes
have no invented Work run, specialist or approval identity.

`internal/companionaccess` reads the existing companion device/owner-token
authority. Existing `companion/server.mjs` handles authentication, Host/Origin
checks, bounded queues, fresh authorization and response projections. There is no
second auth service, Onibi runtime, pairing database, action database or required
supervisor. The optional companion listener itself is an existing Node process.

T4.5 already built copied Gator source with no sibling Onibi, verified dependency
closure, built desktop runtime/assets with vendored notices and cross-built the
full Go runtime. T7 repeats the actual ordinary-operation fixture with only Git
on child PATH, no optional binaries or phone authority, and no creation of the
external catalog/terminal admission store. It also builds real runtime fixtures
while `../onibi` is absent. Broad packaging builds are not repeated without a
source change.

Only adapted tmux controller/capture/text/input patterns and relevant tests were
retained from Onibi. Its daemon, web application, auth, configuration discovery,
SQLite store, lifecycle management, Ghostty and other backends were excluded.
`third_party/onibi/LICENSE` and `NOTICE.md`, source headers and packaged attribution
remain intact; Gator's project licensing is unchanged. Onibi can independently
become an archived reference repository without affecting Gator build or operation.
That repository-owner action was not performed here.

## Verified local workflows

All mutable fixtures use temporary profiles, local deterministic providers or
isolated test-created tmux sockets. No developer session, real model account,
production Work or user tmux configuration is used.

| Workflow | T7 evidence |
| --- | --- |
| Start Work locally and inspect through companion | `companion/inspection.test.mjs`: real Go owners for desktop, terminal and companion frontend transports |
| Verify owner without duplicate workers | Same test compares run/nonce-derived owner, spawn/model call counts, cross-conversation isolation and stopped-owner behavior |
| Authorized semantic intervention | `companion/mutation.test.mjs`: real terminal-host Work cancellation, Inspect-only denial, duplicate admission and observed lease stop |
| Steering / exact approval | `workrun/remote_control_test.go`: cross-process original-owner specialist/decision fixtures; native engines remain controlled fixtures |
| Question binding / stale decision rejection | Workrun tests exercise exact question identity, consumed/changed preview rejection and original semantic callbacks; not a live Codex/Pi/Claude device session |
| Device revocation | Auth, Work mutation, queued inspection/mutation and external tests reject revoked access and withhold pending private responses |
| Exact external pane enrollment and Inspect | `companion/external.test.mjs`: real private tmux server, enrolled pane, actual local grant helper and authenticated snapshot |
| Stale identity and trust withdrawal | Same fixture withdraws trust, renews explicit enrollment and replaces server; old generations never inherit access |
| Literal text / Enter / Ctrl-C | `companion/terminal.test.mjs`: real input recorder, active alternate pane and swapped pane positions; only enrolled pane receives sentinel |
| Keys and aliases | Opt-in Go native tests verify finite key encoding, alias shadowing via harmless isolated sentinels and withdrawn review |
| Receipts / deduplication / revocation | Terminal HTTP test uses six concurrent real helpers, GET reconciliation, conflicting payload refusal, raw-input exclusion and control withdrawal |
| Crash and uncertainty | Actionlog/Workrun/external tests inject crashes and late failures; retained claims never reacquire dispatch permission; these crash windows are deterministic fixtures |
| Browser selection / reconnect / recovery | Ten Chromium journeys cover A/B isolation, untrusted text, revoked content clearing, hidden-view polling, review binding and receipt reconciliation without replay |
| Normal operation without optional terminal facilities | Real ordinary Work, local notes and both desktop/OpenTUI Go-host startup with only Git on PATH |

The Go-host checks exercise the actual transport processes, not interactive
Electron/OpenTUI rendering. Chromium mobile viewports are not physical iPhones.

## T7 commands and results

Host: macOS 15.7.7, Darwin 24.6.0, amd64; tmux 3.8; Node v24.19.0;
installed Go go1.27.0, CGO enabled. The unchanged module declares Go 1.25.13.
Bun is not installed; no toolchain was installed or upgraded.

```sh
GATOR_TMUX_LIVE=1 node --test companion/auth.test.mjs companion/inspection.test.mjs companion/mutation.test.mjs companion/external.test.mjs companion/terminal.test.mjs companion/recovery.test.mjs companion/server.test.mjs companion/health.test.mjs
GATOR_TMUX_LIVE=1 go test -race -count=1 ./internal/executiontarget/... ./internal/actionlog ./internal/workrun
go test -race ./internal/executiontarget/tmux -run '^TestRunnerBoundTimeoutAndErrors$' -count=3 -v
GATOR_TMUX_LIVE=1 go test -race -count=1 ./internal/executiontarget/tmux
node --test companion/client.test.mjs companion/inspection-client.test.mjs companion/mutation-client.test.mjs companion/external-client.test.mjs companion/terminal-client.test.mjs
node --test app-host/tests/execution-independence.test.mjs
npm run check --prefix ui
git diff --check
```

| T7 check | Exact outcome |
| --- | --- |
| Companion auth / inspection / mutation / external / terminal / recovery / server / health, native opt-in enabled | 26 pass, zero failures/skips |
| Combined Go race invocation | Four packages pass; tmux fails `TestRunnerBoundTimeoutAndErrors` once at its three-second subprocess deadline |
| Isolated runner repeat | Three consecutive passes, about 0.43–0.49 seconds each |
| Full tmux race package rerun, including isolated native tests | PASS |
| Chromium journeys | 10 pass, zero failures/skips |
| Ordinary-operation independence fixture | One pass, zero failures/skips |
| OpenTUI TypeScript | PASS |
| Diff checks | PASS |

The initial Go command ran alongside several build/browser/native suites. At
`tmux/controller_test.go:148`, the unbounded-output fixture received zero bytes
and `ErrUnavailable` after 3.04 seconds instead of the expected bounded overflow.
The reproduction attempt was the same test in isolation, repeated three times,
then the full package; all passed without code/configuration changes. This is a
non-reproduced, deadline-sensitive fixture failure, consistent with load but not
proven to be caused by it. It is not omitted or represented as a clean first run.
The production timeout remained enforced and no unbounded output escaped. If it
recurs, retain the command/load details and investigate separately; T7 changes no
timeout or test implementation to obtain a pass.

Session-local raw test evidence is in `/tmp/gator-t7-{companion,go,runner-recheck,
tmux-recheck,browser,independence,ui}.log`; these temporary paths are not durable
release artifacts. This document records the outcomes independently of those files.

Unchanged-source T4.5 evidence is retained, not claimed as freshly rerun: full
`go test ./...` (72 passing packages, 11 without tests), `go vet ./...` (no
diagnostics), app-host (78 pass, one existing skip), desktop TypeScript (pass),
desktop tests (seven pass, one existing skip, plus nested app-host), isolated
desktop/Go builds and Linux/Windows full runtime cross-builds. No implementation
changes since those checks justify repeating the expensive broad build matrix.

## Platform coverage and unverified scenarios

| Platform/check | Coverage |
| --- | --- |
| macOS automated Go/Node/Chromium | Verified locally, with the deadline-test rerun caveat above |
| macOS native tmux | Verified on isolated tmux 3.8; no ordinary sessions used |
| Physical iPhone / Safari | NOT VERIFIED; checklist below only |
| Linux cross-build | T4.5 PASS, full Go runtime with CGO disabled; reused evidence |
| Windows cross-build | T4.5 PASS, full Go runtime with CGO disabled; native tmux unsupported |
| Native Linux runtime | NOT VERIFIED |
| Native WSL runtime | NOT VERIFIED |
| OpenTUI TypeScript | T7 PASS; actual Bun UI runtime not exercised |
| Bun tests / full OpenTUI artifact packaging | Not run: Bun unavailable |
| Signed installer / notarization / installed app permissions | NOT VERIFIED; building desktop assets/runtime is not installer acceptance |
| Locked-screen, sleep/wake, unattended host/network recovery | NOT VERIFIED on a physical deployment |
| Real native coding-engine specialist interactions on a phone | NOT VERIFIED; existing protocol/semantic fixtures are not live-provider acceptance |

## Operational prerequisites and security limitations

Use a configured Gator runtime/profile and Node 22.12+ for the optional companion.
Keep the matching host state/config roots across owner, companion and local grant
commands. Use the existing supported private encrypted connection described in
[companion setup](../companion/README.md); do not expose the loopback listener
directly to the internet or bypass Host/Origin validation. No additional remote
access service is introduced. Pairing remains host-local device-token enrollment
and private token transfer, not a new QR/interactive pairing implementation.

Work must have a surviving actual execution owner. Closing its owning interface
may terminate it; retained conversations do not reattach an OS process. Local
Inspect/control grants expire within 900 seconds. Work grants are held by the
current owner and must be reissued after owner replacement. Specialist/approval
capabilities additionally depend on the exact current interaction.

External tmux requires an installed supported backend (currently accepted
versions 3.4–3.8; native verification here is 3.8), an existing private server,
explicit exact-pane enrollment, read review and device Inspect grant. Mutations
add distinct local control review and operation-specific grants. Enrollment,
backend trust and device authority are independent. Phones/models cannot enroll,
review, choose an executable/socket or mint grants.

Wrong target/generation, missing/expired/revoked grants, changed native evidence,
owner rotation and withdrawn trust fail closed. Queue and execution boundaries
recheck authority. Revocation stops future access/undispatched actions, not effects
already dispatched or bytes a client has already copied. Tokens are bearer
credentials; protect their transfer and private registry/profile storage.

Durable claims precede mutation. Matching IDs return retained receipts; changed
payload/actor/target conflicts. Unknown delivery, including partial text/Enter,
never automatically replays. Receipt GET cannot dispatch. Unknown predecessors
block further terminal actions; there is no trustworthy universal "mark successful"
button. Acknowledgement is native dispatch evidence, not application success or
process termination. Missing admission history disables existing enrollment control
rather than silently recreating replay authority.

Snapshots remain bounded (native deadline three seconds, stdout 64 KiB, returned
text 16 KiB / at most 100 lines); finite HTTP bodies/responses and polling limits
remain enforced. Output is untrusted text, not approval evidence. The external
path has no automatic model/memory ingestion. Raw input/output is excluded from
ordinary diagnostics and terminal receipts; receipts retain identity/digest and
bounded fixed metadata. These checks do not prevent screenshots or a same-user
process reading host data.

The reviewed tmux model accepts user-managed command behavior. Aliases/hooks may
change canonical command effects; there is no proven alias-free CLI path or
automatic runtime-configuration attestation. Known changes require withdrawal
and renewed review. Malicious same-user processes, undetectable runtime changes,
the native preflight/dispatch race, local keyboards and other clients remain
limitations. Per-target serialization coordinates Gator helpers, not the whole
desktop or duplicate enrollments of the same physical pane. Raw terminal input
can execute with host-user privileges and is not a substitute for Gator Work
approvals, specialist protocols or artifact verification.

## Operator-led physical iPhone acceptance — not performed

Run only with explicit operator participation, a disposable workspace/Work task,
a dedicated tmux server/private socket and harmless output/input recorder. Use
existing scripts and the exact local commands documented in T1/T2 and the
companion README; do not use real developer panes. Record date, runtime commit,
iOS/Safari version, private connectivity, device ID (no tokens), each result and
receipt identities. Leave failures/unavailable specialist types unverified.

- [ ] Confirm existing private encrypted connection and the configured allowed
  Host/Origin; connect from the actual iPhone, not a desktop mobile viewport.
- [ ] Enroll a separate test phone token locally using `companion/devices.mjs`,
  transfer it privately and authenticate. Verify unrelated/no-grant targets absent.
- [ ] Start disposable Work locally. Record its actual host/run. Issue that
  device's short-lived Inspect grant using `companion/inspection-access.mjs`.
- [ ] Open Active Work; verify exact conversation/run, live activity and no second
  worker. Switch targets and ensure stale responses cannot replace the selection.
- [ ] Issue only the needed control grant using `companion/control-access.mjs`.
  Review and confirm a harmless steering instruction or cancel request. Separate
  dispatch acknowledgement from observed Work stop.
- [ ] With a supported disposable specialist, grant/answer the exact pending
  question. Reject a stale/consumed question; mark unsupported engines unverified.
- [ ] Review an actual supported bounded argv approval, grant its exact decision,
  confirm the preview on the phone and verify changed/expired previews are refused.
  Do not infer approval from terminal output or a pending-count badge.
- [ ] Create the private disposable tmux server through ordinary local test setup.
  Use `external-tmux candidates --reviewed-read-config`, then `enroll` with the
  exact native pane. No remote discovery/enrollment is permitted.
- [ ] Grant Inspect with `companion/external-access.mjs`. Open External terminals;
  verify label/opaque identity, bounded output, timestamp, and pane-switch isolation.
- [ ] Run local `external-tmux review-control TARGET_ID`; grant only required
  `text_input`, `key_input`, `interrupt` using `external-control-access.mjs`.
- [ ] Review exact harmless text and target; send without Enter, verify no newline.
  Separately review/send Enter and text-plus-Enter in the recorder fixture.
- [ ] Review/send a finite key and Ctrl-C to a recorder with harmless signal
  handling. Confirm the intended pane only; do not interpret Ctrl-C as guaranteed
  cancellation. Selecting another target must invalidate an open review.
- [ ] Disconnect around one explicitly reviewed action. Reconnect/reload and
  retrieve its receipt; verify no automatic POST/input replay. Preserve unknown
  delivery rather than presenting it as success or blindly resending.
- [ ] Background/foreground Safari and hide/show the terminal view. Verify
  appropriate polling pause, stale feedback and refresh without action replay.
- [ ] Disconnect/reconnect the existing private network; confirm sensitive stale
  content clears appropriately and reconnect does not create a mutation queue.
- [ ] Withdraw local control trust with `unreview-control`, confirm input denied
  while independently granted inspection remains. Revoke Inspect or enrollment
  and confirm further snapshots unavailable.
- [ ] Revoke the test device locally with `companion/devices.mjs ... revoke`;
  verify new and queued actions fail and displayed content clears on detection.
  Separately test owner-token rotation only in the disposable profile.
- [ ] Close only the test-owned Work/tmux resources and remove only test token
  exports/profile. Never kill the ordinary server or erase application history.

Locked-but-awake, sleep/wake and actual installed/signed runtime behavior require
additional explicit physical-host runs; they cannot be inferred from this list.

## Maintenance and final product readiness

Maintain upstream attribution in all shipped artifacts, backend compatibility
tests, private profile permissions and backup consistency of catalog plus receipt
history. Withdraw/renew trust before known tmux configuration/binary changes.
Manage device revocation, owner-token rotation and short grant renewals locally.
Do not prune uncertain receipts or restore old state as an idempotency workaround.
Current limits (32 retained enrollments, 128 terminal receipts per target and 4096
globally) fail closed and may require local operator attention.

Recheck deadline-test stability under controlled load if the recorded failure
recurs. Run full supported Bun packaging/tests, native Linux/WSL and signed
installer acceptance in their existing release environments when available; do
not convert their absence into a new terminal integration project.

Engineering migration is complete. Local automated/native fixture operation is
accepted under the documented reviewed-backend model. Physical-device and broader
release deployment acceptance remain open, explicitly bounded operational work.
Onibi is independently archivable; retain Gator's vendored license/provenance.
T7 does not archive it or authorize any further feature tranche.
