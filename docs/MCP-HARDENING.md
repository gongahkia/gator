# MCP transport hardening

Scope: bounded tool discovery, streamed replies, subprocess lifecycle and explicit
legacy/modern tools compatibility. This extends the existing Gator MCP client;
it does not add another agent, runtime, daemon, authentication store or SDK.
No dependency or Go toolchain changes are required.

## Architecture

- `mcp.go`: existing configuration/trust, credential selection, sandbox launch,
  model-tool wrappers and connector entry point.
- `protocol.go`: read-only setup/version checks, complete bounded pagination,
  safe model-facing aliases and tool-result semantics.
- `rpc.go`: bounded JSON-RPC framing, exact response correlation, incremental SSE
  and cancellable request-gate acquisition.
- `http.go`, `headers.go`: authenticated HTTP requests, modern wire metadata,
  explicit legacy sessions, validated schema-derived headers and cancellation.
- `stdio.go`, platform process helpers: owned subprocess pipes, bounded reader
  queue, cancellable I/O, graceful EOF and forced cleanup.

Existing private credential/resource binding, manifest/executable hash trust,
per-tool approval, host capability grants and sandbox policy remain authoritative.
Opening an untrusted or changed configuration performs no discovery or process
launch. Optional integration access remains shared by the main coding session
and delegated workers through the existing coding harness. No new frontend
control or model-facing policy bypass was added.

## Protocol selection

The preferred tools path is `2026-07-28`. A two-second read-only
`server/discover` probe carries modern per-request metadata. A valid response
must advertise that exact supported version and a tools capability. No numeric
"future version is probably compatible" inference is used.

Recognized modern errors are handled explicitly. `UnsupportedProtocolVersion`
may select the implemented legacy revision only when advertised; no mutual
version fails. A recognized modern missing-capability/header error is not
silently downgraded. Legacy RPC setup errors, or legacy HTTP 400/404/405/415
responses, can select the `2025-11-25` handshake. A silent stdio probe can time out
and fall back in the same owned process; its late response is ignored by ID.
An HTTP probe timeout/network/authentication failure is reported rather than
assumed to prove legacy compatibility. Legacy initialization must return the
supported version and a tools capability before initialized notification and
listing. Malformed modern success responses are rejected.

Version selection is restricted to read-only setup. Tool calls never trigger
protocol fallback, process restart, session recreation or retry. Modern
`input_required`, unknown result kinds and tool `isError` results cannot be
misreported as successful ordinary tool results. Server-authored RPC error text
is not copied into diagnostic errors.

The supported subset is initialization/discovery, tool listing and calls, and
bounded notifications needed for lifecycle/cancellation. No resources, prompts,
sampling, elicitation, subscriptions, tasks, legacy HTTP+SSE endpoint transport
or resumable streams are added. No model is invoked to fulfill a server request.
Existing OAuth is retained; this change does not claim complete modern OAuth
conformance or every vendor/server extension.

## Bounds and failure behavior

| Boundary | Limit / behavior |
| --- | --- |
| Servers | Existing maximum 32 |
| Discovery | 32 pages, 256 tools across configured servers, 1 MiB per server catalog |
| Cursors | Opaque, 4 KiB each; loops rejected; no partially enabled catalog |
| Tool names | ASCII letters/digits/underscore/hyphen/dot, 1–128 bytes; bounded safe model aliases; collisions rejected |
| JSON-RPC request/message / SSE event or line | 128 KiB; valid UTF-8 messages |
| Tool result | Existing 64 KiB |
| HTTP stream / stdio call messages | 1 MiB, 256 nonempty events/messages |
| Stdio pending reader queue | 8 bounded messages; backpressure instead of unbounded buffering |
| Queue + operation | Two minutes total, or shorter caller deadline |
| Read-only protocol probe | Two seconds, capped by caller/setup deadline |
| Stdio cancellation notification | At most 100 ms; no claim that a remote effect was undone |
| Shutdown | Close pipes; 250 ms graceful wait, forced owned-process termination, 250 ms bounded reap wait |
| HTTP schema-derived headers | At most 32 mappings/tool, 24 schema nesting levels, 16 KiB generated headers |

SSE parsing handles comments/keepalives, CRLF, multiline data, empty events,
notifications and unrelated responses before the matching result. It returns
as soon as that result is framed, even if the server keeps its connection open.
Missing, malformed or oversized responses fail; output is never silently trimmed
into apparent success. JSON bodies are bounded too.

Both transports serialize calls with context-aware gates. A cancelled queued
call cannot later acquire the gate and dispatch. Close cancels live HTTP reads
and rejects future calls. Stdio cancellation unblocks blocked writes/reads,
requests cancellation when possible, and tears down the owned server. POSIX
servers use an owned process group; Windows retains the existing strict-sandbox
unavailability and has no new unsafe fallback. A process that escapes its group
or cannot be killed is not guaranteed terminated; failure to observe exit is an
explicit shutdown error and private scratch cleanup waits for eventual exit.

A reply lost after dispatch can leave external effects uncertain. No transport
replay is performed; automatic HTTP redirects are disabled. Existing mapped
connector mutations continue to use their separate exact-action approval and
receipt path. Generic trusted MCP tool approval is not a new universal durable
transaction guarantee for arbitrary server effects. Local fixture acknowledgement
never proves a foreground application or remote workflow succeeded.

## Modern HTTP headers

Version/capabilities/client identity are generated by Gator from the selected
protocol. `Mcp-Method` and `Mcp-Name` match the transmitted JSON body. Primitive
`x-mcp-header` mappings must be statically reachable through `properties`, have
unique valid header tokens and use string/boolean/safe integer types. Unsupported
or malformed definitions reject catalog admission as a whole; Gator does not
silently publish a partial list. Non-ASCII/control/edge-whitespace/sentinel values
use the specified Base64 encoding. No annotation can replace Authorization,
redirect the endpoint or select a local executable. Native names remain native
on the wire, while public model aliases cannot collide or exceed 64 characters.

## Verification

Final automated results are recorded below. All protocol servers, accounts and
model replies are controlled private fixtures. Stdio tests execute a child copy
of the Go test binary, never an installed developer server or a shell command.
No paid API request, real-account mutation or physical-device test is involved.

The new adversarial cases cover later-page invocation; empty intermediate pages;
cursor loops; page/tool/byte limits; duplicate/colliding tool identities;
notifications/multiline/CRLF SSE; results before EOF; wrong IDs; malformed/large
messages; invalid UTF-8; native timeouts; queue cancellation; lost replies;
redirected credentials; modern/legacy wire metadata; unsupported versions and
interactions; header binding and injection; untrusted/changed configuration;
stalled subprocess reads/writes; ignored EOF; real process reaping; repeated
close; and no transport replay. Existing OAuth resource-binding tests remain.

The Work integration fixture additionally freezes the original trusted project
configuration, discovers the tool on a later page, invokes it once after the
existing exact-tool approval callback, then executes/verifies the existing
isolated coding patch. This is real Work/HTTP execution with scripted model
responses, not commercial-model quality or third-party-server acceptance.

## Standards checked

Implementation was checked against the primary protocol documents:

- [Legacy lifecycle](https://modelcontextprotocol.io/specification/2025-11-25/basic/lifecycle)
- [Legacy tools/pagination](https://modelcontextprotocol.io/specification/2025-11-25/server/tools)
- [Modern versioning](https://modelcontextprotocol.io/specification/2026-07-28/basic/versioning)
- [Modern HTTP framing/headers](https://modelcontextprotocol.io/specification/2026-07-28/basic/transports/streamable-http)
- [Modern stdio lifecycle](https://modelcontextprotocol.io/specification/2026-07-28/basic/transports/stdio)
- [Modern discovery/result schema](https://modelcontextprotocol.io/specification/2026-07-28/schema)

Compatibility evidence is specific to the two implemented revisions and tested
tools subset. Actual vendor OAuth consent, server-specific schemas/extensions,
Windows-native execution and native Linux sandbox operation need separate
acceptance; cross-compilation is not runtime acceptance.

## Final results (2 November 2026)

| Check | Observed result |
| --- | --- |
| `go test -race -json ./internal/mcp ./internal/connector` | PASS: MCP 36 top-level tests / 87 passing test and subtest events; connector 36 top-level / 48 events. Package elapsed 15.478 s / 3.874 s. Includes the subprocess fixture entry, not 36 independent vendor acceptance scenarios. |
| `go test ./cmd/gator -run '^TestWorkCodeResolvesCapturedIntegrationsUsingOriginalTrust$' -count=1` | PASS, 7.029 s; frozen trust, later-page tool, approval before one dispatch, coding patch verification. |
| `go test -p 2 ./...` | Final PASS: 72 packages, 11 without tests; `cmd/gator` 256.937 s, `workrun` 73.512 s, MCP 7.719 s. |
| `go vet ./...` | PASS, exit 0. |
| `npm test --prefix desktop` (includes application host) | Desktop 31 passed / 1 skipped; host 80 passed / 1 skipped. |
| `npm run check --prefix desktop` | PASS. |
| `npm run build --prefix desktop` | PASS; runtime rebuilt once more against the final backend before Electron confirmation. Existing non-fatal bundler warnings remain. |
| `GATOR_E2E_BACKGROUND=1 npm run test:e2e --prefix desktop -- coding.spec.ts app-control.spec.ts --max-failures=3` with live API variables unset | 14 passed / 1 opt-in paid live-coding test skipped, 2.1 min. Real Electron/Go with scripted model/services; coding implementation, repair, review, follow-up, cancellation, voice and sources. |
| `GATOR_TEST_BINARY="$PWD/desktop/runtime/gator-runtime" node --test companion/*.test.mjs` | Final 37 passed / 5 skipped / 0 cancelled, 39.439 s; Chromium, authentication, owner routing, inspection, semantic controls, external targets, receipts and recovery. |
| `GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go test -c -o /tmp/gator-mcp-linux.test ./internal/mcp` | PASS, compile only. |
| `GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go test -c -o /tmp/gator-mcp-windows.test.exe ./internal/mcp` | PASS, compile only. |
| `gofmt -l` on changed Go files; `git diff --check` | Clean. |

The host already provided Go 1.27.0, Node 24.19.0 and Bun 1.4.2. The module's Go
1.25.13 declaration, dependency manifests and toolchains were not changed. No
applicable Bun-dependent suite remains for this backend work; supported app tests
use Node/Playwright, and Bun was not substituted for those commands. Native
Linux/WSL/Windows operation, signed installers, physical microphone/audio, actual
MCP vendor accounts and phones are not established by these checks.

Node skips are the opt-in panel-secret Electron check and native Windows argv
handoff. Companion skips are two opt-in isolated real-tmux tests, a native Linux
supervisor workflow, native Linux user-manager service and native macOS launchd
service. No ordinary user server/session or account was used. No new paid model
or search evaluation was started.

### Retained failed attempts and corrections

The first transport fixture runs failed because their old responses omitted
required initialization metadata/JSON content type and notification
acknowledgement. Those fixtures now explicitly implement the legacy protocol;
modern fixtures separately require stateless metadata and headers. An initial
subprocess test used plain `exec.Command` while the owned-process cancellation
hook requires `CommandContext`; the fixture was corrected to match the existing
production sandbox launcher. No production timeout was widened.

The first broader Electron run had four passes, one local-voice cleanup failure
and ten unexecuted cases after its failure limit. The voice assertions completed,
but its fixture main process did not exit within the existing ten-second cleanup
deadline. The isolated voice rerun passed in 13.7 s total, and the complete final
14-journey confirmation then passed. There was no production voice/lifetime
change and no claim that an MCP regression caused the earlier cleanup failure.

The first companion run had 36 passes, one failure and five skips. Its health
journey rotated the owner token, then clicked Refresh; automatic polling could
already detect revocation and hide that button. The test now waits directly for
the expired-access state, still asserting cleared output and zero mutations.
That focused journey passed (16.594 s total) and the complete companion rerun
passed. Companion runtime authentication/revocation behavior was not weakened.

No automated test result is presented as universal MCP compatibility, real-account
OAuth acceptance or a guarantee of external application effects. Remaining work
is vendor-specific/manual acceptance and separately scoped optional MCP features,
not automatic replay or broader implicit authority.
