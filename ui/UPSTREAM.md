# OpenCode frontend provenance

Source: https://github.com/anomalyco/opencode

Revision: `f669c46c9604ea3d70b8a7ed1f0c10f3a5ac1f61` (26 October 2026),
`packages/tui` version 2.1.4. License: MIT; retained in `upstream/LICENSE`.

`upstream/` holds pristine source files used for this adaptation, including the
composition root, prompt, transcript, session frame/tabs, and old client/data
boundaries. `upstream/manifest.json` records their SHA-256 digests. These are
review references, excluded from the TypeScript and production build inputs.

`src/` is the maintained adaptation:

| Source | Treatment |
| --- | --- |
| ui/dialog, dialog-select, dialog-prompt, dialog-confirm, select-controller | Retained upstream components and selection behavior. Gator adds opt-in visible disabled choices to dialog-select, with keyboard/mouse activation refused; model availability reasons remain inspectable. |
| context/keymap, interactivity, clipboard | Retained upstream keymap/mode/selection infrastructure. Plugin keymap types extracted locally; upstream config keybind fallback replaced with Gator's. |
| ui/layout, border, animation, toast; util/scroll, selection, locale; theme/color | Retained frontend utilities. Gator supplies theme, clipboard, config, and errors. |
| component/command-palette | Adapted discovery to Gator operations; upstream config/plugin entries excluded. |
| component/session-tabs-rail | Retained upstream rail control and half-row drawing, fed by Gator sessions. |
| app, component/session-frame, session-tabs, prompt | Adapted composition/layout and prompt structure. Upstream API, execution, plugins, sharing, telemetry and service startup removed; Gator adapter supplies the data and actions. |
| context/client, context/data | Replaced with typed Gator client/data projections and managed subscriptions. |
| routes/session | Gator transcript/result/activity projection replaces upstream schema-bound message rendering. |
| config, context/theme | Gator-owned preferences and theme adapter. No upstream profile or plugin/config discovery. |

Dependency closure: Solid 1.9.15 (including upstream's cleanup patch), OpenTUI
core/solid/keymap 0.6.8, fuzzysort, remeda, string-width, their locked dependencies,
and Gator application-host. No `@opencode/core`, client, schema, server, agent,
plugin, or simulation package is in the production dependency graph. The existing
Go JSON-lines transport remains private and authoritative.

Selected initial runtime: Bun 1.4.2, with the native OpenTUI library loaded from
its locked platform package. Upstream used Bun 1.4.4; that version is not assumed
to be installed here. Solid 1.9.15 matches upstream despite OpenTUI's narrower
1.9.12 peer declaration; verify this combination with UI regressions and retain
the patch. `bun.lock` is the frontend lockfile; `app-host/package-lock.json` owns
shared Node-compatible services and Office dependencies. Native platform evidence
and final distribution selection remain recorded in the migration ledger.
Gator's host additionally owns PDF.js/native canvas and classic-level for bounded
legacy preference import. After changing its manifest, run
`bun update @gator/application-host` from `ui/` and inspect the lock diff; a forced
install alone can retain old local-package dependency metadata. Packaging verifies
the resolved production closure and exercises actual PDF/Office/preferences
services from the artifact before accepting it.

Update procedure: fetch a deliberate immutable revision, verify licensing and
dependency changes, refresh originals/hashes, review local adaptations against
the old/new originals, then run shared-host, adapter, renderer, actual PTY and
installed-artifact tests before updating this record. Never import upstream
execution/config defaults as an incidental dependency update.

Short-terminal adaptation: `ui/dialog-select.tsx` clamps the list budget and
post-ellipsis viewport to positive rows. `ui/dialog.tsx` uses top alignment below
24 rows and a positive width. The pinned formula produced a native `LayoutLimit`
at 42×14; `tests/hubs.test.tsx` retains this regression and keyboard reachability.
