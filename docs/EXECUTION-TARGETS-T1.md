# T1: remote inspection of live owned Work

Final scope: see [T4.5 consolidation](EXECUTION-TARGETS-T4.5.md). This is a
historical tranche record. Current scope is owned Work supervision and optional
external tmux; Ghostty T5/T6 are permanently removed from the current roadmap.

T1 adds read-only Work inspection to the existing companion. It builds on
`internal/executiontarget`; it does not add terminal controls or external targets.

## Owner routing

When an existing foreground Work control channel starts, its owner issues one
opaque Work target binding. The binding's owner and native incarnation identify
that control-channel instance using its random nonce; its generation is separately
random. The private `gator/work-control/RUN.json` record includes the binding
alongside the existing loopback endpoint, control token, and nonce. Old controls
without an inspection binding remain unsupported. Existing execution leases,
lifetimes, cancellation, verification, and control operations are unchanged.

`workrun.liveInspection` is an observer in the actual executor. It records bounded
public runner narration/tool names and sanitized specialist status from existing
events, plus a pending-approval count from `Operation`. It never reads a model's
reasoning, returns tool payloads, consumes channels/interactions, sends input,
calls a provider, or changes ownership. It retains the most recent 24 progress
items (512 UTF-8 bytes each), at most 16 specialist summaries, and no terminal
transcript. Specialist native status/question indicators remain semantic data;
native thread IDs, question contents, and protocol payloads are excluded.

The companion's fixed `companion-inspect` helper invokes
`workrun.InspectOwnedWork`. It scans at most 128 control-record names, returns
at most 16 authorized targets, and uses the existing `connectWorkControl` path:
verify the running history lease, literal loopback endpoint, and live health
nonce, then ask that same owner for its inspection projection. History only
locates/verifies a live run; it never supplies invented live output. Target and
generation are checked against the record and response. A stopped, replaced,
unreachable, ungranted, or old owner is unavailable. Discovery has a 15-second
overall helper deadline and bounded HTTP/JSON responses. A truncated catalog
requires narrowing/retiring live work; pagination is deferred.

No `Backend`, conversation worker, UI process, or execution is created for these
reads. Desktop, OpenTUI, and companion-owned Work use the same Go service/control
seam. The inspector does not keep an execution alive after its owner closes.

## Explicit authorization

Conversation access does **not** grant Work inspection, including for the owner
token. The host operator explicitly grants Inspect on one currently live owner
to one enrolled device (or the current owner-token principal):

```sh
# Use the same GATOR_STATE_DIR as the execution host.
node companion/devices.mjs /private/path/owner.token list
node companion/inspection-access.mjs /path/to/gator-runtime \
  /private/path/owner.token RUN_ID DEVICE_ID 300
# Explicit local revocation; 0 revokes, 1–900 grants for that many seconds.
node companion/inspection-access.mjs /path/to/gator-runtime \
  /private/path/owner.token RUN_ID DEVICE_ID 0
```

The run ID is shown in the host's Work details/status. For development use the
matching built private runtime. `owner` can replace DEVICE_ID only when explicitly
sharing with the current owner-token principal. Local administrative requests use
the existing private Work control token; the companion does not forward that
administrative endpoint. The opt-in internal command is routing, not the security
boundary: access to the private control credential is the local authority.

The principal is an opaque hash of the existing authenticated access identity,
which includes device identity, secret hash and owner-token generation. The
existing device registry remains authoritative; no new device database or pairing
system exists. Grants live only in the owning process (maximum 32, maximum 900
seconds), bind the full T0 target/owner/generation/incarnation, and authorize
only remote-human Inspect. Restart requires a fresh local grant. No grants are
issued by HTTP discovery, by conversation access, or to models/external backends.

Every gateway request reauthenticates using existing `companion/auth.mjs`, checks
again after its bounded serial queue, and revalidates before releasing a response.
Device revocation or owner-token rotation while a read is pending suppresses its
response. At the actual owner boundary, the handler checks the live context and
calls T0 `executiontarget.Check` against its current private grant under a mutex.
Grant revocation/expiry and owner generation changes therefore fail closed even
if an earlier UI read succeeded. Device authentication is enforced by the gateway;
the owner enforces target authorization. The owner trusts only the existing
private local control channel, never a phone-supplied actor or grant.

Inspection is read-only; no mutation journal is introduced or needed. T0 delivery
semantics and existing actionlog/native command receipts remain unchanged.

## Companion API and interface

Authenticated, Host/Origin-checked GET APIs:

- `/v1/work`: eligible, explicitly shared live target summaries.
- `/v1/work/TARGET?generation=GENERATION`: selected status and recent progress.
- `/v1/work/TARGET/activity?generation=GENERATION`: same bounded projection.

Other methods are rejected. No URL, socket, path, control token, arbitrary actor,
or grant is accepted from the remote client. Generation is mandatory for selected
reads. JSON is explicitly projected/validated and limited to 128 KiB by the gateway;
owner snapshots are limited to 64 KiB. The typed display contract is in
`app-host/contracts/inspection.ts`. Routes reuse the existing bounded serial queue
and authentication. Normal conversation routes retain their original behavior.

The mobile page has a compact **Active Work · read only** disclosure, task selector,
manual refresh, three-second polling while open/visible/online, conversation/run
association, timestamps, bounded public progress, specialist status, pending
interaction indicators, and explicit limitations. It uses plain text rendering.
Session changes/revocation clear private displayed work. Selection/request epochs
and aborts prevent late responses from overwriting another target. Offline/hidden
state and failed reads are marked stale; disappearing owners clear the selected
output and report unavailable. Existing chat drafts/navigation/approvals are
separate. There is no new approval submission or other control in this view.

## Boundaries and verification

Supported: live owned Work with the updated control channel; semantic specialist
status observed in its parent events. Unsupported: independent PTY attachment or
output, arbitrary native engine transcripts, external tmux/Ghostty, background jobs
without this control channel, completed/offline inspection replay, and all new
mutation endpoints. Completion closes the channel; the view reports unavailable
rather than guessing completed from stale history. Some early runs may have no
conversation association until Work creates it. Specialist summaries are capped;
this is not a complete native-engine inspector.

Tests in `companion/inspection.test.mjs` use real isolated Go UI-backend processes,
a held local fixture model, and the actual local grant command for desktop,
terminal and companion transport modes. They prove cross-process owner routing,
no extra workers/model calls, and stopped-owner handling at that boundary. They
are **not** full Electron/OpenTUI native application acceptance. Go tests cover
owner token/nonce/generation, scope/expiry/revocation, bounds and secret exclusion.
The Chromium browser fixture checks text rendering, polling, selection races,
offline behavior, narrow layout and revocation; it is not physical-phone acceptance.

Future T2 controls require separately authorized operations, exact payload review,
durable dispatch admission/uncertainty handling, and owner serialization. Do not
reuse this Inspect grant for input, cancellation, answers or approval. No T2 work
is enabled by T1.
