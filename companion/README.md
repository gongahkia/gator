# Self-hosted companion transport

Experimental single-owner transport for Gator conversations on an always-on
host. The host owns its files, configured accounts, models and execution. A
client disconnect does not stop admitted work. This does not migrate laptop
work or make an offline laptop's files accessible.

This checkpoint supplies an authenticated API using the existing Go bridge.
Phone/browser UI, device pairing, per-device revocation, file transfer, service
installation and remote account onboarding remain unfinished. Keep Electron and
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
future same-origin browser client, set `GATOR_COMPANION_ORIGIN` to its exact HTTPS
origin; other browser origins are rejected. No permissive CORS is enabled.
Token creation refuses overwrite; rotation atomically replaces an existing token:

```sh
node companion/token.mjs /private/path/gator-companion/access.token --rotate
```

Rotation revokes future requests from all previous clients, including requests
waiting for dispatch. It does not undo actions already admitted. Individual
device enrollment/revocation is not implemented yet. Keep token directories
private; macOS/Linux permissions and symlink checks are enforced. Native Windows
ACL and deployment acceptance remain pending.

## API v1

All requests need `Authorization: Bearer TOKEN`. Responses are JSON with
`Cache-Control: no-store`.

- `GET /v1/chats/CONVERSATION_ID`: open an existing conversation and read its
  current Go display state. It never retries interrupted work.
- `POST /v1/chats/CONVERSATION_ID/actions`: send a supported frontend frame with
  `Content-Type: application/json`. The body limit is 256 KiB. Response includes
  the latest state; transport acknowledgement alone is not task success.

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

The host allows four open conversation processes and at most three executing
chats. The dispatch queue is bounded. Stop/restart the companion deliberately to
release these processes; there is no automatic eviction of active or queued work.
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

## Verification

From the repository root:

```sh
node --test companion/server.test.mjs
```

Tests build the real Go runtime, use a disposable profile and local model fixture,
then exercise authentication, origin and conversation binding, client disconnect,
duplicate admission, token revocation, restart and non-replay. No real accounts
or paid provider requests are used. This does not establish remote deployment,
real-account action delivery or mobile usability.
