# Self-hosted companion transport

Experimental single-owner transport for Gator conversations on an always-on
host. The host owns its files, configured accounts, models and execution. A
client disconnect does not stop admitted work. This does not migrate laptop
work or make an offline laptop's files accessible.

This checkpoint supplies an authenticated API and a small phone/browser client
using the existing Go bridge. Device pairing, per-device revocation, file transfer,
automated deployment and remote account onboarding remain unfinished. Keep Electron and
the terminal available for host setup and recovery. The terminal parity gate is
unchanged. Do not expose this listener directly to the internet.

## Start on a configured host

Use Node 22.12+ and a built Gator runtime. Configure the host's own profile and
create the conversations to expose using Gator on that host. Use a dedicated
profile and retain its `GATOR_CONFIG_DIR` and `GATOR_STATE_DIR` for the companion.
The process serves one owner; possession of its token gives access to that
profile's supported conversation operations.

```sh
mkdir -m 700 /private/path/gator-companion
node companion/token.mjs /private/path/gator-companion/access.token
node companion/main.mjs /path/to/gator-runtime /path/to/host/workspace \
  /private/path/gator-companion/access.token 7341
```

The listener binds only `127.0.0.1`. Use an SSH tunnel, or an HTTPS reverse proxy
on the execution host that preserves the Authorization header. Never send the
bearer token over an unencrypted remote connection or put it in a URL. For a
same-origin browser client, set `GATOR_COMPANION_ORIGIN` to its exact HTTPS
origin (no trailing slash). Preserve that Host header or the listener's exact
`127.0.0.1:PORT` header through the proxy. Other hosts and browser origins are
rejected. No permissive CORS is enabled.
Token creation refuses overwrite; rotation atomically replaces an existing token:

```sh
node companion/token.mjs /private/path/gator-companion/access.token --rotate
```

Rotation revokes future requests from all previous clients, including requests
waiting for dispatch. It does not undo actions already admitted. Individual
device enrollment/revocation is not implemented yet. Keep token directories
private; macOS/Linux permissions and symlink checks are enforced. Native Windows
ACL and deployment acceptance remain pending.

## Open on a phone or browser

Open the configured HTTPS origin on your phone, or `http://127.0.0.1:7341` on
the host. Enter the host token to browse existing conversations. Search titles
and host folder paths, select active/archived/all chats, and use the page controls
to reach older records. You can also expand **Open a known conversation** and
enter its ID. **New chat** creates an empty conversation in the host workspace
shown above the button. Work starts only when you send a message. The shell is
public; every conversation, creation and catalog request requires
the token. The token stays in memory and is cleared on disconnect/reload.

The client displays messages, updates, queue state and exact pending approval
details. Result fields, previews and file/action receipts are available as plain
text; Office rendering and file download remain host operations. Approving
requires opening the current preview and explicitly confirming it. A changed
preview closes the confirmation. Check the resulting receipt to establish the
outcome; acknowledgement alone is not successful delivery.

Draft text and unconfirmed message IDs use this tab's session storage. If a send
loses its acknowledgement, reconnect and use **Retry same message**. Reloading
retains the exact payload/ID so Go can deduplicate it. Unconfirmed text stays
locked until acknowledged; don't resubmit it as new work elsewhere. Requests
are never automatically resent. Closing the tab may discard this browser's
draft; admitted host work remains retained. Disconnect does not delete a draft.
The client polls while visible and disables actions when disconnected, hidden,
or refreshing. Token revocation clears the displayed conversation.

**Conversations** returns to the list without stopping admitted work. Switching
chats retains each chat's draft and any unconfirmed message identity separately.
Opening a chat does not send its draft or resume interrupted work. Catalog reads
use Gator's existing bounded local catalog; they do not call a model or create
a conversation. Search covers titles and folders, not message contents. Partial
or skipped catalog records are disclosed. Refresh retries the same search after
a failure. The list is refreshed explicitly and when the page becomes visible;
an open chat continues polling. Token revocation is enforced on every request
and clears displayed data when the client next contacts the host.

New-chat requests are saved in this tab before dispatch. If the connection drops
or the chat cannot open yet, **Retry new chat** keeps the same creation request
across reloads and host restarts. It never sends a message. **Dismiss retry** only
clears this browser's pending retry; any created chat stays in Conversations.
A changed host workspace requires dismissing the old retry before starting a
different chat. The phone cannot choose an arbitrary host filesystem path.

## API v1

All API requests need `Authorization: Bearer TOKEN`. Responses are JSON with
`Cache-Control: no-store`.

- `GET /v1/chats/CONVERSATION_ID`: open an existing conversation and read its
  current Go display state. It never retries interrupted work.
- `POST /v1/chats/CONVERSATION_ID/actions`: send a supported frontend frame with
  `Content-Type: application/json`. The body limit is 256 KiB. Response includes
  the latest state; transport acknowledgement alone is not task success.
- `GET /v1/chats?q=TEXT&scope=active&after=CURSOR`: read up to 50 catalog entries.
  All parameters are optional; scope is `active`, `archived` or `all`. The response
  contains `items`, an optional `next` cursor, and any `incomplete`/`skipped`
  indicators. Cursors belong to their original search. Unknown or repeated
  parameters are rejected. No host filesystem path can be supplied as a source.
- `POST /v1/chats`: create an empty chat with JSON
  `{"request_id":"client-generated-unique-id"}` (16–96 ASCII letters, digits,
  underscores or hyphens, starting with a letter/digit; UUIDs are supported).
  No other fields are accepted. The body limit is 4 KiB. The response contains
  `request_id`, `conversation_id` and `source`. Creation does not open a worker
  or submit work; use the existing conversation GET to open it. Retain the same
  request until creation and opening are confirmed.

A message uses the durable upload-admission protocol, initially with no files:

```json
{"type":"upload_submit","upload":{"request_id":"client-generated-unique-id","context":"conversation:WORK_ID","text":"What changed?","files":[]}}
```

Replace `WORK_ID` with the exact conversation ID in the URL. Keep the same
`request_id` and identical payload until admission is confirmed, including after
connection loss. Go's journal acknowledges a retry instead of executing twice.
Changing payload under an existing admission ID is rejected by Go.

Supported frames also include `approval`, `queue`, `update` and `receipt`.
They retain the current protocol's exact live approval token, review identity,
version and conversation checks. Clients must show the exact current approval
preview before sending an approval. Historical approval text is not authority.
A client should inspect the corresponding Go result/receipt after acknowledgement.
Setup, navigation, filesystem selection, external-program handoffs, attachments
and profile maintenance are not yet exposed by this transport.

Follow-up cards offer **Mark read**, **Done** and **Reopen**, plus **Earlier
updates** and **Newer updates** for retained history. Read status and resolution
are separate from the check's result: marking a failed check's update done does
not make the check successful, retry it, or authorize an external action. Details
show the next step, evidence and update identity as plain text. Merely opening
the page does not mark an update read or done.

Each status change uses the current conversation, update identity and resolution
version. A stale change is rejected and shown with the refreshed status. If its
response is lost, refresh and inspect the saved status before trying again; the
client never automatically replays the change. Decisions are retained on the
host across companion restarts. Polling preserves expanded details and keyboard
focus. Replying with an update reference and rating its usefulness still require
Gator on the host.

The host allows four open conversation processes and at most three executing
chats. The dispatch queue is bounded. Opening another chat can retire an idle
process. Running work, queues, approvals, setup panels and local operations are
never evicted. If all four processes have pending work, finish or review it before
opening another chat. Crashed processes still require host recovery. Stop/restart
the companion deliberately to release all its processes.
Stopping the companion joins/stops its children using the existing backend
lifecycle. On restart, admitted interrupted work requires explicit recovery.

## Monitoring and availability

Configured recurring checks still run through Gator's existing job supervisor
on this execution host. Start or install it through the host's reviewed Recovery
controls, using the same profile and a stable runtime location. The companion
API does not install or start a supervisor implicitly. Its availability is
separate from the job supervisor's availability. Client-off completion requires
the execution host to stay online. Mobile notifications and offline client
synchronization remain pending.

## macOS launch at login

`macos-service.mjs` prepares a reviewable per-user LaunchAgent. It validates the
runtime, profile, workspace, private token and log paths, refuses to overwrite
an existing plan, and does not start a service. Copy `macos.example.json`, replace
its paths with a stable installation and already-configured dedicated profile,
and create the referenced directories first. The installation must retain
`companion/` and its sibling `app-host/src/` modules. Do not point at temporary
test builds or move the installation after activation. Use a stable Node path.

```sh
node companion/macos-service.mjs /private/path/companion.json /private/path/com.gator.companion.plist
plutil -lint /private/path/com.gator.companion.plist
```

Review the generated arguments/profile paths. When ready to enable it, place
the plist at `~/Library/LaunchAgents/com.gator.companion.plist`, retaining mode
0600 and without overwriting another service. Activation and removal are
explicit operations:

```sh
launchctl bootstrap "gui/$(id -u)" "$HOME/Library/LaunchAgents/com.gator.companion.plist"
launchctl print "gui/$(id -u)/com.gator.companion"
# Stop the service before removing its owned plist:
launchctl bootout "gui/$(id -u)/com.gator.companion"
```

Launchd restarts the listener after exit, with a ten-second throttle. It does
not replay interrupted Go work. This is a login service: the Mac must be awake,
online and logged in. It does not keep a sleeping Mac awake, configure HTTPS,
enroll a phone, or start Gator's separate job supervisor. A permanent companion
should use the same dedicated profile as the reviewed supervisor configuration.
Logs stay in the private directory; log rotation is not automated yet.

## Linux host service

`linux-service.mjs` prepares a per-user systemd unit for an always-on Linux host.
Use a dedicated ordinary user and an already-configured host profile. Copy
`linux.example.json`, replace its paths, create the referenced directories and
private token, and use a Linux runtime built for that host. Retain `companion/`
and its sibling `app-host/src/` together in a stable installation. Node 22.12+
and systemd with `Type=exec` support are required.

```sh
node companion/linux-service.mjs /home/gator/companion.json /home/gator/gator-companion.service
systemd-analyze --user verify /home/gator/gator-companion.service
```

Preparation validates the executable, workspace/profile directories and private
token, writes a new mode-0600 file, and refuses overwrite. It does not install,
start or enable anything. Review the file, including the selected profile and
optional exact HTTPS origin. The token itself is not embedded. The listener
still binds only loopback; configure the tunnel or HTTPS proxy described above.

As the dedicated user, install the reviewed file only if that unit name is free:

```sh
mkdir -p "$HOME/.config/systemd/user"
# Refuse an existing unit or symlink. Link the reviewed file without replacing it.
ln /home/gator/gator-companion.service "$HOME/.config/systemd/user/gator-companion.service"
systemctl --user daemon-reload
systemctl --user enable --now gator-companion.service
systemctl --user status --no-pager gator-companion.service
journalctl --user -u gator-companion.service -n 50 --no-pager
```

For operation after logout and at boot, the host administrator must enable
lingering for this dedicated user, for example `sudo loginctl enable-linger gator`.
Check it with `loginctl show-user gator --property=Linger`. This is a separate
host policy change; the generator never performs it. Lingering keeps the user
manager available after logout, as described in the
[systemd login manager documentation](https://www.freedesktop.org/software/systemd/man/latest/loginctl.html).
It cannot keep a powered-off host running. Disable lingering only if no other
services for this user need it.

Install Gator's separate job supervisor through Recovery on this Linux host with
these same state/configuration paths. The listener alone does not execute
recurring checks. Both services must be enabled and the execution host online;
the phone and original laptop can then disconnect. Host files and credentials
remain on this host; there is no automatic laptop migration.

The unit restarts a failed listener after ten seconds. Stop first sends SIGTERM
to the listener so it can drain its children; after thirty seconds systemd kills
remaining processes in the service. This does not replay interrupted Go work.
Output goes to the host's journal under its configured retention policy. Review
it for startup failures; a successful service start alone is not an authenticated
application health check. Exact argument handling and restart/stop behavior use
[systemd service semantics](https://www.freedesktop.org/software/systemd/man/latest/systemd.service.html).

To remove this reviewed service, stop it before removing its unit:

```sh
systemctl --user disable --now gator-companion.service
rm "$HOME/.config/systemd/user/gator-companion.service"
systemctl --user daemon-reload
```

Keep the profile and token directories. The supervisor is managed separately.
For an installation move, stop the listener, generate and review a new unit with
the new executable paths, replace only the owned unit, reload and start again.
A stopped listener does not stop the separate supervisor.

## Verification

From the repository root:

```sh
node --test companion/server.test.mjs
# After installing the desktop dependencies and Playwright browsers:
GATOR_COMPANION_BROWSERS=chromium,webkit node --test companion/client.test.mjs
# On a logged-in Mac: temporary service, isolated profile, automatic removal.
GATOR_COMPANION_LAUNCHD_TEST=1 node --test companion/macos-service.test.mjs
```

Tests build the real Go runtime, use a disposable profile and local model fixture,
then exercise authentication, origin and conversation binding, client disconnect,
duplicate admission, token revocation, restart and non-replay. No real accounts
or paid provider requests are used. This does not establish remote deployment,
real-account action delivery or mobile usability. Browser tests use a deterministic
host fixture to exercise reload after a lost acknowledgement, stable admission
identity, current approval review/replacement/denial, plain-text untrusted content,
offline behavior, narrow layouts and revocation. A simulated phone viewport is
not physical iOS/Android acceptance.

Catalog checks cover real-Go pagination and search without a model call, bounded
HTTP queries, revocation during a read, and safe idle-process retirement. Browser
checks cover title/folder search, archived scope, previous/next pages, empty and
failed results, separate drafts across switches, retained retry identity, keyboard
selection, 44-pixel controls, and clearing private rows after revocation. Build
the updated Go runtime together with the companion; older runtimes do not expose
the internal `companion-catalog` transport.

Creation checks also exercise concurrent Go processes, interrupted publication,
deleted-chat refusal, backup/restore, and companion restart. Creation receipts
live under retained conversation history and keep a workspace fingerprint, not
the raw workspace path or chat text. Deleting an individual chat leaves its
receipt and rejects an old retry; deleting all history also clears receipts and
their deduplication guarantees. After a history reset, dismiss old browser
creation retries. Source relinking does not rewrite the original creation
request's workspace binding. Interrupted publication fixtures test persisted
boundary states; they are not a physical power-loss test.

The opt-in launchd test checks plist validity, startup, authentication after a
forced listener crash/restart, and asynchronous service removal. It runs a
temporary copied listener, never opens a conversation and does not invoke a
model or real account. Real-Go work durability is exercised by `server.test.mjs`.

Follow-up checks cover 55 real-Go retained updates, paging, conversation binding,
read/done/reopen, stale resolution refusal and receipts across restart, without
model calls. Browser journeys cover failed-check labeling, lost-response recovery
without replay, unavailable storage, retained drafts, keyboard focus, narrow
layouts, offline controls and access revocation. These are local fixture results,
not evidence of physical-phone or real-account acceptance.
