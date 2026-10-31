# T3.1: reviewed tmux backend trust

Gator accepts a locally reviewed, user-trusted backend model. It issues intended
read operations; it does not prove that arbitrary server configuration cannot
translate them into mutations. Review is neither enrollment nor device authority.
The implementation uses the existing private external target catalog and the
existing companion transport. No new registry, protocol, daemon or auth system.

## Source evidence and supported path

The installed native version is tmux 3.8. In upstream 3.8 `cmd-parse.y`,
`cmd_parse_build_command` calls `cmd_parse_expand_alias` before `cmd_parse` resolves
built-ins. `cmd.c:cmd_get_alias` compares configured names without excluding
canonical command names. Thus even `if-shell` can be shadowed. There is no
alias-free guarantee for Gator's supported CLI command path. `-N` prevents server
startup, while `-f /dev/null` does not erase a running server's configuration.
Control mode is not a bypass and static file inspection cannot attest runtime
state. We deliberately do not inspect aliases through an untrusted parser.

Sources: [parser](https://raw.githubusercontent.com/tmux/tmux/3.8/cmd-parse.y),
[alias lookup](https://raw.githubusercontent.com/tmux/tmux/3.8/cmd.c),
[manual](https://man7.org/linux/man-pages/man1/tmux.1.html).

## Local acceptance and identity

Candidate enumeration and enrollment require `--reviewed-read-config`. Before
ANY command (including the offline version invocation), local acceptance obtains
socket device/inode/ctime and executable SHA-256 through passive filesystem
operations. A review record includes these, the local human principal, UTC review
time and exact `trusted_server_read_command_behavior` scope. It does not claim an
alias-free configuration: the owner accepts this backend's command behavior.

Candidate acceptance is invocation-local and does not publish or grant anything.
Enrollment persists review inside the existing entry, adding the server PID and
OS process-start evidence obtained through the now-trusted initial resolution.
The entry's opaque owner, target ID/generation and native incarnation bind it to
the enrolled server/session/pane. No PID or socket pathname alone is authority.
Before commands the adapter compares passive socket evidence, and for enrolled
servers compares OS process start. Constructor validation pins the executable.
Thus a replacement is rejected before running its parser, rather than only after
its output has been inspected. Identity is still checked after capture.

A bare legacy T3 boolean is insufficient. Existing boolean-only entries fail
closed and require explicit new enrollment and separate device grants. No review
is inferred or migrated automatically. The additive review field remains in the
existing versioned catalog; old binaries that reject new fields must not be used
as a downgrade path.

All adapter commands pass the review gate: candidates, enrollment resolution,
grant verification, local status, metadata/list, capture and post-validation.
Catalog authorization also requires a valid local read-scope review. The default
adapter consults current catalog trust before every invocation, so withdrawal
blocks subsequent commands within an ongoing multi-command inspection. Current
authority is rechecked before returning output. Revocation/cleanup itself uses
only catalog operations and never invokes tmux. No recovery path automatically
accepts a replacement.

## Commands and withdrawal

Use the companion's existing profile and an already running private socket:

```sh
GATOR_INTERNAL_COMMANDS=1 /path/to/gator-runtime external-tmux candidates --reviewed-read-config /absolute/path/to/tmux /private/path/socket
GATOR_INTERNAL_COMMANDS=1 /path/to/gator-runtime external-tmux enroll --reviewed-read-config /absolute/path/to/tmux /private/path/socket '%3' 'Build monitor'
GATOR_INTERNAL_COMMANDS=1 /path/to/gator-runtime external-tmux list
node companion/external-access.mjs /path/to/gator-runtime /private/path/access.token TARGET_ID DEVICE_ID 300
GATOR_INTERNAL_COMMANDS=1 /path/to/gator-runtime external-tmux unreview TARGET_ID
```

The flag explicitly accepts the selected backend's runtime configuration and
command behavior. Host-local administrative actions are trusted user actions;
no model tool or remote enrollment/review endpoint exists. Supplying a model or
remote reviewing principal is rejected. This is a policy boundary, not a defense
against arbitrary same-user OS code execution.

`unreview` withdraws read trust from ALL retained enrollments sharing that native
server instance, clears their grants and tombstones them. It works even if the
server is gone. `revoke TARGET_ID` retains its existing single-target enrollment
revocation behavior. Both prevent further inspection. Renewal uses fresh explicit
enrollment, fresh target/generation and new device grants; it does not revive old
authority. Inspect grants retain the 900-second maximum and existing device
revocation/owner rotation checks, independently of review.

## Guarantee and limits

Gator requests only fixed guarded list/display/capture reads, never changes tmux
options or aliases, starts no server, installs no hooks, selects no pane, resizes
nothing and sends no input. Existing output/deadline/HTTP limits are preserved.
The UI describes snapshots as originating from locally trusted backends and does
not present them as attested safe configuration or trusted approval evidence.

Runtime aliases can change after review without changing process/socket identity.
Gator does NOT automatically detect such changes safely. Known changes require
local `unreview` before the change, followed by review/new enrollment if desired.
Undetectable runtime changes remain an accepted limitation of the user-trusted
backend model. Likewise, a malicious same-user process can alter private state,
replace descriptors or impersonate the owner; this is not cryptographic attestation.
Passive checks and dispatch have ordinary local TOCTOU limits, and revocation
cannot recall output already returned or prevent a command already dispatched.

Disposable native tests demonstrate canonical capture aliases causing a harmless
sentinel option change, aliases introduced after review, bootstrap `if-shell`
shadowing, malformed aliases and withdrawal blocking an aliased bootstrap.
Production never installs these aliases. The developer's ordinary server is never
used by these tests.

## T4 conditions (not implemented)

Recommend a separate explicit `--reviewed-control-config`-style acceptance within
the same catalog/local grant workflow, with a distinct control trust scope. Read
review must not authorize mutation. Control also requires exact target/generation
and operation-specific device grants, fresh pre-dispatch identity/authorization,
and durable receipts distinguishing not dispatched, dispatch acknowledged and
unknown outcome. Clients must never choose executables/configuration, rewrite
aliases or issue review. Server replacement invalidates all relevant review and
grants; known configuration changes require withdrawal/renewal of relevant
scopes. Undetectable runtime changes remain acknowledged, including for control.
No T4 inputs, Ctrl-C, receipts or control review issuance were implemented here.

## Verification

Run `go test ./internal/executiontarget/...`,
`go test -race ./internal/executiontarget/...`, `go test ./...`, `go vet ./...`;
`npm test --prefix app-host`, `npm run check --prefix desktop`,
`npm test --prefix desktop`, `npm run check --prefix ui`.

Native opt-in: `GATOR_TMUX_LIVE=1 go test ./internal/executiontarget/... -count=1`.
Companion: `GATOR_TMUX_LIVE=1 node --test companion/auth.test.mjs companion/inspection.test.mjs companion/mutation.test.mjs companion/external.test.mjs companion/recovery.test.mjs companion/server.test.mjs companion/health.test.mjs`.
Chromium: `node --test companion/client.test.mjs companion/inspection-client.test.mjs companion/mutation-client.test.mjs companion/external-client.test.mjs`.
Bun tests run only when Bun is already installed. Automated/native fixture tests
are not physical-phone acceptance or native Linux/WSL acceptance.

## Acceptance evidence — 31 October 2026

**PASS under the explicitly reviewed, user-trusted backend model.** This verdict
makes claim A (intended reads), not claim B (configuration cannot transform them).

| Verification | Result |
| --- | --- |
| Target package tests, including opt-in isolated native aliases | PASS, three packages |
| `go test -race ./internal/executiontarget/...` | PASS |
| `go test ./...` | PASS, 72 packages; 11 without tests |
| `go vet ./...` | PASS, no diagnostics |
| App-host | 77 pass, one existing skip |
| Desktop TypeScript | PASS |
| Desktop tests | Seven pass, one existing skip; nested app-host 77 pass, one skip |
| OpenTUI TypeScript | PASS |
| Companion auth/inspection/mutation/external/recovery/server/health with live opt-in | 23 pass, no skips |
| Chromium journeys | Nine pass |
| Final real-helper external suite, including server-wide unreview, fresh enrollment and replacement | Three pass |
| Linux and Windows tmux adapter cross-compilation | PASS; not native runtime acceptance |
| Formatting and diff checks | PASS |
| Bun | Unavailable; not installed or run |
| Physical phone / native Linux / WSL | Not performed |

Files added: `internal/executiontarget/tmux/review.go`, this document.
Files adapted: tmux `controller.go`, `controller_test.go`, `live_test.go`;
external `catalog.go`, `catalog_test.go`; `cmd/gator/companion_external.go`;
`companion/external.test.mjs`, `companion/public/index.html`,
`companion/README.md`, `docs/EXECUTION-TARGETS-T3.md`.
Existing unrelated work was preserved. No changes to Onibi, toolchains, project
licensing, Work execution ownership, authentication infrastructure or T4 controls.
