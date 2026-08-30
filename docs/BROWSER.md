# Local controlled browser sessions

Gator can control a real local Chromium session for browser-based development
and visual verification. It is not a hosted browser, a remote executor, or a
general computer-use surface. A browser is available to an agent only when the
developer explicitly starts or attaches a local session, selects specific tabs,
and grants that session to one Execute-mode run with `--network allow`.

## Setup and lifecycle

Install the pinned Playwright package and Chromium explicitly. Gator never
downloads a browser while starting a run or opening the TUI:

```sh
gator browser install
gator browser start --visual-capture
gator browser origins SESSION_ID add http://127.0.0.1:3000
gator run --network allow --browser-session SESSION_ID --verify 'go test ./...' 'Test the local UI flow'
```

`browser start` creates a headed Chromium window by default with a new,
temporary profile. It selects its one fresh tab automatically. `--headed=false`
is useful for local CI or a container that already has Chromium dependencies.

To use an existing Chromium-family browser, start it yourself with a literal
loopback remote-debugging address, then attach and select tabs deliberately:

```sh
gator browser attach --cdp http://127.0.0.1:9222
gator browser tabs SESSION_ID
gator browser select SESSION_ID TAB_ID [TAB_ID...]
```

Attached sessions never expose candidate tabs to the model. The CLI lists them
only for the developer; agent tools can see only selected tabs. `gator browser
stop SESSION_ID` closes a managed browser, disconnects an attached browser, and
revokes the local token. The TUI exposes the same operations with `/browser`:
`start`, `attach`, `tabs`, `select`, `use`, `origins`, `visual`, `upload`,
`artifacts`, `stop`, and `none`.

List capture metadata with `gator browser artifacts SESSION_ID`; copy one to a
new developer-selected path without overwriting an existing file through
`gator browser export SESSION_ID ARTIFACT_ID --out /absolute/path`.

## Agent surface and consent

With a granted session, Execute-mode native runs receive selected-tab tools:

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

## Network and profile boundary

Every session begins with no permitted origins. The developer adds exact
origins through the CLI or TUI. Public origins are HTTPS on port 443;
`localhost`, `127.0.0.1`, and `::1` may use HTTP or HTTPS on an explicitly
approved port. Localhost is resolved before approval and must resolve only to
loopback; other private, link-local, multicast, file, data, and credentialed
URLs are rejected.

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
installer. Gator does not currently bundle Node in release archives.

Snapshots are bounded to 32 KiB of visible text and 128 visible actionable
elements. Screenshot observations are bounded to 2 MiB PNG; retained local
browser artifacts and downloads are capped at 8 MiB each, while registered
uploads are capped at 128 MiB. These bounds are enforcement limits, not a
claim that browser page content is safe: all page text, accessibility labels,
console-visible output, and screenshots are untrusted input to a model.
