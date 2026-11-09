# Mobile access after the terminal experience audit

Research dates: 9–10 November 2026. Status: recommendation and queued implementation,
not a completed mobile product or a deployed service.

The user's follow-up authorizes researching and implementing a logical mobile
surface **after all fifteen terminal audit criteria are satisfied**. Keep that
sequence. The CLI/TUI remains the primary local application; an optional mobile
companion does not restore Electron, a desktop GUI or a mascot. Do not push,
publish, deploy, create accounts or send Telegram messages as part of this phase.

## Recommendation

Build the main mobile experience as a responsive web companion, then make it an
installable PWA. Reuse the existing `companion/` transport and authoritative
Go/host execution services. Telegram is a useful optional channel for quick
capture, short replies and alerts; it should not become a second agent or the
only way to inspect consequential actions. A Telegram Mini App could reuse the
same web interface later. This ordering is a product/engineering recommendation
based on the sources and repository inspection below, not a platform requirement.

| Surface | Fit for Gator | Tradeoff / recommended role |
| --- | --- | --- |
| Mobile web / PWA | Full conversation, editable drafts, Following, inspectable memory, exact reviews, files/results and scoped settings in one interface | Best primary mobile surface. Requires touch/keyboard/accessibility work, secure remote connectivity and host availability. |
| Telegram bot | Quick requests, voice messages, updates and brief replies with buttons | Useful optional messaging channel. Telegram carries the messages; complex reviews and document editing need richer surfaces. |
| Telegram Mini App | A web interface opened within Telegram | Reuse the web companion if later wanted; it still needs a hosted web endpoint and Telegram identity validation. |
| Phone SSH/Mosh client | Access to the actual existing terminal application | Power-user fallback; terminal keys, narrow screens and a retained host session are less approachable than touch controls. |
| Native iOS/Android apps | Strong OS integration and app-store distribution | Defer until demonstrated web limitations justify separate app packaging and platform maintenance. |

OpenCode's own documentation also describes a browser UI sharing its server and
session state with terminal clients. That supports the architectural direction;
its coding UI/API is not a drop-in replacement for Gator's Go services. Reuse the
shared-client pattern and Gator's existing companion, rather than introducing an
OpenCode execution engine alongside Work. Command names differ across OpenCode
documentation versions, so revalidate any setup instructions during implementation.
[OpenCode web/client-server documentation](https://opencode.ai/docs/cli/web/)

## Repository evidence

Inspected at `dedbd3e35`; presence of code is not current end-to-end verification.

- `companion/server.mjs` already serves a small browser client and authenticated
  conversation endpoints through the shared `app-host/src/backend.mjs` Go bridge.
  It enforces loopback binding, allowed Host/Origin, device/owner-token checks,
  bounded admissions, revocation and explicit recovery of uncertain actions.
- `companion/auth.mjs` and `devices.mjs` provide existing device enrollment and
  revocation primitives. The current exported bearer-token workflow is not a
  finished phone pairing experience or per-device restricted-role system.
- `companion/public/` already has conversation, update, Work inspection/control
  and separately authorized external-terminal views. Its asset allowlist has no
  web manifest or service worker. The browser currently rejects remote file
  attachments, uses tab session storage for drafts and does not supply Web Push.
- The companion starts managed Go backends, but does not use the terminal host's
  new dialogue dispatch/configuration path. Audit ownership and protocol parity;
  do not claim that the new terminal conversation-during-work flow already works
  remotely just because both surfaces call `ui-backend`.
- `docs/EXECUTION-TARGETS-T7.md` records controlled local/browser acceptance and
  explicitly leaves physical-iPhone acceptance unperformed. The older companion
  README contains historical Electron/terminal-retirement language; use current
  code and `docs/CLI-TUI-PORT.md` for the current product boundary. No README change
  is part of this research.

## Platform findings

**Installability and notifications.** A web manifest describes the installable
app; support and installation prompts differ by browser. Service workers can
provide offline behavior, but are not an unconditional requirement for every
installation route. [MDN installability](https://developer.mozilla.org/en-US/docs/Web/Progressive_web_apps/Guides/Making_PWAs_installable)
Apple documents Web Push for iPhone/iPad Home Screen apps from iOS/iPadOS 16.4.
WebKit also documents Home Screen app support in Safari 26. Permission requests
must be tied to an explicit user gesture; notification delivery still needs a
working server-side push implementation. [Apple Web Push](https://developer.apple.com/documentation/usernotifications/sending-web-push-notifications-in-web-apps-and-browsers),
[WebKit Safari 26](https://webkit.org/blog/17333/webkit-features-in-safari-26-0/),
[WebKit permission behavior](https://webkit.org/blog/13878/)

**Offline does not mean unattended execution on a phone.** Service workers have
limited, browser-managed lifetimes. Use them for an optional cached shell,
explicit draft retention and notifications, not as Gator's scheduler or a
permanently running agent. Work stays with its real execution host.
[MDN offline/background operation](https://developer.mozilla.org/en-US/docs/Web/Progressive_web_apps/Guides/Offline_and_background_operation)

**Telegram can work without a public inbound listener.** The Bot API supports
either long polling or webhooks. Long polling is attractive for a self-hosted
Gator because the host initiates the connection; that is an engineering inference
from the API. Persist incoming update/admission identities before acknowledging
them. Telegram retains unreceived bot updates for at most 24 hours. Ordinary
message text is limited to 4,096 characters; the standard hosted API's file
download limit is 20 MB. These are adapter constraints, not reasons to truncate
an exact approval and treat it as reviewed. [Bot API](https://core.telegram.org/bots/api),
[Bots FAQ](https://core.telegram.org/bots/faq)

**Telegram privacy and richer UI.** Bots support text, files and voice messages,
but Telegram is another party in that message path. Its privacy policy describes
server storage for cloud chats. This differs from connecting a phone directly to
a privately hosted Gator endpoint. Mini Apps provide a richer web interface;
their `initData` must be validated on the backend, rather than trusting
`initDataUnsafe`. Telegram identity does not itself grant Gator account/file or
action permissions. [Bot features](https://core.telegram.org/bots/features),
[Telegram privacy](https://telegram.org/privacy),
[Mini Apps](https://core.telegram.org/bots/webapps)

**Private connectivity.** Tailscale Serve can proxy a local HTTP port over HTTPS
to devices within a tailnet. Funnel instead exposes a service to the public
internet. Start with a private connection for self-hosted users and retain
Gator's own pairing, revocation and action checks. A general-public distribution
will need a separately reviewed connectivity/onboarding strategy; Tailscale is
an option, not a required dependency for ordinary local use.
[Tailscale Serve](https://tailscale.com/docs/features/tailscale-serve),
[Funnel distinction](https://tailscale.com/docs/reference/funnel-vs-sharing)

Mosh supports roaming/intermittent terminal connections, making it a plausible
advanced-client option. Actual Gator key/rendering compatibility on a physical
phone remains to verify. [Mosh](https://mosh.org/)

## Implementation sequence and acceptance

After the fifteen-item terminal ledger is complete:

1. Revalidate the companion against the final CLI/TUI protocol and actual owner
   lifecycle. Share the established services; do not copy the execution loop,
   settings store, scheduler or approval authority into a mobile-specific agent.
2. Provide an understandable host pairing/revocation flow and truthful status:
   phone connected, host reachable, model ready, and monitored commitments are
   separate facts. Preserve existing token/grant and Host/Origin safeguards.
3. Build a calm conversation-first touch UI using the same identities and
   permissions: converse/steer/stop during work, Following, memory correction,
   scoped settings, exact reviews, actionable results and original file context.
   Prioritize the phone keyboard, safe areas, back restoration and readable long
   reviews. Do not port terminal key chords as the only touch controls.
4. Add install metadata and an explicit update flow. Keep private conversation
   APIs/approval previews out of a generic service-worker cache. An offline view
   must show retained/stale state and preserve drafts without executing or blindly
   resending uncertain requests after reconnect/restart or service-worker updates.
5. Add scoped notification subscriptions, useful deep links and per-device
   revocation. Consent, quiet/noise controls, push-service data exposure and actual
   delivery outcomes must be visible. Push acceptance is not proof a person read
   an update or an external action completed.
6. Verify with controlled Go/provider integration, mobile browser rendering and
   physical iPhone/Android checks. Exercise pairing, reconnect, lost admission,
   stale reviews, files, voice-to-editable-draft, tab/background lifecycle,
   notifications, revoked devices and host restart/sleep. Distinguish emulation
   from physical-phone and live-account evidence.

Both the PWA and a Telegram adapter need a reachable execution host. If that host
is an asleep/offline laptop, its local files, models and tasks are unavailable.
An always-on computer/server is a deployment choice, not a capability conferred
by installing a PWA or creating a bot. No remote deployment or account setup was
performed for this research. Telegram remains an optional follow-on rather than
an automatically included second frontend.
