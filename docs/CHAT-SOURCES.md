# Conversation source selection

The Electron composer’s **Sources** button opens **Add sources**. It lists only the configured, supported Gator connections. Selected account labels appear beside the button in wider windows; the source count remains available in compact windows. The menu supports keyboard navigation, Escape/focus return, refresh, clearing selections and **Manage connections**.

Connecting an account and selecting it for a request are separate actions. Selection does not authenticate, widen tool permissions, authorize writes or imply live access. A saved credential is shown as saved, not verified. Missing/expired credentials and action-only connections cannot be newly selected as read sources. Existing invalid selections can be cleared. Removed accounts are never substituted.

Opening or refreshing the menu performs local metadata reads only. It never calls the service, starts a model, downloads data or executes tools. **Settings → Connections → [account] → Check account & read access** is the explicit network check. Its result is bound to the descriptor and credential fingerprint, carries a timestamp, becomes stale after five minutes, and is discarded if account configuration or credentials change. These observations are held only by the current host process; after restart the status correctly returns to unchecked. Even a recent successful check is not proof of future availability or permission for unrelated operations.

**Settings → Connections → Advanced connections** contains service-token setup for the supported Slack, Notion, Atlassian and JSON API adapters. The existing reviewed token/resource workflow remains authoritative. Google setup follows the actual build’s bundled-client availability or explicitly explains the requirement for an operator-supplied desktop OAuth client. No Box, Canva, Dropbox or other integration is represented as implemented by this UI. Mapped MCP configurations retain their existing setup and policy requirements; the source menu introduces no general MCP trust toggle.

## Backend boundary

`internal/workapp/composer_sources.go` owns the bounded display projection and finite refresh/select actions. The catalogue has at most 128 visible entries, with at most 16 selected connections, matching the native connector-tool limit. The opaque selection identity binds the exact host conversation, source folder, selected IDs, display metadata and host-only account fingerprints. The browser never receives those fingerprints, credentials, OAuth client details, executable paths or endpoints.

`cmd/gator/work_composer_sources.go` reads existing settings/auth stores and uses `worksession.Preferences` with compare-and-swap persistence. The owner refreshes the catalogue before every selection and rejects stale identities, unknown/unavailable/repeated IDs and external changes to saved selections. No new registry, permission store, daemon or authentication system was added.

Selections apply to subsequent submissions, including queued messages submitted after the selection. Running requests and previously admitted queue entries keep their frozen options. The UI disables submission while a selection is awaiting acknowledgement. If acknowledgement is missing, it requires refresh before submission; it does not automatically resend a selection. Source choices are separate from model/effort defaults and do not change an executing request. App-side connector changes still use their existing reviewed/CAS path.

## Focused verification

- Go unit and race tests for `internal/workapp` and `cmd/gator`, filtered to source/composer/connection cases: PASS. Coverage includes old account bindings, expiry, revocation, removal, stale checked access, wrong chat, bounded refresh, persistence failure, concurrent preference changes and frozen queue options.
- `node --test app-host/tests/policy.test.mjs`: 12 passed. Renderer source actions accept only finite IDs, request identities and select/refresh operations; additional authority/configuration is rejected.
- `npm run check --prefix desktop`, `npm run build --prefix desktop`, and final `build:ui`: PASS.
- Real Electron journeys (`app-control.spec.ts` / `connections.spec.ts`, filtered to composer sources and native account setup): 3 passed. These cover per-chat selection/return, draft preservation, removal, clearing, original durable running/queued options, advanced token setup, explicit live-read check against a private fixture, and simulated OAuth browser return. No commercial API or real external account was used.
- `git diff --check` and Go formatting: clean.

The first source UI run found two test expectation mismatches: a stale catalogue is rejected before checking individual IDs, and a newly added descriptive label changed an exact button locator. The test expectation and redundant row description were corrected; all three final journeys passed. Whole-suite regression acceptance is still required after the remaining UX stages. Physical account/device and signed-installer acceptance are not claimed.

## Final regression acceptance

The later whole-application run passed: **81 Electron journeys, 2 opt-in live API journeys skipped**. Go, race, Node, typed bridge, build and companion regressions passed as recorded in [Chat UX acceptance](CHAT-UX-ACCEPTANCE.md). Earlier stage-specific counts and pending statements above are historical checkpoints. No additional paid evaluation, real-account mutation or physical-device acceptance is inferred.
