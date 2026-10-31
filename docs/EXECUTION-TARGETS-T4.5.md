# Final execution integration scope — T4.5

Gator remains a terminal-agnostic, local-first personal AI agent for ongoing work:
conversations, memory, files/research, goals, coding specialists, approvals,
durable recovery and local interfaces. Remote execution supervision supports that
agent. It is not the product's primary identity.

The completed integration supports **Gator-owned Work supervision** and
**optional external tmux access**. Ghostty T5/T6 are permanently removed from the
current roadmap. The interrupted investigation produced no implementation.
Gator is not becoming an SSH client, terminal emulator, universal session manager,
Ghostty remote-control tool or multiplexer replacement.

## Final architecture and supported behavior

| Responsibility | Implementation | Boundary |
| --- | --- | --- |
| Capability contracts | `internal/executiontarget` | Opaque identity, owner/generation/incarnation, capabilities, actors, grants and delivery classifications |
| Owned Work inspection | `internal/workrun/inspection.go`, existing control channel, `cmd/gator/companion_inspection.go` | Verify actual live owner using lease/nonce; never create a replacement worker for inspection |
| Semantic Work intervention | `workrun/remote_control.go`, `control_authority.go`, existing native cliagent controls | Cancellation request, supported steering, exact pending question answer, bounded exact command approval |
| Shared device proof | `internal/companionaccess` | Existing private companion registry and owner-token generation; no second authentication system |
| External tmux | `executiontarget/tmux` and `executiontarget/external` | Explicit local pane enrollment, reviewed backend trust, separate device Inspect/control grants |
| Durable actions | Existing `internal/actionlog` store | Work semantic receipts and distinct terminal receipts; cross-process claims/locks, no raw sensitive input |
| Remote transport | Existing `companion/server.mjs` | Auth, revocation, Host/Origin, bounded queues/responses and finite endpoints |
| Presentation | Electron/OpenTUI via app-host; optional companion Active Work/External terminals views | Native Work state and terminal snapshots remain separate |

Work owns its execution context, specialist identities and semantic state. The
native Codex/Pi/Claude paths retain their own supported controls; Gator does not
replace those protocols with terminal keys. Steering support depends on engine
capability (active Claude steering remains unsupported). Remote approvals cover
bounded, complete command argv previews with existing exact decision/expiry
checks; synthetic terminal-write and other unsupported approvals remain refused.
Work cancellation acknowledgement is not proof of termination. Structured
progress is not fabricated stdout or a terminal transcript.

An externally started tmux pane is not a Work execution. Its independently owned
server/processes are verified through explicit native pane/session membership,
socket and OS process incarnation evidence. Its lifetime is not adopted by Gator.
The short-lived Go helper reads bounded snapshots or dispatches explicitly
reviewed literal text, text-plus-Enter, finite keys or Ctrl-C, subject to separate
control trust and exact device grants. Ctrl-C means a terminal key request, not
semantic Work cancellation or guaranteed process cleanup.

Local sandboxed PTY tasks and their process-local registry remain unchanged.
The T0 PTY kind is not a claim that phone PTY attachment/output is implemented.
The T0 Ghostty wire value remains reserved to avoid unnecessary contract changes;
there is no Ghostty adapter, catalog acceptance, API, executable integration,
Automation/Accessibility permission flow or current implementation plan.

## Optional facilities and runtime independence

Normal agent/desktop/OpenTUI operation requires no tmux installation/server,
external enrollment, terminal trust/grants, Tailscale, paired phone, companion
process or Onibi checkout. Native coding engines/providers retain their own normal
requirements; those do not become tmux/Onibi requirements.

External adapters have no eager process discovery on ordinary startup. Candidate
enumeration is an explicit local administrative action. The companion lists only
enrolled, separately authorized targets; opening its external section is optional.
Polling occurs only while that view is open, visible and online. Native failures
are contained as target unavailable/stale or uncertain delivery, without invoking
model execution, rewriting memory, changing scheduler/provider selection, moving
Work ownership or altering artifact validation.

Dependency audit covers Go imports/module files, process invocations, environment
and configuration discovery, tests, build scripts and packaging. No Onibi module,
executable, daemon, endpoint, database, auth registry, YAML discovery, ONIBI_*
override or installer is required. Source provenance mentions are historical,
not executable imports. `desktop/scripts/collect-licenses.mjs` reads
`GATOR/third_party/onibi`, not a sibling checkout.

A temporary copied source tree with **no sibling `onibi` directory** successfully
builds the Go runtime and desktop assets/runtime, runs target/actionlog tests,
and runs real ordinary Work and desktop/OpenTUI Go-host startup. It reuses the
existing pinned Go dependency cache and installed JavaScript dependencies, not
another repository's code/runtime. Go dependency closure contains no Onibi
package. This is an isolated source/dependency test, not an offline clean-machine
installation or a signed installer test.

The new `app-host/tests/execution-independence.test.mjs` runs a real Work execution
against a local deterministic provider, opens real desktop/terminal Go hosts,
and exercises local notes with only Git on PATH. tmux, Onibi and Tailscale are
absent; no phone/token/grants are configured. It asserts ordinary operation does
not even open the external-target catalog or initialize terminal admission. This
verifies host transport startup, not a physical Electron/OpenTUI rendering session.

## Retained source and licensing

The selected Onibi Apache-2.0 controller/list/capture, text-tail, literal-send and
finite-key patterns/tests were adapted into Gator-owned tmux code. Bounded runners,
exact identities, OS evidence, local trust, device authorization, actionlog
admission and companion integration are Gator's implementation. Provenance and
modification records remain in `third_party/onibi/NOTICE.md`, with the complete
original Apache text in `third_party/onibi/LICENSE`.

Excluded: Onibi daemon, web server/UI, pairing/auth, RPC/socket services, SQLite
schemas, configuration/intake discovery, session lifecycle management, fonts and
animations, other muxes, Ghostty, SSH and model terminal tools. No source extraction
or feature expansion occurs in T4.5. Gator's project licensing is unchanged.

Desktop packaging already collects the vendored notices. OpenTUI packaging now
also copies them to `licenses/ONIBI-LICENSE` and `licenses/ONIBI-NOTICE.md`, because
its Go runtime includes the adapted code even when tmux is unused. This requires
only files inside Gator. Other redistributors must preserve those notices too.

Onibi can safely become an archived reference repository: Gator's source,
execution, tests and build paths do not depend on it. Keep the vendored license,
attribution and upstream commit provenance. Archiving/moving/deleting Onibi is a
separate owner action and was not performed here.

## Consolidation decisions

- Preserve Work-specific and external-tmux semantics instead of introducing a
  universal execution/session abstraction or lowest-common-denominator controls.
- Preserve separate receipt bindings: Work run/owner semantics differ from
  globally identified external terminal actions. Both reuse actionlog storage,
  file locks, bounded atomic writes and delivery conventions. This is not a
  duplicate Onibi action database.
- Preserve fresh authorization at queue, owner and dispatch boundaries. Repeated
  checks across waits are deliberate, not redundant permission caching.
- Remove the redundant terminal receipt directory-creation/validation branch;
  `terminalNamespace` already owns those checks and parent syncs.
- Correct terminal target-directory durability: sync its immediate parent
  `.terminal/targets` after creating the target directory, rather than syncing
  the distant action-store root. No receipt schema, dispatch/lifetime semantics,
  existing data or target migration changes.
- Retain reserved T0 Ghostty value with an explicit comment, and mark T0–T4 as
  historical records pointing here. Remove current deferred-T5/T6 expectations.
- Preserve the small runner/native-reader test seams. Their fakes are necessary
  for deterministic crash, revocation, stale-target and partial-delivery tests.

## Security guarantees and limits

Authentication is not terminal authorization. External enrollment and backend
review are local-only human decisions; each device needs exact target/generation,
operation and expiry grants. Inspect never authorizes input. Read trust never
implies control trust. Grant lifetime is at most 900 seconds. Registry revocation
and owner-token rotation invalidate future authority; fresh checks surround
capture and dispatch. Models cannot inherit the human phone actor's grants.

Targets are not selected by title, active pane, list position or PID alone.
Native incarnation changes fail closed, with no fallback/re-enrollment. Work
inspection/control routes to its verified actual owner; external tmux uses its
own verified backend identity. Neither creates a competing Work worker.

Durable claims precede native mutation. Matching duplicate actions do not dispatch
twice; mismatched payload/actor/target conflicts. Uncertain claims survive normal
restart, partial text/Enter remains unknown, and receipt GET never dispatches.
Unknown predecessors block new terminal actions. There is no "mark successful"
button. Receipt-history loss disables old enrollment control until fresh local
enrollment/grants; no silent tombstone pruning restores replay permission.

Terminal output and input stay out of ordinary logs, model context, automatic
memory and action payload storage. Snapshots/review text are rendered as untrusted
text. Only minimal receipt identity is retained by the browser; raw drafts remain
in page memory and are bound to the selected target/generation.

Reviewed tmux configuration is user trust, not attestation: aliases/hooks can
change command behavior. Known configuration changes require withdrawal/renewal;
arbitrary runtime changes and malicious same-user processes are not defeated.
Native preflight/dispatch and other keyboards/clients are not atomic/exclusive.
Dispatch acknowledgement proves neither application receipt nor task completion.
Input is host-user authority independent of coding-tool sandbox permissions.

Journal limits remain 128 terminal receipts per target and 4096 globally, with
32 retained enrollment records; unknown outcomes/capacity limits can require local
owner intervention. No automatic pruning or state rollback is declared safe.
Physical-phone, locked-screen, unattended recovery and native Linux/WSL acceptance
must not be inferred from fixture, cross-compilation or Chromium results.

## Verification and final acceptance

```sh
go test ./internal/executiontarget/...
go test -race ./internal/executiontarget/... ./internal/actionlog
go test ./internal/workrun ./internal/actionlog
go test ./...
go vet ./...
npm test --prefix app-host
npm run check --prefix desktop
npm test --prefix desktop
npm run check --prefix ui
node --test companion/auth.test.mjs companion/inspection.test.mjs companion/mutation.test.mjs companion/external.test.mjs companion/terminal.test.mjs companion/recovery.test.mjs companion/server.test.mjs companion/health.test.mjs
node --test companion/client.test.mjs companion/inspection-client.test.mjs companion/mutation-client.test.mjs companion/external-client.test.mjs companion/terminal-client.test.mjs
GATOR_TMUX_LIVE=1 node --test companion/external.test.mjs companion/terminal.test.mjs
GATOR_TMUX_LIVE=1 go test ./internal/executiontarget/... -count=1
```

Native tmux verification uses only disposable isolated servers/sockets. No ordinary
user session is touched. Bun-dependent checks and full terminal packaging require
an existing Bun installation; none is installed merely to complete consolidation.
Signed Electron installation and physical-device/native Linux/WSL workflows are
separate acceptance work. No further integration tranche is initiated.

## Consolidation verification record

Engineering consolidation: **PASS**. Physical-device release readiness remains
separate. Starting point was clean HEAD `d9ab922e2`; no interrupted T5 cleanup was
needed. No agent-created commit, history rewrite or Onibi modification was made.
The sibling `../onibi` existed at the initial audit and became unavailable later
in the session without any move/delete/archive action by this implementation.
Final checks still passed; no assumption is made about where that repository went.

| Check | Result |
| --- | --- |
| `go test ./internal/executiontarget/...` | PASS, three packages |
| `go test -race ./internal/executiontarget/... ./internal/actionlog` | PASS, four packages |
| `go test ./internal/workrun ./internal/actionlog` | PASS |
| `go test ./...` | PASS, 72 packages; 11 without tests |
| `go vet ./...` | PASS, no diagnostics |
| app-host | 78 pass, one existing skip; includes new independence fixture |
| Desktop TypeScript | PASS |
| Desktop tests | Seven pass, one existing skip; nested app-host 78 pass, one skip |
| OpenTUI TypeScript | PASS |
| Companion auth/inspection/mutation/external/terminal/recovery/server/health | 24 pass, two explicitly opt-in native tests skipped |
| Chromium companion journeys | Ten pass, including Work/tmux controls and recovery |
| Isolated native tmux HTTP suites | Six pass, no skips; private disposable sockets only |
| Opt-in Go target suites | Three packages pass, including native key encoding/alias fixtures |
| Isolated copied-source Go build/dependency closure/target and actionlog tests | PASS; no sibling Onibi |
| Isolated ordinary Work, local notes, desktop and OpenTUI Go hosts | PASS with only Git on PATH; no tmux, Onibi, Tailscale, phone or terminal catalog |
| Isolated `npm run build --prefix desktop` | PASS; Go runtime, frontend assets and bundled attribution produced |
| Linux and Windows full Go runtime cross-builds, CGO disabled | PASS; not native OS acceptance |
| Terminal packaging source/syntax audit | PASS, vendored Onibi LICENSE/NOTICE included; full Bun packaging not run |
| Formatting / diff checks | PASS |
| Bun tests and full terminal artifact packaging | Not run: Bun unavailable; no toolchain installed |
| Signed installer, physical phone, native Linux/WSL, locked-screen/unattended acceptance | Not performed |

The isolated desktop build emitted dependency module-directive and bundle-size
warnings but completed successfully. Those are not Onibi dependencies and no
unrelated bundler rewrite was attempted. Tests use a localhost deterministic
provider; no real account/model or ordinary user tmux session is exercised.

Files changed: `internal/actionlog/terminal.go`,
`internal/executiontarget/target.go`, `ui/scripts/package.mjs`,
`app-host/tests/execution-independence.test.mjs`, `README.txt`,
`companion/README.md`, the historical T0/T1/T2/T3/T3.1/T4 documents and this record.

Remaining acceptance work is operator-focused: verify the actual phone over its
configured private connection, token/device revocation and reconnect recovery;
exercise native Linux/WSL tmux behavior on supported versions; complete supported
Bun production packaging and signed installers using the established release
process. No Ghostty or additional terminal integration is a prerequisite or next
tranche. Archive Onibi only as a separate explicit repository-owner action.
