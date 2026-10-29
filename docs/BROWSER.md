# Local controlled browser sessions

The terminal distribution exposes browser commands and a `/browser` panel. The shared application panel also appears in desktop command navigation. Source-checkout shell examples below use an explicit internal opt-in; see [development instructions](DEVELOPMENT.md).

Gator can control a real local Chromium session for browser-based development
and visual verification. It is not a hosted browser, a remote executor, or a
general computer-use surface. A browser is available to an agent only when the
developer explicitly starts or attaches a local session, selects specific tabs,
and grants that session to one Work run. The browser capability is separate
from ordinary HTTP research and does not imply desktop/app control.

## In the terminal or desktop application

Open `/browser` from the command palette. The shared panel provides:

1. **Install browser**: review the download before installing the pinned runtime.
2. **Open private browser**: a separate headed window starts paused, with
   screenshots off. Add allowed site origins, including required sign-in redirects.
3. **Private sign-in**: use that window yourself while access is paused. Gator
   does not ask for passwords or one-time codes in chat.
4. **Resume agent access**: review the selected tabs, sites and screenshot setting.
   Choose a different shared tab if needed.
5. **Use for next task**: review a separate task grant, then submit your request.
   The grant is consumed by one submitted or queued task in that chat. It does
   not carry into later prompts, new chats, or the Code specialist.
6. **Pause for private takeover**: available while work is running. Wait for the
   acknowledgement before entering private information. Resume explicitly.
7. **Stop browser**: revoke access and close a managed window, or disconnect an
   attached browser. If a controller cannot be reached, the panel offers explicit
   revocation and refresh. It does not claim an uncertain browser action failed.

Every request using the reviewed task grant checks the browser policy at the
controller. A change to selected tabs, allowed sites, upload registrations or
capture permission fails closed until access matches the reviewed policy. Queue
recovery preserves the grant and does not automatically replay interrupted work.
A live task's element references are invalidated across private takeover.

The browser panel shows the next-task selection; clearing it affects future
submission only. Pause or stop the session to block already submitted tasks.
Managed windows have temporary sign-in state; a new window starts fresh. Existing
attached sessions are manageable here, but attaching a CDP browser and registering
uploads still use the advanced CLI. This is not proof of a real account sign-in.

## Setup and lifecycle

Install the pinned Playwright package and Chromium explicitly. Gator never
downloads a browser while starting a run or opening the TUI:

```sh
GATOR_INTERNAL_COMMANDS=1 go run ./cmd/gator browser install
GATOR_INTERNAL_COMMANDS=1 go run ./cmd/gator browser start --visual-capture
GATOR_INTERNAL_COMMANDS=1 go run ./cmd/gator browser origins SESSION_ID add http://127.0.0.1:3000
GATOR_INTERNAL_COMMANDS=1 go run ./cmd/gator work --browser-session SESSION_ID --artifact review.md 'Test the approved local UI flow'
```

`browser start` creates a headed Chromium window by default with a new,
temporary profile. It selects its one fresh tab automatically. `--headed=false`
is useful for local CI or a container that already has Chromium dependencies.

To use an existing Chromium-family browser, start it yourself with a literal
loopback remote-debugging address, then attach and select tabs deliberately:

```sh
GATOR_INTERNAL_COMMANDS=1 go run ./cmd/gator browser attach --cdp http://127.0.0.1:9222
GATOR_INTERNAL_COMMANDS=1 go run ./cmd/gator browser tabs SESSION_ID
GATOR_INTERNAL_COMMANDS=1 go run ./cmd/gator browser select SESSION_ID TAB_ID [TAB_ID...]
```

Attached sessions never expose candidate tabs to the model. The CLI lists them
only for the developer; agent tools can see only selected tabs. `gator browser
stop SESSION_ID` closes a managed browser, disconnects an attached browser, and
revokes the local token. The application panel also manages these sessions;
creating an attached CDP session remains an advanced CLI operation.

List capture metadata with `gator browser artifacts SESSION_ID`; copy one to a
new developer-selected path without overwriting an existing file through
`gator browser export SESSION_ID ARTIFACT_ID --out /absolute/path`.

## Agent surface and consent

With a granted session, the Work manager receives selected-tab tools. An
internal Code specialist needs a separate `--code-browser-session SESSION_ID`
and its existing `--code-capability browser` grant; neither role can borrow the
other's browser authority.

- `browser_tabs`, `browser_snapshot`, and `browser_screenshot` read selected
  browser state. Screenshots need a separate session-level visual-capture
  grant and become bounded untrusted image observations for vision-capable
  providers; text-only models cannot request them.
- `browser_navigate`, `browser_click`, `browser_fill`, `browser_select`, and
  `browser_press` are remote mutations. Each uses a fresh developer approval;
  an “allow always” response is intentionally not remembered for browser
  actions.
- `browser_download` requires its own approval and writes only to Gator's
  private session artifact directory. Ordinary clicks reject downloads.
- `browser_upload` requires both a current approval and a file previously
  registered by the developer with `gator browser allow-upload`. Models use an
  opaque upload ID, never a local path.

Tools operate on short-lived element references from the latest accessibility
snapshot. Gator does not expose raw selectors, DOM evaluation, cookies,
storage, CDP endpoints, clipboard operations, browser permissions, extension
control, or arbitrary filesystem paths. Password, passkey, username,
one-time-code, autocomplete-sensitive, and file inputs cannot be filled by an
agent. Use the headed managed window to log in or take over manually.

## Private sign-in and takeover

Pause agent access **before** entering private information and wait for its
acknowledgement:

```sh
GATOR_INTERNAL_COMMANDS=1 go run ./cmd/gator browser pause SESSION_ID
# Sign in or make the intended change in the headed browser.
GATOR_INTERNAL_COMMANDS=1 go run ./cmd/gator browser resume SESSION_ID
```

Pause is serialized with browser operations. An already-running operation may
finish before the acknowledgement; after acknowledgement, all agent tab reads,
snapshots, screenshots and actions are rejected. Pause remains recorded if the
driver fails. Resuming requires a separate local request and invalidates every
old element reference before access opens again. A model cannot pause/resume
through its browser tools. Failed invalidation keeps access paused.

Approved origins remain in force during private takeover. Authentication
redirects may need additional explicit origin grants; pause does not grant new
network access. A stopped controller cannot be recovered merely by resuming it.
The `/browser` panel exposes private takeover and unavailable-session recovery.
Actual provider sign-in and redirect requirements still need real-account testing.

## Network and profile boundary

Every session begins with no permitted origins. The developer adds exact
origins through the CLI or TUI. Public origins are HTTPS on port 443;
`localhost`, `127.0.0.1`, and `::1` may use HTTP or HTTPS on an explicitly
approved port. Localhost is resolved before approval and must resolve only to
loopback; other private, link-local, multicast, file, data, and credentialed
URLs are rejected. The sidecar revalidates every routed hostname and aborts a
request that resolves to a non-public address (or outside loopback for a local
origin). Chromium does not expose a connection-pinning API, so this is a
revalidation control rather than a claim of DNS pinning.

The managed browser blocks service workers and routes navigation, redirects,
frames, scripts, images, fetch/XHR, and WebSockets through the same exact
origin policy. It closes popups and rejects unapproved downloads. This follows
Playwright's documented routing model and its service-worker caveat:
[network interception](https://playwright.dev/docs/network) and
[service workers](https://playwright.dev/docs/service-workers).

An attached browser is inherently less isolated: it uses the user's existing
Chromium profile and existing service workers cannot be disabled without
changing that profile. Gator applies selected-page routing and never exposes
unselected tabs, but an attached session is appropriate only for a browser
profile and origins the developer is willing to share. Managed temporary
sessions are the recommended authenticated or sensitive workflow.

Managed profile state (cookies, cache, storage, credentials, and temporary
downloads) is removed when the session stops. Gator never exports attached or
managed cookies/storage. Screenshot and download artifacts are local `0600`
files; session journals contain only normal tool-result metadata, not raw image
bytes, cookies, request headers, request bodies, CDP endpoints, or tokens.

## Runtime and limits

The runtime lockfile pins Playwright 1.56.1 and its Chromium revision under
Gator's private state directory. Installation requires locally installed Node
18+ and npm; it uses the checked-in lockfile and Playwright's explicit Chromium
installer. When a Gator update changes the embedded controller or lockfile,
`gator browser status` requires an explicit `gator browser install` refresh.
Gator does not currently bundle Node in release archives.

Browser requests have a 30-second controller/driver limit and also honor caller
cancellation. Stop can interrupt a stalled request. Losing an admitted browser
operation closes that engine, revokes its session and token, and discards its
references. Start a fresh private window and review access again; interrupted
actions are not automatically replayed and may already have affected a website.

Shutdown gives the driver two seconds to close its browser before forcing the
driver to exit. Profiles and downloads use an owned private scratch directory;
cleanup removes it even after forced driver termination. A short bounded retry
handles lingering native file handles; a cleanup failure is returned to the
daemon log. A host crash or killing the entire daemon bypasses this cleanup and
can still leave private scratch data. Stopping an attached session disconnects
it; the separately owned browser profile is retained.

Snapshots are bounded to 32 KiB of visible text and 128 visible actionable
elements. Screenshot observations are bounded to 2 MiB PNG; retained local
browser artifacts and downloads are capped at 8 MiB each, while registered
uploads are capped at 128 MiB. These bounds are enforcement limits, not a
claim that browser page content is safe: all page text, accessibility labels,
and screenshots are untrusted input to a model.
