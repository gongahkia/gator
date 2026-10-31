# T3: explicitly enrolled external tmux inspection

Status: T3.1 formalizes acceptance under the **reviewed, user-trusted backend
model**. See [T3.1](EXECUTION-TARGETS-T3.1.md) for the instance-bound review record,
pre-invocation passive checks, withdrawal and dynamic-configuration limitations.
This does not establish automatic safety for arbitrary tmux configurations.
Physical-phone acceptance was not performed.

T3 adds read-only observation of independently started tmux panes. External panes
are not Work executions: no Work owner, worker, run, specialist, recovery, model
execution or semantic-control routing is involved. T0–T2 remain responsible for
Gator-owned Work. No external mutation operation is implemented.

## Architecture and provenance

`internal/executiontarget/tmux` adapts Onibi's low-level argv runner/controller,
list/capture and text-tail patterns from `internal/tmux/attach.go` and
`internal/render/text.go`; related attach/mux/render test patterns were adapted.
Gator's implementation replaces session-name targeting, unbounded CombinedOutput,
implicit configuration/environment selection and escape-preserving rendering.
`third_party/onibi/NOTICE.md` records source commit and modified files;
`third_party/onibi/LICENSE` preserves the complete upstream Apache-2.0 license.
The desktop license collector includes both in its distributed dependency notices.
Other binary redistributors must include these files. This does not license the
rest of Gator: no root project license existed at extraction time. Onibi's fonts,
animations, daemon, HTTP/auth runtime, RPC, intake, project discovery and SQLite
schemas were excluded; `../onibi` was read only.

`internal/executiontarget/external` owns a versioned private enrollment/grant
catalog, using existing `privatefs`, `workspace` bounded atomic writes and
`filelock` cross-process shared/exclusive locks. It is not a session database or
process supervisor. Catalog maximums: 32 enrollments, eight retained unexpired grants per
target, 512 KiB file; lists return at most 16 authorized live targets. Revoked
entries remain as tombstones and count toward the enrollment limit.

`internal/companionaccess.Reference` factors T2's existing private device-registry
proof out of Workrun. Workrun retains its original `CompanionAuthority` contract
and delegates the same checks. No additional authentication store is created.

The existing Node companion invokes `gator-runtime companion-external` as a
short-lived Go read helper under the companion's OS account/profile. No worker,
maintenance, provider initialization or new daemon is started. Its environment
routing flag is not authorization: target-bound grants and enrollment are checked
inside Go. The existing companion serial queue (maximum 32 pending requests),
Host/Origin checks, no-store headers, device registry and owner rotation remain
in force. Captures do not depend on Electron, OpenTUI or app-host lifetime.

## Exact pane binding and failure policy

Enrollment pins a trusted absolute tmux executable (SHA-256 fingerprint) and
explicit absolute server socket in a private, current-user-owned parent directory.
No phone-supplied executable, native pane ID, socket, configuration or environment
is accepted. No ONIBI configuration or executable overrides are imported.

Identity includes the native `%pane` ID and `$session` membership/creation time,
pane PID plus OS process-start evidence, server PID/start_time plus OS process-start
evidence, and socket device/inode/ctime. macOS uses sysctl process start seconds
and microseconds; Linux uses boot ID plus `/proc/PID/stat` start ticks. The opaque
Gator ID and random generation are newly issued on every enrollment. The backend
instance and incarnation hashes expose none of the native configuration.

Active-pane changes, pane ordering and window renaming do not select a different
pane. Window ID is descriptive local metadata, not identity. Moving the pane into
another session, respawning its process, replacing the executable/socket/server,
or losing the native evidence invalidates enrollment. Resolution before and after
capture must match; raced snapshots are discarded. Persistent metadata never
proves a process survives restart. Any inability to re-establish identity/read
safety conservatively tombstones the target and clears its grants; recovery
requires explicit new local enrollment. No fallback or automatic re-enrollment.
These are available OS/native evidence guarantees, not a cryptographic attestation
against a malicious same-user tmux server, administrator or locally replaced OS.

Supported native probes: macOS, Linux and Linux-based WSL with working procfs.
Windows preview and other OSes return unavailable. Current client/server versions
accepted: tmux 3.4–3.8 (optional patch suffix); unknown versions fail closed.
Automated live testing in this checkout used macOS and tmux 3.8. Linux/WSL native
runtime acceptance remains separate from cross-compilation.

## Read-only command safety and capture limits

Only fixed native tmux `list-panes`, `display-message`, and `capture-pane -p`
commands are permitted. All subprocesses use argv, an explicit socket, `-N`
(no server start) and `-f /dev/null`; the environment contains only fixed PATH,
locale and TERM values. No attach, buffer writes, hooks/options installation,
pane selection, resizing, input, creation or termination is implemented.

Read commands can invoke user-configured tmux after/error hooks. A fixed
`if-shell -F` format guard refuses nonempty command-error/after-list-panes/
after-display-message/after-capture-pane hooks. `-F` is format-only: no OS shell
runs. The CANFAIL pane target prevents a missing pane from selecting another pane
or triggering a missing-target error hook. Nested native command text contains
only constants, validated numeric pane IDs and bounded integers. No OS shell
command string contains user input. Unsupported hooks/configuration fail closed;
the adapter never changes configuration to make a read work. This relies on a
trusted local server/configuration; concurrent hostile same-user configuration
changes cannot be made safe by remote authorization.

Relevant upstream semantics:
[tmux manual](https://raw.githubusercontent.com/tmux/tmux/master/tmux.1),
[format-only if-shell](https://raw.githubusercontent.com/tmux/tmux/3.8/cmd-if-shell.c),
[capture command hooks](https://raw.githubusercontent.com/tmux/tmux/3.8/cmd-capture-pane.c).

Each subprocess has a three-second deadline, stdout capped while consuming at
64 KiB (version probe 256 bytes), stderr capped at 4 KiB and discarded. Overflow
cancels the subprocess; bounded partial capture is reported truncated. The
short-lived helper has a 15-second overall deadline and Node a 20-second timeout.
Native capture requests a fixed maximum of 200 physical rows near the current
screen; HTTP snapshots return at most 100 recent lines and 16 KiB UTF-8 text.
History/row and byte cuts set `truncated`. This is not a full scrollback transcript
or terminal emulator. Candidate enumeration is local only, at most 256 panes.
A list is bounded to 16 authorized targets; inspect the others locally.

CSI/OSC/DCS/SOS/PM/APC sequences and control/format characters are conservatively
removed, with UTF-8-safe trimming. Rendering always uses textContent, never HTML.
This is not a universal terminal sanitizer and terminal text is never trusted
application review evidence. No output is saved in the catalog, diagnostics,
receipts, conversation memory, analytics or model context. Native errors become
a fixed unavailable reason without stderr, filesystem paths or command arguments.

## Host-local workflow

Use the same `GATOR_STATE_DIR` profile as the companion. The executable and socket
below must refer to an already running, independently managed tmux server. The
socket parent must be private (mode 0700) and owned by the companion's OS user.
T3 does not initialize tmux or change existing socket permissions/configuration.
Local commands use the repository's existing internal administrative convention:

```sh
GATOR_INTERNAL_COMMANDS=1 /path/to/gator-runtime external-tmux candidates --reviewed-read-config /absolute/path/to/tmux /private/path/socket
GATOR_INTERNAL_COMMANDS=1 /path/to/gator-runtime external-tmux enroll --reviewed-read-config /absolute/path/to/tmux /private/path/socket '%3' 'Build monitor'
GATOR_INTERNAL_COMMANDS=1 /path/to/gator-runtime external-tmux list
node companion/external-access.mjs /path/to/gator-runtime /private/path/access.token TARGET_ID DEVICE_ID 300
node companion/external-access.mjs /path/to/gator-runtime /private/path/access.token TARGET_ID DEVICE_ID 0
GATOR_INTERNAL_COMMANDS=1 /path/to/gator-runtime external-tmux revoke TARGET_ID
```

`candidates` lists native identity metadata only on the host. `enroll` returns the
opaque target binding. `list` revalidates enrollment status and reports retained
failures locally; it does not capture output. The grant command accepts an
already paired device ID (or explicit `owner` for owner-token access), 1–900
seconds; zero revokes that actor's grant. A grant is neither enrollment nor a
permission for other devices. Normal device revocation and owner-token rotation
invalidate the retained authority reference. The browser cannot enroll or mint
grants. Retained grants across helper/companion restarts still require current
device authority, nonexpired exact binding and re-established native evidence.

## Companion API and interface

Authenticated, read-only endpoints:

- `GET /v1/external`: enrolled, separately authorized, verified targets only.
- `GET /v1/external/:target?generation=...`: selected metadata, no output.
- `GET /v1/external/:target/snapshot?generation=...`: bounded snapshot.

Requests select opaque Gator IDs and exact generations only. Unsupported methods
and extra/repeated query parameters are rejected. `external.ts` defines the typed
response; `external.mjs` validates and explicitly projects backend data before
returning it (64 KiB maximum). No endpoint accepts a grant or native configuration.
Unavailable selected targets return an empty list and fixed unavailable reason.
Lists omit unauthorized/stale targets, including after an identity failure.

Go checks current enrollment, exact generation, human remote actor, Inspect grant,
expiry and current device registry before native access and again after capture
and native revalidation. A final locked authorization read is the result's
linearization point; revocation during capture withholds its output. Node checks
authentication after waiting in the queue and again before returning the response.
Work grants cannot authorize tmux; tmux Inspect grants cannot authorize Work or
terminal input, models, cancellation, keys or interruption. Revocation cannot
recall bytes already returned or screenshots taken by the user.

The separate External terminals section leaves Active Work/chat controls intact.
It displays a friendly label, backend, opaque target ID, capture time, availability
and truncation. It polls every five seconds only while open, visible and online;
manual refresh is available. Per-request epochs, AbortController and bound target
ID/generation prevent A/B selection and stale-response overwrites. Closing,
hiding, disconnecting, errors and disappearing/unauthorized targets clear output.
No draft, offline action queue, input composer, terminal emulator or controls.

## Verification and rollout boundaries

```sh
go test ./internal/executiontarget/...
go test -race ./internal/executiontarget/... ./internal/companionaccess
go test ./...
go vet ./...
GATOR_TMUX_LIVE=1 go test ./internal/executiontarget/tmux -run TestLive -count=1 -v
GATOR_TMUX_LIVE=1 node --test companion/external.test.mjs
node --test companion/external-client.test.mjs
node --test companion/auth.test.mjs companion/server.test.mjs companion/recovery.test.mjs companion/inspection.test.mjs companion/mutation.test.mjs
npm test --prefix app-host
npm run check --prefix desktop
npm test --prefix desktop
npm run check --prefix ui
```

The opt-in tests create isolated private temporary socket namespaces and clean up
only their own sessions. They deliberately use fixture mutations to create/change
panes; production exposes none of those operations. The Node live test exercises
the real Go helper, local enrollment, real paired device registry, companion HTTP,
capture, local grant revocation and server replacement. Fake readers reproduce
revocation while capture is pending; Chromium checks untrusted text, A/B races,
output clearing and polling. These are automated acceptance, not physical-phone
acceptance. Run Bun tests only when Bun is already installed.

Manual phone acceptance: follow the enrollment/grant commands, open the existing
companion over its configured private HTTPS connectivity, select the external
pane and compare its snapshot with that pane. Change the active pane and window
name; refresh must retain the enrolled pane. Revoke the grant/device or close the
pane: subsequent refresh must clear output. Verify Active Work, existing controls
and conversations still work. No physical-device result should be inferred from
Chromium tests.

T4 is deferred: no Ghostty/Zellij/Screen, SSH, raw input/keys, interruption,
external semantic controls, model tools, hooks or supervisor migration. Any later
mutation needs separate operation grants and appropriate durable admission; T2
Work receipts do not confer authority over external terminals.

## Implementation manifest and verification record

Created:

- `internal/executiontarget/tmux/{controller.go,runner.go,text.go,probe_darwin.go,probe_linux.go,probe_other.go,controller_test.go,live_test.go}`.
- `internal/executiontarget/external/{catalog.go,catalog_test.go}`.
- `internal/companionaccess/authority.go` (existing T2 registry proof factored out).
- `cmd/gator/companion_external.go`.
- `app-host/contracts/external.ts`.
- `companion/{external.mjs,external-access.mjs,external.test.mjs,external-client.test.mjs}`.
- `companion/public/external-view.mjs`.
- `third_party/onibi/{LICENSE,NOTICE.md}` and this document.

Adapted existing files:

- `internal/workrun/control_authority.go`: delegate unchanged authority semantics.
- `cmd/gator/desktop_runtime.go`: early, maintenance-free helper/admin routing.
- `app-host/contracts/protocol.ts`: export external display contracts.
- `companion/server.mjs`: read-only external routes/helper and static view module.
- `companion/public/{client.mjs,index.html,styles.css}`: independent snapshot view.
- `companion/README.md`: actual local commands and privacy/lifecycle boundaries.
- `desktop/scripts/collect-licenses.mjs`, `desktop/licenses/DEPENDENCIES.txt`: include
  Onibi attribution/license, preserving previously checked-in notices.

The pre-existing `companion/mutation.test.mjs` working-tree modification was left
untouched. No commits, branch resets, toolchain/dependency changes, or reference
repository modifications were made.

Verification on 31 October 2026:

| Check | Result |
| --- | --- |
| `go test ./internal/executiontarget/...` | PASS, three packages |
| `go test -race ./internal/executiontarget/... ./internal/companionaccess` | PASS; authority package has no standalone tests (exercised through catalog/Work tests) |
| `go test ./...` | PASS, 72 packages; 11 packages without tests |
| `go vet ./...` | PASS, no diagnostics |
| `npm test --prefix app-host` | 77 pass, one existing skip |
| `npm run check --prefix desktop` | PASS |
| `npm test --prefix desktop` | Seven desktop tests pass, one existing skip; nested app-host 77 pass, one skip |
| `npm run check --prefix ui` | PASS, OpenTUI TypeScript |
| `node --test companion/auth.test.mjs companion/server.test.mjs companion/recovery.test.mjs companion/inspection.test.mjs companion/mutation.test.mjs companion/health.test.mjs companion/external.test.mjs` | 22 pass, one opt-in live tmux test skipped |
| `node --test companion/external-client.test.mjs companion/client.test.mjs companion/inspection-client.test.mjs companion/mutation-client.test.mjs` | Nine Chromium journeys pass |
| `GATOR_TMUX_LIVE=1 go test ./internal/executiontarget/tmux -run TestLive -count=1 -v` | One isolated native test passes, macOS/tmux 3.8 |
| `GATOR_TMUX_LIVE=1 node --test companion/external.test.mjs companion/external-client.test.mjs` | Four pass, including real Go/helper/paired-device/tmux HTTP path and browser race journey |
| `GOOS=linux GOARCH=amd64 go test -c ./internal/executiontarget/tmux -o /tmp/gator-t3-tmux-linux.test` and corresponding external catalog command | Compile PASS; native Linux/WSL runtime not exercised |
| Corresponding `GOOS=windows GOARCH=amd64 go test -c` commands | Compile PASS; Windows capture remains unsupported |
| `gofmt -l` on changed Go source; `git diff --check` | PASS, no output |
| Bun tests | Not run: Bun unavailable; nothing installed |
| Physical phone | Not performed; automated acceptance only |

No existing Work lifecycle, native coding control, artifact validation, scheduler,
provider, memory or UI migration schema was modified. The terminal frontend
migration remains ongoing independently; T3 adds display contracts without changing
its frames or execution ownership. The new catalog has its own additive version;
it performs no migration of existing Gator data.

### Explicit local configuration initialization

Before either candidate enumeration or enrollment, a trusted host administrator
must review the existing server's command-alias configuration. The canonical
`if-shell`, `list-panes`, `display-message` and `capture-pane` names can be redefined by configuration. The local owner explicitly accepts the selected
server's command behavior and must withdraw review before known configuration changes.
The required `--reviewed-read-config` flag records this explicit local review;
unreviewed configurations are rejected before connecting. The flag is never
accepted from HTTP. The isolated test servers start with `/dev/null` configuration
and satisfy this prerequisite.

This prerequisite is necessary because tmux expands configured aliases before
executing a command, including the initial safety guard. Safely discovering an
overridden guard using that same command parser would be circular. Gator does
not clear aliases or alter server configuration. Host configurations that cannot
be reviewed/kept trusted are unsupported. The acknowledgement is a local trust
decision, not automated configuration attestation; unknown or concurrently changed
canonical aliases are a remaining limitation. See tmux's
[alias expansion](https://raw.githubusercontent.com/tmux/tmux/3.8/cmd-parse.y) and
[alias lookup](https://raw.githubusercontent.com/tmux/tmux/3.8/cmd.c).
