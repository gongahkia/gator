# Bounded document browser

Gator's native browser tools are a deliberately small HTTPS document
automation surface. They are available only in Execute mode when the developer
has granted network access:

- `browser_navigate` loads one approved public HTTPS main document.
- `browser_snapshot` returns bounded normalized visible text and stable
  `link-N` / `form-N` references from the current document.
- `browser_extract` filters text, links, or forms without a network request.
- `browser_act` follows one link or submits one GET/POST form after a fresh
  destination-and-method approval.

A narrowing profile or role may omit `browser` while retaining `http_fetch`
and `web_search`. Omitting `http` removes the complete native web surface.

This is not Chromium, WebKit, computer use, or an authenticated browser profile.
There is one ephemeral in-memory document per run. Gator does not execute
JavaScript, load images/styles/scripts or other subresources, register service
workers, open WebSockets/popups/tabs, follow redirects, use cookies, download
files, persist storage, take screenshots, or expose arbitrary selectors/DOM
evaluation.

## Network boundary

Every transition uses the same transport as `http_fetch`:

1. Parse only `https` URLs without userinfo and with port 443.
2. Resolve the destination immediately before the request.
3. Reject the entire DNS answer set if any address is local, private,
   link-local, reserved, documentation, multicast, or otherwise non-public.
4. Disable environment proxies and pin the connection dialer to the approved
   host and resolved addresses.
5. Do not follow redirects. A redirect is returned as untrusted data and does
   not replace the current document.

Because the document browser never loads subresources or executes script, a
page cannot create a second hidden request. This narrower design avoids the
request-interception gaps that full browser engines must close. If Gator later
adds Playwright/Chromium, every request type—including frames, fetch/XHR,
images, scripts, WebSockets, and popups—must pass equivalent destination
validation, and service workers must be blocked. Playwright documents that
context routing does not see service-worker-intercepted requests unless service
workers are disabled:
<https://playwright.dev/docs/api/class-browsercontext#browser-context-route>.

## Approval and action boundary

Approval identity is the exact HTTP method plus canonical destination. Browser
allow-always memory is run-local and separate from local-command memory.
Approval events never include form values.

Forms expose field names and types, not values from the page. The model must
supply every submitted value explicitly. Password and file fields are refused,
request bodies are bounded, and methods other than GET/POST are unsupported.
This prevents the browser from becoming a credential extractor or arbitrary
HTTP client. POST is still potentially destructive; the developer must inspect
the method and destination in the approval prompt.

## Bounds and untrusted content

Browser traffic shares the eight-request web budget with `http_fetch` and
`web_search`. Responses use the configured 256 KiB default / 512 KiB maximum
body limit. Snapshot visible text is capped at 32 KiB and link/form metadata at
24 KiB; references and labels have independent limits. Parsed page text, link
labels, URLs, and form metadata are untrusted data, never instructions.

The state is in memory only and disappears when the run ends. There is no
restart recovery because there are no credentials, cookies, or browser
processes to recover.
